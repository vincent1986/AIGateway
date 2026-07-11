package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// handleResponses implements POST /v1/responses (OpenAI Responses API).
// Codex with wire_api = "responses" uses this path.
// Most third-party vendors only support /v1/chat/completions, so we:
//  1) try upstream /v1/responses first for OpenAI-like hosts
//  2) otherwise convert request → chat/completions → wrap back as responses
func (p *proxyServer) handleResponses(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
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
	prov, routeErr := resolveProviderForModel(model)
	if routeErr != nil {
		p.logf("responses 路由失败 model=%q: %v", model, routeErr)
		writeJSON(w, 400, map[string]any{
			"error": map[string]any{"message": routeErr.Error(), "type": "invalid_request_error", "code": "model_not_routed"},
		})
		return
	}

	stream := isStreamRequest(body)
	p.logf("POST responses → %s model=%s stream=%v", prov.Name, model, stream)

	// Prefer native /responses for OpenAI / Azure OpenAI hosts
	if supportsNativeResponses(prov.BaseURL) {
		if err := p.forwardRaw(w, r, prov, "responses", body, stream); err == nil {
			return
		}
		// fall through to chat conversion on failure
		p.logf("native responses 失败，回退 chat/completions")
	}

	// Convert Responses API → Chat Completions for OpenAI-compatible vendors
	chatBody, err := responsesBodyToChat(body)
	if err != nil {
		writeJSON(w, 400, map[string]any{
			"error": map[string]any{"message": "convert to chat: " + err.Error(), "type": "invalid_request_error"},
		})
		return
	}
	// Map OpenAI-only roles (developer → system) for DeepSeek / third-party vendors
	chatBody = normalizeUpstreamChatBody(chatBody)

	if stream {
		p.streamResponsesViaChat(w, r, prov, chatBody, model)
		return
	}

	// Non-stream: single chat completion → responses object
	chatBody = forceStreamFalse(chatBody)
	upstreamURL := joinOpenAIURL(prov.BaseURL, "chat/completions")
	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, upstreamURL, bytes.NewReader(chatBody))
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	if prov.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+prov.APIKey)
		req.Header.Set("api-key", prov.APIKey)
	}

	client := &http.Client{Timeout: 180 * time.Second, Transport: p.client.Transport}
	resp, err := client.Do(req)
	if err != nil {
		p.logf("responses→chat 上游错误: %v", err)
		writeJSON(w, 502, map[string]any{
			"error": map[string]any{"message": "upstream: " + err.Error(), "type": "proxy_error"},
		})
		return
	}
	defer resp.Body.Close()
	upBody, _ := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(resp.StatusCode)
		_, _ = w.Write(upBody)
		p.logf("responses→chat 上游 HTTP %d", resp.StatusCode)
		return
	}

	out, err := chatBodyToResponses(upBody, model)
	if err != nil {
		writeJSON(w, 502, map[string]any{
			"error": map[string]any{"message": "wrap response: " + err.Error(), "type": "proxy_error"},
		})
		return
	}
	recordUsageFromPayload(upBody, prov.Name, model, "responses", resp.StatusCode)

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(200)
	_, _ = w.Write(out)
}

