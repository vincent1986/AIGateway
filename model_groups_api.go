package main

import (
	"fmt"
	"strings"
)

// ModelGroupRouteView is one channel under a virtual model group (UI).
type ModelGroupRouteView struct {
	ID              string `json:"id"`
	ProviderID      string `json:"providerId"`
	ProviderName    string `json:"providerName"`
	ProviderModelID string `json:"providerModelId"`
	Priority        int    `json:"priority"`
	Enabled         bool   `json:"enabled"`
	Status          string `json:"status"`
	UsedTokens      int64  `json:"usedTokens"`
}

// ModelGroupView is a virtual model aggregation for the V2 model board.
type ModelGroupView struct {
	ID       string                `json:"id"`
	Name     string                `json:"name"`
	Enabled  bool                  `json:"enabled"`
	Strategy string                `json:"strategy"`
	Routes   []ModelGroupRouteView `json:"routes"`
}

// ListModelGroups returns virtual model groups with ordered routes (P0 API).
func (a *App) ListModelGroups() ([]ModelGroupView, error) {
	db, err := openDB()
	if err != nil {
		return nil, err
	}
	grows, err := db.Query(`SELECT id, name, enabled, strategy FROM model_groups ORDER BY name COLLATE NOCASE`)
	if err != nil {
		return nil, err
	}
	defer grows.Close()

	var groups []ModelGroupView
	idx := map[string]int{}
	for grows.Next() {
		var g ModelGroupView
		var en int
		if err := grows.Scan(&g.ID, &g.Name, &en, &g.Strategy); err != nil {
			return nil, err
		}
		g.Enabled = intToBool(en)
		g.Routes = []ModelGroupRouteView{}
		idx[g.ID] = len(groups)
		groups = append(groups, g)
	}

	rrows, err := db.Query(`
SELECT r.id, r.group_id, r.provider_id, p.name, r.provider_model_id, r.priority, r.enabled, r.status, r.used_tokens
FROM model_group_routes r
LEFT JOIN providers p ON p.id = r.provider_id
ORDER BY r.group_id, r.priority ASC`)
	if err != nil {
		return nil, err
	}
	defer rrows.Close()
	for rrows.Next() {
		var rv ModelGroupRouteView
		var gid string
		var en int
		if err := rrows.Scan(&rv.ID, &gid, &rv.ProviderID, &rv.ProviderName, &rv.ProviderModelID, &rv.Priority, &en, &rv.Status, &rv.UsedTokens); err != nil {
			return nil, err
		}
		rv.Enabled = intToBool(en)
		i, ok := idx[gid]
		if !ok {
			continue
		}
		groups[i].Routes = append(groups[i].Routes, rv)
	}
	if groups == nil {
		return []ModelGroupView{}, nil
	}
	return groups, nil
}

func appProxyModel(kind ToolKind) string {
	switch kind {
	case ToolCodex:
		return "aiSwitchModel-codex"
	case ToolClaude:
		return "aiSwitchModel-claude"
	case ToolOpenClaw:
		return "aiSwitchModel-openclaw"
	case ToolHarness:
		return "aiSwitchModel-harness"
	default:
		return "aiSwitchModel"
	}
}

func isAppProxyModel(model string) bool {
	m := strings.ToLower(strings.TrimSpace(model))
	return m == "aiswitchmodel" || strings.HasPrefix(m, "aiswitchmodel-")
}

func proxyAliasKey(alias string) string {
	return "proxy_alias:" + strings.TrimSpace(alias)
}

func loadProxyAlias(alias string) string {
	alias = strings.TrimSpace(alias)
	if alias == "" {
		return ""
	}
	if db, err := openDB(); err == nil {
		return metaGet(db, proxyAliasKey(alias))
	}
	return ""
}

// SetAppProxyModel switches an app's stable proxy model alias to a real model group.
// It intentionally does not modify the downstream app config file.
func (a *App) SetAppProxyModel(kind, model string) (ToolConfigStatus, error) {
	k := ToolKind(strings.ToLower(strings.TrimSpace(kind)))
	if !isKnownToolKind(k) {
		return ToolConfigStatus{}, fmt.Errorf("未知工具类型: %s", kind)
	}
	model = strings.TrimSpace(model)
	if model == "" {
		return ToolConfigStatus{}, fmt.Errorf("模型不能为空")
	}
	if isAppProxyModel(model) {
		return ToolConfigStatus{}, fmt.Errorf("代理入口模型不能作为切换目标: %s", model)
	}
	alias := appProxyModel(k)
	db, err := openDB()
	if err != nil {
		return ToolConfigStatus{}, err
	}
	var exists int
	if err := db.QueryRow(`
		SELECT 1
		FROM model_groups g
		JOIN model_group_routes r ON r.group_id = g.id
		JOIN providers p ON p.id = r.provider_id
		JOIN provider_models m ON m.provider_id = r.provider_id AND m.model_id = r.provider_model_id
		WHERE g.id = ? AND g.enabled = 1 AND r.enabled = 1
		  AND r.status NOT IN ('exhausted', 'disabled', 'circuit_open')
		  AND p.enabled = 1 AND m.enabled = 1
		LIMIT 1`, model).Scan(&exists); err != nil {
		return ToolConfigStatus{}, err
	}
	if exists != 1 {
		return ToolConfigStatus{}, fmt.Errorf("目标模型组不存在或没有可用路由: %s", model)
	}
	if err := metaSet(db, proxyAliasKey(alias), model); err != nil {
		return ToolConfigStatus{}, err
	}
	st := a.resolveTool(k)
	st.ProxyModel = alias
	st.ProxyTargetModel = model
	st.Message = fmt.Sprintf("代理模型已切换：%s → %s（应用配置未修改）", alias, model)
	return st, nil
}

// SetModelGroupRoutePriority updates priority for a route (lower = preferred).
func (a *App) SetModelGroupRoutePriority(routeID string, priority int) error {
	db, err := openDB()
	if err != nil {
		return err
	}
	_, err = db.Exec(`UPDATE model_group_routes SET priority = ? WHERE id = ?`, priority, routeID)
	return err
}

// SetModelGroupRouteEnabled enables/disables a route in the failover chain.
func (a *App) SetModelGroupRouteEnabled(routeID string, enabled bool) error {
	db, err := openDB()
	if err != nil {
		return err
	}
	status := "ok"
	if !enabled {
		status = "disabled"
	}
	_, err = db.Exec(`UPDATE model_group_routes SET enabled = ?, status = ? WHERE id = ?`, boolToInt(enabled), status, routeID)
	return err
}

// ReorderModelGroupRoutes sets priority to 10,20,30… by ordered route IDs (drag-and-drop).
func (a *App) ReorderModelGroupRoutes(groupID string, routeIDs []string) error {
	if strings.TrimSpace(groupID) == "" || len(routeIDs) == 0 {
		return fmt.Errorf("groupId and routeIds required")
	}
	db, err := openDB()
	if err != nil {
		return err
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for i, rid := range routeIDs {
		prio := (i + 1) * 10
		if _, err := tx.Exec(`UPDATE model_group_routes SET priority = ? WHERE id = ? AND group_id = ?`, prio, rid, groupID); err != nil {
			return err
		}
		// first enabled route → ok, others standby (unless disabled)
		st := "standby"
		if i == 0 {
			st = "ok"
		}
		_, _ = tx.Exec(`UPDATE model_group_routes SET status = ?
			WHERE id = ? AND group_id = ? AND enabled = 1 AND status != 'exhausted' AND status != 'circuit_open'`,
			st, rid, groupID)
	}
	return tx.Commit()
}
