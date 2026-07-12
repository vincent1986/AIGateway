package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	goruntime "runtime"
	"strings"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

type ToolKind string

const (
	ToolCodex         ToolKind = "codex"
	ToolClaude        ToolKind = "claude"
	ToolClaudeDesktop ToolKind = "claude_desktop"
	ToolGemini        ToolKind = "gemini"
	ToolOpenCode      ToolKind = "opencode"
	ToolOpenClaw      ToolKind = "openclaw"
	ToolHermes        ToolKind = "hermes"
)

type ToolConfigSpec struct {
	Kind        ToolKind `json:"kind"`
	Name        string   `json:"name"`
	ConfigStyle string   `json:"configStyle"`
}

type ModelOption struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Provider string `json:"provider"`
}

type ToolConfigStatus struct {
	Kind                string        `json:"kind"`
	Name                string        `json:"name"`
	Path                string        `json:"path"`
	Found               bool          `json:"found"`
	Exists              bool          `json:"exists"`
	Model               string        `json:"model"`
	ModelProvider       string        `json:"modelProvider"`
	SearchPaths         []string      `json:"searchPaths"`
	Candidates          []ModelOption `json:"candidates"`
	Source              string        `json:"source"`
	Message             string        `json:"message"`
	OS                  string        `json:"os"`
	HasDefaultBackup    bool          `json:"hasDefaultBackup"`
	DefaultBackupPath   string        `json:"defaultBackupPath"`
	DefaultBackupAt     string        `json:"defaultBackupAt"`
	DefaultBackupOrigin string        `json:"defaultBackupOrigin"`
}

type pathOverrides struct {
	Codex         string `json:"codex"`
	Claude        string `json:"claude"`
	ClaudeDesktop string `json:"claudeDesktop"`
	Gemini        string `json:"gemini"`
	OpenCode      string `json:"opencode"`
	OpenClaw      string `json:"openclaw"`
	Hermes        string `json:"hermes"`
}

func (a *App) overridesPath() string {
	return filepath.Join(managerRoot(), "paths.json")
}
func (a *App) loadOverrides() pathOverrides {
	var o pathOverrides
	b, err := os.ReadFile(a.overridesPath())
	if err != nil {
		return o
	}
	_ = json.Unmarshal(b, &o)
	return o
}
func (a *App) saveOverrides(o pathOverrides) error {
	_ = os.MkdirAll(managerRoot(), 0o755)
	b, _ := json.MarshalIndent(o, "", "  ")
	return writeFileAtomic(a.overridesPath(), string(b))
}

func codexSearchPaths() []string {
	var paths []string
	if v := os.Getenv("CODEX_HOME"); v != "" {
		paths = append(paths, filepath.Join(expandPath(v), "config.toml"))
	}
	if h := userHome(); h != "" {
		paths = append(paths, filepath.Join(h, ".codex", "config.toml"))
	}
	return paths
}
func claudeSearchPaths() []string {
	var paths []string
	if v := os.Getenv("CLAUDE_CONFIG_DIR"); v != "" {
		paths = append(paths, filepath.Join(expandPath(v), "settings.json"))
	}
	if h := userHome(); h != "" {
		paths = append(paths, filepath.Join(h, ".claude", "settings.json"), filepath.Join(h, ".claude.json"))
	}
	return paths
}

func toolConfigSpecs() []ToolConfigSpec {
	return []ToolConfigSpec{
		{Kind: ToolCodex, Name: "Codex", ConfigStyle: "toml"},
		{Kind: ToolClaude, Name: "Claude Code", ConfigStyle: "json"},
		{Kind: ToolClaudeDesktop, Name: "Claude Desktop", ConfigStyle: "json"},
		{Kind: ToolGemini, Name: "Gemini CLI", ConfigStyle: "json_or_env"},
		{Kind: ToolOpenCode, Name: "OpenCode", ConfigStyle: "json"},
		{Kind: ToolOpenClaw, Name: "OpenClaw", ConfigStyle: "json5"},
		{Kind: ToolHermes, Name: "Hermes Agent", ConfigStyle: "yaml_or_json"},
	}
}