// streamResponsesViaChat proxies chat/completions SSE and re-emits OpenAI
// Responses API events that Codex expects (must end with response.completed).
func (p *proxyServer) streamResponsesViaChat(w http.ResponseWriter, r *http.Request, prov Provider, chatBody []byte, model string) {
	chatBody = forceStreamTrue(chatBody)
	upstreamURL := joinOpenAIURL(prov.BaseURL, "chat/completions")
	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, upstreamURL, bytes.NewReader(chatBody))
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	if prov.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+prov.APIKey)
		req.Header.Set("api-key", prov.APIKey)
	}

	// No overall timeout — stream may run long; rely on context cancel.
	client := &http.Client{Transport: p.client.Transport}
	resp, err := client.Do(req)
	if err != nil {
		p.logf("responses stream 上游错误: %v", err)
		writeJSON(w, 502, map[string]any{
			"error": map[string]any{"message": "upstream: " + err.Error(), "type": "proxy_error"},
		})
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		upBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		// Client asked for stream — still return JSON error (Codex surfaces it).
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(resp.StatusCode)
		_, _ = w.Write(upBody)
		p.logf("responses stream 上游 HTTP %d", resp.StatusCode)
		return
	}

	sse := newResponsesSSE(w)
	if !sse.ok {
		http.Error(w, "streaming unsupported", 500)
		return
	}
	sse.begin()

	respID := fmt.Sprintf("resp_%d", time.Now().UnixNano())
	msgID := respID + "_msg"
	createdAt := time.Now().Unix()

	// Accumulate assistant text + tool calls
	var textBuf strings.Builder
	// tool index → {id, name, arguments}
	type tcAcc struct {
		id, name, args string
		added          bool
	}
	tools := map[int]*tcAcc{}
	msgItemAdded := false
	contentPartAdded := false

	inProgress := buildResponsesObject(respID, model, createdAt, "in_progress", "", nil)
	sse.emit("response.created", map[string]any{"response": cloneMap(inProgress)})
	sse.emit("response.in_progress", map[string]any{"response": cloneMap(inProgress)})

	scanner := bufio.NewScanner(resp.Body)
	// chat chunks can be large (tool args)
	scanner.Buffer(make([]byte, 0, 64*1024), 8<<20)

	finishReason := ""
	usage := any(nil)

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" || strings.HasPrefix(line, ":") {
			continue
		}
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" || data == "[DONE]" {
			continue
		}

		var chunk map[string]any
		if json.Unmarshal([]byte(data), &chunk) != nil {
			continue
		}
		if u, ok := chunk["usage"]; ok && u != nil {
			usage = u
		}
		// reuse upstream id if present
		if id, _ := chunk["id"].(string); id != "" && strings.HasPrefix(respID, "resp_") {
			// keep our resp_ id for stability once emitted; ignore
		}

		choices, _ := chunk["choices"].([]any)
		if len(choices) == 0 {
			continue
		}
		c0, _ := choices[0].(map[string]any)
		if c0 == nil {
			continue
		}
		if fr, ok := c0["finish_reason"].(string); ok && fr != "" {
			finishReason = fr
		}
		delta, _ := c0["delta"].(map[string]any)
		if delta == nil {
			// some vendors put message instead of delta on last chunk
			if msg, ok := c0["message"].(map[string]any); ok {
				delta = msg
			} else {
				continue
			}
		}

		// text content
		if content, ok := delta["content"].(string); ok && content != "" {
			if !msgItemAdded {
				sse.emit("response.output_item.added", map[string]any{
					"output_index": 0,
					"item": map[string]any{
						"type":   "message",
						"id":     msgID,
						"role":   "assistant",
						"status": "in_progress",
						"content": []any{},
					},
				})
				msgItemAdded = true
			}
			if !contentPartAdded {
				sse.emit("response.content_part.added", map[string]any{
					"item_id":       msgID,
					"output_index":  0,
					"content_index": 0,
					"part": map[string]any{
						"type": "output_text",
						"text": "",
					},
				})
				contentPartAdded = true
			}
			textBuf.WriteString(content)
			sse.emit("response.output_text.delta", map[string]any{
				"item_id":       msgID,
				"output_index":  0,
				"content_index": 0,
				"delta":         content,
				"logprobs":      []any{},
			})
		}

		// tool_calls
		if tcs, ok := delta["tool_calls"].([]any); ok {
			for _, raw := range tcs {
				tc, _ := raw.(map[string]any)
				if tc == nil {
					continue
				}
				idx := 0
				switch v := tc["index"].(type) {
				case float64:
					idx = int(v)
				case int:
					idx = v
				}
				acc := tools[idx]
				if acc == nil {
					acc = &tcAcc{}
					tools[idx] = acc
				}
				if id, _ := tc["id"].(string); id != "" {
					acc.id = id
				}
				if fn, ok := tc["function"].(map[string]any); ok {
					if n, _ := fn["name"].(string); n != "" {
						acc.name = n
					}
					switch a := fn["arguments"].(type) {
					case string:
						acc.args += a
					}
				}
				// flat name/arguments
				if n, _ := tc["name"].(string); n != "" {
					acc.name = n
				}
				if a, ok := tc["arguments"].(string); ok {
					acc.args += a
				}
				if !acc.added && acc.name != "" {
					callID := acc.id
					if callID == "" {
						callID = fmt.Sprintf("call_%s_%d", respID, idx)
						acc.id = callID
					}
					itemID := "fc_" + callID
					// function_call items are separate outputs after message (if any)
					outIdx := 0
					if msgItemAdded {
						outIdx = 1 + idx
					} else {
						outIdx = idx
					}
					sse.emit("response.output_item.added", map[string]any{
						"output_index": outIdx,
						"item": map[string]any{
							"type":      "function_call",
							"id":        itemID,
							"call_id":   callID,
							"name":      acc.name,
							"arguments": "",
							"status":    "in_progress",
						},
					})
					acc.added = true
				}
				if acc.added {
					// emit args delta for this chunk only
					argDelta := ""
					if fn, ok := tc["function"].(map[string]any); ok {
						if a, ok := fn["arguments"].(string); ok {
							argDelta = a
						}
					}
					if a, ok := tc["arguments"].(string); ok && argDelta == "" {
						argDelta = a
					}
					if argDelta != "" {
						callID := acc.id
						itemID := "fc_" + callID
						outIdx := idx
						if msgItemAdded {
							outIdx = 1 + idx
						}
						sse.emit("response.function_call_arguments.delta", map[string]any{
							"item_id":       itemID,
							"output_index":  outIdx,
							"delta":         argDelta,
						})
					}
				}
			}
		}
	}
	if err := scanner.Err(); err != nil {
		p.logf("responses stream 读取上游: %v", err)
	}

	// Close out text message item
	finalText := textBuf.String()
	if msgItemAdded {
		if contentPartAdded {
			sse.emit("response.output_text.done", map[string]any{
				"item_id":       msgID,
				"output_index":  0,
				"content_index": 0,
				"text":          finalText,
				"logprobs":      []any{},
			})
			sse.emit("response.content_part.done", map[string]any{
				"item_id":       msgID,
				"output_index":  0,
				"content_index": 0,
				"part": map[string]any{
					"type": "output_text",
					"text": finalText,
				},
			})
		}
		sse.emit("response.output_item.done", map[string]any{
			"output_index": 0,
			"item": map[string]any{
				"type":   "message",
				"id":     msgID,
				"role":   "assistant",
				"status": "completed",
				"content": []map[string]any{
					{"type": "output_text", "text": finalText},
				},
			},
		})
	}

	// Close tool call items
	// stable order by index
	maxIdx := -1
	for i := range tools {
		if i > maxIdx {
			maxIdx = i
		}
	}
	for i := 0; i <= maxIdx; i++ {
		acc := tools[i]
		if acc == nil {
			continue
		}
		callID := acc.id
		if callID == "" {
			callID = fmt.Sprintf("call_%s_%d", respID, i)
		}
		itemID := "fc_" + callID
		outIdx := i
		if msgItemAdded {
			outIdx = 1 + i
		}
		if acc.added {
			sse.emit("response.function_call_arguments.done", map[string]any{
				"item_id":      itemID,
				"output_index": outIdx,
				"arguments":    acc.args,
			})
			sse.emit("response.output_item.done", map[string]any{
				"output_index": outIdx,
				"item": map[string]any{
					"type":      "function_call",
					"id":        itemID,
					"call_id":   callID,
					"name":      acc.name,
					"arguments": acc.args,
					"status":    "completed",
				},
			})
		}
	}

	// Build final response.completed payload (Codex requires this event)
	outputs := make([]any, 0, 1+len(tools))
	if msgItemAdded || finalText != "" {
		outputs = append(outputs, map[string]any{
			"type":   "message",
			"id":     msgID,
			"role":   "assistant",
			"status": "completed",
			"content": []map[string]any{
				{"type": "output_text", "text": finalText},
			},
		})
	}
	for i := 0; i <= maxIdx; i++ {
		acc := tools[i]
		if acc == nil || acc.name == "" {
			continue
		}
		callID := acc.id
		if callID == "" {
			callID = fmt.Sprintf("call_%s_%d", respID, i)
		}
		outputs = append(outputs, map[string]any{
			"type":      "function_call",
			"id":        "fc_" + callID,
			"call_id":   callID,
			"name":      acc.name,
			"arguments": acc.args,
			"status":    "completed",
		})
	}
	// empty assistant message if nothing at all (avoid empty output)
	if len(outputs) == 0 {
		outputs = append(outputs, map[string]any{
			"type":   "message",
			"id":     msgID,
			"role":   "assistant",
			"status": "completed",
			"content": []map[string]any{
				{"type": "output_text", "text": ""},
			},
		})
	}

	final := buildResponsesObject(respID, model, createdAt, "completed", finalText, outputs)
	// Codex strictly deserializes ResponseUsage with input_tokens / output_tokens
	// (chat vendors return prompt_tokens / completion_tokens).
	final["usage"] = normalizeUsageToResponses(usage)
	_ = finishReason

	// token stats
	if um, ok := final["usage"].(map[string]any); ok {
		in, out, total := tokensFromUsageMap(um)
		recordUsage(prov.Name, model, "responses", 200, in, out, total)
	}

	sse.emit("response.completed", map[string]any{"response": final})
	p.logf("responses stream 完成 model=%s text_len=%d tools=%d", model, len(finalText), len(tools))
}

