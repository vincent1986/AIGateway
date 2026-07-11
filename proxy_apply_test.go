package main

import (
	"strings"
	"testing"
)

func TestRemoveCodexProxyAndRewriteBases(t *testing.T) {
	in := `model = "gpt-4o"
model_provider = "codex_proxy"

[model_providers.deepseek]
name = "deepseek"
base_url = "https://api.deepseek.com/v1"
env_key = "deepseek_api_key"
api_key = "sk-ds"

[model_providers.codex_proxy]
name = "OpenAI Proxy"
base_url = "http://127.0.0.1:18080/v1"
env_key = "codex_proxy_api_key"
api_key = "proxy"

[[models]]
name = "OpenAI Proxy"
provider = "codex_proxy"
model = "gpt-4o"
`
	local := "http://127.0.0.1:18080/v1"
	_ = rememberOriginalBases(in, local)
	out := removeTomlProviderBlock(in, "codex_proxy")
	out = setAllProvidersBaseURL(out, local)
	out = setTomlTopLevelString(out, "model_provider", "deepseek")
	out = removeModelsWithProvider(out, "codex_proxy")
	if strings.Contains(out, "codex_proxy") {
		t.Fatalf("codex_proxy remains:\n%s", out)
	}
	if !strings.Contains(out, `base_url = "http://127.0.0.1:18080/v1"`) {
		t.Fatalf("local base missing:\n%s", out)
	}
	if got := readTomlTopLevelString(out, "model_provider"); got != "deepseek" {
		t.Fatalf("provider=%q", got)
	}
}