func toolSpec(kind ToolKind) (ToolConfigSpec, bool) {
	for _, spec := range toolConfigSpecs() {
		if spec.Kind == kind {
			return spec, true
		}
	}
	return ToolConfigSpec{}, false
}

func (a *App) ListToolConfigSpecs() []ToolConfigSpec {
	return toolConfigSpecs()
}

func genericToolSearchPaths(kind ToolKind) []string {
	home := userHome()
	if home == "" {
		return nil
	}
	switch kind {
	case ToolClaudeDesktop:
		return []string{filepath.Join(home, "Library", "Application Support", "Claude-3p", "claude_desktop_config.json"), filepath.Join(home, "Library", "Application Support", "Claude", "claude_desktop_config.json")}
	case ToolGemini:
		return []string{filepath.Join(home, ".gemini", "settings.json"), filepath.Join(home, ".gemini", ".env")}
	case ToolOpenCode:
		return []string{filepath.Join(home, ".config", "opencode", "opencode.json"), filepath.Join(home, ".opencode", "opencode.json"), filepath.Join(home, ".opencode", "config.json")}
	case ToolOpenClaw:
		return []string{filepath.Join(home, ".openclaw", "openclaw.json"), filepath.Join(home, ".openclaw", "config.json")}
	case ToolHermes:
		return []string{filepath.Join(home, ".hermes", "config.yaml"), filepath.Join(home, ".hermes", "config.json"), filepath.Join(home, ".config", "hermes", "config.yaml")}
	default:
		return nil
	}
}

func (a *App) DiscoverToolConfigs() []ToolConfigStatus {
	out := make([]ToolConfigStatus, 0, len(toolConfigSpecs()))
	for _, spec := range toolConfigSpecs() {
		out = append(out, a.resolveTool(spec.Kind))
	}
	return out
}
func (a *App) GetToolConfig(kind string) ToolConfigStatus {
	return a.resolveTool(ToolKind(strings.ToLower(kind)))
}

func (a *App) resolveTool(kind ToolKind) ToolConfigStatus {
	st := ToolConfigStatus{Kind: string(kind), OS: goruntime.GOOS}
	switch kind {
	case ToolCodex:
		st.Name, st.SearchPaths = "Codex", codexSearchPaths()
	case ToolClaude:
		st.Name, st.SearchPaths = "Claude Code", claudeSearchPaths()
	default:
		if spec, ok := toolSpec(kind); ok {
			st.Name, st.SearchPaths = spec.Name, genericToolSearchPaths(kind)
		} else {
			st.Message = "未知工具"
			return st
		}
	}
	ov := a.loadOverrides()
	override := ""
	switch kind {
	case ToolCodex:
		override = ov.Codex
	case ToolClaude:
		override = ov.Claude
	case ToolClaudeDesktop:
		override = ov.ClaudeDesktop
	case ToolGemini:
		override = ov.Gemini
	case ToolOpenCode:
		override = ov.OpenCode
	case ToolOpenClaw:
		override = ov.OpenClaw
	case ToolHermes:
		override = ov.Hermes
	}
	if override != "" {
		st.Path = expandPath(override)
		st.Source = "override"
		if fileExists(st.Path) {
			st.Found, st.Exists = true, true
			a.fillFromFile(&st)
			st.Message = "使用手动路径"
			return st
		}
		st.Message = "手动路径不存在"
	}
	for _, p := range st.SearchPaths {
		if fileExists(p) {
			st.Path, st.Found, st.Exists, st.Source = p, true, true, "auto"
			a.fillFromFile(&st)
			st.Message = "自动搜索成功"
			return st
		}
	}
	if len(st.SearchPaths) > 0 {
		st.Path = st.SearchPaths[0]
	}
	st.Message = "未找到配置文件"
	return st
}