type responsesSSE struct {
	w   http.ResponseWriter
	f   http.Flusher
	ok  bool
	seq int
}

func newResponsesSSE(w http.ResponseWriter) *responsesSSE {
	f, _ := w.(http.Flusher)
	return &responsesSSE{w: w, f: f, ok: true}
}

func (s *responsesSSE) begin() {
	h := s.w.Header()
	h.Set("Content-Type", "text/event-stream; charset=utf-8")
	h.Set("Cache-Control", "no-cache")
	h.Set("Connection", "keep-alive")
	h.Set("X-Accel-Buffering", "no")
	s.w.WriteHeader(200)
	if s.f != nil {
		s.f.Flush()
	}
}

func (s *responsesSSE) emit(typ string, payload map[string]any) {
	if payload == nil {
		payload = map[string]any{}
	}
	s.seq++
	payload["type"] = typ
	payload["sequence_number"] = s.seq
	b, err := json.Marshal(payload)
	if err != nil {
		return
	}
	// OpenAI Responses SSE: event line + data JSON that also includes type
	_, _ = fmt.Fprintf(s.w, "event: %s\ndata: %s\n\n", typ, b)
	if s.f != nil {
		s.f.Flush()
	}
}

func forceStreamTrue(body []byte) []byte {
	var m map[string]any
	if json.Unmarshal(body, &m) != nil {
		return body
	}
	m["stream"] = true
	b, err := json.Marshal(m)
	if err != nil {
		return body
	}
	return b
}

func buildResponsesObject(id, model string, createdAt int64, status, outputText string, output []any) map[string]any {
	if output == nil {
		output = []any{}
	}
	// Fields match OpenAI Responses API Response object closely enough for Codex
	// (strict clients require several keys present, including usage on completed).
	obj := map[string]any{
		"id":                  id,
		"object":              "response",
		"created_at":          createdAt,
		"status":              status,
		"model":               model,
		"output_text":         outputText,
		"output":              output,
		"error":               nil,
		"incomplete_details":  nil,
		"instructions":        nil,
		"metadata":            nil,
		"parallel_tool_calls": true,
		"temperature":         nil,
		"tool_choice":         "auto",
		"tools":               []any{},
		"top_p":               nil,
		"truncation":          "disabled",
	}
	if status == "completed" {
		obj["usage"] = normalizeUsageToResponses(nil)
	}
	return obj
}

