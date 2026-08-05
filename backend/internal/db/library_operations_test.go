package db

import (
	"os"
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

	repair, err := database.RepairLibraryIndexes(true)
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
