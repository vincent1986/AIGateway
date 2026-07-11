package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestIsGatewayVirtualModel(t *testing.T) {
	for _, id := range []string{
		"aiSwitchModel", "AISWITCHMODEL", "aigateway", "default",
		"aiSwitchModel-chatgpt", "aiSwitchModel-claude",
		"aiSwitchModel-openclaw", "aiSwitchModel-harness",
		"aigateway/aiSwitchModel-claude",
	} {
		if !isGatewayVirtualModel(id) {
			t.Fatalf("expected virtual: %s", id)
		}
	}
	for _, id := range []string{"deepseek-v4-pro", "gpt-4o", "aigateway/deepseek-v4-pro"} {
		if isGatewayVirtualModel(id) {
			t.Fatalf("not virtual: %s", id)
		}
	}
}

func TestPerToolActiveModelsIndependent(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)
	closeDB()
	defer closeDB()

	a := NewApp()
	useProxy := true
	_ = a.SaveProviders([]Provider{{
		ID: "p1", Name: "Demo", BaseURL: "https://api.example.com/v1", APIKey: "sk",
		UseProxy: &useProxy,
		Models: []ProviderModel{
			{ID: "model-a", Name: "A", Enabled: true, IsDefault: true},
			{ID: "model-b", Name: "B", Enabled: true},
			{ID: "model-c", Name: "C", Enabled: true},
		},
	}})

	if _, err := a.SetActiveGatewayModel("chatgpt", "model-a"); err != nil {
		t.Fatal(err)
	}
	if _, err := a.SetActiveGatewayModel("claude", "model-b"); err != nil {
		t.Fatal(err)
	}
	if _, err := a.SetActiveGatewayModel("openclaw", "model-c"); err != nil {
		t.Fatal(err)
	}

	// chatgpt route
	cands, err := resolveRoutesForModel(virtualModelForTool("chatgpt"))
	if err != nil {
		t.Fatal(err)
	}
	if cands[0].UpstreamModel != "model-a" {
		t.Fatalf("chatgpt up=%s", cands[0].UpstreamModel)
	}
	// claude independent
	cands, err = resolveRoutesForModel(virtualModelForTool("claude"))
	if err != nil {
		t.Fatal(err)
	}
	if cands[0].UpstreamModel != "model-b" {
		t.Fatalf("claude up=%s", cands[0].UpstreamModel)
	}
	// openclaw independent
	cands, err = resolveRoutesForModel("aigateway/" + virtualModelForTool("openclaw"))
	if err != nil {
		t.Fatal(err)
	}
	if cands[0].UpstreamModel != "model-c" {
		t.Fatalf("openclaw up=%s", cands[0].UpstreamModel)
	}

	// switch claude only — others unchanged
	if _, err := a.SetActiveGatewayModel("claude", "model-c"); err != nil {
		t.Fatal(err)
	}
	cands, _ = resolveRoutesForModel(virtualModelForTool("chatgpt"))
	if cands[0].UpstreamModel != "model-a" {
		t.Fatalf("chatgpt should stay model-a, got %s", cands[0].UpstreamModel)
	}
	cands, _ = resolveRoutesForModel(virtualModelForTool("claude"))
	if cands[0].UpstreamModel != "model-c" {
		t.Fatalf("claude should be model-c, got %s", cands[0].UpstreamModel)
	}

	list := a.ListActiveGatewayModels()
	if len(list) != 4 {
		t.Fatalf("list len=%d", len(list))
	}
}

func TestHandleModelsIncludesPerToolVirtual(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)
	closeDB()
	defer closeDB()

	_, _ = NewApp().UpsertProvider(Provider{
		ID: "p1", Name: "Demo", BaseURL: "https://api.example.com/v1", APIKey: "sk",
		Models: []ProviderModel{{ID: "m1", Name: "M1", Enabled: true}},
	})

	p := newProxyServer()
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/models", p.handleModels)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/v1/models")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	need := map[string]bool{
		gatewayVirtualModelChatGPT:  false,
		gatewayVirtualModelClaude:   false,
		gatewayVirtualModelOpenClaw: false,
		gatewayVirtualModelHarness:  false,
	}
	for _, d := range out.Data {
		if _, ok := need[d.ID]; ok {
			need[d.ID] = true
		}
	}
	for id, ok := range need {
		if !ok {
			t.Fatalf("missing virtual id %s in %+v", id, out.Data)
		}
	}
}

func TestInjectClaudePinsClaudeVirtualModel(t *testing.T) {
	out, err := injectGatewayClaude(`{}`, "http://127.0.0.1:18080/v1")
	if err != nil {
		t.Fatal(err)
	}
	var root map[string]any
	if err := json.Unmarshal([]byte(out), &root); err != nil {
		t.Fatal(err)
	}
	want := virtualModelForTool(toolKeyClaude)
	if root["model"] != want {
		t.Fatalf("model=%v want %s", root["model"], want)
	}
	env := root["env"].(map[string]any)
	if env["ANTHROPIC_MODEL"] != want {
		t.Fatalf("ANTHROPIC_MODEL=%v", env["ANTHROPIC_MODEL"])
	}
}

func TestInjectCodexPinsChatGPTVirtualModel(t *testing.T) {
	out, err := injectGatewayCodex("", "http://127.0.0.1:18080/v1")
	if err != nil {
		t.Fatal(err)
	}
	want := virtualModelForTool(toolKeyChatGPT)
	if !containsStr(out, `model = "`+want+`"`) {
		t.Fatalf("missing model pin:\n%s", out)
	}
}

func containsStr(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		(len(s) > 0 && (func() bool {
			for i := 0; i+len(sub) <= len(s); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
			return false
		})()))
}
