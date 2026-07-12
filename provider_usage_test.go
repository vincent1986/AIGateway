package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDeleteProviderPersistsFilteredList(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	app := NewApp()
	in := []Provider{
		{ID: "one", Name: "One", BaseURL: "https://one.example/v1", APIFormat: "openai"},
		{ID: "two", Name: "Two", BaseURL: "https://two.example/v1", APIFormat: "anthropic"},
	}
	if err := app.SaveProviders(in); err != nil {
		t.Fatal(err)
	}
	out, err := app.DeleteProvider("one")
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 || out[0].ID != "two" {
		t.Fatalf("unexpected providers: %#v", out)
	}
	if !fileExists(filepath.Join(home, ".codex-manager", "providers.json")) {
		t.Fatal("providers store not written")
	}
	if out[0].APIFormat != APIFormatAnthropicMessages {
		t.Fatalf("format not normalized: %s", out[0].APIFormat)
	}
}

func TestManagedToolWritersPreserveUnrelatedConfig(t *testing.T) {
	cases := []struct {
		kind                 ToolKind
		name, initial, model string
		checks               []string
	}{
		{ToolClaudeDesktop, "claude_desktop_config.json", `{"mcpServers":{"keep":{}}}`, "claude-sonnet-4-6", []string{`"mcpServers"`, `"inferenceProvider": "gateway"`, `"inferenceModels"`}},
		{ToolGemini, "settings.json", `{"ui":{"theme":"dark"}}`, "gemini-2.5-pro", []string{`"theme": "dark"`, `"name": "gemini-2.5-pro"`}},
		{ToolOpenCode, "opencode.json", `{"permission":{"bash":"ask"}}`, "gpt-test", []string{`"permission"`, `"model": "custom/gpt-test"`, `"baseURL": "https://gateway.example/v1"`}},
		{ToolOpenClaw, "openclaw.json", "{// comment\nchannels: { telegram: { enabled: true, }, },\n}", "gpt-test", []string{`"channels"`, `"primary": "custom/gpt-test"`, `"baseUrl": "https://gateway.example/v1"`}},
		{ToolHermes, "config.yaml", "terminal:\n  backend: local\n", "gpt-test", []string{"terminal:", "default: gpt-test", "provider: custom", "base_url: https://gateway.example/v1"}},
	}
	app := NewApp()
	for _, tc := range cases {
		t.Run(string(tc.kind), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), tc.name)
			if err := os.WriteFile(path, []byte(tc.initial), 0o600); err != nil {
				t.Fatal(err)
			}
			_, err := app.ApplyToolModel(ModelApplyRequest{Kind: string(tc.kind), Path: path, Model: tc.model, Provider: "custom", BaseURL: "https://gateway.example/v1", APIKey: "secret", Name: "Test Model"})
			if err != nil {
				t.Fatal(err)
			}
			body, _ := os.ReadFile(path)
			for _, check := range tc.checks {
				if !strings.Contains(string(body), check) {
					t.Fatalf("missing %q in:\n%s", check, body)
				}
			}
			if mode := fileMode(path); mode != 0o600 {
				t.Fatalf("mode changed to %o", mode)
			}
		})
	}
}

func fileMode(path string) os.FileMode { info, _ := os.Stat(path); return info.Mode().Perm() }

func TestPreviewGeminiReportsSettingsAndEnvWithoutWriting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	before := `{"ui":{"theme":"dark"}}`
	if err := os.WriteFile(path, []byte(before), 0o600); err != nil {
		t.Fatal(err)
	}
	results, err := NewApp().PreviewApplyToolModel(ModelApplyRequest{Kind: string(ToolGemini), Path: path, Model: "gemini-test", BaseURL: "https://gemini.example", APIKey: "secret"})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 || !results[0].Changed || !results[1].Changed {
		t.Fatalf("preview results: %#v", results)
	}
	body, _ := os.ReadFile(path)
	if string(body) != before || fileExists(filepath.Join(dir, ".env")) {
		t.Fatal("dry-run modified Gemini files")
	}
}

func TestMultiToolConfigDiscoveryParsers(t *testing.T) {
	cases := []struct {
		kind                        ToolKind
		name, body, model, provider string
	}{
		{ToolGemini, ".env", "GEMINI_MODEL=gemini-2.5-pro\nGOOGLE_GEMINI_BASE_URL=https://gemini.example/v1\n", "gemini-2.5-pro", "https://gemini.example/v1"},
		{ToolOpenClaw, "openclaw.json", "{agents:{defaults:{model:{primary:\"custom/claw-model\"}}},models:{providers:{custom:{baseUrl:\"https://claw.example/v1\"}}}}", "claw-model", "https://claw.example/v1"},
		{ToolHermes, "config.yaml", "model:\n  default: hermes-model\n  provider: custom\n  base_url: https://hermes.example/v1\n", "hermes-model", "https://hermes.example/v1"},
	}
	app := NewApp()
	for _, tc := range cases {
		t.Run(string(tc.kind), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), tc.name)
			if err := os.WriteFile(path, []byte(tc.body), 0o600); err != nil {
				t.Fatal(err)
			}
			st := ToolConfigStatus{Kind: string(tc.kind), Path: path}
			app.fillFromFile(&st)
			if st.Model != tc.model || st.ModelProvider != tc.provider {
				t.Fatalf("parsed %#v", st)
			}
		})
	}
}

func TestEnableChatStreamUsage(t *testing.T) {
	out := string(enableChatStreamUsage([]byte(`{"model":"demo","messages":[]}`)))
	if !strings.Contains(out, `"stream":true`) || !strings.Contains(out, `"include_usage":true`) {
		t.Fatalf("stream options missing: %s", out)
	}
}

func TestUsageFromJSONBodyAndClear(t *testing.T) {
	prompt, completion, total := usageFromJSONBody([]byte(`{"usage":{"prompt_tokens":11,"completion_tokens":7,"total_tokens":18}}`))
	if prompt != 11 || completion != 7 || total != 18 {
		t.Fatalf("usage = %d %d %d", prompt, completion, total)
	}
	proxy := newProxyServer()
	proxy.recordRequest(ProxyRequestLog{Status: 200, LatencyMs: 20, PromptTokens: 11, CompletionTokens: 7, TotalTokens: 18})
	if proxy.status().Usage.TotalTokens != 18 {
		t.Fatalf("usage not recorded: %#v", proxy.status().Usage)
	}
	proxy.clearUsageStats()
	st := proxy.status()
	if st.Usage.TotalRequests != 0 || st.Usage.TotalTokens != 0 || len(st.Requests) != 0 {
		t.Fatalf("usage not cleared: %#v", st)
	}
}
