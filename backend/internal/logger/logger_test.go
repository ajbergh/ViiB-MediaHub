package logger

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
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

func TestScanLogAppendsAndMirrorsScanDiagnostics(t *testing.T) {
	directory := t.TempDir()
	defer Close()
	for _, session := range []string{"first", "second"} {
		if err := Init(directory); err != nil {
			t.Fatal(err)
		}
		Scan("decision session=%s engine=spotify", session)
		Scanner("scanner session=%s", session)
		Analysis("analysis session=%s", session)
		API("unrelated API message")
		Close()
	}
	contents, err := os.ReadFile(filepath.Join(directory, "scan.log"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(contents)
	for _, value := range []string{"decision session=first", "decision session=second", "scanner session=first", "analysis session=second"} {
		if !strings.Contains(text, value) {
			t.Fatalf("missing %q in scan log: %s", value, text)
		}
	}
	if strings.Count(text, "Scan Logger Initialized") != 2 || strings.Contains(text, "unrelated API message") {
		t.Fatalf("scan sink contents: %s", text)
	}
	viib, err := os.ReadFile(filepath.Join(directory, "viib.log"))
	if err != nil || !strings.Contains(string(viib), "analysis session=second") || strings.Contains(string(viib), "decision session=") {
		t.Fatalf("application sink changed: %s %v", viib, err)
	}
}

func TestScanLogReinitializationAndConcurrentWrites(t *testing.T) {
	first, second := t.TempDir(), t.TempDir()
	defer Close()
	if err := Init(first); err != nil {
		t.Fatal(err)
	}
	Scan("first directory")
	if err := Init(second); err != nil {
		t.Fatal(err)
	}
	var workers sync.WaitGroup
	for i := 0; i < 10; i++ {
		workers.Add(1)
		go func(i int) { defer workers.Done(); Scan("worker=%d", i) }(i)
	}
	workers.Wait()
	Close()
	Scan("after close")
	contents, err := os.ReadFile(filepath.Join(second, "scan.log"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(contents), "worker=") != 10 || strings.Contains(string(contents), "first directory") || strings.Contains(string(contents), "after close") {
		t.Fatalf("sink lifecycle: %s", contents)
	}
}
