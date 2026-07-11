package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
)

// Per-tool virtual model ids written into tool configs on InjectGateway.
// Each app requests its own virtual id so proxy bindings never cross-affect.
const (
	gatewayVirtualModel         = "aiSwitchModel" // legacy alias → chatgpt
	gatewayVirtualModelChatGPT  = "aiSwitchModel-chatgpt"
	gatewayVirtualModelClaude   = "aiSwitchModel-claude"
	gatewayVirtualModelOpenClaw = "aiSwitchModel-openclaw"
	gatewayVirtualModelHarness  = "aiSwitchModel-harness"
)

// meta keys
const (
	metaActiveGatewayModel  = "active_gateway_model"  // legacy single binding
	metaActiveGatewayModels = "active_gateway_models" // JSON map tool→model
)

// tool keys stored in meta JSON
const (
	toolKeyChatGPT  = "chatgpt"
	toolKeyClaude   = "claude"
	toolKeyOpenClaw = "openclaw"
	toolKeyHarness  = "harness"
)

var (
	activeModelMu    sync.RWMutex
	activeModelCache map[string]string // toolKey → real model id
)

func allToolKeys() []string {
	return []string{toolKeyChatGPT, toolKeyClaude, toolKeyOpenClaw, toolKeyHarness}
}

func normalizeToolKey(kind string) string {
	k := strings.ToLower(strings.TrimSpace(kind))
	switch k {
	case "codex", "chatgpt", "gpt", "openai-codex":
		return toolKeyChatGPT
	case "claude", "claude_code", "claudecode":
		return toolKeyClaude
	case "openclaw":
		return toolKeyOpenClaw
	case "harness":
		return toolKeyHarness
	case "":
		return toolKeyChatGPT
	default:
		return k
	}
}

// virtualModelForTool returns the stable model id pinned in that tool's config.
func virtualModelForTool(kind string) string {
	switch normalizeToolKey(kind) {
	case toolKeyClaude:
		return gatewayVirtualModelClaude
	case toolKeyOpenClaw:
		return gatewayVirtualModelOpenClaw
	case toolKeyHarness:
		return gatewayVirtualModelHarness
	default:
		return gatewayVirtualModelChatGPT
	}
}

// toolKeyFromVirtualModel maps a client model string to tool key; ok=false if not virtual.
func toolKeyFromVirtualModel(model string) (toolKey string, ok bool) {
	m := strings.ToLower(strings.TrimSpace(model))
	// strip aigateway/ prefix used by OpenClaw-style refs
	if strings.HasPrefix(m, "aigateway/") {
		m = strings.TrimPrefix(m, "aigateway/")
	}
	switch m {
	case "aiswitchmodel-chatgpt", "aiswitchmodel_chatgpt", "ai-switch-model-chatgpt":
		return toolKeyChatGPT, true
	case "aiswitchmodel-claude", "aiswitchmodel_claude", "ai-switch-model-claude":
		return toolKeyClaude, true
	case "aiswitchmodel-openclaw", "aiswitchmodel_openclaw", "ai-switch-model-openclaw":
		return toolKeyOpenClaw, true
	case "aiswitchmodel-harness", "aiswitchmodel_harness", "ai-switch-model-harness":
		return toolKeyHarness, true
	// legacy shared aliases → ChatGPT binding (backward compatible)
	case "aiswitchmodel", "ai-switch-model", "ai_switch_model",
		"aigateway", "default", "gateway", "codex-proxy", "":
		return toolKeyChatGPT, true
	default:
		return "", false
	}
}

// isGatewayVirtualModel reports whether the client model is a hot-switch alias.
func isGatewayVirtualModel(model string) bool {
	_, ok := toolKeyFromVirtualModel(model)
	// toolKeyFromVirtualModel also treats empty as chatgpt — avoid empty string
	if strings.TrimSpace(model) == "" {
		return false
	}
	return ok
}

