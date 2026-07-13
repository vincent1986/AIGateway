package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
)

// handleAnthropicMessages provides the Anthropic-compatible endpoint used by
// Claude Code. The gateway keeps one internal OpenAI routing path and adapts
// only the wire format at the boundary.
func (p *proxyServer) handleAnthropicMessages(w http.ResponseWriter, r *http.Request) {
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
		writeJSON(w, http.StatusBadRequest, anthropicError("read request: "+err.Error()))
		return
	}
	openAI, stream, err := anthropicRequestToOpenAI(body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, anthropicError(err.Error()))
		return
	}

	inner := httptest.NewRequestWithContext(r.Context(), http.MethodPost, "/v1/chat/completions", bytes.NewReader(openAI))
	inner.Header = r.Header.Clone()
	inner.Header.Set("Content-Type", "application/json")
	inner.Header.Set("Accept", "text/event-stream")
	rr := httptest.NewRecorder()
	p.handleChatCompletions(rr, inner)
	if rr.Code >= 400 {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(rr.Code)
		_, _ = w.Write(rr.Body.Bytes())
		return
	}

	if stream {
		out, err := openAIStreamToAnthropic(rr.Body.Bytes())
		if err != nil {
			writeJSON(w, http.StatusBadGateway, anthropicError(err.Error()))
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(out)
		return
	}
	out, err := openAIResponseToAnthropic(rr.Body.Bytes())
	if err != nil {
		writeJSON(w, http.StatusBadGateway, anthropicError(err.Error()))
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(out)
}

func anthropicRequestToOpenAI(body []byte) ([]byte, bool, error) {
	var in map[string]any
	if err := json.Unmarshal(body, &in); err != nil {
		return nil, false, fmt.Errorf("invalid Anthropic JSON: %w", err)
	}
	out := map[string]any{
		"model":    strings.TrimSpace(fmt.Sprint(in["model"])),
		"messages": []any{},
	}
	if out["model"] == "" {
		return nil, false, fmt.Errorf("missing model")
	}
	messages := out["messages"].([]any)
	if system := anthropicText(in["system"]); system != "" {
		messages = append(messages, map[string]any{"role": "system", "content": system})
	}
	if raw, ok := in["messages"].([]any); ok {
		for _, item := range raw {
			m, ok := item.(map[string]any)
			if !ok {
				continue
			}
			role := fmt.Sprint(m["role"])
			content, toolCalls, toolResults := anthropicContent(m["content"])
			if role == "assistant" && len(toolCalls) > 0 {
				messages = append(messages, map[string]any{"role": "assistant", "content": content, "tool_calls": toolCalls})
			} else if role == "user" && len(toolResults) > 0 {
				messages = append(messages, toolResults...)
			} else {
				messages = append(messages, map[string]any{"role": role, "content": content})
			}
		}
	}
	out["messages"] = messages
	for _, key := range []string{"max_tokens", "temperature", "top_p", "stream"} {
		if value, ok := in[key]; ok {
			out[key] = value
		}
	}
	if tools, ok := in["tools"].([]any); ok {
		converted := make([]any, 0, len(tools))
		for _, raw := range tools {
			tool, _ := raw.(map[string]any)
			converted = append(converted, map[string]any{
				"type": "function",
				"function": map[string]any{
					"name": tool["name"], "description": tool["description"], "parameters": tool["input_schema"],
				},
			})
		}
		out["tools"] = converted
	}
	encoded, err := json.Marshal(out)
	return encoded, in["stream"] == true, err
}

func anthropicContent(value any) (string, []any, []any) {
	if text, ok := value.(string); ok {
		return text, nil, nil
	}
	blocks, _ := value.([]any)
	var texts []string
	var calls []any
	var results []any
	for _, raw := range blocks {
		block, _ := raw.(map[string]any)
		switch block["type"] {
		case "text":
			texts = append(texts, fmt.Sprint(block["text"]))
		case "tool_use":
			input, _ := json.Marshal(block["input"])
			calls = append(calls, map[string]any{"id": block["id"], "type": "function", "function": map[string]any{"name": block["name"], "arguments": string(input)}})
		case "tool_result":
			results = append(results, map[string]any{"role": "tool", "tool_call_id": block["tool_use_id"], "content": anthropicText(block["content"])})
		}
	}
	return strings.Join(texts, "\n"), calls, results
}

func anthropicText(value any) string {
	if text, ok := value.(string); ok {
		return text
	}
	if blocks, ok := value.([]any); ok {
		var out []string
		for _, block := range blocks {
			if m, ok := block.(map[string]any); ok && m["type"] == "text" {
				out = append(out, fmt.Sprint(m["text"]))
			}
		}
		return strings.Join(out, "\n")
	}
	return ""
}

func openAIResponseToAnthropic(body []byte) ([]byte, error) {
	var in map[string]any
	if err := json.Unmarshal(body, &in); err != nil {
		return nil, fmt.Errorf("invalid OpenAI response: %w", err)
	}
	result := map[string]any{
		"id": "msg_" + fmt.Sprint(in["id"]), "type": "message", "role": "assistant",
		"model": in["model"], "stop_reason": "end_turn", "content": []any{},
	}
	if choices, _ := in["choices"].([]any); len(choices) > 0 {
		choice, _ := choices[0].(map[string]any)
		message, _ := choice["message"].(map[string]any)
		if text := fmt.Sprint(message["content"]); text != "<nil>" && text != "" {
			result["content"] = []any{map[string]any{"type": "text", "text": text}}
		}
		if calls, _ := message["tool_calls"].([]any); len(calls) > 0 {
			content := result["content"].([]any)
			for _, raw := range calls {
				call, _ := raw.(map[string]any)
				fn, _ := call["function"].(map[string]any)
				var input any
				_ = json.Unmarshal([]byte(fmt.Sprint(fn["arguments"])), &input)
				content = append(content, map[string]any{"type": "tool_use", "id": call["id"], "name": fn["name"], "input": input})
			}
			result["content"] = content
		}
	}
	if usage, ok := in["usage"].(map[string]any); ok {
		result["usage"] = map[string]any{"input_tokens": usage["prompt_tokens"], "output_tokens": usage["completion_tokens"]}
	}
	return json.Marshal(result)
}

func openAIStreamToAnthropic(body []byte) ([]byte, error) {
	var text strings.Builder
	var model string
	var id = "msg_stream"
	for _, line := range strings.Split(string(body), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "data:") || strings.TrimSpace(strings.TrimPrefix(line, "data:")) == "[DONE]" {
			continue
		}
		var chunk map[string]any
		if json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(line, "data:"))), &chunk) != nil {
			continue
		}
		if v := fmt.Sprint(chunk["id"]); v != "<nil>" && v != "" {
			id = "msg_" + v
		}
		if v := fmt.Sprint(chunk["model"]); v != "<nil>" && v != "" {
			model = v
		}
		choices, _ := chunk["choices"].([]any)
		if len(choices) > 0 {
			delta, _ := choices[0].(map[string]any)
			message, _ := delta["delta"].(map[string]any)
			if v, ok := message["content"].(string); ok {
				text.WriteString(v)
			}
		}
	}
	result, err := json.Marshal(map[string]any{"id": id, "type": "message", "role": "assistant", "model": model, "stop_reason": "end_turn", "content": []any{map[string]any{"type": "text", "text": text.String()}}})
	if err != nil {
		return nil, err
	}
	// Emit a valid Anthropic event sequence after the upstream stream closes.
	var out bytes.Buffer
	for _, event := range []map[string]any{
		{"type": "message_start", "message": map[string]any{"id": id, "type": "message", "role": "assistant", "model": model, "content": []any{}, "stop_reason": nil}},
		{"type": "content_block_start", "index": 0, "content_block": map[string]any{"type": "text", "text": ""}},
		{"type": "content_block_delta", "index": 0, "delta": map[string]any{"type": "text_delta", "text": text.String()}},
		{"type": "content_block_stop", "index": 0},
		{"type": "message_delta", "delta": map[string]any{"stop_reason": "end_turn"}},
		{"type": "message_stop"},
	} {
		b, _ := json.Marshal(event)
		out.WriteString("event: ")
		out.WriteString(fmt.Sprint(event["type"]))
		out.WriteString("\ndata: ")
		out.Write(b)
		out.WriteString("\n\n")
	}
	_ = result
	return out.Bytes(), nil
}

func anthropicError(message string) map[string]any {
	return map[string]any{"type": "error", "error": map[string]any{"type": "invalid_request_error", "message": message}}
}
