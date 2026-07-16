package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	goruntime "runtime"
	"strings"

	"github.com/pelletier/go-toml/v2"
	json5 "github.com/yosuke-furukawa/json5/encoding/json5"
)

// ToolDriver abstracts downstream AI tools (PRD 4.2).
type ToolDriver interface {
	ToolID() string
	ToolName() string
	ConfigType() string // toml | json | yaml | env
	DefaultPaths() []string
	PreferredPath() string
	// DetectConfig reports whether path is a usable configuration file.
	DetectConfig(path string) (ConfigState, error)
	// InspectConfig reads the tool-specific model/provider view from content.
	InspectConfig(content []byte, path string) (ToolConfigView, error)
	// ApplyModel changes only the tool-specific model/provider fields.
	ApplyModel(content []byte, path string, req ModelInjection) ([]byte, error)
	// InjectGateway rewrites config so traffic hits local gateway.
	InjectGateway(configPath, gatewayURL, apiKey string) error
	// Detect whether path looks managed by AIGateway.
	IsManaged(configPath string) bool
	// ValidateConfig verifies the fields that the tool actually consumes.
	ValidateConfig(path string, expected ExpectedConfig) error
	// ValidateConfigContent validates content before it is written to disk.
	ValidateConfigContent(content []byte, path string, expected ExpectedConfig) error
}

type ConfigState struct {
	Path    string `json:"path"`
	Exists  bool   `json:"exists"`
	Managed bool   `json:"managed"`
}

type ToolConfigView struct {
	Model         string        `json:"model"`
	ModelProvider string        `json:"modelProvider"`
	Candidates    []ModelOption `json:"candidates,omitempty"`
}

type ExpectedConfig struct {
	Model          string
	RequireModel   bool
	RequireManaged bool
}

type ModelInjection struct {
	Model    string
	Provider string
	BaseURL  string
	APIKey   string
	Name     string
}

func detectToolConfig(d ToolDriver, path string) (ConfigState, error) {
	path = expandPath(path)
	if path == "" {
		return ConfigState{}, fmt.Errorf("配置路径为空")
	}
	st, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return ConfigState{Path: path}, nil
		}
		return ConfigState{Path: path}, err
	}
	if st.IsDir() {
		return ConfigState{Path: path}, fmt.Errorf("配置路径是目录: %s", path)
	}
	return ConfigState{Path: path, Exists: true, Managed: d.IsManaged(path)}, nil
}

func validateToolConfig(d ToolDriver, path string, expected ExpectedConfig) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("写入后读取配置失败: %w", err)
	}
	return d.ValidateConfigContent(content, path, expected)
}

func validateToolConfigContentWithDriver(d ToolDriver, content []byte, path string, expected ExpectedConfig) error {
	view, err := d.InspectConfig(content, path)
	if err != nil {
		return err
	}
	if expected.RequireModel && strings.TrimSpace(view.Model) != strings.TrimSpace(expected.Model) {
		return fmt.Errorf("配置写入校验失败: 期望模型 %q，实际读取为 %q", expected.Model, view.Model)
	}
	if expected.RequireManaged && !d.IsManaged(path) {
		return fmt.Errorf("配置写入校验失败: 未检测到 AIGateway 接管标记")
	}
	return nil
}

var toolRegistry = []ToolDriver{
	&chatgptDriver{},
	&claudeDriver{},
	&openclawDriver{},
	&harnessDriver{},
	&grokDriver{},
}

func driverByID(id string) ToolDriver {
	id = strings.ToLower(strings.TrimSpace(id))
	// aliases
	if id == "codex" {
		id = "chatgpt"
	}
	for _, d := range toolRegistry {
		if d.ToolID() == id {
			return d
		}
	}
	return nil
}

func toolKindFromDriverID(id string) ToolKind {
	switch strings.ToLower(id) {
	case "chatgpt", "codex":
		return ToolCodex
	case "claude", "claude_code":
		return ToolClaude
	case "openclaw":
		return ToolOpenClaw
	case "harness":
		return ToolHarness
	case "grok", "grok_cli", "grok-build":
		return ToolGrok
	default:
		return ToolKind(id)
	}
}

// --- ChatGPT (legacy codex paths) ---

type chatgptDriver struct{}

