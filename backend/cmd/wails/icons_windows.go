//go:build windows

package main

import _ "embed"

// trayIconData embeds the Windows system tray icon (ICO format).
//
//go:embed build/windows/icon.ico
var trayIconData []byte
