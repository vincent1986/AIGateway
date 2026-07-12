package main

import (
	"bufio"
	"bytes"
	"compress/flate"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type ProxyConfig struct {
	Enabled   bool   `json:"enabled"`
	Host      string `json:"host"`
	Port      int    `json:"port"`
	AutoStart bool   `json:"autoStart"`
	ListenKey string `json:"listenKey"`
}

type ProxyStatus struct {
	Running   bool              `json:"running"`
	BaseURL   string            `json:"baseUrl"`
	Host      string            `json:"host"`
	Port      int               `json:"port"`
	AutoStart bool              `json:"autoStart"`
	ListenKey string            `json:"listenKey"`
	LastError string            `json:"lastError"`
	Logs      []string          `json:"logs"`
	Requests  []ProxyRequestLog `json:"requests"`
	Usage     ProxyUsageStats   `json:"usage"`
}

type ProxyRequestLog struct {
	At               string `json:"at"`
	Method           string `json:"method"`
	Path             string `json:"path"`
	Provider         string `json:"provider"`
	Model            string `json:"model"`
	Format           string `json:"format"`
	Status           int    `json:"status"`
	LatencyMs        int64  `json:"latencyMs"`
	PromptTokens     int    `json:"promptTokens,omitempty"`
	CompletionTokens int    `json:"completionTokens,omitempty"`
	TotalTokens      int    `json:"totalTokens,omitempty"`
	Error            string `json:"error,omitempty"`
}

type ProxyUsageStats struct {
	TotalRequests    int     `json:"totalRequests"`
	ErrorRequests    int     `json:"errorRequests"`
	AvgLatencyMs     float64 `json:"avgLatencyMs"`
	PromptTokens     int     `json:"promptTokens"`
	CompletionTokens int     `json:"completionTokens"`
	TotalTokens      int     `json:"totalTokens"`
}

type proxyServer struct {
	mu         sync.Mutex
	cfg        ProxyConfig
	srv        *http.Server
	ln         net.Listener
	run        bool
	err        string
	logs       []string
	requests   []ProxyRequestLog
	usage      ProxyUsageStats
	latencySum int64
	client     *http.Client
}

func defaultProxyConfig() ProxyConfig {
	return ProxyConfig{Host: "127.0.0.1", Port: 18080}
}

func proxyConfigPath() string { return filepath.Join(managerRoot(), "proxy.json") }

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
	if cfg.Port <= 0 {
		cfg.Port = 18080
	}
	return cfg
}

func saveProxyConfig(cfg ProxyConfig) error {
	_ = os.MkdirAll(managerRoot(), 0o755)
	if cfg.Host == "" {
		cfg.Host = "127.0.0.1"
	}
	if cfg.Port <= 0 {
		cfg.Port = 18080
	}
	b, _ := json.MarshalIndent(cfg, "", "  ")
	return writeFileAtomicMode(proxyConfigPath(), string(b), 0o600)
}

