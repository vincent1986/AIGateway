package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// APIFormat: openai_chat | openai_responses | anthropic_messages | auto
// auto = detect from base URL; non-openai is converted to OpenAI-compatible when calling upstream.

const (
	APIFormatAuto              = "auto"
	APIFormatOpenAIChat        = "openai_chat"
	APIFormatOpenAIResponses   = "openai_responses"
	APIFormatAnthropicMessages = "anthropic_messages"
)

type ProviderModel struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Enabled   bool   `json:"enabled"`
	IsDefault bool   `json:"isDefault"`
	OwnedBy   string `json:"ownedBy,omitempty"`
}

type Provider struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	BaseURL   string          `json:"baseUrl"`
	APIKey    string          `json:"apiKey"`
	Color     string          `json:"color"`
	APIFormat string          `json:"apiFormat"` // openai_chat | openai_responses | anthropic_messages | auto
	Models    []ProviderModel `json:"models"`
}

type FetchModelItem struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	OwnedBy string `json:"ownedBy"`
}

type ConnectionTestResult struct {
	OK         bool     `json:"ok"`
	Message    string   `json:"message"`
	Endpoint   string   `json:"endpoint"`
	StatusCode int      `json:"statusCode"`
	LatencyMs  int64    `json:"latencyMs"`
	ModelCount int      `json:"modelCount"`
	Sample     []string `json:"sample"`
	Error      string   `json:"error,omitempty"`
	APIFormat  string   `json:"apiFormat,omitempty"`
}

type providersFile struct {
	Version   int        `json:"version"`
	UpdatedAt string     `json:"updatedAt"`
	Providers []Provider `json:"providers"`
}

func providersStorePath() string {
	return filepath.Join(managerRoot(), "providers.json")
}

func NormalizeAPIFormat(apiFormat string) string {
	switch strings.ToLower(strings.TrimSpace(apiFormat)) {
	case "", "auto":
		return APIFormatAuto
	case "openai", "openai_chat", "chat":
		return APIFormatOpenAIChat
	case "openai_responses", "responses":
		return APIFormatOpenAIResponses
	case "anthropic", "anthropic_messages", "messages":
		return APIFormatAnthropicMessages
	default:
		return APIFormatAuto
	}
}

// ResolveAPIFormat returns the concrete upstream protocol after auto-detection.
func ResolveAPIFormat(p Provider) string {
	f := NormalizeAPIFormat(p.APIFormat)
	if f != APIFormatAuto {
		return f
	}
	// auto
	u := strings.ToLower(p.BaseURL)
	if strings.Contains(u, "anthropic.com") && !strings.Contains(u, "compatible") {
		return APIFormatAnthropicMessages
	}
	return APIFormatOpenAIChat
}

func (a *App) ListProviders() ([]Provider, error) {
	list, err := loadProvidersFromDisk()
	if err != nil {
		return nil, err
	}
	if list == nil {
		return []Provider{}, nil
	}
	return list, nil
}

func (a *App) SaveProviders(list []Provider) error {
	if list == nil {
		list = []Provider{}
	}
	for i := range list {
		list[i].BaseURL = strings.TrimRight(strings.TrimSpace(list[i].BaseURL), "/")
		list[i].APIFormat = NormalizeAPIFormat(list[i].APIFormat)
		if list[i].Models == nil {
			list[i].Models = []ProviderModel{}
		}
	}
	return saveProvidersToDisk(list)
}

func (a *App) DeleteProvider(providerID string) ([]Provider, error) {
	providerID = strings.TrimSpace(providerID)
	if providerID == "" {
		return nil, fmt.Errorf("厂家 ID 不能为空")
	}
	list, err := loadProvidersFromDisk()
	if err != nil {
		return nil, err
	}
	next := make([]Provider, 0, len(list))
	found := false
	for _, p := range list {
		if p.ID == providerID {
			found = true
			continue
		}
		next = append(next, p)
	}
	if !found {
		return list, fmt.Errorf("未找到厂家 %q", providerID)
	}
	if err := saveProvidersToDisk(next); err != nil {
		return list, err
	}
	return next, nil
}

func (a *App) FetchProviderModels(baseURL, apiKey string) ([]FetchModelItem, error) {
	return fetchModelsOpenAI(baseURL, apiKey)
}

func (a *App) TestProviderConnection(baseURL, apiKey, apiFormat string) ConnectionTestResult {
	return probeProvider(baseURL, apiKey, apiFormat)
}

func probeProvider(baseURL, apiKey, apiFormat string) ConnectionTestResult {
	res := ConnectionTestResult{OK: false}
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	apiKey = strings.TrimSpace(apiKey)
	if baseURL == "" {
		res.Message, res.Error = "测试失败", "请先填写 API Base URL"
		return res
	}
	if apiKey == "" {
		res.Message, res.Error = "测试失败", "请先填写 API Key"
		return res
	}
	if _, err := url.ParseRequestURI(baseURL); err != nil {
		res.Message, res.Error = "测试失败", fmt.Sprintf("URL 无效: %v", err)
		return res
	}

	p := Provider{BaseURL: baseURL, APIKey: apiKey, APIFormat: apiFormat}
	fmtName := ResolveAPIFormat(p)
	res.APIFormat = fmtName

	if fmtName == APIFormatAnthropicMessages {
		items, code, lat, endpoint, err := fetchModelsAnthropicDetailed(baseURL, apiKey)
		res.Endpoint, res.StatusCode, res.LatencyMs = endpoint, code, lat
		if err != nil {
			res.Message, res.Error = "连接失败", err.Error()
			return res
		}
		res.OK = true
		res.ModelCount = len(items)
		res.Message = fmt.Sprintf("连接成功，发现 %d 个 Anthropic 模型", len(items))
		for i := 0; i < len(items) && i < 8; i++ {
			res.Sample = append(res.Sample, items[i].ID)
		}
		return res
	}

	items, code, lat, endpoint, err := fetchModelsOpenAIDetailed(baseURL, apiKey)
	res.Endpoint = endpoint
	res.StatusCode = code
	res.LatencyMs = lat
	if err != nil {
		res.Message, res.Error = "连接失败", err.Error()
		return res
	}
	res.OK = true
	res.ModelCount = len(items)
	res.Message = fmt.Sprintf("连接成功，发现 %d 个模型（标准 OpenAI）", len(items))
	n := len(items)
	if n > 8 {
		n = 8
	}
	for i := 0; i < n; i++ {
		res.Sample = append(res.Sample, items[i].ID)
	}
	return res
}

