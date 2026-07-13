package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

var appLogMu sync.Mutex

func appLogPath() string {
	return filepath.Join(managerRoot(), "app.log")
}

func (a *App) appLogf(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	if a != nil && a.proxy != nil {
		a.proxy.logf("[app] %s", msg)
	}

	line := time.Now().Format("2006-01-02 15:04:05") + " " + msg + "\n"
	appLogMu.Lock()
	defer appLogMu.Unlock()
	path := appLogPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.WriteString(line)
}

func shortHash(hash string) string {
	if len(hash) <= 12 {
		return hash
	}
	return hash[:12]
}

func (a *App) GetAppLogs() (string, error) {
	f, err := os.Open(appLogPath())
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return "", err
	}
	const maxBytes int64 = 64 * 1024
	if info.Size() > maxBytes {
		if _, err := f.Seek(-maxBytes, io.SeekEnd); err != nil {
			return "", err
		}
	}
	b, err := io.ReadAll(f)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func (a *App) AppDebugLog(message string) {
	a.appLogf("ui %s", message)
}