func newProxyServer() *proxyServer {
	return &proxyServer{
		cfg: loadProxyConfig(),
		client: &http.Client{
			Transport: &http.Transport{
				Proxy:               http.ProxyFromEnvironment,
				MaxIdleConns:        32,
				IdleConnTimeout:     90 * time.Second,
				TLSHandshakeTimeout: 15 * time.Second,
			},
		},
		logs: make([]string, 0, 50),
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
	return fmt.Sprintf("http://%s:%d/v1", p.cfg.Host, p.cfg.Port)
}

func (p *proxyServer) status() ProxyStatus {
	p.mu.Lock()
	defer p.mu.Unlock()
	return ProxyStatus{
		Running: p.run, BaseURL: fmt.Sprintf("http://%s:%d/v1", p.cfg.Host, p.cfg.Port),
		Host: p.cfg.Host, Port: p.cfg.Port, AutoStart: p.cfg.AutoStart,
		ListenKey: p.cfg.ListenKey, LastError: p.err, Logs: append([]string(nil), p.logs...),
		Requests: append([]ProxyRequestLog(nil), p.requests...), Usage: p.usage,
	}
}

func (p *proxyServer) recordRequest(log ProxyRequestLog) {
	if log.At == "" {
		log.At = time.Now().Format(time.RFC3339)
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.requests = append(p.requests, log)
	if len(p.requests) > 100 {
		p.requests = p.requests[len(p.requests)-100:]
	}
	p.usage.TotalRequests++
	if log.Status >= 400 || log.Error != "" {
		p.usage.ErrorRequests++
	}
	p.usage.PromptTokens += log.PromptTokens
	p.usage.CompletionTokens += log.CompletionTokens
	p.usage.TotalTokens += log.TotalTokens
	p.latencySum += log.LatencyMs
	if p.usage.TotalRequests > 0 {
		p.usage.AvgLatencyMs = float64(p.latencySum) / float64(p.usage.TotalRequests)
	}
}

func (p *proxyServer) clearUsageStats() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.requests = nil
	p.usage = ProxyUsageStats{}
	p.latencySum = 0
	p.logs = append(p.logs, time.Now().Format("15:04:05")+" 已清空 token 统计")
	if len(p.logs) > 80 {
		p.logs = p.logs[len(p.logs)-80:]
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
		return fmt.Errorf("端口无效")
	}
	if err := validateProxyExposure(cfg); err != nil {
		return err
	}
	p.mu.Lock()
	running := p.run
	p.cfg = cfg
	p.mu.Unlock()
	if err := saveProxyConfig(cfg); err != nil {
		return err
	}
	if running {
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
	if err := validateProxyExposure(cfg); err != nil {
		p.mu.Lock()
		p.err = err.Error()
		p.mu.Unlock()
		return err
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", p.handleHealth)
	mux.HandleFunc("/v1/models", p.handleModels)
	mux.HandleFunc("/models", p.handleModels)
	mux.HandleFunc("/v1/chat/completions", p.handleChatCompletions)
	mux.HandleFunc("/chat/completions", p.handleChatCompletions)
	mux.HandleFunc("/v1/completions", func(w http.ResponseWriter, r *http.Request) { p.forwardOpenAI(w, r, "completions") })
	mux.HandleFunc("/v1/embeddings", func(w http.ResponseWriter, r *http.Request) { p.forwardOpenAI(w, r, "embeddings") })
	mux.HandleFunc("/v1/responses", p.handleResponses)
	mux.HandleFunc("/responses", p.handleResponses)
	mux.HandleFunc("/v1/", p.handleOpenAIProxy)
	mux.HandleFunc("/", p.handleRoot)

	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		p.mu.Lock()
		p.err = err.Error()
		p.mu.Unlock()
		return err
	}
	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 15 * time.Second}
	p.mu.Lock()
	p.ln, p.srv, p.run, p.err = ln, srv, true, ""
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
		p.logs = append(p.logs, time.Now().Format("15:04:05")+" "+msg)
		p.mu.Unlock()
	}()
	return nil
}

func validateProxyExposure(cfg ProxyConfig) error {
	host := strings.TrimSpace(strings.Trim(cfg.Host, "[]"))
	if strings.EqualFold(host, "localhost") {
		return nil
	}
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
		return nil
	}
	if strings.TrimSpace(cfg.ListenKey) == "" {
		return fmt.Errorf("监听非本机地址 %q 时必须设置接入密钥", cfg.Host)
	}
	return nil
}

func (p *proxyServer) stop() error {
	p.mu.Lock()
	srv, ln := p.srv, p.ln
	p.run = false
	cfg := p.cfg
	cfg.Enabled = false
	p.cfg = cfg
	p.mu.Unlock()
	_ = saveProxyConfig(cfg)
	if ln != nil {
		_ = ln.Close()
	}
	if srv != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		_ = srv.Shutdown(ctx)
		cancel()
		_ = srv.Close()
	}
	if p.client != nil {
		p.client.CloseIdleConnections()
	}
	p.mu.Lock()
	p.srv, p.ln = nil, nil
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
		"note":    "Client uses standard OpenAI; non-OpenAI upstreams are converted automatically",
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
	if strings.HasPrefix(strings.ToLower(auth), "bearer ") && strings.TrimSpace(auth[7:]) == key {
		return nil
	}
	if r.Header.Get("api-key") == key || r.Header.Get("x-api-key") == key {
		return nil
	}
	return fmt.Errorf("unauthorized")
}

