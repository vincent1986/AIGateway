package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// DeepSeek Claude Code official migration (from DeepSeek docs):
//
//	export ANTHROPIC_BASE_URL=https://api.deepseek.com/anthropic
//	export ANTHROPIC_AUTH_TOKEN=<DeepSeek API Key>
//	export ANTHROPIC_MODEL=deepseek-v4-pro[1m]
//	export ANTHROPIC_DEFAULT_OPUS_MODEL=deepseek-v4-pro[1m]
//	export ANTHROPIC_DEFAULT_SONNET_MODEL=deepseek-v4-pro[1m]
//	export ANTHROPIC_DEFAULT_HAIKU_MODEL=deepseek-v4-flash
//	export CLAUDE_CODE_SUBAGENT_MODEL=deepseek-v4-flash
//	export CLAUDE_CODE_EFFORT_LEVEL=max

const (
	deepSeekAnthropicBase = "https://api.deepseek.com/anthropic"
	deepSeekDefaultPro    = "deepseek-v4-pro[1m]"
	deepSeekDefaultFlash  = "deepseek-v4-flash"
)

// Claude Code appends /v1/messages to ANTHROPIC_BASE_URL itself. The local
// proxy exposes that route, so its client-facing base must not include /v1.
func normalizeClaudeGatewayURL(baseURL string) string {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if isLocalProxyURL(baseURL) {
		baseURL = strings.TrimSuffix(baseURL, "/v1")
	}
	return strings.TrimRight(baseURL, "/")
}

// ClaudeDeepSeekRequest configures Claude Code for DeepSeek Anthropic-compatible API.
type ClaudeDeepSeekRequest struct {
	APIKey      string `json:"apiKey"`      // DeepSeek API Key
	Path        string `json:"path"`        // optional settings.json path
	MainModel   string `json:"mainModel"`   // default deepseek-v4-pro[1m]
	HaikuModel  string `json:"haikuModel"`  // default deepseek-v4-flash
	EffortLevel string `json:"effortLevel"` // default max
	// Deprecated compatibility field. Model changes are scoped to settings.json;
	// this flag is intentionally ignored and never changes global environment.
	SetSystemEnv bool `json:"setSystemEnv"`
}

// ApplyDeepSeekClaudeCode migrates Claude Code to DeepSeek per official docs.
func (a *App) ApplyDeepSeekClaudeCode(req ClaudeDeepSeekRequest) (ToolConfigStatus, error) {
	apiKey := strings.TrimSpace(req.APIKey)
	if apiKey == "" {
		if list, err := loadProvidersFromDisk(); err == nil {
			for _, p := range list {
				if isDeepSeekProvider(p) && strings.TrimSpace(p.APIKey) != "" {
					apiKey = strings.TrimSpace(p.APIKey)
					break
				}
			}
		}
	}
	if apiKey == "" {
		// also try secret store
		apiKey = loadProviderSecret("deepseek")
	}
	if apiKey == "" {
		return ToolConfigStatus{}, fmt.Errorf("请提供 DeepSeek API Key（或在厂家中配置 DeepSeek）")
	}

	mainModel := strings.TrimSpace(req.MainModel)
	if mainModel == "" {
		mainModel = deepSeekDefaultPro
	}
	mainModel = normalizeDeepSeekMainModel(mainModel)

	haiku := strings.TrimSpace(req.HaikuModel)
	if haiku == "" {
		haiku = deepSeekDefaultFlash
	}
	effort := strings.TrimSpace(req.EffortLevel)
	if effort == "" {
		effort = "max"
	}

	st := a.resolveTool(ToolClaude)
	path := expandPath(req.Path)
	if path == "" {
		path = st.Path
	}
	if path == "" {
		home := userHome()
		if home == "" {
			return st, fmt.Errorf("无法定位 home 目录")
		}
		path = filepath.Join(home, ".claude", "settings.json")
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return st, err
	}

	var content string
	var raw []byte
	if fileExists(path) {
		b, err := os.ReadFile(path)
		if err != nil {
			return st, err
		}
		raw = b
		content = string(b)
	}

	autoBakMsg := ""
	if len(raw) > 0 {
		created, err := ensureDefaultBackup(ToolClaude, path)
		if err != nil {
			return st, fmt.Errorf("备份默认配置失败: %w", err)
		}
		if created {
			autoBakMsg = "（已自动备份默认配置）"
		}
		savePreWriteSnapshot(ToolClaude, path, raw)
	}

	next, err := applyClaudeDeepSeekSettings(content, apiKey, mainModel, haiku, effort)
	if err != nil {
		return st, err
	}
	if err := writeFileAtomic(path, next); err != nil {
		return st, err
	}
	if err := validateToolConfigWrite(ToolClaude, path, mainModel); err != nil {
		return st, err
	}

	_, _ = a.SetToolConfigPath("claude", path)

	st = a.resolveTool(ToolClaude)
	st.Message = fmt.Sprintf(
		"已按 DeepSeek 官方文档迁移 Claude Code：BASE=%s MODEL=%s%s",
		deepSeekAnthropicBase, mainModel, autoBakMsg,
	)
	return st, nil
}

func isDeepSeekProvider(p Provider) bool {
	n := strings.ToLower(p.Name + " " + p.BaseURL + " " + p.ID)
	return strings.Contains(n, "deepseek")
}

