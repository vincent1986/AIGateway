package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
)

// ModelApplyRequest is the full payload for switching models on Codex / Claude Code.
// Frontend can pass provider context so both tools get base URL + key, not only model id.
type ModelApplyRequest struct {
	Kind     string `json:"kind"`     // codex | claude
	Path     string `json:"path"`     // config file path (optional if auto-discovered)
	Model    string `json:"model"`    // model id
	Provider string `json:"provider"` // codex model_provider id, e.g. deepseek
	BaseURL  string `json:"baseUrl"`  // API base url
	APIKey   string `json:"apiKey"`   // api key
	Name     string `json:"name"`     // display name for [[models]]
}

// ApplyToolModel switches model with full provider context (recommended).
func (a *App) ApplyToolModel(req ModelApplyRequest) (ToolConfigStatus, error) {
	k := ToolKind(strings.ToLower(strings.TrimSpace(req.Kind)))
	model := strings.TrimSpace(req.Model)
	if model == "" {
		return ToolConfigStatus{}, fmt.Errorf("模型不能为空")
	}
	if k != ToolCodex && k != ToolClaude {
		return ToolConfigStatus{}, fmt.Errorf("未知工具类型: %s", req.Kind)
	}

	path := expandPath(strings.TrimSpace(req.Path))
	if path == "" {
		st := a.resolveTool(k)
		path = st.Path
	}
	if path == "" {
		return ToolConfigStatus{}, fmt.Errorf("未指定配置文件路径")
	}

	var content string
	var raw []byte
	if fileExists(path) {
		b, err := os.ReadFile(path)
		if err != nil {
			return ToolConfigStatus{}, err
		}
		raw = b
		content = string(b)
	}

	autoBakMsg := ""
	if len(raw) > 0 {
		created, err := ensureDefaultBackup(k, path)
		if err != nil {
			return ToolConfigStatus{}, fmt.Errorf("备份默认配置失败: %w", err)
		}
		if created {
			autoBakMsg = "（已自动备份默认配置）"
		}
		savePreWriteSnapshot(k, path, raw)
	}

	providerID := strings.TrimSpace(req.Provider)
	baseURL := strings.TrimRight(strings.TrimSpace(req.BaseURL), "/")
	apiKey := strings.TrimSpace(req.APIKey)
	displayName := strings.TrimSpace(req.Name)
	if displayName == "" {
		displayName = model
	}

	var next string
	var err error
	switch k {
	case ToolCodex:
		// If provider not given, try match from [[models]]
		if providerID == "" {
			providerID = matchCodexProvider(content, model)
		}
		// Derive provider id from name/url when still empty
		if providerID == "" && baseURL != "" {
			providerID = deriveProviderID(baseURL, displayName)
		}
		if providerID == "" {
			providerID = "custom"
		}
		// Resolve key before switch if missing
		if apiKey == "" {
			apiKey = loadProviderSecret(providerID)
		}
		envVar := providerEnvVarName(providerID, displayName)
		// Auto proxy: if vendor wants proxy, ensure proxy is running then use local base.
		writeBase := baseURL
		if providerIDWantsProxy(providerID, displayName, baseURL) {
			if a.proxy == nil {
				a.proxy = newProxyServer()
			}
			if !a.proxy.status().Running {
				if err := a.proxy.start(); err != nil {
					autoBakMsg += "；自动启动代理失败: " + err.Error()
				}
			}
			if a.proxy.status().Running {
				writeBase = a.proxy.baseURL()
				if baseURL != "" && !isLocalProxyURL(baseURL) {
					_ = rememberOriginalBases(
						"[model_providers."+providerID+"]\nbase_url = \""+baseURL+"\"\n",
						writeBase,
					)
				}
			}
		}
		// never create codex_proxy as a provider
		if providerID == "codex_proxy" {
			providerID = deriveProviderID(baseURL, displayName)
			if providerID == "" || providerID == "custom" {
				providerID = "deepseek"
			}
			envVar = providerEnvVarName(providerID, displayName)
		}
		next = applyCodexModelSwitch(content, model, providerID, displayName, writeBase, apiKey)
		next = removeTomlProviderBlock(next, "codex_proxy")
		// 1) 全局系统环境变量: 厂家名称_api_key = api_key
		// 2) config.toml env_key = "厂家名称_api_key"
		// 3) 写入 shell / launchctl / setx 保证生效
		if apiKey != "" {
			if err := setSystemEnvVar(envVar, apiKey); err != nil {
				// still continue — config was written; surface warning in message
				autoBakMsg += "；环境变量写入警告: " + err.Error()
			} else {
				autoBakMsg += "；已设置系统环境变量 " + envVar
			}
		}
		if writeBase != "" && isLocalProxyURL(writeBase) {
			autoBakMsg += "；base_url→本地代理"
		}
	case ToolClaude:
		// DeepSeek: use official Anthropic-compatible endpoint + multi-model env mapping
		if isDeepSeekHint(providerID, baseURL, model) {
			if apiKey == "" {
				apiKey = loadProviderSecret("deepseek")
			}
			main := normalizeDeepSeekMainModel(model)
			haiku := deepSeekDefaultFlash
			if strings.Contains(strings.ToLower(main), "flash") {
				haiku = main
			}
			next, err = applyClaudeDeepSeekSettings(content, apiKey, main, haiku, "max")
			if err != nil {
				return ToolConfigStatus{}, err
			}
			if apiKey != "" {
				if envErr := setClaudeDeepSeekSystemEnv(apiKey, main, haiku, "max", true); envErr != nil {
					autoBakMsg += "；环境变量写入警告: " + envErr.Error()
				} else {
					autoBakMsg += "；已按 DeepSeek 官方写入 Claude 环境变量"
				}
			}
			model = main // reflect normalized id in status message
		} else {
			next, err = applyClaudeModelSwitch(content, model, baseURL, apiKey, providerID)
			if err != nil {
				return ToolConfigStatus{}, err
			}
			// Claude: also set system env for third-party gateways
			if apiKey != "" {
				_ = setSystemEnvVar("ANTHROPIC_API_KEY", apiKey)
				_ = setSystemEnvVar("ANTHROPIC_AUTH_TOKEN", apiKey)
				if baseURL != "" {
					_ = setSystemEnvVar("ANTHROPIC_BASE_URL", baseURL)
				}
			}
		}
	}

	next = preserveLineEndings(content, next)
	if err := writeFileAtomic(path, next); err != nil {
		return ToolConfigStatus{}, err
	}

	_, _ = a.SetToolConfigPath(string(k), path)
	st := a.resolveTool(k)
	msg := "模型已切换为 " + model + autoBakMsg
	if k == ToolCodex {
		envVar := providerEnvVarName(providerID, displayName)
		msg += "；env_key=" + envVar
	}
	st.Message = msg
	return st, nil
}