func (p *proxyServer) handleModels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", 405)
		return
	}
	if err := p.checkListenAuth(r); err != nil {
		http.Error(w, err.Error(), 401)
		return
	}
	providers, _ := loadProvidersFromDisk()
	type item struct {
		ID      string `json:"id"`
		Object  string `json:"object"`
		OwnedBy string `json:"owned_by"`
	}
	data := []item{}
	seen := map[string]bool{}
	for _, prov := range providers {
		for _, m := range prov.Models {
			if !m.Enabled || m.ID == "" || seen[m.ID] {
				continue
			}
			seen[m.ID] = true
			owned := prov.Name
			if m.OwnedBy != "" {
				owned = m.OwnedBy
			}
			data = append(data, item{ID: m.ID, Object: "model", OwnedBy: owned})
		}
	}
	writeJSON(w, 200, map[string]any{"object": "list", "data": data})
}

func (p *proxyServer) handleChatCompletions(w http.ResponseWriter, r *http.Request) {
	p.forwardOpenAI(w, r, "chat/completions")
}

func (p *proxyServer) handleOpenAIProxy(w http.ResponseWriter, r *http.Request) {
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/v1/"), "/")
	if path == "" {
		p.handleRoot(w, r)
		return
	}
	if path == "responses" || strings.HasPrefix(path, "responses/") {
		p.handleResponses(w, r)
		return
	}
	p.forwardOpenAI(w, r, path)
}

// forwardOpenAI: client always speaks OpenAI; convert when upstream is non-standard.
func (p *proxyServer) forwardOpenAI(w http.ResponseWriter, r *http.Request, openAIPath string) {
	if r.Method != http.MethodPost && r.Method != http.MethodGet {
		http.Error(w, "method not allowed", 405)
		return
	}
	if err := p.checkListenAuth(r); err != nil {
		http.Error(w, err.Error(), 401)
		return
	}
	body, err := readRequestBody(r, 32<<20)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}

	model := extractModel(body, r)
	prov, routeErr := resolveProviderForModel(model)
	if routeErr != nil {
		p.logf("路由失败 model=%q: %v", model, routeErr)
		writeJSON(w, 400, map[string]any{"error": map[string]any{"message": routeErr.Error(), "type": "invalid_request_error"}})
		return
	}

	fmtName := ResolveAPIFormat(prov)
	stream := isStreamRequest(body)
	p.logf("%s %s → %s model=%s format=%s stream=%v", r.Method, openAIPath, prov.Name, model, fmtName, stream)
	start := time.Now()

	// Non-standard upstream: Anthropic native
	if fmtName == APIFormatAnthropicMessages && (openAIPath == "chat/completions" || openAIPath == "responses") {
		p.forwardAnthropicFromOpenAI(w, r, prov, body, stream, openAIPath == "responses")
		return
	}
	if fmtName == APIFormatOpenAIResponses && openAIPath == "responses" {
		body = sanitizeResponsesRequestForProvider(body, prov)
	}

	// Standard OpenAI-compatible upstream
	upstreamURL := joinOpenAIURL(prov.BaseURL, openAIPath)
	if r.URL.RawQuery != "" {
		if strings.Contains(upstreamURL, "?") {
			upstreamURL += "&" + r.URL.RawQuery
		} else {
			upstreamURL += "?" + r.URL.RawQuery
		}
	}
	req, err := http.NewRequestWithContext(r.Context(), r.Method, upstreamURL, bytes.NewReader(body))
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	if prov.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+prov.APIKey)
		req.Header.Set("api-key", prov.APIKey)
	}
	if stream {
		req.Header.Set("Accept", "text/event-stream")
	}
	client := p.client
	if !stream {
		client = &http.Client{Timeout: 180 * time.Second, Transport: p.client.Transport}
	}
	resp, err := client.Do(req)
	if err != nil {
		p.logf("上游错误: %v", err)
		p.recordRequest(ProxyRequestLog{Method: r.Method, Path: openAIPath, Provider: prov.Name, Model: model, Format: fmtName, Status: 502, LatencyMs: time.Since(start).Milliseconds(), Error: err.Error()})
		writeJSON(w, 502, map[string]any{"error": map[string]any{"message": "upstream: " + err.Error()}})
		return
	}
	defer resp.Body.Close()
	if !stream {
		body, readErr := io.ReadAll(resp.Body)
		log := ProxyRequestLog{Method: r.Method, Path: openAIPath, Provider: prov.Name, Model: model, Format: fmtName, Status: resp.StatusCode, LatencyMs: time.Since(start).Milliseconds()}
		log.PromptTokens, log.CompletionTokens, log.TotalTokens = usageFromJSONBody(body)
		if readErr != nil {
			log.Error = readErr.Error()
		}
		p.recordRequest(log)
		copyHeaders(w, resp)
		w.WriteHeader(resp.StatusCode)
		_, _ = w.Write(body)
		return
	}
	copyHeaders(w, resp)
	w.WriteHeader(resp.StatusCode)
	prompt, completion, total, streamErr := relaySSE(w, resp.Body)
	log := ProxyRequestLog{Method: r.Method, Path: openAIPath, Provider: prov.Name, Model: model, Format: fmtName, Status: resp.StatusCode, LatencyMs: time.Since(start).Milliseconds(), PromptTokens: prompt, CompletionTokens: completion, TotalTokens: total}
	if streamErr != nil && streamErr != io.EOF {
		log.Error = streamErr.Error()
	}
	p.recordRequest(log)
}

