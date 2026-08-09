package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestEnsureOpenClawGatewayModeLocalCreatesAndSets(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "openclaw.json")

	changed, err := ensureOpenClawGatewayModeLocal(path)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("expected first write to change config")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"mode": "local"`) {
		t.Fatalf("missing gateway.mode=local: %s", b)
	}

	changed, err = ensureOpenClawGatewayModeLocal(path)
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Fatal("second call should be a no-op")
	}
}

func TestEnsureOpenClawGatewayModeLocalPreservesProviders(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "openclaw.json")
	original := `{
  "models": {
    "providers": {
      "openai": { "baseUrl": "https://api.openai.com/v1" }
    }
  },
  "gateway": { "port": 19001 }
}
`
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	changed, err := ensureOpenClawGatewayModeLocal(path)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("expected mode write when missing")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if !strings.Contains(s, `"mode": "local"`) {
		t.Fatalf("mode not local: %s", s)
	}
	if !strings.Contains(s, `"openai"`) || !strings.Contains(s, "19001") {
		t.Fatalf("lost existing fields: %s", s)
	}
}

func TestEnsureOpenClawGatewayModeLocalRejectsRemote(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "openclaw.json")
	original := `{"gateway":{"mode":"remote","port":19001}}`
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ensureOpenClawGatewayModeLocal(path); err == nil || !strings.Contains(err.Error(), "remote") {
		t.Fatalf("expected remote rejection, err=%v", err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"mode":"remote"`) && !strings.Contains(string(b), `"mode": "remote"`) {
		t.Fatalf("remote mode must remain unchanged: %s", b)
	}
}

func TestReadOpenClawGatewayPort(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "openclaw.json")
	if err := os.WriteFile(path, []byte(`{"gateway":{"port":19111}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := readOpenClawGatewayPort(path); got != 19111 {
		t.Fatalf("port=%d", got)
	}
	if got := readOpenClawGatewayPort(filepath.Join(tmp, "missing.json")); got != defaultOpenClawGatewayPort {
		t.Fatalf("default port=%d", got)
	}
}

func TestResolveOpenClawBinaryUsesLookPath(t *testing.T) {
	old := lookPathFn
	t.Cleanup(func() { lookPathFn = old })
	lookPathFn = func(file string) (string, error) {
		if file == "openclaw" {
			return "/fake/bin/openclaw", nil
		}
		return "", os.ErrNotExist
	}
	got, err := resolveOpenClawBinary()
	if err != nil {
		t.Fatal(err)
	}
	if got != "/fake/bin/openclaw" {
		t.Fatalf("got=%q", got)
	}
}

func TestResolveOpenClawBinaryMissing(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)
	old := lookPathFn
	t.Cleanup(func() { lookPathFn = old })
	lookPathFn = func(string) (string, error) { return "", os.ErrNotExist }
	_, err := resolveOpenClawBinary()
	if err == nil || !strings.Contains(err.Error(), "npm install -g openclaw") {
		t.Fatalf("err=%v", err)
	}
}

func TestLaunchOpenClawAlreadyRunning(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)
	closeDB()
	defer closeDB()

	path := filepath.Join(tmp, ".openclaw", "openclaw.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"gateway":{"mode":"local","port":18789},"models":{"providers":{"aigateway":{"baseUrl":"http://127.0.0.1:18080/v1"}}}}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	oldLook := lookPathFn
	oldDial := dialOpenClawFn
	oldRun := runOpenClawCommandFn
	oldStart := startDetachedFn
	t.Cleanup(func() {
		lookPathFn = oldLook
		dialOpenClawFn = oldDial
		runOpenClawCommandFn = oldRun
		startDetachedFn = oldStart
	})
	lookPathFn = func(string) (string, error) { return "/bin/openclaw", nil }
	dialOpenClawFn = func(int) bool { return true }
	runOpenClawCommandFn = func(string, time.Duration, ...string) (string, error) {
		t.Fatal("should not run CLI when already reachable")
		return "", nil
	}
	startDetachedFn = func(string, ...string) error {
		t.Fatal("should not start detached when already reachable")
		return nil
	}

	a := NewApp()
	a.proxy = newProxyServer()
	a.proxy.cfg.Port = 0
	if _, err := a.SetToolConfigPath("openclaw", path); err != nil {
		t.Fatal(err)
	}
	res, err := a.LaunchOpenClaw()
	if err != nil {
		t.Fatal(err)
	}
	if !res.AlreadyRunning || res.Started {
		t.Fatalf("result=%+v", res)
	}
	if res.DashboardURL != "http://127.0.0.1:18789" {
		t.Fatalf("url=%q", res.DashboardURL)
	}
}

func TestLaunchOpenClawStartsDetachedWhenServiceFails(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)
	closeDB()
	defer closeDB()

	path := filepath.Join(tmp, ".openclaw", "openclaw.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"models":{}}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	oldLook := lookPathFn
	oldDial := dialOpenClawFn
	oldRun := runOpenClawCommandFn
	oldStart := startDetachedFn
	t.Cleanup(func() {
		lookPathFn = oldLook
		dialOpenClawFn = oldDial
		runOpenClawCommandFn = oldRun
		startDetachedFn = oldStart
	})

	lookPathFn = func(string) (string, error) { return "/bin/openclaw", nil }
	calls := 0
	dialOpenClawFn = func(int) bool {
		calls++
		// first checks before start fail; after detached start succeed
		return calls >= 3
	}
	runOpenClawCommandFn = func(bin string, _ time.Duration, args ...string) (string, error) {
		if len(args) >= 2 && args[0] == "gateway" && args[1] == "start" {
			return "service not installed", os.ErrNotExist
		}
		return "", nil
	}
	started := false
	startDetachedFn = func(bin string, args ...string) error {
		started = true
		if bin != "/bin/openclaw" || len(args) < 2 || args[0] != "gateway" || args[1] != "run" {
			t.Fatalf("unexpected start: %s %v", bin, args)
		}
		return nil
	}

	a := NewApp()
	a.proxy = newProxyServer()
	a.proxy.cfg.Port = 0
	if _, err := a.SetToolConfigPath("openclaw", path); err != nil {
		t.Fatal(err)
	}
	res, err := a.LaunchOpenClaw()
	if err != nil {
		t.Fatal(err)
	}
	if !started || !res.Started || res.AlreadyRunning {
		t.Fatalf("started=%v result=%+v", started, res)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"mode": "local"`) {
		t.Fatalf("expected gateway.mode=local written: %s", b)
	}
}

func TestOpenClawStatusLooksRunning(t *testing.T) {
	if !openClawStatusLooksRunning(`{"service":{"running":true}}`) {
		t.Fatal("json running")
	}
	if openClawStatusLooksRunning(`Gateway is not running`) {
		t.Fatal("not running")
	}
}
