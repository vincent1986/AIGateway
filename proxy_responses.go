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

func (p *proxyServer) handleResponses(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
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
		writeJSON(w, 400, map[string]any{"error": map[string]any{"message": routeErr.Error()}})
		return
	}

	fmtName := ResolveAPIFormat(prov)
	stream := isStreamRequest(body)
	p.logf("POST responses → %s model=%s format=%s", prov.Name, model, fmtName)

	if fmtName == APIFormatAnthropicMessages {
		p.forwardAnthropicFromOpenAI(w, r, prov, body, stream, true)
		return
	}
	if fmtName == APIFormatOpenAIResponses {
		body = sanitizeResponsesRequestForProvider(body, prov)
		p.forwardNativeResponses(w, r, prov, body, stream)
		return
	}

	// OpenAI-compatible: convert responses → chat → wrap back
	chatBody, err := responsesBodyToChat(body)
	if err != nil {
		writeJSON(w, 400, map[string]any{"error": map[string]any{"message": err.Error()}})
		return
	}
	if stream {
		chatBody = enableChatStreamUsage(chatBody)
	} else {
		chatBody = forceStreamFalse(chatBody)
	}
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
	if stream {
		req.Header.Set("Accept", "text/event-stream")
	}
	client := p.client
	if !stream {
		client = &http.Client{Timeout: 180 * time.Second, Transport: p.client.Transport}
	}
	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		p.recordRequest(ProxyRequestLog{Method: r.Method, Path: "responses->chat/completions", Provider: prov.Name, Model: model, Format: APIFormatOpenAIChat, Status: 502, LatencyMs: time.Since(start).Milliseconds(), Error: err.Error()})
		writeJSON(w, 502, map[string]any{"error": map[string]any{"message": err.Error()}})
		return
	}
	defer resp.Body.Close()
	if stream && resp.StatusCode >= 200 && resp.StatusCode < 300 {
		log := p.streamChatAsResponses(w, resp.Body, model)
		log.Method, log.Path, log.Provider, log.Model, log.Format, log.Status = r.Method, "responses->chat/completions", prov.Name, model, APIFormatOpenAIChat, resp.StatusCode
		log.LatencyMs = time.Since(start).Milliseconds()
		p.recordRequest(log)
		return
	}
	upBody, _ := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	log := ProxyRequestLog{Method: r.Method, Path: "responses->chat/completions", Provider: prov.Name, Model: model, Format: APIFormatOpenAIChat, Status: resp.StatusCode, LatencyMs: time.Since(start).Milliseconds()}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		p.recordRequest(log)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(resp.StatusCode)
		_, _ = w.Write(upBody)
		return
	}
	out, err := chatBodyToResponses(upBody, model)
	if err != nil {
		log.Error = err.Error()
		p.recordRequest(log)
		writeJSON(w, 502, map[string]any{"error": map[string]any{"message": err.Error()}})
		return
	}
	log.PromptTokens, log.CompletionTokens, log.TotalTokens = usageFromJSONBody(out)
	p.recordRequest(log)
	writeResponsesResult(w, out, stream)
}

