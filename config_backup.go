package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// backupMeta describes the saved default (baseline) config snapshot.
type backupMeta struct {
	Kind          string `json:"kind"`
	OriginalPath  string `json:"originalPath"`
	BackupPath    string `json:"backupPath"`
	BackedUpAt    string `json:"backedUpAt"`
	SHA256        string `json:"sha256"`
	Bytes         int    `json:"bytes"`
	Auto          bool   `json:"auto"`              // true if created automatically before first edit
	Missing       bool   `json:"missing,omitempty"` // true when original config did not exist
	EnvBackupPath string `json:"envBackupPath,omitempty"`
}

type environmentBackup struct {
	SecretsExists          bool            `json:"secretsExists"`
	Secrets                []byte          `json:"secrets,omitempty"`
	ProvidersEnvExists     bool            `json:"providersEnvExists"`
	ProvidersEnv           []byte          `json:"providersEnv,omitempty"`
	ProfileHadManagerBlock map[string]bool `json:"profileHadManagerBlock,omitempty"`
}

type takeoverMeta struct {
	Kind          string `json:"kind"`
	OriginalPath  string `json:"originalPath"`
	BackupPath    string `json:"backupPath"`
	BackedUpAt    string `json:"backedUpAt"`
	SHA256        string `json:"sha256"`
	Bytes         int    `json:"bytes"`
	Missing       bool   `json:"missing,omitempty"`
	EnvBackupPath string `json:"envBackupPath,omitempty"`
}

func managerRoot() string {
	home, _ := os.UserHomeDir()
	if home == "" {
		return ".codex-manager"
	}
	return filepath.Join(home, ".codex-manager")
}

func backupDir(kind ToolKind) string {
	return filepath.Join(managerRoot(), "backups", string(kind))
}

func backupPathKey(originalPath string) string {
	sum := sha256.Sum256([]byte(expandPath(originalPath)))
	return hex.EncodeToString(sum[:])[:16]
}

func defaultBackupFile(kind ToolKind, originalPath string) string {
	ext := filepath.Ext(originalPath)
	if ext == "" {
		if kind == ToolCodex {
			ext = ".toml"
		} else {
			ext = ".json"
		}
	}
	return filepath.Join(backupDir(kind), "default-"+backupPathKey(originalPath)+ext)
}

func legacyDefaultBackupFile(kind ToolKind, originalPath string) string {
	ext := filepath.Ext(originalPath)
	if ext == "" {
		if kind == ToolCodex {
			ext = ".toml"
		} else {
			ext = ".json"
		}
	}
	return filepath.Join(backupDir(kind), "default"+ext)
}

func defaultBackupMetaFile(kind ToolKind, originalPath string) string {
	return filepath.Join(backupDir(kind), "default-"+backupPathKey(originalPath)+".meta.json")
}

func takeoverBackupFile(kind ToolKind, originalPath string) string {
	ext := filepath.Ext(originalPath)
	if ext == "" {
		if kind == ToolCodex {
			ext = ".toml"
		} else {
			ext = ".json"
		}
	}
	return filepath.Join(backupDir(kind), "takeover-"+backupPathKey(originalPath)+ext)
}

func takeoverMetaFile(kind ToolKind, originalPath string) string {
	return filepath.Join(backupDir(kind), "takeover-"+backupPathKey(originalPath)+".meta.json")
}

func takeoverEnvironmentBackupPath(kind ToolKind, originalPath string) string {
	return filepath.Join(backupDir(kind), "takeover-"+backupPathKey(originalPath)+".env.json")
}

func saveTakeoverBackup(kind ToolKind, originalPath string) error {
	originalPath = expandPath(originalPath)
	if originalPath == "" {
		return fmt.Errorf("接管备份路径为空")
	}
	if fileExists(originalPath) && isManagedConfigFile(kind, originalPath) {
		if _, ok := loadTakeoverMeta(kind, originalPath); ok {
			return nil
		}
	}
	if err := os.MkdirAll(backupDir(kind), 0o700); err != nil {
		return err
	}
	meta := takeoverMeta{
		Kind:         string(kind),
		OriginalPath: originalPath,
		BackupPath:   takeoverBackupFile(kind, originalPath),
		BackedUpAt:   time.Now().Format(time.RFC3339),
	}
	var content []byte
	if raw, err := os.ReadFile(originalPath); err == nil {
		content = raw
	} else if os.IsNotExist(err) {
		meta.Missing = true
	} else {
		return err
	}
	meta.SHA256 = hashBytes(content)
	meta.Bytes = len(content)
	if err := os.WriteFile(meta.BackupPath, content, 0o600); err != nil {
		return err
	}
	meta.EnvBackupPath = takeoverEnvironmentBackupPath(kind, originalPath)
	if err := saveEnvironmentBackupTo(meta.EnvBackupPath); err != nil {
		return err
	}
	b, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(takeoverMetaFile(kind, originalPath), b, 0o600)
}

