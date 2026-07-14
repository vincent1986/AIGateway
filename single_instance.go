package main

import (
	"github.com/wailsapp/wails/v2/pkg/options"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

var (
	windowShowFn       = wailsruntime.WindowShow
	windowUnminimiseFn = wailsruntime.WindowUnminimise
)

func (a *App) handleSecondInstanceLaunch(_ options.SecondInstanceData) {
	if a == nil || a.ctx == nil {
		return
	}
	windowUnminimiseFn(a.ctx)
	windowShowFn(a.ctx)
}
