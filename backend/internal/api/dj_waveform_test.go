package api

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateWaveformDefersFormatsHandledByTheBrowser(t *testing.T) {
	t.Parallel()

	for _, extension := range []string{".ogg", ".opus", ".flac", ".wav", ".m4a", ".aac", ".unknown"} {
		t.Run(extension, func(t *testing.T) {
			_, err := generateWaveform(filepath.Join(t.TempDir(), "track"+extension))
			if !errors.Is(err, errClientWaveformRequired) {
				t.Fatalf("generateWaveform(%q) error = %v, want client generation marker", extension, err)
			}
		})
	}
}

func TestGenerateWaveformNormalizesMP3Extension(t *testing.T) {
	t.Parallel()

	_, err := generateWaveform(filepath.Join(t.TempDir(), "missing.MP3"))
	if errors.Is(err, errClientWaveformRequired) {
		t.Fatalf("uppercase MP3 was incorrectly deferred to the browser: %v", err)
	}
	if err == nil || !strings.Contains(err.Error(), "failed to open file") {
		t.Fatalf("generateWaveform() error = %v, want MP3 decoder open failure", err)
	}
}