func (a *App) fillFromFile(st *ToolConfigStatus) {
	b, err := os.ReadFile(st.Path)
	if err != nil {
		st.Message = err.Error()
		return
	}
	content := string(b)
	if readManagedToolStatus(st, content) {
		return
	}
	switch ToolKind(st.Kind) {
	case ToolCodex:
		st.Model = readTomlTopLevelString(content, "model")
		st.ModelProvider = readTomlTopLevelString(content, "model_provider")
		st.Candidates = parseCodexModels(content)
	case ToolGemini:
		if strings.HasSuffix(strings.ToLower(st.Path), ".env") {
			st.Model = readEnvValue(content, "GEMINI_MODEL")
			st.ModelProvider = readEnvValue(content, "GOOGLE_GEMINI_BASE_URL")
			if st.ModelProvider == "" {
				st.ModelProvider = readEnvValue(content, "GEMINI_API_BASE")
			}
			return
		}
		fillFromLooseJSON(st, content)
	case ToolOpenClaw:
		fillFromLooseJSON(st, stripJSON5(content))
	case ToolHermes:
		if strings.HasSuffix(strings.ToLower(st.Path), ".yaml") || strings.HasSuffix(strings.ToLower(st.Path), ".yml") {
			st.Model = readYAMLScalar(content, "model")
			st.ModelProvider = readYAMLScalar(content, "base_url")
			return
		}
		fillFromLooseJSON(st, content)
	default:
		fillFromLooseJSON(st, content)
	}
}

func fillFromLooseJSON(st *ToolConfigStatus, content string) {
	var raw map[string]any
	if json.Unmarshal([]byte(content), &raw) == nil {
		if v, ok := raw["model"].(string); ok {
			st.Model = v
		}
		if env, ok := raw["env"].(map[string]any); ok {
			if v, ok := env["ANTHROPIC_BASE_URL"].(string); ok {
				st.ModelProvider = v
			}
		}
	}
}

func readEnvValue(content, key string) string {
	re := regexp.MustCompile(`(?m)^\s*(?:export\s+)?` + regexp.QuoteMeta(key) + `\s*=\s*["']?([^"'\r\n#]*)`)
	if m := re.FindStringSubmatch(content); len(m) == 2 {
		return strings.TrimSpace(m[1])
	}
	return ""
}

func readYAMLScalar(content, key string) string {
	re := regexp.MustCompile(`(?m)^\s*` + regexp.QuoteMeta(key) + `\s*:\s*["']?([^"'\r\n#]+)`)
	if m := re.FindStringSubmatch(content); len(m) == 2 {
		return strings.TrimSpace(m[1])
	}
	return ""
}

func stripJSON5(content string) string {
	blockComments := regexp.MustCompile(`(?s)/\*.*?\*/`)
	lineComments := regexp.MustCompile(`(?m)^\s*//.*$`)
	trailingCommas := regexp.MustCompile(`,\s*([}\]])`)
	content = blockComments.ReplaceAllString(content, "")
	content = lineComments.ReplaceAllString(content, "")
	return trailingCommas.ReplaceAllString(content, "$1")
}

func readTomlTopLevelString(content, key string) string {
	re := regexp.MustCompile(`(?m)^\s*` + regexp.QuoteMeta(key) + `\s*=\s*"([^"]*)"`)
	lines := strings.Split(content, "\n")
	inTable := false
	for _, line := range lines {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "[") {
			inTable = true
			continue
		}
		if inTable {
			continue
		}
		if m := re.FindStringSubmatch(line); len(m) == 2 {
			return m[1]
		}
	}
	if m := re.FindStringSubmatch(content); len(m) == 2 {
		return m[1]
	}
	return ""
}

func setTomlTopLevelString(content, key, value string) string {
	re := regexp.MustCompile(`(?m)^(\s*)` + regexp.QuoteMeta(key) + `\s*=\s*"[^"]*"`)
	lines := strings.Split(content, "\n")
	inTable := false
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "[") {
			inTable = true
		}
		if inTable {
			continue
		}
		if re.MatchString(line) {
			lines[i] = re.ReplaceAllString(line, `${1}`+key+` = "`+escapeTomlString(value)+`"`)
			return strings.Join(lines, "\n")
		}
	}
	insert := key + ` = "` + escapeTomlString(value) + `"`
	return insert + "\n" + content
}

