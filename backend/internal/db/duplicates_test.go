package db

import (
	"path/filepath"
	"testing"
	"time"
)

func TestDuplicateGroupsAndIgnoredVisibility(t *testing.T) {
	database, err := New(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	now := time.Now().UnixMilli()
	first := Song{ID: "first", Title: "Song", Artist: "Artist", Album: "Album", FilePath: filepath.Join(t.TempDir(), "one.flac"), FileHash: "same-fingerprint", AddedAt: now, Duration: 180}
	second := Song{ID: "second", Title: "Song Copy", Artist: "Artist", Album: "Album", FilePath: filepath.Join(t.TempDir(), "two.flac"), FileHash: "same-fingerprint", AddedAt: now, Duration: 180}
	if err := database.SaveSong(&first); err != nil {
		t.Fatal(err)
	}
	if err := database.SaveSong(&second); err != nil {
		t.Fatal(err)
	}

	groups, err := database.GetDuplicateGroups()
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 1 || len(groups[0].Songs) != 2 {
		t.Fatalf("unexpected duplicate groups: %#v", groups)
	}

	if err := database.SetSongIgnored(second.ID, true); err != nil {
		t.Fatal(err)
	}
	visible, err := database.GetAllSongs()
	if err != nil {
		t.Fatal(err)
	}
	if len(visible) != 1 || visible[0].ID != first.ID {
		t.Fatalf("unexpected visible songs: %#v", visible)
	}
	ignored, err := database.GetIgnoredSongs()
	if err != nil {
		t.Fatal(err)
	}
	if len(ignored) != 1 || ignored[0].ID != second.ID {
		t.Fatalf("unexpected ignored songs: %#v", ignored)
	}
}