func isDeepSeekHint(providerHint, baseURL, model string) bool {
	h := strings.ToLower(providerHint + " " + baseURL + " " + model)
	return strings.Contains(h, "deepseek")
}

func normalizeDeepSeekMainModel(model string) string {
	m := strings.TrimSpace(model)
	switch strings.ToLower(m) {
	case "deepseek-v4-pro", "deepseek-v4-pro[1m]":
		return deepSeekDefaultPro
	case "deepseek-chat", "deepseek-reasoner":
		// keep as-is for older ids; user may still want them
		return m
	default:
		return m
	}
}

func applyClaudeDeepSeekSettings(content, apiKey, mainModel, haiku, effort string) (string, error) {
	var raw map[string]any
	if strings.TrimSpace(content) == "" {
		raw = map[string]any{}
	} else if err := json.Unmarshal([]byte(content), &raw); err != nil {
		return "", fmt.Errorf("解析 settings.json 失败: %w", err)
	}
	if raw == nil {
		raw = map[string]any{}
	}

	raw["model"] = mainModel

	env, _ := raw["env"].(map[string]any)
	if env == nil {
		env = map[string]any{}
	}
	// Claude Code rejects configurations that define both credential modes.
	delete(env, "ANTHROPIC_API_KEY")

	// Official DeepSeek → Claude Code env block
	env["ANTHROPIC_BASE_URL"] = deepSeekAnthropicBase
	env["ANTHROPIC_AUTH_TOKEN"] = apiKey
	env["ANTHROPIC_MODEL"] = mainModel
	env["ANTHROPIC_CUSTOM_MODEL_OPTION"] = mainModel
	env["ANTHROPIC_CUSTOM_MODEL_OPTION_NAME"] = mainModel
	env["ANTHROPIC_DEFAULT_OPUS_MODEL"] = mainModel
	env["ANTHROPIC_DEFAULT_SONNET_MODEL"] = mainModel
	env["ANTHROPIC_DEFAULT_HAIKU_MODEL"] = haiku
	env["CLAUDE_CODE_SUBAGENT_MODEL"] = haiku
	env["CLAUDE_CODE_EFFORT_LEVEL"] = effort

	raw["env"] = env

	b, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return "", err
	}
	return string(b) + "\n", nil
}

// applyClaudeEnvForProvider fills Claude settings env for a provider (DeepSeek uses official mapping).
func applyClaudeEnvForProvider(env map[string]any, model, baseURL, apiKey, providerHint string) {
	if env == nil {
		return
	}
	// Use one credential mode consistently. AUTH_TOKEN is the mode supported
	// for custom Anthropic-compatible gateways.
	delete(env, "ANTHROPIC_API_KEY")
	if isDeepSeekHint(providerHint, baseURL, model) {
		main := normalizeDeepSeekMainModel(model)
		if main == "" {
			main = deepSeekDefaultPro
		}
		haiku := deepSeekDefaultFlash
		if strings.Contains(strings.ToLower(main), "flash") {
			// user selected flash as primary — apply everywhere
			env["ANTHROPIC_BASE_URL"] = deepSeekAnthropicBase
			if apiKey != "" {
				env["ANTHROPIC_AUTH_TOKEN"] = apiKey
			}
			env["ANTHROPIC_MODEL"] = main
			env["ANTHROPIC_CUSTOM_MODEL_OPTION"] = main
			env["ANTHROPIC_CUSTOM_MODEL_OPTION_NAME"] = main
			env["ANTHROPIC_DEFAULT_OPUS_MODEL"] = main
			env["ANTHROPIC_DEFAULT_SONNET_MODEL"] = main
			env["ANTHROPIC_DEFAULT_HAIKU_MODEL"] = main
			env["CLAUDE_CODE_SUBAGENT_MODEL"] = main
			env["CLAUDE_CODE_EFFORT_LEVEL"] = "max"
			return
		}
		env["ANTHROPIC_BASE_URL"] = deepSeekAnthropicBase
		if apiKey != "" {
			env["ANTHROPIC_AUTH_TOKEN"] = apiKey
		}
		env["ANTHROPIC_MODEL"] = main
		env["ANTHROPIC_CUSTOM_MODEL_OPTION"] = main
		env["ANTHROPIC_CUSTOM_MODEL_OPTION_NAME"] = main
		env["ANTHROPIC_DEFAULT_OPUS_MODEL"] = main
		env["ANTHROPIC_DEFAULT_SONNET_MODEL"] = main
		env["ANTHROPIC_DEFAULT_HAIKU_MODEL"] = haiku
		env["CLAUDE_CODE_SUBAGENT_MODEL"] = haiku
		env["CLAUDE_CODE_EFFORT_LEVEL"] = "max"
		return
	}

	// generic third-party / anthropic
	if model != "" {
		env["ANTHROPIC_MODEL"] = model
		env["ANTHROPIC_CUSTOM_MODEL_OPTION"] = model
		env["ANTHROPIC_CUSTOM_MODEL_OPTION_NAME"] = model
		env["ANTHROPIC_DEFAULT_OPUS_MODEL"] = model
		env["ANTHROPIC_DEFAULT_SONNET_MODEL"] = model
		env["ANTHROPIC_DEFAULT_HAIKU_MODEL"] = model
	}
	if baseURL != "" {
		env["ANTHROPIC_BASE_URL"] = normalizeClaudeGatewayURL(baseURL)
	}
	if apiKey != "" {
		env["ANTHROPIC_AUTH_TOKEN"] = apiKey
	}
}
