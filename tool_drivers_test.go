package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInjectJSONBaseURL(t *testing.T) {
	tmp := t.TempDir()
	p := filepath.Join(tmp, "config.json")
	if err := injectJSONBaseURL(p, "http://127.0.0.1:18080/v1", "sk-x"); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if !strings.Contains(s, "127.0.0.1:18080") {
		t.Fatalf("missing gateway: %s", s)
	}
}

func TestDriverRegistry(t *testing.T) {
	for _, id := range []string{"chatgpt", "claude", "openclaw", "harness", "grok", "codex"} {
		if driverByID(id) == nil {
			t.Fatalf("missing driver %s", id)
		}
	}
	if driverByID("chatgpt").ToolName() != "ChatGPT" {
		t.Fatal("name")
	}
}

func TestDriverConfigAdaptersInspectAndValidate(t *testing.T) {
	cases := []struct {
		id      string
		path    string
		content string
		model   string
	}{
		{
			id:      "chatgpt",
			path:    "config.toml",
			content: "model = \"m1\"\nmodel_provider = \"p1\"\n",
			model:   "m1",
		},
		{
			id:      "claude",
			path:    "settings.json",
			content: `{"model":"m2","env":{"ANTHROPIC_MODEL":"m2"}}`,
			model:   "m2",
		},
		{
			id:      "openclaw",
			path:    "openclaw.json",
			content: `{"agents":{"defaults":{"model":{"primary":"aigateway/m3"}}}}`,
			model:   "m3",
		},
		{
			id:      "harness",
			path:    "config.yaml",
			content: "model: m4\nprovider: aigateway\n",
			model:   "m4",
		},
		{
			id:      "grok",
			path:    "config.toml",
			content: "[model.my-local-model]\nmodel = \"m5\"\nbase_url = \"http://127.0.0.1:8000/v1\"\nname = \"Local\"\n\n[models]\ndefault = \"my-local-model\"\n",
			model:   "my-local-model",
		},
		{
			id:      "grok",
			path:    "config.toml",
			content: "[model.\"gpt-4.1\"]\nmodel = \"gpt-4.1\"\nbase_url = \"http://127.0.0.1:8000/v1\"\n\n[models]\ndefault = \"gpt-4.1\"\n",
			model:   "gpt-4.1",
		},
	}
	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			d := driverByID(tc.id)
			view, err := d.InspectConfig([]byte(tc.content), tc.path)
			if err != nil {
				t.Fatal(err)
			}
			if view.Model != tc.model {
				t.Fatalf("model=%q want %q", view.Model, tc.model)
			}
			if err := d.ValidateConfigContent([]byte(tc.content), tc.path, ExpectedConfig{
				Model:        tc.model,
				RequireModel: true,
			}); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestDriverApplyModelUsesFormatSpecificAdapters(t *testing.T) {
	cases := []struct {
		id      string
		path    string
		initial string
		req     ModelInjection
		check   func(t *testing.T, out []byte)
	}{
		{
			id:      "chatgpt",
			path:    "config.toml",
			initial: "model = \"old\"\nmodel_provider = \"old\"\n",
			req:     ModelInjection{Model: "m1", Provider: "p1", BaseURL: "https://example.com/v1", APIKey: "key"},
			check: func(t *testing.T, out []byte) {
				if got := readTomlTopLevelString(string(out), "model"); got != "m1" {
					t.Fatalf("model=%q", got)
				}
			},
		},
		{
			id:      "claude",
			path:    "settings.json",
			initial: "{}",
			req:     ModelInjection{Model: "m2", Provider: "custom", BaseURL: "https://example.com", APIKey: "key"},
			check: func(t *testing.T, out []byte) {
				var root map[string]any
				if err := json.Unmarshal(out, &root); err != nil {
					t.Fatal(err)
				}
				if root["model"] != "m2" {
					t.Fatalf("root=%v", root)
				}
			},
		},
		{
			id:      "openclaw",
			path:    "openclaw.json",
			initial: "{}",
			req:     ModelInjection{Model: "m3", Provider: "aigateway", BaseURL: "http://127.0.0.1:18080/v1"},
			check: func(t *testing.T, out []byte) {
				if _, provider := readOpenClawModel(string(out)); provider != "aigateway" {
					t.Fatalf("provider=%q output=%s", provider, out)
				}
			},
		},
		{
			id:      "harness",
			path:    "config.yaml",
			initial: "model: old\n",
			req:     ModelInjection{Model: "m4", Provider: "aigateway", BaseURL: "http://127.0.0.1:18080/v1"},
			check: func(t *testing.T, out []byte) {
				if model, _ := readGenericToolModel(string(out), "config.yaml"); model != "m4" {
					t.Fatalf("model=%q output=%s", model, out)
				}
			},
		},
		{
			id:      "grok",
			path:    "config.toml",
			initial: "[model.old]\nmodel = \"old-backend\"\nbase_url = \"http://old/v1\"\n\n[models]\ndefault = \"old\"\n",
			req:     ModelInjection{Model: "m5", Provider: "aigateway", BaseURL: "http://127.0.0.1:18080/v1"},
			check: func(t *testing.T, out []byte) {
				model, provider := readGrokModelConfig(string(out))
				if model != "m5" || provider != "http://127.0.0.1:18080/v1" {
					t.Fatalf("model=%q provider=%q output=%s", model, provider, out)
				}
				if !strings.Contains(string(out), `[model."m5"]`) || !strings.Contains(string(out), `[models]`) {
					t.Fatalf("missing official Grok schema: %s", out)
				}
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			d := driverByID(tc.id)
			out, err := d.ApplyModel([]byte(tc.initial), tc.path, tc.req)
			if err != nil {
				t.Fatal(err)
			}
			tc.check(t, out)
		})
	}
}

func TestIsManagedIgnoresLocalOllamaAndDetectsGatewayMarkers(t *testing.T) {
	tmp := t.TempDir()
	ollama := filepath.Join(tmp, "ollama.json")
	managed := filepath.Join(tmp, "managed.json")
	if err := os.WriteFile(ollama, []byte(`{"env":{"OPENAI_BASE_URL":"http://127.0.0.1:11434/v1"},"model":"llama3"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(managed, []byte(`{"env":{"OPENAI_BASE_URL":"http://127.0.0.1:19090/v1"},"model":"aiSwitchModel-claude"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if (&claudeDriver{}).IsManaged(ollama) {
		t.Fatal("local Ollama URL must not count as AIGateway managed")
	}
	if !(&claudeDriver{}).IsManaged(managed) {
		t.Fatal("aiSwitchModel alias should count as AIGateway managed even on non-default port")
	}
	if (&openclawDriver{}).IsManaged(ollama) || (&harnessDriver{}).IsManaged(ollama) || (&grokDriver{}).IsManaged(ollama) {
		t.Fatal("Ollama false-positive across drivers")
	}
}

func TestGrokDriverInjectGateway(t *testing.T) {
	tmp := t.TempDir()
	p := filepath.Join(tmp, "config.toml")
	if err := (&grokDriver{}).InjectGateway(p, "http://127.0.0.1:18080/v1", ""); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	model, provider := readGrokModelConfig(string(b))
	if model != appProxyModel(ToolGrok) || provider != "http://127.0.0.1:18080/v1" {
		t.Fatalf("model=%q provider=%q output=%s", model, provider, b)
	}
	if !strings.Contains(string(b), `[model."`+appProxyModel(ToolGrok)+`"]`) || !strings.Contains(string(b), `[models]`) || strings.Contains(string(b), "[model_providers.aigateway]") {
		t.Fatalf("missing OpenAI-compatible provider block: %s", b)
	}
}

func TestInjectGatewayCodex(t *testing.T) {
	out, err := injectGatewayCodex("", "http://127.0.0.1:18080/v1")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "aigateway") {
		t.Fatal(out)
	}
}

func TestInjectOpenClawGatewayUsesOpenClawSchema(t *testing.T) {
	tmp := t.TempDir()
	p := filepath.Join(tmp, "openclaw.json")
	if err := injectOpenClawGateway(p, "http://127.0.0.1:18080/v1", "", appProxyModel(ToolOpenClaw), "aigateway"); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	var root map[string]any
	if err := json.Unmarshal(b, &root); err != nil {
		t.Fatal(err)
	}
	models := root["models"].(map[string]any)
	if models["mode"] != "merge" {
		t.Fatalf("mode=%v", models["mode"])
	}
	providers := models["providers"].(map[string]any)
	pv := providers["aigateway"].(map[string]any)
	if pv["baseUrl"] != "http://127.0.0.1:18080/v1" {
		t.Fatalf("provider=%v", pv)
	}
	if pv["apiKey"] != "aigateway" || pv["api"] != "openai-completions" {
		t.Fatalf("provider schema=%v", pv)
	}
	if _, ok := pv["base_url"]; ok {
		t.Fatalf("legacy base_url was not removed: %v", pv)
	}
	agents := root["agents"].(map[string]any)
	defaults := agents["defaults"].(map[string]any)
	model := defaults["model"].(map[string]any)
	if model["primary"] != "aigateway/"+appProxyModel(ToolOpenClaw) {
		t.Fatalf("primary=%v", model["primary"])
	}
}

func TestInjectOpenClawGatewayAcceptsJSON5AndPreservesProviderFields(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	tmp := t.TempDir()
	p := filepath.Join(tmp, "openclaw.json")
	original := "{\n  // JSON5 comment\n  models: { providers: { aigateway: { headers: { 'x-test': 'keep' }, models: [{ id: 'old' }] } } },\n}\n"
	if err := os.WriteFile(p, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := injectOpenClawGateway(p, "http://127.0.0.1:18080/v1", "", "m", "aigateway"); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	var root map[string]any
	if err := json.Unmarshal(b, &root); err != nil {
		t.Fatal(err)
	}
	pv := root["models"].(map[string]any)["providers"].(map[string]any)["aigateway"].(map[string]any)
	if pv["headers"].(map[string]any)["x-test"] != "keep" {
		t.Fatalf("provider headers lost: %+v", pv)
	}
	models := pv["models"].([]any)
	if len(models) < 2 {
		t.Fatalf("provider models not preserved: %+v", models)
	}
	if _, provider := readOpenClawModel(string(b)); provider != "aigateway" {
		t.Fatalf("provider=%q", provider)
	}
}

func TestInjectOpenClawGatewayRemovesLegacyRootAndProviderKeys(t *testing.T) {
	p := filepath.Join(t.TempDir(), "openclaw.json")
	legacy := `{"apiBaseUrl":"http://old/v1","baseUrl":"http://old/v1","base_url":"http://old/v1","openai":{},"models":{"providers":{"aigateway":{"base_url":"http://old/v1","api_key":"old","api":"openai"}}}}`
	if err := os.WriteFile(p, []byte(legacy), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := injectOpenClawGateway(p, "http://127.0.0.1:18080/v1", "", "m", "aigateway"); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	var root map[string]any
	if err := json.Unmarshal(b, &root); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"apiBaseUrl", "baseUrl", "base_url", "openai"} {
		if _, ok := root[key]; ok {
			t.Fatalf("legacy root key %q remains: %s", key, b)
		}
	}
	pv := root["models"].(map[string]any)["providers"].(map[string]any)["aigateway"].(map[string]any)
	if pv["api"] != "openai-completions" || pv["baseUrl"] != "http://127.0.0.1:18080/v1" {
		t.Fatalf("provider=%v", pv)
	}
	for _, key := range []string{"base_url", "api_key"} {
		if _, ok := pv[key]; ok {
			t.Fatalf("legacy provider key %q remains: %s", key, b)
		}
	}
}

func TestInjectOpenClawGatewayRejectsMalformedJSON5(t *testing.T) {
	p := filepath.Join(t.TempDir(), "openclaw.json")
	if err := os.WriteFile(p, []byte("{ models: [ }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := injectOpenClawGateway(p, "http://127.0.0.1:18080/v1", "", "m", "p"); err == nil {
		t.Fatal("expected malformed JSON5 error")
	}
}

func TestInjectGatewayClaudeRejectsInvalidJSON(t *testing.T) {
	original := "{\n  // invalid json\n  \"env\": {}\n}\n"
	if _, err := injectGatewayClaude(original, "http://127.0.0.1:18080/v1"); err == nil {
		t.Fatal("expected invalid JSON error")
	}
}

func TestInjectGatewayClaudeNormalizesLocalProxyBase(t *testing.T) {
	out, err := injectGatewayClaude(`{"env":{}}`, "http://127.0.0.1:18080/v1")
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatal(err)
	}
	env := doc["env"].(map[string]any)
	if got := env["ANTHROPIC_BASE_URL"]; got != "http://127.0.0.1:18080" {
		t.Fatalf("anthropic base=%v", got)
	}
	if got := doc["apiBaseUrl"]; got != "http://127.0.0.1:18080" {
		t.Fatalf("api base=%v", got)
	}
}
