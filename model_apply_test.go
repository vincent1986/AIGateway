package main

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestProviderEnvVarName(t *testing.T) {
	if got := providerEnvVarName("deepseek", "DeepSeek"); got != "deepseek_api_key" {
		t.Fatalf("got %q", got)
	}
	if got := providerEnvVarName("openai", "OpenAI"); got != "openai_api_key" {
		t.Fatalf("got %q", got)
	}
}

func TestApplyCodexModelSwitchDeepseek(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)

	in := `model = "old"
model_provider = "custom-ollama"

[model_providers.custom-ollama]
name = "Ollama"
base_url = "http://localhost:11434/v1"

[[models]]
name = "old"
provider = "custom-ollama"
model = "old"
`
	out := applyCodexModelSwitch(in, "deepseek-v4-pro", "deepseek", "deepseek-v4-pro",
		"https://api.deepseek.com", "sk-test-key")

	if got := readTomlTopLevelString(out, "model"); got != "deepseek-v4-pro" {
		t.Fatalf("model=%q", got)
	}
	if got := readTomlTopLevelString(out, "model_provider"); got != "deepseek" {
		t.Fatalf("provider=%q", got)
	}
	if !strings.Contains(out, `[model_providers.deepseek]`) {
		t.Fatalf("missing provider block:\n%s", out)
	}
	if !strings.Contains(out, `env_key = "deepseek_api_key"`) {
		t.Fatalf("missing env_key=deepseek_api_key:\n%s", out)
	}
	if !strings.Contains(out, `api_key = "sk-test-key"`) {
		t.Fatalf("missing api_key:\n%s", out)
	}
	// wire_api is deprecated — must not be written
	if strings.Contains(out, `wire_api`) {
		t.Fatalf("wire_api should not be written (deprecated):\n%s", out)
	}
	if !strings.Contains(out, `[model_providers.custom-ollama]`) {
		t.Fatalf("lost ollama block:\n%s", out)
	}
}

func TestUpsertCodexModelsOnlyOne(t *testing.T) {
	in := `model = "m1"
model_provider = "p1"

[model_providers.p1]
name = "P1"
base_url = "http://x"

[[models]]
name = "first"
provider = "p1"
model = "m1"

[[models]]
name = "dup"
provider = "p1"
model = "m1"

[[models]]
name = "other"
provider = "p2"
model = "m2"
`
	// switch to m3 → only m3 remains
	out := applyCodexModelSwitch(in, "m3", "p1", "M3", "http://x", "sk")
	entries := parseCodexModels(out)
	if len(entries) != 1 {
		t.Fatalf("want exactly 1 [[models]], got %d\n%s", len(entries), out)
	}
	if entries[0].ID != "m3" || entries[0].Provider != "p1" {
		t.Fatalf("%+v\n%s", entries[0], out)
	}
	if strings.Contains(out, `model = "m1"`) || strings.Contains(out, `model = "m2"`) {
		// top-level model = "m3" is fine; old [[models]] model= must be gone
		// check only under models by count
		if strings.Count(out, "[[models]]") != 1 {
			t.Fatalf("stale models remain:\n%s", out)
		}
	}
	// switch again to m1 → still only one
	out = applyCodexModelSwitch(out, "m1", "p1", "M1 Display", "http://x", "sk")
	entries = parseCodexModels(out)
	if len(entries) != 1 || entries[0].ID != "m1" || entries[0].Name != "M1 Display" {
		t.Fatalf("%+v\n%s", entries, out)
	}
	if c := strings.Count(out, "[[models]]"); c != 1 {
		t.Fatalf("[[models]] count=%d\n%s", c, out)
	}
}

func TestMisusedEnvKeyMigrated(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)

	in := `model = "x"
model_provider = "deepseek"

[model_providers.deepseek]
name = "deepseek"
base_url = "https://old.example.com"
env_key = "sk-old-secret-key-xxxxxxxxxxxxxxxx"
`
	out := applyCodexModelSwitch(in, "deepseek-v4-pro", "deepseek", "deepseek-v4-pro",
		"https://api.deepseek.com", "sk-new")
	if !strings.Contains(out, `env_key = "deepseek_api_key"`) {
		t.Fatalf("expected deepseek_api_key:\n%s", out)
	}
	if strings.Contains(out, `env_key = "sk-old`) {
		t.Fatalf("stale secret env_key kept:\n%s", out)
	}
	if !strings.Contains(out, `api_key = "sk-new"`) {
		t.Fatalf("api_key missing:\n%s", out)
	}
}