// normalizeUsageToResponses converts chat.completions usage
// (prompt_tokens / completion_tokens) into Responses API usage
// (input_tokens / output_tokens). Always returns a complete object.
func normalizeUsageToResponses(u any) map[string]any {
	out := map[string]any{
		"input_tokens":  0,
		"output_tokens": 0,
		"total_tokens":  0,
		"input_tokens_details": map[string]any{
			"cached_tokens": 0,
		},
		"output_tokens_details": map[string]any{
			"reasoning_tokens": 0,
		},
	}
	m, ok := u.(map[string]any)
	if !ok || m == nil {
		return out
	}

	inTok := firstInt(m, "input_tokens", "prompt_tokens")
	outTok := firstInt(m, "output_tokens", "completion_tokens")
	total := firstInt(m, "total_tokens")
	if total == 0 {
		total = inTok + outTok
	}
	out["input_tokens"] = inTok
	out["output_tokens"] = outTok
	out["total_tokens"] = total

	cached := 0
	if d, ok := m["input_tokens_details"].(map[string]any); ok {
		cached = firstInt(d, "cached_tokens")
	}
	if cached == 0 {
		if d, ok := m["prompt_tokens_details"].(map[string]any); ok {
			cached = firstInt(d, "cached_tokens")
		}
	}
	if cached == 0 {
		cached = firstInt(m, "prompt_cache_hit_tokens")
	}
	out["input_tokens_details"] = map[string]any{"cached_tokens": cached}

	reasoning := 0
	if d, ok := m["output_tokens_details"].(map[string]any); ok {
		reasoning = firstInt(d, "reasoning_tokens")
	}
	if reasoning == 0 {
		if d, ok := m["completion_tokens_details"].(map[string]any); ok {
			reasoning = firstInt(d, "reasoning_tokens")
		}
	}
	out["output_tokens_details"] = map[string]any{"reasoning_tokens": reasoning}

	return out
}

func firstInt(m map[string]any, keys ...string) int {
	for _, k := range keys {
		if v, ok := asInt(m[k]); ok {
			return v
		}
	}
	return 0
}

func asInt(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case int32:
		return int(n), true
	case int64:
		return int(n), true
	case float64:
		return int(n), true
	case float32:
		return int(n), true
	case json.Number:
		i, err := n.Int64()
		if err != nil {
			return 0, false
		}
		return int(i), true
	default:
		return 0, false
	}
}

func cloneMap(m map[string]any) map[string]any {
	if m == nil {
		return map[string]any{}
	}
	b, err := json.Marshal(m)
	if err != nil {
		out := make(map[string]any, len(m))
		for k, v := range m {
			out[k] = v
		}
		return out
	}
	var out map[string]any
	if json.Unmarshal(b, &out) != nil {
		return map[string]any{}
	}
	return out
}

func supportsNativeResponses(baseURL string) bool {
	u := strings.ToLower(baseURL)
	return strings.Contains(u, "api.openai.com") ||
		strings.Contains(u, "openai.azure.com") ||
		strings.Contains(u, "chatgpt.com")
}

