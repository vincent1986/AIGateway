package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSetTomlTopLevel(t *testing.T) {
	in := "model = \"a\"\nmodel_provider = \"p\"\n\n[model_providers.x]\nname = \"x\"\n"
	out := setTomlTopLevelString(in, "model", "b")
	if got := readTomlTopLevelString(out, "model"); got != "b" {
		t.Fatalf("model=%q", got)
	}
	if got := readTomlTopLevelString(out, "model_provider"); got != "p" {
		t.Fatalf("provider=%q", got)
	}
	cands := parseCodexModels("[[models]]\nname = \"N\"\nprovider = \"p1\"\nmodel = \"m1\"\n")
	if len(cands) != 1 || cands[0].ID != "m1" || cands[0].Provider != "p1" {
		t.Fatalf("%+v", cands)
	}
}

func TestValidateToolConfigWriteUsesToolSchema(t *testing.T) {
	tmp := t.TempDir()
	claudePath := filepath.Join(tmp, "settings.json")
	if err := os.WriteFile(claudePath, []byte(`{"model":"claude-test","env":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := validateToolConfigWrite(ToolClaude, claudePath, "claude-test"); err != nil {
		t.Fatal(err)
	}
	if err := validateToolConfigWrite(ToolClaude, claudePath, "wrong-model"); err == nil {
		t.Fatal("expected schema validation failure")
	}
}

func TestHarnessYAMLOnlyUpdatesTopLevelSchema(t *testing.T) {
	in := "providers:\n  model: nested\nmodel: old\nprovider: old-provider\n"
	out, err := applyHarnessYAML(in, "http://gateway/v1", "key", "new-model", "new-provider")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "  model: new-model") || !strings.Contains(out, "model: \"new-model\"") {
		t.Fatalf("nested or top-level model update is wrong:\n%s", out)
	}
	model, provider := readGenericToolModel(out, "config.yaml")
	if model != "new-model" || provider != "new-provider" {
		t.Fatalf("model=%q provider=%q output=%s", model, provider, out)
	}
}

func TestHarnessYAMLRejectsUnknownSchema(t *testing.T) {
	if _, err := applyHarnessYAML("providers:\n  endpoint: https://example.com\n", "http://gateway/v1", "", "m", "p"); err == nil {
		t.Fatal("expected unknown Harness schema to be rejected")
	}
}

func TestToolManagedStatusIsPerApp(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)

	codexDir := filepath.Join(tmp, ".codex")
	openclawDir := filepath.Join(tmp, ".openclaw")
	if err := os.MkdirAll(codexDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(openclawDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(codexDir, "config.toml"), []byte(`model = "m"
model_provider = "aigateway"
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(openclawDir, "openclaw.json"), []byte(`{"models":{"providers":{"direct":{"base_url":"https://example.com/v1"}}}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	a := NewApp()
	codex := a.GetToolConfig("codex")
	if !codex.Managed {
		t.Fatalf("codex should be managed: %+v", codex)
	}
	openclaw := a.GetToolConfig("openclaw")
	if openclaw.Managed {
		t.Fatalf("openclaw should not inherit codex managed state: %+v", openclaw)
	}
}