func (p *proxyServer) forwardNativeResponses(w http.ResponseWriter, r *http.Request, prov Provider, body []byte, stream bool) {
	upstreamURL := joinOpenAIURL(prov.BaseURL, "responses")
	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, upstreamURL, bytes.NewReader(body))
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
	start := time.Now()
	resp, err := p.client.Do(req)
	if err != nil {
		p.recordRequest(ProxyRequestLog{Method: r.Method, Path: "responses", Provider: prov.Name, Model: extractModel(body, r), Format: APIFormatOpenAIResponses, Status: 502, LatencyMs: time.Since(start).Milliseconds(), Error: err.Error()})
		writeJSON(w, 502, map[string]any{"error": map[string]any{"message": "upstream: " + err.Error()}})
		return
	}
	defer resp.Body.Close()
	if !stream {
		upBody, readErr := io.ReadAll(resp.Body)
		log := ProxyRequestLog{Method: r.Method, Path: "responses", Provider: prov.Name, Model: extractModel(body, r), Format: APIFormatOpenAIResponses, Status: resp.StatusCode, LatencyMs: time.Since(start).Milliseconds()}
		log.PromptTokens, log.CompletionTokens, log.TotalTokens = usageFromJSONBody(upBody)
		if readErr != nil {
			log.Error = readErr.Error()
		}
		p.recordRequest(log)
		copyHeaders(w, resp)
		w.WriteHeader(resp.StatusCode)
		_, _ = w.Write(upBody)
		return
	}
	copyHeaders(w, resp)
	w.WriteHeader(resp.StatusCode)
	prompt, completion, total, streamErr := relaySSE(w, resp.Body)
	log := ProxyRequestLog{Method: r.Method, Path: "responses", Provider: prov.Name, Model: extractModel(body, r), Format: APIFormatOpenAIResponses, Status: resp.StatusCode, LatencyMs: time.Since(start).Milliseconds(), PromptTokens: prompt, CompletionTokens: completion, TotalTokens: total}
	if streamErr != nil {
		log.Error = streamErr.Error()
	}
	p.recordRequest(log)
}

func enableChatStreamUsage(body []byte) []byte {
	var req map[string]any
	if json.Unmarshal(body, &req) != nil {
		return body
	}
	req["stream"] = true
	req["stream_options"] = map[string]any{"include_usage": true}
	out, err := json.Marshal(req)
	if err != nil {
		return body
	}
	return out
}

type streamedToolCall struct{ ID, Name, Arguments string }

func (p *proxyServer) streamChatAsResponses(w http.ResponseWriter, body io.Reader, model string) ProxyRequestLog {
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	responseID := fmt.Sprintf("resp_%d", time.Now().UnixNano())
	created := map[string]any{"id": responseID, "object": "response", "created_at": time.Now().Unix(), "status": "in_progress", "model": model, "output": []any{}}
	writeSSEEvent(w, "response.created", map[string]any{"type": "response.created", "response": created})
	message := map[string]any{"id": "msg_" + responseID, "type": "message", "role": "assistant", "status": "in_progress", "content": []any{}}
	writeSSEEvent(w, "response.output_item.added", map[string]any{"type": "response.output_item.added", "output_index": 0, "item": message})
	writeSSEEvent(w, "response.content_part.added", map[string]any{"type": "response.content_part.added", "output_index": 0, "content_index": 0, "part": map[string]any{"type": "output_text", "text": ""}})
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 64<<10), 4<<20)
	textValue := ""
	tools := map[int]*streamedToolCall{}
	prompt, completion, total := 0, 0, 0
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
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
		if p0, c0, t0 := usageFromMap(chunk); p0 > 0 || c0 > 0 || t0 > 0 {
			prompt, completion, total = p0, c0, t0
		}
		choices, _ := chunk["choices"].([]any)
		if len(choices) == 0 {
			continue
		}
		choice, _ := choices[0].(map[string]any)
		delta, _ := choice["delta"].(map[string]any)
		if value, _ := delta["content"].(string); value != "" {
			textValue += value
			writeSSEEvent(w, "response.output_text.delta", map[string]any{"type": "response.output_text.delta", "output_index": 0, "content_index": 0, "delta": value})
		}
		if calls, ok := delta["tool_calls"].([]any); ok {
			for _, raw := range calls {
				call, _ := raw.(map[string]any)
				index := intFromAny(call["index"])
				state := tools[index]
				if state == nil {
					state = &streamedToolCall{}
					tools[index] = state
					writeSSEEvent(w, "response.output_item.added", map[string]any{"type": "response.output_item.added", "output_index": index + 1, "item": map[string]any{"type": "function_call", "status": "in_progress"}})
				}
				if id, _ := call["id"].(string); id != "" {
					state.ID = id
				}
				fn, _ := call["function"].(map[string]any)
				if name, _ := fn["name"].(string); name != "" {
					state.Name = name
				}
				if args, _ := fn["arguments"].(string); args != "" {
					state.Arguments += args
					writeSSEEvent(w, "response.function_call_arguments.delta", map[string]any{"type": "response.function_call_arguments.delta", "output_index": index + 1, "delta": args})
				}
			}
		}
	}
	output := make([]any, 0, 1+len(tools))
	item := map[string]any{"id": "msg_" + responseID, "type": "message", "role": "assistant", "status": "completed", "content": []any{map[string]any{"type": "output_text", "text": textValue}}}
	writeSSEEvent(w, "response.output_text.done", map[string]any{"type": "response.output_text.done", "output_index": 0, "content_index": 0, "text": textValue})
	writeSSEEvent(w, "response.output_item.done", map[string]any{"type": "response.output_item.done", "output_index": 0, "item": item})
	output = append(output, item)
	for index := 0; index < len(tools); index++ {
		if call := tools[index]; call != nil {
			item := map[string]any{"type": "function_call", "id": call.ID, "call_id": call.ID, "name": call.Name, "arguments": call.Arguments, "status": "completed"}
			writeSSEEvent(w, "response.function_call_arguments.done", map[string]any{"type": "response.function_call_arguments.done", "output_index": index + 1, "arguments": call.Arguments})
			writeSSEEvent(w, "response.output_item.done", map[string]any{"type": "response.output_item.done", "output_index": index + 1, "item": item})
			output = append(output, item)
		}
	}
	completed := cloneMap(created)
	completed["status"] = "completed"
	completed["output"] = output
	completed["usage"] = map[string]any{"input_tokens": prompt, "output_tokens": completion, "total_tokens": total}
	writeSSEEvent(w, "response.completed", map[string]any{"type": "response.completed", "response": completed})
	log := ProxyRequestLog{PromptTokens: prompt, CompletionTokens: completion, TotalTokens: total}
	if err := scanner.Err(); err != nil {
		log.Error = err.Error()
	}
	return log
}

