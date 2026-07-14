package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"
)

// ProviderModel is a model entry under a provider.
type ProviderModel struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Enabled   bool   `json:"enabled"`
	IsDefault bool   `json:"isDefault"`
	OwnedBy   string `json:"ownedBy,omitempty"`
}

// TokenPackage is a purchased / allocated token quota plan for a vendor.
type TokenPackage struct {
	ID          string `json:"id"`
	Name        string `json:"name"`        // e.g. "标准 100 万"
	TotalTokens int64  `json:"totalTokens"` // package quota (tokens)
	// UsedOffset: tokens already consumed before tracking started (manual)
	UsedOffset int64   `json:"usedOffset"`
	Price      float64 `json:"price"`    // optional cost
	Currency   string  `json:"currency"` // CNY / USD
	StartAt    string  `json:"startAt"`  // YYYY-MM-DD
	ExpireAt   string  `json:"expireAt"` // YYYY-MM-DD optional
	Note       string  `json:"note"`
	Active     bool    `json:"active"` // current plan for remaining calc
}

// Provider is a multi-vendor API configuration.
type Provider struct {
	ID      string          `json:"id"`
	Name    string          `json:"name"`
	BaseURL string          `json:"baseUrl"`
	APIKey  string          `json:"apiKey"`
	Color   string          `json:"color"`
	Models  []ProviderModel `json:"models"`
	// UseProxy: when true, applying models to Codex uses local proxy base_url
	// (if proxy is running). Local providers like Ollama default to false.
	// nil means auto (local → false, others → true).
	UseProxy *bool `json:"useProxy"`
	// FormatStandard: "openai" normalizes payloads; "passthrough" forwards body as-is.
	FormatStandard string `json:"formatStandard"`
	// TokenPackages: purchased token plans for this vendor
	TokenPackages []TokenPackage `json:"tokenPackages"`
}

// FetchModelItem is a lightweight model returned from remote API.
type FetchModelItem struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	OwnedBy string `json:"ownedBy"`
}

// ConnectionTestResult is the detailed result of "测试连接".
type ConnectionTestResult struct {
	OK         bool     `json:"ok"`
	Message    string   `json:"message"`
	Endpoint   string   `json:"endpoint"`
	StatusCode int      `json:"statusCode"`
	LatencyMs  int64    `json:"latencyMs"`
	ModelCount int      `json:"modelCount"`
	Sample     []string `json:"sample"` // first few model ids
	Error      string   `json:"error,omitempty"`
}

type providersFile struct {
	Version   int        `json:"version"`
	UpdatedAt string     `json:"updatedAt"`
	Providers []Provider `json:"providers"`
}

func providersStorePath() string {
	return filepath.Join(managerRoot(), "providers.json")
}

// ListProviders returns all saved providers (seeds Ollama on first empty store).
func (a *App) ListProviders() ([]Provider, error) {
	list, err := loadProvidersFromDisk()
	if err != nil {
		return nil, err
	}
	list, changed := ensureInitialProviders(list)
	if changed {
		if err := saveProvidersToDisk(list); err != nil {
			return list, nil // still return seeded list
		}
	}
	if list == nil {
		return []Provider{}, nil
	}
	return list, nil
}

// defaultOllamaProvider is the built-in local OpenAI-compatible Ollama endpoint.
func defaultOllamaProvider() Provider {
	useProxy := false
	return Provider{
		ID:       "ollama",
		Name:     "Ollama",
		BaseURL:  "http://127.0.0.1:11434/v1",
		APIKey:   "", // Ollama ignores key; keep it empty for local usage
		Color:    "#c4c4c4",
		UseProxy: &useProxy,
		Models:   []ProviderModel{},
	}
}

// ensureInitialProviders seeds Ollama only on first run. Once the user has
// saved/deleted providers, an empty list is a valid user choice.
func ensureInitialProviders(list []Provider) ([]Provider, bool) {
	if list == nil {
		list = []Provider{}
	}
	if len(list) == 0 {
		if providersInitialized() {
			return list, false
		}
		list = append(list, defaultOllamaProvider())
		return list, true
	}
	return list, false
}

// providerWantsProxy reports whether Codex should point this vendor at local proxy.
func providerWantsProxy(p Provider) bool {
	if p.UseProxy != nil {
		return *p.UseProxy
	}
	// auto: local/Ollama → direct; cloud → proxy
	return !isLocalOrNoAuthProvider(p)
}

