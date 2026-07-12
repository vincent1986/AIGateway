package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/tailscale/hujson"
	"gopkg.in/yaml.v3"
)

type pendingConfigWrite struct {
	Path    string
	Content string
	Reason  string
}

func buildManagedToolWrites(kind ToolKind, path, model, providerID, name, baseURL, apiKey string) ([]pendingConfigWrite, error) {
	if model == "" {
		return nil, fmt.Errorf("模型不能为空")
	}
	if path == "" {
		return nil, fmt.Errorf("未指定路径")
	}
	content, err := readOptionalFile(path)
	if err != nil {
		return nil, err
	}
	switch kind {
	case ToolClaudeDesktop:
		if baseURL == "" {
			return nil, fmt.Errorf("Claude Desktop 第三方接管需要 Anthropic gateway 地址")
		}
		if !isClaudeDesktopRoleModel(model) {
			return nil, fmt.Errorf("Claude Desktop 直连模型必须使用 claude-sonnet/opus/haiku 角色 ID")
		}
		next, err := updateJSONObject(content, func(raw map[string]any) {
			raw["inferenceProvider"] = "gateway"
			raw["inferenceGatewayBaseUrl"] = strings.TrimRight(baseURL, "/")
			raw["inferenceGatewayAuthScheme"] = "bearer"
			if apiKey != "" {
				raw["inferenceGatewayApiKey"] = apiKey
			}
			raw["inferenceModels"] = []string{model}
		})
		return oneWrite(path, next, "apply claude desktop provider"), err
	case ToolGemini:
		if strings.EqualFold(filepath.Ext(path), ".env") {
			next := updateEnv(content, map[string]string{"GEMINI_MODEL": model, "GOOGLE_GEMINI_BASE_URL": baseURL, "GEMINI_API_KEY": apiKey})
			return oneWrite(path, next, "apply gemini environment"), nil
		}
		next, err := updateJSONObject(content, func(raw map[string]any) { nestedMap(raw, "model")["name"] = model })
		if err != nil {
			return nil, err
		}
		envPath := filepath.Join(filepath.Dir(path), ".env")
		env, err := readOptionalFile(envPath)
		if err != nil {
			return nil, err
		}
		env = updateEnv(env, map[string]string{"GOOGLE_GEMINI_BASE_URL": baseURL, "GEMINI_API_KEY": apiKey})
		return []pendingConfigWrite{{path, next, "apply gemini model"}, {envPath, env, "apply gemini endpoint"}}, nil
	case ToolOpenCode:
		next, err := updateJSONObject(content, func(raw map[string]any) {
			raw["$schema"] = "https://opencode.ai/config.json"
			raw["model"] = providerID + "/" + model
			provider := nestedMap(nestedMap(raw, "provider"), providerID)
			provider["npm"] = "@ai-sdk/openai-compatible"
			provider["name"] = name
			options := nestedMap(provider, "options")
			if baseURL != "" {
				options["baseURL"] = baseURL
			}
			if apiKey != "" {
				options["apiKey"] = apiKey
			}
			nestedMap(provider, "models")[model] = map[string]any{"name": name}
		})
		return oneWrite(path, next, "apply opencode provider"), err
	case ToolOpenClaw:
		next, err := updateJSON5Object(content, func(raw map[string]any) {
			raw["agents"] = ensureMap(raw["agents"])
			defaults := nestedMap(raw["agents"].(map[string]any), "defaults")
			nestedMap(defaults, "model")["primary"] = providerID + "/" + model
			models := nestedMap(raw, "models")
			models["mode"] = "merge"
			provider := nestedMap(nestedMap(models, "providers"), providerID)
			if baseURL != "" {
				provider["baseUrl"] = baseURL
			}
			if apiKey != "" {
				provider["apiKey"] = apiKey
			}
			provider["api"] = "openai-completions"
			provider["models"] = []any{map[string]any{"id": model, "name": name}}
		})
		return oneWrite(path, next, "apply openclaw provider"), err
	case ToolHermes:
		if ext := strings.ToLower(filepath.Ext(path)); ext == ".json" {
			next, err := updateJSONObject(content, func(raw map[string]any) {
				cfg := nestedMap(raw, "model")
				cfg["default"], cfg["provider"], cfg["base_url"] = model, "custom", baseURL
				if apiKey != "" {
					cfg["api_key"] = apiKey
				}
			})
			return oneWrite(path, next, "apply hermes provider"), err
		}
		next, err := updateHermesYAML(content, model, baseURL, apiKey)
		return oneWrite(path, next, "apply hermes provider"), err
	default:
		return nil, fmt.Errorf("%s 不支持该安全写入器", kind)
	}
}