func escapeTomlString(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	return strings.ReplaceAll(s, `"`, `\"`)
}

func parseCodexModels(content string) []ModelOption {
	var out []ModelOption
	lines := strings.Split(content, "\n")
	in, name, model, prov := false, "", "", ""
	flush := func() {
		if in && model != "" {
			n := name
			if n == "" {
				n = model
			}
			out = append(out, ModelOption{ID: model, Name: n, Provider: prov})
		}
		name, model, prov = "", "", ""
	}
	for _, line := range lines {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "[") {
			if trim == "[[models]]" {
				flush()
				in = true
				continue
			}
			flush()
			in = false
			continue
		}
		if !in {
			continue
		}
		if m := regexp.MustCompile(`^\s*model\s*=\s*"([^"]*)"`).FindStringSubmatch(line); len(m) == 2 {
			model = m[1]
		}
		if m := regexp.MustCompile(`^\s*name\s*=\s*"([^"]*)"`).FindStringSubmatch(line); len(m) == 2 {
			name = m[1]
		}
		if m := regexp.MustCompile(`^\s*provider\s*=\s*"([^"]*)"`).FindStringSubmatch(line); len(m) == 2 {
			prov = m[1]
		}
	}
	flush()
	return out
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
			v := strings.Trim(strings.TrimSpace(trim[i+1:]), `"`)
			if _, ok := kv[k]; !ok {
				order = append(order, k)
			}
			kv[k] = v
		}
	}
	return kv, order
}

func findTomlTableEnd(content string, from int) int {
	i := from
	for i < len(content) {
		lineEnd := strings.IndexByte(content[i:], '\n')
		if lineEnd < 0 {
			line := content[i:]
			if strings.HasPrefix(strings.TrimSpace(line), "[") {
				return i
			}
			return len(content)
		}
		line := content[i : i+lineEnd]
		if strings.HasPrefix(strings.TrimSpace(line), "[") {
			return i
		}
		i += lineEnd + 1
	}
	return len(content)
}

func (a *App) PickToolConfig(kind string) (ToolConfigStatus, error) {
	k := ToolKind(strings.ToLower(kind))
	home := userHome()
	filters := []wailsruntime.FileFilter{{DisplayName: "All", Pattern: "*.*"}}
	title := "选择配置文件"
	def := home
	if k == ToolCodex {
		title = "选择 Codex config.toml"
		filters = []wailsruntime.FileFilter{{DisplayName: "TOML", Pattern: "*.toml"}}
		def = filepath.Join(home, ".codex")
	} else {
		title = "选择 Claude settings.json"
		filters = []wailsruntime.FileFilter{{DisplayName: "JSON", Pattern: "*.json"}}
		def = filepath.Join(home, ".claude")
	}
	if st, err := os.Stat(def); err != nil || !st.IsDir() {
		def = home
	}
	path, err := wailsruntime.OpenFileDialog(a.ctx, wailsruntime.OpenDialogOptions{
		Title: title, DefaultDirectory: def, Filters: filters, ShowHiddenFiles: true,
	})
	if err != nil {
		return ToolConfigStatus{}, err
	}
	if path == "" {
		return a.resolveTool(k), nil
	}
	return a.SetToolConfigPath(string(k), path)
}

func (a *App) SetToolConfigPath(kind, path string) (ToolConfigStatus, error) {
	k := ToolKind(strings.ToLower(kind))
	path = expandPath(path)
	ov := a.loadOverrides()
	switch k {
	case ToolCodex:
		ov.Codex = path
	case ToolClaude:
		ov.Claude = path
	case ToolClaudeDesktop:
		ov.ClaudeDesktop = path
	case ToolGemini:
		ov.Gemini = path
	case ToolOpenCode:
		ov.OpenCode = path
	case ToolOpenClaw:
		ov.OpenClaw = path
	case ToolHermes:
		ov.Hermes = path
	}
	_ = a.saveOverrides(ov)
	return a.resolveTool(k), nil
}