func (d *chatgptDriver) ToolID() string         { return "chatgpt" }
func (d *chatgptDriver) ToolName() string       { return "ChatGPT" }
func (d *chatgptDriver) ConfigType() string     { return "toml" }
func (d *chatgptDriver) DefaultPaths() []string { return codexSearchPaths() }
func (d *chatgptDriver) PreferredPath() string  { return preferredCodexConfigPath() }
func (d *chatgptDriver) DetectConfig(path string) (ConfigState, error) {
	return detectToolConfig(d, path)
}
func (d *chatgptDriver) InspectConfig(content []byte, path string) (ToolConfigView, error) {
	var doc map[string]any
	if err := toml.Unmarshal(content, &doc); err != nil {
		return ToolConfigView{}, fmt.Errorf("Codex TOML 解析失败: %w", err)
	}
	s := string(content)
	return ToolConfigView{
		Model:         readTomlTopLevelString(s, "model"),
		ModelProvider: readTomlTopLevelString(s, "model_provider"),
		Candidates:    parseCodexModels(s),
	}, nil
}
func (d *chatgptDriver) ApplyModel(content []byte, path string, req ModelInjection) ([]byte, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = req.Model
	}
	next := applyCodexModelSwitch(string(content), req.Model, req.Provider, name, req.BaseURL, req.APIKey)
	return []byte(removeTomlProviderBlock(next, "codex_proxy")), nil
}
func (d *chatgptDriver) ValidateConfig(path string, expected ExpectedConfig) error {
	return validateToolConfig(d, path, expected)
}
func (d *chatgptDriver) ValidateConfigContent(content []byte, path string, expected ExpectedConfig) error {
	return validateToolConfigContentWithDriver(d, content, path, expected)
}
func (d *chatgptDriver) InjectGateway(configPath, gatewayURL, apiKey string) error {
	raw, _ := os.ReadFile(configPath)
	next, err := injectGatewayCodex(string(raw), gatewayURL)
	if err != nil {
		return err
	}
	return writeFileAtomic(configPath, preserveLineEndings(string(raw), next))
}
func (d *chatgptDriver) IsManaged(configPath string) bool {
	b, err := os.ReadFile(configPath)
	if err != nil {
		return false
	}
	s := string(b)
	return strings.Contains(s, "model_providers.aigateway") || strings.Contains(s, `model_provider = "aigateway"`)
}

// --- Claude Code ---

type claudeDriver struct{}

func (d *claudeDriver) ToolID() string         { return "claude" }
func (d *claudeDriver) ToolName() string       { return "Claude Code" }
func (d *claudeDriver) ConfigType() string     { return "json" }
func (d *claudeDriver) DefaultPaths() []string { return claudeSearchPaths() }
func (d *claudeDriver) PreferredPath() string  { return preferredClaudeConfigPath() }
func (d *claudeDriver) DetectConfig(path string) (ConfigState, error) {
	return detectToolConfig(d, path)
}
func (d *claudeDriver) InspectConfig(content []byte, path string) (ToolConfigView, error) {
	var doc map[string]any
	if err := json.Unmarshal(content, &doc); err != nil {
		return ToolConfigView{}, fmt.Errorf("Claude settings.json 解析失败: %w", err)
	}
	model, provider := readClaudeModel(string(content), path)
	return ToolConfigView{Model: model, ModelProvider: provider}, nil
}
func (d *claudeDriver) ApplyModel(content []byte, path string, req ModelInjection) ([]byte, error) {
	req.BaseURL = normalizeClaudeGatewayURL(req.BaseURL)
	next, err := applyClaudeModelSwitch(string(content), req.Model, req.BaseURL, req.APIKey, req.Provider)
	if err != nil {
		return nil, err
	}
	return []byte(next), nil
}
func (d *claudeDriver) ValidateConfig(path string, expected ExpectedConfig) error {
	return validateToolConfig(d, path, expected)
}
func (d *claudeDriver) ValidateConfigContent(content []byte, path string, expected ExpectedConfig) error {
	return validateToolConfigContentWithDriver(d, content, path, expected)
}
func (d *claudeDriver) InjectGateway(configPath, gatewayURL, apiKey string) error {
	raw, _ := os.ReadFile(configPath)
	next, err := injectGatewayClaude(string(raw), gatewayURL)
	if err != nil {
		return err
	}
	return writeFileAtomic(configPath, next)
}
func (d *claudeDriver) IsManaged(configPath string) bool {
	b, err := os.ReadFile(configPath)
	if err != nil {
		return false
	}
	s := string(b)
	return strings.Contains(s, "127.0.0.1:") || strings.Contains(s, "localhost:") || strings.Contains(s, "aigateway")
}

