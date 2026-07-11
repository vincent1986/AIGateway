package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestResponsesBodyToChat(t *testing.T) {
	in := `{
		"model": "deepseek-chat",
		"instructions": "be brief",
		"input": [{"role":"user","content":"hi"}]
	}`
	out, err := responsesBodyToChat([]byte(in))
	if err != nil {
		t.Fatal(err)
	}
	var chat map[string]any
	if err := json.Unmarshal(out, &chat); err != nil {
		t.Fatal(err)
	}
	if chat["model"] != "deepseek-chat" {
		t.Fatalf("%v", chat["model"])
	}
	msgs := chat["messages"].([]any)
	if len(msgs) < 2 {
		t.Fatalf("msgs=%v", msgs)
	}
}

func TestResponsesToolLoopToChat(t *testing.T) {
	// Codex multi-turn: developer + user + function_call + function_call_output
	in := `{
		"model":"deepseek-v4-pro",
		"input":[
			{"role":"developer","content":"sys"},
			{"role":"user","content":"list files"},
			{"type":"function_call","call_id":"call_1","name":"shell","arguments":"{\"cmd\":\"ls\"}"},
			{"type":"function_call_output","call_id":"call_1","output":"a.txt\nb.txt"},
			{"role":"user","content":"now cat a.txt"}
		],
		"tools":[{"type":"function","name":"shell","parameters":{"type":"object","properties":{}}}]
	}`
	out, err := responsesBodyToChat([]byte(in))
	if err != nil {
		t.Fatal(err)
	}
	out = normalizeUpstreamChatBody(out)
	var chat map[string]any
	if err := json.Unmarshal(out, &chat); err != nil {
		t.Fatal(err)
	}
	msgs := chat["messages"].([]any)
	// system (from developer), user, assistant(tool_calls), tool, user
	if len(msgs) < 5 {
		t.Fatalf("want >=5 messages, got %d: %s", len(msgs), out)
	}
	roles := make([]string, 0, len(msgs))
	for _, m := range msgs {
		roles = append(roles, m.(map[string]any)["role"].(string))
	}
	// developer → system
	if roles[0] != "system" {
		t.Fatalf("roles[0]=%v want system: %v", roles[0], roles)
	}
	// find assistant with tool_calls
	foundTC := false
	foundTool := false
	for _, raw := range msgs {
		m := raw.(map[string]any)
		if m["role"] == "assistant" {
			if tcs, ok := m["tool_calls"].([]any); ok && len(tcs) > 0 {
				foundTC = true
				tc0 := tcs[0].(map[string]any)
				fn := tc0["function"].(map[string]any)
				if fn["name"] != "shell" {
					t.Fatalf("tool name=%v", fn["name"])
				}
			}
		}
		if m["role"] == "tool" {
			foundTool = true
			if m["tool_call_id"] != "call_1" {
				t.Fatalf("tool_call_id=%v", m["tool_call_id"])
			}
			if m["content"] != "a.txt\nb.txt" {
				t.Fatalf("tool content=%v", m["content"])
			}
		}
	}
	if !foundTC || !foundTool {
		t.Fatalf("missing tool loop msgs foundTC=%v foundTool=%v roles=%v\n%s", foundTC, foundTool, roles, out)
	}
	// tools must have nested function
	tools := chat["tools"].([]any)
	if _, ok := tools[0].(map[string]any)["function"]; !ok {
		t.Fatalf("tools not nested: %s", out)
	}
}