func sanitizeResponsesRequestForProvider(body []byte, p Provider) []byte {
	var req map[string]any
	if json.Unmarshal(body, &req) != nil {
		return body
	}
	if shouldDisableWebSearch(p, req) {
		req["tools"] = removeHostedTool(req["tools"], "web_search")
		req["web_search"] = "disabled"
	}
	req["input"] = downgradeUnsupportedInputParts(req["input"])
	b, err := json.Marshal(req)
	if err != nil {
		return body
	}
	return b
}

func shouldDisableWebSearch(p Provider, req map[string]any) bool {
	hostNeedles := []string{"xiaomimimo.com", "longcat.chat", "minimax.io", "minimaxi.com"}
	modelNeedles := []string{"mimo", "longcat", "minimax", "qwen3-coder"}
	base := strings.ToLower(p.BaseURL)
	for _, h := range hostNeedles {
		if strings.Contains(base, h) {
			return true
		}
	}
	model, _ := req["model"].(string)
	model = strings.ToLower(model)
	if slash := strings.LastIndex(model, "/"); slash >= 0 {
		model = model[slash+1:]
	}
	for _, prefix := range modelNeedles {
		if strings.HasPrefix(model, prefix) {
			return true
		}
	}
	return false
}

func removeHostedTool(raw any, toolType string) any {
	arr, ok := raw.([]any)
	if !ok {
		return raw
	}
	out := make([]any, 0, len(arr))
	for _, item := range arr {
		m, ok := item.(map[string]any)
		if ok && m["type"] == toolType {
			continue
		}
		out = append(out, item)
	}
	return out
}

