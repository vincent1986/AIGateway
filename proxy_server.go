package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// ProxyConfig controls the local OpenAI-compatible gateway.
type ProxyConfig struct {
	Enabled   bool   `json:"enabled"`
	Host      string `json:"host"` // default 127.0.0.1
	Port      int    `json:"port"` // default 18080
	AutoStart bool   `json:"autoStart"`
	// Optional key clients must send; empty = accept any Bearer token
	ListenKey string `json:"listenKey"`
}

// ProxyStatus is exposed to the UI.
type ProxyStatus struct {
	Running   bool   `json:"running"`
	BaseURL   string `json:"baseUrl"`   // http://127.0.0.1:18080/v1
	Host      string `json:"host"`
	Port      int    `json:"port"`
	AutoStart bool   `json:"autoStart"`
	ListenKey string `json:"listenKey"`
	LastError string `json:"lastError"`
	// recent log lines (newest last)
	Logs []string `json:"logs"`
}

type proxyServer struct {
	mu     sync.Mutex
	cfg    ProxyConfig
	srv    *http.Server
	ln     net.Listener
	run    bool
	err    string
	logs   []string
	client *http.Client
}

func defaultProxyConfig() ProxyConfig {
	return ProxyConfig{
		Enabled:   false,
		Host:      "127.0.0.1",
		Port:      18080,
		AutoStart: false,
		ListenKey: "",
	}
}

func proxyConfigPath() string {
	return filepath.Join(managerRoot(), "proxy.json")
}

func loadProxyConfig() ProxyConfig {
	cfg := defaultProxyConfig()
	b, err := os.ReadFile(proxyConfigPath())
	if err != nil {
		return cfg
	}
	_ = json.Unmarshal(b, &cfg)
	if cfg.Host == "" {
		cfg.Host = "127.0.0.1"
	}
	if cfg.Port <= 0 || cfg.Port > 65535 {
		cfg.Port = 18080
	}
	return cfg
}

func saveProxyConfig(cfg ProxyConfig) error {
	if err := os.MkdirAll(managerRoot(), 0o755); err != nil {
		return err
	}
	if cfg.Host == "" {
		cfg.Host = "127.0.0.1"
	}
	if cfg.Port <= 0 {
		cfg.Port = 18080
	}
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(proxyConfigPath(), b, 0o644)
}

func newProxyServer() *proxyServer {
	return &proxyServer{
		cfg: loadProxyConfig(),
		// smart transport: system proxy for remote APIs, never proxy loopback
		// (critical on Windows with Clash / corporate HTTP_PROXY)
		client: newHTTPClient(0),
		logs:   make([]string, 0, 50),
	}
}

func (p *proxyServer) logf(format string, args ...any) {
	line := time.Now().Format("15:04:05") + " " + fmt.Sprintf(format, args...)
	p.mu.Lock()
	defer p.mu.Unlock()
	p.logs = append(p.logs, line)
	if len(p.logs) > 80 {
		p.logs = p.logs[len(p.logs)-80:]
	}
}

func (p *proxyServer) baseURL() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return formatBaseURL(p.cfg.Host, p.cfg.Port)
}

func (p *proxyServer) status() ProxyStatus {
	p.mu.Lock()
	defer p.mu.Unlock()
	logs := append([]string(nil), p.logs...)
	return ProxyStatus{
		Running:   p.run,
		BaseURL:   formatBaseURL(p.cfg.Host, p.cfg.Port),
		Host:      clientFacingHost(p.cfg.Host), // report client-usable host
		Port:      p.cfg.Port,
		AutoStart: p.cfg.AutoStart,
		ListenKey: p.cfg.ListenKey,
		LastError: p.err,
		Logs:      logs,
	}
}

func (p *proxyServer) getConfig() ProxyConfig {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.cfg
}

func (p *proxyServer) setConfig(cfg ProxyConfig) error {
	if cfg.Host == "" {
		cfg.Host = "127.0.0.1"
	}
	if cfg.Port <= 0 || cfg.Port > 65535 {
		return fmt.Errorf("端口无效: %d", cfg.Port)
	}
	p.mu.Lock()
	running := p.run
	p.cfg = cfg
	p.mu.Unlock()
	if err := saveProxyConfig(cfg); err != nil {
		return err
	}
	if running {
		// restart to apply bind address/port
		_ = p.stop()
		return p.start()
	}
	return nil
}