func readManagedToolStatus(st *ToolConfigStatus, content string) bool {
	kind := ToolKind(st.Kind)
	if kind == ToolCodex || kind == ToolClaude {
		return false
	}
	if kind == ToolHermes && (strings.HasSuffix(strings.ToLower(st.Path), ".yaml") || strings.HasSuffix(strings.ToLower(st.Path), ".yml")) {
		var raw map[string]any
		if yaml.Unmarshal([]byte(content), &raw) != nil {
			return true
		}
		model := ensureStringMap(raw["model"])
		st.Model = stringValue(model["default"])
		st.ModelProvider = stringValue(model["base_url"])
		return true
	}
	if kind == ToolGemini && strings.HasSuffix(strings.ToLower(st.Path), ".env") {
		st.Model = readEnvValue(content, "GEMINI_MODEL")
		st.ModelProvider = readEnvValue(content, "GOOGLE_GEMINI_BASE_URL")
		return true
	}
	standard := []byte(content)
	if kind == ToolOpenClaw {
		value, err := hujson.Parse(quoteJSON5Keys(standard))
		if err != nil {
			return true
		}
		value.Standardize()
		standard = value.Pack()
	}
	var raw map[string]any
	if json.Unmarshal(standard, &raw) != nil {
		return true
	}
	switch kind {
	case ToolClaudeDesktop:
		st.ModelProvider = stringValue(raw["inferenceGatewayBaseUrl"])
		if models, ok := raw["inferenceModels"].([]any); ok && len(models) > 0 {
			st.Model = stringValue(models[0])
		}
	case ToolGemini:
		st.Model = stringValue(ensureStringMap(raw["model"])["name"])
	case ToolOpenCode:
		providerID, model := splitModelRef(stringValue(raw["model"]))
		st.Model = model
		provider := ensureStringMap(ensureStringMap(raw["provider"])[providerID])
		st.ModelProvider = stringValue(ensureStringMap(provider["options"])["baseURL"])
	case ToolOpenClaw:
		primary := stringValue(ensureStringMap(ensureStringMap(ensureStringMap(raw["agents"])["defaults"])["model"])["primary"])
		providerID, model := splitModelRef(primary)
		st.Model = model
		provider := ensureStringMap(ensureStringMap(ensureStringMap(raw["models"])["providers"])[providerID])
		st.ModelProvider = stringValue(provider["baseUrl"])
	case ToolHermes:
		model := ensureStringMap(raw["model"])
		st.Model, st.ModelProvider = stringValue(model["default"]), stringValue(model["base_url"])
	}
	return true
}

func ensureStringMap(value any) map[string]any { out, _ := value.(map[string]any); return out }
func stringValue(value any) string             { out, _ := value.(string); return out }
func splitModelRef(ref string) (string, string) {
	if i := strings.IndexByte(ref, '/'); i >= 0 {
		return ref[:i], ref[i+1:]
	}
	return "", ref
}

func oneWrite(path, content, reason string) []pendingConfigWrite {
	return []pendingConfigWrite{{path, content, reason}}
}

func readOptionalFile(path string) (string, error) {
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return "", nil
	}
	return string(b), err
}

func updateJSONObject(content string, mutate func(map[string]any)) (string, error) {
	raw := map[string]any{}
	if strings.TrimSpace(content) != "" {
		if err := json.Unmarshal([]byte(content), &raw); err != nil {
			return "", fmt.Errorf("JSON 配置无效: %w", err)
		}
	}
	mutate(raw)
	b, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return "", err
	}
	return string(b) + "\n", nil
}

func updateJSON5Object(content string, mutate func(map[string]any)) (string, error) {
	standard := []byte("{}")
	if strings.TrimSpace(content) != "" {
		value, err := hujson.Parse(quoteJSON5Keys([]byte(content)))
		if err != nil {
			return "", fmt.Errorf("JSON5 配置无效: %w", err)
		}
		value.Standardize()
		standard = value.Pack()
	}
	return updateJSONObject(string(standard), mutate)
}

var json5BareKey = regexp.MustCompile(`([,{]\s*)([A-Za-z_$][A-Za-z0-9_$-]*)(\s*:)`)
var json5LineBareKey = regexp.MustCompile(`(?m)^(\s*)([A-Za-z_$][A-Za-z0-9_$-]*)(\s*:)`)

func quoteJSON5Keys(content []byte) []byte {
	content = json5BareKey.ReplaceAll(content, []byte(`${1}"${2}"${3}`))
	return json5LineBareKey.ReplaceAll(content, []byte(`${1}"${2}"${3}`))
}

