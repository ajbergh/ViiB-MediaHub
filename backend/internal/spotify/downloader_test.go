package spotify

import (
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
