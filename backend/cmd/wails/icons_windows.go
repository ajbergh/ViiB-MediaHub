//go:build windows

package main

import _ "embed"

// Use the same validated, multi-resolution ICO as the executable and installer.
//
//go:embed appicon.ico
var trayIconData []byte
