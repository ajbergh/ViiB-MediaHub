package scanner

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/db"
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

func TestStemPackageDirectoryIsExcludedFromMusicScanningAndReconciled(t *testing.T) {
	root := t.TempDir()
	libraryPath := filepath.Join(root, "Music")
	stemDir := filepath.Join(libraryPath, "Artist", "Album", "Track.VIIBSTEMS")
	if err := os.MkdirAll(stemDir, 0o700); err != nil {
		t.Fatal(err)
	}
	stemWAV := filepath.Join(stemDir, "vocals.wav")
	if err := os.WriteFile(stemWAV, []byte("stem audio"), 0o600); err != nil {
		t.Fatal(err)
	}
	database, err := db.New(filepath.Join(root, "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := database.AddScanFolder(&db.ScanFolder{ID: "music", Path: libraryPath, AddedAt: time.Now().UnixMilli()}); err != nil {
		t.Fatal(err)
	}
	if err := database.SaveSong(&db.Song{ID: "old-stem-row", Title: "Vocals", FilePath: stemWAV, AddedAt: 1}); err != nil {
		t.Fatal(err)
	}
	if err := database.SaveFileMetadataCache(db.FileMetadataCache{FilePath: stemWAV, FileSize: 10, Mtime: 1, LastVerified: 1}); err != nil {
		t.Fatal(err)
	}

	s := New(database, filepath.Join(root, "data"))
	defer s.Close()
	result, err := s.ScanAll()
	if err != nil {
		t.Fatal(err)
	}
	if result.RemovedSongs != 1 {
		t.Fatalf("full scan removed %d songs, want one previously misindexed stem file", result.RemovedSongs)
	}
	paths, err := database.GetAllFilePaths()
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range paths {
		if filepath.Clean(path) == filepath.Clean(stemWAV) {
			t.Fatalf("stem WAV was retained in the music catalog: %q", path)
		}
	}
	metadata, err := database.GetFileMetadataCache(stemWAV)
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatal(err)
	}
	if metadata != nil {
		t.Fatalf("stale file metadata cache was retained: %+v", metadata)
	}
}

func TestStemPackagePathDetectionIsCaseInsensitiveAndComponentBased(t *testing.T) {
	if !pathContainsStemPackageDirectory(filepath.Join("Music", "Artist", "Album.VIIBSTEMS", "vocals.wav")) {
		t.Fatal("expected mixed-case stem package component to be recognized")
	}
	if pathContainsStemPackageDirectory(filepath.Join("Music", "Album.viibstems-backup", "song.wav")) {
		t.Fatal("a similar suffix without the exact package extension must not be excluded")
	}
}