func (p *proxyServer) forwardAnthropicFromOpenAI(w http.ResponseWriter, r *http.Request, prov Provider, openaiBody []byte, stream, asResponses bool) {
	// Always convert client OpenAI payload → Anthropic messages
	var chatBody []byte
	var err error
	if asResponses {
		chatBody, err = responsesBodyToChat(openaiBody)
		if err != nil {
			writeJSON(w, 400, map[string]any{"error": map[string]any{"message": err.Error()}})
			return
		}
	} else {
		chatBody = openaiBody
	}
	anthBody, err := convertOpenAIChatToAnthropic(chatBody)
	if err != nil {
		writeJSON(w, 400, map[string]any{"error": map[string]any{"message": "convert: " + err.Error()}})
		return
	}
	// Anthropic streaming is different; force non-stream for reliability then wrap
	anthBody = forceStreamFalse(anthBody)

	url := anthropicMessagesURL(prov.BaseURL)
	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, url, bytes.NewReader(anthBody))
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", prov.APIKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	client := &http.Client{Timeout: 180 * time.Second, Transport: p.client.Transport}
	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		p.recordRequest(ProxyRequestLog{Method: r.Method, Path: r.URL.Path, Provider: prov.Name, Model: extractModel(openaiBody, r), Format: APIFormatAnthropicMessages, Status: 502, LatencyMs: time.Since(start).Milliseconds(), Error: err.Error()})
		writeJSON(w, 502, map[string]any{"error": map[string]any{"message": "upstream: " + err.Error()}})
		return
	}
	defer resp.Body.Close()
	upBody, _ := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	model := extractModel(openaiBody, r)
	log := ProxyRequestLog{Method: r.Method, Path: r.URL.Path, Provider: prov.Name, Model: model, Format: APIFormatAnthropicMessages, Status: resp.StatusCode, LatencyMs: time.Since(start).Milliseconds()}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		p.recordRequest(log)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(resp.StatusCode)
		_, _ = w.Write(upBody)
		p.logf("anthropic HTTP %d", resp.StatusCode)
		return
	}
	chatOut, err := convertAnthropicToOpenAIChat(upBody, model)
	if err != nil {
		log.Error = err.Error()
		p.recordRequest(log)
		writeJSON(w, 502, map[string]any{"error": map[string]any{"message": err.Error()}})
		return
	}
	if asResponses {
		out, err := chatBodyToResponses(chatOut, model)
		if err != nil {
			log.Error = err.Error()
			p.recordRequest(log)
			writeJSON(w, 502, map[string]any{"error": map[string]any{"message": err.Error()}})
			return
		}
		log.PromptTokens, log.CompletionTokens, log.TotalTokens = usageFromJSONBody(out)
		p.recordRequest(log)
		writeResponsesResult(w, out, stream)
		p.logf("anthropic→responses OK model=%s", model)
		return
	}
	log.PromptTokens, log.CompletionTokens, log.TotalTokens = usageFromJSONBody(chatOut)
	p.recordRequest(log)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(200)
	_, _ = w.Write(chatOut)
	p.logf("anthropic→openai chat OK model=%s", model)
}