// SetToolModel keeps the simple API; prefers ApplyToolModel when base URL/key needed.
func (a *App) SetToolModel(kind, path, model, provider string) (ToolConfigStatus, error) {
	return a.ApplyToolModel(ModelApplyRequest{
		Kind:     kind,
		Path:     path,
		Model:    model,
		Provider: provider,
	})
}

// --- Codex TOML ---

func applyCodexModelSwitch(content, model, providerID, displayName, baseURL, apiKey string) string {
	// 1) top-level model + model_provider
	out := setTomlTopLevelString(content, "model", model)
	out = setTomlTopLevelString(out, "model_provider", providerID)

	// 2) ALWAYS normalize [model_providers.<id>] so env_key/api_key stay consistent.
	//    Previously we skipped this when baseURL+apiKey were empty, which left blocks
	//    without env_key = "DEEPSEEK_API_KEY" after a model-only switch.
	if providerID != "" {
		// Fill missing key from our secret store / existing block
		if apiKey == "" {
			apiKey = loadProviderSecret(providerID)
		}
		if apiKey == "" {
			apiKey = readExistingProviderField(content, providerID, "api_key")
			// legacy: secret mistakenly stored in env_key
			if apiKey == "" {
				if ek := readExistingProviderField(content, providerID, "env_key"); ek != "" && !looksLikeEnvVarName(ek) {
					apiKey = ek
				}
			}
		}
		if baseURL == "" {
			baseURL = readExistingProviderField(content, providerID, "base_url")
		}
		if displayName == "" || displayName == model {
			if n := readExistingProviderField(content, providerID, "name"); n != "" {
				displayName = n
			}
		}
		out = upsertCodexModelProvider(out, providerID, displayName, baseURL, apiKey)
	}

	// 3) ensure [[models]] entry linking model + provider
	out = upsertCodexModelsEntry(out, displayName, providerID, model)
	// 4) strip deprecated wire_api keys from all provider tables
	out = stripAllWireAPI(out)
	return out
}

