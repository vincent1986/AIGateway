package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const gatewayProviderID = "aigateway"

// InjectGateway points a tool's base_url at the local AIGateway forever (PRD 3.4).
// kind: codex|chatgpt|claude|openclaw|harness
func (a *App) InjectGateway(kind string) (ToolConfigStatus, error) {
	rawKind := strings.ToLower(strings.TrimSpace(kind))
	if rawKind == "chatgpt" {
		rawKind = "codex"
	}
	k := ToolKind(rawKind)
	d := driverByID(kind)
	if d == nil {
		d = driverByID(string(k))
	}
	if d == nil && k != ToolCodex && k != ToolClaude {
		return ToolConfigStatus{}, fmt.Errorf("未知工具类型: %s", kind)
	}

	if a.proxy == nil {
		a.proxy = newProxyServer()
	}
	if !a.proxy.status().Running {
		if err := a.proxy.start(); err != nil {
			return a.resolveTool(k), fmt.Errorf("启动网关失败: %w", err)
		}
	}
	base := a.proxy.baseURL()
	if base == "" {
		base = "http://127.0.0.1:18080/v1"
	}

	st := a.resolveTool(k)
	path := st.Path
	if path == "" && d != nil {
		path = d.PreferredPath()
	}
	if path == "" {
		return st, fmt.Errorf("未找到配置路径，请先手动选择")
	}

	var raw []byte
	if fileExists(path) {
		b, err := os.ReadFile(path)
		if err != nil {
			return st, err
		}
		raw = b
		if _, err := ensureDefaultBackup(k, path); err != nil {
			// new tools may not have backup kind registered — still snapshot file
			_ = err
		}
		savePreWriteSnapshot(k, path, raw)
	} else {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return st, err
		}
	}

	var err error
	// OpenClaw needs provider catalog from local enabled models
	if k == ToolOpenClaw {
		err = injectOpenClawGateway(path, base, "aigateway", collectEnabledModelIDs())
	} else if d != nil {
		err = d.InjectGateway(path, base, "aigateway")
	} else {
		// legacy codex/claude only
		var next string
		switch k {
		case ToolCodex:
			next, err = injectGatewayCodex(string(raw), base)
		case ToolClaude:
			next, err = injectGatewayClaude(string(raw), base)
		}
		if err == nil {
			next = preserveLineEndings(string(raw), next)
			err = writeFileAtomic(path, next)
		}
	}
	_ = k // tool-specific virtual model applied inside inject helpers
	if err != nil {
		return st, err
	}

	// remember override path
	ov := a.loadOverrides()
	switch k {
	case ToolCodex:
		if strings.TrimSpace(ov.Codex) == "" {
			ov.Codex = path
		}
	case ToolClaude:
		if strings.TrimSpace(ov.Claude) == "" {
			ov.Claude = path
		}
	case ToolOpenClaw:
		if strings.TrimSpace(ov.OpenClaw) == "" {
			ov.OpenClaw = path
		}
	case ToolHarness:
		if strings.TrimSpace(ov.Harness) == "" {
			ov.Harness = path
		}
	}
	_ = a.saveOverrides(ov)

	st = a.resolveTool(k)
	virt := virtualModelForTool(string(k))
	active := resolveActiveModelIDForTool(string(k))
	if active == "" {
		st.Message = fmt.Sprintf("已接管：base_url → %s，模型固定为 %s（在应用管理切换模型，各应用独立）", base, virt)
	} else {
		st.Message = fmt.Sprintf("已接管：base_url → %s，模型=%s → 当前 %s（仅本应用；热切换不写配置文件）", base, virt, active)
	}
	return st, nil
}

// RollbackGateway restores the default backup for a tool (卸载/还原).
func (a *App) RollbackGateway(kind string) (ToolConfigStatus, error) {
	rawKind := strings.ToLower(strings.TrimSpace(kind))
	if rawKind == "chatgpt" {
		rawKind = "codex"
	}
	return a.RestoreDefaultConfig(rawKind)
}

func injectGatewayCodex(content, gatewayBase string) (string, error) {
	if strings.TrimSpace(content) == "" {
		content = "# AIGateway managed\n"
	}
	content = removeTomlProviderBlock(content, "codex_proxy")
	// Local gateway usually has no auth — use inline api_key so Codex does not
	// require a missing system env (Missing environment variable: aigateway_api_key).
	const localKey = "aigateway"
	content = upsertCodexModelProvider(content, gatewayProviderID, "AIGateway", gatewayBase, localKey)
	content = setProviderField(content, gatewayProviderID, "base_url", gatewayBase)
	content = setProviderField(content, gatewayProviderID, "api_key", localKey)
	// Drop env_key so Codex won't look for unset AIGATEWAY env vars
	content = removeProviderField(content, gatewayProviderID, "env_key")
	content = setTomlTopLevelString(content, "model_provider", gatewayProviderID)
	// Pin ChatGPT-scoped virtual model — independent of Claude/OpenClaw/Harness.
	content = setTomlTopLevelString(content, "model", virtualModelForTool(toolKeyChatGPT))
	return content, nil
}

// injectGatewayClaude rewrites Claude Code ~/.claude/settings.json per official docs:
// route traffic via ANTHROPIC_BASE_URL (+ AUTH_TOKEN/API_KEY).
// Model is pinned to virtual aiSwitchModel; hot-switch happens in the proxy only.
// Do NOT set OPENAI_BASE_URL or top-level apiBaseUrl — Claude Code ignores those.
func injectGatewayClaude(content, gatewayBase string) (string, error) {
	var root map[string]any
	if strings.TrimSpace(content) == "" {
		root = map[string]any{}
	} else if err := json.Unmarshal([]byte(content), &root); err != nil {
		root = map[string]any{}
	}
	env, _ := root["env"].(map[string]any)
	if env == nil {
		env = map[string]any{}
	}
	env["ANTHROPIC_BASE_URL"] = gatewayBase
	// Local gateway accepts any non-empty key; keep existing tokens if present.
	if s, _ := env["ANTHROPIC_AUTH_TOKEN"].(string); strings.TrimSpace(s) == "" {
		env["ANTHROPIC_AUTH_TOKEN"] = "aigateway"
	}
	if s, _ := env["ANTHROPIC_API_KEY"].(string); strings.TrimSpace(s) == "" {
		env["ANTHROPIC_API_KEY"] = "aigateway"
	}
	// Pin Claude-scoped virtual model (independent of other apps)
	virt := virtualModelForTool(toolKeyClaude)
	env["ANTHROPIC_MODEL"] = virt
	env["ANTHROPIC_DEFAULT_OPUS_MODEL"] = virt
	env["ANTHROPIC_DEFAULT_SONNET_MODEL"] = virt
	env["ANTHROPIC_DEFAULT_HAIKU_MODEL"] = virt
	// Clean legacy incorrect keys from earlier AIGateway versions
	delete(env, "OPENAI_BASE_URL")
	delete(env, "OPENAI_API_KEY")
	root["env"] = env
	root["model"] = virt
	delete(root, "apiBaseUrl")
	delete(root, "baseUrl")
	delete(root, "base_url")
	b, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return "", err
	}
	return string(b) + "\n", nil
}
