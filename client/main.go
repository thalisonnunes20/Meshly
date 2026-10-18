package main

import (
	"context"
	_ "embed"
	"embed"

	"github.com/getlantern/systray"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

//go:embed all:frontend/src
var assets embed.FS

//go:embed icon.ico
var iconBytes []byte

func main() {
	app := NewApp()

	err := wails.Run(&options.App{
		Title:             "Meshly",
		Width:             460,
		Height:            720,
		MinWidth:          420,
		MinHeight:         600,
		HideWindowOnClose: true,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 9, G: 9, B: 11, A: 1},
		OnStartup: func(ctx context.Context) {
			app.startup(ctx)

			// Setup System Tray
			go func() {
				systray.Run(func() {
					systray.SetIcon(iconBytes)
					systray.SetTitle("Meshly")
					systray.SetTooltip("Meshly Virtual LAN")

					mShow := systray.AddMenuItem("Abrir Meshly", "Exibir janela principal")
					systray.AddSeparator()
					mQuit := systray.AddMenuItem("Sair", "Encerrar aplicativo")

					for {
						select {
						case <-mShow.ClickedCh:
							runtime.WindowShow(ctx)
							runtime.WindowUnminimise(ctx)
						case <-mQuit.ClickedCh:
							systray.Quit()
							runtime.Quit(ctx)
							return
						}
					}
				}, func() {})
			}()
		},
		OnBeforeClose: func(ctx context.Context) (prevent bool) {
			runtime.WindowHide(ctx)
			return true // Minimize / Hide to system tray instead of closing!
		},
		Bind: []interface{}{
			app,
		},
		Windows: &windows.Options{
			WebviewIsTransparent: false,
			WindowIsTranslucent:  false,
			BackdropType:         windows.Mica,
			Theme:                windows.Dark,
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}
