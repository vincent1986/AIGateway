package main

import (
	"database/sql"
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

func TestProxyAliasRoutesToTargetModelGroup(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)
	closeDB()
	defer closeDB()

	if err := saveProvidersToDisk([]Provider{{
		ID: "p1", Name: "DeepSeek", BaseURL: "https://api.deepseek.com/v1", APIKey: "sk-ds",
		Models: []ProviderModel{{ID: "deepseek-chat", Enabled: true}},
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := (&App{}).SetAppProxyModel("claude", "deepseek-chat"); err != nil {
		t.Fatal(err)
	}
	cands, err := resolveRoutesForModel(appProxyModel(ToolClaude))
	if err != nil {
		t.Fatal(err)
	}
	if len(cands) != 1 || cands[0].UpstreamModel != "deepseek-chat" {
		t.Fatalf("bad alias routes: %+v", cands)
	}
	cands, err = resolveRoutesForModel(appProxyModel(ToolClaude) + "[1m]")
	if err != nil || len(cands) != 1 || cands[0].UpstreamModel != "deepseek-chat" {
		t.Fatalf("extended-context alias routes: %v %+v", err, cands)
	}
}

func TestSetAppProxyModelRejectsProxyAliasTarget(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)
	closeDB()
	defer closeDB()

	if _, err := (&App{}).SetAppProxyModel("claude", appProxyModel(ToolClaude)); err == nil {
		t.Fatal("expected proxy alias target to be rejected")
	}
}

func TestSetAppProxyModelRejectsUnknownTarget(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)
	closeDB()
	defer closeDB()

	if _, err := (&App{}).SetAppProxyModel("claude", "missing-model"); err == nil {
		t.Fatal("expected unknown model group to be rejected")
	}
}

func TestClearUsageStatsMigratesLegacyRoutesWithoutUsedTokens(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)
	closeDB()
	defer closeDB()

	if err := os.MkdirAll(filepath.Join(tmp, ".codex-manager"), 0o755); err != nil {
		t.Fatal(err)
	}
	raw, err := sql.Open("sqlite", "file:"+filepath.Join(tmp, ".codex-manager", "aigateway.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(`
CREATE TABLE meta (key TEXT PRIMARY KEY, value TEXT NOT NULL);
CREATE TABLE model_group_routes (
  id TEXT PRIMARY KEY,
  group_id TEXT NOT NULL,
  provider_id TEXT NOT NULL,
  provider_model_id TEXT NOT NULL,
  priority INTEGER NOT NULL DEFAULT 100,
  enabled INTEGER NOT NULL DEFAULT 1,
  status TEXT NOT NULL DEFAULT 'ok'
);
CREATE TABLE usage_events (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  time TEXT NOT NULL,
  provider_name TEXT NOT NULL DEFAULT '',
  model TEXT NOT NULL DEFAULT '',
  endpoint TEXT NOT NULL DEFAULT '',
  status INTEGER NOT NULL DEFAULT 0,
  input_tokens INTEGER NOT NULL DEFAULT 0,
  output_tokens INTEGER NOT NULL DEFAULT 0,
  total_tokens INTEGER NOT NULL DEFAULT 0
);
INSERT INTO model_group_routes(id, group_id, provider_id, provider_model_id)
VALUES('r1', 'deepseek-chat', 'p1', 'deepseek-chat');
INSERT INTO usage_events(time, provider_name, model, endpoint, status, input_tokens, output_tokens, total_tokens)
VALUES('2026-07-12T00:00:00Z', 'DeepSeek', 'deepseek-chat', 'chat/completions', 200, 1, 2, 3);
`); err != nil {
		_ = raw.Close()
		t.Fatal(err)
	}
	if err := raw.Close(); err != nil {
		t.Fatal(err)
	}

	if _, err := (&App{}).ClearUsageStats(); err != nil {
		t.Fatal(err)
	}
	db, err := openDB()
	if err != nil {
		t.Fatal(err)
	}
	var used int
	if err := db.QueryRow(`SELECT used_tokens FROM model_group_routes WHERE id = 'r1'`).Scan(&used); err != nil {
		t.Fatal(err)
	}
	if used != 0 {
		t.Fatalf("used_tokens=%d", used)
	}
}