func TestChatBodyToResponsesToolCalls(t *testing.T) {
	chat := `{
		"id":"chatcmpl-1",
		"choices":[{
			"message":{
				"role":"assistant",
				"content":null,
				"tool_calls":[{
					"id":"call_abc",
					"type":"function",
					"function":{"name":"shell","arguments":"{\"cmd\":\"pwd\"}"}
				}]
			}
		}]
	}`
	out, err := chatBodyToResponses([]byte(chat), "deepseek-v4-pro")
	if err != nil {
		t.Fatal(err)
	}
	var resp map[string]any
	if err := json.Unmarshal(out, &resp); err != nil {
		t.Fatal(err)
	}
	outputs := resp["output"].([]any)
	found := false
	for _, o := range outputs {
		m := o.(map[string]any)
		if m["type"] == "function_call" && m["name"] == "shell" && m["call_id"] == "call_abc" {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing function_call output: %s", out)
	}
}

func TestNormalizeToolsMissingFunction(t *testing.T) {
	// Codex / Responses flat tool (no nested function) — DeepSeek rejects this
	in := `{
		"model":"deepseek-chat",
		"messages":[{"role":"user","content":"hi"}],
		"tools":[
			{
				"type":"function",
				"name":"shell",
				"description":"run a command",
				"parameters":{"type":"object","properties":{"cmd":{"type":"string"}}}
			},
			{"type":"web_search"},
			{
				"type":"function",
				"function":{"name":"already_ok","parameters":{"type":"object","properties":{}}}
			}
		],
		"tool_choice":{"type":"function","name":"shell"}
	}`
	out := normalizeUpstreamChatBody([]byte(in))
	var m map[string]any
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatal(err)
	}
	tools := m["tools"].([]any)
	if len(tools) != 2 {
		t.Fatalf("want 2 function tools (web_search dropped), got %d: %s", len(tools), out)
	}
	for i, item := range tools {
		tmap := item.(map[string]any)
		fn, ok := tmap["function"].(map[string]any)
		if !ok {
			t.Fatalf("tools[%d] missing function: %v", i, tmap)
		}
		if name, _ := fn["name"].(string); name == "" {
			t.Fatalf("tools[%d] empty name: %v", i, fn)
		}
		if _, ok := fn["parameters"]; !ok {
			t.Fatalf("tools[%d] missing parameters: %v", i, fn)
		}
	}
	// first tool should be shell with nested function
	fn0 := tools[0].(map[string]any)["function"].(map[string]any)
	if fn0["name"] != "shell" {
		t.Fatalf("first tool name=%v", fn0["name"])
	}
	tc := m["tool_choice"].(map[string]any)
	if tc["type"] != "function" {
		t.Fatalf("tool_choice type=%v", tc["type"])
	}
	if tc["function"].(map[string]any)["name"] != "shell" {
		t.Fatalf("tool_choice function=%v", tc["function"])
	}
}

func TestResponsesToolsConverted(t *testing.T) {
	in := `{
		"model":"deepseek-chat",
		"input":"hi",
		"tools":[
			{"type":"function","name":"read_file","description":"read","parameters":{"type":"object","properties":{}}}
		]
	}`
	out, err := responsesBodyToChat([]byte(in))
	if err != nil {
		t.Fatal(err)
	}
	out = normalizeUpstreamChatBody(out)
	var chat map[string]any
	if err := json.Unmarshal(out, &chat); err != nil {
		t.Fatal(err)
	}
	tools := chat["tools"].([]any)
	t0 := tools[0].(map[string]any)
	if _, ok := t0["function"].(map[string]any); !ok {
		t.Fatalf("expected nested function: %s", out)
	}
}

func TestNormalizeDeveloperRole(t *testing.T) {
	if got := normalizeChatRole("developer"); got != "system" {
		t.Fatalf("developer→%q", got)
	}
	if got := normalizeChatRole("function"); got != "tool" {
		t.Fatalf("function→%q", got)
	}
	if got := normalizeChatRole("user"); got != "user" {
		t.Fatalf("user→%q", got)
	}

	// chat/completions body with Codex-style developer role
	in := `{
		"model":"deepseek-chat",
		"messages":[
			{"role":"system","content":"sys"},
			{"role":"developer","content":"dev instructions"},
			{"role":"user","content":"hi"}
		]
	}`
	out := normalizeUpstreamChatBody([]byte(in))
	var m map[string]any
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatal(err)
	}
	msgs := m["messages"].([]any)
	roles := make([]string, 0, len(msgs))
	for _, item := range msgs {
		msg := item.(map[string]any)
		roles = append(roles, msg["role"].(string))
	}
	for _, r := range roles {
		if r == "developer" {
			t.Fatalf("developer not rewritten: %v", roles)
		}
	}
	if roles[1] != "system" {
		t.Fatalf("want developer→system, got %v", roles)
	}

	// responses conversion also rewrites developer in input
	respIn := `{
		"model":"deepseek-chat",
		"input":[
			{"role":"developer","content":"rules"},
			{"role":"user","content":"hi"}
		]
	}`
	chatBody, err := responsesBodyToChat([]byte(respIn))
	if err != nil {
		t.Fatal(err)
	}
	chatBody = normalizeUpstreamChatBody(chatBody)
	var chat map[string]any
	if err := json.Unmarshal(chatBody, &chat); err != nil {
		t.Fatal(err)
	}
	for _, item := range chat["messages"].([]any) {
		role := item.(map[string]any)["role"].(string)
		if role == "developer" {
			t.Fatalf("developer remained in converted chat: %s", chatBody)
		}
	}
}