// allVirtualModelIDs listed by /v1/models (real tools pin one of these).
func allVirtualModelIDs() []string {
	return []string{
		gatewayVirtualModelChatGPT,
		gatewayVirtualModelClaude,
		gatewayVirtualModelOpenClaw,
		gatewayVirtualModelHarness,
		gatewayVirtualModel, // legacy
	}
}

func loadActiveModelMap() map[string]string {
	activeModelMu.RLock()
	if activeModelCache != nil {
		out := make(map[string]string, len(activeModelCache))
		for k, v := range activeModelCache {
			out[k] = v
		}
		activeModelMu.RUnlock()
		return out
	}
	activeModelMu.RUnlock()

	out := map[string]string{}
	if db, err := openDB(); err == nil {
		if raw := strings.TrimSpace(metaGet(db, metaActiveGatewayModels)); raw != "" {
			_ = json.Unmarshal([]byte(raw), &out)
		}
		// migrate legacy single key → chatgpt (and fill empty slots as soft default)
		if legacy := strings.TrimSpace(metaGet(db, metaActiveGatewayModel)); legacy != "" {
			if out[toolKeyChatGPT] == "" {
				out[toolKeyChatGPT] = legacy
			}
		}
	}
	if out == nil {
		out = map[string]string{}
	}
	activeModelMu.Lock()
	activeModelCache = out
	activeModelMu.Unlock()
	return out
}

func saveActiveModelMap(m map[string]string) error {
	if m == nil {
		m = map[string]string{}
	}
	activeModelMu.Lock()
	activeModelCache = m
	activeModelMu.Unlock()
	b, err := json.Marshal(m)
	if err != nil {
		return err
	}
	if db, err := openDB(); err == nil {
		if err := metaSet(db, metaActiveGatewayModels, string(b)); err != nil {
			return err
		}
		// keep legacy key in sync with chatgpt for older readers
		if v := m[toolKeyChatGPT]; v != "" {
			_ = metaSet(db, metaActiveGatewayModel, v)
		}
	}
	return nil
}

// resolveActiveModelIDForTool returns the real model bound for a tool key.
func resolveActiveModelIDForTool(toolKey string) string {
	toolKey = normalizeToolKey(toolKey)
	m := loadActiveModelMap()
	if v := strings.TrimSpace(m[toolKey]); v != "" {
		return v
	}
	// do not auto-write across tools — only fall back to discovery without polluting map
	return discoverDefaultModelID()
}

// resolveActiveModelID is legacy helper (chatgpt / shared alias path).
func resolveActiveModelID() string {
	return resolveActiveModelIDForTool(toolKeyChatGPT)
}

func discoverDefaultModelID() string {
	list, err := loadProvidersFromDisk()
	if err != nil {
		return ""
	}
	for _, p := range list {
		for _, m := range p.Models {
			if m.Enabled && m.IsDefault && strings.TrimSpace(m.ID) != "" {
				return strings.TrimSpace(m.ID)
			}
		}
	}
	for _, p := range list {
		for _, m := range p.Models {
			if m.Enabled && strings.TrimSpace(m.ID) != "" {
				return strings.TrimSpace(m.ID)
			}
		}
	}
	if db, err := openDB(); err == nil {
		var id string
		_ = db.QueryRow(`SELECT id FROM model_groups WHERE enabled = 1 ORDER BY name COLLATE NOCASE LIMIT 1`).Scan(&id)
		return strings.TrimSpace(id)
	}
	return ""
}

func normalizeRealModelID(modelID string) (string, error) {
	modelID = strings.TrimSpace(modelID)
	if modelID == "" {
		return "", fmt.Errorf("模型不能为空")
	}
	if isGatewayVirtualModel(modelID) {
		return "", fmt.Errorf("不能将虚拟模型设为绑定目标")
	}
	if i := strings.Index(modelID, "/"); i > 0 {
		pref := strings.ToLower(modelID[:i])
		if pref == "aigateway" {
			modelID = modelID[i+1:]
		}
	}
	if isGatewayVirtualModel(modelID) {
		return "", fmt.Errorf("不能将虚拟模型设为绑定目标")
	}
	return modelID, nil
}

