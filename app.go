package main

import "context"

type App struct {
	ctx   context.Context
	proxy *proxyServer
}

func NewApp() *App {
	return &App{proxy: newProxyServer()}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	if a.proxy != nil {
		cfg := a.proxy.getConfig()
		if cfg.AutoStart || cfg.Enabled {
			if err := a.proxy.start(); err != nil {
				a.proxy.logf("自动启动失败: %v", err)
			}
		}
	}
}

func (a *App) shutdown(ctx context.Context) {
	if a.proxy != nil {
		_ = a.proxy.stop()
	}
}