func isLocalOrNoAuthProvider(p Provider) bool {
	u := strings.ToLower(p.BaseURL + " " + p.Name + " " + p.ID)
	if strings.Contains(u, "ollama") || strings.Contains(u, ":11434") {
		return true
	}
	if strings.Contains(u, "127.0.0.1") || strings.Contains(u, "localhost") {
		return true
	}
	return false
}

func providerAPIKeyOK(p Provider) bool {
	if strings.TrimSpace(p.APIKey) != "" {
		return true
	}
	// Ollama / local OpenAI-compatible servers often need no key
	return isLocalOrNoAuthProvider(p)
}

// providerIDWantsProxy looks up saved providers by id/name/url and reports
// whether Codex should write local proxy as base_url for this vendor.
func providerIDWantsProxy(providerID, displayName, baseURL string) bool {
	list, err := loadProvidersFromDisk()
	if err != nil {
		// fallback: local → no proxy
		return !isLocalOrNoAuthProvider(Provider{Name: displayName, BaseURL: baseURL, ID: providerID})
	}
	pid := strings.ToLower(strings.TrimSpace(providerID))
	dname := strings.ToLower(strings.TrimSpace(displayName))
	burl := strings.ToLower(strings.TrimSpace(baseURL))
	for _, p := range list {
		if strings.ToLower(p.ID) == pid ||
			slugify(p.Name) == pid ||
			strings.ToLower(slugify(p.ID)) == pid ||
			(dname != "" && strings.ToLower(p.Name) == dname) ||
			(burl != "" && strings.Contains(strings.ToLower(p.BaseURL), burl)) {
			return providerWantsProxy(p)
		}
		// match by model ownership later not needed
		if pid != "" && (strings.Contains(strings.ToLower(p.Name), pid) || strings.Contains(strings.ToLower(p.BaseURL), pid)) {
			return providerWantsProxy(p)
		}
	}
	return !isLocalOrNoAuthProvider(Provider{Name: displayName, BaseURL: baseURL, ID: providerID})
}

// SaveProviders replaces the full provider list.
// After save: auto-start proxy if any vendor wants it, and sync Codex base_urls.
func (a *App) SaveProviders(list []Provider) error {
	if list == nil {
		list = []Provider{}
	}
	// normalize
	for i := range list {
		list[i].ID = strings.TrimSpace(list[i].ID)
		list[i].Name = strings.TrimSpace(list[i].Name)
		list[i].BaseURL = strings.TrimRight(strings.TrimSpace(list[i].BaseURL), "/")
		if list[i].Models == nil {
			list[i].Models = []ProviderModel{}
		}
		if list[i].TokenPackages == nil {
			list[i].TokenPackages = []TokenPackage{}
		}
		// normalize packages + at most one active
		act := 0
		for j := range list[i].TokenPackages {
			tp := &list[i].TokenPackages[j]
			tp.ID = strings.TrimSpace(tp.ID)
			tp.Name = strings.TrimSpace(tp.Name)
			if tp.ID == "" {
				tp.ID = fmt.Sprintf("pkg_%d_%d", time.Now().UnixNano(), j)
			}
			if tp.Name == "" {
				tp.Name = "未命名套餐"
			}
			if tp.Currency == "" {
				tp.Currency = "CNY"
			}
			if tp.TotalTokens < 0 {
				tp.TotalTokens = 0
			}
			if tp.UsedOffset < 0 {
				tp.UsedOffset = 0
			}
			if tp.Active {
				act++
				if act > 1 {
					tp.Active = false
				}
			}
		}
		// ensure at most one default model
		defCount := 0
		for j := range list[i].Models {
			if list[i].Models[j].IsDefault {
				defCount++
				if defCount > 1 {
					list[i].Models[j].IsDefault = false
				}
			}
			if list[i].Models[j].Name == "" {
				list[i].Models[j].Name = list[i].Models[j].ID
			}
		}
	}
	if err := saveProvidersToDisk(list); err != nil {
		return err
	}
	markProvidersInitialized()
	// Auto: 厂家选「走代理」→ 启动代理并写入 Codex base_url
	_, _ = a.EnsureProxyRouting()
	return nil
}

