package main

import (
	"embed"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	app := NewApp()

	// Window sizes describe the client content area. Frontend CSS fills 100% of
	// that area (not screen 100vh) so layout matches init size and resizes cleanly.
	err := wails.Run(&options.App{
		Title:            "AI Switch",
		Width:            1180,
		Height:           780,
		MinWidth:         640,
		MinHeight:        420,
		DisableResize:    false,
		Fullscreen:       false,
		WindowStartState: options.Normal,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 15, G: 20, B: 25, A: 1},
		OnStartup:        app.startup,
		OnShutdown:       app.shutdown,
		Bind: []interface{}{
			app,
		},
		Windows: &windows.Options{
			WebviewIsTransparent: false,
			WindowIsTranslucent:  false,
			DisableWindowIcon:   false,
			Theme:               windows.SystemDefault,
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}
