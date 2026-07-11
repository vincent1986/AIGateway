package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInjectJSONBaseURL(t *testing.T) {
	tmp := t.TempDir()
	p := filepath.Join(tmp, "config.json")
	if err := injectJSONBaseURL(p, "http://127.0.0.1:18080/v1", "sk-x"); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if !strings.Contains(s, "127.0.0.1:18080") {
		t.Fatalf("missing gateway: %s", s)
	}
}

func TestDriverRegistry(t *testing.T) {
	for _, id := range []string{"chatgpt", "claude", "openclaw", "harness", "codex"} {
		if driverByID(id) == nil {
			t.Fatalf("missing driver %s", id)
		}
	}
	if driverByID("chatgpt").ToolName() != "ChatGPT" {
		t.Fatal("name")
	}
}

func TestInjectGatewayCodex(t *testing.T) {
	out, err := injectGatewayCodex("", "http://127.0.0.1:18080/v1")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "aigateway") {
		t.Fatal(out)
	}
}
