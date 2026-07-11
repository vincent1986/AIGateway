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

// ToolKind identifies a target CLI product.
type ToolKind string

const (
	ToolCodex    ToolKind = "codex" // ChatGPT / Codex config paths
	ToolClaude   ToolKind = "claude"
	ToolOpenClaw ToolKind = "openclaw"
	ToolHarness  ToolKind = "harness"
)

// ModelOption is a model entry discovered from a tool config.
type ModelOption struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Provider string `json:"provider"`
}

// ToolConfigStatus describes discovery and current model state.
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
	Source              string        `json:"source"` // auto | manual | override
	Message             string        `json:"message"`
	OS                  string        `json:"os"` // darwin | windows | linux
	HasDefaultBackup    bool          `json:"hasDefaultBackup"`
	DefaultBackupPath   string        `json:"defaultBackupPath"`
	DefaultBackupAt     string        `json:"defaultBackupAt"`
	DefaultBackupOrigin string        `json:"defaultBackupOrigin"`
}

type pathOverrides struct {
	Codex    string `json:"codex"`
	Claude   string `json:"claude"`
	OpenClaw string `json:"openclaw"`
	Harness  string `json:"harness"`
}

func (a *App) overridesPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".codex-manager", "paths.json")
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
	dir := filepath.Dir(a.overridesPath())
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(o, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(a.overridesPath(), b, 0o644)
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

func firstExisting(paths []string) string {
	for _, p := range paths {
		if fileExists(p) {
			return p
		}
	}
	return ""
}

// DiscoverToolConfigs auto-searches registered tool config files.
func (a *App) DiscoverToolConfigs() []ToolConfigStatus {
	out := make([]ToolConfigStatus, 0, len(toolRegistry))
	for _, d := range toolRegistry {
		out = append(out, a.resolveTool(toolKindFromDriverID(d.ToolID())))
	}
	return out
}

// GetToolConfig returns status for one tool kind: "codex" | "claude".
func (a *App) GetToolConfig(kind string) ToolConfigStatus {
	return a.resolveTool(ToolKind(strings.ToLower(strings.TrimSpace(kind))))
}

func (a *App) resolveTool(kind ToolKind) ToolConfigStatus {
	st := ToolConfigStatus{Kind: string(kind), OS: goruntime.GOOS}
	// Map kind → driver (codex stored as kind "codex" for compat)
	driverID := string(kind)
	if kind == ToolCodex {
		driverID = "chatgpt"
	}
	d := driverByID(driverID)
	if d == nil {
		// fallback legacy
		switch kind {
		case ToolCodex:
			st.Name = "ChatGPT"
			st.SearchPaths = codexSearchPaths()
		case ToolClaude:
			st.Name = "Claude Code"
			st.SearchPaths = claudeSearchPaths()
		default:
			st.Message = "未知工具类型"
			return st
		}
	} else {
		st.Name = d.ToolName()
		st.SearchPaths = d.DefaultPaths()
		// expose kind for UI: keep codex/claude/openclaw/harness
		if kind == ToolCodex {
			st.Kind = "codex"
		}
	}

	ov := a.loadOverrides()
	override := ""
	switch kind {
	case ToolCodex:
		override = strings.TrimSpace(ov.Codex)
	case ToolClaude:
		override = strings.TrimSpace(ov.Claude)
	case ToolOpenClaw:
		override = strings.TrimSpace(ov.OpenClaw)
	case ToolHarness:
		override = strings.TrimSpace(ov.Harness)
	}

	if override != "" {
		st.Path = expandPath(override)
		st.Source = "override"
		if fileExists(st.Path) {
			st.Found = true
			st.Exists = true
			a.fillFromFile(&st)
			fillBackupInfo(&st)
			st.Message = "使用手动指定路径"
			return st
		}
		st.Exists = false
		st.Found = false
		st.Message = "手动路径不存在，已回退自动搜索"
	}

	found := firstExisting(st.SearchPaths)
	if found != "" {
		st.Path = found
		st.Found = true
		st.Exists = true
		st.Source = "auto"
		a.fillFromFile(&st)
		fillBackupInfo(&st)
		st.Message = "自动搜索成功"
		return st
	}

	// default preferred path even if missing (OS-correct)
	if d != nil {
		st.Path = d.PreferredPath()
	} else {
		switch kind {
		case ToolCodex:
			st.Path = preferredCodexConfigPath()
		case ToolClaude:
			st.Path = preferredClaudeConfigPath()
		default:
			if len(st.SearchPaths) > 0 {
				st.Path = st.SearchPaths[0]
			}
		}
	}
	st.Found = false
	st.Exists = false
	st.Source = "auto"
	st.Message = "未找到配置文件，请手动选择"
	fillBackupInfo(&st)
	return st
}

func (a *App) fillFromFile(st *ToolConfigStatus) {
	b, err := os.ReadFile(st.Path)
	if err != nil {
		st.Message = "读取失败: " + err.Error()
		return
	}
	content := string(b)
	switch ToolKind(st.Kind) {
	case ToolCodex:
		st.Model = readTomlTopLevelString(content, "model")
		st.ModelProvider = readTomlTopLevelString(content, "model_provider")
		st.Candidates = parseCodexModels(content)
	case ToolClaude:
		st.Model, st.ModelProvider = readClaudeModel(content, st.Path)
	case ToolOpenClaw:
		st.Model, st.ModelProvider = readOpenClawModel(content)
	case ToolHarness:
		st.Model, st.ModelProvider = readHarnessModel(content)
	}
}

func readTomlTopLevelString(content, key string) string {
	// Only match keys before the first table header, or any top-level assignment
	// Prefer first non-comment assignment of key at beginning of line.
	re := regexp.MustCompile(`(?m)^\s*` + regexp.QuoteMeta(key) + `\s*=\s*"([^"]*)"`)
	// Walk line by line and skip those inside tables by simple heuristic:
	// track if we're past a [[models]] etc — still allow global model which is usually near top.
	// For reliability: take the first match that is not under an indented section after [[models]].
	lines := strings.Split(content, "\n")
	inTable := false
	for _, line := range lines {
		trim := strings.TrimSpace(line)
		if trim == "" || strings.HasPrefix(trim, "#") {
			continue
		}
		if strings.HasPrefix(trim, "[") {
			// top-level model should be before any table; once in table, only allow non-array tables?
			// model_provider/model are global — stop at first [table]
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
	// fallback: first occurrence in whole file for model only if not found
	if m := re.FindStringSubmatch(content); len(m) == 2 {
		return m[1]
	}
	return ""
}

func setTomlTopLevelString(content, key, value string) string {
	re := regexp.MustCompile(`(?m)^(\s*)` + regexp.QuoteMeta(key) + `\s*=\s*"[^"]*"`)
	lines := strings.Split(content, "\n")
	inTable := false
	replaced := false
	for i, line := range lines {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "[") {
			inTable = true
		}
		if inTable {
			continue
		}
		if re.MatchString(line) {
			lines[i] = re.ReplaceAllString(line, `${1}`+key+` = "`+escapeTomlString(value)+`"`)
			replaced = true
			break
		}
	}
	if replaced {
		return strings.Join(lines, "\n")
	}
	// insert near top after any leading comments
	insert := key + ` = "` + escapeTomlString(value) + `"`
	if strings.TrimSpace(content) == "" {
		return insert + "\n"
	}
	// put after initial comment block
	idx := 0
	for idx < len(lines) {
		t := strings.TrimSpace(lines[idx])
		if t == "" || strings.HasPrefix(t, "#") {
			idx++
			continue
		}
		break
	}
	out := make([]string, 0, len(lines)+1)
	out = append(out, lines[:idx]...)
	out = append(out, insert)
	out = append(out, lines[idx:]...)
	return strings.Join(out, "\n")
}

func escapeTomlString(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return s
}

func parseCodexModels(content string) []ModelOption {
	reName := regexp.MustCompile(`^\s*name\s*=\s*"([^"]*)"`)
	reModel := regexp.MustCompile(`^\s*model\s*=\s*"([^"]*)"`)
	reProv := regexp.MustCompile(`^\s*provider\s*=\s*"([^"]*)"`)

	var out []ModelOption
	lines := strings.Split(content, "\n")
	inModels := false
	var name, model, prov string
	flush := func() {
		if !inModels {
			return
		}
		if model != "" {
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
				inModels = true
				continue
			}
			// leaving models section
			flush()
			inModels = false
			continue
		}
		if !inModels {
			continue
		}
		if m := reModel.FindStringSubmatch(line); len(m) == 2 {
			model = m[1]
		}
		if m := reName.FindStringSubmatch(line); len(m) == 2 {
			name = m[1]
		}
		if m := reProv.FindStringSubmatch(line); len(m) == 2 {
			prov = m[1]
		}
	}
	flush()
	return out
}

func readClaudeModel(content, path string) (model, provider string) {
	// settings.json or .claude.json
	var raw map[string]any
	if err := json.Unmarshal([]byte(content), &raw); err != nil {
		return "", ""
	}
	if v, ok := raw["model"].(string); ok {
		model = v
	}
	// some setups put provider-ish env under env
	if env, ok := raw["env"].(map[string]any); ok {
		if v, ok := env["ANTHROPIC_MODEL"].(string); ok && model == "" {
			model = v
		}
		if v, ok := env["ANTHROPIC_BASE_URL"].(string); ok {
			provider = v
		}
	}
	_ = path
	return model, provider
}

func setClaudeModel(content, model string) (string, error) {
	// minimal model-only switch; full base/key uses applyClaudeModelSwitch
	return applyClaudeModelSwitch(content, model, "", "", "")
}

// PickToolConfig opens a file dialog to manually choose a config file.
func (a *App) PickToolConfig(kind string) (ToolConfigStatus, error) {
	k := ToolKind(strings.ToLower(strings.TrimSpace(kind)))
	title := "选择配置文件"
	filters := []wailsruntime.FileFilter{}
	defaultDir := defaultDirForKind(k)
	home := userHome()

	switch k {
	case ToolCodex:
		title = "选择 Codex 配置文件 (config.toml)"
		// Windows dialog filters often need both display name and pattern
		filters = []wailsruntime.FileFilter{
			{DisplayName: "TOML (*.toml)", Pattern: "*.toml"},
			{DisplayName: "All files (*.*)", Pattern: "*.*"},
		}
	case ToolClaude:
		title = "选择 Claude Code 配置文件 (settings.json)"
		filters = []wailsruntime.FileFilter{
			{DisplayName: "JSON (*.json)", Pattern: "*.json"},
			{DisplayName: "All files (*.*)", Pattern: "*.*"},
		}
	case ToolOpenClaw:
		title = "选择 OpenClaw 配置文件"
		filters = []wailsruntime.FileFilter{
			{DisplayName: "JSON (*.json)", Pattern: "*.json"},
			{DisplayName: "All files (*.*)", Pattern: "*.*"},
		}
	case ToolHarness:
		title = "选择 Harness 配置文件"
		filters = []wailsruntime.FileFilter{
			{DisplayName: "YAML (*.yaml;*.yml)", Pattern: "*.yaml;*.yml"},
			{DisplayName: "JSON (*.json)", Pattern: "*.json"},
			{DisplayName: "All files (*.*)", Pattern: "*.*"},
		}
	default:
		return ToolConfigStatus{}, fmt.Errorf("未知工具类型: %s", kind)
	}

	if defaultDir != "" {
		if st, err := os.Stat(defaultDir); err != nil || !st.IsDir() {
			defaultDir = home
		}
	}

	path, err := wailsruntime.OpenFileDialog(a.ctx, wailsruntime.OpenDialogOptions{
		Title:            title,
		DefaultDirectory: defaultDir,
		Filters:          filters,
		ShowHiddenFiles:  true, // ~/.codex may be hidden on some systems
	})
	if err != nil {
		return ToolConfigStatus{}, err
	}
	if strings.TrimSpace(path) == "" {
		// cancelled
		return a.resolveTool(k), nil
	}
	return a.SetToolConfigPath(string(k), path)
}

// SetToolConfigPath saves a manual path override and returns status.
func (a *App) SetToolConfigPath(kind, path string) (ToolConfigStatus, error) {
	k := ToolKind(strings.ToLower(strings.TrimSpace(kind)))
	path = expandPath(strings.TrimSpace(path))
	if path == "" {
		return ToolConfigStatus{}, fmt.Errorf("路径不能为空")
	}
	ov := a.loadOverrides()
	switch k {
	case ToolCodex:
		ov.Codex = path
	case ToolClaude:
		ov.Claude = path
	case ToolOpenClaw:
		ov.OpenClaw = path
	case ToolHarness:
		ov.Harness = path
	default:
		return ToolConfigStatus{}, fmt.Errorf("未知工具类型: %s", kind)
	}
	if err := a.saveOverrides(ov); err != nil {
		return ToolConfigStatus{}, err
	}
	st := a.resolveTool(k)
	return st, nil
}

// ClearToolConfigPath removes manual override and re-runs auto search.
func (a *App) ClearToolConfigPath(kind string) (ToolConfigStatus, error) {
	k := ToolKind(strings.ToLower(strings.TrimSpace(kind)))
	ov := a.loadOverrides()
	switch k {
	case ToolCodex:
		ov.Codex = ""
	case ToolClaude:
		ov.Claude = ""
	case ToolOpenClaw:
		ov.OpenClaw = ""
	case ToolHarness:
		ov.Harness = ""
	default:
		return ToolConfigStatus{}, fmt.Errorf("未知工具类型: %s", kind)
	}
	if err := a.saveOverrides(ov); err != nil {
		return ToolConfigStatus{}, err
	}
	return a.resolveTool(k), nil
}

func matchCodexProvider(content, model string) string {
	for _, m := range parseCodexModels(content) {
		if m.ID == model && m.Provider != "" {
			return m.Provider
		}
	}
	return ""
}

// RevealConfigPath opens the file in system file manager (Finder / Explorer).
func (a *App) RevealConfigPath(path string) error {
	path = expandPath(strings.TrimSpace(path))
	if path == "" {
		return fmt.Errorf("路径为空")
	}
	// Prefer absolute path for shell tools
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}

	exists := fileExists(path)
	dir := path
	if exists {
		dir = filepath.Dir(path)
	} else if st, err := os.Stat(path); err == nil && st.IsDir() {
		dir = path
		exists = false
	} else {
		// open parent if file missing
		dir = filepath.Dir(path)
	}

	switch goruntime.GOOS {
	case "darwin":
		if exists {
			return exec.Command("open", "-R", path).Start()
		}
		return exec.Command("open", dir).Start()
	case "windows":
		// explorer /select,"C:\path\to\file" — arg must be a single token
		if exists {
			// Use cmd to avoid quoting issues with spaces in paths
			return exec.Command("cmd", "/c", "explorer", "/select,"+path).Start()
		}
		return exec.Command("explorer", dir).Start()
	default:
		if exists {
			// try to open parent; xdg-open doesn't have reveal
			return exec.Command("xdg-open", dir).Start()
		}
		return exec.Command("xdg-open", dir).Start()
	}
}

// ReadConfigText returns raw config file content for preview.
func (a *App) ReadConfigText(path string) (string, error) {
	path = expandPath(strings.TrimSpace(path))
	if !fileExists(path) {
		return "", fmt.Errorf("文件不存在: %s", path)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	// cap preview size
	const max = 200_000
	if len(b) > max {
		return string(b[:max]) + "\n…(截断)", nil
	}
	return string(b), nil
}
