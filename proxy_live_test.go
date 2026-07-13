package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

var liveHTTP = &http.Client{Transport: &http.Transport{DisableKeepAlives: true}, Timeout: 5 * time.Second}

func TestProxyLiveHTTP(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)

	// upstream mock (OpenAI-compatible)
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/models":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"object": "list",
				"data":   []map[string]string{{"id": "deepseek-chat", "owned_by": "deepseek"}},
			})
		case "/v1/chat/completions":
			if r.Header.Get("Authorization") != "Bearer sk-live" {
				http.Error(w, "auth", 401)
				return
			}
			body, _ := io.ReadAll(r.Body)
			var req map[string]any
			_ = json.Unmarshal(body, &req)
			if req["model"] != "deepseek-chat" {
				http.Error(w, "model", 400)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "chatcmpl-live",
				"choices": []map[string]any{
					{"message": map[string]string{"role": "assistant", "content": "pong"}},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(up.Close)

	if err := saveProvidersToDisk([]Provider{{
		ID: "p1", Name: "DeepSeek", BaseURL: up.URL + "/v1", APIKey: "sk-live",
		Models: []ProviderModel{{ID: "deepseek-chat", Enabled: true, IsDefault: true}},
	}}); err != nil {
		t.Fatal(err)
	}

	// free port
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()

	px := newProxyServer()
	px.cfg.Host = "127.0.0.1"
	px.cfg.Port = port
	if err := px.start(); err != nil {
		t.Fatalf("start proxy: %v", err)
	}
	defer func() {
		_ = px.stop()
	}()

	// wait ready
	base := px.baseURL()
	deadline := time.Now().Add(3 * time.Second)
	for {
		resp, err := liveHTTP.Get("http://127.0.0.1:" + itoa(port) + "/health")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == 200 {
				break
			}
		}
		if time.Now().After(deadline) {
			t.Fatal("proxy not healthy")
		}
		time.Sleep(50 * time.Millisecond)
	}

	// health
	resp, err := liveHTTP.Get("http://127.0.0.1:" + itoa(port) + "/health")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("health %d %s", resp.StatusCode, b)
	}
	t.Logf("health OK: %s", b)

	// models via proxy (aggregated from providers.json)
	resp, err = liveHTTP.Get(base + "/models")
	if err != nil {
		t.Fatal(err)
	}
	b, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("models %d %s", resp.StatusCode, b)
	}
	var models map[string]any
	_ = json.Unmarshal(b, &models)
	t.Logf("models OK: %s", b)

	// chat via proxy → upstream
	chatBody, _ := json.Marshal(map[string]any{
		"model": "deepseek-chat",
		"messages": []map[string]string{
			{"role": "user", "content": "ping"},
		},
	})
	resp, err = liveHTTP.Post(base+"/chat/completions", "application/json", bytes.NewReader(chatBody))
	if err != nil {
		t.Fatal(err)
	}
	b, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("chat %d %s", resp.StatusCode, b)
	}
	var chat map[string]any
	if err := json.Unmarshal(b, &chat); err != nil {
		t.Fatal(err)
	}
	if chat["id"] != "chatcmpl-live" {
		t.Fatalf("chat resp: %s", b)
	}
	t.Logf("chat OK: %s", b)

	// unknown model should 400
	bad, _ := json.Marshal(map[string]any{"model": "no-such", "messages": []any{}})
	resp, err = liveHTTP.Post(base+"/chat/completions", "application/json", bytes.NewReader(bad))
	if err != nil {
		t.Fatal(err)
	}
	b, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode == 200 {
		t.Fatalf("expected error for unknown model, got %s", b)
	}
	t.Logf("unknown model correctly rejected: %d %s", resp.StatusCode, b)

	st := px.status()
	if !st.Running {
		t.Fatal("status not running")
	}
	t.Logf("status baseURL=%s logs=%d", st.BaseURL, len(st.Logs))
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [16]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
