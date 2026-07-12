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

type ConfigSnapshot struct {
	ID        string `json:"id"`
	CreatedAt string `json:"createdAt"`
	Path      string `json:"path"`
	Backup    string `json:"backup"`
	Reason    string `json:"reason"`
	SHA256    string `json:"sha256"`
	Size      int    `json:"size"`
}

type ConfigWriteResult struct {
	Path      string          `json:"path"`
	Changed   bool            `json:"changed"`
	Snapshot  *ConfigSnapshot `json:"snapshot,omitempty"`
	Diff      string          `json:"diff,omitempty"`
	Message   string          `json:"message"`
	CreatedAt string          `json:"createdAt"`
}

func snapshotsRoot() string {
	return filepath.Join(managerRoot(), "snapshots")
}

func snapshotLedgerPath() string {
	return filepath.Join(snapshotsRoot(), "ledger.jsonl")
}

func createConfigSnapshot(path, reason string, content []byte) (*ConfigSnapshot, error) {
	if len(content) == 0 {
		return nil, nil
	}
	now := time.Now().UTC()
	sum := sha256.Sum256(content)
	id := now.Format("20060102T150405.000000000Z") + "-" + hex.EncodeToString(sum[:])[:12]
	backup := filepath.Join(snapshotsRoot(), id+"-"+slugify(filepath.Base(path))+".bak")
	if err := os.MkdirAll(filepath.Dir(backup), 0o755); err != nil {
		return nil, err
	}
	if err := writeFileAtomicMode(backup, string(content), 0o600); err != nil {
		return nil, err
	}
	s := &ConfigSnapshot{
		ID:        id,
		CreatedAt: now.Format(time.RFC3339Nano),
		Path:      path,
		Backup:    backup,
		Reason:    reason,
		SHA256:    hex.EncodeToString(sum[:]),
		Size:      len(content),
	}
	if err := appendSnapshotLedger(*s); err != nil {
		return nil, err
	}
	return s, nil
}

func appendSnapshotLedger(s ConfigSnapshot) error {
	if err := os.MkdirAll(filepath.Dir(snapshotLedgerPath()), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(snapshotLedgerPath(), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	b, err := json.Marshal(s)
	if err != nil {
		return err
	}
	if _, err := f.Write(append(b, '\n')); err != nil {
		return err
	}
	return f.Sync()
}

func writeConfigWithSnapshot(path, next, reason string) (ConfigWriteResult, error) {
	path = expandPath(path)
	result := ConfigWriteResult{Path: path, CreatedAt: time.Now().Format(time.RFC3339Nano)}
	raw, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return result, err
	}
	original := string(raw)
	if original == next {
		result.Message = "配置未变化"
		return result, nil
	}
	if len(raw) > 0 {
		s, err := createConfigSnapshot(path, reason, raw)
		if err != nil {
			return result, err
		}
		result.Snapshot = s
	}
	if err := writeFileAtomic(path, preserveLineEndings(original, next)); err != nil {
		return result, err
	}
	result.Changed = true
	result.Diff = unifiedTextDiff(original, next, 120)
	result.Message = "配置已安全写入"
	return result, nil
}

func previewConfigWrite(path, next string) (ConfigWriteResult, error) {
	path = expandPath(path)
	raw, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return ConfigWriteResult{Path: path}, err
	}
	original := string(raw)
	return ConfigWriteResult{
		Path:      path,
		Changed:   original != next,
		Diff:      unifiedTextDiff(original, next, 200),
		Message:   "dry-run",
		CreatedAt: time.Now().Format(time.RFC3339Nano),
	}, nil
}

func unifiedTextDiff(before, after string, maxLines int) string {
	if before == after {
		return ""
	}
	a := strings.Split(before, "\n")
	b := strings.Split(after, "\n")
	var out []string
	out = append(out, "--- before", "+++ after")
	i, j := 0, 0
	for i < len(a) || j < len(b) {
		if i < len(a) && j < len(b) && a[i] == b[j] {
			i++
			j++
			continue
		}
		if len(out) >= maxLines {
			out = append(out, fmt.Sprintf("... diff truncated after %d lines", maxLines))
			break
		}
		if i < len(a) {
			out = append(out, "-"+a[i])
			i++
		}
		if j < len(b) {
			out = append(out, "+"+b[j])
			j++
		}
	}
	return strings.Join(out, "\n")
}
