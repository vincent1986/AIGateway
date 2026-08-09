package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type takeoverCase struct {
	kind         string
	path         string
	original     string
	managedProbe string
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestDefaultBackupAndRestore(t *testing.T) {
	// isolate manager root by using temp HOME
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	// also USERPROFILE for expand on some systems
	t.Setenv("USERPROFILE", tmp)

	cfgDir := filepath.Join(tmp, ".codex")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cfgPath := filepath.Join(cfgDir, "config.toml")
	original := "model = \"orig-model\"\nmodel_provider = \"p1\"\n"
	if err := os.WriteFile(cfgPath, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	// first ensure creates backup
	created, err := ensureDefaultBackup(ToolCodex, cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if !created {
		t.Fatal("expected backup created")
	}
	// second call is no-op
	created, err = ensureDefaultBackup(ToolCodex, cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if created {
		t.Fatal("should not recreate default backup")
	}

	// modify live file
	if err := os.WriteFile(cfgPath, []byte("model = \"changed\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	a := NewApp()
	st, err := a.RestoreDefaultConfig("codex")
	if err != nil {
		t.Fatal(err)
	}
	if !st.HasDefaultBackup {
		t.Fatal("expected hasDefaultBackup")
	}
	b, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != original {
		t.Fatalf("restored content = %q want %q", string(b), original)
	}

	// manual backup update
	if err := os.WriteFile(cfgPath, []byte("model = \"new-default\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	st, err = a.BackupDefaultConfig("codex", cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if !st.HasDefaultBackup {
		t.Fatal("expected backup after manual")
	}
	if err := os.WriteFile(cfgPath, []byte("model = \"tmp\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	st, err = a.RestoreDefaultConfig("codex")
	if err != nil {
		t.Fatal(err)
	}
	b, _ = os.ReadFile(cfgPath)
	if string(b) != "model = \"new-default\"\n" {
		t.Fatalf("got %q", string(b))
	}

	st, err = a.ClearDefaultBackup("codex")
	if err != nil {
		t.Fatal(err)
	}
	if st.HasDefaultBackup {
		t.Fatal("backup should be cleared")
	}
}

func TestDefaultBackupsAreIsolatedByConfigPath(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)
	first := filepath.Join(tmp, "one", "settings.json")
	second := filepath.Join(tmp, "two", "settings.json")
	for path, content := range map[string]string{first: "{\"model\":\"one\"}\n", second: "{\"model\":\"two\"}\n"} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		if created, err := ensureDefaultBackup(ToolOpenClaw, path); err != nil || !created {
			t.Fatalf("backup %s created=%v err=%v", path, created, err)
		}
	}
	m1, ok := loadBackupMeta(ToolOpenClaw, first)
	if !ok {
		t.Fatal("missing first backup")
	}
	m2, ok := loadBackupMeta(ToolOpenClaw, second)
	if !ok {
		t.Fatal("missing second backup")
	}
	if m1.BackupPath == m2.BackupPath {
		t.Fatalf("backup paths collided: %s", m1.BackupPath)
	}
	if info, err := os.Stat(m1.BackupPath); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("first backup permissions: info=%v err=%v", info, err)
	}
}

func TestChatGPTTakeoverAndRollbackRestoresOriginalConfig(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)
	closeDB()
	defer closeDB()

	path := filepath.Join(tmp, ".codex", "config.toml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	original := "model_provider = \"openai\"\nmodel = \"gpt-5.5\"\n\n[model_providers.openai]\nname = \"OpenAI\"\nbase_url = \"https://api.openai.com/v1\"\n"
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	a := NewApp()
	if _, err := a.SetToolConfigPath("chatgpt", path); err != nil {
		t.Fatal(err)
	}
	a.proxy = newProxyServer()
	a.proxy.cfg.Port = 0
	if _, err := a.InjectGateway("chatgpt"); err != nil {
		t.Fatal(err)
	}
	managed, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(managed) == original || !strings.Contains(string(managed), "aigateway") {
		t.Fatalf("takeover did not update config: %s", managed)
	}

	st, err := a.RollbackGateway("chatgpt")
	if err != nil {
		t.Fatal(err)
	}
	if st.Kind != string(ToolCodex) {
		t.Fatalf("rollback kind=%q", st.Kind)
	}
	restored, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(restored) != original {
		t.Fatalf("restored config differs:\n got: %s\nwant: %s", restored, original)
	}
}

func TestRollbackUsesTakeoverBackupWhenCurrentPathChanged(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)
	closeDB()
	defer closeDB()

	originalPath := filepath.Join(tmp, ".codex", "config.toml")
	if err := os.MkdirAll(filepath.Dir(originalPath), 0o755); err != nil {
		t.Fatal(err)
	}
	original := "model_provider = \"openai\"\nmodel = \"gpt-5.5\"\n\n[model_providers.openai]\nname = \"OpenAI\"\nbase_url = \"https://api.openai.com/v1\"\n"
	if err := os.WriteFile(originalPath, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	otherPath := filepath.Join(tmp, "other", "config.toml")
	if err := os.MkdirAll(filepath.Dir(otherPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(otherPath, []byte("model = \"other\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	a := NewApp()
	if _, err := a.SetToolConfigPath("chatgpt", originalPath); err != nil {
		t.Fatal(err)
	}
	a.proxy = newProxyServer()
	a.proxy.cfg.Port = 0
	if _, err := a.InjectGateway("chatgpt"); err != nil {
		t.Fatal(err)
	}
	if _, err := a.SetToolConfigPath("chatgpt", otherPath); err != nil {
		t.Fatal(err)
	}
	st, err := a.RollbackGateway("chatgpt")
	if err != nil {
		t.Fatal(err)
	}
	if st.Path != originalPath {
		t.Fatalf("rollback path=%q, want %q", st.Path, originalPath)
	}
	restored, err := os.ReadFile(originalPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(restored) != original {
		t.Fatalf("restored config differs:\n got: %s\nwant: %s", restored, original)
	}
}

func TestRepeatedTakeoverDoesNotOverwriteOriginalTakeoverBackup(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)
	closeDB()
	defer closeDB()

	path := filepath.Join(tmp, ".codex", "config.toml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	original := "model_provider = \"openai\"\nmodel = \"gpt-5.5\"\n\n[model_providers.openai]\nname = \"OpenAI\"\nbase_url = \"https://api.openai.com/v1\"\n"
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	a := NewApp()
	if _, err := a.SetToolConfigPath("chatgpt", path); err != nil {
		t.Fatal(err)
	}
	a.proxy = newProxyServer()
	a.proxy.cfg.Port = 0
	if _, err := a.InjectGateway("chatgpt"); err != nil {
		t.Fatal(err)
	}
	firstBackup, err := os.ReadFile(takeoverBackupFile(ToolCodex, path))
	if err != nil {
		t.Fatal(err)
	}
	if string(firstBackup) != original {
		t.Fatalf("first takeover backup differs:\n%s", firstBackup)
	}
	if _, err := a.InjectGateway("chatgpt"); err == nil {
		t.Fatal("expected repeated takeover to be rejected")
	}
	secondBackup, err := os.ReadFile(takeoverBackupFile(ToolCodex, path))
	if err != nil {
		t.Fatal(err)
	}
	if string(secondBackup) != original {
		t.Fatalf("repeated takeover overwrote original backup:\n%s", secondBackup)
	}
	if _, err := a.RollbackGateway("chatgpt"); err != nil {
		t.Fatal(err)
	}
	restored, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(restored) != original {
		t.Fatalf("restored config differs:\n got: %s\nwant: %s", restored, original)
	}
}

func TestRollbackFallsBackToDefaultWhenTakeoverBackupWasOverwritten(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)
	closeDB()
	defer closeDB()

	path := filepath.Join(tmp, ".codex", "config.toml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	original := "model_provider = \"openai\"\nmodel = \"gpt-5.5\"\n\n[model_providers.openai]\nname = \"OpenAI\"\nbase_url = \"https://api.openai.com/v1\"\n"
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	a := NewApp()
	if _, err := a.SetToolConfigPath("chatgpt", path); err != nil {
		t.Fatal(err)
	}
	a.proxy = newProxyServer()
	a.proxy.cfg.Port = 0
	if _, err := a.InjectGateway("chatgpt"); err != nil {
		t.Fatal(err)
	}
	managed, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(takeoverBackupFile(ToolCodex, path), managed, 0o600); err != nil {
		t.Fatal(err)
	}
	meta, ok := loadTakeoverMeta(ToolCodex, path)
	if !ok {
		t.Fatal("missing takeover meta")
	}
	meta.SHA256 = hashBytes(managed)
	meta.Bytes = len(managed)
	if err := os.WriteFile(takeoverMetaFile(ToolCodex, path), mustJSON(t, meta), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := a.RollbackGateway("chatgpt"); err != nil {
		t.Fatal(err)
	}
	restored, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(restored) != original {
		t.Fatalf("fallback restore differs:\n got: %s\nwant: %s", restored, original)
	}
}

func TestMissingDefaultBackupRestoreDeletesCreatedConfig(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)
	closeDB()
	defer closeDB()

	cfgPath := filepath.Join(tmp, ".openclaw", "openclaw.json")
	created, err := ensureDefaultBackup(ToolOpenClaw, cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if !created {
		t.Fatal("expected missing baseline backup")
	}
	if err := os.MkdirAll(filepath.Dir(cfgPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfgPath, []byte(`{"models":{"providers":{}}}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	a := NewApp()
	if _, err := a.SetToolConfigPath("openclaw", cfgPath); err != nil {
		t.Fatal(err)
	}
	st, err := a.RestoreDefaultConfig("openclaw")
	if err != nil {
		t.Fatal(err)
	}
	if st.Exists {
		t.Fatalf("expected restored state to have no config file: %+v", st)
	}
	if _, err := os.Stat(cfgPath); !os.IsNotExist(err) {
		t.Fatalf("expected config removed, err=%v", err)
	}
}

func TestRestoreDefaultConfigRestoresAndCleansManagedEnvironment(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)
	t.Setenv("OLD_PROVIDER_KEY", "")
	t.Setenv("NEW_PROVIDER_KEY", "")
	closeDB()
	defer closeDB()

	path := filepath.Join(tmp, ".claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"model":"original"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(tmp, ".codex-manager", "env"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmp, ".codex-manager", "env", "secrets.json"), []byte(`{"OLD_PROVIDER_KEY":"old"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := ensureDefaultBackup(ToolClaude, path); err != nil {
		t.Fatal(err)
	}
	if err := setSystemEnvVar("NEW_PROVIDER_KEY", "new"); err != nil {
		t.Logf("environment setup warning: %v", err)
	}
	a := NewApp()
	if _, err := a.SetToolConfigPath("claude", path); err != nil {
		t.Fatal(err)
	}
	st, err := a.RestoreDefaultConfig("claude")
	if err != nil {
		t.Fatal(err)
	}
	if st.Model != "original" {
		t.Fatalf("model=%q", st.Model)
	}
	if os.Getenv("NEW_PROVIDER_KEY") != "" {
		t.Fatalf("new environment key was not cleared")
	}
	if os.Getenv("OLD_PROVIDER_KEY") != "old" {
		t.Fatalf("original environment key was not restored: %q", os.Getenv("OLD_PROVIDER_KEY"))
	}
	secrets, err := os.ReadFile(filepath.Join(tmp, ".codex-manager", "env", "secrets.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(secrets) != `{"OLD_PROVIDER_KEY":"old"}` {
		t.Fatalf("secrets=%s", secrets)
	}
}

func TestRollbackCleansTakeoverArtifactsForAllKinds(t *testing.T) {
	cases := []takeoverCase{
		{
			kind:         "chatgpt",
			path:         filepath.Join(".codex", "config.toml"),
			original:     "model_provider = \"openai\"\nmodel = \"gpt-5.5\"\n\n[model_providers.openai]\nname = \"OpenAI\"\nbase_url = \"https://api.openai.com/v1\"\n",
			managedProbe: "model_provider = \"aigateway\"",
		},
		{
			kind:         "claude",
			path:         filepath.Join(".claude", "settings.json"),
			original:     `{"model":"original","env":{"OLD_KEY":"old"}}` + "\n",
			managedProbe: `"OPENAI_BASE_URL": "http://127.0.0.1:`,
		},
		{
			kind:         "openclaw",
			path:         filepath.Join(".openclaw", "openclaw.json"),
			original:     `{"models":{"providers":{"openai":{"baseUrl":"https://api.openai.com/v1","apiKey":"old","api":"openai-completions","models":[{"id":"m","name":"m"}]}}},"agents":{"defaults":{"model":{"primary":"openai/m","fallbacks":[]}}},"env":{"vars":{"OPENAI_BASE_URL":"https://api.openai.com/v1","OPENAI_API_KEY":"old"}}}` + "\n",
			managedProbe: `aigateway`,
		},
		{
			kind:         "harness",
			path:         filepath.Join(".harness", "config.yaml"),
			original:     "model: original\nprovider: openai\nbase_url: https://api.openai.com/v1\napi_key: old\n",
			managedProbe: "base_url: \"http://127.0.0.1:",
		},
		{
			kind:         "grok",
			path:         filepath.Join(".grok", "config.toml"),
			original:     "[model.\"old\"]\nmodel = \"old\"\nbase_url = \"https://api.x.ai/v1\"\nname = \"xAI\"\n\n[models]\ndefault = \"old\"\n",
			managedProbe: "aiSwitchModel-grok",
		},
	}

	for _, tc := range cases {
		t.Run(tc.kind, func(t *testing.T) {
			tmp := t.TempDir()
			t.Setenv("HOME", tmp)
			t.Setenv("USERPROFILE", tmp)
			closeDB()
			defer closeDB()

			path := filepath.Join(tmp, tc.path)
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(tc.original), 0o644); err != nil {
				t.Fatal(err)
			}

			a := NewApp()
			if _, err := a.SetToolConfigPath(tc.kind, path); err != nil {
				t.Fatal(err)
			}
			a.proxy = newProxyServer()
			a.proxy.cfg.Port = 0

			st, err := a.InjectGateway(tc.kind)
			if err != nil {
				t.Fatal(err)
			}
			if !st.Managed {
				t.Fatalf("expected managed after takeover: %+v", st)
			}
			managed, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(managed), tc.managedProbe) {
				t.Fatalf("takeover did not rewrite config: %s", managed)
			}
			if _, ok := loadTakeoverMeta(normalizeToolKind(tc.kind), path); !ok {
				t.Fatalf("missing takeover meta after takeover for %s", tc.kind)
			}

			st, err = a.RollbackGateway(tc.kind)
			if err != nil {
				t.Fatal(err)
			}
			if st.Managed {
				t.Fatalf("expected un-managed after rollback: %+v", st)
			}
			restored, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if string(restored) != tc.original {
				t.Fatalf("restored config differs for %s:\n got: %s\nwant: %s", tc.kind, restored, tc.original)
			}
			if _, ok := loadTakeoverMeta(normalizeToolKind(tc.kind), path); ok {
				t.Fatalf("takeover meta should be deleted after rollback for %s", tc.kind)
			}
			if _, err := os.Stat(takeoverBackupFile(normalizeToolKind(tc.kind), path)); !os.IsNotExist(err) {
				t.Fatalf("takeover backup should be deleted after rollback for %s, err=%v", tc.kind, err)
			}
			if _, err := os.Stat(takeoverEnvironmentBackupPath(normalizeToolKind(tc.kind), path)); !os.IsNotExist(err) {
				t.Fatalf("takeover env backup should be deleted after rollback for %s, err=%v", tc.kind, err)
			}
		})
	}
}

func TestInjectGatewayRefusesToOverwriteManagedConfigWithoutMeta(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)
	closeDB()
	defer closeDB()

	path := filepath.Join(tmp, ".codex", "config.toml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	original := "model_provider = \"openai\"\nmodel = \"gpt-5.5\"\n\n[model_providers.openai]\nname = \"OpenAI\"\nbase_url = \"https://api.openai.com/v1\"\n"
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	a := NewApp()
	if _, err := a.SetToolConfigPath("chatgpt", path); err != nil {
		t.Fatal(err)
	}
	a.proxy = newProxyServer()
	a.proxy.cfg.Port = 0
	if _, err := a.InjectGateway("chatgpt"); err != nil {
		t.Fatal(err)
	}

	// Simulate state loss: managed config remains, but takeover metadata disappears.
	_ = os.Remove(takeoverMetaFile(ToolCodex, path))
	_ = os.Remove(takeoverBackupFile(ToolCodex, path))
	_ = os.Remove(takeoverEnvironmentBackupPath(ToolCodex, path))

	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.InjectGateway("chatgpt"); err == nil {
		t.Fatal("expected takeover to be rejected when managed state exists without metadata")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatalf("managed config should remain unchanged when takeover is refused:\n got: %s\nwant: %s", after, before)
	}
}
