package main

import (
	"bufio"
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

// Stream path: client sends model=aiSwitchModel + stream=true; proxy rewrites to
// active upstream model and pipes SSE with flush + [DONE].
func TestProxyStreamChatCompletionsVirtualModel(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)
	closeDB()
	defer closeDB()

	var gotUpstreamModel string
	var gotStream bool
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			http.NotFound(w, r)
			return
		}
		body, _ := io.ReadAll(r.Body)
		var req map[string]any
		_ = json.Unmarshal(body, &req)
		gotUpstreamModel, _ = req["model"].(string)
		gotStream, _ = req["stream"].(bool)
		if gotUpstreamModel != "deepseek-v4-pro" {
			http.Error(w, "bad model", 400)
			return
		}
		if !gotStream {
			http.Error(w, "stream required", 400)
			return
		}
		if r.Header.Get("Authorization") != "Bearer sk-ds" {
			http.Error(w, "bad auth", 401)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
		w.WriteHeader(200)
		fl, _ := w.(http.Flusher)
		for _, c := range []string{
			`data: {"id":"c1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"role":"assistant","content":null}}]}` + "\n\n",
			`data: {"id":"c1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"content":"STREAM_"}}]}` + "\n\n",
			`data: {"id":"c1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"content":"OK"}}]}` + "\n\n",
			"data: [DONE]\n\n",
		} {
			_, _ = w.Write([]byte(c))
			if fl != nil {
				fl.Flush()
			}
		}
	}))
	defer upstream.Close()

	useProxy := true
	if err := saveProvidersToDisk([]Provider{{
		ID: "demo-deepseek", Name: "DeepSeek", BaseURL: upstream.URL + "/v1", APIKey: "sk-ds",
		UseProxy: &useProxy,
		Models:   []ProviderModel{{ID: "deepseek-v4-pro", Name: "pro", Enabled: true, IsDefault: true}},
	}}); err != nil {
		t.Fatal(err)
	}
	// rebuild SQL routes + active binding
	a := NewApp()
	if err := a.SaveProviders([]Provider{{
		ID: "demo-deepseek", Name: "DeepSeek", BaseURL: upstream.URL + "/v1", APIKey: "sk-ds",
		UseProxy: &useProxy,
		Models:   []ProviderModel{{ID: "deepseek-v4-pro", Name: "pro", Enabled: true, IsDefault: true}},
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.SetActiveGatewayModel("deepseek-v4-pro"); err != nil {
		t.Fatal(err)
	}

	px := newProxyServer()
	body, _ := json.Marshal(map[string]any{
		"model":  "aiSwitchModel",
		"stream": true,
		"messages": []map[string]string{
			{"role": "user", "content": "hi"},
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	rr := httptest.NewRecorder()
	px.handleChatCompletions(rr, req)
	if rr.Code != 200 {
		t.Fatalf("status %d body %s", rr.Code, rr.Body.String())
	}
	ct := rr.Header().Get("Content-Type")
	if !strings.Contains(ct, "event-stream") {
		t.Fatalf("content-type=%s body=%s", ct, rr.Body.String())
	}
	raw := rr.Body.String()
	if !strings.Contains(raw, "[DONE]") {
		t.Fatalf("missing DONE:\n%s", raw)
	}
	var acc strings.Builder
	sc := bufio.NewScanner(strings.NewReader(raw))
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data: "))
		if data == "[DONE]" {
			break
		}
		var obj map[string]any
		if json.Unmarshal([]byte(data), &obj) != nil {
			continue
		}
		choices, _ := obj["choices"].([]any)
		for _, c := range choices {
			cm, _ := c.(map[string]any)
			delta, _ := cm["delta"].(map[string]any)
			if s, ok := delta["content"].(string); ok {
				acc.WriteString(s)
			}
		}
	}
	if acc.String() != "STREAM_OK" {
		t.Fatalf("assembled=%q raw=\n%s", acc.String(), raw)
	}
	if gotUpstreamModel != "deepseek-v4-pro" || !gotStream {
		t.Fatalf("upstream model=%q stream=%v", gotUpstreamModel, gotStream)
	}
}