func (p *proxyServer) forwardRaw(w http.ResponseWriter, r *http.Request, prov Provider, path string, body []byte, stream bool) error {
	upstreamURL := joinOpenAIURL(prov.BaseURL, path)
	req, err := http.NewRequestWithContext(r.Context(), r.Method, upstreamURL, bytes.NewReader(body))
	if err != nil {
		return err
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
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == 404 {
		return fmt.Errorf("upstream 404")
	}
	for k, vv := range resp.Header {
		lk := strings.ToLower(k)
		if lk == "connection" || lk == "transfer-encoding" {
			continue
		}
		for _, v := range vv {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	if stream {
		flusher, ok := w.(http.Flusher)
		buf := make([]byte, 4096)
		for {
			n, readErr := resp.Body.Read(buf)
			if n > 0 {
				_, _ = w.Write(buf[:n])
				if ok {
					flusher.Flush()
				}
			}
			if readErr != nil {
				break
			}
		}
		return nil
	}
	_, _ = io.Copy(w, resp.Body)
	return nil
}

func forceStreamFalse(body []byte) []byte {
	var m map[string]any
	if json.Unmarshal(body, &m) != nil {
		return body
	}
	m["stream"] = false
	b, err := json.Marshal(m)
	if err != nil {
		return body
	}
	return b
}

// normalizeChatRole maps OpenAI-only / client-specific roles to values accepted by
// most OpenAI-compatible vendors (DeepSeek, Qwen, Moonshot, …):
// system | user | assistant | tool
//
// Codex frequently sends role=developer (OpenAI o1/GPT-5 style instructions).
func normalizeChatRole(role string) string {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "developer":
		return "system"
	case "system", "user", "assistant", "tool":
		return strings.ToLower(strings.TrimSpace(role))
	case "function":
		// legacy OpenAI function-calling role
		return "tool"
	case "latest_reminder":
		// some hosts accept this; keep for those that do
		return "latest_reminder"
	default:
		if role == "" {
			return "user"
		}
		// unknown roles → user to avoid hard 400s
		return "user"
	}
}

// normalizeUpstreamChatBody rewrites chat/completions payloads for third-party
// vendors that reject OpenAI-only roles / Responses-style tool schemas:
//   - role "developer" → "system"
//   - tools flat {type,name,parameters} → {type:"function", function:{name,...}}
//   - drop non-function host tools (web_search, file_search, …)
//   - tool_choice Responses shape → chat function shape
func normalizeUpstreamChatBody(body []byte) []byte {
	if len(body) == 0 {
		return body
	}
	var m map[string]any
	if json.Unmarshal(body, &m) != nil {
		return body
	}
	changed := false
	normalizeMsgList := func(key string) {
		arr, ok := m[key].([]any)
		if !ok {
			return
		}
		for i, item := range arr {
			msg, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if role, ok := msg["role"].(string); ok {
				if nr := normalizeChatRole(role); nr != role {
					msg["role"] = nr
					changed = true
				}
			}
			// assistant tool_calls may use flat name without function wrapper on some clients
			if tcs, ok := msg["tool_calls"].([]any); ok {
				ntcs, tcChanged := normalizeToolCallsForChat(tcs)
				if tcChanged {
					msg["tool_calls"] = ntcs
					changed = true
				}
			}
			arr[i] = msg
		}
		m[key] = arr
	}
	normalizeMsgList("messages")
	normalizeMsgList("input")

	if tools, ok := m["tools"].([]any); ok {
		nt, toolsChanged := normalizeToolsForChat(tools)
		if toolsChanged {
			if len(nt) == 0 {
				delete(m, "tools")
				// no callable tools left — neutralize tool_choice
				if _, has := m["tool_choice"]; has {
					m["tool_choice"] = "none"
				}
			} else {
				m["tools"] = nt
			}
			changed = true
		}
	}
	if tc, ok := m["tool_choice"]; ok {
		if ntc, tcChanged := normalizeToolChoiceForChat(tc); tcChanged {
			m["tool_choice"] = ntc
			changed = true
		}
	}

	if !changed {
		return body
	}
	out, err := json.Marshal(m)
	if err != nil {
		return body
	}
	return out
}

// normalizeToolsForChat converts Responses API / flat tool defs into classic
// Chat Completions shape required by DeepSeek and most OpenAI-compatible APIs:
//
//	{"type":"function","function":{"name":"...","description":"...","parameters":{...}}}
//
// Responses API often sends the flat form without nested "function":
//
//	{"type":"function","name":"...","description":"...","parameters":{...}}
func normalizeToolsForChat(tools []any) ([]any, bool) {
	if len(tools) == 0 {
		return tools, false
	}
	out := make([]any, 0, len(tools))
	changed := false
	for _, item := range tools {
		t, ok := item.(map[string]any)
		if !ok {
			// drop unparseable
			changed = true
			continue
		}
		// already classic chat form
		if fn, ok := t["function"].(map[string]any); ok {
			// ensure type
			nt := map[string]any{"type": "function", "function": fn}
			if typ, _ := t["type"].(string); typ != "" && typ != "function" {
				// non-function with nested function is unusual — keep if has name
			}
			// normalize parameters key: some use input_schema
			if _, has := fn["parameters"]; !has {
				if schema, ok := fn["input_schema"]; ok {
					fn["parameters"] = schema
					delete(fn, "input_schema")
					changed = true
				}
			}
			// also Parameters may live at top level in hybrid payloads
			if _, has := fn["parameters"]; !has {
				if schema, ok := t["parameters"]; ok {
					fn["parameters"] = schema
					changed = true
				} else if schema, ok := t["input_schema"]; ok {
					fn["parameters"] = schema
					changed = true
				}
			}
			nt["function"] = fn
			out = append(out, nt)
			// type missing?
			if typ, _ := t["type"].(string); typ == "" {
				changed = true
			}
			continue
		}

		typ, _ := t["type"].(string)
		typ = strings.ToLower(strings.TrimSpace(typ))

		// Flat function tool (Responses API style)
		name, _ := t["name"].(string)
		if typ == "function" || typ == "" || name != "" {
			// Only treat as function if we have a name (required)
			if strings.TrimSpace(name) == "" {
				// not a function tool — drop host tools like web_search without name in function form
				if typ != "" && typ != "function" {
					changed = true
					continue
				}
				// empty name, skip
				changed = true
				continue
			}
			// Skip known non-function host tools even if they have names
			if isNonFunctionHostTool(typ) {
				changed = true
				continue
			}
			fn := map[string]any{"name": name}
			if d, ok := t["description"]; ok {
				fn["description"] = d
			}
			if p, ok := t["parameters"]; ok {
				fn["parameters"] = p
			} else if p, ok := t["input_schema"]; ok {
				fn["parameters"] = p
			} else if p, ok := t["schema"]; ok {
				fn["parameters"] = p
			} else {
				// DeepSeek requires parameters object; empty object is fine
				fn["parameters"] = map[string]any{
					"type":       "object",
					"properties": map[string]any{},
				}
			}
			// strict / additionalProperties sometimes present — ignore
			out = append(out, map[string]any{
				"type":     "function",
				"function": fn,
			})
			changed = true
			continue
		}

		// Explicit non-function tools (web_search, file_search, computer, …)
		if isNonFunctionHostTool(typ) {
			changed = true
			continue
		}

		// Unknown shape — drop to avoid 400
		changed = true
	}
	if !changed && len(out) == len(tools) {
		return tools, false
	}
	return out, true
}

func isNonFunctionHostTool(typ string) bool {
	switch strings.ToLower(strings.TrimSpace(typ)) {
	case "web_search", "web_search_preview", "file_search", "code_interpreter",
		"computer", "computer_use", "computer_use_preview", "image_generation",
		"mcp", "custom":
		return true
	default:
		return false
	}
}

// normalizeToolChoiceForChat converts Responses-style tool_choice to chat form.
// Responses: {"type":"function","name":"foo"}
// Chat:     {"type":"function","function":{"name":"foo"}}
func normalizeToolChoiceForChat(tc any) (any, bool) {
	switch v := tc.(type) {
	case string:
		return v, false
	case map[string]any:
		// already chat form
		if fn, ok := v["function"].(map[string]any); ok {
			if _, hasName := fn["name"]; hasName {
				if typ, _ := v["type"].(string); typ == "" {
					nv := map[string]any{"type": "function", "function": fn}
					return nv, true
				}
				return v, false
			}
		}
		typ, _ := v["type"].(string)
		name, _ := v["name"].(string)
		if strings.EqualFold(typ, "function") || name != "" {
			if name == "" {
				// try nested
				if fn, ok := v["function"].(map[string]any); ok {
					name, _ = fn["name"].(string)
				}
			}
			if name == "" {
				return "auto", true
			}
			return map[string]any{
				"type": "function",
				"function": map[string]any{
					"name": name,
				},
			}, true
		}
		// type auto/none/required as object — uncommon
		if strings.EqualFold(typ, "auto") || strings.EqualFold(typ, "none") || strings.EqualFold(typ, "required") {
			return strings.ToLower(typ), true
		}
		return "auto", true
	default:
		return tc, false
	}
}

func normalizeToolCallsForChat(tcs []any) ([]any, bool) {
	out := make([]any, 0, len(tcs))
	changed := false
	for _, item := range tcs {
		tc, ok := item.(map[string]any)
		if !ok {
			out = append(out, item)
			continue
		}
		// classic: {id, type, function:{name, arguments}}
		if fn, ok := tc["function"].(map[string]any); ok {
			// ensure type
			if typ, _ := tc["type"].(string); typ == "" {
				tc["type"] = "function"
				changed = true
			}
			// arguments must be string for many vendors
			if args, ok := fn["arguments"]; ok {
				switch a := args.(type) {
				case string:
					// ok
				default:
					b, err := json.Marshal(a)
					if err == nil {
						fn["arguments"] = string(b)
						tc["function"] = fn
						changed = true
					}
				}
			}
			out = append(out, tc)
			continue
		}
		// flat: {id, type, name, arguments}
		name, _ := tc["name"].(string)
		if name != "" {
			fn := map[string]any{"name": name}
			if args, ok := tc["arguments"]; ok {
				switch a := args.(type) {
				case string:
					fn["arguments"] = a
				default:
					b, err := json.Marshal(a)
					if err == nil {
						fn["arguments"] = string(b)
					} else {
						fn["arguments"] = "{}"
					}
				}
			} else {
				fn["arguments"] = "{}"
			}
			ntc := map[string]any{
				"type":     "function",
				"function": fn,
			}
			if id, ok := tc["id"]; ok {
				ntc["id"] = id
			}
			out = append(out, ntc)
			changed = true
			continue
		}
		out = append(out, tc)
	}
	return out, changed
}

// responsesBodyToChat converts OpenAI Responses API JSON to Chat Completions JSON.
func responsesBodyToChat(body []byte) ([]byte, error) {
	var req map[string]any
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, err
	}
	model, _ := req["model"].(string)
	if strings.TrimSpace(model) == "" {
		return nil, fmt.Errorf("missing model")
	}

	messages := make([]map[string]any, 0, 8)

	// instructions → system
	if inst, ok := req["instructions"].(string); ok && strings.TrimSpace(inst) != "" {
		messages = append(messages, map[string]any{"role": "system", "content": inst})
	}

	// input: string | array of messages/items (Codex multi-turn + tool loop)
	switch in := req["input"].(type) {
	case string:
		if strings.TrimSpace(in) != "" {
			messages = append(messages, map[string]any{"role": "user", "content": in})
		}
	case []any:
		messages = append(messages, inputItemsToChatMessages(in)...)
	case map[string]any:
		messages = append(messages, inputItemsToChatMessages([]any{in})...)
	}

	// Some Codex payloads put messages under "messages" still
	if len(messages) == 0 {
		if arr, ok := req["messages"].([]any); ok {
			messages = append(messages, inputItemsToChatMessages(arr)...)
		}
	}

	if len(messages) == 0 {
		messages = append(messages, map[string]any{"role": "user", "content": ""})
	}

	// ensure all roles are vendor-safe before marshal
	for i := range messages {
		if role, ok := messages[i]["role"].(string); ok {
			messages[i]["role"] = normalizeChatRole(role)
		}
	}

	chat := map[string]any{
		"model":    model,
		"messages": messages,
	}
	// optional fields mapping
	if v, ok := req["temperature"]; ok {
		chat["temperature"] = v
	}
	if v, ok := req["top_p"]; ok {
		chat["top_p"] = v
	}
	if v, ok := req["stream"]; ok {
		chat["stream"] = v
	}
	// usage on last stream chunk helps debugging / clients
	if stream, _ := chat["stream"].(bool); stream {
		chat["stream_options"] = map[string]any{"include_usage": true}
	}
	if v, ok := req["max_output_tokens"]; ok {
		chat["max_tokens"] = v
	} else if v, ok := req["max_tokens"]; ok {
		chat["max_tokens"] = v
	}
	if v, ok := req["tools"]; ok {
		if arr, ok := v.([]any); ok {
			nt, _ := normalizeToolsForChat(arr)
			if len(nt) > 0 {
				chat["tools"] = nt
			}
		} else {
			chat["tools"] = v
		}
	}
	if v, ok := req["tool_choice"]; ok {
		ntc, _ := normalizeToolChoiceForChat(v)
		// only set if tools present (or choice is none/auto string)
		if _, hasTools := chat["tools"]; hasTools {
			chat["tool_choice"] = ntc
		} else if s, ok := ntc.(string); ok && (s == "none" || s == "auto") {
			// ignore
		}
	}
	// parallel_tool_calls is widely supported
	if v, ok := req["parallel_tool_calls"]; ok {
		chat["parallel_tool_calls"] = v
	}

	return json.Marshal(chat)
}

// inputItemsToChatMessages converts Responses API input items into chat messages.
// Handles Codex multi-turn tool loops:
//   message / input_text / function_call / function_call_output / reasoning(skip)
func inputItemsToChatMessages(items []any) []map[string]any {
	out := make([]map[string]any, 0, len(items))
	// pending assistant tool_calls to merge into one assistant message
	var pendingTCs []any

	flushToolCalls := func() {
		if len(pendingTCs) == 0 {
			return
		}
		out = append(out, map[string]any{
			"role":       "assistant",
			"content":    nil,
			"tool_calls": pendingTCs,
		})
		pendingTCs = nil
	}

	for _, item := range items {
		// plain string
		if s, ok := item.(string); ok {
			flushToolCalls()
			if strings.TrimSpace(s) != "" {
				out = append(out, map[string]any{"role": "user", "content": s})
			}
			continue
		}
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}

		typ, _ := m["type"].(string)
		typ = strings.ToLower(strings.TrimSpace(typ))

		switch typ {
		case "function_call", "custom_tool_call":
			// assistant tool call item → accumulate into tool_calls
			name, _ := m["name"].(string)
			args := ""
			switch a := m["arguments"].(type) {
			case string:
				args = a
			default:
				if a != nil {
					if b, err := json.Marshal(a); err == nil {
						args = string(b)
					}
				}
			}
			if args == "" {
				args = "{}"
			}
			callID, _ := m["call_id"].(string)
			if callID == "" {
				callID, _ = m["id"].(string)
			}
			if callID == "" {
				callID = fmt.Sprintf("call_%d", len(pendingTCs))
			}
			if name == "" {
				continue
			}
			pendingTCs = append(pendingTCs, map[string]any{
				"id":   callID,
				"type": "function",
				"function": map[string]any{
					"name":      name,
					"arguments": args,
				},
			})
			continue

		case "function_call_output", "custom_tool_call_output", "tool_result":
			flushToolCalls()
			callID, _ := m["call_id"].(string)
			if callID == "" {
				callID, _ = m["id"].(string)
			}
			content := ""
			switch o := m["output"].(type) {
			case string:
				content = o
			default:
				if o == nil {
					if c, ok := m["content"].(string); ok {
						content = c
					}
				} else if b, err := json.Marshal(o); err == nil {
					content = string(b)
				}
			}
			msg := map[string]any{
				"role":    "tool",
				"content": content,
			}
			if callID != "" {
				msg["tool_call_id"] = callID
			}
			out = append(out, msg)
			continue

		case "reasoning", "summary", "web_search_call", "file_search_call",
			"code_interpreter_call", "computer_call", "image_generation_call",
			"mcp_call", "mcp_list_tools", "mcp_approval_request":
			// not representable / not needed for chat vendors
			continue

		case "item_reference":
			continue
		}

		// Chat-shaped message with role (may also have type:"message")
		if role, ok := m["role"].(string); ok && role != "" {
			flushToolCalls()
			content := m["content"]
			if arr, ok := content.([]any); ok {
				content = partsToText(arr)
			}
			msg := map[string]any{
				"role":    normalizeChatRole(role),
				"content": content,
			}
			// pass through tool_calls if already present
			if tcs, ok := m["tool_calls"].([]any); ok && len(tcs) > 0 {
				ntcs, _ := normalizeToolCallsForChat(tcs)
				msg["tool_calls"] = ntcs
				if msg["content"] == "" {
					msg["content"] = nil
				}
			}
			// tool result via role=tool
			if normalizeChatRole(role) == "tool" {
				if id, ok := m["tool_call_id"].(string); ok {
					msg["tool_call_id"] = id
				} else if id, ok := m["call_id"].(string); ok {
					msg["tool_call_id"] = id
				}
			}
			out = append(out, msg)
			continue
		}

		// type message without going through role branch above
		if typ == "message" {
			flushToolCalls()
			role, _ := m["role"].(string)
			if role == "" {
				role = "user"
			}
			content := m["content"]
			if arr, ok := content.([]any); ok {
				content = partsToText(arr)
			}
			out = append(out, map[string]any{
				"role":    normalizeChatRole(role),
				"content": content,
			})
			continue
		}

		// {type:"input_text", text:"..."}
		if typ == "input_text" || typ == "text" || typ == "output_text" {
			flushToolCalls()
			if t, ok := m["text"].(string); ok && t != "" {
				role := "user"
				if typ == "output_text" {
					role = "assistant"
				}
				out = append(out, map[string]any{"role": role, "content": t})
			}
			continue
		}

		// fallback single-item conversion
		if msg := itemToChatMessage(m); msg != nil {
			flushToolCalls()
			out = append(out, msg)
		}
	}
	flushToolCalls()
	return out
}