func copyResponse(w http.ResponseWriter, resp *http.Response, stream bool) {
	copyHeaders(w, resp)
	w.WriteHeader(resp.StatusCode)
	if stream {
		flusher, ok := w.(http.Flusher)
		buf := make([]byte, 4096)
		for {
			n, err := resp.Body.Read(buf)
			if n > 0 {
				_, _ = w.Write(buf[:n])
				if ok {
					flusher.Flush()
				}
			}
			if err != nil {
				break
			}
		}
		return
	}
	_, _ = io.Copy(w, resp.Body)
}

func relaySSE(w http.ResponseWriter, body io.Reader) (int, int, int, error) {
	flusher, _ := w.(http.Flusher)
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 64<<10), 4<<20)
	prompt, completion, total := 0, 0, 0
	for scanner.Scan() {
		line := scanner.Text()
		_, _ = io.WriteString(w, line+"\n")
		if flusher != nil {
			flusher.Flush()
		}
		if strings.HasPrefix(line, "data:") {
			data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if data != "" && data != "[DONE]" {
				if p, c, t := usageFromEventJSON([]byte(data)); t > 0 || p > 0 || c > 0 {
					prompt, completion, total = p, c, t
				}
			}
		}
	}
	return prompt, completion, total, scanner.Err()
}

func usageFromEventJSON(body []byte) (int, int, int) {
	var payload map[string]any
	if json.Unmarshal(body, &payload) != nil {
		return 0, 0, 0
	}
	if p, c, t := usageFromMap(payload); p > 0 || c > 0 || t > 0 {
		return p, c, t
	}
	for _, key := range []string{"response", "data"} {
		if nested, ok := payload[key].(map[string]any); ok {
			if p, c, t := usageFromMap(nested); p > 0 || c > 0 || t > 0 {
				return p, c, t
			}
		}
	}
	return 0, 0, 0
}

func usageFromMap(payload map[string]any) (int, int, int) {
	usage, _ := payload["usage"].(map[string]any)
	if usage == nil {
		return 0, 0, 0
	}
	prompt := intFromAny(usage["prompt_tokens"])
	if prompt == 0 {
		prompt = intFromAny(usage["input_tokens"])
	}
	completion := intFromAny(usage["completion_tokens"])
	if completion == 0 {
		completion = intFromAny(usage["output_tokens"])
	}
	total := intFromAny(usage["total_tokens"])
	if total == 0 {
		total = prompt + completion
	}
	return prompt, completion, total
}

func copyHeaders(w http.ResponseWriter, resp *http.Response) {
	for k, vv := range resp.Header {
		lk := strings.ToLower(k)
		if lk == "connection" || lk == "transfer-encoding" {
			continue
		}
		for _, v := range vv {
			w.Header().Add(k, v)
		}
	}
}

func usageFromJSONBody(body []byte) (int, int, int) {
	var payload map[string]any
	if json.Unmarshal(body, &payload) != nil {
		return 0, 0, 0
	}
	return usageFromMap(payload)
}

func intFromAny(v any) int {
	switch x := v.(type) {
	case float64:
		return int(x)
	case int:
		return x
	case json.Number:
		n, _ := x.Int64()
		return int(n)
	default:
		return 0
	}
}

