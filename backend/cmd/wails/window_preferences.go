package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type windowClosePreference struct {
	CloseAction string `json:"closeAction"`
}

const (
	closeActionHide = "hide"
	closeActionQuit = "quit"
)

func windowPreferencesPath(dataDir string) string {
	return filepath.Join(dataDir, "window-preferences.json")
}

func loadWindowCloseAction(dataDir string) string {
	data, err := os.ReadFile(windowPreferencesPath(dataDir))
	if err != nil {
		return closeActionHide
	}

	var preferences windowClosePreference
	if json.Unmarshal(data, &preferences) != nil || preferences.CloseAction != closeActionQuit {
		return closeActionHide
	}
	return closeActionQuit
}

func saveWindowCloseAction(dataDir, action string) error {
	if action != closeActionHide && action != closeActionQuit {
		return fmt.Errorf("unsupported window close action %q", action)
	}

	data, err := json.Marshal(windowClosePreference{CloseAction: action})
	if err != nil {
		return fmt.Errorf("encode window preferences: %w", err)
	}

	path := windowPreferencesPath(dataDir)
	temporaryPath := path + ".tmp"
	if err := os.WriteFile(temporaryPath, data, 0600); err != nil {
		return fmt.Errorf("write window preferences: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("save window preferences: %w", err)
	}
	return nil
}