func itemToChatMessage(item any) map[string]any {
	m, ok := item.(map[string]any)
	if !ok {
		if s, ok := item.(string); ok {
			return map[string]any{"role": "user", "content": s}
		}
		return nil
	}
	// Already a chat message
	if role, ok := m["role"].(string); ok {
		content := m["content"]
		// content may be array of parts
		if arr, ok := content.([]any); ok {
			content = partsToText(arr)
		}
		msg := map[string]any{"role": normalizeChatRole(role), "content": content}
		if tcs, ok := m["tool_calls"].([]any); ok && len(tcs) > 0 {
			ntcs, _ := normalizeToolCallsForChat(tcs)
			msg["tool_calls"] = ntcs
		}
		if normalizeChatRole(role) == "tool" {
			if id, ok := m["tool_call_id"].(string); ok {
				msg["tool_call_id"] = id
			}
		}
		return msg
	}
	// Responses item: {type:"message", role, content:[...]}
	if typ, _ := m["type"].(string); typ == "message" || typ == "" {
		role, _ := m["role"].(string)
		if role == "" {
			role = "user"
		}
		content := m["content"]
		if arr, ok := content.([]any); ok {
			content = partsToText(arr)
		}
		return map[string]any{"role": normalizeChatRole(role), "content": content}
	}
	// {type:"input_text", text:"..."}
	if typ, _ := m["type"].(string); typ == "input_text" || typ == "text" {
		if t, ok := m["text"].(string); ok {
			return map[string]any{"role": "user", "content": t}
		}
	}
	return nil
}