func downgradeUnsupportedInputParts(raw any) any {
	arr, ok := raw.([]any)
	if !ok {
		return raw
	}
	for _, item := range arr {
		msg, ok := item.(map[string]any)
		if !ok {
			continue
		}
		parts, ok := msg["content"].([]any)
		if !ok {
			continue
		}
		var textParts []any
		for _, partRaw := range parts {
			part, ok := partRaw.(map[string]any)
			if !ok {
				continue
			}
			typ, _ := part["type"].(string)
			switch typ {
			case "input_text", "output_text", "text":
				textParts = append(textParts, part)
			case "input_image", "image_url":
				textParts = append(textParts, map[string]any{"type": "input_text", "text": "[image attachment omitted for upstream compatibility]"})
			case "input_file", "file", "input_audio", "audio":
				textParts = append(textParts, map[string]any{"type": "input_text", "text": "[" + typ + " attachment omitted for upstream compatibility]"})
			default:
				textParts = append(textParts, part)
			}
		}
		msg["content"] = textParts
	}
	return arr
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

func responsesBodyToChat(body []byte) ([]byte, error) {
	var req map[string]any
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, err
	}
	model, _ := req["model"].(string)
	if model == "" {
		return nil, fmt.Errorf("missing model")
	}
	messages := make([]map[string]any, 0, 8)
	if inst, ok := req["instructions"].(string); ok && inst != "" {
		messages = append(messages, map[string]any{"role": "system", "content": inst})
	}
	switch in := req["input"].(type) {
	case string:
		messages = append(messages, map[string]any{"role": "user", "content": in})
	case []any:
		for _, item := range in {
			if msg := itemToChatMessage(item); msg != nil {
				messages = append(messages, msg)
			}
		}
	}
	if len(messages) == 0 {
		if arr, ok := req["messages"].([]any); ok {
			for _, item := range arr {
				if msg := itemToChatMessage(item); msg != nil {
					messages = append(messages, msg)
				}
			}
		}
	}
	if len(messages) == 0 {
		messages = append(messages, map[string]any{"role": "user", "content": ""})
	}
	chat := map[string]any{"model": model, "messages": messages}
	if v, ok := req["temperature"]; ok {
		chat["temperature"] = v
	}
	if v, ok := req["max_output_tokens"]; ok {
		chat["max_tokens"] = v
	}
	if tools, ok := req["tools"].([]any); ok {
		chat["tools"] = responsesToolsToChat(tools)
	}
	if choice, ok := req["tool_choice"]; ok {
		chat["tool_choice"] = choice
	}
	return json.Marshal(chat)
}

func responsesToolsToChat(tools []any) []map[string]any {
	out := make([]map[string]any, 0, len(tools))
	for _, item := range tools {
		tool, ok := item.(map[string]any)
		if !ok || tool["type"] != "function" {
			continue
		}
		out = append(out, map[string]any{
			"type": "function",
			"function": map[string]any{
				"name": tool["name"], "description": tool["description"], "parameters": tool["parameters"],
			},
		})
	}
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
	if role, ok := m["role"].(string); ok {
		content := m["content"]
		if arr, ok := content.([]any); ok {
			content = partsToText(arr)
		}
		return map[string]any{"role": role, "content": content}
	}
	if typ, _ := m["type"].(string); typ == "function_call" {
		return map[string]any{
			"role": "assistant", "content": nil,
			"tool_calls": []map[string]any{{
				"id": m["call_id"], "type": "function",
				"function": map[string]any{"name": m["name"], "arguments": m["arguments"]},
			}},
		}
	} else if typ == "function_call_output" {
		return map[string]any{"role": "tool", "tool_call_id": m["call_id"], "content": m["output"]}
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
		if t, ok := m["text"].(string); ok {
			b.WriteString(t)
		}
	}
	return b.String()
}

