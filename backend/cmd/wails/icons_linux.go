//go:build linux

package main

import _ "embed"

// trayIconData embeds the Linux system tray icon (PNG format).
// systray on Linux uses PNG via libappindicator/StatusNotifierItem.
//
//go:embed build/appicon.png
var trayIconData []byte