func partsToText(parts []any) string {
	var b strings.Builder
	for _, p := range parts {
		m, ok := p.(map[string]any)
		if !ok {
			continue
		}
		typ, _ := m["type"].(string)
		switch typ {
		case "input_text", "output_text", "text":
			if t, ok := m["text"].(string); ok {
				b.WriteString(t)
			}
		default:
			if t, ok := m["text"].(string); ok {
				b.WriteString(t)
			}
		}
	}
	return b.String()
}

// chatBodyToResponses wraps a chat.completion JSON as a responses-style object.
func chatBodyToResponses(chatBody []byte, model string) ([]byte, error) {
	var chat map[string]any
	if err := json.Unmarshal(chatBody, &chat); err != nil {
		return nil, err
	}

	text := ""
	var toolCalls []any
	// choices[0].message.content / tool_calls
	if choices, ok := chat["choices"].([]any); ok && len(choices) > 0 {
		if c0, ok := choices[0].(map[string]any); ok {
			if msg, ok := c0["message"].(map[string]any); ok {
				switch c := msg["content"].(type) {
				case string:
					text = c
				case []any:
					text = partsToText(c)
				}
				if tcs, ok := msg["tool_calls"].([]any); ok {
					toolCalls = tcs
				}
			}
			// some return text field
			if text == "" {
				if t, ok := c0["text"].(string); ok {
					text = t
				}
			}
		}
	}

	id, _ := chat["id"].(string)
	if id == "" {
		id = fmt.Sprintf("resp_%d", time.Now().UnixNano())
	} else {
		// normalize chatcmpl-xxx → resp-ish
		id = strings.Replace(id, "chatcmpl-", "resp_", 1)
	}

	outputs := make([]any, 0, 1+len(toolCalls))
	if text != "" || len(toolCalls) == 0 {
		outputs = append(outputs, map[string]any{
			"type":   "message",
			"id":     id + "_msg",
			"role":   "assistant",
			"status": "completed",
			"content": []map[string]any{
				{"type": "output_text", "text": text},
			},
		})
	}
	for i, raw := range toolCalls {
		tc, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		name, args, callID := "", "{}", ""
		if id0, _ := tc["id"].(string); id0 != "" {
			callID = id0
		}
		if fn, ok := tc["function"].(map[string]any); ok {
			name, _ = fn["name"].(string)
			switch a := fn["arguments"].(type) {
			case string:
				args = a
			default:
				if a != nil {
					if b, err := json.Marshal(a); err == nil {
						args = string(b)
					}
				}
			}
		}
		if callID == "" {
			callID = fmt.Sprintf("call_%d", i)
		}
		if name == "" {
			continue
		}
		outputs = append(outputs, map[string]any{
			"type":      "function_call",
			"id":        "fc_" + callID,
			"call_id":   callID,
			"name":      name,
			"arguments": args,
			"status":    "completed",
		})
	}

	out := buildResponsesObject(id, model, time.Now().Unix(), "completed", text, outputs)
	if u, ok := chat["usage"]; ok {
		out["usage"] = normalizeUsageToResponses(u)
	} else {
		out["usage"] = normalizeUsageToResponses(nil)
	}
	return json.Marshal(out)
}
