package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestExpandPathHome(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		t.Skip("no home")
	}
	got := expandPath("~/foo/bar")
	want := filepath.Join(home, "foo", "bar")
	if got != want {
		t.Fatalf("expandPath ~/ = %q want %q", got, want)
	}
	got = expandPath(`~\foo`)
	want = filepath.Join(home, "foo")
	if got != want {
		t.Fatalf("expandPath ~\\ = %q want %q", got, want)
	}
}

func TestExpandWindowsEnv(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		t.Skip("no home")
	}
	t.Setenv("USERPROFILE", home)
	got := expandPath(`%USERPROFILE%\.codex\config.toml`)
	want := filepath.Join(home, ".codex", "config.toml")
	if got != want {
		t.Fatalf("expand %%USERPROFILE%% = %q want %q", got, want)
	}
}

func TestCodexSearchPathsContainDefault(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		t.Skip("no home")
	}
	paths := codexSearchPaths()
	if len(paths) == 0 {
		t.Fatal("empty search paths")
	}
	def := filepath.Join(home, ".codex", "config.toml")
	found := false
	for _, p := range paths {
		if filepath.Clean(p) == filepath.Clean(def) {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("default %q not in %v", def, paths)
	}
	// Windows should not search Unix /etc
	if runtime.GOOS == "windows" {
		for _, p := range paths {
			if strings.HasPrefix(p, "/etc/") {
				t.Fatalf("windows path list includes unix path: %s", p)
			}
		}
	}
}

func TestClaudeSearchPathsContainDefault(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		t.Skip("no home")
	}
	paths := claudeSearchPaths()
	def := filepath.Join(home, ".claude", "settings.json")
	found := false
	for _, p := range paths {
		if filepath.Clean(p) == filepath.Clean(def) {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("default %q not in %v", def, paths)
	}
}

func TestPreserveLineEndingsCRLF(t *testing.T) {
	orig := "model = \"a\"\r\n"
	next := "model = \"b\"\n"
	got := preserveLineEndings(orig, next)
	if !strings.Contains(got, "\r\n") {
		t.Fatalf("expected CRLF, got %q", got)
	}
}

func TestGetSystemInfo(t *testing.T) {
	a := NewApp()
	info := a.GetSystemInfo()
	if info.OS != runtime.GOOS {
		t.Fatalf("os %q", info.OS)
	}
	if info.RevealLabel == "" {
		t.Fatal("empty reveal label")
	}
	switch runtime.GOOS {
	case "darwin":
		if info.PlatformName != "macOS" || !strings.Contains(info.RevealLabel, "访达") {
			t.Fatalf("%+v", info)
		}
	case "windows":
		if info.PlatformName != "Windows" || !strings.Contains(info.RevealLabel, "资源管理器") {
			t.Fatalf("%+v", info)
		}
	}
}

func TestPreferredPaths(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		t.Skip("no home")
	}
	os.Unsetenv("CODEX_HOME")
	os.Unsetenv("CLAUDE_CONFIG_DIR")
	cp := preferredCodexConfigPath()
	wantCodex := filepath.Join(home, ".codex", "config.toml")
	if filepath.Clean(cp) != filepath.Clean(wantCodex) {
		t.Fatalf("codex preferred %q want %q", cp, wantCodex)
	}
	lp := preferredClaudeConfigPath()
	wantClaude := filepath.Join(home, ".claude", "settings.json")
	if filepath.Clean(lp) != filepath.Clean(wantClaude) {
		t.Fatalf("claude preferred %q want %q", lp, wantClaude)
	}
}