func TestChatBodyToResponses(t *testing.T) {
	chat := `{
		"id":"chatcmpl-abc",
		"choices":[{"message":{"role":"assistant","content":"hello world"}}]
	}`
	out, err := chatBodyToResponses([]byte(chat), "deepseek-chat")
	if err != nil {
		t.Fatal(err)
	}
	var resp map[string]any
	if err := json.Unmarshal(out, &resp); err != nil {
		t.Fatal(err)
	}
	if resp["output_text"] != "hello world" {
		t.Fatalf("%v", resp["output_text"])
	}
	if resp["object"] != "response" {
		t.Fatalf("%v", resp["object"])
	}
}

func TestHandleResponsesViaChat(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)

	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "chatcmpl-x",
			"choices": []map[string]any{
				{"message": map[string]string{"role": "assistant", "content": "via-chat"}},
			},
		})
	}))
	defer up.Close()

	if err := saveProvidersToDisk([]Provider{{
		ID: "p1", Name: "DeepSeek", BaseURL: up.URL + "/v1", APIKey: "sk",
		Models: []ProviderModel{{ID: "deepseek-chat", Enabled: true}},
	}}); err != nil {
		t.Fatal(err)
	}

	px := newProxyServer()
	body := `{"model":"deepseek-chat","input":"ping"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	px.handleResponses(rr, req)
	if rr.Code != 200 {
		t.Fatalf("status %d body %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp["output_text"] != "via-chat" {
		t.Fatalf("%s", rr.Body.String())
	}
}

func TestHandleResponsesStreamEmitsCompleted(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)

	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			http.NotFound(w, r)
			return
		}
		// verify client asked for stream
		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req["stream"] != true {
			t.Errorf("expected stream=true, got %v", req["stream"])
		}
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		chunks := []string{
			`{"id":"chatcmpl-1","choices":[{"delta":{"role":"assistant"}}]}`,
			`{"id":"chatcmpl-1","choices":[{"delta":{"content":"Hel"}}]}`,
			`{"id":"chatcmpl-1","choices":[{"delta":{"content":"lo"}}]}`,
			`{"id":"chatcmpl-1","choices":[{"delta":{},"finish_reason":"stop"}]}`,
		}
		for _, c := range chunks {
			_, _ = fmt.Fprintf(w, "data: %s\n\n", c)
			if flusher != nil {
				flusher.Flush()
			}
		}
		_, _ = fmt.Fprintf(w, "data: [DONE]\n\n")
		if flusher != nil {
			flusher.Flush()
		}
	}))
	defer up.Close()

	if err := saveProvidersToDisk([]Provider{{
		ID: "p1", Name: "DeepSeek", BaseURL: up.URL + "/v1", APIKey: "sk",
		Models: []ProviderModel{{ID: "deepseek-chat", Enabled: true}},
	}}); err != nil {
		t.Fatal(err)
	}

	px := newProxyServer()
	body := `{"model":"deepseek-chat","input":"ping","stream":true}`
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	px.handleResponses(rr, req)
	if rr.Code != 200 {
		t.Fatalf("status %d body %s", rr.Code, rr.Body.String())
	}
	raw := rr.Body.String()
	if !strings.Contains(raw, "event: response.created") {
		t.Fatalf("missing response.created:\n%s", raw)
	}
	if !strings.Contains(raw, "event: response.output_text.delta") {
		t.Fatalf("missing output_text.delta:\n%s", raw)
	}
	if !strings.Contains(raw, "event: response.completed") {
		t.Fatalf("missing response.completed:\n%s", raw)
	}
	// data payload must nest full response and include type
	if !strings.Contains(raw, `"type":"response.completed"`) {
		t.Fatalf("completed event missing type field:\n%s", raw)
	}
	if !strings.Contains(raw, `"status":"completed"`) {
		t.Fatalf("completed response missing status:\n%s", raw)
	}
	if !strings.Contains(raw, "Hello") {
		t.Fatalf("missing streamed text:\n%s", raw)
	}
}

func TestNormalizeUsageToResponses(t *testing.T) {
	// chat-style DeepSeek usage
	chatUsage := map[string]any{
		"prompt_tokens":     10,
		"completion_tokens": 26,
		"total_tokens":      36,
		"prompt_tokens_details": map[string]any{
			"cached_tokens": 2,
		},
		"completion_tokens_details": map[string]any{
			"reasoning_tokens": 24,
		},
		"prompt_cache_hit_tokens": 2,
	}
	u := normalizeUsageToResponses(chatUsage)
	if u["input_tokens"] != 10 {
		t.Fatalf("input_tokens=%v", u["input_tokens"])
	}
	if u["output_tokens"] != 26 {
		t.Fatalf("output_tokens=%v", u["output_tokens"])
	}
	if u["total_tokens"] != 36 {
		t.Fatalf("total_tokens=%v", u["total_tokens"])
	}
	if u["input_tokens_details"].(map[string]any)["cached_tokens"] != 2 {
		t.Fatalf("cached=%v", u["input_tokens_details"])
	}
	if u["output_tokens_details"].(map[string]any)["reasoning_tokens"] != 24 {
		t.Fatalf("reasoning=%v", u["output_tokens_details"])
	}
	// nil → zeros but fields present
	z := normalizeUsageToResponses(nil)
	if _, ok := z["input_tokens"]; !ok {
		t.Fatal("missing input_tokens on empty usage")
	}
}

func TestCompletedResponseHasInputTokens(t *testing.T) {
	chat := `{
		"id":"chatcmpl-1",
		"choices":[{"message":{"role":"assistant","content":"hi"}}],
		"usage":{"prompt_tokens":3,"completion_tokens":1,"total_tokens":4}
	}`
	out, err := chatBodyToResponses([]byte(chat), "deepseek-v4-pro")
	if err != nil {
		t.Fatal(err)
	}
	var resp map[string]any
	if err := json.Unmarshal(out, &resp); err != nil {
		t.Fatal(err)
	}
	usage, ok := resp["usage"].(map[string]any)
	if !ok {
		t.Fatalf("missing usage: %s", out)
	}
	if _, ok := usage["input_tokens"]; !ok {
		t.Fatalf("missing input_tokens: %s", out)
	}
	// ensure chat-style keys are not the only form
	if usage["input_tokens"].(float64) != 3 {
		t.Fatalf("input_tokens=%v", usage["input_tokens"])
	}
}

func TestResponsesSSECompletedEnvelope(t *testing.T) {
	rr := httptest.NewRecorder()
	sse := newResponsesSSE(rr)
	sse.begin()
	final := buildResponsesObject("resp_1", "m", 1, "completed", "hi", []any{
		map[string]any{"type": "message", "role": "assistant", "content": []any{
			map[string]any{"type": "output_text", "text": "hi"},
		}},
	})
	sse.emit("response.completed", map[string]any{"response": final})
	body := rr.Body.String()
	if !strings.Contains(body, "event: response.completed") {
		t.Fatal(body)
	}
	// parse data line
	for _, line := range strings.Split(body, "\n") {
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		var ev map[string]any
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &ev); err != nil {
			t.Fatal(err)
		}
		if ev["type"] != "response.completed" {
			t.Fatalf("%v", ev["type"])
		}
		resp, ok := ev["response"].(map[string]any)
		if !ok {
			t.Fatalf("response not nested: %v", ev)
		}
		if resp["status"] != "completed" {
			t.Fatalf("%v", resp["status"])
		}
	}
}
