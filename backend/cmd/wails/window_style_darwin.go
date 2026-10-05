//go:build darwin

// Defines window style darwin functionality for package main.

package main

// macOS uses its native title bar and traffic-light controls.
func useFramelessWindow() bool { return false }
