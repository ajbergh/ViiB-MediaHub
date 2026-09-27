//go:build !darwin

package main

// Windows and Linux use the app's custom draggable title bar.
func useFramelessWindow() bool { return true }
