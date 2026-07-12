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

// ClaudeDeepSeekRequest configures Claude Code for DeepSeek Anthropic-compatible API.
type ClaudeDeepSeekRequest struct {
	APIKey      string `json:"apiKey"`      // DeepSeek API Key
	Path        string `json:"path"`        // optional settings.json path
	MainModel   string `json:"mainModel"`   // default deepseek-v4-pro[1m]
	HaikuModel  string `json:"haikuModel"`  // default deepseek-v4-flash
	EffortLevel string `json:"effortLevel"` // default max
	// SetSystemEnv also writes user/shell env so new terminals inherit
	SetSystemEnv bool `json:"setSystemEnv"`
}

// ApplyDeepSeekClaudeCode migrates Claude Code to DeepSeek per official docs.
func (a *App) ApplyDeepSeekClaudeCode(req ClaudeDeepSeekRequest) (ToolConfigStatus, error) {
	apiKey := strings.TrimSpace(req.APIKey)
	if apiKey == "" {
		// try from saved DeepSeek provider
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
		return ToolConfigStatus{}, fmt.Errorf("请提供 DeepSeek API Key（或在厂家中配置 DeepSeek）")
	}

	mainModel := strings.TrimSpace(req.MainModel)
	if mainModel == "" {
		mainModel = deepSeekDefaultPro
	}
	haiku := strings.TrimSpace(req.HaikuModel)
	if haiku == "" {
		haiku = deepSeekDefaultFlash
	}
	effort := strings.TrimSpace(req.EffortLevel)
	if effort == "" {
		effort = "max"
	}

	// resolve settings path
	st := a.resolveTool(ToolClaude)
	path := expandPath(req.Path)
	if path == "" {
		path = st.Path
	}
	if path == "" {
		// create default user settings
		home := userHome()
		if home == "" {
			return st, fmt.Errorf("无法定位 home 目录")
		}
		path = filepath.Join(home, ".claude", "settings.json")
	}

	// ensure parent dir
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return st, err
	}

	var content string
	if fileExists(path) {
		b, err := os.ReadFile(path)
		if err != nil {
			return st, err
		}
		content = string(b)
	}

	next, err := applyClaudeDeepSeekSettings(content, apiKey, mainModel, haiku, effort)
	if err != nil {
		return st, err
	}
	if _, err := writeConfigWithSnapshot(path, next, "apply claude deepseek"); err != nil {
		return st, err
	}

	// remember path override
	_, _ = a.SetToolConfigPath("claude", path)

	// system env (optional, default true)
	setEnv := req.SetSystemEnv
	// zero value false — treat as true unless explicitly false via separate field;
	// for Wails JSON, missing bool is false, so default enable when not specified:
	// We always set process env; shell persistence when SetSystemEnv is true OR always try best-effort.
	_ = setClaudeDeepSeekSystemEnv(apiKey, mainModel, haiku, effort, true)

	st = a.resolveTool(ToolClaude)
	st.Message = fmt.Sprintf(
		"已迁移 Claude Code → DeepSeek：BASE=%s MODEL=%s（settings: %s）",
		deepSeekAnthropicBase, mainModel, path,
	)
	_ = setEnv
	return st, nil
}