func loadTakeoverMeta(kind ToolKind, originalPath string) (takeoverMeta, bool) {
	originalPath = expandPath(originalPath)
	b, err := os.ReadFile(takeoverMetaFile(kind, originalPath))
	if err != nil {
		return takeoverMeta{}, false
	}
	var meta takeoverMeta
	if json.Unmarshal(b, &meta) != nil || meta.BackupPath == "" || !fileExists(meta.BackupPath) {
		return takeoverMeta{}, false
	}
	return meta, true
}

func findTakeoverMeta(kind ToolKind, originalPath string) (takeoverMeta, bool) {
	if meta, ok := loadTakeoverMeta(kind, originalPath); ok {
		return meta, true
	}
	entries, err := filepath.Glob(filepath.Join(backupDir(kind), "takeover-*.meta.json"))
	if err != nil {
		return takeoverMeta{}, false
	}
	var newest takeoverMeta
	var newestAt time.Time
	for _, path := range entries {
		b, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var meta takeoverMeta
		if json.Unmarshal(b, &meta) != nil || meta.Kind != string(kind) || meta.BackupPath == "" || !fileExists(meta.BackupPath) {
			continue
		}
		at, _ := time.Parse(time.RFC3339, meta.BackedUpAt)
		if newest.BackupPath == "" || at.After(newestAt) {
			newest = meta
			newestAt = at
		}
	}
	if newest.BackupPath == "" {
		return takeoverMeta{}, false
	}
	return newest, true
}

func driverIDForKind(kind ToolKind) string {
	if kind == ToolCodex {
		return "chatgpt"
	}
	return string(kind)
}

func isManagedConfigFile(kind ToolKind, path string) bool {
	if d := driverByID(driverIDForKind(kind)); d != nil {
		return d.IsManaged(path)
	}
	return false
}