func fetchModelsOpenAI(baseURL, apiKey string) ([]FetchModelItem, error) {
	items, _, _, _, err := fetchModelsOpenAIDetailed(baseURL, apiKey)
	return items, err
}

func fetchModelsOpenAIDetailed(baseURL, apiKey string) ([]FetchModelItem, int, int64, string, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	endpoint := baseURL
	if !strings.HasSuffix(strings.ToLower(endpoint), "/models") {
		if strings.HasSuffix(strings.ToLower(endpoint), "/v1") {
			endpoint += "/models"
		} else {
			endpoint += "/v1/models"
		}
	}
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, 0, 0, endpoint, err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("api-key", apiKey)
	start := time.Now()
	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	lat := time.Since(start).Milliseconds()
	if err != nil {
		return nil, 0, lat, endpoint, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg := strings.TrimSpace(string(body))
		if len(msg) > 300 {
			msg = msg[:300] + "…"
		}
		return nil, resp.StatusCode, lat, endpoint, fmt.Errorf("HTTP %d: %s", resp.StatusCode, msg)
	}
	items, err := parseModelsResponse(body)
	if err != nil {
		return nil, resp.StatusCode, lat, endpoint, err
	}
	return items, resp.StatusCode, lat, endpoint, nil
}

func fetchModelsAnthropicDetailed(baseURL, apiKey string) ([]FetchModelItem, int, int64, string, error) {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	endpoint := base + "/v1/models"
	if strings.HasSuffix(strings.ToLower(base), "/v1") {
		endpoint = base + "/models"
	}
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, 0, 0, endpoint, err
	}
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	start := time.Now()
	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	lat := time.Since(start).Milliseconds()
	if err != nil {
		return nil, 0, lat, endpoint, err
	}
	defer resp.Body.Close()
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if readErr != nil {
		return nil, resp.StatusCode, lat, endpoint, readErr
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg := strings.TrimSpace(string(body))
		if len(msg) > 300 {
			msg = msg[:300] + "…"
		}
		return nil, resp.StatusCode, lat, endpoint, fmt.Errorf("HTTP %d: %s", resp.StatusCode, msg)
	}
	var payload struct {
		Data []struct {
			ID          string `json:"id"`
			DisplayName string `json:"display_name"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, resp.StatusCode, lat, endpoint, fmt.Errorf("无法解析 Anthropic 模型列表: %w", err)
	}
	items := make([]FetchModelItem, 0, len(payload.Data))
	for _, model := range payload.Data {
		if model.ID == "" {
			continue
		}
		name := model.DisplayName
		if name == "" {
			name = model.ID
		}
		items = append(items, FetchModelItem{ID: model.ID, Name: name, OwnedBy: "anthropic"})
	}
	return items, resp.StatusCode, lat, endpoint, nil
}

func parseModelsResponse(body []byte) ([]FetchModelItem, error) {
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
	return nil, fmt.Errorf("无法解析模型列表")
}

func loadProvidersFromDisk() ([]Provider, error) {
	b, err := os.ReadFile(providersStorePath())
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
			return nil, err
		}
		for i := range list {
			list[i].APIFormat = NormalizeAPIFormat(list[i].APIFormat)
		}
		return list, nil
	}
	if f.Providers == nil {
		return []Provider{}, nil
	}
	for i := range f.Providers {
		f.Providers[i].APIFormat = NormalizeAPIFormat(f.Providers[i].APIFormat)
	}
	return f.Providers, nil
}

func saveProvidersToDisk(list []Provider) error {
	if err := os.MkdirAll(managerRoot(), 0o755); err != nil {
		return err
	}
	f := providersFile{Version: 1, UpdatedAt: time.Now().Format(time.RFC3339), Providers: list}
	b, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomicMode(providersStorePath(), string(b), 0o600)
}

func resolveProviderForModel(model string) (Provider, error) {
	model = strings.TrimSpace(model)
	if model == "" {
		return Provider{}, fmt.Errorf("请求中缺少 model 字段")
	}
	providers, err := loadProvidersFromDisk()
	if err != nil {
		return Provider{}, err
	}
	if len(providers) == 0 {
		return Provider{}, fmt.Errorf("未配置任何厂家")
	}
	var fallback *Provider
	for i := range providers {
		p := &providers[i]
		for _, m := range p.Models {
			if m.ID != model {
				continue
			}
			if p.BaseURL == "" {
				return Provider{}, fmt.Errorf("厂家 %s 未配置 API", p.Name)
			}
			// allow empty key for local
			if m.Enabled {
				return *p, nil
			}
			if fallback == nil {
				cp := *p
				fallback = &cp
			}
		}
	}
	if fallback != nil {
		return *fallback, nil
	}
	if len(providers) == 1 && providers[0].BaseURL != "" {
		return providers[0], nil
	}
	return Provider{}, fmt.Errorf("未找到模型 %q 所属厂家", model)
}
