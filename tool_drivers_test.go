package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInjectOpenClawGateway(t *testing.T) {
	tmp := t.TempDir()
	p := filepath.Join(tmp, "openclaw.json")
	if err := injectOpenClawGateway(p, "http://127.0.0.1:18080/v1", "aigateway", []string{"gpt-test"}); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	var root map[string]any
	if err := json.Unmarshal(b, &root); err != nil {
		t.Fatal(err)
	}
	models := root["models"].(map[string]any)
	providers := models["providers"].(map[string]any)
	ag := providers["aigateway"].(map[string]any)
	if ag["baseUrl"] != "http://127.0.0.1:18080/v1" {
		t.Fatalf("baseUrl=%v", ag["baseUrl"])
	}
	if ag["api"] != "openai-completions" {
		t.Fatalf("api=%v", ag["api"])
	}
	// must NOT use legacy root OPENAI_BASE_URL
	if _, ok := root["baseUrl"]; ok {
		t.Fatal("legacy root baseUrl should not be set")
	}
	agents := root["agents"].(map[string]any)
	defaults := agents["defaults"].(map[string]any)
	model := defaults["model"].(map[string]any)
	// Takeover pins virtual hot-switch model (not a concrete upstream id)
	if model["primary"] != "aigateway/"+gatewayVirtualModel {
		t.Fatalf("primary=%v want aigateway/%s", model["primary"], gatewayVirtualModel)
	}
}

func TestApplyOpenClawModelSwitch(t *testing.T) {
	out, err := applyOpenClawModelSwitch(`{}`, "deepseek-v4-pro", "http://127.0.0.1:18080/v1", "aigateway")
	if err != nil {
		t.Fatal(err)
	}
	var root map[string]any
	if err := json.Unmarshal([]byte(out), &root); err != nil {
		t.Fatal(err)
	}
	primary := root["agents"].(map[string]any)["defaults"].(map[string]any)["model"].(map[string]any)["primary"]
	if primary != "aigateway/deepseek-v4-pro" {
		t.Fatalf("primary=%v", primary)
	}
}

func TestInjectGatewayClaudeOfficial(t *testing.T) {
	out, err := injectGatewayClaude(`{"permissions":{}}`, "http://127.0.0.1:18080/v1")
	if err != nil {
		t.Fatal(err)
	}
	var root map[string]any
	if err := json.Unmarshal([]byte(out), &root); err != nil {
		t.Fatal(err)
	}
	env := root["env"].(map[string]any)
	if env["ANTHROPIC_BASE_URL"] != "http://127.0.0.1:18080/v1" {
		t.Fatalf("base=%v", env["ANTHROPIC_BASE_URL"])
	}
	if _, ok := env["OPENAI_BASE_URL"]; ok {
		t.Fatal("OPENAI_BASE_URL must not be set for Claude Code")
	}
	if _, ok := root["apiBaseUrl"]; ok {
		t.Fatal("apiBaseUrl must not be set for Claude Code")
	}
	if env["ANTHROPIC_AUTH_TOKEN"] != "aigateway" {
		t.Fatalf("token=%v", env["ANTHROPIC_AUTH_TOKEN"])
	}
}

func TestInjectHarnessYAML(t *testing.T) {
	tmp := t.TempDir()
	p := filepath.Join(tmp, "config.yaml")
	if err := injectHarnessYAML(p, "http://127.0.0.1:18080/v1", "aigateway", "m1"); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	// Inject always pins virtual model for proxy hot-switch
	if !strings.Contains(s, "model: "+gatewayVirtualModel) {
		t.Fatalf("missing virtual model: %s", s)
	}
	if !strings.Contains(s, "127.0.0.1:18080") {
		t.Fatalf("missing gateway: %s", s)
	}
	if !strings.Contains(s, "provider:") {
		t.Fatalf("missing provider block: %s", s)
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
	// OpenClaw preferred path is openclaw.json
	d := driverByID("openclaw")
	if !strings.HasSuffix(d.PreferredPath(), "openclaw.json") {
		t.Fatalf("preferred=%s", d.PreferredPath())
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
