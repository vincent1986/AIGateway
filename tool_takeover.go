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
// kind: "codex" (ChatGPT) | "claude"
func (a *App) InjectGateway(kind string) (ToolConfigStatus, error) {
	k := ToolKind(strings.ToLower(strings.TrimSpace(kind)))
	if k != ToolCodex && k != ToolClaude {
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
	if path == "" {
		if k == ToolCodex {
			path = preferredCodexConfigPath()
		} else {
			path = preferredClaudeConfigPath()
		}
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
			return st, fmt.Errorf("备份失败: %w", err)
		}
		savePreWriteSnapshot(k, path, raw)
	} else {
		// ensure parent dir
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return st, err
		}
		raw = []byte{}
	}

	var next string
	var err error
	switch k {
	case ToolCodex:
		next, err = injectGatewayCodex(string(raw), base)
	case ToolClaude:
		next, err = injectGatewayClaude(string(raw), base)
	}
	if err != nil {
		return st, err
	}
	next = preserveLineEndings(string(raw), next)
	if err := writeFileAtomic(path, next); err != nil {
		return st, err
	}

	// remember override path if missing
	ov := a.loadOverrides()
	if k == ToolCodex && strings.TrimSpace(ov.Codex) == "" {
		ov.Codex = path
		_ = a.saveOverrides(ov)
	}
	if k == ToolClaude && strings.TrimSpace(ov.Claude) == "" {
		ov.Claude = path
		_ = a.saveOverrides(ov)
	}

	st = a.resolveTool(k)
	st.Message = fmt.Sprintf("已接管：base_url → %s（后续模型切换在「模型管理」完成）", base)
	return st, nil
}

// RollbackGateway restores the default backup for a tool (卸载/还原).
func (a *App) RollbackGateway(kind string) (ToolConfigStatus, error) {
	return a.RestoreDefaultConfig(kind)
}

func injectGatewayCodex(content, gatewayBase string) (string, error) {
	if strings.TrimSpace(content) == "" {
		content = "# AIGateway managed\n"
	}
	// strip legacy codex_proxy block
	content = removeTomlProviderBlock(content, "codex_proxy")
	content = upsertCodexModelProvider(content, gatewayProviderID, "AIGateway", gatewayBase, "")
	content = setProviderField(content, gatewayProviderID, "base_url", gatewayBase)
	// point active provider at gateway
	content = setTomlTopLevelString(content, "model_provider", gatewayProviderID)
	// keep model if present; otherwise leave unset
	return content, nil
}

func injectGatewayClaude(content, gatewayBase string) (string, error) {
	// settings.json style: merge env + optional anthropic base
	var root map[string]any
	if strings.TrimSpace(content) == "" {
		root = map[string]any{}
	} else if err := json.Unmarshal([]byte(content), &root); err != nil {
		// not JSON — wrap as minimal settings
		root = map[string]any{}
	}
	env, _ := root["env"].(map[string]any)
	if env == nil {
		env = map[string]any{}
	}
	// OpenAI-compatible clients often use OPENAI_BASE_URL; Claude Code may use ANTHROPIC_BASE_URL
	env["OPENAI_BASE_URL"] = gatewayBase
	env["ANTHROPIC_BASE_URL"] = gatewayBase
	root["env"] = env
	// some forks store apiBaseUrl
	root["apiBaseUrl"] = gatewayBase
	b, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return "", err
	}
	return string(b) + "\n", nil
}


