package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// UsageEvent is one completed API call recorded by the local proxy.
type UsageEvent struct {
	Time          string `json:"time"` // RFC3339
	Day           string `json:"day"`  // YYYY-MM-DD local
	Provider      string `json:"provider"`
	Model         string `json:"model"`
	Endpoint      string `json:"endpoint"` // chat/completions | responses
	InputTokens   int    `json:"inputTokens"`
	OutputTokens  int    `json:"outputTokens"`
	TotalTokens   int    `json:"totalTokens"`
	Status        int    `json:"status"`
}

// UsageTotals aggregates token counts.
type UsageTotals struct {
	Calls        int `json:"calls"`
	InputTokens  int `json:"inputTokens"`
	OutputTokens int `json:"outputTokens"`
	TotalTokens  int `json:"totalTokens"`
}

// UsageBucket is totals for a key (model / provider / day).
type UsageBucket struct {
	Key          string `json:"key"`
	Calls        int    `json:"calls"`
	InputTokens  int    `json:"inputTokens"`
	OutputTokens int    `json:"outputTokens"`
	TotalTokens  int    `json:"totalTokens"`
}

// UsageStats is exposed to the UI.
type UsageStats struct {
	Total   UsageTotals   `json:"total"`
	ByDay   []UsageBucket `json:"byDay"`
	ByModel []UsageBucket `json:"byModel"`
	ByProvider []UsageBucket `json:"byProvider"`
	Recent  []UsageEvent  `json:"recent"`
}

type usageFile struct {
	Version int          `json:"version"`
	Events  []UsageEvent `json:"events"`
}

var usageMu sync.Mutex

func usageStorePath() string {
	return filepath.Join(managerRoot(), "usage.json")
}

func loadUsageFile() usageFile {
	var f usageFile
	b, err := os.ReadFile(usageStorePath())
	if err != nil {
		return usageFile{Version: 1, Events: []UsageEvent{}}
	}
	if json.Unmarshal(b, &f) != nil || f.Events == nil {
		return usageFile{Version: 1, Events: []UsageEvent{}}
	}
	return f
}

func saveUsageFile(f usageFile) error {
	if err := os.MkdirAll(managerRoot(), 0o755); err != nil {
		return err
	}
	f.Version = 1
	// cap history to last 5000 events
	if len(f.Events) > 5000 {
		f.Events = f.Events[len(f.Events)-5000:]
	}
	b, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(usageStorePath(), b, 0o600)
}

// recordUsage appends one usage event (thread-safe).
func recordUsage(provider, model, endpoint string, status, inTok, outTok, total int) {
	if inTok == 0 && outTok == 0 && total == 0 {
		// still record call counts with zero tokens (errors may skip)
		if status < 200 || status >= 300 {
			return
		}
	}
	if total == 0 {
		total = inTok + outTok
	}
	now := time.Now()
	ev := UsageEvent{
		Time:         now.Format(time.RFC3339),
		Day:          now.Format("2006-01-02"),
		Provider:     strings.TrimSpace(provider),
		Model:        strings.TrimSpace(model),
		Endpoint:     strings.TrimSpace(endpoint),
		InputTokens:  inTok,
		OutputTokens: outTok,
		TotalTokens:  total,
		Status:       status,
	}
	usageMu.Lock()
	defer usageMu.Unlock()
	f := loadUsageFile()
	f.Events = append(f.Events, ev)
	_ = saveUsageFile(f)
}

// recordUsageFromPayload extracts chat or responses usage from a JSON body.
func recordUsageFromPayload(body []byte, provider, model, endpoint string, status int) {
	if len(body) == 0 || status < 200 || status >= 300 {
		return
	}
	var m map[string]any
	if json.Unmarshal(body, &m) != nil {
		return
	}
	in, out, total := extractTokensFromMap(m)
	// responses wrap: { response: { usage: ... } } or usage at top
	if in == 0 && out == 0 {
		if resp, ok := m["response"].(map[string]any); ok {
			in, out, total = extractTokensFromMap(resp)
		}
	}
	if model == "" {
		if v, ok := m["model"].(string); ok {
			model = v
		}
	}
	if in == 0 && out == 0 && total == 0 {
		return
	}
	recordUsage(provider, model, endpoint, status, in, out, total)
}

