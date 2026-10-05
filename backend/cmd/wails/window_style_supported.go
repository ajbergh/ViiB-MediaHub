//go:build !darwin

// Defines window style supported functionality for package main.

package main

// Windows and Linux use the app's custom draggable title bar.
func useFramelessWindow() bool { return true }
