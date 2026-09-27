//go:build darwin

package main

// macOS uses its native title bar and traffic-light controls.
func useFramelessWindow() bool { return false }