func isManagedConfigContent(kind ToolKind, path string, content []byte) bool {
	tmp, err := os.CreateTemp("", "aigateway-managed-*"+filepath.Ext(path))
	if err != nil {
		return false
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := tmp.Write(content); err != nil {
		_ = tmp.Close()
		return false
	}
	_ = tmp.Close()
	return isManagedConfigFile(kind, tmpPath)
}

func legacyDefaultBackupMetaFile(kind ToolKind) string {
	return filepath.Join(backupDir(kind), "default.meta.json")
}

func hashBytes(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func loadBackupMeta(kind ToolKind, originalPath string) (backupMeta, bool) {
	var m backupMeta
	originalPath = expandPath(originalPath)
	b, err := os.ReadFile(defaultBackupMetaFile(kind, originalPath))
	legacy := false
	if err != nil {
		b, err = os.ReadFile(legacyDefaultBackupMetaFile(kind))
		legacy = err == nil
	}
	if err != nil {
		return m, false
	}
	if err := json.Unmarshal(b, &m); err != nil {
		return m, false
	}
	if m.BackupPath == "" && m.OriginalPath != "" {
		if legacy {
			m.BackupPath = legacyDefaultBackupFile(kind, m.OriginalPath)
		} else {
			m.BackupPath = defaultBackupFile(kind, m.OriginalPath)
		}
	}
	if originalPath != "" && expandPath(m.OriginalPath) != originalPath {
		return backupMeta{}, false
	}
	if m.BackupPath == "" || !fileExists(m.BackupPath) {
		return m, false
	}
	return m, true
}

// findBackupMeta is a compatibility fallback for older path overrides and
// legacy backups whose metadata path does not match the currently discovered
// config path exactly.
func findBackupMeta(kind ToolKind, originalPath string) (backupMeta, bool) {
	if meta, ok := loadBackupMeta(kind, originalPath); ok {
		return meta, true
	}
	entries, err := filepath.Glob(filepath.Join(backupDir(kind), "*.meta.json"))
	if err != nil {
		return backupMeta{}, false
	}
	var candidate backupMeta
	found := 0
	for _, path := range entries {
		b, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var meta backupMeta
		if json.Unmarshal(b, &meta) != nil || meta.Kind != string(kind) || meta.BackupPath == "" || !fileExists(meta.BackupPath) {
			continue
		}
		if meta.OriginalPath == "" {
			meta.OriginalPath = originalPath
		}
		candidate = meta
		found++
	}
	if found == 1 {
		return candidate, true
	}
	return backupMeta{}, false
}

func saveBackupMeta(m backupMeta) error {
	metaPath := defaultBackupMetaFile(ToolKind(m.Kind), m.OriginalPath)
	if err := os.MkdirAll(filepath.Dir(metaPath), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(metaPath, b, 0o600)
}

func environmentBackupPath(kind ToolKind, originalPath string) string {
	return filepath.Join(backupDir(kind), "default-"+backupPathKey(originalPath)+".env.json")
}

func managerEnvProfilePaths() []string {
	home, _ := os.UserHomeDir()
	if home == "" {
		return nil
	}
	return []string{filepath.Join(home, ".zprofile"), filepath.Join(home, ".zshrc"), filepath.Join(home, ".bash_profile"), filepath.Join(home, ".profile")}
}

func saveEnvironmentBackup(kind ToolKind, originalPath string) error {
	return saveEnvironmentBackupTo(environmentBackupPath(kind, originalPath))
}

func saveEnvironmentBackupTo(path string) error {
	b := environmentBackup{}
	secretsPath := filepath.Join(managerRoot(), "env", "secrets.json")
	if raw, err := os.ReadFile(secretsPath); err == nil {
		b.SecretsExists, b.Secrets = true, raw
	}
	providersPath := filepath.Join(managerRoot(), "env", "providers.env")
	if raw, err := os.ReadFile(providersPath); err == nil {
		b.ProvidersEnvExists, b.ProvidersEnv = true, raw
	}
	b.ProfileHadManagerBlock = map[string]bool{}
	marker := "# >>> codex-manager env >>>"
	for _, path := range managerEnvProfilePaths() {
		raw, err := os.ReadFile(path)
		b.ProfileHadManagerBlock[path] = err == nil && strings.Contains(string(raw), marker)
	}
	raw, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o600)
}

func ensureEnvironmentBackup(kind ToolKind, originalPath string, meta *backupMeta) error {
	if meta == nil || meta.EnvBackupPath != "" {
		return nil
	}
	if err := saveEnvironmentBackup(kind, originalPath); err != nil {
		return err
	}
	meta.EnvBackupPath = environmentBackupPath(kind, originalPath)
	return saveBackupMeta(*meta)
}

// fillBackupInfo attaches default-backup status onto ToolConfigStatus.
func fillBackupInfo(st *ToolConfigStatus) {
	k := ToolKind(st.Kind)
	if m, ok := loadBackupMeta(k, st.Path); ok {
		st.HasDefaultBackup = true
		st.DefaultBackupPath = m.BackupPath
		st.DefaultBackupAt = m.BackedUpAt
		st.DefaultBackupOrigin = m.OriginalPath
	} else {
		st.HasDefaultBackup = false
		st.DefaultBackupPath = ""
		st.DefaultBackupAt = ""
		st.DefaultBackupOrigin = ""
	}
	if m, ok := findTakeoverMeta(k, st.Path); ok {
		st.HasTakeoverBackup = true
		st.TakeoverBackupPath = m.BackupPath
		st.TakeoverBackupAt = m.BackedUpAt
		st.TakeoverBackupOrigin = m.OriginalPath
	} else {
		st.HasTakeoverBackup = false
		st.TakeoverBackupPath = ""
		st.TakeoverBackupAt = ""
		st.TakeoverBackupOrigin = ""
	}
}

// ensureDefaultBackup saves the current config as the baseline default
// if no default backup exists yet. Does nothing if already backed up.
// Returns true if a new backup was created.
func ensureDefaultBackup(kind ToolKind, path string) (created bool, err error) {
	path = expandPath(path)
	if path == "" {
		return false, nil
	}
	if meta, ok := loadBackupMeta(kind, path); ok {
		if err := ensureEnvironmentBackup(kind, path, &meta); err != nil {
			return false, err
		}
		return false, nil
	}
	if !fileExists(path) {
		if err := saveMissingDefaultBackup(kind, path); err != nil {
			return false, err
		}
		meta, _ := loadBackupMeta(kind, path)
		if err := ensureEnvironmentBackup(kind, path, &meta); err != nil {
			return false, err
		}
		return true, nil
	}
	if err := saveDefaultBackup(kind, path, true); err != nil {
		return false, err
	}
	meta, _ := loadBackupMeta(kind, path)
	if err := ensureEnvironmentBackup(kind, path, &meta); err != nil {
		return false, err
	}
	return true, nil
}

// saveMissingDefaultBackup records that the original config file did not exist.
// Restore uses this marker to remove the takeover-created file.
func saveMissingDefaultBackup(kind ToolKind, path string) error {
	path = expandPath(path)
	if path == "" {
		return fmt.Errorf("路径为空")
	}
	dir := backupDir(kind)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	bak := defaultBackupFile(kind, path)
	if err := os.WriteFile(bak, nil, 0o600); err != nil {
		return err
	}
	meta := backupMeta{
		Kind:         string(kind),
		OriginalPath: path,
		BackupPath:   bak,
		BackedUpAt:   time.Now().Format(time.RFC3339),
		SHA256:       hashBytes(nil),
		Bytes:        0,
		Auto:         true,
		Missing:      true,
	}
	return saveBackupMeta(meta)
}

// saveDefaultBackup overwrites the default baseline snapshot from the current file.
func saveDefaultBackup(kind ToolKind, path string, auto bool) error {
	path = expandPath(path)
	if path == "" {
		return fmt.Errorf("路径为空")
	}
	if !fileExists(path) {
		return fmt.Errorf("配置文件不存在: %s", path)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	dir := backupDir(kind)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}

	bak := defaultBackupFile(kind, path)
	if err := os.WriteFile(bak, content, 0o600); err != nil {
		return err
	}

	// timestamped history copy (best-effort)
	ts := time.Now().Format("20060102-150405")
	hist := filepath.Join(dir, fmt.Sprintf("snapshot-%s%s", ts, filepath.Ext(bak)))
	_ = os.WriteFile(hist, content, 0o600)

	meta := backupMeta{
		Kind:         string(kind),
		OriginalPath: path,
		BackupPath:   bak,
		BackedUpAt:   time.Now().Format(time.RFC3339),
		SHA256:       hashBytes(content),
		Bytes:        len(content),
		Auto:         auto,
	}
	return saveBackupMeta(meta)
}

// savePreWriteSnapshot stores a rolling pre-edit copy (not the "default").
func savePreWriteSnapshot(kind ToolKind, path string, content []byte) {
	if len(content) == 0 {
		return
	}
	dir := filepath.Join(backupDir(kind), "history")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return
	}
	ext := filepath.Ext(path)
	if ext == "" {
		ext = ".bak"
	}
	_ = os.WriteFile(filepath.Join(dir, "pre-edit.latest"+ext), content, 0o600)
	ts := time.Now().Format("20060102-150405")
	_ = os.WriteFile(filepath.Join(dir, "pre-edit-"+ts+ext), content, 0o600)
}

func isKnownToolKind(k ToolKind) bool {
	switch k {
	case ToolCodex, ToolClaude, ToolOpenClaw, ToolHarness:
		return true
	default:
		return false
	}
}

// BackupDefaultConfig manually (re)saves current config as the restorable default.
func (a *App) BackupDefaultConfig(kind, path string) (ToolConfigStatus, error) {
	k := normalizeToolKind(kind)
	if !isKnownToolKind(k) {
		return ToolConfigStatus{}, fmt.Errorf("未知工具类型: %s", kind)
	}
	path = expandPath(strings.TrimSpace(path))
	if path == "" {
		st := a.resolveTool(k)
		path = st.Path
	}
	if err := saveDefaultBackup(k, path, false); err != nil {
		return ToolConfigStatus{}, err
	}
	st := a.resolveTool(k)
	st.Message = "已备份为默认配置"
	return st, nil
}

func (a *App) restoreTakeoverConfig(kind ToolKind) (ToolConfigStatus, error) {
	st := a.resolveTool(kind)
	a.appLogf("rollback resolve kind=%q path=%q found=%v exists=%v managed=%v hasTakeover=%v takeoverOrigin=%q", kind, st.Path, st.Found, st.Exists, st.Managed, st.HasTakeoverBackup, st.TakeoverBackupOrigin)
	meta, ok := findTakeoverMeta(kind, st.Path)
	if !ok {
		a.appLogf("rollback takeover meta not found kind=%q currentPath=%q backupDir=%q", kind, st.Path, backupDir(kind))
		return ToolConfigStatus{}, fmt.Errorf("没有找到本次接管前备份，请先执行接管")
	}
	target := expandPath(meta.OriginalPath)
	if target == "" {
		a.appLogf("rollback target empty kind=%q metaOriginal=%q", kind, meta.OriginalPath)
		return ToolConfigStatus{}, fmt.Errorf("无法确定接管还原目标路径")
	}
	if info, err := os.Stat(target); err == nil {
		a.appLogf("rollback target stat path=%q mode=%s size=%d", target, info.Mode().String(), info.Size())
	} else {
		a.appLogf("rollback target stat path=%q err=%v", target, err)
	}
	if info, err := os.Stat(meta.BackupPath); err == nil {
		a.appLogf("rollback backup selected path=%q mode=%s size=%d sha=%s missing=%v env=%q backedUpAt=%q", meta.BackupPath, info.Mode().String(), info.Size(), shortHash(meta.SHA256), meta.Missing, meta.EnvBackupPath, meta.BackedUpAt)
	} else {
		a.appLogf("rollback backup stat path=%q err=%v", meta.BackupPath, err)
	}
	if meta.Missing {
		a.appLogf("rollback original was missing, removing target=%q", target)
		if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
			return ToolConfigStatus{}, fmt.Errorf("删除接管创建的配置失败: %w", err)
		}
		if err := restoreEnvironmentBackupPath(meta.EnvBackupPath); err != nil {
			a.appLogf("rollback env restore failed path=%q err=%v", meta.EnvBackupPath, err)
			return ToolConfigStatus{}, err
		}
		a.appLogf("rollback env restore ok path=%q", meta.EnvBackupPath)
		if _, err := a.ClearToolConfigPath(string(kind)); err != nil {
			a.appLogf("rollback clear path override failed kind=%q err=%v", kind, err)
		}
		st = a.resolveTool(kind)
		st.Message = "已解除接管，已还原接管前状态（原配置不存在，已删除接管创建的文件）"
		return st, nil
	} else {
		content, err := os.ReadFile(meta.BackupPath)
		if err != nil {
			a.appLogf("rollback backup read failed path=%q err=%v", meta.BackupPath, err)
			return ToolConfigStatus{}, fmt.Errorf("读取接管前备份失败: %w", err)
		}
		if isManagedConfigContent(kind, meta.BackupPath, content) {
			a.appLogf("rollback takeover backup is managed, trying default fallback kind=%q takeover=%q", kind, meta.BackupPath)
			if fallback, ok := loadBackupMeta(kind, target); ok && fallback.BackupPath != "" {
				if fallbackContent, err := os.ReadFile(fallback.BackupPath); err == nil && !isManagedConfigContent(kind, fallback.BackupPath, fallbackContent) {
					content = fallbackContent
					meta.BackupPath = fallback.BackupPath
					meta.SHA256 = hashBytes(fallbackContent)
					meta.Bytes = len(fallbackContent)
					a.appLogf("rollback using default fallback backup path=%q sha=%s bytes=%d", fallback.BackupPath, shortHash(meta.SHA256), meta.Bytes)
				} else if err != nil {
					a.appLogf("rollback default fallback read failed path=%q err=%v", fallback.BackupPath, err)
				} else {
					a.appLogf("rollback default fallback is also managed path=%q", fallback.BackupPath)
				}
			} else {
				a.appLogf("rollback default fallback not found kind=%q target=%q", kind, target)
			}
		}
		beforeHash := ""
		beforeBytes := 0
		if before, err := os.ReadFile(target); err == nil {
			beforeHash = hashBytes(before)
			beforeBytes = len(before)
		}
		a.appLogf("rollback write start target=%q backup=%q beforeSha=%s beforeBytes=%d backupSha=%s backupBytes=%d", target, meta.BackupPath, shortHash(beforeHash), beforeBytes, shortHash(hashBytes(content)), len(content))
		if err := writeFileAtomic(target, string(content)); err != nil {
			a.appLogf("rollback write failed target=%q err=%v", target, err)
			return ToolConfigStatus{}, err
		}
		if restored, err := os.ReadFile(target); err != nil || hashBytes(restored) != meta.SHA256 {
			if err != nil {
				a.appLogf("rollback verify read failed target=%q err=%v", target, err)
				return ToolConfigStatus{}, fmt.Errorf("接管还原后校验失败: %w", err)
			}
			a.appLogf("rollback verify mismatch target=%q gotSha=%s wantSha=%s gotBytes=%d", target, shortHash(hashBytes(restored)), shortHash(meta.SHA256), len(restored))
			return ToolConfigStatus{}, fmt.Errorf("接管还原后校验失败: 文件内容与接管前备份不一致")
		}
		a.appLogf("rollback write verified target=%q sha=%s bytes=%d", target, shortHash(meta.SHA256), len(content))
	}
	if err := restoreEnvironmentBackupPath(meta.EnvBackupPath); err != nil {
		a.appLogf("rollback env restore failed path=%q err=%v", meta.EnvBackupPath, err)
		return ToolConfigStatus{}, err
	}
	a.appLogf("rollback env restore ok path=%q", meta.EnvBackupPath)
	if _, err := a.SetToolConfigPath(string(kind), target); err != nil {
		a.appLogf("rollback save path override failed kind=%q target=%q err=%v", kind, target, err)
	}
	st = a.resolveTool(kind)
	st.Message = "已解除接管，已还原接管前配置"
	return st, nil
}

