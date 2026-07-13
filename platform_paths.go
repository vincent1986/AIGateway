package main

import (
	"os"
	"path/filepath"
	"regexp"
	goruntime "runtime"
	"strings"
)

// SystemInfo exposes OS details to the frontend for labels/UX.
type SystemInfo struct {
	OS           string `json:"os"` // darwin | windows | linux
	Arch         string `json:"arch"`
	HomeDir      string `json:"homeDir"`
	PathSep      string `json:"pathSep"`
	FileManager  string `json:"fileManager"`  // Finder | Explorer | Files
	RevealLabel  string `json:"revealLabel"`  // 在访达中显示 / 在资源管理器中显示
	PlatformName string `json:"platformName"` // macOS / Windows / Linux
}

// GetSystemInfo returns current OS info for UI adaptation.
func (a *App) GetSystemInfo() SystemInfo {
	info := SystemInfo{
		OS:   goruntime.GOOS,
		Arch: goruntime.GOARCH,
	}
	home, _ := os.UserHomeDir()
	info.HomeDir = home
	info.PathSep = string(os.PathSeparator)

	switch goruntime.GOOS {
	case "darwin":
		info.FileManager = "Finder"
		info.RevealLabel = "在访达中显示"
		info.PlatformName = "macOS"
	case "windows":
		info.FileManager = "Explorer"
		info.RevealLabel = "在资源管理器中显示"
		info.PlatformName = "Windows"
	default:
		info.FileManager = "Files"
		info.RevealLabel = "在文件管理器中显示"
		info.PlatformName = "Linux"
	}
	return info
}

// expandPath expands ~, ~/ , %VAR%, $VAR and cleans the path for the current OS.
// Accepts both macOS/Linux (/) and Windows (\) separators in user input.
func expandPath(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return p
	}

	// Expand Windows-style and Unix-style env vars first
	p = expandWindowsEnv(p)
	p = os.ExpandEnv(p) // $VAR and ${VAR}

	home, err := os.UserHomeDir()
	if err != nil {
		home = ""
	}

	if p == "~" && home != "" {
		return home
	}
	// ~/foo or ~\foo
	if home != "" && strings.HasPrefix(p, "~") && len(p) >= 2 && (p[1] == '/' || p[1] == '\\') {
		rest := ""
		if len(p) > 2 {
			rest = normalizeSeparators(p[2:])
		}
		return filepath.Clean(filepath.Join(home, rest))
	}

	return filepath.Clean(normalizeSeparators(p))
}

