package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type ProviderHealthRequest struct {
	BaseURL   string `json:"baseUrl"`
	APIKey    string `json:"apiKey"`
	APIFormat string `json:"apiFormat"`
	Model     string `json:"model"`
}

type ProviderHealthResult struct {
	OK         bool   `json:"ok"`
	Message    string `json:"message"`
	Endpoint   string `json:"endpoint"`
	StatusCode int    `json:"statusCode"`
	LatencyMs  int64  `json:"latencyMs"`
	APIFormat  string `json:"apiFormat"`
	Error      string `json:"error,omitempty"`
	Sample     string `json:"sample,omitempty"`
}

func (a *App) StreamCheckProvider(req ProviderHealthRequest) ProviderHealthResult {
	p := Provider{BaseURL: strings.TrimRight(strings.TrimSpace(req.BaseURL), "/"), APIKey: strings.TrimSpace(req.APIKey), APIFormat: req.APIFormat}
	format := ResolveAPIFormat(p)
	model := strings.TrimSpace(req.Model)
	if model == "" {
		model = "test-model"
	}
	res := ProviderHealthResult{APIFormat: format}
	if p.BaseURL == "" || p.APIKey == "" {
		res.Message, res.Error = "健康检查失败", "请先填写 API Base URL 和 API Key"
		return res
	}
	var endpoint string
	var body []byte
	var headers = map[string]string{"Content-Type": "application/json"}
	switch format {
	case APIFormatAnthropicMessages:
		endpoint = anthropicMessagesURL(p.BaseURL)
		headers["x-api-key"] = p.APIKey
		headers["anthropic-version"] = "2023-06-01"
		body, _ = json.Marshal(map[string]any{"model": model, "max_tokens": 16, "stream": true, "messages": []map[string]any{{"role": "user", "content": "ping"}}})
	case APIFormatOpenAIResponses:
		endpoint = joinOpenAIURL(p.BaseURL, "responses")
		headers["Authorization"] = "Bearer " + p.APIKey
		body, _ = json.Marshal(map[string]any{"model": model, "stream": true, "input": "ping"})
	default:
		endpoint = joinOpenAIURL(p.BaseURL, "chat/completions")
		headers["Authorization"] = "Bearer " + p.APIKey
		body, _ = json.Marshal(map[string]any{"model": model, "stream": true, "messages": []map[string]any{{"role": "user", "content": "ping"}}})
	}
	res.Endpoint = endpoint
	httpReq, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		res.Message, res.Error = "健康检查失败", err.Error()
		return res
	}
	for k, v := range headers {
		httpReq.Header.Set(k, v)
	}
	httpReq.Header.Set("Accept", "text/event-stream")
	start := time.Now()
	resp, err := (&http.Client{Timeout: 45 * time.Second}).Do(httpReq)
	res.LatencyMs = time.Since(start).Milliseconds()
	if err != nil {
		res.Message, res.Error = "健康检查失败", err.Error()
		return res
	}
	defer resp.Body.Close()
	res.StatusCode = resp.StatusCode
	sample, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
	res.Sample = string(sample)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		res.Message = "健康检查失败"
		res.Error = fmt.Sprintf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(res.Sample))
		return res
	}
	if !strings.Contains(res.Sample, "data:") && !strings.Contains(res.Sample, "event:") {
		res.Message = "健康检查失败"
		res.Error = "未检测到 SSE 数据"
		return res
	}
	res.OK = true
	res.Message = "流式健康检查通过"
	return res
}
