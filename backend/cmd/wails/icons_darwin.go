//go:build darwin

// Defines icons darwin functionality for package main.

package main

import _ "embed"

// trayIconData embeds the macOS system tray icon (PNG format).
// systray on macOS renders a template image from PNG bytes.
//
//go:embed build/appicon.png
var trayIconData []byte