func persistActiveGatewayModelForTool(toolKey, modelID string) error {
	toolKey = normalizeToolKey(toolKey)
	modelID, err := normalizeRealModelID(modelID)
	if err != nil {
		return err
	}
	m := loadActiveModelMap()
	if m == nil {
		m = map[string]string{}
	}
	// copy to avoid races with concurrent readers holding the same map ref
	next := make(map[string]string, len(m)+1)
	for k, v := range m {
		next[k] = v
	}
	next[toolKey] = modelID
	return saveActiveModelMap(next)
}

// persistActiveGatewayModel binds for chatgpt only (legacy callers / codex auto-sync).
func persistActiveGatewayModel(modelID string) error {
	return persistActiveGatewayModelForTool(toolKeyChatGPT, modelID)
}

// ActiveGatewayModelInfo is exposed to the UI (one tool).
type ActiveGatewayModelInfo struct {
	Kind         string   `json:"kind"`         // chatgpt | claude | openclaw | harness
	VirtualModel string   `json:"virtualModel"` // pinned in tool config
	ActiveModel  string   `json:"activeModel"`  // real model group
	Aliases      []string `json:"aliases"`
}

// GetActiveGatewayModel returns binding for one tool (empty kind → chatgpt).
func (a *App) GetActiveGatewayModel(kind string) ActiveGatewayModelInfo {
	toolKey := normalizeToolKey(kind)
	virt := virtualModelForTool(toolKey)
	return ActiveGatewayModelInfo{
		Kind:         toolKey,
		VirtualModel: virt,
		ActiveModel:  resolveActiveModelIDForTool(toolKey),
		Aliases:      []string{virt, gatewayVirtualModel},
	}
}

// ListActiveGatewayModels returns bindings for all managed tools.
func (a *App) ListActiveGatewayModels() []ActiveGatewayModelInfo {
	out := make([]ActiveGatewayModelInfo, 0, 4)
	for _, k := range allToolKeys() {
		out = append(out, a.GetActiveGatewayModel(k))
	}
	return out
}

// SetActiveGatewayModel binds one tool's virtual model to a real model group.
// kind: chatgpt|codex|claude|openclaw|harness — independent per app.
func (a *App) SetActiveGatewayModel(kind, modelID string) (ActiveGatewayModelInfo, error) {
	toolKey := normalizeToolKey(kind)
	modelID = strings.TrimSpace(modelID)
	if modelID == "" {
		return a.GetActiveGatewayModel(toolKey), fmt.Errorf("模型不能为空")
	}
	// validate routable
	if cands, err := resolveRoutesLegacy(modelID); err != nil || len(cands) == 0 {
		if db, err2 := openDB(); err2 == nil {
			var n int
			_ = db.QueryRow(`SELECT COUNT(1) FROM model_groups WHERE id = ?`, modelID).Scan(&n)
			if n == 0 {
				if c2, e2 := resolveRoutesFromSQL(db, modelID); e2 != nil || len(c2) == 0 {
					if err != nil {
						return a.GetActiveGatewayModel(toolKey), fmt.Errorf("无法设为默认: %w", err)
					}
					return a.GetActiveGatewayModel(toolKey), fmt.Errorf("未找到模型 %q 的可用路由", modelID)
				}
			}
		} else if err != nil {
			return a.GetActiveGatewayModel(toolKey), fmt.Errorf("无法设为默认: %w", err)
		}
	}
	if err := persistActiveGatewayModelForTool(toolKey, modelID); err != nil {
		return a.GetActiveGatewayModel(toolKey), err
	}
	if a.proxy != nil {
		st := a.proxy.status()
		if !st.Running {
			_ = a.proxy.start()
		}
	}
	return a.GetActiveGatewayModel(toolKey), nil
}

// clearActiveModelCache used by closeDB / tests.
func clearActiveModelCache() {
	activeModelMu.Lock()
	activeModelCache = nil
	activeModelMu.Unlock()
}