// readExistingProviderField reads one string field from [model_providers.<id>].
func readExistingProviderField(content, providerID, field string) string {
	reHeader := regexp.MustCompile(`(?m)^\[model_providers\.` + regexp.QuoteMeta(providerID) + `\]\s*$`)
	loc := reHeader.FindStringIndex(content)
	if loc == nil {
		return ""
	}
	end := findTomlTableEnd(content, loc[1])
	block := content[loc[0]:end]
	kv, _ := parseProviderKV(block)
	return strings.TrimSpace(kv[field])
}

// loadProviderSecret loads a previously saved key for this provider from secrets store.
func loadProviderSecret(providerID string) string {
	secPath := filepath.Join(managerRoot(), "env", "secrets.json")
	b, err := os.ReadFile(secPath)
	if err != nil {
		return ""
	}
	var secrets map[string]string
	if json.Unmarshal(b, &secrets) != nil {
		return ""
	}
	// preferred: 厂家名称_api_key
	if v := secrets[providerEnvVarName(providerID, providerID)]; v != "" {
		return v
	}
	if envName, ok := secrets["_provider_"+providerID]; ok && envName != "" {
		if v := secrets[envName]; v != "" {
			return v
		}
	}
	return ""
}

func deriveProviderID(baseURL, name string) string {
	// try host
	u := strings.ToLower(baseURL)
	u = strings.TrimPrefix(u, "https://")
	u = strings.TrimPrefix(u, "http://")
	host := u
	if i := strings.Index(host, "/"); i >= 0 {
		host = host[:i]
	}
	var id string
	switch {
	case strings.Contains(host, "deepseek"):
		id = "deepseek"
	case strings.Contains(host, "openai"):
		id = "openai-custom" // reserved built-in: cannot use "openai"
	case strings.Contains(host, "anthropic"):
		id = "anthropic-custom" // avoid reserved built-in if any
	case strings.Contains(host, "moonshot"):
		id = "moonshot"
	case strings.Contains(host, "dashscope") || strings.Contains(host, "aliyun"):
		id = "qwen"
	case strings.Contains(host, "bigmodel") || strings.Contains(host, "zhipu"):
		id = "zhipu"
	case strings.Contains(host, "minimax"):
		id = "minimax"
	case strings.Contains(host, "localhost") || strings.Contains(host, "127.0.0.1") || strings.Contains(host, "ollama"):
		// local/ollama — never use reserved "ollama"
		if strings.Contains(host, "ollama") || strings.Contains(host, "11434") {
			id = "ollama-local"
		} else {
			id = "local"
		}
	}
	if id == "" {
		// slug from name
		slug := slugify(name)
		if slug != "" && slug != "custom" {
			id = slug
		}
	}
	if id == "" && host != "" {
		parts := strings.Split(host, ".")
		if len(parts) > 0 && parts[0] != "api" && parts[0] != "www" {
			id = slugify(parts[0])
		} else if len(parts) > 1 {
			id = slugify(parts[len(parts)-2])
		}
	}
	if id == "" {
		id = "custom"
	}
	return sanitizeCodexProviderID(id)
}

func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		} else if r == '-' || r == '_' || r == ' ' {
			b.WriteByte('_')
		}
	}
	out := strings.Trim(b.String(), "_")
	if out == "" {
		return "custom"
	}
	return out
}

