package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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