// recordUsageFromSSE scans chat/completions or responses SSE for usage objects.
func recordUsageFromSSE(buf []byte, provider, model, endpoint string, status int) {
	if len(buf) == 0 || status < 200 || status >= 300 {
		return
	}
	lines := strings.Split(string(buf), "\n")
	var lastUsage map[string]any
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" || data == "[DONE]" {
			continue
		}
		var m map[string]any
		if json.Unmarshal([]byte(data), &m) != nil {
			continue
		}
		// chat chunk usage
		if u, ok := m["usage"].(map[string]any); ok && u != nil {
			lastUsage = u
			if mid, ok := m["model"].(string); ok && mid != "" {
				model = mid
			}
		}
		// responses completed
		if typ, _ := m["type"].(string); typ == "response.completed" {
			if resp, ok := m["response"].(map[string]any); ok {
				if u, ok := resp["usage"].(map[string]any); ok {
					lastUsage = u
				}
				if mid, ok := resp["model"].(string); ok && mid != "" {
					model = mid
				}
			}
		}
	}
	if lastUsage == nil {
		return
	}
	in, out, total := tokensFromUsageMap(lastUsage)
	if in == 0 && out == 0 && total == 0 {
		return
	}
	recordUsage(provider, model, endpoint, status, in, out, total)
}

func extractTokensFromMap(m map[string]any) (in, out, total int) {
	if m == nil {
		return
	}
	if u, ok := m["usage"].(map[string]any); ok {
		return tokensFromUsageMap(u)
	}
	return tokensFromUsageMap(m)
}

func tokensFromUsageMap(u map[string]any) (in, out, total int) {
	if u == nil {
		return
	}
	// Responses API
	in = firstInt(u, "input_tokens", "prompt_tokens")
	out = firstInt(u, "output_tokens", "completion_tokens")
	total = firstInt(u, "total_tokens")
	if total == 0 {
		total = in + out
	}
	return
}

// GetUsageStats returns aggregated token usage for the UI.
func (a *App) GetUsageStats() UsageStats {
	usageMu.Lock()
	defer usageMu.Unlock()
	f := loadUsageFile()
	return aggregateUsage(f.Events)
}

// ProviderPackageStatus is quota vs usage for one vendor's active token package.
type ProviderPackageStatus struct {
	ProviderID   string  `json:"providerId"`
	ProviderName string  `json:"providerName"`
	PackageID    string  `json:"packageId"`
	PackageName  string  `json:"packageName"`
	TotalTokens  int64   `json:"totalTokens"`
	UsedTokens   int64   `json:"usedTokens"`   // offset + proxy-tracked
	ProxyTokens  int64   `json:"proxyTokens"`  // from usage log only
	Remaining    int64   `json:"remaining"`
	PercentUsed  float64 `json:"percentUsed"`
	ExpireAt     string  `json:"expireAt"`
	Expired      bool    `json:"expired"`
	HasPackage   bool    `json:"hasPackage"`
}