// RestoreDefaultConfig restores the file from the default backup snapshot.
func (a *App) RestoreDefaultConfig(kind string) (ToolConfigStatus, error) {
	k := normalizeToolKind(kind)
	if !isKnownToolKind(k) {
		return ToolConfigStatus{}, fmt.Errorf("未知工具类型: %s", kind)
	}
	st := a.resolveTool(k)
	meta, ok := findBackupMeta(k, st.Path)
	if !ok {
		return ToolConfigStatus{}, fmt.Errorf("没有可还原的默认备份，请先备份或修改一次以自动生成")
	}
	content, err := os.ReadFile(meta.BackupPath)
	if err != nil {
		return ToolConfigStatus{}, fmt.Errorf("读取备份失败: %w", err)
	}

	target := expandPath(meta.OriginalPath)
	if target == "" {
		target = st.Path
	}
	target = expandPath(target)
	if target == "" {
		return ToolConfigStatus{}, fmt.Errorf("无法确定还原目标路径")
	}

	// safety: snapshot current before restore
	if fileExists(target) {
		if cur, err := os.ReadFile(target); err == nil {
			savePreWriteSnapshot(k, target, cur)
		}
	}

	if meta.Missing {
		if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
			return ToolConfigStatus{}, fmt.Errorf("删除接管创建的配置失败: %w", err)
		}
		if err := restoreEnvironmentBackup(meta); err != nil {
			return ToolConfigStatus{}, err
		}
		_, _ = a.ClearToolConfigPath(string(k))
		st = a.resolveTool(k)
		st.Message = "已还原为接管前状态（原配置不存在，已删除接管创建的文件）"
		return st, nil
	}

	if err := writeFileAtomic(target, string(content)); err != nil {
		return ToolConfigStatus{}, err
	}
	if meta.SHA256 != "" {
		if restored, err := os.ReadFile(target); err != nil || hashBytes(restored) != meta.SHA256 {
			if err != nil {
				return ToolConfigStatus{}, fmt.Errorf("还原后校验失败: %w", err)
			}
			return ToolConfigStatus{}, fmt.Errorf("还原后校验失败: 文件内容与备份不一致")
		}
	}
	if err := restoreEnvironmentBackup(meta); err != nil {
		return ToolConfigStatus{}, err
	}

	_, _ = a.SetToolConfigPath(string(k), target)
	st = a.resolveTool(k)
	st.Message = "已还原为默认配置"
	return st, nil
}

// ClearDefaultBackup removes the default backup (does not touch the live config).
func (a *App) ClearDefaultBackup(kind string) (ToolConfigStatus, error) {
	k := normalizeToolKind(kind)
	if !isKnownToolKind(k) {
		return ToolConfigStatus{}, fmt.Errorf("未知工具类型: %s", kind)
	}
	st := a.resolveTool(k)
	if m, ok := loadBackupMeta(k, st.Path); ok && m.BackupPath != "" {
		_ = os.Remove(m.BackupPath)
	}
	_ = os.Remove(defaultBackupMetaFile(k, st.Path))
	st = a.resolveTool(k)
	st.Message = "已清除默认备份"
	return st, nil
}