func nestedMap(parent map[string]any, key string) map[string]any {
	child := ensureMap(parent[key])
	parent[key] = child
	return child
}

func ensureMap(value any) map[string]any {
	if out, ok := value.(map[string]any); ok {
		return out
	}
	return map[string]any{}
}

func updateEnv(content string, values map[string]string) string {
	lines := strings.Split(strings.TrimSuffix(content, "\n"), "\n")
	if len(lines) == 1 && lines[0] == "" {
		lines = nil
	}
	for key, value := range values {
		if value == "" {
			continue
		}
		re := regexp.MustCompile(`^\s*(?:export\s+)?` + regexp.QuoteMeta(key) + `\s*=`)
		replacement := key + "=" + quoteEnv(value)
		found := false
		for i, line := range lines {
			if re.MatchString(line) {
				lines[i], found = replacement, true
				break
			}
		}
		if !found {
			lines = append(lines, replacement)
		}
	}
	return strings.Join(lines, "\n") + "\n"
}

func quoteEnv(value string) string {
	if !strings.ContainsAny(value, " #\t\"'") {
		return value
	}
	return `"` + strings.ReplaceAll(strings.ReplaceAll(value, `\`, `\\`), `"`, `\"`) + `"`
}

func updateHermesYAML(content, model, baseURL, apiKey string) (string, error) {
	root := &yaml.Node{}
	if strings.TrimSpace(content) == "" {
		root = &yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{{Kind: yaml.MappingNode}}}
	} else if err := yaml.Unmarshal([]byte(content), root); err != nil {
		return "", fmt.Errorf("YAML 配置无效: %w", err)
	}
	if len(root.Content) == 0 || root.Content[0].Kind != yaml.MappingNode {
		return "", fmt.Errorf("Hermes YAML 根节点必须是对象")
	}
	modelNode := yamlMapChild(root.Content[0], "model")
	setYAMLScalar(modelNode, "default", model)
	setYAMLScalar(modelNode, "provider", "custom")
	if baseURL != "" {
		setYAMLScalar(modelNode, "base_url", baseURL)
	}
	if apiKey != "" {
		setYAMLScalar(modelNode, "api_key", apiKey)
	}
	var out bytes.Buffer
	enc := yaml.NewEncoder(&out)
	enc.SetIndent(2)
	if err := enc.Encode(root); err != nil {
		return "", err
	}
	_ = enc.Close()
	return out.String(), nil
}

func yamlMapChild(parent *yaml.Node, key string) *yaml.Node {
	for i := 0; i+1 < len(parent.Content); i += 2 {
		if parent.Content[i].Value == key {
			if parent.Content[i+1].Kind != yaml.MappingNode {
				parent.Content[i+1] = &yaml.Node{Kind: yaml.MappingNode}
			}
			return parent.Content[i+1]
		}
	}
	child := &yaml.Node{Kind: yaml.MappingNode}
	parent.Content = append(parent.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}, child)
	return child
}

func setYAMLScalar(parent *yaml.Node, key, value string) {
	for i := 0; i+1 < len(parent.Content); i += 2 {
		if parent.Content[i].Value == key {
			parent.Content[i+1] = &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value}
			return
		}
	}
	parent.Content = append(parent.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value})
}

func isClaudeDesktopRoleModel(model string) bool {
	model = strings.TrimPrefix(strings.ToLower(model), "anthropic/")
	return strings.HasPrefix(model, "claude-sonnet-") || strings.HasPrefix(model, "claude-opus-") || strings.HasPrefix(model, "claude-haiku-")
}

func applyConfigWrites(writes []pendingConfigWrite) ([]ConfigWriteResult, error) {
	type original struct {
		path    string
		content []byte
		existed bool
	}
	originals := make([]original, 0, len(writes))
	results := make([]ConfigWriteResult, 0, len(writes))
	for _, write := range writes {
		before, err := os.ReadFile(write.Path)
		existed := err == nil
		if err != nil && !os.IsNotExist(err) {
			return results, err
		}
		result, err := writeConfigWithSnapshot(write.Path, write.Content, write.Reason)
		if err != nil {
			for i := len(originals) - 1; i >= 0; i-- {
				if originals[i].existed {
					_ = writeFileAtomic(originals[i].path, string(originals[i].content))
				} else {
					_ = os.Remove(originals[i].path)
				}
			}
			return results, err
		}
		originals = append(originals, original{write.Path, before, existed})
		results = append(results, result)
	}
	return results, nil
}
