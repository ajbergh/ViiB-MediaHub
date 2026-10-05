// Tests and fixtures for library operations behavior.

package db

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func openOperationsTestDB(t *testing.T, dataDir string) *DB {
	t.Helper()
	database, err := New(filepath.Join(dataDir, "library.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	if err := database.EnsureLibrarySyncSchema(); err != nil {
		database.Close()
		t.Fatalf("ensure sync schema: %v", err)
	}
	return database
}

func TestLibraryDiagnosticsMetadataAndBackup(t *testing.T) {
	dataDir := t.TempDir()
	database := openOperationsTestDB(t, dataDir)
	defer database.Close()

	existingPath := filepath.Join(dataDir, "existing.mp3")
	if err := os.WriteFile(existingPath, []byte("audio"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := database.AddScanFolder(&ScanFolder{ID: "root", Path: dataDir}); err != nil {
		t.Fatal(err)
	}
	missingPath := filepath.Join(dataDir, "missing.mp3")
	if err := database.SaveSongs([]Song{
		{ID: "existing", Title: "Existing", Artist: "Artist", Album: "Album", FilePath: existingPath, Duration: 1, AddedAt: 1, FileHash: "hash-existing"},
		{ID: "missing", Title: "Missing", Artist: "Artist", Album: "Album", FilePath: missingPath, Duration: 1, AddedAt: 1, FileHash: "hash-missing"},
	}); err != nil {
		t.Fatalf("save songs: %v", err)
	}
	if _, err := database.conn.Exec(`INSERT INTO playlists(id, name, song_ids, created_at) VALUES('playlist', 'Broken', '["existing","unknown"]', 1)`); err != nil {
		t.Fatalf("insert playlist: %v", err)
	}

	diagnostics, err := database.RunLibraryDiagnostics()
	if err != nil {
		t.Fatalf("diagnostics: %v", err)
	}
	if diagnostics.Integrity != "ok" || len(diagnostics.MissingMedia) != 1 || len(diagnostics.BrokenPlaylistReferences) != 1 {
		t.Fatalf("unexpected diagnostics: %#v", diagnostics)
	}

	title := "Updated Title"
	genres := []string{"ambient", "AMBIENT", "post-rock"}
	updated, err := database.UpdateSongMetadata("existing", SongMetadataPatch{Title: &title, Genre: &genres})
	if err != nil {
		t.Fatalf("update metadata: %v", err)
	}
	if updated.Title != title || len(updated.Genre) != 2 || updated.Genre[0] != "Ambient" {
		t.Fatalf("unexpected metadata update: %#v", updated)
	}

	copyPath := filepath.Join(dataDir, "backup-copy.db")
	if err := database.CreateConsistentCopy(copyPath); err != nil {
		t.Fatalf("create consistent copy: %v", err)
	}
	if err := ValidateSQLiteCopy(copyPath); err != nil {
		t.Fatalf("validate copy: %v", err)
	}
	if _, err := database.conn.Exec(`UPDATE song_search SET title = 'corrupt' WHERE song_id = 'existing'`); err != nil {
		t.Fatalf("corrupt search index fixture: %v", err)
	}

	repair, err := database.RepairLibraryIndexes(true, "missing")
	if err != nil {
		t.Fatalf("repair library: %v", err)
	}
	if repair["removedMissing"] != 1 || repair["removedPlaylistReferences"] != 1 {
		t.Fatalf("unexpected repair result: %#v", repair)
	}
	var indexedTitle string
	if err := database.conn.QueryRow(`SELECT title FROM song_search WHERE song_id = 'existing'`).Scan(&indexedTitle); err != nil {
		t.Fatalf("read rebuilt search row: %v", err)
	}
	if indexedTitle != "updated title" {
		t.Fatalf("search repair did not rebuild metadata: %q", indexedTitle)
	}
}

func TestApplyPendingRestoreCreatesRollback(t *testing.T) {
	dataDir := t.TempDir()
	current := openOperationsTestDB(t, dataDir)
	if err := current.SaveSong(&Song{ID: "old", Title: "Old", Artist: "A", Album: "A", FilePath: filepath.Join(dataDir, "old.mp3"), Duration: 1, AddedAt: 1, FileHash: "old-hash"}); err != nil {
		t.Fatal(err)
	}
	current.Close()

	sourceDir := t.TempDir()
	replacement := openOperationsTestDB(t, sourceDir)
	if err := replacement.SaveSong(&Song{ID: "new", Title: "New", Artist: "A", Album: "A", FilePath: filepath.Join(dataDir, "new.mp3"), Duration: 1, AddedAt: 1, FileHash: "new-hash"}); err != nil {
		t.Fatal(err)
	}
	copyPath := filepath.Join(dataDir, "restore-pending", "library.db")
	if err := replacement.CreateConsistentCopy(copyPath); err != nil {
		t.Fatal(err)
	}
	replacement.Close()
	if err := os.WriteFile(filepath.Join(dataDir, "restore-pending", "restore.json"), []byte(`{"staged":true}`), 0600); err != nil {
		t.Fatal(err)
	}

	applied, rollback, err := ApplyPendingRestore(dataDir)
	if err != nil || !applied || rollback == "" {
		t.Fatalf("apply restore: applied=%v rollback=%q err=%v", applied, rollback, err)
	}
	if err := ValidateSQLiteCopy(filepath.Join(dataDir, "library.db")); err != nil {
		t.Fatalf("validate restored DB: %v", err)
	}
	restored := openOperationsTestDB(t, dataDir)
	defer restored.Close()
	if _, err := restored.getSongForOperation("new"); err != nil {
		t.Fatalf("restored song missing: %v", err)
	}
	if _, err := os.Stat(rollback); err != nil {
		t.Fatalf("rollback file missing: %v", err)
	}
}

func TestRepairOnlyDeletesConfirmedMissingOnReadableRoots(t *testing.T) {
	root := t.TempDir()
	database := openOperationsTestDB(t, t.TempDir())
	defer database.Close()
	if err := database.AddScanFolder(&ScanFolder{ID: "root", Path: root}); err != nil {
		t.Fatal(err)
	}
	offline := filepath.Join(root, "offline")
	if err := os.Mkdir(offline, 0700); err != nil {
		t.Fatal(err)
	}
	if err := database.AddScanFolder(&ScanFolder{ID: "offline", Path: offline}); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(offline); err != nil {
		t.Fatal(err)
	}
	for _, song := range []Song{{ID: "confirmed", FilePath: filepath.Join(root, "a.mp3"), AddedAt: 1}, {ID: "unconfirmed", FilePath: filepath.Join(root, "b.mp3"), AddedAt: 1}, {ID: "offline", FilePath: filepath.Join(offline, "c.mp3"), AddedAt: 1}, {ID: "invalid", FilePath: root, AddedAt: 1}} {
		if err := database.SaveSong(&song); err != nil {
			t.Fatal(err)
		}
	}
	report, err := database.RunLibraryDiagnostics()
	if err != nil {
		t.Fatal(err)
	}
	if len(report.MissingMedia) != 2 || len(report.UnavailableMedia) != 2 {
		t.Fatalf("incorrect classification: %+v", report)
	}
	result, err := database.RepairLibraryIndexes(true, "confirmed", "offline", "invalid")
	if err != nil {
		t.Fatal(err)
	}
	if result["removedMissing"] != 1 {
		t.Fatalf("incorrect removals: %+v", result)
	}
	for _, id := range []string{"unconfirmed", "offline", "invalid"} {
		if _, err := database.GetSongByID(id); err != nil {
			t.Fatalf("lost %s: %v", id, err)
		}
	}
}

func TestRestoreCrashWALAndExclusiveAccess(t *testing.T) {
	if dataDir := os.Getenv("VIIB_RESTORE_CRASH_FIXTURE"); dataDir != "" {
		database := openOperationsTestDB(t, dataDir)
		if _, err := database.conn.Exec(`PRAGMA wal_autocheckpoint=0`); err != nil {
			t.Fatal(err)
		}
		if err := database.SaveSong(&Song{ID: "committed-wal", FilePath: filepath.Join(dataDir, "track.mp3"), AddedAt: 1}); err != nil {
			t.Fatal(err)
		}
		os.Exit(0) // Deliberately leave committed WAL without closing/checkpointing.
	}
	dataDir := t.TempDir()
	source := openOperationsTestDB(t, t.TempDir())
	pending := filepath.Join(dataDir, "restore-pending", "library.db")
	if err := source.CreateConsistentCopy(pending); err != nil {
		t.Fatal(err)
	}
	source.Close()
	if err := os.WriteFile(filepath.Join(dataDir, "restore-pending", "restore.json"), []byte(`{}`), 0600); err != nil {
		t.Fatal(err)
	}
	command := exec.Command(os.Args[0], "-test.run=^TestRestoreCrashWALAndExclusiveAccess$")
	command.Env = append(os.Environ(), "VIIB_RESTORE_CRASH_FIXTURE="+dataDir)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("crash fixture: %s %v", output, err)
	}
	if _, err := os.Stat(filepath.Join(dataDir, "library.db-wal")); err != nil {
		t.Fatal("fixture did not leave WAL", err)
	}
	current := openOperationsTestDB(t, dataDir)
	if applied, _, err := ApplyPendingRestore(dataDir); err == nil || applied {
		t.Fatal("restore allowed while application owns database")
	}
	current.Close()
	// Recreate WAL-only crash state after the clean close above.
	command = exec.Command(os.Args[0], "-test.run=^TestRestoreCrashWALAndExclusiveAccess$")
	command.Env = append(os.Environ(), "VIIB_RESTORE_CRASH_FIXTURE="+dataDir)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("crash fixture: %s %v", output, err)
	}
	// A non-empty temporary destination forces activation preparation to fail.
	temporary := filepath.Join(dataDir, "library.db.restore-new")
	if err := os.Mkdir(temporary, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(temporary, "block"), []byte("block"), 0600); err != nil {
		t.Fatal(err)
	}
	applied, rollback, err := ApplyPendingRestore(dataDir)
	if err == nil || applied || rollback == "" {
		t.Fatalf("expected recoverable failure: %v %s %v", applied, rollback, err)
	}
	copy, err := New(rollback)
	if err != nil {
		t.Fatal(err)
	}
	defer copy.Close()
	if _, err := copy.GetSongByID("committed-wal"); err != nil {
		t.Fatal("rollback lost committed WAL row", err)
	}
}
