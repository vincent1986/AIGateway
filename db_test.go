package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSQLiteProvidersRoundTrip(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)
	closeDB()
	defer closeDB()

	list := []Provider{
		{
			ID: "p1", Name: "DeepSeek", BaseURL: "https://api.deepseek.com/v1", APIKey: "sk-ds", Color: "#3fb950",
			Models: []ProviderModel{
				{ID: "deepseek-chat", Name: "chat", Enabled: true, IsDefault: true},
			},
		},
		{
			ID: "p2", Name: "Silicon", BaseURL: "https://api.siliconflow.cn/v1", APIKey: "sk-sf",
			Models: []ProviderModel{
				{ID: "deepseek-chat", Name: "chat-sf", Enabled: true},
			},
		},
	}
	if err := saveProvidersToDisk(list); err != nil {
		t.Fatal(err)
	}
	// DB file exists
	if _, err := os.Stat(filepath.Join(tmp, ".codex-manager", "aigateway.db")); err != nil {
		t.Fatal(err)
	}
	got, err := loadProvidersFromDisk()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("providers=%d", len(got))
	}

	// model group aggregated
	groups, err := (&App{}).ListModelGroups()
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, g := range groups {
		if g.ID == "deepseek-chat" {
			found = true
			if len(g.Routes) != 2 {
				t.Fatalf("routes=%d want 2", len(g.Routes))
			}
			// priority ordered
			if g.Routes[0].Priority > g.Routes[1].Priority {
				t.Fatalf("priority order wrong: %+v", g.Routes)
			}
		}
	}
	if !found {
		t.Fatal("missing model group deepseek-chat")
	}

	// resolve routes order
	cands, err := resolveRoutesForModel("deepseek-chat")
	if err != nil {
		t.Fatal(err)
	}
	if len(cands) < 2 {
		t.Fatalf("cands=%d", len(cands))
	}
	if cands[0].Provider.Name != "DeepSeek" {
		t.Fatalf("first route=%s", cands[0].Provider.Name)
	}
}

func TestJSONMigrateToSQLite(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)
	closeDB()
	defer closeDB()

	// write legacy JSON only
	list := []Provider{
		{
			ID: "ollama", Name: "Ollama", BaseURL: "http://127.0.0.1:11434/v1", APIKey: "ollama",
			Models: []ProviderModel{{ID: "llama3", Enabled: true}},
		},
	}
	if err := saveProvidersJSONFile(list); err != nil {
		t.Fatal(err)
	}
	// openDB migrates
	db, err := openDB()
	if err != nil {
		t.Fatal(err)
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(1) FROM providers`).Scan(&n); err != nil || n < 1 {
		t.Fatalf("migrated n=%d err=%v", n, err)
	}
	got, err := loadProvidersFromDisk()
	if err != nil || len(got) < 1 {
		t.Fatalf("%v %+v", err, got)
	}
}

func TestFailoverHelpers(t *testing.T) {
	if !isFailoverStatus(429) || !isFailoverStatus(401) {
		t.Fatal("status")
	}
	if !isFailoverBody([]byte(`{"error":{"code":"insufficient_quota"}}`)) {
		t.Fatal("body")
	}
	m := exhaustedErrorJSON("x")
	if m["error"].(map[string]any)["code"] != "model_group_all_exhausted" {
		t.Fatal(m)
	}
}