func readRequestBody(r *http.Request, limit int64) ([]byte, error) {
	defer r.Body.Close()
	reader := io.Reader(r.Body)
	encoding := strings.ToLower(strings.TrimSpace(r.Header.Get("Content-Encoding")))
	switch encoding {
	case "", "identity":
	case "gzip":
		gz, err := gzip.NewReader(r.Body)
		if err != nil {
			return nil, err
		}
		defer gz.Close()
		reader = gz
	case "deflate":
		fl := flate.NewReader(r.Body)
		defer fl.Close()
		reader = fl
	default:
		return nil, fmt.Errorf("unsupported content-encoding %q", encoding)
	}
	return io.ReadAll(io.LimitReader(reader, limit))
}

func extractModel(body []byte, r *http.Request) string {
	if len(body) > 0 {
		var m struct {
			Model string `json:"model"`
		}
		if json.Unmarshal(body, &m) == nil && m.Model != "" {
			return m.Model
		}
	}
	return r.URL.Query().Get("model")
}

func isStreamRequest(body []byte) bool {
	var m struct {
		Stream bool `json:"stream"`
	}
	_ = json.Unmarshal(body, &m)
	return m.Stream
}

func joinOpenAIURL(baseURL, openAIPath string) string {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	path := strings.TrimLeft(openAIPath, "/")
	if strings.HasSuffix(strings.ToLower(base), "/v1") {
		return base + "/" + path
	}
	if strings.HasPrefix(path, "v1/") {
		return base + "/" + path
	}
	return base + "/v1/" + path
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

// --- App bindings ---
func (a *App) GetProxyStatus() ProxyStatus {
	if a.proxy == nil {
		return ProxyStatus{}
	}
	return a.proxy.status()
}
func (a *App) GetProxyConfig() ProxyConfig {
	if a.proxy == nil {
		return loadProxyConfig()
	}
	return a.proxy.getConfig()
}
func (a *App) SaveProxyConfig(cfg ProxyConfig) (ProxyStatus, error) {
	if a.proxy == nil {
		a.proxy = newProxyServer()
	}
	if err := a.proxy.setConfig(cfg); err != nil {
		return a.proxy.status(), err
	}
	return a.proxy.status(), nil
}
func (a *App) StartProxy() (ProxyStatus, error) {
	if a.proxy == nil {
		a.proxy = newProxyServer()
	}
	if err := a.proxy.start(); err != nil {
		return a.proxy.status(), err
	}
	return a.proxy.status(), nil
}
func (a *App) StopProxy() (ProxyStatus, error) {
	if a.proxy == nil {
		return ProxyStatus{}, nil
	}
	_ = a.proxy.stop()
	return a.proxy.status(), nil
}

func (a *App) ClearProxyUsageStats() ProxyStatus {
	if a.proxy == nil {
		a.proxy = newProxyServer()
	}
	a.proxy.clearUsageStats()
	return a.proxy.status()
}

// ApplyProxyToCodex rewrites all provider base_url in config.toml to local proxy.
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
	localBase := a.proxy.baseURL()
	_ = rememberOriginalBases(content, localBase)
	content = removeTomlProviderBlock(content, "codex_proxy")
	content = setAllProvidersBaseURL(content, localBase)
	if cur := readTomlTopLevelString(content, "model_provider"); cur == "" || cur == "codex_proxy" {
		if providerID != "" && providerID != "codex_proxy" {
			content = setTomlTopLevelString(content, "model_provider", providerID)
		} else if alt := firstRealProviderID(content); alt != "" {
			content = setTomlTopLevelString(content, "model_provider", alt)
		}
	}
	if model != "" {
		content = setTomlTopLevelString(content, "model", model)
	}
	content = removeModelsWithProvider(content, "codex_proxy")
	if _, err := writeConfigWithSnapshot(path, content, "codex proxy takeover"); err != nil {
		return st, err
	}
	st = a.resolveTool(ToolCodex)
	st.Message = "已将 base_url 改为本地代理 " + localBase
	return st, nil
}

