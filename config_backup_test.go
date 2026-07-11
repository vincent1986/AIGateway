package main

import (
	"os"
	"path/filepath"
	"testing"
)

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
