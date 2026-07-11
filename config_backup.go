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
	Kind         string `json:"kind"`
	OriginalPath string `json:"originalPath"`
	BackupPath   string `json:"backupPath"`
	BackedUpAt   string `json:"backedUpAt"`
	SHA256       string `json:"sha256"`
	Bytes        int    `json:"bytes"`
	Auto         bool   `json:"auto"` // true if created automatically before first edit
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

func defaultBackupFile(kind ToolKind, originalPath string) string {
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

func defaultBackupMetaFile(kind ToolKind) string {
	return filepath.Join(backupDir(kind), "default.meta.json")
}

func hashBytes(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func loadBackupMeta(kind ToolKind) (backupMeta, bool) {
	var m backupMeta
	b, err := os.ReadFile(defaultBackupMetaFile(kind))
	if err != nil {
		return m, false
	}
	if err := json.Unmarshal(b, &m); err != nil {
		return m, false
	}
	if m.BackupPath == "" && m.OriginalPath != "" {
		m.BackupPath = defaultBackupFile(kind, m.OriginalPath)
	}
	if m.BackupPath == "" || !fileExists(m.BackupPath) {
		return m, false
	}
	return m, true
}

func saveBackupMeta(m backupMeta) error {
	metaPath := defaultBackupMetaFile(ToolKind(m.Kind))
	if err := os.MkdirAll(filepath.Dir(metaPath), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(metaPath, b, 0o644)
}

// fillBackupInfo attaches default-backup status onto ToolConfigStatus.
func fillBackupInfo(st *ToolConfigStatus) {
	k := ToolKind(st.Kind)
	if m, ok := loadBackupMeta(k); ok {
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
}

// ensureDefaultBackup saves the current config as the baseline default
// if no default backup exists yet. Does nothing if already backed up.
// Returns true if a new backup was created.
func ensureDefaultBackup(kind ToolKind, path string) (created bool, err error) {
	path = expandPath(path)
	if path == "" || !fileExists(path) {
		return false, nil
	}
	if _, ok := loadBackupMeta(kind); ok {
		return false, nil
	}
	if err := saveDefaultBackup(kind, path, true); err != nil {
		return false, err
	}
	return true, nil
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
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	bak := defaultBackupFile(kind, path)
	if err := os.WriteFile(bak, content, 0o644); err != nil {
		return err
	}

	// timestamped history copy (best-effort)
	ts := time.Now().Format("20060102-150405")
	hist := filepath.Join(dir, fmt.Sprintf("snapshot-%s%s", ts, filepath.Ext(bak)))
	_ = os.WriteFile(hist, content, 0o644)

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
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	ext := filepath.Ext(path)
	if ext == "" {
		ext = ".bak"
	}
	_ = os.WriteFile(filepath.Join(dir, "pre-edit.latest"+ext), content, 0o644)
	ts := time.Now().Format("20060102-150405")
	_ = os.WriteFile(filepath.Join(dir, "pre-edit-"+ts+ext), content, 0o644)
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
	k := ToolKind(strings.ToLower(strings.TrimSpace(kind)))
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

// RestoreDefaultConfig restores the file from the default backup snapshot.
func (a *App) RestoreDefaultConfig(kind string) (ToolConfigStatus, error) {
	k := ToolKind(strings.ToLower(strings.TrimSpace(kind)))
	if !isKnownToolKind(k) {
		return ToolConfigStatus{}, fmt.Errorf("未知工具类型: %s", kind)
	}
	meta, ok := loadBackupMeta(k)
	if !ok {
		return ToolConfigStatus{}, fmt.Errorf("没有可还原的默认备份，请先备份或修改一次以自动生成")
	}
	content, err := os.ReadFile(meta.BackupPath)
	if err != nil {
		return ToolConfigStatus{}, fmt.Errorf("读取备份失败: %w", err)
	}

	st := a.resolveTool(k)
	target := st.Path
	if target == "" {
		target = meta.OriginalPath
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

	if err := writeFileAtomic(target, string(content)); err != nil {
		return ToolConfigStatus{}, err
	}

	_, _ = a.SetToolConfigPath(string(k), target)
	st = a.resolveTool(k)
	st.Message = "已还原为默认配置"
	return st, nil
}

// ClearDefaultBackup removes the default backup (does not touch the live config).
func (a *App) ClearDefaultBackup(kind string) (ToolConfigStatus, error) {
	k := ToolKind(strings.ToLower(strings.TrimSpace(kind)))
	if !isKnownToolKind(k) {
		return ToolConfigStatus{}, fmt.Errorf("未知工具类型: %s", kind)
	}
	if m, ok := loadBackupMeta(k); ok && m.BackupPath != "" {
		_ = os.Remove(m.BackupPath)
	}
	_ = os.Remove(defaultBackupMetaFile(k))
	st := a.resolveTool(k)
	st.Message = "已清除默认备份"
	return st, nil
}