// UpsertProvider creates or updates one provider and returns the full list.
func (a *App) UpsertProvider(p Provider) ([]Provider, error) {
	list, err := loadProvidersFromDisk()
	if err != nil {
		return nil, err
	}
	p.ID = strings.TrimSpace(p.ID)
	p.Name = strings.TrimSpace(p.Name)
	p.BaseURL = strings.TrimRight(strings.TrimSpace(p.BaseURL), "/")
	if p.Name == "" {
		return nil, fmt.Errorf("厂家名称不能为空")
	}
	if p.BaseURL == "" {
		return nil, fmt.Errorf("API Base URL 不能为空")
	}
	if p.ID == "" {
		p.ID = fmt.Sprintf("p_%d", time.Now().UnixNano())
	}
	if p.Models == nil {
		p.Models = []ProviderModel{}
	}
	if p.TokenPackages == nil {
		p.TokenPackages = []TokenPackage{}
	}
	found := false
	for i := range list {
		if list[i].ID == p.ID {
			// preserve models if incoming empty and existing has some? No — trust payload
			list[i] = p
			found = true
			break
		}
	}
	if !found {
		list = append(list, p)
	}
	if err := saveProvidersToDisk(list); err != nil {
		return nil, err
	}
	markProvidersInitialized()
	_, _ = a.EnsureProxyRouting()
	return list, nil
}

// DeleteProvider removes a provider by id.
func (a *App) DeleteProvider(id string) ([]Provider, error) {
	id = strings.TrimSpace(id)
	list, err := loadProvidersFromDisk()
	if err != nil {
		return nil, err
	}
	next := make([]Provider, 0, len(list))
	removed := false
	for _, p := range list {
		if p.ID != id {
			next = append(next, p)
		} else {
			removed = true
		}
	}
	if !removed {
		return list, fmt.Errorf("厂家不存在: %s", id)
	}
	if err := saveProvidersToDisk(next); err != nil {
		return nil, err
	}
	markProvidersInitialized()
	_, _ = a.EnsureProxyRouting()
	return next, nil
}

// FetchProviderModels calls OpenAI-compatible GET {baseUrl}/models.
func (a *App) FetchProviderModels(baseURL, apiKey string) ([]FetchModelItem, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	apiKey = strings.TrimSpace(apiKey)
	if baseURL == "" {
		return nil, fmt.Errorf("请先填写 API Base URL")
	}
	// Allow empty key for Ollama / localhost
	if apiKey == "" && !isLocalOrNoAuthProvider(Provider{BaseURL: baseURL}) {
		return nil, fmt.Errorf("请先填写 API Key")
	}
	if _, err := url.ParseRequestURI(baseURL); err != nil {
		return nil, fmt.Errorf("API Base URL 无效: %v", err)
	}

	endpoint := modelsEndpoint(baseURL)
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
		req.Header.Set("api-key", apiKey)
	}
	req.Header.Set("Content-Type", "application/json")

	// Bypass Windows system proxy for localhost (Ollama); use system proxy for cloud APIs
	client := newHTTPClient(30 * time.Second)
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg := strings.TrimSpace(string(body))
		if len(msg) > 300 {
			msg = msg[:300] + "…"
		}
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, msg)
	}

	items, err := parseModelsResponse(body)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("接口未返回模型列表")
	}
	return items, nil
}

// TestProviderConnection pings /models and returns a detailed result for UI display.
func (a *App) TestProviderConnection(baseURL, apiKey string) ConnectionTestResult {
	return a.probeProvider(baseURL, apiKey)
}