func (p *proxyServer) start() error {
	p.mu.Lock()
	if p.run {
		p.mu.Unlock()
		return nil
	}
	cfg := p.cfg
	p.mu.Unlock()

	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	mux := http.NewServeMux()
	mux.HandleFunc("/health", p.handleHealth)
	mux.HandleFunc("/v1/models", p.handleModels)
	mux.HandleFunc("/models", p.handleModels)
	mux.HandleFunc("/v1/chat/completions", p.handleChatCompletions)
	mux.HandleFunc("/chat/completions", p.handleChatCompletions)
	mux.HandleFunc("/v1/completions", p.handleCompletions)
	mux.HandleFunc("/completions", p.handleCompletions)
	mux.HandleFunc("/v1/embeddings", p.handleEmbeddings)
	mux.HandleFunc("/embeddings", p.handleEmbeddings)
	// Codex wire_api = "responses" uses this path
	mux.HandleFunc("/v1/responses", p.handleResponses)
	mux.HandleFunc("/responses", p.handleResponses)
	// catch-all OpenAI-style under /v1/*
	mux.HandleFunc("/v1/", p.handleOpenAIProxy)
	mux.HandleFunc("/", p.handleRoot)

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		p.mu.Lock()
		p.err = err.Error()
		p.mu.Unlock()
		return fmt.Errorf("监听 %s 失败: %w", addr, err)
	}

	srv := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 15 * time.Second,
	}

	p.mu.Lock()
	p.ln = ln
	p.srv = srv
	p.run = true
	p.err = ""
	p.cfg.Enabled = true
	p.mu.Unlock()
	_ = saveProxyConfig(p.getConfig())

	p.logf("代理已启动 %s", p.baseURL())

	go func() {
		err := srv.Serve(ln)
		msg := "代理已停止"
		errStr := ""
		if err != nil && err != http.ErrServerClosed {
			errStr = err.Error()
			msg = "代理异常退出: " + errStr
		}
		p.mu.Lock()
		p.run = false
		if errStr != "" {
			p.err = errStr
		}
		// log without re-entering logf (would deadlock on p.mu)
		line := time.Now().Format("15:04:05") + " " + msg
		p.logs = append(p.logs, line)
		if len(p.logs) > 80 {
			p.logs = p.logs[len(p.logs)-80:]
		}
		p.mu.Unlock()
	}()
	return nil
}

func (p *proxyServer) stop() error {
	p.mu.Lock()
	srv := p.srv
	ln := p.ln
	p.run = false
	cfg := p.cfg
	cfg.Enabled = false
	p.cfg = cfg
	p.mu.Unlock()
	_ = saveProxyConfig(cfg)

	if srv == nil && ln == nil {
		return nil
	}
	// Close listener first so Serve returns promptly
	if ln != nil {
		_ = ln.Close()
	}
	if srv != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		_ = srv.Shutdown(ctx)
		cancel()
		// force close if still hanging
		_ = srv.Close()
	}
	// Drop idle upstream connections so tests / process exit promptly
	if p.client != nil {
		p.client.CloseIdleConnections()
	}
	p.mu.Lock()
	p.srv = nil
	p.ln = nil
	p.mu.Unlock()
	return nil
}

func (p *proxyServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{"ok": true, "service": "codex-openai-proxy"})
}

func (p *proxyServer) handleRoot(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	writeJSON(w, 200, map[string]any{
		"service": "codex-openai-proxy",
		"base":    p.baseURL(),
		"docs":    "OpenAI-compatible: /v1/models, /v1/chat/completions",
	})
}

func (p *proxyServer) checkListenAuth(r *http.Request) error {
	p.mu.Lock()
	key := strings.TrimSpace(p.cfg.ListenKey)
	p.mu.Unlock()
	if key == "" {
		return nil
	}
	auth := r.Header.Get("Authorization")
	if strings.HasPrefix(strings.ToLower(auth), "bearer ") {
		tok := strings.TrimSpace(auth[7:])
		if tok == key {
			return nil
		}
	}
	if r.Header.Get("api-key") == key || r.Header.Get("x-api-key") == key {
		return nil
	}
	return fmt.Errorf("unauthorized")
}

