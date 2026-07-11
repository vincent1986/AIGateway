package main

import (
	"context"
)

// App struct
type App struct {
	ctx   context.Context
	proxy *proxyServer
}

// NewApp creates a new App application struct
func NewApp() *App {
	return &App{
		proxy: newProxyServer(),
	}
}

// startup is called when the app starts. The context is saved
// so we can call the runtime methods
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	// auto-start OpenAI proxy if configured
	if a.proxy != nil {
		cfg := a.proxy.getConfig()
		if cfg.AutoStart || cfg.Enabled {
			if err := a.proxy.start(); err != nil {
				a.proxy.logf("自动启动失败: %v", err)
			}
		}
	}
}

// shutdown stops background services.
func (a *App) shutdown(ctx context.Context) {
	if a.proxy != nil {
		_ = a.proxy.stop()
	}
}
