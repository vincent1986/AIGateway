package main

import (
	"encoding/json"
	"fmt"
	"strings"
)

// convertOpenAIChatToAnthropic converts standard OpenAI chat.completions body
// into Anthropic Messages API body.
func convertOpenAIChatToAnthropic(body []byte) ([]byte, error) {
	var req map[string]any
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, err
	}
	model, _ := req["model"].(string)
	if model == "" {
		return nil, fmt.Errorf("missing model")
	}

	var systemParts []string
	messages := make([]map[string]any, 0, 8)

	if arr, ok := req["messages"].([]any); ok {
		for _, it := range arr {
			m, ok := it.(map[string]any)
			if !ok {
				continue
			}
			role, _ := m["role"].(string)
			switch role {
			case "system", "developer":
				content := normalizeContent(m["content"])
				if s, ok := content.(string); ok && s != "" {
					systemParts = append(systemParts, s)
				}
			case "user":
				messages = append(messages, map[string]any{
					"role":    role,
					"content": normalizeContent(m["content"]),
				})
			case "assistant":
				content := openAIAssistantContentToAnthropic(m)
				messages = append(messages, map[string]any{"role": role, "content": content})
			case "tool":
				toolID, _ := m["tool_call_id"].(string)
				if toolID == "" {
					return nil, fmt.Errorf("tool message missing tool_call_id")
				}
				messages = append(messages, map[string]any{
					"role": "user",
					"content": []map[string]any{{
						"type":        "tool_result",
						"tool_use_id": toolID,
						"content":     normalizeContent(m["content"]),
					}},
				})
			}
		}
	}

	if len(messages) == 0 {
		messages = append(messages, map[string]any{"role": "user", "content": ""})
	}

	out := map[string]any{
		"model":    model,
		"messages": messages,
	}
	// Anthropic requires max_tokens
	maxTokens := 4096
	if v, ok := req["max_tokens"].(float64); ok && v > 0 {
		maxTokens = int(v)
	} else if v, ok := req["max_completion_tokens"].(float64); ok && v > 0 {
		maxTokens = int(v)
	}
	out["max_tokens"] = maxTokens

	if len(systemParts) > 0 {
		out["system"] = strings.Join(systemParts, "\n\n")
	}
	if v, ok := req["temperature"]; ok {
		out["temperature"] = v
	}
	if v, ok := req["top_p"]; ok {
		out["top_p"] = v
	}
	if v, ok := req["stream"]; ok {
		out["stream"] = v
	}
	if v, ok := req["stop"]; ok {
		out["stop_sequences"] = v
	}
	// tools mapping is best-effort (OpenAI tools → Anthropic tools)
	if tools, ok := req["tools"].([]any); ok && len(tools) > 0 {
		out["tools"] = openaiToolsToAnthropic(tools)
	}

	return json.Marshal(out)
}