// --- OpenClaw ---

type openclawDriver struct{}

func (d *openclawDriver) ToolID() string     { return "openclaw" }
func (d *openclawDriver) ToolName() string   { return "OpenClaw" }
func (d *openclawDriver) ConfigType() string { return "json5" }
func (d *openclawDriver) DetectConfig(path string) (ConfigState, error) {
	return detectToolConfig(d, path)
}
func (d *openclawDriver) InspectConfig(content []byte, path string) (ToolConfigView, error) {
	var doc map[string]any
	if err := unmarshalOpenClawJSON5(content, &doc); err != nil {
		return ToolConfigView{}, fmt.Errorf("OpenClaw JSON5 解析失败: %w", err)
	}
	model, provider := readOpenClawModel(string(content))
	return ToolConfigView{Model: model, ModelProvider: provider}, nil
}
func (d *openclawDriver) ApplyModel(content []byte, path string, req ModelInjection) ([]byte, error) {
	next, err := renderOpenClawGateway(content, req.BaseURL, req.APIKey, req.Model, req.Provider)
	if err != nil {
		return nil, err
	}
	return []byte(next), nil
}
func (d *openclawDriver) ValidateConfig(path string, expected ExpectedConfig) error {
	return validateToolConfig(d, path, expected)
}
func (d *openclawDriver) ValidateConfigContent(content []byte, path string, expected ExpectedConfig) error {
	return validateToolConfigContentWithDriver(d, content, path, expected)
}
func (d *openclawDriver) DefaultPaths() []string {
	home := userHome()
	var paths []string
	if home != "" {
		paths = append(paths,
			filepath.Join(home, ".openclaw", "openclaw.json"),
			filepath.Join(home, ".openclaw", "config.json"),
			filepath.Join(home, ".config", "openclaw", "config.json"),
			filepath.Join(home, ".openclaw.json"),
		)
	}
	if goruntime.GOOS == "darwin" && home != "" {
		paths = append(paths, filepath.Join(home, "Library", "Application Support", "OpenClaw", "config.json"))
	}
	return uniquePaths(paths)
}
func (d *openclawDriver) PreferredPath() string {
	if home := userHome(); home != "" {
		return filepath.Join(home, ".openclaw", "openclaw.json")
	}
	return "openclaw.json"
}
func (d *openclawDriver) InjectGateway(configPath, gatewayURL, apiKey string) error {
	return injectOpenClawGateway(configPath, gatewayURL, apiKey, appProxyModel(ToolOpenClaw), gatewayProviderID)
}
func (d *openclawDriver) IsManaged(configPath string) bool {
	return fileContainsAny(configPath, "127.0.0.1:", "localhost:", "aigateway")
}

// unmarshalOpenClawJSON5 accepts the JSON5 forms used by OpenClaw. The
// upstream Go package handles comments, unquoted keys and trailing commas but
// not single-quoted strings, so normalize only those strings before decoding.
func unmarshalOpenClawJSON5(data []byte, out any) error {
	normalized, err := normalizeJSON5SingleQuotes(string(data))
	if err != nil {
		return err
	}
	return json5.Unmarshal([]byte(normalized), out)
}

