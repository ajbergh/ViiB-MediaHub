// Tests Spotify download behavior, artifact validation, and cancellation.
package spotify

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestSanitizeFilenameWindowsSafety(t *testing.T) {
	tests := map[string]string{
		`AC/DC: Live?`: "AC_DC_ Live_",
		"...":          "Unknown",
		"CON":          "_CON",
		"LPT9.txt":     "_LPT9.txt",
	}
	for input, expected := range tests {
		if actual := sanitizeFilename(input); actual != expected {
			t.Errorf("sanitizeFilename(%q) = %q, want %q", input, actual, expected)
		}
	}
}

func TestSanitizeFilenameTruncatesOnRuneBoundary(t *testing.T) {
	actual := sanitizeFilename(strings.Repeat("é", 100))
	if !utf8.ValidString(actual) {
		t.Fatalf("result is invalid UTF-8: %q", actual)
	}
	if len(actual) > 120 {
		t.Fatalf("result is %d bytes, want at most 120", len(actual))
	}
}

func TestFailedReplacementPreservesExistingDownload(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "Artist", "Album", "Artist - Title.ogg")
	if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
		t.Fatal(err)
	}
	previous := []byte("previous file requiring validation or replacement")
	if err := os.WriteFile(target, previous, 0600); err != nil {
		t.Fatal(err)
	}
	downloader := NewDownloader(&SessionManager{}, root)
	_, err := downloader.DownloadTrack(context.Background(), "5r9W9MJLvHk83fcZSPQ8SE", "Artist", "Title", "Album", nil, nil, nil)
	if err == nil {
		t.Fatal("expected session failure")
	}
	retained, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(retained, previous) {
		t.Fatal("failed replacement changed prior media")
	}
}

func TestValidatedCandidateReplacesExistingDestination(t *testing.T) {
	root := t.TempDir()
	destination := filepath.Join(root, "track.ogg")
	candidate := filepath.Join(root, "candidate.ogg")
	if err := os.WriteFile(destination, []byte("prior"), 0600); err != nil {
		t.Fatal(err)
	}
	replacement := append(testOggPage(2, []byte("headers")), testOggPage(4, []byte("audio"))...)
	if err := os.WriteFile(candidate, replacement, 0600); err != nil {
		t.Fatal(err)
	}
	if err := validateOggPages(context.Background(), candidate); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(candidate, destination); err != nil {
		t.Fatal(err)
	}
	result, err := os.ReadFile(destination)
	if err != nil || !bytes.Equal(result, replacement) {
		t.Fatal("candidate did not replace destination")
	}
}