// normalizeSeparators converts \ and / into the OS path separator safely.
func normalizeSeparators(p string) string {
	// Treat backslash as a path separator (Windows paths pasted on any OS)
	p = strings.ReplaceAll(p, `\`, `/`)
	return filepath.FromSlash(p)
}

var winEnvRe = regexp.MustCompile(`%([^%]+)%`)

func expandWindowsEnv(p string) string {
	return winEnvRe.ReplaceAllStringFunc(p, func(m string) string {
		name := m[1 : len(m)-1]
		if v := os.Getenv(name); v != "" {
			return v
		}
		// common aliases
		switch strings.ToUpper(name) {
		case "USERPROFILE", "HOME":
			if h, err := os.UserHomeDir(); err == nil && h != "" {
				return h
			}
		case "APPDATA":
			if v := os.Getenv("APPDATA"); v != "" {
				return v
			}
		case "LOCALAPPDATA":
			if v := os.Getenv("LOCALAPPDATA"); v != "" {
				return v
			}
		}
		return m
	})
}

func userHome() string {
	h, _ := os.UserHomeDir()
	return h
}

// preferredCodexConfigPath is the default create/search target for the current OS.
func preferredCodexConfigPath() string {
	if v := strings.TrimSpace(os.Getenv("CODEX_HOME")); v != "" {
		return filepath.Join(expandPath(v), "config.toml")
	}
	if home := userHome(); home != "" {
		return filepath.Join(home, ".codex", "config.toml")
	}
	return "config.toml"
}

func preferredClaudeConfigPath() string {
	if v := strings.TrimSpace(os.Getenv("CLAUDE_CONFIG_DIR")); v != "" {
		return filepath.Join(expandPath(v), "settings.json")
	}
	if home := userHome(); home != "" {
		return filepath.Join(home, ".claude", "settings.json")
	}
	return "settings.json"
}

func codexSearchPaths() []string {
	var paths []string
	home := userHome()

	// 1) CODEX_HOME override (all platforms)
	if v := strings.TrimSpace(os.Getenv("CODEX_HOME")); v != "" {
		paths = append(paths, filepath.Join(expandPath(v), "config.toml"))
	}

	// 2) Default user home: ~/.codex or %USERPROFILE%\.codex
	if home != "" {
		paths = append(paths, filepath.Join(home, ".codex", "config.toml"))
	}

	switch goruntime.GOOS {
	case "darwin":
		if home != "" {
			// Codex desktop / app support locations
			paths = append(paths,
				filepath.Join(home, "Library", "Application Support", "Codex", "config.toml"),
				filepath.Join(home, "Library", "Application Support", "com.openai.codex", "config.toml"),
			)
		}
		paths = append(paths, "/etc/codex/config.toml")
	case "windows":
		// Explicit USERPROFILE (same as home usually, but keep env-expanded form)
		if up := strings.TrimSpace(os.Getenv("USERPROFILE")); up != "" {
			paths = append(paths, filepath.Join(up, ".codex", "config.toml"))
		}
		// Roaming / Local AppData variants some installers use
		if app := strings.TrimSpace(os.Getenv("APPDATA")); app != "" {
			paths = append(paths,
				filepath.Join(app, "Codex", "config.toml"),
				filepath.Join(app, "openai", "codex", "config.toml"),
			)
		}
		if local := strings.TrimSpace(os.Getenv("LOCALAPPDATA")); local != "" {
			paths = append(paths,
				filepath.Join(local, "Codex", "config.toml"),
				filepath.Join(local, "openai", "codex", "config.toml"),
			)
		}
	default: // linux / other
		if home != "" {
			paths = append(paths,
				filepath.Join(home, ".config", "codex", "config.toml"),
			)
		}
		paths = append(paths, "/etc/codex/config.toml")
	}

	return uniquePaths(paths)
}

func claudeSearchPaths() []string {
	var paths []string
	home := userHome()

	// 1) CLAUDE_CONFIG_DIR override
	if v := strings.TrimSpace(os.Getenv("CLAUDE_CONFIG_DIR")); v != "" {
		base := expandPath(v)
		paths = append(paths,
			filepath.Join(base, "settings.json"),
		)
	}

	// 2) Standard user locations
	if home != "" {
		paths = append(paths,
			filepath.Join(home, ".claude", "settings.json"),
		)
	}

	switch goruntime.GOOS {
	case "darwin":
		if home != "" {
			paths = append(paths,
				filepath.Join(home, "Library", "Application Support", "Claude", "settings.json"),
				filepath.Join(home, "Library", "Application Support", "ClaudeCode", "settings.json"),
			)
		}
	case "windows":
		if up := strings.TrimSpace(os.Getenv("USERPROFILE")); up != "" {
			paths = append(paths,
				filepath.Join(up, ".claude", "settings.json"),
			)
		}
		if app := strings.TrimSpace(os.Getenv("APPDATA")); app != "" {
			paths = append(paths,
				filepath.Join(app, "Claude", "settings.json"),
				filepath.Join(app, "Claude Code", "settings.json"),
				filepath.Join(app, "ClaudeCode", "settings.json"),
			)
		}
		if local := strings.TrimSpace(os.Getenv("LOCALAPPDATA")); local != "" {
			paths = append(paths,
				filepath.Join(local, "Claude", "settings.json"),
				filepath.Join(local, "ClaudeCode", "settings.json"),
			)
		}
	default:
		if home != "" {
			paths = append(paths,
				filepath.Join(home, ".config", "claude", "settings.json"),
			)
		}
	}

	return uniquePaths(paths)
}

func uniquePaths(in []string) []string {
	// Windows paths are case-insensitive
	caseInsensitive := goruntime.GOOS == "windows"
	seen := map[string]bool{}
	var out []string
	for _, p := range in {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		p = expandPath(p)
		key := p
		if caseInsensitive {
			key = strings.ToLower(p)
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, p)
	}
	return out
}

// preserveLineEndings keeps CRLF if the original file used Windows newlines.
func preserveLineEndings(original, next string) string {
	if strings.Contains(original, "\r\n") {
		// normalize then convert
		next = strings.ReplaceAll(next, "\r\n", "\n")
		next = strings.ReplaceAll(next, "\n", "\r\n")
	}
	return next
}

// writeFileAtomic writes content preserving existing mode when possible.
// On Windows, permission bits are largely ignored but the call remains valid.
func writeFileAtomic(path, content string) error {
	mode := os.FileMode(0o644)
	if st, err := os.Stat(path); err == nil {
		// On Windows Mode().Perm() may be 0666; still fine
		if perm := st.Mode().Perm(); perm != 0 {
			mode = perm
		}
	}
	// ensure parent directory exists
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), mode)
}

func defaultDirForKind(kind ToolKind) string {
	home := userHome()
	switch kind {
	case ToolCodex:
		if v := strings.TrimSpace(os.Getenv("CODEX_HOME")); v != "" {
			return expandPath(v)
		}
		if home != "" {
			p := filepath.Join(home, ".codex")
			if st, err := os.Stat(p); err == nil && st.IsDir() {
				return p
			}
		}
	case ToolClaude:
		if v := strings.TrimSpace(os.Getenv("CLAUDE_CONFIG_DIR")); v != "" {
			return expandPath(v)
		}
		if home != "" {
			p := filepath.Join(home, ".claude")
			if st, err := os.Stat(p); err == nil && st.IsDir() {
				return p
			}
		}
	}
	return home
}