func normalizeJSON5SingleQuotes(source string) (string, error) {
	var out strings.Builder
	inDouble := false
	escaped := false
	for i := 0; i < len(source); i++ {
		ch := source[i]
		if inDouble {
			out.WriteByte(ch)
			if escaped {
				escaped = false
			} else if ch == '\\' {
				escaped = true
			} else if ch == '"' {
				inDouble = false
			}
			continue
		}
		if ch == '"' {
			inDouble = true
			out.WriteByte(ch)
			continue
		}
		if ch != '\'' {
			out.WriteByte(ch)
			continue
		}

		out.WriteByte('"')
		closed := false
		for i++; i < len(source); i++ {
			ch = source[i]
			if ch == '\'' {
				out.WriteByte('"')
				closed = true
				break
			}
			if ch == '\\' && i+1 < len(source) {
				next := source[i+1]
				switch next {
				case '\'', '"':
					out.WriteByte('\\')
					if next == '\'' {
						out.WriteByte('\'')
					} else {
						out.WriteByte('"')
					}
				default:
					out.WriteByte('\\')
					out.WriteByte(next)
				}
				i++
				continue
			}
			if ch == '"' {
				out.WriteByte('\\')
			}
			out.WriteByte(ch)
		}
		if !closed {
			return "", fmt.Errorf("未闭合的 JSON5 单引号字符串")
		}
	}
	return out.String(), nil
}

// --- Harness ---

type harnessDriver struct{}

func (d *harnessDriver) ToolID() string     { return "harness" }
func (d *harnessDriver) ToolName() string   { return "Harness" }
func (d *harnessDriver) ConfigType() string { return "yaml" }
func (d *harnessDriver) DetectConfig(path string) (ConfigState, error) {
	return detectToolConfig(d, path)
}
func (d *harnessDriver) InspectConfig(content []byte, path string) (ToolConfigView, error) {
	model, provider := readGenericToolModel(string(content), path)
	return ToolConfigView{Model: model, ModelProvider: provider}, nil
}
func (d *harnessDriver) ApplyModel(content []byte, path string, req ModelInjection) ([]byte, error) {
	next, err := applyHarnessYAML(string(content), req.BaseURL, req.APIKey, req.Model, req.Provider)
	if err != nil {
		return nil, err
	}
	return []byte(next), nil
}
func (d *harnessDriver) ValidateConfig(path string, expected ExpectedConfig) error {
	return validateToolConfig(d, path, expected)
}
func (d *harnessDriver) ValidateConfigContent(content []byte, path string, expected ExpectedConfig) error {
	return validateToolConfigContentWithDriver(d, content, path, expected)
}
func (d *harnessDriver) DefaultPaths() []string {
	home := userHome()
	var paths []string
	if home != "" {
		paths = append(paths,
			filepath.Join(home, ".harness", "config.yaml"),
			filepath.Join(home, ".harness", "config.yml"),
			filepath.Join(home, ".config", "harness", "config.yaml"),
			filepath.Join(home, ".harness.yaml"),
		)
	}
	return uniquePaths(paths)
}
func (d *harnessDriver) PreferredPath() string {
	if home := userHome(); home != "" {
		return filepath.Join(home, ".harness", "config.yaml")
	}
	return "harness-config.yaml"
}
func (d *harnessDriver) InjectGateway(configPath, gatewayURL, apiKey string) error {
	return injectGenericModelGateway(configPath, gatewayURL, apiKey, appProxyModel(ToolHarness), gatewayProviderID)
}
func (d *harnessDriver) IsManaged(configPath string) bool {
	return fileContainsAny(configPath, "127.0.0.1:", "localhost:", "aigateway")
}

// --- Grok CLI ---

type grokDriver struct{}

