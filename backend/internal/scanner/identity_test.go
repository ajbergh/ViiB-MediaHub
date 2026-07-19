package scanner

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMediaFingerprintSurvivesMove(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "first.flac")
	second := filepath.Join(dir, "renamed.flac")
	if err := os.WriteFile(first, []byte("stable audio payload"), 0o600); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(first)
	if err != nil {
		t.Fatal(err)
	}
	before, err := computeMediaFingerprint(first, info)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(first, second); err != nil {
		t.Fatal(err)
	}
	info, err = os.Stat(second)
	if err != nil {
		t.Fatal(err)
	}
	after, err := computeMediaFingerprint(second, info)
	if err != nil {
		t.Fatal(err)
	}
	if before != after {
		t.Fatalf("fingerprint changed after move: %s != %s", before, after)
	}
}
