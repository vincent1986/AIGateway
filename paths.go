package main

import (
	"os"
	"path/filepath"
	"regexp"
	goruntime "runtime"
	"strings"
)

func managerRoot() string {
	home, _ := os.UserHomeDir()
	if home == "" {
		return ".codex-manager"
	}
	return filepath.Join(home, ".codex-manager")
}

func userHome() string {
	h, _ := os.UserHomeDir()
	return h
}

func expandPath(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return p
	}
	p = expandWindowsEnv(p)
	p = os.ExpandEnv(p)
	home := userHome()
	if p == "~" && home != "" {
		return home
	}
	if home != "" && strings.HasPrefix(p, "~") && len(p) >= 2 && (p[1] == '/' || p[1] == '\\') {
		rest := ""
		if len(p) > 2 {
			rest = normalizeSeparators(p[2:])
		}
		return filepath.Clean(filepath.Join(home, rest))
	}
	return filepath.Clean(normalizeSeparators(p))
}

var winEnvRe = regexp.MustCompile(`%([^%]+)%`)

func expandWindowsEnv(p string) string {
	return winEnvRe.ReplaceAllStringFunc(p, func(m string) string {
		name := m[1 : len(m)-1]
		if v := os.Getenv(name); v != "" {
			return v
		}
		if strings.EqualFold(name, "USERPROFILE") || strings.EqualFold(name, "HOME") {
			return userHome()
		}
		return m
	})
}

func normalizeSeparators(p string) string {
	p = strings.ReplaceAll(p, `\`, `/`)
	return filepath.FromSlash(p)
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

func writeFileAtomic(path, content string) error {
	mode := os.FileMode(0o644)
	if st, err := os.Stat(path); err == nil && st.Mode().Perm() != 0 {
		mode = st.Mode().Perm()
	}
	return writeFileAtomicMode(path, content, mode)
}

func writeFileAtomicMode(path, content string, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if _, err := tmp.Write([]byte(content)); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	if dirFile, err := os.Open(dir); err == nil {
		_ = dirFile.Sync()
		_ = dirFile.Close()
	}
	return nil
}

func preserveLineEndings(original, next string) string {
	if strings.Contains(original, "\r\n") {
		next = strings.ReplaceAll(next, "\r\n", "\n")
		next = strings.ReplaceAll(next, "\n", "\r\n")
	}
	return next
}

type SystemInfo struct {
	OS           string `json:"os"`
	Arch         string `json:"arch"`
	HomeDir      string `json:"homeDir"`
	PathSep      string `json:"pathSep"`
	FileManager  string `json:"fileManager"`
	RevealLabel  string `json:"revealLabel"`
	PlatformName string `json:"platformName"`
}

func (a *App) GetSystemInfo() SystemInfo {
	info := SystemInfo{OS: goruntime.GOOS, Arch: goruntime.GOARCH, HomeDir: userHome(), PathSep: string(os.PathSeparator)}
	switch goruntime.GOOS {
	case "darwin":
		info.FileManager, info.RevealLabel, info.PlatformName = "Finder", "在访达中显示", "macOS"
	case "windows":
		info.FileManager, info.RevealLabel, info.PlatformName = "Explorer", "在资源管理器中显示", "Windows"
	default:
		info.FileManager, info.RevealLabel, info.PlatformName = "Files", "在文件管理器中显示", "Linux"
	}
	return info
}

func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
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