func (a *App) PreviewApplyProxyToCodex(providerID, model string) (ConfigWriteResult, error) {
	st := a.resolveTool(ToolCodex)
	if a.proxy == nil || !a.proxy.status().Running {
		return ConfigWriteResult{Path: st.Path}, fmt.Errorf("请先启动代理服务")
	}
	path := st.Path
	if path == "" || !fileExists(path) {
		return ConfigWriteResult{Path: path}, fmt.Errorf("未找到 Codex 配置文件")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return ConfigWriteResult{Path: path}, err
	}
	content := string(raw)
	localBase := a.proxy.baseURL()
	content = removeTomlProviderBlock(content, "codex_proxy")
	content = setAllProvidersBaseURL(content, localBase)
	if cur := readTomlTopLevelString(content, "model_provider"); cur == "" || cur == "codex_proxy" {
		if providerID != "" && providerID != "codex_proxy" {
			content = setTomlTopLevelString(content, "model_provider", providerID)
		} else if alt := firstRealProviderID(content); alt != "" {
			content = setTomlTopLevelString(content, "model_provider", alt)
		}
	}
	if model != "" {
		content = setTomlTopLevelString(content, "model", model)
	}
	content = removeModelsWithProvider(content, "codex_proxy")
	return previewConfigWrite(path, content)
}

func (a *App) RestoreCodexOriginalBases() (ToolConfigStatus, error) {
	st := a.resolveTool(ToolCodex)
	path := st.Path
	if path == "" || !fileExists(path) {
		return st, fmt.Errorf("未找到配置文件")
	}
	orig, err := loadOriginalProviderState()
	if err != nil || len(orig.Providers) == 0 {
		return st, fmt.Errorf("没有可恢复的原始 base_url")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return st, err
	}
	content := string(raw)
	content = removeTomlProviderBlock(content, "codex_proxy")
	for id, saved := range orig.Providers {
		if id == "codex_proxy" {
			continue
		}
		content = restoreProviderField(content, id, "base_url", saved.BaseURL)
		content = restoreProviderField(content, id, "wire_api", saved.WireAPI)
	}
	if _, err := writeConfigWithSnapshot(path, content, "codex proxy restore"); err != nil {
		return st, err
	}
	st = a.resolveTool(ToolCodex)
	st.Message = "已恢复原始 provider 接管状态"
	return st, nil
}

// toml helpers used by proxy apply
func originalBasesPath() string {
	return filepath.Join(managerRoot(), "proxy-original-bases.json")
}

type savedProviderField struct {
	Present bool   `json:"present"`
	Value   string `json:"value,omitempty"`
}

type savedProviderState struct {
	BaseURL savedProviderField `json:"baseUrl"`
	WireAPI savedProviderField `json:"wireApi"`
}

type originalProviderState struct {
	Version   int                           `json:"version"`
	Providers map[string]savedProviderState `json:"providers"`
}

func loadOriginalProviderState() (originalProviderState, error) {
	b, err := os.ReadFile(originalBasesPath())
	if err != nil {
		return originalProviderState{}, err
	}
	var state originalProviderState
	if json.Unmarshal(b, &state) == nil && len(state.Providers) > 0 {
		return state, nil
	}
	// Backward compatibility with the old {provider: base_url} store.
	var legacy map[string]string
	if err := json.Unmarshal(b, &legacy); err != nil {
		return originalProviderState{}, err
	}
	state = originalProviderState{Version: 1, Providers: map[string]savedProviderState{}}
	for id, base := range legacy {
		state.Providers[id] = savedProviderState{BaseURL: savedProviderField{Present: true, Value: base}}
	}
	return state, nil
}
func rememberOriginalBases(content, localBase string) error {
	existing, _ := loadOriginalProviderState()
	if existing.Providers == nil {
		existing = originalProviderState{Version: 2, Providers: map[string]savedProviderState{}}
	}
	for _, table := range parseTomlProviderTables(content) {
		id := table.ID
		if id == "codex_proxy" {
			continue
		}
		kv, _ := parseProviderKV(content[table.Start:table.End])
		base, hasBase := kv["base_url"]
		base = strings.TrimSpace(base)
		if base == "" || isLocalProxyURL(base) {
			continue
		}
		if _, saved := existing.Providers[id]; saved {
			continue
		}
		wire, hasWire := kv["wire_api"]
		existing.Providers[id] = savedProviderState{
			BaseURL: savedProviderField{Present: hasBase, Value: strings.TrimSpace(base)},
			WireAPI: savedProviderField{Present: hasWire, Value: strings.TrimSpace(wire)},
		}
	}
	existing.Version = 2
	_ = os.MkdirAll(managerRoot(), 0o755)
	b, _ := json.MarshalIndent(existing, "", "  ")
	return writeFileAtomicMode(originalBasesPath(), string(b), 0o600)
}