// GetProviderPackageStatuses returns package remaining for each provider.
func (a *App) GetProviderPackageStatuses() []ProviderPackageStatus {
	list, err := loadProvidersFromDisk()
	if err != nil {
		return nil
	}
	usageMu.Lock()
	f := loadUsageFile()
	usageMu.Unlock()
	// map provider name/id → total tokens used
	usedBy := map[string]int64{}
	for _, e := range f.Events {
		k := strings.TrimSpace(e.Provider)
		if k == "" {
			continue
		}
		usedBy[strings.ToLower(k)] += int64(e.TotalTokens)
	}

	out := make([]ProviderPackageStatus, 0, len(list))
	for _, p := range list {
		st := ProviderPackageStatus{
			ProviderID:   p.ID,
			ProviderName: p.Name,
		}
		// match usage by name (proxy records Name)
		proxyUsed := usedBy[strings.ToLower(strings.TrimSpace(p.Name))]
		if proxyUsed == 0 {
			proxyUsed = usedBy[strings.ToLower(strings.TrimSpace(p.ID))]
		}
		st.ProxyTokens = proxyUsed

		var active *TokenPackage
		for i := range p.TokenPackages {
			if p.TokenPackages[i].Active {
				active = &p.TokenPackages[i]
				break
			}
		}
		// fallback: first package
		if active == nil && len(p.TokenPackages) > 0 {
			active = &p.TokenPackages[0]
		}
		if active == nil {
			out = append(out, st)
			continue
		}
		st.HasPackage = true
		st.PackageID = active.ID
		st.PackageName = active.Name
		st.TotalTokens = active.TotalTokens
		st.ExpireAt = active.ExpireAt
		if active.ExpireAt != "" {
			if t, err := time.Parse("2006-01-02", active.ExpireAt); err == nil {
				// expire end of day
				st.Expired = time.Now().After(t.Add(24*time.Hour - time.Second))
			}
		}
		st.UsedTokens = active.UsedOffset + proxyUsed
		st.Remaining = st.TotalTokens - st.UsedTokens
		if st.Remaining < 0 {
			st.Remaining = 0
		}
		if st.TotalTokens > 0 {
			st.PercentUsed = float64(st.UsedTokens) / float64(st.TotalTokens) * 100
			if st.PercentUsed > 100 {
				st.PercentUsed = 100
			}
		}
		out = append(out, st)
	}
	return out
}

// ClearUsageStats wipes recorded usage.
func (a *App) ClearUsageStats() (UsageStats, error) {
	usageMu.Lock()
	defer usageMu.Unlock()
	if err := saveUsageFile(usageFile{Version: 1, Events: []UsageEvent{}}); err != nil {
		return UsageStats{}, err
	}
	return aggregateUsage(nil), nil
}

func aggregateUsage(events []UsageEvent) UsageStats {
	st := UsageStats{
		ByDay:      []UsageBucket{},
		ByModel:    []UsageBucket{},
		ByProvider: []UsageBucket{},
		Recent:     []UsageEvent{},
	}
	if len(events) == 0 {
		return st
	}
	dayM := map[string]*UsageBucket{}
	modelM := map[string]*UsageBucket{}
	provM := map[string]*UsageBucket{}

	add := func(m map[string]*UsageBucket, key string, e UsageEvent) {
		if key == "" {
			key = "unknown"
		}
		b, ok := m[key]
		if !ok {
			b = &UsageBucket{Key: key}
			m[key] = b
		}
		b.Calls++
		b.InputTokens += e.InputTokens
		b.OutputTokens += e.OutputTokens
		b.TotalTokens += e.TotalTokens
	}

	for _, e := range events {
		st.Total.Calls++
		st.Total.InputTokens += e.InputTokens
		st.Total.OutputTokens += e.OutputTokens
		st.Total.TotalTokens += e.TotalTokens
		add(dayM, e.Day, e)
		add(modelM, e.Model, e)
		add(provM, e.Provider, e)
	}

	st.ByDay = sortBuckets(dayM, true)       // day ascending
	st.ByModel = sortBuckets(modelM, false) // total desc
	st.ByProvider = sortBuckets(provM, false)

	// recent last 50
	n := len(events)
	start := 0
	if n > 50 {
		start = n - 50
	}
	recent := make([]UsageEvent, n-start)
	copy(recent, events[start:])
	// reverse newest first
	for i, j := 0, len(recent)-1; i < j; i, j = i+1, j-1 {
		recent[i], recent[j] = recent[j], recent[i]
	}
	st.Recent = recent
	return st
}

func sortBuckets(m map[string]*UsageBucket, byKeyAsc bool) []UsageBucket {
	out := make([]UsageBucket, 0, len(m))
	for _, b := range m {
		out = append(out, *b)
	}
	sort.Slice(out, func(i, j int) bool {
		if byKeyAsc {
			return out[i].Key < out[j].Key
		}
		if out[i].TotalTokens != out[j].TotalTokens {
			return out[i].TotalTokens > out[j].TotalTokens
		}
		return out[i].Key < out[j].Key
	})
	// keep last 30 days only when by day
	if byKeyAsc && len(out) > 30 {
		out = out[len(out)-30:]
	}
	return out
}

