package main

import (
	"encoding/json"
	"testing"
)

func TestConvertOpenAIChatToAnthropic(t *testing.T) {
	in := `{"model":"claude-3","messages":[{"role":"system","content":"sys"},{"role":"user","content":"hi"}],"max_tokens":100}`
	out, err := convertOpenAIChatToAnthropic([]byte(in))
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatal(err)
	}
	if m["model"] != "claude-3" {
		t.Fatalf("%v", m["model"])
	}
	if m["system"] != "sys" {
		t.Fatalf("system=%v", m["system"])
	}
	msgs := m["messages"].([]any)
	if len(msgs) != 1 {
		t.Fatalf("msgs=%v", msgs)
	}
}

func TestConvertAnthropicToOpenAIChat(t *testing.T) {
	in := `{"id":"msg_1","model":"claude-3","content":[{"type":"text","text":"hello"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":2}}`
	out, err := convertAnthropicToOpenAIChat([]byte(in), "claude-3")
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	_ = json.Unmarshal(out, &m)
	choices := m["choices"].([]any)
	c0 := choices[0].(map[string]any)
	msg := c0["message"].(map[string]any)
	if msg["content"] != "hello" {
		t.Fatalf("%v", msg)
	}
}

func TestConvertAnthropicToolUseToOpenAIToolCall(t *testing.T) {
	in := `{"id":"msg_1","content":[{"type":"tool_use","id":"call_1","name":"weather","input":{"city":"Tokyo"}}],"stop_reason":"tool_use"}`
	out, err := convertAnthropicToOpenAIChat([]byte(in), "claude-3")
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(out, &payload); err != nil {
		t.Fatal(err)
	}
	message := payload["choices"].([]any)[0].(map[string]any)["message"].(map[string]any)
	calls := message["tool_calls"].([]any)
	fn := calls[0].(map[string]any)["function"].(map[string]any)
	if fn["name"] != "weather" || fn["arguments"] != `{"city":"Tokyo"}` {
		t.Fatalf("unexpected tool call: %#v", calls[0])
	}
}

func TestConvertOpenAIToolMessagesToAnthropic(t *testing.T) {
	in := `{"model":"claude-3","messages":[{"role":"assistant","content":null,"tool_calls":[{"id":"call_1","type":"function","function":{"name":"weather","arguments":"{\"city\":\"Tokyo\"}"}}]},{"role":"tool","tool_call_id":"call_1","content":"sunny"}]}`
	out, err := convertOpenAIChatToAnthropic([]byte(in))
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(out, &payload); err != nil {
		t.Fatal(err)
	}
	messages := payload["messages"].([]any)
	assistantContent := messages[0].(map[string]any)["content"].([]any)
	if assistantContent[0].(map[string]any)["type"] != "tool_use" {
		t.Fatalf("assistant content: %#v", assistantContent)
	}
	toolContent := messages[1].(map[string]any)["content"].([]any)
	if toolContent[0].(map[string]any)["tool_use_id"] != "call_1" {
		t.Fatalf("tool content: %#v", toolContent)
	}
}

func TestResolveAPIFormat(t *testing.T) {
	if ResolveAPIFormat(Provider{APIFormat: "auto", BaseURL: "https://api.anthropic.com"}) != APIFormatAnthropicMessages {
		t.Fatal("expected anthropic")
	}
	if ResolveAPIFormat(Provider{APIFormat: "auto", BaseURL: "https://api.deepseek.com/v1"}) != APIFormatOpenAIChat {
		t.Fatal("expected openai")
	}
	if ResolveAPIFormat(Provider{APIFormat: "openai", BaseURL: "https://api.anthropic.com"}) != APIFormatOpenAIChat {
		t.Fatal("explicit openai")
	}
}
