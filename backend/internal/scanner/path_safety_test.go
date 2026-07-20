package scanner

import (
	"path/filepath"
	"testing"
)

func TestIsSubPathUsesPathComponents(t *testing.T) {
	root := filepath.Join(string(filepath.Separator), "music")
	if !isSubPath(root, filepath.Join(root, "artist", "song.flac")) {
		t.Fatal("expected nested path to be accepted")
	}
	if isSubPath(root, filepath.Join(string(filepath.Separator), "music-old", "song.flac")) {
		t.Fatal("prefix sibling must not be treated as a child")
	}
	if !isSubPath(root, root) {
		t.Fatal("root should contain itself")
	}
}