func (a *App) ClearToolConfigPath(kind string) (ToolConfigStatus, error) {
	k := ToolKind(strings.ToLower(kind))
	ov := a.loadOverrides()
	switch k {
	case ToolCodex:
		ov.Codex = ""
	case ToolClaude:
		ov.Claude = ""
	case ToolClaudeDesktop:
		ov.ClaudeDesktop = ""
	case ToolGemini:
		ov.Gemini = ""
	case ToolOpenCode:
		ov.OpenCode = ""
	case ToolOpenClaw:
		ov.OpenClaw = ""
	case ToolHermes:
		ov.Hermes = ""
	}
	_ = a.saveOverrides(ov)
	return a.resolveTool(k), nil
}

func (a *App) ReadConfigText(path string) (string, error) {
	path = expandPath(path)
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	if len(b) > 200000 {
		return string(b[:200000]) + "\n…", nil
	}
	return string(b), nil
}

func (a *App) RevealConfigPath(path string) error {
	path = expandPath(path)
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	switch goruntime.GOOS {
	case "darwin":
		if fileExists(path) {
			return exec.Command("open", "-R", path).Start()
		}
		return exec.Command("open", filepath.Dir(path)).Start()
	case "windows":
		if fileExists(path) {
			return exec.Command("cmd", "/c", "explorer", "/select,"+path).Start()
		}
		return exec.Command("explorer", filepath.Dir(path)).Start()
	default:
		return exec.Command("xdg-open", filepath.Dir(path)).Start()
	}
}

// Minimal stubs for backup APIs used by frontend
func (a *App) BackupDefaultConfig(kind, path string) (ToolConfigStatus, error) {
	return a.resolveTool(ToolKind(kind)), nil
}
func (a *App) RestoreDefaultConfig(kind string) (ToolConfigStatus, error) {
	return a.resolveTool(ToolKind(kind)), fmt.Errorf("请使用备份目录手动恢复")
}
func (a *App) ClearDefaultBackup(kind string) (ToolConfigStatus, error) {
	return a.resolveTool(ToolKind(kind)), nil
}

// Model apply simplified
type ModelApplyRequest struct {
	Kind     string `json:"kind"`
	Path     string `json:"path"`
	Model    string `json:"model"`
	Provider string `json:"provider"`
	BaseURL  string `json:"baseUrl"`
	APIKey   string `json:"apiKey"`
	Name     string `json:"name"`
}

func providerEnvVarName(providerID, displayName string) string {
	base := slugify(providerID)
	if base == "" || base == "custom" {
		base = slugify(displayName)
	}
	if base == "" {
		base = "custom"
	}
	return strings.ToLower(base) + "_api_key"
}

