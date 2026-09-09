package logger

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitAppendsAcrossSessions(t *testing.T) {
	directory := t.TempDir()
	defer Close()

	if err := Init(directory); err != nil {
		t.Fatal(err)
	}
	Log("Test", "first session")
	Close()

	if err := Init(directory); err != nil {
		t.Fatal(err)
	}
	Log("Test", "second session")
	Close()

	contents, err := os.ReadFile(filepath.Join(directory, "viib.log"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(contents)
	if !strings.Contains(text, "first session") || !strings.Contains(text, "second session") {
		t.Fatalf("log did not retain both sessions:\n%s", text)
	}
	if strings.Count(text, "Logger Initialized") != 2 {
		t.Fatalf("session headers = %d, want 2:\n%s", strings.Count(text, "Logger Initialized"), text)
	}
}