func (p *proxyServer) handleModels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := p.checkListenAuth(r); err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	providers, err := loadProvidersFromDisk()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	type item struct {
		ID      string `json:"id"`
		Object  string `json:"object"`
		OwnedBy string `json:"owned_by"`
	}
	data := make([]item, 0, 32)
	seen := map[string]bool{}
	// Stable virtual model first — tools pin this id; proxy hot-switches upstream.
	active := resolveActiveModelID()
	data = append(data, item{
		ID:      gatewayVirtualModel,
		Object:  "model",
		OwnedBy: "aigateway",
	})
	seen[gatewayVirtualModel] = true
	// also list aigateway alias for OpenClaw-style provider/model pickers
	if !seen["aigateway"] {
		data = append(data, item{ID: "aigateway", Object: "model", OwnedBy: "aigateway"})
		seen["aigateway"] = true
	}
	_ = active // available for clients that inspect owned_by / future fields
	for _, prov := range providers {
		for _, m := range prov.Models {
			if !m.Enabled {
				continue
			}
			id := strings.TrimSpace(m.ID)
			if id == "" || seen[id] {
				continue
			}
			seen[id] = true
			owned := prov.Name
			if m.OwnedBy != "" {
				owned = m.OwnedBy
			}
			data = append(data, item{ID: id, Object: "model", OwnedBy: owned})
		}
	}
	writeJSON(w, 200, map[string]any{"object": "list", "data": data})
}

func (p *proxyServer) handleChatCompletions(w http.ResponseWriter, r *http.Request) {
	p.forwardOpenAI(w, r, "chat/completions")
}

func (p *proxyServer) handleCompletions(w http.ResponseWriter, r *http.Request) {
	p.forwardOpenAI(w, r, "completions")
}

func (p *proxyServer) handleEmbeddings(w http.ResponseWriter, r *http.Request) {
	p.forwardOpenAI(w, r, "embeddings")
}

func (p *proxyServer) handleOpenAIProxy(w http.ResponseWriter, r *http.Request) {
	// /v1/xxx → xxx
	path := strings.TrimPrefix(r.URL.Path, "/v1/")
	path = strings.Trim(path, "/")
	if path == "" {
		p.handleRoot(w, r)
		return
	}
	// explicit branch for responses (also registered, but catch-all may hit first on some Go versions)
	if path == "responses" || strings.HasPrefix(path, "responses/") {
		p.handleResponses(w, r)
		return
	}
	p.forwardOpenAI(w, r, path)
}

