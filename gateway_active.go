package main

import (
	"fmt"
	"strings"
	"sync"
)

// Stable virtual model id written into tool configs once (InjectGateway).
// Clients keep requesting this model forever; AIGateway maps it to the active
// real model group in memory/SQLite — no further config file edits.
const gatewayVirtualModel = "aiSwitchModel"

// meta key for currently selected real model group id
const metaActiveGatewayModel = "active_gateway_model"

var (
	activeModelMu sync.RWMutex
	// in-memory cache; source of truth is SQLite meta when available
	activeModelCache string
)

// isGatewayVirtualModel reports whether the client model is the stable hot-switch alias.
func isGatewayVirtualModel(model string) bool {
	m := strings.ToLower(strings.TrimSpace(model))
	switch m {
	case "aiswitchmodel", "ai-switch-model", "ai_switch_model",
		"aigateway", "aigateway/default", "aigateway/aiswitchmodel",
		"default", "gateway", "codex-proxy":
		return true
	default:
		// also accept aigateway/<anything> that is only the virtual name
		if strings.HasPrefix(m, "aigateway/") {
			rest := strings.TrimPrefix(m, "aigateway/")
			return rest == "" || rest == "default" || rest == "aiswitchmodel" || rest == "ai-switch-model"
		}
		return false
	}
}

// resolveActiveModelID returns the real model group currently bound to aiSwitchModel.
func resolveActiveModelID() string {
	activeModelMu.RLock()
	cached := strings.TrimSpace(activeModelCache)
	activeModelMu.RUnlock()
	if cached != "" {
		return cached
	}
	if db, err := openDB(); err == nil {
		if v := strings.TrimSpace(metaGet(db, metaActiveGatewayModel)); v != "" {
			activeModelMu.Lock()
			activeModelCache = v
			activeModelMu.Unlock()
			return v
		}
	}
	// fallback: first enabled provider default, else first enabled model
	id := discoverDefaultModelID()
	if id != "" {
		_ = persistActiveGatewayModel(id)
	}
	return id
}

func discoverDefaultModelID() string {
	list, err := loadProvidersFromDisk()
	if err != nil {
		return ""
	}
	// prefer explicit isDefault
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
	// try model_groups table
	if db, err := openDB(); err == nil {
		var id string
		_ = db.QueryRow(`SELECT id FROM model_groups WHERE enabled = 1 ORDER BY name COLLATE NOCASE LIMIT 1`).Scan(&id)
		return strings.TrimSpace(id)
	}
	return ""
}

func persistActiveGatewayModel(modelID string) error {
	modelID = strings.TrimSpace(modelID)
	if modelID == "" {
		return fmt.Errorf("模型不能为空")
	}
	// strip virtual prefixes if user pasted aigateway/xxx by mistake
	if isGatewayVirtualModel(modelID) {
		return fmt.Errorf("不能将虚拟模型 %s 设为自身目标", gatewayVirtualModel)
	}
	if i := strings.Index(modelID, "/"); i > 0 {
		// aigateway/deepseek-v4-pro → deepseek-v4-pro only when provider is our alias
		// keep full id for provider/model style real models? Real group ids are plain model ids.
		// If user passes provider/model and group is just model, use rest when provider is aigateway
		pref := strings.ToLower(modelID[:i])
		if pref == "aigateway" {
			modelID = modelID[i+1:]
		}
	}
	activeModelMu.Lock()
	activeModelCache = modelID
	activeModelMu.Unlock()
	if db, err := openDB(); err == nil {
		return metaSet(db, metaActiveGatewayModel, modelID)
	}
	return nil
}

// ActiveGatewayModelInfo is exposed to the UI.
type ActiveGatewayModelInfo struct {
	// VirtualModel is the stable id tools should request (aiSwitchModel).
	VirtualModel string `json:"virtualModel"`
	// ActiveModel is the real model group currently routed.
	ActiveModel string `json:"activeModel"`
	// Aliases accepted by the proxy for the virtual model.
	Aliases []string `json:"aliases"`
}

// GetActiveGatewayModel returns virtual alias + currently bound real model.
func (a *App) GetActiveGatewayModel() ActiveGatewayModelInfo {
	return ActiveGatewayModelInfo{
		VirtualModel: gatewayVirtualModel,
		ActiveModel:  resolveActiveModelID(),
		Aliases: []string{
			gatewayVirtualModel,
			"aigateway",
			"default",
		},
	}
}

// SetActiveGatewayModel binds the virtual model (aiSwitchModel) to a real model group.
// This is a hot switch: tools keep their config; next proxy requests use the new upstream.
func (a *App) SetActiveGatewayModel(modelID string) (ActiveGatewayModelInfo, error) {
	modelID = strings.TrimSpace(modelID)
	if modelID == "" {
		return a.GetActiveGatewayModel(), fmt.Errorf("模型不能为空")
	}
	// validate: model should be routable
	cands, err := resolveRoutesLegacy(modelID)
	if err != nil || len(cands) == 0 {
		// also allow pure group id present in SQL even if no live key yet
		if db, err2 := openDB(); err2 == nil {
			var n int
			_ = db.QueryRow(`SELECT COUNT(1) FROM model_groups WHERE id = ?`, modelID).Scan(&n)
			if n == 0 {
				// try SQL routes directly
				if c2, e2 := resolveRoutesFromSQL(db, modelID); e2 != nil || len(c2) == 0 {
					if err != nil {
						return a.GetActiveGatewayModel(), fmt.Errorf("无法设为默认: %w", err)
					}
					return a.GetActiveGatewayModel(), fmt.Errorf("未找到模型 %q 的可用路由", modelID)
				}
			}
		} else if err != nil {
			return a.GetActiveGatewayModel(), fmt.Errorf("无法设为默认: %w", err)
		}
	}
	if err := persistActiveGatewayModel(modelID); err != nil {
		return a.GetActiveGatewayModel(), err
	}
	// ensure proxy running so switch is live
	if a.proxy != nil {
		st := a.proxy.status()
		if !st.Running {
			_ = a.proxy.start()
		}
	}
	info := a.GetActiveGatewayModel()
	return info, nil
}
