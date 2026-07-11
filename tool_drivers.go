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

func (d *chatgptDriver) ToolID() string     { return "chatgpt" }
func (d *chatgptDriver) ToolName() string   { return "ChatGPT" }
func (d *chatgptDriver) ConfigType() string { return "toml" }
func (d *chatgptDriver) DefaultPaths() []string {
	return codexSearchPaths()
}
func (d *chatgptDriver) PreferredPath() string { return preferredCodexConfigPath() }
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

func (d *claudeDriver) ToolID() string     { return "claude" }
func (d *claudeDriver) ToolName() string   { return "Claude Code" }
func (d *claudeDriver) ConfigType() string { return "json" }
func (d *claudeDriver) DefaultPaths() []string {
	return claudeSearchPaths()
}
func (d *claudeDriver) PreferredPath() string { return preferredClaudeConfigPath() }
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
	return strings.Contains(s, "127.0.0.1:18080") || strings.Contains(s, "ANTHROPIC_BASE_URL")
}

// --- OpenClaw ---
// Official config: ~/.openclaw/openclaw.json
// Custom OpenAI-compatible gateway via models.providers.<id>:
//
//	{
//	  "models": {
//	    "mode": "merge",
//	    "providers": {
//	      "aigateway": {
//	        "baseUrl": "http://127.0.0.1:18080/v1",
//	        "apiKey": "aigateway",
//	        "api": "openai-completions",
//	        "models": [{ "id": "...", "name": "..." }]
//	      }
//	    }
//	  },
//	  "agents": {
//	    "defaults": {
//	      "model": { "primary": "aigateway/<model>" },
//	      "models": { "aigateway/<model>": { "alias": "..." } }
//	    }
//	  }
//	}
//
// Model switch: set agents.defaults.model.primary = "aigateway/<model>"
// (or openclaw models set aigateway/<model>). Root-level baseUrl / OPENAI_BASE_URL is WRONG.

type openclawDriver struct{}

