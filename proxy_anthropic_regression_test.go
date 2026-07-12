package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAnthropicToolUseRoundTrip(t *testing.T) {
	anthropic := []byte(`{"id":"msg_1","model":"claude-3","content":[{"type":"tool_use","id":"call_1","name":"weather","input":{"city":"Tokyo"}}],"stop_reason":"tool_use"}`)
	openai, err := convertAnthropicToOpenAIChat(anthropic, "claude-3")
	if err != nil {
		t.Fatal(err)
	}
	var chat map[string]any
	if err := json.Unmarshal(openai, &chat); err != nil {
		t.Fatal(err)
	}
	message := chat["choices"].([]any)[0].(map[string]any)["message"].(map[string]any)
	calls := message["tool_calls"].([]any)
	if len(calls) != 1 {
		t.Fatalf("expected one tool call, got %d", len(calls))
	}
	call := calls[0].(map[string]any)
	if call["id"] != "call_1" || call["type"] != "function" {
		t.Fatalf("unexpected call: %#v", call)
	}

	body := []byte(`{"model":"claude-3","messages":[{"role":"assistant","content":null,"tool_calls":[{"id":"call_1","type":"function","function":{"name":"weather","arguments":"{\"city\":\"Tokyo\"}"}}]},{"role":"tool","tool_call_id":"call_1","content":"sunny"}]}`)
	converted, err := convertOpenAIChatToAnthropic(body)
	if err != nil {
		t.Fatal(err)
	}
	var req map[string]any
	if err := json.Unmarshal(converted, &req); err != nil {
		t.Fatal(err)
	}
	messages := req["messages"].([]any)
	if messages[0].(map[string]any)["content"].([]any)[0].(map[string]any)["type"] != "tool_use" {
		t.Fatalf("assistant tool call was not converted: %#v", messages[0])
	}
	if messages[1].(map[string]any)["content"].([]any)[0].(map[string]any)["tool_use_id"] != "call_1" {
		t.Fatalf("tool result id was not preserved: %#v", messages[1])
	}
}

func TestAnthropicConnectionProbeUsesNativeModelsEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("x-api-key") != "valid-key" || r.Header.Get("anthropic-version") == "" {
			http.Error(w, "bad headers", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"claude-3-5-sonnet","display_name":"Claude Sonnet"}]}`))
	}))
	defer server.Close()

	items, status, _, endpoint, err := fetchModelsAnthropicDetailed(server.URL+"/v1", "valid-key")
	if err != nil {
		t.Fatal(err)
	}
	if status != http.StatusOK || endpoint != server.URL+"/v1/models" || len(items) != 1 || items[0].ID != "claude-3-5-sonnet" {
		t.Fatalf("unexpected models result: status=%d endpoint=%s items=%#v", status, endpoint, items)
	}
}

func TestValidateProxyExposureRequiresKeyForNonLoopback(t *testing.T) {
	if err := validateProxyExposure(ProxyConfig{Host: "127.0.0.1", Port: 18080}); err != nil {
		t.Fatalf("loopback should be allowed: %v", err)
	}
	if err := validateProxyExposure(ProxyConfig{Host: "0.0.0.0", Port: 18080}); err == nil {
		t.Fatal("expected non-loopback without key to fail")
	}
	if err := validateProxyExposure(ProxyConfig{Host: "0.0.0.0", Port: 18080, ListenKey: "secret"}); err != nil {
		t.Fatalf("non-loopback with key should be allowed: %v", err)
	}
}

func TestSetProviderFieldPreservesProviderBlock(t *testing.T) {
	in := `[model_providers.deepseek]
# keep this comment
name = "DeepSeek"
base_url = "https://old.example/v1"
requires_openai_auth = true
custom = ["still", "array"]
`
	out := setProviderField(in, "deepseek", "base_url", "http://127.0.0.1:18080/v1")
	for _, want := range []string{
		"# keep this comment",
		`requires_openai_auth = true`,
		`custom = ["still", "array"]`,
		`base_url = "http://127.0.0.1:18080/v1"`,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in:\n%s", want, out)
		}
	}
}

func TestWriteResponsesResultStreamCompletes(t *testing.T) {
	body := []byte(`{"id":"resp_1","object":"response","status":"completed","model":"m","output_text":"","output":[{"type":"function_call","id":"fc_1","call_id":"call_1","name":"shell","arguments":"{\"cmd\":\"pwd\"}","status":"completed"}]}`)
	rec := httptest.NewRecorder()
	writeResponsesResult(rec, body, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "event: response.completed") ||
		!strings.Contains(rec.Body.String(), "event: response.function_call_arguments.done") {
		t.Fatalf("missing responses SSE events:\n%s", rec.Body.String())
	}
}