func (d *grokDriver) ToolID() string     { return "grok" }
func (d *grokDriver) ToolName() string   { return "Grok CLI" }
func (d *grokDriver) ConfigType() string { return "toml" }
func (d *grokDriver) DetectConfig(path string) (ConfigState, error) {
	return detectToolConfig(d, path)
}
func (d *grokDriver) InspectConfig(content []byte, path string) (ToolConfigView, error) {
	var doc map[string]any
	if err := toml.Unmarshal(content, &doc); err != nil {
		return ToolConfigView{}, fmt.Errorf("Grok CLI TOML 解析失败: %w", err)
	}
	s := string(content)
	model, provider := readGrokModelConfig(s)
	return ToolConfigView{Model: model, ModelProvider: provider}, nil
}
func (d *grokDriver) ApplyModel(content []byte, path string, req ModelInjection) ([]byte, error) {
	next := applyGrokTOML(string(content), req.BaseURL, req.APIKey, req.Model, req.Provider, req.Name)
	return []byte(next), nil
}
func (d *grokDriver) ValidateConfig(path string, expected ExpectedConfig) error {
	return validateToolConfig(d, path, expected)
}
func (d *grokDriver) ValidateConfigContent(content []byte, path string, expected ExpectedConfig) error {
	return validateToolConfigContentWithDriver(d, content, path, expected)
}
func (d *grokDriver) DefaultPaths() []string {
	home := userHome()
	var paths []string
	if v := strings.TrimSpace(os.Getenv("GROK_CONFIG_DIR")); v != "" {
		base := expandPath(v)
		paths = append(paths, filepath.Join(base, "config.toml"))
	}
	if home != "" {
		paths = append(paths,
			filepath.Join(home, ".grok", "config.toml"),
			filepath.Join(home, ".config", "grok", "config.toml"),
			filepath.Join(home, ".xai", "grok", "config.toml"),
		)
	}
	if goruntime.GOOS == "darwin" && home != "" {
		paths = append(paths, filepath.Join(home, "Library", "Application Support", "Grok", "config.toml"))
	}
	if cwd, err := os.Getwd(); err == nil && cwd != "" {
		paths = append(paths, filepath.Join(cwd, ".grok", "config.toml"))
	}
	return uniquePaths(paths)
}
func (d *grokDriver) PreferredPath() string {
	if v := strings.TrimSpace(os.Getenv("GROK_CONFIG_DIR")); v != "" {
		return filepath.Join(expandPath(v), "config.toml")
	}
	if home := userHome(); home != "" {
		return filepath.Join(home, ".grok", "config.toml")
	}
	return "grok-config.toml"
}
func (d *grokDriver) InjectGateway(configPath, gatewayURL, apiKey string) error {
	raw, err := os.ReadFile(configPath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	next := applyGrokTOML(string(raw), gatewayURL, apiKey, appProxyModel(ToolGrok), gatewayProviderID, "AIGateway")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		return err
	}
	return writeFileAtomic(configPath, next)
}
func (d *grokDriver) IsManaged(configPath string) bool {
	return fileContainsAny(configPath, "127.0.0.1:", "localhost:", "aigateway")
}

func injectJSONBaseURL(configPath, gatewayURL, apiKey string) error {
	raw, err := os.ReadFile(configPath)
	var root map[string]any
	if err != nil || len(raw) == 0 {
		root = map[string]any{}
	} else if err := json.Unmarshal(raw, &root); err != nil {
		return fmt.Errorf("解析 OpenClaw 配置失败（当前仅安全写入标准 JSON；请先移除 JSON5 注释/尾逗号后重试）: %w", err)
	}
	root["baseUrl"] = gatewayURL
	root["base_url"] = gatewayURL
	root["apiBaseUrl"] = gatewayURL
	env, _ := root["env"].(map[string]any)
	if env == nil {
		env = map[string]any{}
	}
	env["OPENAI_BASE_URL"] = gatewayURL
	if apiKey != "" {
		env["OPENAI_API_KEY"] = apiKey
	}
	root["env"] = env
	// nested openai block
	oa, _ := root["openai"].(map[string]any)
	if oa == nil {
		oa = map[string]any{}
	}
	oa["baseUrl"] = gatewayURL
	oa["base_url"] = gatewayURL
	root["openai"] = oa
	b, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		return err
	}
	return writeFileAtomic(configPath, string(b)+"\n")
}

func injectOpenClawGateway(configPath, gatewayURL, apiKey, model, providerID string) error {
	raw, err := os.ReadFile(configPath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	next, err := renderOpenClawGateway(raw, gatewayURL, apiKey, model, providerID)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		return err
	}
	return writeFileAtomic(configPath, next)
}