func openAIAssistantContentToAnthropic(message map[string]any) any {
	parts := make([]map[string]any, 0, 4)
	if text, ok := normalizeContent(message["content"]).(string); ok && text != "" {
		parts = append(parts, map[string]any{"type": "text", "text": text})
	}
	if calls, ok := message["tool_calls"].([]any); ok {
		for _, call := range calls {
			cm, ok := call.(map[string]any)
			if !ok {
				continue
			}
			fn, ok := cm["function"].(map[string]any)
			if !ok {
				continue
			}
			args := map[string]any{}
			switch raw := fn["arguments"].(type) {
			case string:
				if strings.TrimSpace(raw) != "" && json.Unmarshal([]byte(raw), &args) != nil {
					args = map[string]any{"_raw": raw}
				}
			case map[string]any:
				args = raw
			}
			parts = append(parts, map[string]any{
				"type": "tool_use", "id": cm["id"], "name": fn["name"], "input": args,
			})
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return parts
}

func openaiToolsToAnthropic(tools []any) []map[string]any {
	out := make([]map[string]any, 0, len(tools))
	for _, t := range tools {
		m, ok := t.(map[string]any)
		if !ok {
			continue
		}
		// OpenAI: {type:"function", function:{name, description, parameters}}
		if fn, ok := m["function"].(map[string]any); ok {
			out = append(out, map[string]any{
				"name":         fn["name"],
				"description":  fn["description"],
				"input_schema": fn["parameters"],
			})
			continue
		}
		// already anthropic-like
		if _, ok := m["name"]; ok {
			out = append(out, m)
		}
	}
	return out
}

func normalizeContent(c any) any {
	switch v := c.(type) {
	case string:
		return v
	case []any:
		// multimodal parts → plain text join for broad compatibility
		var b strings.Builder
		for _, p := range v {
			m, ok := p.(map[string]any)
			if !ok {
				continue
			}
			typ, _ := m["type"].(string)
			if typ == "text" || typ == "input_text" || typ == "output_text" {
				if t, ok := m["text"].(string); ok {
					b.WriteString(t)
				}
			}
		}
		return b.String()
	default:
		if v == nil {
			return ""
		}
		return fmt.Sprint(v)
	}
}

// convertAnthropicToOpenAIChat wraps Anthropic Messages response as OpenAI chat.completion.
func convertAnthropicToOpenAIChat(body []byte, model string) ([]byte, error) {
	var resp map[string]any
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, err
	}
	// error passthrough shape
	if errObj, ok := resp["error"]; ok {
		return json.Marshal(map[string]any{"error": errObj})
	}

	text := ""
	toolCalls := make([]map[string]any, 0, 2)
	if content, ok := resp["content"].([]any); ok {
		var b strings.Builder
		for _, p := range content {
			m, ok := p.(map[string]any)
			if !ok {
				continue
			}
			typ, _ := m["type"].(string)
			if typ == "text" {
				if t, ok := m["text"].(string); ok {
					b.WriteString(t)
				}
			} else if typ == "tool_use" {
				arguments, _ := json.Marshal(m["input"])
				toolCalls = append(toolCalls, map[string]any{
					"id": m["id"], "type": "function",
					"function": map[string]any{"name": m["name"], "arguments": string(arguments)},
				})
			}
		}
		text = b.String()
	}

	id, _ := resp["id"].(string)
	if id == "" {
		id = "chatcmpl-anthropic"
	}
	if model == "" {
		model, _ = resp["model"].(string)
	}

	finish := "stop"
	if sr, ok := resp["stop_reason"].(string); ok {
		switch sr {
		case "end_turn", "stop_sequence":
			finish = "stop"
		case "max_tokens":
			finish = "length"
		case "tool_use":
			finish = "tool_calls"
		default:
			finish = sr
		}
	}

	message := map[string]any{"role": "assistant", "content": text}
	if len(toolCalls) > 0 {
		message["tool_calls"] = toolCalls
		if text == "" {
			message["content"] = nil
		}
	}
	out := map[string]any{
		"id":      id,
		"object":  "chat.completion",
		"created": 0,
		"model":   model,
		"choices": []map[string]any{
			{
				"index":         0,
				"message":       message,
				"finish_reason": finish,
			},
		},
	}
	if u, ok := resp["usage"].(map[string]any); ok {
		// map anthropic usage → openai
		inTok, _ := u["input_tokens"].(float64)
		outTok, _ := u["output_tokens"].(float64)
		out["usage"] = map[string]any{
			"prompt_tokens":     int(inTok),
			"completion_tokens": int(outTok),
			"total_tokens":      int(inTok + outTok),
		}
	}
	return json.Marshal(out)
}

// anthropicMessagesURL builds the Anthropic messages endpoint from base.
func anthropicMessagesURL(baseURL string) string {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	// accept bases like https://api.anthropic.com or .../v1
	if strings.HasSuffix(strings.ToLower(base), "/v1") {
		return base + "/messages"
	}
	if strings.HasSuffix(strings.ToLower(base), "/messages") {
		return base
	}
	return base + "/v1/messages"
}