func (a *App) ApplyToolModel(req ModelApplyRequest) (ToolConfigStatus, error) {
	k := ToolKind(strings.ToLower(req.Kind))
	model := strings.TrimSpace(req.Model)
	if model == "" {
		return ToolConfigStatus{}, fmt.Errorf("模型不能为空")
	}
	path := expandPath(req.Path)
	if path == "" {
		path = a.resolveTool(k).Path
	}
	if path == "" {
		return ToolConfigStatus{}, fmt.Errorf("未指定路径")
	}
	var content string
	if fileExists(path) {
		b, err := os.ReadFile(path)
		if err != nil {
			return ToolConfigStatus{}, err
		}
		content = string(b)
	}
	providerID := strings.TrimSpace(req.Provider)
	if providerID == "" {
		providerID = "custom"
	}
	if providerID == "codex_proxy" {
		providerID = "deepseek"
	}
	baseURL := strings.TrimRight(strings.TrimSpace(req.BaseURL), "/")
	apiKey := strings.TrimSpace(req.APIKey)
	name := req.Name
	if name == "" {
		name = model
	}
	writeBase := baseURL
	if a.proxy != nil && a.proxy.status().Running && k != ToolClaudeDesktop && k != ToolGemini {
		writeBase = a.proxy.baseURL()
	}

	switch k {
	case ToolCodex:
		out := setTomlTopLevelString(content, "model", model)
		out = setTomlTopLevelString(out, "model_provider", providerID)
		envVar := providerEnvVarName(providerID, name)
		// ensure provider block
		out = ensureProviderBlock(out, providerID, name, writeBase, envVar, apiKey)
		out = removeTomlProviderBlock(out, "codex_proxy")
		if _, err := writeConfigWithSnapshot(path, out, "apply codex model"); err != nil {
			return ToolConfigStatus{}, err
		}
		if apiKey != "" {
			_ = os.Setenv(envVar, apiKey)
		}
	case ToolClaude:
		var raw map[string]any
		if content != "" {
			_ = json.Unmarshal([]byte(content), &raw)
		}
		if raw == nil {
			raw = map[string]any{}
		}
		raw["model"] = model
		env, _ := raw["env"].(map[string]any)
		if env == nil {
			env = map[string]any{}
		}
		env["ANTHROPIC_MODEL"] = model
		if writeBase != "" {
			env["ANTHROPIC_BASE_URL"] = writeBase
		}
		if apiKey != "" {
			env["ANTHROPIC_API_KEY"] = apiKey
			env["ANTHROPIC_AUTH_TOKEN"] = apiKey
		}
		raw["env"] = env
		b, _ := json.MarshalIndent(raw, "", "  ")
		if _, err := writeConfigWithSnapshot(path, string(b)+"\n", "apply claude model"); err != nil {
			return ToolConfigStatus{}, err
		}
	default:
		writes, err := buildManagedToolWrites(k, path, model, providerID, name, writeBase, apiKey)
		if err != nil {
			return a.resolveTool(k), err
		}
		if _, err := applyConfigWrites(writes); err != nil {
			return a.resolveTool(k), err
		}
	}
	st := a.resolveTool(k)
	st.Message = "模型已切换为 " + model
	if writeBase != "" && isLocalProxyURL(writeBase) {
		st.Message += "；base_url→本地代理"
	}
	return st, nil
}

func (a *App) PreviewApplyToolModel(req ModelApplyRequest) ([]ConfigWriteResult, error) {
	k := ToolKind(strings.ToLower(req.Kind))
	path := expandPath(req.Path)
	if path == "" {
		path = a.resolveTool(k).Path
	}
	providerID := strings.TrimSpace(req.Provider)
	if providerID == "" {
		providerID = "custom"
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = strings.TrimSpace(req.Model)
	}
	base := strings.TrimRight(strings.TrimSpace(req.BaseURL), "/")
	if a.proxy != nil && a.proxy.status().Running && k != ToolClaudeDesktop && k != ToolGemini {
		base = a.proxy.baseURL()
	}
	writes, err := buildManagedToolWrites(k, path, strings.TrimSpace(req.Model), providerID, name, base, strings.TrimSpace(req.APIKey))
	if err != nil {
		return nil, err
	}
	results := make([]ConfigWriteResult, 0, len(writes))
	for _, write := range writes {
		result, err := previewConfigWrite(write.Path, write.Content)
		if err != nil {
			return nil, err
		}
		results = append(results, result)
	}
	return results, nil
}

func (a *App) SetToolModel(kind, path, model, provider string) (ToolConfigStatus, error) {
	return a.ApplyToolModel(ModelApplyRequest{Kind: kind, Path: path, Model: model, Provider: provider})
}

func ensureProviderBlock(content, id, name, baseURL, envKey, apiKey string) string {
	re := regexp.MustCompile(`(?m)^\[model_providers\.` + regexp.QuoteMeta(id) + `\]\s*$`)
	if !re.MatchString(content) {
		content += "\n[model_providers." + id + "]\n"
	}
	for _, field := range []struct{ key, value string }{
		{"name", name}, {"base_url", baseURL}, {"env_key", envKey}, {"api_key", apiKey}, {"wire_api", "chat"},
	} {
		if field.value != "" {
			content = setProviderField(content, id, field.key, field.value)
		}
	}
	return content
}
