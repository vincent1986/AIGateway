package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	goruntime "runtime"
	"strings"
)

// ToolDriver abstracts downstream AI tools (PRD 4.2).
type ToolDriver interface {
	ToolID() string
	ToolName() string
	ConfigType() string // toml | json | yaml | env
	DefaultPaths() []string
	PreferredPath() string
	// InjectGateway rewrites config so traffic hits local gateway.
	InjectGateway(configPath, gatewayURL, apiKey string) error
	// Detect whether path looks managed by AIGateway.
	IsManaged(configPath string) bool
}

var toolRegistry = []ToolDriver{
	&chatgptDriver{},
	&claudeDriver{},
	&openclawDriver{},
	&harnessDriver{},
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
	default:
		return ToolKind(id)
	}
}

// --- ChatGPT (legacy codex paths) ---

type chatgptDriver struct{}

func (d *chatgptDriver) ToolID() string   { return "chatgpt" }
func (d *chatgptDriver) ToolName() string { return "ChatGPT" }
func (d *chatgptDriver) ConfigType() string { return "toml" }
func (d *chatgptDriver) DefaultPaths() []string { return codexSearchPaths() }
func (d *chatgptDriver) PreferredPath() string  { return preferredCodexConfigPath() }
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

func (d *claudeDriver) ToolID() string   { return "claude" }
func (d *claudeDriver) ToolName() string { return "Claude Code" }
func (d *claudeDriver) ConfigType() string { return "json" }
func (d *claudeDriver) DefaultPaths() []string { return claudeSearchPaths() }
func (d *claudeDriver) PreferredPath() string  { return preferredClaudeConfigPath() }
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
	return strings.Contains(s, "127.0.0.1:18080") || strings.Contains(s, "aigateway")
}

// --- OpenClaw ---

type openclawDriver struct{}

func (d *openclawDriver) ToolID() string   { return "openclaw" }
func (d *openclawDriver) ToolName() string { return "OpenClaw" }
func (d *openclawDriver) ConfigType() string { return "json" }
func (d *openclawDriver) DefaultPaths() []string {
	home := userHome()
	var paths []string
	if home != "" {
		paths = append(paths,
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
		return filepath.Join(home, ".openclaw", "config.json")
	}
	return "openclaw-config.json"
}
func (d *openclawDriver) InjectGateway(configPath, gatewayURL, apiKey string) error {
	return injectJSONBaseURL(configPath, gatewayURL, apiKey)
}
func (d *openclawDriver) IsManaged(configPath string) bool {
	return fileContainsAny(configPath, "127.0.0.1:18080", "aigateway")
}

// --- Harness ---

type harnessDriver struct{}

func (d *harnessDriver) ToolID() string   { return "harness" }
func (d *harnessDriver) ToolName() string { return "Harness" }
func (d *harnessDriver) ConfigType() string { return "yaml" }
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
	// Prefer JSON if file is json; else YAML-ish key rewrite
	if strings.HasSuffix(strings.ToLower(configPath), ".json") {
		return injectJSONBaseURL(configPath, gatewayURL, apiKey)
	}
	return injectYAMLBaseURL(configPath, gatewayURL)
}
func (d *harnessDriver) IsManaged(configPath string) bool {
	return fileContainsAny(configPath, "127.0.0.1:18080", "aigateway")
}

func injectJSONBaseURL(configPath, gatewayURL, apiKey string) error {
	raw, err := os.ReadFile(configPath)
	var root map[string]any
	if err != nil || len(raw) == 0 {
		root = map[string]any{}
	} else if json.Unmarshal(raw, &root) != nil {
		root = map[string]any{}
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