func TestApplyClaudeModelSwitch(t *testing.T) {
	in := `{"permissions":{"allow":["Bash"]}}`
	// non-DeepSeek third party keeps given base URL
	out, err := applyClaudeModelSwitch(in, "gpt-test", "https://gateway.example.com/v1", "sk-abc", "custom")
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := json.Unmarshal([]byte(out), &raw); err != nil {
		t.Fatal(err)
	}
	if raw["model"] != "gpt-test" {
		t.Fatalf("model=%v", raw["model"])
	}
	env := raw["env"].(map[string]any)
	if env["ANTHROPIC_BASE_URL"] != "https://gateway.example.com/v1" {
		t.Fatalf("base=%v", env["ANTHROPIC_BASE_URL"])
	}
	if env["ANTHROPIC_AUTH_TOKEN"] != "sk-abc" {
		t.Fatalf("token=%v", env["ANTHROPIC_AUTH_TOKEN"])
	}
}

func TestApplyClaudeDeepSeekOfficial(t *testing.T) {
	in := `{"permissions":{"allow":["Bash"]}}`
	out, err := applyClaudeModelSwitch(in, "deepseek-v4-pro", "https://api.deepseek.com/v1", "sk-ds", "deepseek")
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := json.Unmarshal([]byte(out), &raw); err != nil {
		t.Fatal(err)
	}
	if raw["model"] != deepSeekDefaultPro {
		t.Fatalf("model=%v want %s", raw["model"], deepSeekDefaultPro)
	}
	env := raw["env"].(map[string]any)
	if env["ANTHROPIC_BASE_URL"] != deepSeekAnthropicBase {
		t.Fatalf("base=%v want %s", env["ANTHROPIC_BASE_URL"], deepSeekAnthropicBase)
	}
	if env["ANTHROPIC_AUTH_TOKEN"] != "sk-ds" {
		t.Fatalf("token=%v", env["ANTHROPIC_AUTH_TOKEN"])
	}
	if env["ANTHROPIC_MODEL"] != deepSeekDefaultPro {
		t.Fatalf("ANTHROPIC_MODEL=%v", env["ANTHROPIC_MODEL"])
	}
	if env["ANTHROPIC_DEFAULT_OPUS_MODEL"] != deepSeekDefaultPro {
		t.Fatalf("opus=%v", env["ANTHROPIC_DEFAULT_OPUS_MODEL"])
	}
	if env["ANTHROPIC_DEFAULT_HAIKU_MODEL"] != deepSeekDefaultFlash {
		t.Fatalf("haiku=%v", env["ANTHROPIC_DEFAULT_HAIKU_MODEL"])
	}
	if env["CLAUDE_CODE_SUBAGENT_MODEL"] != deepSeekDefaultFlash {
		t.Fatalf("subagent=%v", env["CLAUDE_CODE_SUBAGENT_MODEL"])
	}
	if env["CLAUDE_CODE_EFFORT_LEVEL"] != "max" {
		t.Fatalf("effort=%v", env["CLAUDE_CODE_EFFORT_LEVEL"])
	}
}

func TestApplyClaudeDeepSeekSettingsStandalone(t *testing.T) {
	out, err := applyClaudeDeepSeekSettings("{}", "sk-x", deepSeekDefaultPro, deepSeekDefaultFlash, "max")
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := json.Unmarshal([]byte(out), &raw); err != nil {
		t.Fatal(err)
	}
	env := raw["env"].(map[string]any)
	if env["ANTHROPIC_BASE_URL"] != "https://api.deepseek.com/anthropic" {
		t.Fatalf("base=%v", env["ANTHROPIC_BASE_URL"])
	}
}

func TestDeriveProviderID(t *testing.T) {
	if got := deriveProviderID("https://api.deepseek.com", "x"); got != "deepseek" {
		t.Fatalf("%q", got)
	}
}

func TestLooksLikeEnvVarName(t *testing.T) {
	if !looksLikeEnvVarName("deepseek_api_key") {
		t.Fatal("expected env name")
	}
	if !looksLikeEnvVarName("DEEPSEEK_API_KEY") {
		t.Fatal("expected upper env name")
	}
	if looksLikeEnvVarName("sk-7ee4aacce3c349f98be10db962642d70") {
		t.Fatal("sk- should not be env name")
	}
}

func TestSetSystemEnvVarProcess(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)

	name := "deepseek_api_key"
	val := "sk-unit-test-value"
	if err := setSystemEnvVar(name, val); err != nil {
		// launchctl/setx may fail in CI; process env must still work
		t.Logf("setSystemEnvVar warning: %v", err)
	}
	if os.Getenv(name) != val {
		t.Fatalf("process env not set: %q", os.Getenv(name))
	}
	// secrets file
	sec := filepathJoin(tmp, ".codex-manager", "env", "secrets.json")
	b, err := os.ReadFile(sec)
	if err != nil {
		// managerRoot uses UserHomeDir which is tmp via HOME
		t.Fatalf("secrets: %v", err)
	}
	var m map[string]string
	if json.Unmarshal(b, &m) != nil || m[name] != val {
		t.Fatalf("secrets content: %s", b)
	}
}

func filepathJoin(elem ...string) string {
	return strings.Join(elem, string(os.PathSeparator))
}