// forwardOpenAI accepts standard OpenAI request and forwards to the provider
// that owns the model. V2: tries priority-ordered routes with failover on 429/quota.
func (p *proxyServer) forwardOpenAI(w http.ResponseWriter, r *http.Request, openAIPath string) {
	if r.Method != http.MethodPost && r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := p.checkListenAuth(r); err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 32<<20))
	if err != nil {
		http.Error(w, "read body: "+err.Error(), 400)
		return
	}
	_ = r.Body.Close()

	model := extractModel(body, r)
	cands, routeErr := resolveRoutesForModel(model)
	if routeErr != nil || len(cands) == 0 {
		msg := "model not routed"
		if routeErr != nil {
			msg = routeErr.Error()
		}
		p.logf("路由失败 model=%q: %s", model, msg)
		writeJSON(w, 400, map[string]any{
			"error": map[string]any{
				"message": msg,
				"type":    "invalid_request_error",
				"code":    "model_not_routed",
			},
		})
		return
	}

	// Codex / newer OpenAI clients send role=developer; most vendors reject it.
	// Applied per-candidate when format is openai (not passthrough).
	baseBody := body
	stream := isStreamRequest(baseBody)

	var lastStatus int
	var lastBody []byte
	var lastErr error

	for i, cand := range cands {
		prov := cand.Provider
		reqBody := rewriteRequestModel(baseBody, cand.UpstreamModel)
		// openai format: normalize chat payloads; passthrough: raw body
		fmtStd := cand.Format
		if fmtStd == "" {
			fmtStd = prov.FormatStandard
		}
		if fmtStd != "passthrough" {
			if openAIPath == "chat/completions" || strings.HasPrefix(openAIPath, "chat/completions") {
				reqBody = normalizeUpstreamChatBody(reqBody)
				if isStreamRequest(reqBody) {
					reqBody = ensureStreamUsage(reqBody)
				}
			}
		}
		stream = isStreamRequest(reqBody)

		upstreamURL := joinOpenAIURL(prov.BaseURL, openAIPath)
		if r.URL.RawQuery != "" {
			if strings.Contains(upstreamURL, "?") {
				upstreamURL += "&" + r.URL.RawQuery
			} else {
				upstreamURL += "?" + r.URL.RawQuery
			}
		}

		p.logf("%s %s → %s client_model=%s up=%s try=%d/%d stream=%v",
			r.Method, openAIPath, prov.Name, model, cand.UpstreamModel, i+1, len(cands), stream)

		req, err := http.NewRequestWithContext(r.Context(), r.Method, upstreamURL, bytes.NewReader(reqBody))
		if err != nil {
			lastErr = err
			continue
		}
		req.Header.Set("Content-Type", "application/json")
		key := strings.TrimSpace(prov.APIKey)
		if key == "" && isLocalOrNoAuthProvider(prov) {
			key = "ollama"
		}
		if key != "" {
			req.Header.Set("Authorization", "Bearer "+key)
			req.Header.Set("api-key", key)
		}
		if v := r.Header.Get("Accept"); v != "" {
			req.Header.Set("Accept", v)
		}
		if stream {
			req.Header.Set("Accept", "text/event-stream")
		}

		client := p.client
		if !stream {
			client = &http.Client{Timeout: 120 * time.Second, Transport: p.client.Transport}
		}
		resp, err := client.Do(req)
		if err != nil {
			p.logf("上游错误 %s: %v", prov.Name, err)
			lastErr = err
			// network error → try next within ~300ms budget
			if i+1 < len(cands) {
				time.Sleep(50 * time.Millisecond)
			}
			continue
		}

		// Stream: once headers sent we cannot failover transparently without buffering.
		// For stream failures that return quickly with error status, failover before writing.
		if stream {
			if isFailoverStatus(resp.StatusCode) {
				b, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
				_ = resp.Body.Close()
				lastStatus = resp.StatusCode
				lastBody = b
				p.logf("熔断切换 status=%d provider=%s → next", resp.StatusCode, prov.Name)
				if i+1 < len(cands) {
					time.Sleep(50 * time.Millisecond)
					continue
				}
				break
			}
			// success stream path — peek first chunk for early error SSE before committing headers
			peek := make([]byte, 4096)
			n0, peekErr := resp.Body.Read(peek)
			peekData := peek[:max(0, n0)]
			if n0 > 0 && isFailoverBody(peekData) && i+1 < len(cands) {
				p.logf("流式首包熔断 provider=%s → next", prov.Name)
				_ = resp.Body.Close()
				lastStatus = resp.StatusCode
				lastBody = peekData
				time.Sleep(50 * time.Millisecond)
				continue
			}
			for k, vv := range resp.Header {
				lk := strings.ToLower(k)
				if lk == "connection" || lk == "keep-alive" || lk == "transfer-encoding" || lk == "proxy-authenticate" || lk == "proxy-authorization" || lk == "te" || lk == "trailers" || lk == "upgrade" {
					continue
				}
				for _, v := range vv {
					w.Header().Add(k, v)
				}
			}
			w.WriteHeader(resp.StatusCode)
			flusher, ok := w.(http.Flusher)
			var acc bytes.Buffer
			if n0 > 0 {
				_, _ = w.Write(peekData)
				_, _ = acc.Write(peekData)
				if ok {
					flusher.Flush()
				}
			}
			if peekErr != nil {
				_ = resp.Body.Close()
				recordUsageFromSSE(acc.Bytes(), prov.Name, model, openAIPath, resp.StatusCode)
				return
			}
			buf := make([]byte, 4096)
			for {
				n, readErr := resp.Body.Read(buf)
				if n > 0 {
					_, _ = w.Write(buf[:n])
					if acc.Len() < 2<<20 {
						_, _ = acc.Write(buf[:n])
					}
					if ok {
						flusher.Flush()
					}
				}
				if readErr != nil {
					break
				}
			}
			_ = resp.Body.Close()
			recordUsageFromSSE(acc.Bytes(), prov.Name, model, openAIPath, resp.StatusCode)
			return
		}

		// Non-stream
		upBody, _ := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
		_ = resp.Body.Close()
		lastStatus = resp.StatusCode
		lastBody = upBody

		if isFailoverStatus(resp.StatusCode) || (resp.StatusCode >= 400 && isFailoverBody(upBody)) {
			p.logf("熔断切换 status=%d provider=%s body_hint=%v", resp.StatusCode, prov.Name, isFailoverBody(upBody))
			if i+1 < len(cands) {
				time.Sleep(50 * time.Millisecond)
				continue
			}
			break
		}

		// success
		for k, vv := range resp.Header {
			lk := strings.ToLower(k)
			if lk == "connection" || lk == "keep-alive" || lk == "transfer-encoding" || lk == "proxy-authenticate" || lk == "proxy-authorization" || lk == "te" || lk == "trailers" || lk == "upgrade" {
				continue
			}
			for _, v := range vv {
				w.Header().Add(k, v)
			}
		}
		w.WriteHeader(resp.StatusCode)
		recordUsageFromPayload(upBody, prov.Name, model, openAIPath, resp.StatusCode)
		_, _ = w.Write(upBody)
		return
	}

	// all candidates failed
	p.logf("全部渠道耗尽 model=%q lastStatus=%d lastErr=%v", model, lastStatus, lastErr)
	if lastStatus > 0 && len(lastBody) > 0 && lastStatus < 500 {
		// return last upstream error if client-ish, else exhausted
		for _, h := range []string{"Content-Type"} {
			_ = h
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(lastStatus)
		_, _ = w.Write(lastBody)
		return
	}
	writeJSON(w, 429, exhaustedErrorJSON(model))
}

