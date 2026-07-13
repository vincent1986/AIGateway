package main

import (
	"os"
	"testing"
	"time"
)

// TestDaemonProxy starts the local OpenAI proxy and blocks.
// Usage:
//
//	RUN_PROXY_DAEMON=1 go test -run TestDaemonProxy -timeout 0
//
// Stop with Ctrl+C or kill the process.
func TestDaemonProxy(t *testing.T) {
	if os.Getenv("RUN_PROXY_DAEMON") == "" {
		t.Skip("set RUN_PROXY_DAEMON=1 to start proxy daemon")
	}
	p := newProxyServer()
	cfg := p.getConfig()
	cfg.Enabled = true
	if cfg.Host == "" {
		cfg.Host = "127.0.0.1"
	}
	if cfg.Port <= 0 {
		cfg.Port = 18080
	}
	_ = p.setConfig(cfg)
	if err := p.start(); err != nil {
		t.Fatalf("start proxy: %v", err)
	}
	t.Logf("proxy listening on %s (Ctrl+C to stop)", p.baseURL())
	// Keep process alive until killed
	for {
		time.Sleep(time.Hour)
	}
}
