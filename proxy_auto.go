package main

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

// EnsureProxyRouting starts the local proxy when any vendor wants proxy,
// then rewrites Codex config.toml so:
//   - useProxy vendors → local proxy base_url
//   - direct vendors   → real base_url from providers.json
//   - deprecated wire_api lines are removed
//
// Called automatically when saving providers or applying models (no manual
// "Codex 地址改为本地代理" step required).
func (a *App) EnsureProxyRouting() (ProxyStatus, error) {
	list, err := loadProvidersFromDisk()
	if err != nil {
		return ProxyStatus{}, err
	}
	wantAny := false
	for _, p := range list {
		if providerWantsProxy(p) {
			wantAny = true
			break
		}
	}
	if a.proxy == nil {
		a.proxy = newProxyServer()
	}
	st := a.proxy.status()
	if wantAny && !st.Running {
		if err := a.proxy.start(); err != nil {
			return a.proxy.status(), fmt.Errorf("自动启动代理失败: %w", err)
		}
		st = a.proxy.status()
	}
	// Sync Codex even if proxy already running / only direct vendors
	if msg, err := a.syncCodexProviderBases(list); err != nil {
		// don't fail hard — provider save already succeeded
		a.proxy.logf("同步 Codex base_url: %v", err)
		st = a.proxy.status()
		st.LastError = err.Error()
		return st, nil
	} else if msg != "" {
		a.proxy.logf("%s", msg)
	}
	return a.proxy.status(), nil
}

// syncCodexProviderBases updates ~/.codex/config.toml model_providers for each app vendor.
func (a *App) syncCodexProviderBases(list []Provider) (string, error) {
	st := a.resolveTool(ToolCodex)
	path := st.Path
	if path == "" {
		// try preferred path even if file not yet discovered
		path = preferredCodexConfigPath()
	}
	if path == "" {
		return "", nil
	}
	var content string
	var raw []byte
	if fileExists(path) {
		b, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		raw = b
		content = string(b)
	} else {
		// no codex config yet — nothing to sync
		return "", nil
	}

	proxyRunning := a.proxy != nil && a.proxy.status().Running
	localBase := ""
	if proxyRunning {
		localBase = a.proxy.baseURL()
		_ = rememberOriginalBases(content, localBase)
	}

	content = removeTomlProviderBlock(content, "codex_proxy")
	// wire_api is deprecated by Codex — remove from all provider blocks
	content = stripAllWireAPI(content)
	// rename reserved built-in provider table keys if still present
	content = sanitizeReservedCodexProviderTables(content)
	nProxy, nDirect := 0, 0

	for _, p := range list {
		id := codexProviderID(p)
		if id == "" || id == "codex_proxy" {
			continue
		}
		realBase := strings.TrimRight(strings.TrimSpace(p.BaseURL), "/")
		apiKey := strings.TrimSpace(p.APIKey)
		name := strings.TrimSpace(p.Name)
		if name == "" {
			name = id
		}

		// ensure provider block exists with env_key (no wire_api)
		content = upsertCodexModelProvider(content, id, name, realBase, apiKey)

		if providerWantsProxy(p) && proxyRunning && localBase != "" {
			content = setProviderField(content, id, "base_url", localBase)
			nProxy++
		} else {
			if realBase != "" {
				content = setProviderField(content, id, "base_url", realBase)
			}
			nDirect++
		}
	}

	content = stripAllWireAPI(content)
	content = preserveLineEndings(string(raw), content)
	if err := writeFileAtomic(path, content); err != nil {
		return "", err
	}
	return fmt.Sprintf("已同步 Codex：%d 个走代理，%d 个直连", nProxy, nDirect), nil
}

// stripAllWireAPI removes deprecated wire_api lines from config.toml.
func stripAllWireAPI(content string) string {
	// full-line removals (chat / responses / any value)
	re := regexp.MustCompile(`(?m)^\s*wire_api\s*=\s*.*\n?`)
	return re.ReplaceAllString(content, "")
}

// sanitizeReservedCodexProviderTables renames any reserved [model_providers.x]
// blocks still present in a config.toml string (migration / self-heal).
func sanitizeReservedCodexProviderTables(content string) string {
	for reserved, alt := range codexReservedProviderIDs {
		old := "[model_providers." + reserved + "]"
		neu := "[model_providers." + alt + "]"
		if strings.Contains(content, old) {
			content = strings.ReplaceAll(content, old, neu)
		}
		// also top-level model_provider = "reserved"
		content = strings.ReplaceAll(content, `model_provider = "`+reserved+`"`, `model_provider = "`+alt+`"`)
		content = strings.ReplaceAll(content, `provider = "`+reserved+`"`, `provider = "`+alt+`"`)
	}
	return content
}

// codexReservedProviderIDs cannot be used as [model_providers.<id>] keys —
// Codex treats them as built-ins and rejects overrides.
var codexReservedProviderIDs = map[string]string{
	"openai":    "openai-custom",
	"ollama":    "ollama-local",
	"anthropic": "anthropic-custom",
}

// sanitizeCodexProviderID renames reserved built-in provider table keys.
func sanitizeCodexProviderID(id string) string {
	id = strings.TrimSpace(strings.ToLower(id))
	if id == "" {
		return "custom"
	}
	// allow already-renamed forms
	if alt, ok := codexReservedProviderIDs[id]; ok {
		return alt
	}
	return id
}

// codexProviderID picks a stable toml table key for a vendor.
func codexProviderID(p Provider) string {
	var id string
	if sid := slugify(p.ID); sid != "" && sid != "custom" && !strings.HasPrefix(sid, "p_") {
		// prefer semantic ids (sanitized — never use reserved built-ins)
		n := strings.ToLower(p.Name + p.BaseURL)
		if strings.Contains(n, "ollama") {
			id = "ollama-local"
		} else if strings.Contains(n, "deepseek") {
			id = "deepseek"
		} else if strings.Contains(n, "openai") {
			id = "openai-custom"
		} else {
			id = sid
		}
	}
	if id == "" {
		if d := deriveProviderID(p.BaseURL, p.Name); d != "" {
			id = d
		} else {
			id = slugify(p.Name)
		}
	}
	return sanitizeCodexProviderID(id)
}
