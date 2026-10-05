// Tests and fixtures for path safety behavior.

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

func TestFullScansPreserveIgnoredRecords(t *testing.T) {
	root := t.TempDir()
	database, err := db.New(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	path := filepath.Join(root, "ignored.mp3")
	if err := os.WriteFile(path, []byte("audio"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := database.AddScanFolder(&db.ScanFolder{ID: "root", Path: root}); err != nil {
		t.Fatal(err)
	}
	if err := database.SaveSong(&db.Song{ID: "ignored", Title: "User title", FilePath: path, AddedAt: 1}); err != nil {
		t.Fatal(err)
	}
	if err := database.SetSongIgnored("ignored", true); err != nil {
		t.Fatal(err)
	}
	scanner := New(database, t.TempDir())
	defer scanner.Close()
	for i := 0; i < 2; i++ {
		result, err := scanner.ScanAll()
		if err != nil {
			t.Fatal(err)
		}
		if result.RemovedSongs != 0 {
			t.Fatalf("removed ignored record: %+v", result)
		}
		ignored, err := database.GetIgnoredFilePaths()
		if err != nil || len(ignored) != 1 || ignored[0] != path {
			t.Fatalf("ignored state lost: %v %v", ignored, err)
		}
		song, err := database.GetSongByID("ignored")
		if err != nil || song.Title != "User title" {
			t.Fatalf("metadata lost: %v %v", song, err)
		}
	}
}

func TestMtimeDeletionRequiresAvailableRootAndPathBoundary(t *testing.T) {
	root := t.TempDir()
	music := filepath.Join(root, "Music")
	sibling := filepath.Join(root, "Music2")
	for _, dir := range []string{music, sibling} {
		if err := os.Mkdir(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	database, err := db.New(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := database.AddScanFolder(&db.ScanFolder{ID: "root", Path: music}); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(music, "missing.mp3")
	other := filepath.Join(sibling, "other.mp3")
	for _, path := range []string{missing, other} {
		if err := database.SaveFileMetadataCache(db.FileMetadataCache{FilePath: path, FileSize: 1, Mtime: 1}); err != nil {
			t.Fatal(err)
		}
	}
	scanner := New(database, t.TempDir())
	defer scanner.Close()
	detector := newMtimeChangeDetector(scanner)
	changes, err := detector.GetChangesSince(time.Time{}, []string{music})
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 1 || changes[0].Path != missing || changes[0].ChangeType != ChangeTypeDeleted {
		t.Fatalf("wrong deletions: %+v", changes)
	}
	if err := os.Remove(music); err != nil {
		t.Fatal(err)
	}
	changes, err = detector.GetChangesSince(time.Time{}, []string{music})
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 0 {
		t.Fatalf("offline root produced deletions: %+v", changes)
	}
	if err := database.SaveSong(&db.Song{ID: "offline", FilePath: missing, AddedAt: 1}); err != nil {
		t.Fatal(err)
	}
	result, err := scanner.ProcessChanges([]FileChange{{Path: missing, ChangeType: ChangeTypeDeleted}})
	if err != nil {
		t.Fatal(err)
	}
	if result.RemovedSongs != 0 {
		t.Fatal("incremental application deleted offline media")
	}
}

func TestUnchangedParentSignatureDoesNotHideNestedChanges(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "nested")
	if err := os.Mkdir(nested, 0700); err != nil {
		t.Fatal(err)
	}
	database, err := db.New(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	scanner := New(database, t.TempDir())
	defer scanner.Close()
	sig, err := scanner.computeQuickDirectorySignature(root)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(nested, "new.mp3")
	if err := os.WriteFile(path, []byte("audio"), 0600); err != nil {
		t.Fatal(err)
	}
	changes, _, _, err := scanner.checkDirectoryWithSignature(root, map[string]db.DirectorySignature{root: sig}, map[string]db.FileMetadataCache{})
	if err != nil || len(changes) != 1 || changes[0].Path != path {
		t.Fatalf("nested addition hidden: %+v %v", changes, err)
	}
}
