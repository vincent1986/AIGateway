package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestIsGatewayVirtualModel(t *testing.T) {
	for _, id := range []string{"aiSwitchModel", "AISWITCHMODEL", "aigateway", "default", "aigateway/aiSwitchModel"} {
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

func TestVirtualModelRoutesToActive(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)
	closeDB()
	defer closeDB()

	a := NewApp()
	useProxy := true
	_, err := a.UpsertProvider(Provider{
		ID: "p1", Name: "Demo", BaseURL: "https://api.example.com/v1", APIKey: "sk",
		UseProxy: &useProxy,
		Models: []ProviderModel{
			{ID: "model-a", Name: "A", Enabled: true, IsDefault: true},
			{ID: "model-b", Name: "B", Enabled: true},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	// clear auto-seeded ollama for cleaner asserts if present
	_ = a.SaveProviders([]Provider{{
		ID: "p1", Name: "Demo", BaseURL: "https://api.example.com/v1", APIKey: "sk",
		UseProxy: &useProxy,
		Models: []ProviderModel{
			{ID: "model-a", Name: "A", Enabled: true, IsDefault: true},
			{ID: "model-b", Name: "B", Enabled: true},
		},
	}})

	info, err := a.SetActiveGatewayModel("model-b")
	if err != nil {
		t.Fatal(err)
	}
	if info.ActiveModel != "model-b" {
		t.Fatalf("active=%s", info.ActiveModel)
	}
	if info.VirtualModel != gatewayVirtualModel {
		t.Fatalf("virtual=%s", info.VirtualModel)
	}

	cands, err := resolveRoutesForModel(gatewayVirtualModel)
	if err != nil {
		t.Fatal(err)
	}
	if len(cands) == 0 {
		t.Fatal("no candidates")
	}
	if cands[0].UpstreamModel != "model-b" {
		t.Fatalf("upstream=%s want model-b", cands[0].UpstreamModel)
	}
	if cands[0].Provider.BaseURL != "https://api.example.com/v1" {
		t.Fatalf("base=%s", cands[0].Provider.BaseURL)
	}

	// switch to model-a without touching configs
	if _, err := a.SetActiveGatewayModel("model-a"); err != nil {
		t.Fatal(err)
	}
	cands, err = resolveRoutesForModel("aigateway")
	if err != nil {
		t.Fatal(err)
	}
	if cands[0].UpstreamModel != "model-a" {
		t.Fatalf("upstream=%s want model-a", cands[0].UpstreamModel)
	}
}

func TestHandleModelsIncludesVirtual(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)
	closeDB()
	defer closeDB()

	a := NewApp()
	_, _ = a.UpsertProvider(Provider{
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
	found := false
	for _, d := range out.Data {
		if d.ID == gatewayVirtualModel {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing virtual model in list: %+v", out.Data)
	}
}

func TestInjectClaudePinsVirtualModel(t *testing.T) {
	out, err := injectGatewayClaude(`{}`, "http://127.0.0.1:18080/v1")
	if err != nil {
		t.Fatal(err)
	}
	var root map[string]any
	if err := json.Unmarshal([]byte(out), &root); err != nil {
		t.Fatal(err)
	}
	if root["model"] != gatewayVirtualModel {
		t.Fatalf("model=%v", root["model"])
	}
	env := root["env"].(map[string]any)
	if env["ANTHROPIC_MODEL"] != gatewayVirtualModel {
		t.Fatalf("ANTHROPIC_MODEL=%v", env["ANTHROPIC_MODEL"])
	}
}