func chatBodyToResponses(chatBody []byte, model string) ([]byte, error) {
	var chat map[string]any
	if err := json.Unmarshal(chatBody, &chat); err != nil {
		return nil, err
	}
	text := ""
	var toolCalls []any
	if choices, ok := chat["choices"].([]any); ok && len(choices) > 0 {
		if c0, ok := choices[0].(map[string]any); ok {
			if msg, ok := c0["message"].(map[string]any); ok {
				if s, ok := msg["content"].(string); ok {
					text = s
				}
				toolCalls, _ = msg["tool_calls"].([]any)
			}
		}
	}
	id, _ := chat["id"].(string)
	if id == "" {
		id = fmt.Sprintf("resp_%d", time.Now().UnixNano())
	} else {
		id = strings.Replace(id, "chatcmpl-", "resp_", 1)
	}
	output := make([]map[string]any, 0, 1+len(toolCalls))
	if text != "" || len(toolCalls) == 0 {
		output = append(output, map[string]any{
			"type": "message", "role": "assistant", "status": "completed",
			"content": []map[string]any{{"type": "output_text", "text": text}},
		})
	}
	for _, raw := range toolCalls {
		call, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		fn, _ := call["function"].(map[string]any)
		output = append(output, map[string]any{
			"type": "function_call", "id": call["id"], "call_id": call["id"],
			"name": fn["name"], "arguments": fn["arguments"], "status": "completed",
		})
	}
	out := map[string]any{
		"id": id, "object": "response", "created_at": time.Now().Unix(),
		"status": "completed", "model": model, "output_text": text,
		"output": output,
	}
	if usage, ok := chat["usage"]; ok {
		out["usage"] = usage
	}
	return json.Marshal(out)
}

func writeResponsesResult(w http.ResponseWriter, body []byte, stream bool) {
	if !stream {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	var response map[string]any
	if json.Unmarshal(body, &response) != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": map[string]any{"message": "invalid converted response"}})
		return
	}
	created := cloneMap(response)
	created["status"] = "in_progress"
	created["output"] = []any{}
	writeSSEEvent(w, "response.created", map[string]any{"type": "response.created", "response": created})
	if output, ok := response["output"].([]any); ok {
		for i, raw := range output {
			item, _ := raw.(map[string]any)
			writeSSEEvent(w, "response.output_item.added", map[string]any{"type": "response.output_item.added", "output_index": i, "item": item})
			if item["type"] == "message" {
				if content, ok := item["content"].([]any); ok {
					for j, rawPart := range content {
						part, _ := rawPart.(map[string]any)
						writeSSEEvent(w, "response.content_part.added", map[string]any{"type": "response.content_part.added", "output_index": i, "content_index": j, "part": part})
						if delta, _ := part["text"].(string); delta != "" {
							writeSSEEvent(w, "response.output_text.delta", map[string]any{"type": "response.output_text.delta", "output_index": i, "content_index": j, "delta": delta})
						}
						writeSSEEvent(w, "response.output_text.done", map[string]any{"type": "response.output_text.done", "output_index": i, "content_index": j, "text": part["text"]})
						writeSSEEvent(w, "response.content_part.done", map[string]any{"type": "response.content_part.done", "output_index": i, "content_index": j, "part": part})
					}
				}
			} else if item["type"] == "function_call" {
				if delta, _ := item["arguments"].(string); delta != "" {
					writeSSEEvent(w, "response.function_call_arguments.delta", map[string]any{"type": "response.function_call_arguments.delta", "output_index": i, "delta": delta})
				}
				writeSSEEvent(w, "response.function_call_arguments.done", map[string]any{"type": "response.function_call_arguments.done", "output_index": i, "arguments": item["arguments"]})
			}
			writeSSEEvent(w, "response.output_item.done", map[string]any{"type": "response.output_item.done", "output_index": i, "item": item})
		}
	}
	writeSSEEvent(w, "response.completed", map[string]any{"type": "response.completed", "response": response})
}

func cloneMap(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func writeSSEEvent(w http.ResponseWriter, event string, payload any) {
	b, _ := json.Marshal(payload)
	_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, b)
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
}