func (d *openclawDriver) ToolID() string     { return "openclaw" }
func (d *openclawDriver) ToolName() string   { return "OpenClaw" }
func (d *openclawDriver) ConfigType() string { return "json" }
func (d *openclawDriver) DefaultPaths() []string {
	home := userHome()
	var paths []string
	if home != "" {
		// Official path first (docs.openclaw.ai)
		paths = append(paths,
			filepath.Join(home, ".openclaw", "openclaw.json"),
			filepath.Join(home, ".openclaw", "config.json"), // legacy
			filepath.Join(home, ".config", "openclaw", "openclaw.json"),
			filepath.Join(home, ".config", "openclaw", "config.json"),
			filepath.Join(home, ".openclaw.json"),
		)
	}
	if goruntime.GOOS == "darwin" && home != "" {
		paths = append(paths, filepath.Join(home, "Library", "Application Support", "OpenClaw", "openclaw.json"))
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
	return injectOpenClawGateway(configPath, gatewayURL, apiKey, nil)
}
func (d *openclawDriver) IsManaged(configPath string) bool {
	return fileContainsAny(configPath, `"aigateway"`, "127.0.0.1:18080")
}

// --- Harness (generic agent YAML/JSON harness) ---
// Common pattern used by lightweight coding harnesses:
//
//	model: <id>
//	provider:
//	  type: openai
//	  base_url: http://127.0.0.1:18080/v1
//	  api_key: aigateway
//	llm:
//	  model: <id>
//	  base_url: ...
//	  api_key: ...

type harnessDriver struct{}

func (d *harnessDriver) ToolID() string     { return "harness" }
func (d *harnessDriver) ToolName() string   { return "Harness" }
func (d *harnessDriver) ConfigType() string { return "yaml" }
func (d *harnessDriver) DefaultPaths() []string {
	home := userHome()
	var paths []string
	if home != "" {
		paths = append(paths,
			filepath.Join(home, ".harness", "config.yaml"),
			filepath.Join(home, ".harness", "config.yml"),
			filepath.Join(home, ".harness", "config.json"),
			filepath.Join(home, ".config", "harness", "config.yaml"),
			filepath.Join(home, ".harness.yaml"),
			filepath.Join(home, ".harness.json"),
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
	if strings.HasSuffix(strings.ToLower(configPath), ".json") {
		return injectHarnessJSON(configPath, gatewayURL, apiKey, "")
	}
	return injectHarnessYAML(configPath, gatewayURL, apiKey, "")
}
func (d *harnessDriver) IsManaged(configPath string) bool {
	return fileContainsAny(configPath, "127.0.0.1:18080", "aigateway")
}

// --- OpenClaw inject / model switch ---

const openclawProviderID = "aigateway"

func injectOpenClawGateway(configPath, gatewayURL, apiKey string, modelIDs []string) error {
	raw, err := os.ReadFile(configPath)
	var root map[string]any
	if err != nil || len(raw) == 0 {
		root = map[string]any{}
	} else if json.Unmarshal(raw, &root) != nil {
		// JSON5 may have comments — try strip simple // and /* */ then reparse
		cleaned := stripJSONComments(string(raw))
		if json.Unmarshal([]byte(cleaned), &root) != nil {
			root = map[string]any{}
		}
	}
	if root == nil {
		root = map[string]any{}
	}
	if apiKey == "" {
		apiKey = "aigateway"
	}
	// Pin virtual model for hot-switch; real models still listed for discovery.
	if len(modelIDs) == 0 {
		modelIDs = collectEnabledModelIDs()
	}
	// Pin OpenClaw-scoped virtual model (independent of other apps)
	virt := virtualModelForTool(toolKeyOpenClaw)
	hasVirtual := false
	for _, mid := range modelIDs {
		if mid == virt {
			hasVirtual = true
			break
		}
	}
	if !hasVirtual {
		modelIDs = append([]string{virt}, modelIDs...)
	}

	// models.mode + models.providers.aigateway
	modelsRoot, _ := root["models"].(map[string]any)
	if modelsRoot == nil {
		modelsRoot = map[string]any{}
	}
	modelsRoot["mode"] = "merge"
	providers, _ := modelsRoot["providers"].(map[string]any)
	if providers == nil {
		providers = map[string]any{}
	}
	modelObjs := make([]any, 0, len(modelIDs))
	for _, mid := range modelIDs {
		mid = strings.TrimSpace(mid)
		if mid == "" {
			continue
		}
		name := mid
		if mid == virt {
			name = "AIGateway OpenClaw (" + virt + ")"
		}
		modelObjs = append(modelObjs, map[string]any{
			"id":   mid,
			"name": name,
		})
	}
	providers[openclawProviderID] = map[string]any{
		"baseUrl": gatewayURL,
		"apiKey":  apiKey,
		"api":     "openai-completions",
		"models":  modelObjs,
	}
	modelsRoot["providers"] = providers
	root["models"] = modelsRoot

	// agents.defaults.model.primary + allowlist — pin OpenClaw virtual model
	agents, _ := root["agents"].(map[string]any)
	if agents == nil {
		agents = map[string]any{}
	}
	defaults, _ := agents["defaults"].(map[string]any)
	if defaults == nil {
		defaults = map[string]any{}
	}
	primary := openclawProviderID + "/" + virt
	modelBlock, _ := defaults["model"].(map[string]any)
	if modelBlock == nil {
		modelBlock = map[string]any{}
	}
	modelBlock["primary"] = primary
	defaults["model"] = modelBlock

	allow, _ := defaults["models"].(map[string]any)
	if allow == nil {
		allow = map[string]any{}
	}
	for _, mid := range modelIDs {
		ref := openclawProviderID + "/" + mid
		alias := mid
		if mid == virt {
			alias = "AIGateway-OpenClaw"
		}
		if _, ok := allow[ref]; !ok {
			allow[ref] = map[string]any{"alias": alias}
		}
	}
	defaults["models"] = allow
	agents["defaults"] = defaults
	root["agents"] = agents

	// Remove incorrect legacy root keys from older injectJSONBaseURL
	delete(root, "baseUrl")
	delete(root, "base_url")
	delete(root, "apiBaseUrl")
	if env, ok := root["env"].(map[string]any); ok {
		delete(env, "OPENAI_BASE_URL")
		if len(env) == 0 {
			delete(root, "env")
		} else {
			root["env"] = env
		}
	}
	delete(root, "openai")

	b, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		return err
	}
	return writeFileAtomic(configPath, string(b)+"\n")
}

func applyOpenClawModelSwitch(content, model, baseURL, apiKey string) (string, error) {
	var root map[string]any
	if strings.TrimSpace(content) == "" {
		root = map[string]any{}
	} else if err := json.Unmarshal([]byte(content), &root); err != nil {
		cleaned := stripJSONComments(content)
		if err2 := json.Unmarshal([]byte(cleaned), &root); err2 != nil {
			root = map[string]any{}
		}
	}
	if root == nil {
		root = map[string]any{}
	}
	if apiKey == "" {
		apiKey = "aigateway"
	}
	model = strings.TrimSpace(model)
	if model == "" {
		return "", errf("模型不能为空")
	}
	// strip provider prefix if user passed aigateway/xxx
	if i := strings.Index(model, "/"); i > 0 {
		// keep full ref if already provider/model; use model part for catalog id
		// but set primary as given if it has aigateway/ or any provider/
	}
	modelID := model
	primary := model
	if !strings.Contains(model, "/") {
		primary = openclawProviderID + "/" + model
		modelID = model
	} else {
		parts := strings.SplitN(model, "/", 2)
		modelID = parts[1]
	}

	// ensure provider exists when baseURL given
	if baseURL != "" {
		modelsRoot, _ := root["models"].(map[string]any)
		if modelsRoot == nil {
			modelsRoot = map[string]any{}
		}
		modelsRoot["mode"] = "merge"
		providers, _ := modelsRoot["providers"].(map[string]any)
		if providers == nil {
			providers = map[string]any{}
		}
		prov, _ := providers[openclawProviderID].(map[string]any)
		if prov == nil {
			prov = map[string]any{}
		}
		prov["baseUrl"] = baseURL
		prov["apiKey"] = apiKey
		prov["api"] = "openai-completions"
		// ensure model in catalog
		existing, _ := prov["models"].([]any)
		found := false
		for _, m := range existing {
			if mm, ok := m.(map[string]any); ok {
				if id, _ := mm["id"].(string); id == modelID {
					found = true
					break
				}
			}
		}
		if !found {
			existing = append(existing, map[string]any{"id": modelID, "name": modelID})
		}
		prov["models"] = existing
		providers[openclawProviderID] = prov
		modelsRoot["providers"] = providers
		root["models"] = modelsRoot
	}

	agents, _ := root["agents"].(map[string]any)
	if agents == nil {
		agents = map[string]any{}
	}
	defaults, _ := agents["defaults"].(map[string]any)
	if defaults == nil {
		defaults = map[string]any{}
	}
	modelBlock, _ := defaults["model"].(map[string]any)
	if modelBlock == nil {
		modelBlock = map[string]any{}
	}
	modelBlock["primary"] = primary
	defaults["model"] = modelBlock
	allow, _ := defaults["models"].(map[string]any)
	if allow == nil {
		allow = map[string]any{}
	}
	if _, ok := allow[primary]; !ok {
		allow[primary] = map[string]any{"alias": modelID}
	}
	defaults["models"] = allow
	agents["defaults"] = defaults
	root["agents"] = agents

	b, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return "", err
	}
	return string(b) + "\n", nil
}

func readOpenClawModel(content string) (model, provider string) {
	var root map[string]any
	if json.Unmarshal([]byte(content), &root) != nil {
		_ = json.Unmarshal([]byte(stripJSONComments(content)), &root)
	}
	if root == nil {
		return "", ""
	}
	agents, _ := root["agents"].(map[string]any)
	defaults, _ := agents["defaults"].(map[string]any)
	modelBlock, _ := defaults["model"].(map[string]any)
	if primary, ok := modelBlock["primary"].(string); ok {
		model = primary
		if i := strings.Index(primary, "/"); i > 0 {
			provider = primary[:i]
		}
	}
	return model, provider
}

// --- Harness inject / model switch ---

func injectHarnessYAML(configPath, gatewayURL, apiKey, model string) error {
	raw, _ := os.ReadFile(configPath)
	content := string(raw)
	if content == "" {
		content = "# AIGateway managed\n"
	}
	if apiKey == "" {
		apiKey = "aigateway"
	}
	// Always pin Harness-scoped virtual model for independent hot-switch
	model = virtualModelForTool(toolKeyHarness)
	// top-level keys commonly read by agent harnesses
	content = upsertYAMLKey(content, "model", model)
	content = upsertYAMLKey(content, "base_url", gatewayURL)
	content = upsertYAMLKey(content, "baseUrl", gatewayURL)
	content = upsertYAMLKey(content, "api_key", apiKey)
	content = upsertYAMLKey(content, "apiKey", apiKey)
	// nested provider / llm blocks (best-effort flat keys for simple parsers)
	content = upsertYAMLKey(content, "provider_type", "openai")
	content = upsertYAMLNested(content, "provider", map[string]string{
		"type":     "openai",
		"base_url": gatewayURL,
		"api_key":  apiKey,
		"name":     "aigateway",
	})
	content = upsertYAMLNested(content, "llm", map[string]string{
		"model":    model,
		"base_url": gatewayURL,
		"api_key":  apiKey,
		"provider": "openai",
	})
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		return err
	}
	return writeFileAtomic(configPath, content)
}

