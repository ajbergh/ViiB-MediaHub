//go:build linux

// Defines icons linux functionality for package main.

package main

import _ "embed"

// trayIconData embeds the Linux system tray icon (PNG format).
// systray on Linux uses PNG via libappindicator/StatusNotifierItem.
//
//go:embed build/appicon.png
var trayIconData []byte