func renderOpenClawGateway(raw []byte, gatewayURL, apiKey, model, providerID string) (string, error) {
	var root map[string]any
	if len(raw) == 0 {
		root = map[string]any{}
	} else if err := unmarshalOpenClawJSON5(raw, &root); err != nil {
		return "", fmt.Errorf("解析 OpenClaw JSON5 配置失败（为避免覆盖现有配置，已停止写入）: %w", err)
	}
	if apiKey == "" {
		apiKey = "aigateway"
	}
	if providerID == "" {
		providerID = gatewayProviderID
	}
	if model == "" {
		model = "aiSwitchModel"
	}

	models, _ := root["models"].(map[string]any)
	if models == nil {
		models = map[string]any{}
	}
	models["mode"] = "merge"
	providers, _ := models["providers"].(map[string]any)
	if providers == nil {
		providers = map[string]any{}
	}
	provider, _ := providers[providerID].(map[string]any)
	if provider == nil {
		provider = map[string]any{}
	}
	// OpenClaw's current schema uses camelCase keys and a protocol enum.
	// Remove legacy AIGateway keys so upgrades do not leave the config invalid.
	delete(provider, "base_url")
	delete(provider, "api_key")
	provider["baseUrl"] = gatewayURL
	provider["apiKey"] = apiKey
	provider["api"] = "openai-completions"
	providerModels, _ := provider["models"].([]any)
	modelIDs := []string{model}
	// OpenClaw sessions can retain a model alias from another app. Declare
	// configured aliases as well, so OpenClaw does not reject the request
	// before it reaches AIGateway's alias router.
	for _, kind := range []ToolKind{ToolCodex, ToolClaude, ToolOpenClaw, ToolHarness, ToolGrok} {
		alias := appProxyModel(kind)
		if alias != model && loadProxyAlias(alias) != "" {
			modelIDs = append(modelIDs, alias)
		}
	}
	for _, modelID := range modelIDs {
		foundModel := false
		for _, item := range providerModels {
			entry, _ := item.(map[string]any)
			if entry == nil || fmt.Sprint(entry["id"]) != modelID {
				continue
			}
			entry["name"] = modelID
			foundModel = true
		}
		if !foundModel {
			providerModels = append(providerModels, map[string]any{"id": modelID, "name": modelID})
		}
	}
	provider["models"] = providerModels
	providers[providerID] = provider
	models["providers"] = providers
	root["models"] = models

	agents, _ := root["agents"].(map[string]any)
	if agents == nil {
		agents = map[string]any{}
	}
	defaults, _ := agents["defaults"].(map[string]any)
	if defaults == nil {
		defaults = map[string]any{}
	}
	defaults["model"] = map[string]any{
		"primary":   providerID + "/" + model,
		"fallbacks": []string{},
	}
	agents["defaults"] = defaults
	root["agents"] = agents

	env, _ := root["env"].(map[string]any)
	if env == nil {
		env = map[string]any{}
	}
	vars, _ := env["vars"].(map[string]any)
	if vars == nil {
		vars = map[string]any{}
	}
	vars["OPENAI_BASE_URL"] = gatewayURL
	vars["OPENAI_API_KEY"] = apiKey
	env["vars"] = vars
	root["env"] = env
	// These were written by older AIGateway/OpenClaw integrations and are not
	// valid root-level OpenClaw settings in current releases.
	delete(root, "apiBaseUrl")
	delete(root, "baseUrl")
	delete(root, "base_url")
	delete(root, "openai")

	b, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return "", err
	}
	return string(b) + "\n", nil
}

func injectYAMLBaseURL(configPath, gatewayURL string) error {
	raw, _ := os.ReadFile(configPath)
	content := string(raw)
	if content == "" {
		content = "# AIGateway managed\n"
	}
	// simple line upserts for common keys
	content = upsertYAMLKey(content, "base_url", gatewayURL)
	content = upsertYAMLKey(content, "baseUrl", gatewayURL)
	content = upsertYAMLKey(content, "openai.base_url", gatewayURL)
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		return err
	}
	return writeFileAtomic(configPath, content)
}

func upsertYAMLKey(content, key, value string) string {
	// only top-level simple keys
	simple := key
	if i := strings.LastIndex(key, "."); i >= 0 {
		simple = key[i+1:]
	}
	lines := strings.Split(content, "\n")
	found := false
	prefix := simple + ":"
	for i, ln := range lines {
		trim := strings.TrimSpace(ln)
		if strings.HasPrefix(trim, prefix) {
			lines[i] = simple + ": " + value
			found = true
			break
		}
	}
	if !found {
		lines = append(lines, simple+": "+value)
	}
	return strings.Join(lines, "\n")
}

func fileContainsAny(path string, needles ...string) bool {
	b, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	s := string(b)
	for _, n := range needles {
		if strings.Contains(s, n) {
			return true
		}
	}
	return false
}