// ensureStreamUsage adds stream_options.include_usage for chat streams.
func ensureStreamUsage(body []byte) []byte {
	var m map[string]any
	if json.Unmarshal(body, &m) != nil {
		return body
	}
	m["stream"] = true
	so, _ := m["stream_options"].(map[string]any)
	if so == nil {
		so = map[string]any{}
	}
	so["include_usage"] = true
	m["stream_options"] = so
	b, err := json.Marshal(m)
	if err != nil {
		return body
	}
	return b
}

func extractModel(body []byte, r *http.Request) string {
	if len(body) > 0 {
		var m struct {
			Model string `json:"model"`
		}
		if json.Unmarshal(body, &m) == nil && strings.TrimSpace(m.Model) != "" {
			return strings.TrimSpace(m.Model)
		}
	}
	if q := r.URL.Query().Get("model"); q != "" {
		return q
	}
	return ""
}

func isStreamRequest(body []byte) bool {
	if len(body) == 0 {
		return false
	}
	var m struct {
		Stream bool `json:"stream"`
	}
	_ = json.Unmarshal(body, &m)
	return m.Stream
}

func joinOpenAIURL(baseURL, openAIPath string) string {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	path := strings.TrimLeft(openAIPath, "/")
	// base may already end with /v1
	if strings.HasSuffix(strings.ToLower(base), "/v1") {
		return base + "/" + path
	}
	// if path already includes v1
	if strings.HasPrefix(path, "v1/") {
		return base + "/" + path
	}
	return base + "/v1/" + path
}

// resolveProviderForModel is implemented in route.go (V2 multi-route + SQLite).

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

// --- App bindings ---

// GetProxyStatus returns runtime status of the local OpenAI proxy.
func (a *App) GetProxyStatus() ProxyStatus {
	if a.proxy == nil {
		return ProxyStatus{}
	}
	return a.proxy.status()
}

// GetProxyConfig returns saved proxy settings.
func (a *App) GetProxyConfig() ProxyConfig {
	if a.proxy == nil {
		return loadProxyConfig()
	}
	return a.proxy.getConfig()
}

// SaveProxyConfig persists settings; restarts if running.
func (a *App) SaveProxyConfig(cfg ProxyConfig) (ProxyStatus, error) {
	if a.proxy == nil {
		a.proxy = newProxyServer()
	}
	if err := a.proxy.setConfig(cfg); err != nil {
		return a.proxy.status(), err
	}
	return a.proxy.status(), nil
}

