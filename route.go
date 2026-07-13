package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
)

// RouteCandidate is one upstream channel for a model group, ordered by priority.
type RouteCandidate struct {
	GroupID       string
	Provider      Provider
	UpstreamModel string // model id sent to the provider
	Priority      int
	Status        string
	Format        string // openai | passthrough
}

// resolveProviderForModel returns the first healthy route (compat wrapper).
func resolveProviderForModel(model string) (Provider, error) {
	cands, err := resolveRoutesForModel(model)
	if err != nil {
		return Provider{}, err
	}
	if len(cands) == 0 {
		return Provider{}, fmt.Errorf("未找到可用路由: %s", model)
	}
	return cands[0].Provider, nil
}

// resolveRoutesForModel returns ordered failover candidates for a client model name.
func resolveRoutesForModel(model string) ([]RouteCandidate, error) {
	model = normalizeClientModelID(model)
	if model == "" {
		return nil, fmt.Errorf("请求中缺少 model 字段")
	}
	// Claude Code appends [1m] for its extended-context mode. The suffix is a
	// client hint, not part of the provider model ID or AIGateway alias.
	model = strings.TrimSpace(strings.TrimSuffix(model, "[1m]"))
	if db, err := openDB(); err == nil {
		if cands, err2 := resolveRoutesFromSQL(db, model); err2 == nil && len(cands) > 0 {
			return cands, nil
		}
		if target := strings.TrimSpace(metaGet(db, proxyAliasKey(model))); target != "" && target != model {
			if cands, err2 := resolveRoutesFromSQL(db, target); err2 == nil && len(cands) > 0 {
				return cands, nil
			}
		}
	}
	return resolveRoutesLegacy(model)
}

func normalizeClientModelID(model string) string {
	model = strings.TrimSpace(model)
	return strings.TrimSpace(strings.TrimSuffix(model, "[1m]"))
}

func resolveRoutesFromSQL(db *sql.DB, model string) ([]RouteCandidate, error) {
	rows, err := db.Query(`
SELECT r.group_id, r.provider_id, r.provider_model_id, r.priority, r.status,
       p.name, p.base_url, p.api_key, p.color, p.use_proxy, p.format_standard
FROM model_group_routes r
JOIN providers p ON p.id = r.provider_id
WHERE r.group_id = ? AND r.enabled = 1 AND p.enabled = 1
  AND r.status NOT IN ('exhausted', 'disabled', 'circuit_open')
ORDER BY r.priority ASC, r.id ASC
`, model)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []RouteCandidate
	for rows.Next() {
		var c RouteCandidate
		var pid, fmtStd string
		var useProxy sql.NullInt64
		if err := rows.Scan(&c.GroupID, &pid, &c.UpstreamModel, &c.Priority, &c.Status,
			&c.Provider.Name, &c.Provider.BaseURL, &c.Provider.APIKey, &c.Provider.Color, &useProxy, &fmtStd); err != nil {
			return nil, err
		}
		c.Provider.ID = pid
		c.Provider.UseProxy = useProxyFromSQL(useProxy)
		c.Provider.FormatStandard = fmtStd
		c.Format = fmtStd
		if c.Format == "" {
			c.Format = "openai"
			c.Provider.FormatStandard = "openai"
		}
		if c.Provider.BaseURL == "" || !providerAPIKeyOK(c.Provider) {
			continue
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func resolveRoutesLegacy(model string) ([]RouteCandidate, error) {
	providers, err := loadProvidersFromDisk()
	if err != nil {
		return nil, err
	}
	if len(providers) == 0 {
		return nil, fmt.Errorf("未配置任何厂家，请先在「厂家管理」中添加")
	}

	var out []RouteCandidate
	var disabled []RouteCandidate
	prio := 10
	for _, p := range providers {
		for _, m := range p.Models {
			if m.ID != model {
				continue
			}
			if p.BaseURL == "" || !providerAPIKeyOK(p) {
				continue
			}
			c := RouteCandidate{
				GroupID: model, Provider: p, UpstreamModel: m.ID,
				Priority: prio, Status: "ok", Format: "openai",
			}
			prio += 10
			if m.Enabled {
				out = append(out, c)
			} else {
				c.Status = "disabled"
				disabled = append(disabled, c)
			}
		}
	}
	if len(out) > 0 {
		return out, nil
	}
	if len(disabled) > 0 {
		return disabled[:1], nil
	}

	if i := strings.IndexAny(model, "/:"); i > 0 {
		prefix := strings.ToLower(model[:i])
		rest := model[i+1:]
		for _, p := range providers {
			if strings.ToLower(slugify(p.Name)) == prefix || strings.ToLower(p.ID) == prefix {
				if p.BaseURL == "" || !providerAPIKeyOK(p) {
					return nil, fmt.Errorf("厂家 %s 未配置完整 API/Key", p.Name)
				}
				up := rest
				if up == "" {
					up = model
				}
				return []RouteCandidate{{
					GroupID: model, Provider: p, UpstreamModel: up, Priority: 10, Status: "ok", Format: "openai",
				}}, nil
			}
		}
	}

	if len(providers) == 1 {
		p := providers[0]
		if p.BaseURL == "" || !providerAPIKeyOK(p) {
			return nil, fmt.Errorf("厂家 %s 未配置完整 API/Key", p.Name)
		}
		return []RouteCandidate{{
			GroupID: model, Provider: p, UpstreamModel: model, Priority: 10, Status: "ok", Format: "openai",
		}}, nil
	}

	names := make([]string, 0, len(providers))
	for _, p := range providers {
		names = append(names, p.Name)
	}
	return nil, fmt.Errorf("未找到模型 %q 所属厂家（已配置: %s）。请先在厂家中获取并启用该模型", model, strings.Join(names, ", "))
}

func exhaustedErrorJSON(model string) map[string]any {
	return map[string]any{
		"error": map[string]any{
			"message": "AIGateway Error: All provider backups for this model group have been exhausted or rate-limited.",
			"type":    "insufficient_quota",
			"param":   "model_group",
			"code":    "model_group_all_exhausted",
			"model":   model,
		},
	}
}

func isFailoverStatus(code int) bool {
	switch code {
	case 401, 402, 403, 408, 429, 500, 502, 503, 504:
		return true
	default:
		return false
	}
}

func isFailoverBody(body []byte) bool {
	s := strings.ToLower(string(body))
	keys := []string{
		"rate limit", "rate_limit", "too many requests",
		"insufficient_quota", "quota", "balance", "余额", "额度", "限流",
		"exceeded", "overloaded", "capacity",
	}
	for _, k := range keys {
		if strings.Contains(s, k) {
			return true
		}
	}
	return false
}

// rewriteRequestModel sets body.model to upstream model id when different.
func rewriteRequestModel(body []byte, upstreamModel string) []byte {
	if len(body) == 0 || upstreamModel == "" {
		return body
	}
	var m map[string]any
	if json.Unmarshal(body, &m) != nil {
		return body
	}
	cur, _ := m["model"].(string)
	if cur == upstreamModel {
		return body
	}
	m["model"] = upstreamModel
	b, err := json.Marshal(m)
	if err != nil {
		return body
	}
	return b
}