func upsertCodexModelProvider(content, id, name, baseURL, apiKey string) string {
	if id == "" {
		return content
	}
	if name == "" {
		name = id
	}

	// Find existing [model_providers.id] section
	reHeader := regexp.MustCompile(`(?m)^\[model_providers\.` + regexp.QuoteMeta(id) + `\]\s*$`)
	loc := reHeader.FindStringIndex(content)
	if loc == nil {
		newBlock := buildProviderBlock(id, name, baseURL, apiKey, nil)
		insertAt := strings.Index(content, "[[models]]")
		if insertAt < 0 {
			if strings.TrimSpace(content) == "" {
				return newBlock
			}
			if !strings.HasSuffix(content, "\n") {
				content += "\n"
			}
			return content + "\n" + newBlock
		}
		return content[:insertAt] + newBlock + "\n" + content[insertAt:]
	}

	start := loc[0]
	end := findTomlTableEnd(content, loc[1])
	oldBlock := content[start:end]
	kv, order := parseProviderKV(oldBlock)
	merged := buildProviderBlock(id, name, baseURL, apiKey, &providerKV{kv: kv, order: order})
	return content[:start] + merged + content[end:]
}

type providerKV struct {
	kv    map[string]string
	order []string
}

func parseProviderKV(oldBlock string) (map[string]string, []string) {
	kv := map[string]string{}
	var order []string
	for _, line := range strings.Split(oldBlock, "\n") {
		trim := strings.TrimSpace(line)
		if trim == "" || strings.HasPrefix(trim, "#") || strings.HasPrefix(trim, "[") {
			continue
		}
		if i := strings.Index(trim, "="); i > 0 {
			k := strings.TrimSpace(trim[:i])
			v := strings.TrimSpace(trim[i+1:])
			v = strings.Trim(v, `"`)
			if _, ok := kv[k]; !ok {
				order = append(order, k)
			}
			kv[k] = v
		}
	}
	return kv, order
}

// looksLikeEnvVarName reports whether s is an env var name (OFFICIAL env_key usage),
// not a raw API secret mistakenly stored in env_key.
func looksLikeEnvVarName(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" || len(s) > 64 {
		return false
	}
	// secrets often start with sk- / rk- / key-
	low := strings.ToLower(s)
	if strings.HasPrefix(low, "sk-") || strings.HasPrefix(low, "rk-") ||
		strings.HasPrefix(low, "key-") || strings.HasPrefix(low, "bearer ") {
		return false
	}
	// env names: UPPER_SNAKE or mixed alnum underscore, no spaces
	hasLetter := false
	for i, r := range s {
		if r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' {
			hasLetter = true
		}
		if !(r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_') {
			return false
		}
		if i == 0 && r >= '0' && r <= '9' {
			return false
		}
	}
	if !hasLetter {
		return false
	}
	// long base64-ish blobs are secrets
	if len(s) >= 32 && !strings.Contains(s, "_") {
		return false
	}
	return true
}

// defaultEnvKeyName is an alias of providerEnvVarName for backward-compatible call sites.
func defaultEnvKeyName(providerID string) string {
	return providerEnvVarName(providerID, providerID)
}

// buildProviderBlock writes a compatible [model_providers.x] table.
// Compatibility rules when apiKey is provided:
//  1. Always set env_key to a proper env var NAME (Codex official).
//  2. Always set api_key to the secret when we have one (community / inline builds).
//  3. Preserve prior env_key name if it was already a valid env var name.
//  4. If prior env_key held a raw secret (misuse), migrate: move secret→api_key, env_key→standard name.
//  5. Preserve unrelated keys (http_headers, query_params, etc.).
//  6. Do NOT write wire_api — deprecated by Codex; strip if present.
func buildProviderBlock(id, name, baseURL, apiKey string, existing *providerKV) string {
	kv := map[string]string{}
	var order []string
	if existing != nil {
		for k, v := range existing.kv {
			// wire_api is deprecated in Codex — never keep or rewrite it
			if k == "wire_api" {
				continue
			}
			kv[k] = v
		}
		for _, k := range existing.order {
			if k == "wire_api" {
				continue
			}
			order = append(order, k)
		}
	}

	setKV := func(k, v string) {
		if k == "wire_api" {
			return
		}
		if _, ok := kv[k]; !ok {
			order = append(order, k)
		}
		kv[k] = v
	}

	if name != "" {
		setKV("name", name)
	}
	if baseURL != "" {
		setKV("base_url", baseURL)
	}

	// --- env_key + api_key ---
	// Rule (user):
	//   1) System env:  {厂家名称}_api_key = <api_key value>
	//   2) config.toml: env_key = "{厂家名称}_api_key"   (NAME only)
	//   3) Ensure system env takes effect (launchctl / setx / shell profile)
	existingEnv := strings.TrimSpace(kv["env_key"])
	existingAPI := strings.TrimSpace(kv["api_key"])

	// Recover secret from misused env_key (value looks like a key, not VAR_NAME)
	if apiKey == "" && existingEnv != "" && !looksLikeEnvVarName(existingEnv) {
		apiKey = existingEnv
	}
	if apiKey == "" {
		apiKey = existingAPI
	}

	// Canonical env var name: deepseek_api_key / openai_api_key / ...
	envVar := providerEnvVarName(id, name)

	// Always point env_key at the system variable NAME (never the secret)
	setKV("env_key", envVar)

	// Keep api_key as inline fallback for tools that read config directly
	if apiKey != "" {
		setKV("api_key", apiKey)
	}

	// Preferred field order for readability (no wire_api — deprecated)
	preferred := []string{"name", "base_url", "env_key", "api_key", "requires_openai_auth"}
	var out []string
	out = append(out, "[model_providers."+id+"]")
	seen := map[string]bool{}
	for _, k := range preferred {
		if v, ok := kv[k]; ok {
			out = append(out, k+` = "`+escapeTomlString(v)+`"`)
			seen[k] = true
		}
	}
	for _, k := range order {
		if seen[k] || k == "wire_api" {
			continue
		}
		if v, ok := kv[k]; ok {
			// skip empty
			if v == "" {
				continue
			}
			out = append(out, k+` = "`+escapeTomlString(v)+`"`)
			seen[k] = true
		}
	}
	return strings.Join(out, "\n") + "\n"
}

