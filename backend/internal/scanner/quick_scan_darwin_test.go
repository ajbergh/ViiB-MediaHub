// Verifies incremental scans through macOS symlinks and library root changes.
package scanner

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ajbergh/viib-mediahub/internal/db"
)

func TestDarwinQuickScanReconcilesSymlinkedRoots(t *testing.T) {
	root := t.TempDir()
	music := filepath.Join(root, "Music")
	if err := os.Mkdir(music, 0o700); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(root, "Music Alias")
	if err := os.Symlink(music, alias); err != nil {
		t.Fatal(err)
	}
	scanRoot, err := resolveScanRoot(alias)
	if err != nil {
		t.Fatal(err)
	}
	database, err := db.New(filepath.Join(root, "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := database.SaveScanState(db.ScanState{LastScanTime: 1, MacOSEventID: 12345}); err != nil {
		t.Fatal(err)
	}
	if err := database.AddScanFolder(&db.ScanFolder{ID: "new-root", Path: alias}); err != nil {
		t.Fatal(err)
	}
	// A second, disconnected root must not prevent the scan of reachable music.
	if err := database.AddScanFolder(&db.ScanFolder{ID: "offline", Path: filepath.Join(root, "offline")}); err != nil {
		t.Fatal(err)
	}
	newTrack := filepath.Join(scanRoot, "new.mp3")
	if err := os.WriteFile(newTrack, []byte("new track"), 0o600); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(scanRoot, "missing.mp3")
	if err := database.SaveFileMetadataCache(db.FileMetadataCache{FilePath: missing, FileSize: 10, Mtime: 1}); err != nil {
		t.Fatal(err)
	}
	legacyPath := filepath.Join(alias, "legacy.wav")
	writeTestWAV(t, legacyPath)
	info, err := os.Stat(legacyPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.SaveFileMetadataCache(db.FileMetadataCache{
		FilePath: legacyPath, FileSize: info.Size(), Mtime: info.ModTime().UnixMilli(),
	}); err != nil {
		t.Fatal(err)
	}
	fingerprint, err := computeMediaFingerprint(legacyPath, info)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.SaveSong(&db.Song{ID: "legacy", FilePath: legacyPath, FileHash: fingerprint}); err != nil {
		t.Fatal(err)
	}
	canonicalLegacyPath := filepath.Join(scanRoot, "legacy.wav")
	id, err := database.ResolveSongIdentity(canonicalLegacyPath, fingerprint, "new-id")
	if err != nil || id != "legacy" {
		t.Fatalf("resolved legacy identity = %q, %v", id, err)
	}

	scanner := New(database, filepath.Join(root, "data"))
	defer scanner.Close()
	for attempt := 0; attempt < 2; attempt++ {
		result, err := scanner.QuickStartup()
		if err != nil {
			t.Fatal(err)
		}
		changes := result.ChangedFiles
		if len(changes) != 2 || changes[0].Path != newTrack || changes[0].ChangeType != ChangeTypeCreated ||
			changes[1].Path != missing || changes[1].ChangeType != ChangeTypeDeleted {
			t.Fatalf("scan %d reported %+v", attempt+1, changes)
		}
	}

	if err := os.Remove(legacyPath); err != nil {
		t.Fatal(err)
	}
	result, err := scanner.QuickStartup()
	if err != nil {
		t.Fatal(err)
	}
	for _, change := range result.ChangedFiles {
		if change.Path == legacyPath && change.ChangeType == ChangeTypeDeleted {
			return
		}
	}
	t.Fatalf("legacy path deletion missing from %+v", result.ChangedFiles)
}
