package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// migrateJSONIfNeeded imports providers.json + usage.json once into SQLite.
func migrateJSONIfNeeded(db *sql.DB) error {
	if metaGet(db, "json_migrated") == "1" {
		return nil
	}
	// If DB already has providers (fresh write path), mark done.
	var n int
	_ = db.QueryRow(`SELECT COUNT(1) FROM providers`).Scan(&n)
	if n > 0 {
		_ = metaSet(db, "json_migrated", "1")
		return nil
	}

	list, err := loadProvidersJSONFile()
	if err != nil {
		return err
	}
	if len(list) > 0 {
		if err := replaceProvidersInDB(db, list); err != nil {
			return fmt.Errorf("migrate providers: %w", err)
		}
	}

	// usage.json
	if b, err := os.ReadFile(usageStorePath()); err == nil {
		var f usageFile
		if json.Unmarshal(b, &f) == nil {
			for _, e := range f.Events {
				_, _ = db.Exec(`INSERT INTO usage_events(time, provider_id, provider_name, model, group_id, endpoint, status, input_tokens, output_tokens, total_tokens)
					VALUES(?,?,?,?,?,?,?,?,?,?)`,
					e.Time, "", e.Provider, e.Model, e.Model, e.Endpoint, e.Status, e.InputTokens, e.OutputTokens, e.TotalTokens)
			}
		}
	}

	_ = metaSet(db, "json_migrated", "1")
	_ = metaSet(db, "json_migrated_at", time.Now().UTC().Format(time.RFC3339))
	return nil
}

// loadProvidersJSONFile reads legacy providers.json without touching SQLite.
func loadProvidersJSONFile() ([]Provider, error) {
	path := providersStorePath()
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []Provider{}, nil
		}
		return nil, err
	}
	var f providersFile
	if err := json.Unmarshal(b, &f); err != nil {
		var list []Provider
		if err2 := json.Unmarshal(b, &list); err2 != nil {
			return nil, fmt.Errorf("解析 providers.json 失败: %w", err)
		}
		return list, nil
	}
	if f.Providers == nil {
		return []Provider{}, nil
	}
	return f.Providers, nil
}

// replaceProvidersInDB fully rewrites providers + models + packages and rebuilds model groups.
func replaceProvidersInDB(db *sql.DB, list []Provider) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec(`DELETE FROM model_group_routes`); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM model_groups`); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM token_packages`); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM provider_models`); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM providers`); err != nil {
		return err
	}

	now := time.Now().UTC().Format(time.RFC3339)
	for _, p := range list {
		if p.ID == "" {
			continue
		}
		_, err := tx.Exec(`INSERT INTO providers(id, name, base_url, api_key, color, use_proxy, format_standard, enabled, created_at, updated_at)
			VALUES(?,?,?,?,?,?,?,?,?,?)`,
			p.ID, p.Name, p.BaseURL, p.APIKey, p.Color, useProxyToSQL(p.UseProxy), "openai", 1, now, now)
		if err != nil {
			return err
		}
		for _, m := range p.Models {
			if m.ID == "" {
				continue
			}
			mid := p.ID + "::" + m.ID
			_, err := tx.Exec(`INSERT INTO provider_models(id, provider_id, model_id, name, enabled, is_default, owned_by)
				VALUES(?,?,?,?,?,?,?)`,
				mid, p.ID, m.ID, nullStr(m.Name, m.ID), boolToInt(m.Enabled), boolToInt(m.IsDefault), m.OwnedBy)
			if err != nil {
				return err
			}
		}
		for _, pkg := range p.TokenPackages {
			if pkg.ID == "" {
				pkg.ID = fmt.Sprintf("pkg_%s_%d", p.ID, time.Now().UnixNano())
			}
			_, err := tx.Exec(`INSERT INTO token_packages(id, provider_id, name, total_tokens, used_offset, price, currency, start_at, expire_at, note, active)
				VALUES(?,?,?,?,?,?,?,?,?,?,?)`,
				pkg.ID, p.ID, pkg.Name, pkg.TotalTokens, pkg.UsedOffset, pkg.Price, nullStr(pkg.Currency, "CNY"),
				pkg.StartAt, pkg.ExpireAt, pkg.Note, boolToInt(pkg.Active))
			if err != nil {
				return err
			}
		}
	}

	if err := rebuildModelGroupsTx(tx, list); err != nil {
		return err
	}
	return tx.Commit()
}

func nullStr(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

// rebuildModelGroupsTx aggregates same model_id across providers into groups with priority order.
func rebuildModelGroupsTx(tx *sql.Tx, list []Provider) error {
	// groupID -> routes in discovery order (first provider becomes priority 10, then 20...)
	type route struct {
		providerID string
		modelID    string
		enabled    bool
	}
	groups := map[string][]route{}
	order := []string{}

	for _, p := range list {
		for _, m := range p.Models {
			if m.ID == "" {
				continue
			}
			gid := m.ID
			if _, ok := groups[gid]; !ok {
				order = append(order, gid)
			}
			groups[gid] = append(groups[gid], route{providerID: p.ID, modelID: m.ID, enabled: m.Enabled})
		}
	}

	now := time.Now().UTC().Format(time.RFC3339)
	for _, gid := range order {
		routes := groups[gid]
		_, err := tx.Exec(`INSERT INTO model_groups(id, name, enabled, strategy, created_at, updated_at)
			VALUES(?,?,?,?,?,?)`, gid, gid, 1, "priority", now, now)
		if err != nil {
			return err
		}
		for i, r := range routes {
			priority := (i + 1) * 10
			status := "ok"
			if i > 0 {
				status = "standby"
			}
			if !r.enabled {
				status = "disabled"
			}
			rid := fmt.Sprintf("%s::%s::%s", gid, r.providerID, r.modelID)
			_, err := tx.Exec(`INSERT INTO model_group_routes(id, group_id, provider_id, provider_model_id, priority, enabled, status, used_tokens)
				VALUES(?,?,?,?,?,?,?,0)`,
				rid, gid, r.providerID, r.modelID, priority, boolToInt(r.enabled), status)
			if err != nil {
				return err
			}
		}
	}
	return nil
}