// findTomlTableEnd returns the index of the next line that starts a [table], or len(content).
func findTomlTableEnd(content string, from int) int {
	if from >= len(content) {
		return len(content)
	}
	// search from next line
	i := from
	for i < len(content) {
		// find line start
		lineEnd := strings.IndexByte(content[i:], '\n')
		var line string
		if lineEnd < 0 {
			line = content[i:]
			trim := strings.TrimSpace(line)
			if strings.HasPrefix(trim, "[") {
				return i
			}
			return len(content)
		}
		line = content[i : i+lineEnd]
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "[") {
			return i
		}
		i += lineEnd + 1
	}
	return len(content)
}

// saveProviderSecret stores API key for env_key resolution and generates a
// sourceable env file: ~/.codex-manager/env/providers.env
func saveProviderSecret(providerID, envName, apiKey string) error {
	if envName == "" || apiKey == "" {
		return nil
	}
	dir := filepath.Join(managerRoot(), "env")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}

	// secrets map
	secPath := filepath.Join(dir, "secrets.json")
	secrets := map[string]string{}
	if b, err := os.ReadFile(secPath); err == nil {
		_ = json.Unmarshal(b, &secrets)
	}
	secrets[envName] = apiKey
	secrets["_provider_"+providerID] = envName
	b, err := json.MarshalIndent(secrets, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(secPath, b, 0o600); err != nil {
		return err
	}

	// shell-exportable env file (all known secrets)
	var lines []string
	lines = append(lines, "# Auto-generated by Codex model manager — do not commit")
	lines = append(lines, "# source ~/.codex-manager/env/providers.env")
	// stable order
	keys := make([]string, 0, len(secrets))
	for k := range secrets {
		if strings.HasPrefix(k, "_") {
			continue
		}
		keys = append(keys, k)
	}
	// simple sort
	for i := 0; i < len(keys); i++ {
		for j := i + 1; j < len(keys); j++ {
			if keys[j] < keys[i] {
				keys[i], keys[j] = keys[j], keys[i]
			}
		}
	}
	for _, k := range keys {
		// single-quote escape for shell
		v := secrets[k]
		v = strings.ReplaceAll(v, `'`, `'"'"'`)
		lines = append(lines, fmt.Sprintf("export %s='%s'", k, v))
	}
	envFile := filepath.Join(dir, "providers.env")
	return os.WriteFile(envFile, []byte(strings.Join(lines, "\n")+"\n"), 0o600)
}