func injectHarnessJSON(configPath, gatewayURL, apiKey, model string) error {
	raw, err := os.ReadFile(configPath)
	var root map[string]any
	if err != nil || len(raw) == 0 {
		root = map[string]any{}
	} else if json.Unmarshal(raw, &root) != nil {
		root = map[string]any{}
	}
	if apiKey == "" {
		apiKey = "aigateway"
	}
	// Pin Harness-scoped virtual model — independent binding
	model = virtualModelForTool(toolKeyHarness)
	root["model"] = model
	root["base_url"] = gatewayURL
	root["baseUrl"] = gatewayURL
	root["api_key"] = apiKey
	root["apiKey"] = apiKey
	root["provider"] = map[string]any{
		"type": "openai", "base_url": gatewayURL, "api_key": apiKey, "name": "aigateway",
	}
	root["llm"] = map[string]any{
		"model": model, "base_url": gatewayURL, "api_key": apiKey, "provider": "openai",
	}
	b, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		return err
	}
	return writeFileAtomic(configPath, string(b)+"\n")
}

func applyHarnessModelSwitch(content, model, baseURL, apiKey string, isJSON bool) (string, error) {
	if model == "" {
		return "", errf("模型不能为空")
	}
	if apiKey == "" {
		apiKey = "aigateway"
	}
	if isJSON {
		var root map[string]any
		if strings.TrimSpace(content) == "" {
			root = map[string]any{}
		} else if json.Unmarshal([]byte(content), &root) != nil {
			root = map[string]any{}
		}
		root["model"] = model
		if baseURL != "" {
			root["base_url"] = baseURL
			root["baseUrl"] = baseURL
			if apiKey != "" {
				root["api_key"] = apiKey
				root["apiKey"] = apiKey
			}
			root["provider"] = map[string]any{
				"type": "openai", "base_url": baseURL, "api_key": apiKey, "name": "aigateway",
			}
			root["llm"] = map[string]any{
				"model": model, "base_url": baseURL, "api_key": apiKey, "provider": "openai",
			}
		} else if llm, ok := root["llm"].(map[string]any); ok {
			llm["model"] = model
			root["llm"] = llm
		}
		b, err := json.MarshalIndent(root, "", "  ")
		if err != nil {
			return "", err
		}
		return string(b) + "\n", nil
	}
	// YAML
	out := content
	if out == "" {
		out = "# AIGateway managed\n"
	}
	out = upsertYAMLKey(out, "model", model)
	if baseURL != "" {
		out = upsertYAMLKey(out, "base_url", baseURL)
		out = upsertYAMLKey(out, "baseUrl", baseURL)
		out = upsertYAMLKey(out, "api_key", apiKey)
		out = upsertYAMLNested(out, "provider", map[string]string{
			"type": "openai", "base_url": baseURL, "api_key": apiKey, "name": "aigateway",
		})
		out = upsertYAMLNested(out, "llm", map[string]string{
			"model": model, "base_url": baseURL, "api_key": apiKey, "provider": "openai",
		})
	} else {
		// only update model inside llm block if present
		out = upsertYAMLNested(out, "llm", map[string]string{"model": model})
	}
	return out, nil
}

