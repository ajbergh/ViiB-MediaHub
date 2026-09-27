package main

import "testing"

func TestWindowCloseActionDefaultsToHide(t *testing.T) {
	if action := loadWindowCloseAction(t.TempDir()); action != closeActionHide {
		t.Fatalf("loadWindowCloseAction() = %q, want %q", action, closeActionHide)
	}
}

func TestWindowCloseActionRoundTrip(t *testing.T) {
	directory := t.TempDir()
	if err := saveWindowCloseAction(directory, closeActionQuit); err != nil {
		t.Fatalf("saveWindowCloseAction() error = %v", err)
	}
	if action := loadWindowCloseAction(directory); action != closeActionQuit {
		t.Fatalf("loadWindowCloseAction() = %q, want %q", action, closeActionQuit)
	}
}

func TestWindowCloseActionRejectsUnknownValues(t *testing.T) {
	if err := saveWindowCloseAction(t.TempDir(), "minimise"); err == nil {
		t.Fatal("saveWindowCloseAction() accepted an unknown action")
	}
}
