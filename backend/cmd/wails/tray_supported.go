//go:build !darwin

package main

import (
	"github.com/ajbergh/viib-mediahub/internal/logger"
	"github.com/getlantern/systray"
)

func hideWindowOnClose() bool {
	return true
}

func startSystemTray(app *App, quitChan chan struct{}) func() {
	trayReady := make(chan struct{})
	go func() {
		systray.Run(
			func() {
				systray.SetIcon(trayIconData)
				systray.SetTitle("ViiB MediaHub")
				systray.SetTooltip("ViiB MediaHub - Local Media Player")

				mShow := systray.AddMenuItem("Show ViiB MediaHub", "Show the application window")
				systray.AddSeparator()
				mQuit := systray.AddMenuItem("Quit", "Quit the application")

				logger.Main("System tray initialized")
				close(trayReady)

				for {
					select {
					case <-mShow.ClickedCh:
						logger.Main("Systray: Show clicked")
						app.ShowWindow()
					case <-mQuit.ClickedCh:
						logger.Main("Systray: Quit clicked")
						close(quitChan)
						return
					}
				}
			},
			func() {
				logger.Main("Systray exited")
			},
		)
	}()

	<-trayReady
	logger.Main("System tray ready, starting Wails...")
	return systray.Quit
}