// StartProxy starts the local OpenAI-compatible gateway.
func (a *App) StartProxy() (ProxyStatus, error) {
	if a.proxy == nil {
		a.proxy = newProxyServer()
	}
	if err := a.proxy.start(); err != nil {
		return a.proxy.status(), err
	}
	return a.proxy.status(), nil
}

// StopProxy stops the gateway.
func (a *App) StopProxy() (ProxyStatus, error) {
	if a.proxy == nil {
		return ProxyStatus{}, nil
	}
	if err := a.proxy.stop(); err != nil {
		return a.proxy.status(), err
	}
	return a.proxy.status(), nil
}

// ApplyProxyToCodex rewrites Codex config.toml base_url for vendors that opted
// into proxy (Provider.UseProxy). Does NOT create codex_proxy.
// Real upstream URLs stay in app providers.json for routing.
// Providers with useProxy=false (e.g. Ollama) keep their real base_url.
func (a *App) ApplyProxyToCodex(providerID, model string) (ToolConfigStatus, error) {
	st := a.resolveTool(ToolCodex)
	if a.proxy == nil || !a.proxy.status().Running {
		return st, fmt.Errorf("请先启动代理服务")
	}
	path := st.Path
	if path == "" || !fileExists(path) {
		return st, fmt.Errorf("未找到 Codex 配置文件")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return st, err
	}
	content := string(raw)
	_, _ = ensureDefaultBackup(ToolCodex, path)
	savePreWriteSnapshot(ToolCodex, path, raw)

	localBase := a.proxy.baseURL()
	// remember original base_urls (skip already-local)
	_ = rememberOriginalBases(content, localBase)

	// 1) delete legacy [model_providers.codex_proxy]
	content = removeTomlProviderBlock(content, "codex_proxy")

	// 2) only rewrite providers that want proxy; restore/direct for others
	proxyIDs, direct := providerProxyRouting()
	if len(proxyIDs) == 0 {
		// no vendor opted in — still allow rewriting all non-local as before
		content = setAllProvidersBaseURL(content, localBase)
	} else {
		content = setProvidersBaseURLForIDs(content, localBase, proxyIDs)
		// ensure direct vendors keep real base_url
		for id, base := range direct {
			if base != "" && !isLocalProxyURL(base) {
				content = setProviderField(content, id, "base_url", base)
			}
		}
	}

	// 3) if model_provider was codex_proxy, switch to a real vendor
	curProv := readTomlTopLevelString(content, "model_provider")
	if curProv == "" || curProv == "codex_proxy" {
		if providerID != "" && providerID != "codex_proxy" {
			content = setTomlTopLevelString(content, "model_provider", providerID)
		} else if alt := firstRealProviderID(content); alt != "" {
			content = setTomlTopLevelString(content, "model_provider", alt)
		}
	}
	// 4) optional model id
	if model != "" {
		content = setTomlTopLevelString(content, "model", model)
	}
	// 5) strip [[models]] entries that reference codex_proxy
	content = removeModelsWithProvider(content, "codex_proxy")

	content = preserveLineEndings(string(raw), content)
	if err := writeFileAtomic(path, content); err != nil {
		return st, err
	}
	st = a.resolveTool(ToolCodex)
	if len(proxyIDs) > 0 {
		st.Message = fmt.Sprintf("已将启用代理的厂家 base_url 改为 %s（直连厂家保留原地址，共 %d 个走代理）", localBase, len(proxyIDs))
	} else {
		st.Message = "已将 config.toml 中各厂家 base_url 改为本地代理 " + localBase + "（已删除 codex_proxy）"
	}
	return st, nil
}

