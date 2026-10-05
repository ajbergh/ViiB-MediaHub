//go:build !darwin

// Defines tray supported functionality for package main.

package main

import (
	"runtime"
	"sync"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/logger"
	"github.com/getlantern/systray"
)

func supportsSystemTray() bool {
	return true
}

func hideWindowOnClose(dataDir string) bool {
	return loadWindowCloseAction(dataDir) == closeActionHide
}

func startSystemTray(app *App, quitChan chan struct{}) func() {
	trayReady := make(chan struct{})
	trayStopped := make(chan struct{})
	done := make(chan struct{})
	var stopOnce sync.Once
	var quitOnce sync.Once
	requestQuit := func() { quitOnce.Do(func() { close(quitChan) }) }
	go func() {
		// The native window and message pump must stay on the same OS thread.
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		defer func() {
			close(trayStopped)
			select {
			case <-done:
			default:
				requestQuit()
			}
		}()
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
						requestQuit()
						return
					case <-done:
						return
					}
				}
			},
			func() {
				logger.Main("Systray exited")
				select {
				case <-done:
				default:
					requestQuit()
				}
			},
		)
	}()

	select {
	case <-trayReady:
		logger.Main("System tray ready, starting Wails...")
	case <-trayStopped:
		logger.Main("System tray stopped before initialization")
		requestQuit()
	case <-time.After(5 * time.Second):
		logger.Main("System tray initialization timed out; exiting")
		requestQuit()
	}
	return func() {
		stopOnce.Do(func() {
			close(done)
			systray.Quit()
		})
	}
}
