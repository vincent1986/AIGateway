package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestJoinOpenAIURL(t *testing.T) {
	cases := []struct {
		base, path, want string
	}{
		{"https://api.deepseek.com/v1", "chat/completions", "https://api.deepseek.com/v1/chat/completions"},
		{"https://api.deepseek.com", "chat/completions", "https://api.deepseek.com/v1/chat/completions"},
		{"https://api.openai.com/v1/", "models", "https://api.openai.com/v1/models"},
	}
	for _, c := range cases {
		got := joinOpenAIURL(c.base, c.path)
		if got != c.want {
			t.Fatalf("join(%q,%q)=%q want %q", c.base, c.path, got, c.want)
		}
	}
}

func TestAnthropicRequestToOpenAI(t *testing.T) {
	in := []byte(`{"model":"aiSwitchModel-claude","system":"be concise","max_tokens":128,"messages":[{"role":"user","content":[{"type":"text","text":"hello"}]}],"stream":false}`)
	out, stream, err := anthropicRequestToOpenAI(in)
	if err != nil {
		t.Fatal(err)
	}
	if stream {
		t.Fatal("expected non-stream request")
	}
	var got map[string]any
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatal(err)
	}
	if got["model"] != "aiSwitchModel-claude" {
		t.Fatalf("model=%v", got["model"])
	}
	messages := got["messages"].([]any)
	if len(messages) != 2 || messages[0].(map[string]any)["role"] != "system" {
		t.Fatalf("messages=%v", messages)
	}
}

func TestOpenAIResponseToAnthropic(t *testing.T) {
	in := []byte(`{"id":"chatcmpl-1","model":"deepseek-v4-pro","choices":[{"message":{"role":"assistant","content":"hello"}}],"usage":{"prompt_tokens":2,"completion_tokens":3}}`)
	out, err := openAIResponseToAnthropic(in)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatal(err)
	}
	if got["type"] != "message" || got["role"] != "assistant" {
		t.Fatalf("response=%v", got)
	}
	content := got["content"].([]any)[0].(map[string]any)
	if content["type"] != "text" || content["text"] != "hello" {
		t.Fatalf("content=%v", content)
	}
}

func TestResolveProviderForModel(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)

	list := []Provider{
		{
			ID: "p1", Name: "DeepSeek", BaseURL: "https://api.deepseek.com/v1", APIKey: "sk-ds",
			Models: []ProviderModel{
				{ID: "deepseek-chat", Name: "chat", Enabled: true, IsDefault: true},
			},
		},
		{
			ID: "p2", Name: "OpenAI", BaseURL: "https://api.openai.com/v1", APIKey: "sk-oa",
			Models: []ProviderModel{
				{ID: "gpt-4o", Name: "4o", Enabled: true},
			},
		},
	}
	if err := saveProvidersToDisk(list); err != nil {
		t.Fatal(err)
	}

	p, err := resolveProviderForModel("deepseek-chat")
	if err != nil || p.Name != "DeepSeek" {
		t.Fatalf("%v %+v", err, p)
	}
	p, err = resolveProviderForModel("gpt-4o")
	if err != nil || p.Name != "OpenAI" {
		t.Fatalf("%v %+v", err, p)
	}
	_, err = resolveProviderForModel("unknown-model")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestProxyChatForward(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)

	// fake upstream OpenAI-compatible server
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("Authorization") != "Bearer sk-ds" {
			http.Error(w, "bad auth", 401)
			return
		}
		body, _ := io.ReadAll(r.Body)
		var req map[string]any
		_ = json.Unmarshal(body, &req)
		if req["model"] != "deepseek-chat" {
			http.Error(w, "bad model", 400)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "chatcmpl-test",
			"choices": []map[string]any{
				{"message": map[string]string{"role": "assistant", "content": "hi"}},
			},
		})
	}))
	defer upstream.Close()

	list := []Provider{{
		ID: "p1", Name: "DeepSeek", BaseURL: upstream.URL + "/v1", APIKey: "sk-ds",
		Models: []ProviderModel{{ID: "deepseek-chat", Enabled: true}},
	}}
	if err := saveProvidersToDisk(list); err != nil {
		t.Fatal(err)
	}

	px := newProxyServer()
	px.cfg.Host = "127.0.0.1"
	px.cfg.Port = 0 // will pick in listen — our start uses fixed port; use handler directly

	// call handler via ResponseRecorder
	body, _ := json.Marshal(map[string]any{
		"model": "deepseek-chat",
		"messages": []map[string]string{
			{"role": "user", "content": "hello"},
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	px.handleChatCompletions(rr, req)
	if rr.Code != 200 {
		t.Fatalf("status %d body %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp["id"] != "chatcmpl-test" {
		t.Fatalf("%v", resp)
	}
}

func TestProxyUsageStatsUseUpstreamModelForAppProxyAlias(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)
	closeDB()
	defer closeDB()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			http.NotFound(w, r)
			return
		}
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), `"model":"deepseek-chat"`) {
			http.Error(w, "bad upstream model: "+string(body), http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "chatcmpl-test",
			"choices": []map[string]any{
				{"message": map[string]string{"role": "assistant", "content": "hi"}},
			},
			"usage": map[string]any{"prompt_tokens": 3, "completion_tokens": 4, "total_tokens": 7},
		})
	}))
	defer upstream.Close()

	if err := saveProvidersToDisk([]Provider{{
		ID: "p1", Name: "DeepSeek", BaseURL: upstream.URL + "/v1", APIKey: "sk-ds",
		Models: []ProviderModel{{ID: "deepseek-chat", Enabled: true}},
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := (&App{}).SetAppProxyModel("claude", "deepseek-chat"); err != nil {
		t.Fatal(err)
	}

	body, _ := json.Marshal(map[string]any{
		"model": appProxyModel(ToolClaude),
		"messages": []map[string]string{
			{"role": "user", "content": "hello"},
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	newProxyServer().handleChatCompletions(rr, req)
	if rr.Code != 200 {
		t.Fatalf("status %d body %s", rr.Code, rr.Body.String())
	}

	st := (&App{}).GetUsageStats()
	if st.Total.TotalTokens != 7 {
		t.Fatalf("tokens=%d", st.Total.TotalTokens)
	}
	if len(st.ByModel) != 1 || st.ByModel[0].Key != "deepseek-chat" {
		t.Fatalf("ByModel=%+v", st.ByModel)
	}
}

func TestProxyConfigPersist(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)
	cfg := ProxyConfig{Host: "127.0.0.1", Port: 19090, AutoStart: true}
	if err := saveProxyConfig(cfg); err != nil {
		t.Fatal(err)
	}
	got := loadProxyConfig()
	if got.Port != 19090 || !got.AutoStart {
		t.Fatalf("%+v", got)
	}
	// ensure file under tmp
	if _, err := os.Stat(filepath.Join(tmp, ".codex-manager", "proxy.json")); err != nil {
		t.Fatal(err)
	}
}