// upsertCodexModelsEntry keeps exactly ONE [[models]] entry — the model being
// switched to. Older model rows are removed so config.toml does not accumulate
// history when the user changes models.
func upsertCodexModelsEntry(content, name, provider, model string) string {
	model = strings.TrimSpace(model)
	provider = strings.TrimSpace(provider)
	name = strings.TrimSpace(name)
	if model == "" {
		// still strip multi-model clutter if any
		return rewriteCodexModelsSection(content, nil)
	}
	if name == "" {
		name = model
	}
	return rewriteCodexModelsSection(content, []ModelOption{
		{ID: model, Name: name, Provider: provider},
	})
}

// rewriteCodexModelsSection removes every [[models]] table and appends a clean,
// deduplicated list at the end of the file.
func rewriteCodexModelsSection(content string, entries []ModelOption) string {
	// strip all [[models]] array-of-tables blocks
	lines := strings.Split(content, "\n")
	var head []string
	inModels := false
	for _, line := range lines {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "[") {
			if trim == "[[models]]" {
				inModels = true
				continue
			}
			// any other table ends models rows
			inModels = false
			head = append(head, line)
			continue
		}
		if inModels {
			continue
		}
		head = append(head, line)
	}

	// trim trailing empty lines in head
	for len(head) > 0 && strings.TrimSpace(head[len(head)-1]) == "" {
		head = head[:len(head)-1]
	}

	var b strings.Builder
	b.WriteString(strings.Join(head, "\n"))
	if b.Len() > 0 {
		b.WriteString("\n")
	}
	for _, e := range entries {
		mid := strings.TrimSpace(e.ID)
		if mid == "" {
			continue
		}
		n := strings.TrimSpace(e.Name)
		if n == "" {
			n = mid
		}
		b.WriteString("\n[[models]]\n")
		b.WriteString(`name = "` + escapeTomlString(n) + "\"\n")
		if strings.TrimSpace(e.Provider) != "" {
			b.WriteString(`provider = "` + escapeTomlString(e.Provider) + "\"\n")
		}
		b.WriteString(`model = "` + escapeTomlString(mid) + "\"\n")
	}
	return b.String()
}

// --- Claude Code settings.json ---
// Official / community tutorial pattern:
// {
//   "model": "...",
//   "env": {
//     "ANTHROPIC_BASE_URL": "...",
//     "ANTHROPIC_AUTH_TOKEN": "...",  // or ANTHROPIC_API_KEY
//     "ANTHROPIC_MODEL": "...",
//     "ANTHROPIC_DEFAULT_SONNET_MODEL": "...",
//     "ANTHROPIC_DEFAULT_HAIKU_MODEL": "..."
//   }
// }

func applyClaudeModelSwitch(content, model, baseURL, apiKey, providerHint string) (string, error) {
	// DeepSeek path: official Anthropic-compatible mapping (not OpenAI /v1 base)
	if isDeepSeekHint(providerHint, baseURL, model) {
		main := normalizeDeepSeekMainModel(model)
		haiku := deepSeekDefaultFlash
		if strings.Contains(strings.ToLower(main), "flash") {
			haiku = main
		}
		return applyClaudeDeepSeekSettings(content, apiKey, main, haiku, "max")
	}

	var raw map[string]any
	if strings.TrimSpace(content) == "" {
		raw = map[string]any{}
	} else if err := json.Unmarshal([]byte(content), &raw); err != nil {
		return "", fmt.Errorf("解析 Claude settings.json 失败: %w", err)
	}
	if raw == nil {
		raw = map[string]any{}
	}

	// top-level model (official settings key)
	raw["model"] = model

	env, _ := raw["env"].(map[string]any)
	if env == nil {
		env = map[string]any{}
	}
	applyClaudeEnvForProvider(env, model, baseURL, apiKey, providerHint)
	raw["env"] = env

	// Also persist for shell export (Claude may inherit user shell env)
	if apiKey != "" {
		_ = saveProviderSecret("claude", "ANTHROPIC_API_KEY", apiKey)
		_ = saveProviderSecret("claude_auth", "ANTHROPIC_AUTH_TOKEN", apiKey)
		_ = os.Setenv("ANTHROPIC_API_KEY", apiKey)
		_ = os.Setenv("ANTHROPIC_AUTH_TOKEN", apiKey)
		if baseURL != "" {
			_ = os.Setenv("ANTHROPIC_BASE_URL", baseURL)
		}
	}

	b, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return "", err
	}
	return string(b) + "\n", nil
}