func restoreProviderField(content, providerID, field string, saved savedProviderField) string {
	if saved.Present {
		return setProviderField(content, providerID, field, saved.Value)
	}
	return removeProviderField(content, providerID, field)
}

func removeProviderField(content, providerID, field string) string {
	table, ok := findProviderTable(content, providerID)
	if !ok {
		return content
	}
	block := content[table.Start:table.End]
	lines := strings.SplitAfter(block, "\n")
	for i, line := range lines {
		trim := strings.TrimSpace(strings.TrimSuffix(line, "\n"))
		if eq := strings.IndexByte(trim, '='); eq >= 0 && strings.TrimSpace(trim[:eq]) == field {
			lines = append(lines[:i], lines[i+1:]...)
			break
		}
	}
	return content[:table.Start] + strings.Join(lines, "") + content[table.End:]
}
func isLocalProxyURL(u string) bool {
	u = strings.ToLower(u)
	return strings.Contains(u, "127.0.0.1") || strings.Contains(u, "localhost")
}
func removeTomlProviderBlock(content, id string) string {
	table, ok := findProviderTable(content, id)
	if !ok {
		return content
	}
	return content[:table.Start] + content[table.End:]
}
func setAllProvidersBaseURL(content, baseURL string) string {
	ids := tomlProviderIDs(content)
	for i := len(ids) - 1; i >= 0; i-- {
		id := ids[i]
		if id == "codex_proxy" {
			continue
		}
		content = setProviderField(content, id, "base_url", baseURL)
		content = setProviderField(content, id, "wire_api", "chat")
	}
	return content
}
func setProviderField(content, providerID, field, value string) string {
	table, ok := findProviderTable(content, providerID)
	if !ok {
		return content
	}
	block := content[table.Start:table.End]
	lines := strings.Split(block, "\n")
	replaced := false
	for i, line := range lines {
		trim := strings.TrimSpace(line)
		eq := strings.IndexByte(trim, '=')
		if eq < 0 || strings.TrimSpace(trim[:eq]) != field {
			continue
		}
		indent := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
		comment := tomlInlineComment(trim[eq+1:])
		lines[i] = indent + field + ` = "` + escapeTomlString(value) + `"` + comment
		replaced = true
		break
	}
	if !replaced {
		insert := field + ` = "` + escapeTomlString(value) + `"`
		if len(lines) > 0 && lines[len(lines)-1] == "" {
			lines = append(lines[:len(lines)-1], insert, "")
		} else {
			lines = append(lines, insert)
		}
	}
	return content[:table.Start] + strings.Join(lines, "\n") + content[table.End:]
}

func tomlInlineComment(valuePart string) string {
	inString, escaped := false, false
	for i, r := range valuePart {
		if escaped {
			escaped = false
			continue
		}
		if r == '\\' && inString {
			escaped = true
			continue
		}
		if r == '"' {
			inString = !inString
			continue
		}
		if r == '#' && !inString {
			return " " + strings.TrimSpace(valuePart[i:])
		}
	}
	return ""
}
func firstRealProviderID(content string) string {
	for _, id := range tomlProviderIDs(content) {
		if id != "codex_proxy" {
			return id
		}
	}
	return ""
}
func removeModelsWithProvider(content, provider string) string {
	lines := strings.Split(content, "\n")
	var out, buf []string
	in := false
	cur := ""
	flush := func() {
		if !in {
			return
		}
		if cur != provider {
			out = append(out, buf...)
		}
		buf, cur, in = nil, "", false
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
					cur = strings.Trim(strings.TrimSpace(trim[i+1:]), `"`)
				}
			}
			continue
		}
		out = append(out, line)
	}
	flush()
	return strings.Join(out, "\n")
}
