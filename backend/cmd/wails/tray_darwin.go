//go:build darwin

package main

import "github.com/ajbergh/viib-mediahub/internal/logger"

func hideWindowOnClose() bool {
	return false
}

func startSystemTray(_ *App, _ chan struct{}) func() {
	logger.Main("Starting Wails with the standard macOS application lifecycle")
	return func() {}
}