func readHarnessModel(content string) (model, provider string) {
	// try JSON first
	var root map[string]any
	if json.Unmarshal([]byte(content), &root) == nil {
		if v, ok := root["model"].(string); ok {
			model = v
		}
		if llm, ok := root["llm"].(map[string]any); ok {
			if v, ok := llm["model"].(string); ok && model == "" {
				model = v
			}
			if v, ok := llm["provider"].(string); ok {
				provider = v
			}
		}
		return model, provider
	}
	// YAML: first model: line
	for _, ln := range strings.Split(content, "\n") {
		trim := strings.TrimSpace(ln)
		if strings.HasPrefix(trim, "model:") {
			model = strings.TrimSpace(strings.TrimPrefix(trim, "model:"))
			model = strings.Trim(model, `"'`)
			break
		}
	}
	return model, provider
}

// --- helpers ---

func collectEnabledModelIDs() []string {
	list, err := loadProvidersFromDisk()
	if err != nil {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	for _, p := range list {
		for _, m := range p.Models {
			if !m.Enabled || m.ID == "" || seen[m.ID] {
				continue
			}
			seen[m.ID] = true
			out = append(out, m.ID)
			if len(out) >= 32 {
				return out
			}
		}
	}
	return out
}

func stripJSONComments(s string) string {
	// best-effort: remove // line comments and /* block */
	var b strings.Builder
	inStr := false
	esc := false
	i := 0
	for i < len(s) {
		c := s[i]
		if inStr {
			b.WriteByte(c)
			if esc {
				esc = false
			} else if c == '\\' {
				esc = true
			} else if c == '"' {
				inStr = false
			}
			i++
			continue
		}
		if c == '"' {
			inStr = true
			b.WriteByte(c)
			i++
			continue
		}
		if c == '/' && i+1 < len(s) && s[i+1] == '/' {
			for i < len(s) && s[i] != '\n' {
				i++
			}
			continue
		}
		if c == '/' && i+1 < len(s) && s[i+1] == '*' {
			i += 2
			for i+1 < len(s) && !(s[i] == '*' && s[i+1] == '/') {
				i++
			}
			if i+1 < len(s) {
				i += 2
			}
			continue
		}
		b.WriteByte(c)
		i++
	}
	return b.String()
}

func upsertYAMLKey(content, key, value string) string {
	simple := key
	if i := strings.LastIndex(key, "."); i >= 0 {
		simple = key[i+1:]
	}
	lines := strings.Split(content, "\n")
	found := false
	prefix := simple + ":"
	for i, ln := range lines {
		trim := strings.TrimSpace(ln)
		// only top-level (no leading indent)
		if strings.HasPrefix(ln, " ") || strings.HasPrefix(ln, "\t") {
			continue
		}
		if strings.HasPrefix(trim, prefix) {
			lines[i] = simple + ": " + value
			found = true
			break
		}
	}
	if !found {
		if len(lines) > 0 && lines[len(lines)-1] == "" {
			lines = append(lines[:len(lines)-1], simple+": "+value, "")
		} else {
			lines = append(lines, simple+": "+value)
		}
	}
	return strings.Join(lines, "\n")
}

// upsertYAMLNested writes a simple nested map block at top level.
func upsertYAMLNested(content, section string, kv map[string]string) string {
	lines := strings.Split(content, "\n")
	start := -1
	end := -1
	for i, ln := range lines {
		trim := strings.TrimSpace(ln)
		if start < 0 {
			if !strings.HasPrefix(ln, " ") && !strings.HasPrefix(ln, "\t") &&
				(trim == section+":" || strings.HasPrefix(trim, section+":")) {
				start = i
				continue
			}
		} else {
			// next top-level key ends the block
			if trim != "" && !strings.HasPrefix(ln, " ") && !strings.HasPrefix(ln, "\t") && !strings.HasPrefix(trim, "#") {
				end = i
				break
			}
		}
	}
	block := []string{section + ":"}
	// stable-ish order
	order := []string{"type", "name", "provider", "model", "base_url", "api_key"}
	seen := map[string]bool{}
	for _, k := range order {
		if v, ok := kv[k]; ok {
			block = append(block, "  "+k+": "+v)
			seen[k] = true
		}
	}
	for k, v := range kv {
		if !seen[k] {
			block = append(block, "  "+k+": "+v)
		}
	}
	if start >= 0 {
		if end < 0 {
			end = len(lines)
		}
		out := append([]string{}, lines[:start]...)
		out = append(out, block...)
		out = append(out, lines[end:]...)
		return strings.Join(out, "\n")
	}
	// append
	if len(lines) > 0 && lines[len(lines)-1] != "" {
		lines = append(lines, "")
	}
	lines = append(lines, block...)
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

func errf(msg string) error {
	return &simpleError{msg}
}

type simpleError struct{ s string }

func (e *simpleError) Error() string { return e.s }