func (a *App) probeProvider(baseURL, apiKey string) ConnectionTestResult {
	res := ConnectionTestResult{OK: false}
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	apiKey = strings.TrimSpace(apiKey)

	if baseURL == "" {
		res.Message = "测试失败"
		res.Error = "请先填写 API Base URL"
		return res
	}
	if apiKey == "" && !isLocalOrNoAuthProvider(Provider{BaseURL: baseURL}) {
		res.Message = "测试失败"
		res.Error = "请先填写 API Key"
		return res
	}
	if _, err := url.ParseRequestURI(baseURL); err != nil {
		res.Message = "测试失败"
		res.Error = fmt.Sprintf("API Base URL 无效: %v", err)
		return res
	}

	endpoint := modelsEndpoint(baseURL)
	res.Endpoint = endpoint

	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		res.Message = "测试失败"
		res.Error = err.Error()
		return res
	}
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
		req.Header.Set("api-key", apiKey)
	}
	req.Header.Set("Content-Type", "application/json")

	client := newHTTPClient(30 * time.Second)
	start := time.Now()
	resp, err := client.Do(req)
	res.LatencyMs = time.Since(start).Milliseconds()
	if err != nil {
		res.Message = "连接失败"
		res.Error = fmt.Sprintf("请求失败: %v", err)
		return res
	}
	defer resp.Body.Close()
	res.StatusCode = resp.StatusCode

	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		res.Message = "连接失败"
		res.Error = fmt.Sprintf("读取响应失败: %v", err)
		return res
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg := strings.TrimSpace(string(body))
		if len(msg) > 400 {
			msg = msg[:400] + "…"
		}
		res.Message = "接口返回错误"
		res.Error = fmt.Sprintf("HTTP %d: %s", resp.StatusCode, msg)
		return res
	}

	items, err := parseModelsResponse(body)
	if err != nil {
		res.Message = "响应无法解析"
		res.Error = err.Error()
		return res
	}
	if len(items) == 0 {
		res.Message = "连接成功但无模型"
		res.Error = "接口未返回模型列表"
		res.OK = false
		return res
	}

	res.OK = true
	res.ModelCount = len(items)
	res.Message = fmt.Sprintf("连接成功，发现 %d 个模型", len(items))
	// sample up to 8 ids
	n := len(items)
	if n > 8 {
		n = 8
	}
	res.Sample = make([]string, 0, n)
	for i := 0; i < n; i++ {
		res.Sample = append(res.Sample, items[i].ID)
	}
	return res
}

func modelsEndpoint(baseURL string) string {
	base := strings.TrimRight(baseURL, "/")
	// already ends with /models
	if strings.HasSuffix(strings.ToLower(base), "/models") {
		return base
	}
	return base + "/models"
}

func parseModelsResponse(body []byte) ([]FetchModelItem, error) {
	// OpenAI: { "data": [ { "id", "owned_by" } ] }
	var openai struct {
		Data []struct {
			ID      string `json:"id"`
			OwnedBy string `json:"owned_by"`
			Name    string `json:"name"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &openai); err == nil && len(openai.Data) > 0 {
		out := make([]FetchModelItem, 0, len(openai.Data))
		for _, d := range openai.Data {
			if strings.TrimSpace(d.ID) == "" {
				continue
			}
			name := d.Name
			if name == "" {
				name = d.ID
			}
			out = append(out, FetchModelItem{ID: d.ID, Name: name, OwnedBy: d.OwnedBy})
		}
		if len(out) > 0 {
			return out, nil
		}
	}

	// Some APIs: { "models": [ "a", "b" ] } or objects
	var alt struct {
		Models []json.RawMessage `json:"models"`
	}
	if err := json.Unmarshal(body, &alt); err == nil && len(alt.Models) > 0 {
		out := make([]FetchModelItem, 0, len(alt.Models))
		for _, raw := range alt.Models {
			var s string
			if json.Unmarshal(raw, &s) == nil && s != "" {
				out = append(out, FetchModelItem{ID: s, Name: s})
				continue
			}
			var obj struct {
				ID      string `json:"id"`
				Name    string `json:"name"`
				OwnedBy string `json:"owned_by"`
			}
			if json.Unmarshal(raw, &obj) == nil && obj.ID != "" {
				name := obj.Name
				if name == "" {
					name = obj.ID
				}
				out = append(out, FetchModelItem{ID: obj.ID, Name: name, OwnedBy: obj.OwnedBy})
			}
		}
		if len(out) > 0 {
			return out, nil
		}
	}

	// Bare array
	var arr []struct {
		ID      string `json:"id"`
		Name    string `json:"name"`
		OwnedBy string `json:"owned_by"`
	}
	if err := json.Unmarshal(body, &arr); err == nil && len(arr) > 0 {
		out := make([]FetchModelItem, 0, len(arr))
		for _, d := range arr {
			if d.ID == "" {
				continue
			}
			name := d.Name
			if name == "" {
				name = d.ID
			}
			out = append(out, FetchModelItem{ID: d.ID, Name: name, OwnedBy: d.OwnedBy})
		}
		if len(out) > 0 {
			return out, nil
		}
	}

	return nil, fmt.Errorf("无法解析模型列表响应")
}

// loadProvidersFromDisk / saveProvidersToDisk live in db_providers.go (SQLite + JSON mirror).
