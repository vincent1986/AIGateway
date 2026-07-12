package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestProxyTakeoverRestoresWireAPIAndRemovesInjectedField(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	original := `[model_providers.responses]
base_url = "https://responses.example/v1"
wire_api = "responses"

[model_providers.chat]
base_url = "https://chat.example/v1"
`
	if err := rememberOriginalBases(original, "http://127.0.0.1:18080/v1"); err != nil {
		t.Fatal(err)
	}
	taken := setAllProvidersBaseURL(original, "http://127.0.0.1:18080/v1")
	state, err := loadOriginalProviderState()
	if err != nil {
		t.Fatal(err)
	}
	for id, saved := range state.Providers {
		taken = restoreProviderField(taken, id, "base_url", saved.BaseURL)
		taken = restoreProviderField(taken, id, "wire_api", saved.WireAPI)
	}
	if !strings.Contains(taken, `wire_api = "responses"`) {
		t.Fatalf("responses wire_api not restored:\n%s", taken)
	}
	chat, _ := findProviderTable(taken, "chat")
	if strings.Contains(taken[chat.Start:chat.End], "wire_api") {
		t.Fatalf("injected chat wire_api not removed:\n%s", taken[chat.Start:chat.End])
	}
}

func TestAnthropicConnectionProbeUsesNetwork(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("x-api-key") != "valid-key" {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"claude-test","display_name":"Claude Test"}]}`))
	}))
	defer server.Close()

	if got := probeProvider(server.URL, "bad-key", "anthropic"); got.OK || got.StatusCode != http.StatusUnauthorized {
		t.Fatalf("invalid key unexpectedly succeeded: %#v", got)
	}
	if got := probeProvider(server.URL, "valid-key", "anthropic"); !got.OK || got.ModelCount != 1 {
		t.Fatalf("valid probe failed: %#v", got)
	}
}

func TestValidateProxyExposure(t *testing.T) {
	if err := validateProxyExposure(ProxyConfig{Host: "127.0.0.1", Port: 18080}); err != nil {
		t.Fatalf("loopback should not require a key: %v", err)
	}
	if err := validateProxyExposure(ProxyConfig{Host: "0.0.0.0", Port: 18080}); err == nil {
		t.Fatal("public listener without a key should be rejected")
	}
	if err := validateProxyExposure(ProxyConfig{Host: "0.0.0.0", Port: 18080, ListenKey: "secret"}); err != nil {
		t.Fatalf("public listener with a key should be accepted: %v", err)
	}
}

func TestSetProviderFieldPreservesTOMLValuesAndComments(t *testing.T) {
	in := `[model_providers.demo]
name = "Demo"
base_url = "https://old.example/v1" # keep this note
requires_openai_auth = true
query_params = { region = "cn" }
`
	out := setProviderField(in, "demo", "base_url", "http://127.0.0.1:18080/v1")
	for _, want := range []string{
		`base_url = "http://127.0.0.1:18080/v1" # keep this note`,
		`requires_openai_auth = true`,
		`query_params = { region = "cn" }`,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in:\n%s", want, out)
		}
	}
}

func TestChatToolCallToResponsesAndSSE(t *testing.T) {
	chat := `{"id":"chatcmpl-1","choices":[{"message":{"role":"assistant","content":null,"tool_calls":[{"id":"call_1","type":"function","function":{"name":"shell","arguments":"{\"cmd\":\"pwd\"}"}}]}}]}`
	body, err := chatBodyToResponses([]byte(chat), "test-model")
	if err != nil {
		t.Fatal(err)
	}
	var response map[string]any
	if err := json.Unmarshal(body, &response); err != nil {
		t.Fatal(err)
	}
	item := response["output"].([]any)[0].(map[string]any)
	if item["type"] != "function_call" || item["name"] != "shell" {
		t.Fatalf("unexpected response item: %#v", item)
	}

	recorder := httptest.NewRecorder()
	writeResponsesResult(recorder, body, true)
	if got := recorder.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/event-stream") {
		t.Fatalf("content type = %q", got)
	}
	for _, event := range []string{"response.created", "response.function_call_arguments.done", "response.completed"} {
		if !strings.Contains(recorder.Body.String(), "event: "+event) {
			t.Fatalf("missing event %s in:\n%s", event, recorder.Body.String())
		}
	}
}

func TestRelaySSECollectsResponsesUsage(t *testing.T) {
	in := "event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"usage\":{\"input_tokens\":9,\"output_tokens\":4,\"total_tokens\":13}}}\n\n"
	recorder := httptest.NewRecorder()
	prompt, completion, total, err := relaySSE(recorder, strings.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}
	if prompt != 9 || completion != 4 || total != 13 {
		t.Fatalf("usage = %d %d %d", prompt, completion, total)
	}
	if recorder.Body.String() != in {
		t.Fatalf("stream changed: %q", recorder.Body.String())
	}
}

func TestStreamChatAsResponsesEmitsDeltasAndUsage(t *testing.T) {
	in := "data: {\"choices\":[{\"delta\":{\"content\":\"hel\"}}]}\n\n" +
		"data: {\"choices\":[{\"delta\":{\"content\":\"lo\"}}]}\n\n" +
		"data: {\"choices\":[],\"usage\":{\"prompt_tokens\":3,\"completion_tokens\":2,\"total_tokens\":5}}\n\n" +
		"data: [DONE]\n\n"
	recorder := httptest.NewRecorder()
	log := newProxyServer().streamChatAsResponses(recorder, strings.NewReader(in), "demo")
	if log.TotalTokens != 5 {
		t.Fatalf("usage not collected: %#v", log)
	}
	out := recorder.Body.String()
	for _, want := range []string{`"delta":"hel"`, `"delta":"lo"`, `"text":"hello"`, "event: response.completed"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in:\n%s", want, out)
		}
	}
}
