package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestParseModelsOpenAI(t *testing.T) {
	body := []byte(`{"data":[{"id":"gpt-4o","owned_by":"openai"},{"id":"gpt-4o-mini","owned_by":"openai"}]}`)
	items, err := parseModelsResponse(body)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[0].ID != "gpt-4o" {
		t.Fatalf("%+v", items)
	}
}

func TestFetchProviderModels(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("Authorization") != "Bearer sk-test" {
			http.Error(w, "unauthorized", 401)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]string{
				{"id": "m1", "owned_by": "x"},
			},
		})
	}))
	defer srv.Close()

	a := NewApp()
	items, err := a.FetchProviderModels(srv.URL+"/v1", "sk-test")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != "m1" {
		t.Fatalf("%+v", items)
	}
}

func TestProvidersPersist(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)
	closeDB()
	defer closeDB()

	a := NewApp()
	// empty store seeds Ollama
	seeded, err := a.ListProviders()
	if err != nil {
		t.Fatal(err)
	}
	if len(seeded) < 1 || seeded[0].Name != "Ollama" {
		t.Fatalf("expected default Ollama, got %+v", seeded)
	}
	if providerWantsProxy(seeded[0]) {
		t.Fatal("Ollama should not use proxy by default")
	}

	useProxy := true
	list, err := a.UpsertProvider(Provider{
		ID:       "p1",
		Name:     "Demo",
		BaseURL:  "https://api.example.com/v1",
		APIKey:   "sk",
		Color:    "#fff",
		UseProxy: &useProxy,
		Models:   []ProviderModel{{ID: "m", Name: "M", Enabled: true, IsDefault: true}},
		TokenPackages: []TokenPackage{{
			ID: "pkg1", Name: "100万", TotalTokens: 1_000_000, Active: true, Currency: "CNY",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(list) < 2 {
		t.Fatalf("len=%d want ollama+demo", len(list))
	}
	loaded, err := a.ListProviders()
	if err != nil {
		t.Fatal(err)
	}
	var demo *Provider
	for i := range loaded {
		if loaded[i].ID == "p1" {
			demo = &loaded[i]
		}
	}
	if demo == nil || demo.Name != "Demo" || !providerWantsProxy(*demo) {
		t.Fatalf("demo missing or proxy flag wrong: %+v", demo)
	}
	loaded, err = a.DeleteProvider("p1")
	if err != nil {
		t.Fatal(err)
	}
	// Ollama remains
	if len(loaded) < 1 {
		t.Fatalf("expected ollama remain, got %d", len(loaded))
	}
}

func TestDeleteLastProviderDoesNotReseedOllama(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)
	closeDB()
	defer closeDB()

	a := NewApp()
	seeded, err := a.ListProviders()
	if err != nil {
		t.Fatal(err)
	}
	if len(seeded) != 1 || seeded[0].ID != "ollama" {
		t.Fatalf("seeded=%+v", seeded)
	}
	after, err := a.DeleteProvider("ollama")
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != 0 {
		t.Fatalf("after delete=%+v", after)
	}
	loaded, err := a.ListProviders()
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 0 {
		t.Fatalf("ollama was reseeded after delete: %+v", loaded)
	}
}

func TestFetchOllamaAllowsEmptyKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]string{{"id": "llama3.2", "owned_by": "ollama"}},
		})
	}))
	defer srv.Close()
	// Pretend local by using 127.0.0.1 in URL — replace host
	// httptest URL is 127.0.0.1 already
	a := NewApp()
	items, err := a.FetchProviderModels(srv.URL+"/v1", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != "llama3.2" {
		t.Fatalf("%+v", items)
	}
}
