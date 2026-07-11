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
