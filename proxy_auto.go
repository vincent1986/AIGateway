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

// codexProviderID picks a stable toml table key for a vendor.
func codexProviderID(p Provider) string {
	if id := slugify(p.ID); id != "" && id != "custom" && !strings.HasPrefix(id, "p_") {
		// prefer semantic ids like ollama / deepseek
		n := strings.ToLower(p.Name + p.BaseURL)
		if strings.Contains(n, "ollama") {
			return "ollama"
		}
		if strings.Contains(n, "deepseek") {
			return "deepseek"
		}
		if strings.Contains(n, "openai") {
			return "openai"
		}
	}
	if id := deriveProviderID(p.BaseURL, p.Name); id != "" {
		return id
	}
	return slugify(p.Name)
}
