//go:build linux

// Tests and fixtures for journal linux behavior.

package scanner

import (
	"github.com/ajbergh/viib-mediahub/internal/db"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLinuxDirectoryTimestampCannotHideChildEdit(t *testing.T) {
	root := t.TempDir()
	database, err := db.New(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	scanner := New(database, t.TempDir())
	defer scanner.Close()
	path := filepath.Join(root, "song.mp3")
	if err := os.WriteFile(path, []byte("audio"), 0600); err != nil {
		t.Fatal(err)
	}
	since := time.Now().Add(-time.Minute)
	signatures := map[string]db.DirectorySignature{root: {Path: root, LastVerified: time.Now().Add(time.Hour).UnixMilli(), LatestMtime: since.Add(-time.Minute).UnixMilli()}}
	detector := &LinuxMtimeDetector{scanner: scanner}
	changes, err := detector.scanDirectoryForChanges(root, since.Unix(), signatures)
	if err != nil || len(changes) != 1 || changes[0].Path != path {
		t.Fatalf("child edit hidden by parent: %+v %v", changes, err)
	}
}