// providerProxyRouting returns codex provider ids that should use local proxy,
// and a map of direct (no-proxy) provider id → real base URL.
func providerProxyRouting() (proxyIDs []string, direct map[string]string) {
	direct = map[string]string{}
	list, err := loadProvidersFromDisk()
	if err != nil {
		return nil, direct
	}
	for _, p := range list {
		id := slugify(p.Name)
		if id == "" {
			id = slugify(p.ID)
		}
		if id == "" {
			continue
		}
		// also accept common id forms
		candidates := []string{id, strings.ToLower(p.ID), slugify(p.ID)}
		if providerWantsProxy(p) {
			for _, c := range candidates {
				if c != "" {
					proxyIDs = append(proxyIDs, c)
				}
			}
		} else {
			for _, c := range candidates {
				if c != "" {
					direct[c] = strings.TrimRight(p.BaseURL, "/")
				}
			}
		}
	}
	// unique proxyIDs
	seen := map[string]bool{}
	out := make([]string, 0, len(proxyIDs))
	for _, id := range proxyIDs {
		if seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out, direct
}

func setProvidersBaseURLForIDs(content, baseURL string, ids []string) string {
	allow := map[string]bool{}
	for _, id := range ids {
		allow[id] = true
	}
	re := regexp.MustCompile(`(?m)^\[model_providers\.([^\]]+)\]\s*$`)
	idxs := re.FindAllStringSubmatchIndex(content, -1)
	for i := len(idxs) - 1; i >= 0; i-- {
		loc := idxs[i]
		id := content[loc[2]:loc[3]]
		if id == "codex_proxy" || !allow[id] {
			continue
		}
		content = setProviderField(content, id, "base_url", baseURL)
	}
	return content
}

// RestoreCodexOriginalBases restores base_url saved before ApplyProxyToCodex.
func (a *App) RestoreCodexOriginalBases() (ToolConfigStatus, error) {
	st := a.resolveTool(ToolCodex)
	path := st.Path
	if path == "" || !fileExists(path) {
		return st, fmt.Errorf("未找到 Codex 配置文件")
	}
	orig, err := loadOriginalBases()
	if err != nil || len(orig) == 0 {
		return st, fmt.Errorf("没有可恢复的原始 base_url 记录")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return st, err
	}
	content := string(raw)
	_, _ = ensureDefaultBackup(ToolCodex, path)
	savePreWriteSnapshot(ToolCodex, path, raw)

	content = removeTomlProviderBlock(content, "codex_proxy")
	for id, base := range orig {
		if id == "codex_proxy" || base == "" {
			continue
		}
		content = setProviderField(content, id, "base_url", base)
	}
	content = preserveLineEndings(string(raw), content)
	if err := writeFileAtomic(path, content); err != nil {
		return st, err
	}
	st = a.resolveTool(ToolCodex)
	st.Message = "已恢复各厂家原始 base_url"
	return st, nil
}

func originalBasesPath() string {
	return filepath.Join(managerRoot(), "proxy-original-bases.json")
}

func loadOriginalBases() (map[string]string, error) {
	b, err := os.ReadFile(originalBasesPath())
	if err != nil {
		return nil, err
	}
	var m map[string]string
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	return m, nil
}

func rememberOriginalBases(content, localBase string) error {
	existing, _ := loadOriginalBases()
	if existing == nil {
		existing = map[string]string{}
	}
	// parse all [model_providers.x] base_url
	re := regexp.MustCompile(`(?m)^\[model_providers\.([^\]]+)\]\s*$`)
	idxs := re.FindAllStringSubmatchIndex(content, -1)
	for _, loc := range idxs {
		id := content[loc[2]:loc[3]]
		if id == "codex_proxy" {
			continue
		}
		end := findTomlTableEnd(content, loc[1])
		block := content[loc[0]:end]
		kv, _ := parseProviderKV(block)
		base := strings.TrimSpace(kv["base_url"])
		if base == "" || isLocalProxyURL(base) {
			continue
		}
		// only save first seen original (don't overwrite with local)
		if _, ok := existing[id]; !ok {
			existing[id] = base
		}
	}
	if err := os.MkdirAll(managerRoot(), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(existing, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(originalBasesPath(), b, 0o644)
}

func isLocalProxyURL(u string) bool {
	u = strings.ToLower(strings.TrimSpace(u))
	if u == "" {
		return false
	}
	// parse host if URL-shaped
	if strings.Contains(u, "://") {
		if pu, err := url.Parse(u); err == nil && pu.Hostname() != "" {
			return isLoopbackHost(pu.Hostname())
		}
	}
	return strings.Contains(u, "127.0.0.1") ||
		strings.Contains(u, "localhost") ||
		strings.Contains(u, "[::1]") ||
		strings.Contains(u, "::1")
}

func removeTomlProviderBlock(content, id string) string {
	reHeader := regexp.MustCompile(`(?m)^\[model_providers\.` + regexp.QuoteMeta(id) + `\]\s*$`)
	loc := reHeader.FindStringIndex(content)
	if loc == nil {
		return content
	}
	end := findTomlTableEnd(content, loc[1])
	// also drop trailing blank lines after block
	for end < len(content) && (content[end] == '\n' || content[end] == '\r') {
		end++
		if end < len(content) && content[end-1] == '\n' && content[end] != '\n' && content[end] != '\r' {
			break
		}
	}
	return content[:loc[0]] + content[end:]
}

func setAllProvidersBaseURL(content, baseURL string) string {
	re := regexp.MustCompile(`(?m)^\[model_providers\.([^\]]+)\]\s*$`)
	idxs := re.FindAllStringSubmatchIndex(content, -1)
	// process from end so offsets stay valid
	for i := len(idxs) - 1; i >= 0; i-- {
		loc := idxs[i]
		id := content[loc[2]:loc[3]]
		if id == "codex_proxy" {
			continue
		}
		content = setProviderField(content, id, "base_url", baseURL)
	}
	return content
}

func setProviderField(content, providerID, field, value string) string {
	reHeader := regexp.MustCompile(`(?m)^\[model_providers\.` + regexp.QuoteMeta(providerID) + `\]\s*$`)
	loc := reHeader.FindStringIndex(content)
	if loc == nil {
		return content
	}
	end := findTomlTableEnd(content, loc[1])
	block := content[loc[0]:end]
	kv, order := parseProviderKV(block)
	if _, ok := kv[field]; !ok {
		order = append(order, field)
	}
	kv[field] = value
	return content[:loc[0]] + rebuildProviderBlock(providerID, kv, order) + content[end:]
}

// removeProviderField drops a key from [model_providers.<id>] if present.
func removeProviderField(content, providerID, field string) string {
	reHeader := regexp.MustCompile(`(?m)^\[model_providers\.` + regexp.QuoteMeta(providerID) + `\]\s*$`)
	loc := reHeader.FindStringIndex(content)
	if loc == nil {
		return content
	}
	end := findTomlTableEnd(content, loc[1])
	block := content[loc[0]:end]
	kv, order := parseProviderKV(block)
	if _, ok := kv[field]; !ok {
		return content
	}
	delete(kv, field)
	var order2 []string
	for _, k := range order {
		if k != field {
			order2 = append(order2, k)
		}
	}
	return content[:loc[0]] + rebuildProviderBlock(providerID, kv, order2) + content[end:]
}

func rebuildProviderBlock(providerID string, kv map[string]string, order []string) string {
	var lines []string
	lines = append(lines, "[model_providers."+providerID+"]")
	preferred := []string{"name", "base_url", "env_key", "api_key", "requires_openai_auth", "provider"}
	seen := map[string]bool{}
	for _, k := range preferred {
		if v, ok := kv[k]; ok && v != "" {
			lines = append(lines, k+` = "`+escapeTomlString(v)+`"`)
			seen[k] = true
		}
	}
	for _, k := range order {
		if seen[k] {
			continue
		}
		if v, ok := kv[k]; ok && v != "" {
			lines = append(lines, k+` = "`+escapeTomlString(v)+`"`)
		}
	}
	return strings.Join(lines, "\n") + "\n"
}

func firstRealProviderID(content string) string {
	re := regexp.MustCompile(`(?m)^\[model_providers\.([^\]]+)\]\s*$`)
	for _, m := range re.FindAllStringSubmatch(content, -1) {
		if m[1] != "codex_proxy" {
			return m[1]
		}
	}
	return ""
}

func removeModelsWithProvider(content, provider string) string {
	lines := strings.Split(content, "\n")
	var out []string
	in := false
	var buf []string
	curProv := ""
	flush := func() {
		if !in {
			return
		}
		if curProv != provider {
			out = append(out, buf...)
		}
		buf = nil
		curProv = ""
		in = false
	}
	for _, line := range lines {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "[") {
			if trim == "[[models]]" {
				flush()
				in = true
				buf = []string{line}
				continue
			}
			flush()
			out = append(out, line)
			continue
		}
		if in {
			buf = append(buf, line)
			if strings.HasPrefix(trim, "provider") {
				if i := strings.Index(trim, "="); i > 0 {
					curProv = strings.Trim(strings.TrimSpace(trim[i+1:]), `"`)
				}
			}
			continue
		}
		out = append(out, line)
	}
	flush()
	return strings.Join(out, "\n")
}