func isDeepSeekProvider(p Provider) bool {
	n := strings.ToLower(p.Name + " " + p.BaseURL + " " + p.ID)
	return strings.Contains(n, "deepseek")
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

	// top-level model
	raw["model"] = mainModel

	env, _ := raw["env"].(map[string]any)
	if env == nil {
		env = map[string]any{}
	}

	// Official DeepSeek → Claude Code env block
	env["ANTHROPIC_BASE_URL"] = deepSeekAnthropicBase
	env["ANTHROPIC_AUTH_TOKEN"] = apiKey
	// keep API_KEY in sync for tools that still read it
	env["ANTHROPIC_API_KEY"] = apiKey
	env["ANTHROPIC_MODEL"] = mainModel
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

func setClaudeDeepSeekSystemEnv(apiKey, mainModel, haiku, effort string, persist bool) error {
	vars := map[string]string{
		"ANTHROPIC_BASE_URL":             deepSeekAnthropicBase,
		"ANTHROPIC_AUTH_TOKEN":           apiKey,
		"ANTHROPIC_API_KEY":              apiKey,
		"ANTHROPIC_MODEL":                mainModel,
		"ANTHROPIC_DEFAULT_OPUS_MODEL":   mainModel,
		"ANTHROPIC_DEFAULT_SONNET_MODEL": mainModel,
		"ANTHROPIC_DEFAULT_HAIKU_MODEL":  haiku,
		"CLAUDE_CODE_SUBAGENT_MODEL":     haiku,
		"CLAUDE_CODE_EFFORT_LEVEL":       effort,
	}
	for k, v := range vars {
		_ = os.Setenv(k, v)
		if persist {
			_ = setSystemEnvVar(k, v)
		}
	}
	return nil
}

// ApplyToolModel Claude branch helper: if DeepSeek-like, use official Anthropic base.
func applyClaudeEnvForProvider(env map[string]any, model, baseURL, apiKey, providerHint string) {
	if env == nil {
		return
	}
	hint := strings.ToLower(providerHint + " " + baseURL)
	isDS := strings.Contains(hint, "deepseek")

	if isDS {
		// Official Anthropic-compatible endpoint (NOT /v1 openai path)
		env["ANTHROPIC_BASE_URL"] = deepSeekAnthropicBase
		if apiKey != "" {
			env["ANTHROPIC_AUTH_TOKEN"] = apiKey
			env["ANTHROPIC_API_KEY"] = apiKey
		}
		// Model defaults: if user picks flash, use as haiku/subagent; else pro for main
		main := model
		if main == "" {
			main = deepSeekDefaultPro
		}
		haiku := deepSeekDefaultFlash
		if strings.Contains(strings.ToLower(main), "flash") {
			haiku = main
			// still keep pro for opus/sonnet if main is flash? docs use pro for main roles
			// If user selected flash as main model, apply flash everywhere they chose
			env["ANTHROPIC_MODEL"] = main
			env["ANTHROPIC_DEFAULT_OPUS_MODEL"] = main
			env["ANTHROPIC_DEFAULT_SONNET_MODEL"] = main
			env["ANTHROPIC_DEFAULT_HAIKU_MODEL"] = main
			env["CLAUDE_CODE_SUBAGENT_MODEL"] = main
		} else {
			// normalize short ids to official tagged names when possible
			if main == "deepseek-v4-pro" {
				main = deepSeekDefaultPro
			}
			env["ANTHROPIC_MODEL"] = main
			env["ANTHROPIC_DEFAULT_OPUS_MODEL"] = main
			env["ANTHROPIC_DEFAULT_SONNET_MODEL"] = main
			env["ANTHROPIC_DEFAULT_HAIKU_MODEL"] = haiku
			env["CLAUDE_CODE_SUBAGENT_MODEL"] = haiku
		}
		env["CLAUDE_CODE_EFFORT_LEVEL"] = "max"
		return
	}

	// generic third-party / anthropic
	if model != "" {
		env["ANTHROPIC_MODEL"] = model
		env["ANTHROPIC_DEFAULT_OPUS_MODEL"] = model
		env["ANTHROPIC_DEFAULT_SONNET_MODEL"] = model
		env["ANTHROPIC_DEFAULT_HAIKU_MODEL"] = model
	}
	if baseURL != "" {
		env["ANTHROPIC_BASE_URL"] = baseURL
	}
	if apiKey != "" {
		env["ANTHROPIC_AUTH_TOKEN"] = apiKey
		env["ANTHROPIC_API_KEY"] = apiKey
	}
}
