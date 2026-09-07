package db

import (
	"path/filepath"
	"testing"
)

func TestJobSchemaMigratesLegacyTableWithoutPriority(t *testing.T) {
	database, err := New(filepath.Join(t.TempDir(), "legacy.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	// Simulate a database created before the priority column existed.
	if _, err := database.conn.Exec(`
		CREATE TABLE operation_jobs (
			id TEXT PRIMARY KEY, type TEXT NOT NULL, status TEXT NOT NULL,
			progress_current INTEGER NOT NULL DEFAULT 0,
			progress_total INTEGER NOT NULL DEFAULT 0,
			message TEXT, parameters TEXT, result TEXT,
			error_code TEXT, error_message TEXT,
			attempts INTEGER NOT NULL DEFAULT 0,
			created_at INTEGER NOT NULL, started_at INTEGER,
			completed_at INTEGER, updated_at INTEGER NOT NULL
		);`); err != nil {
		t.Fatal(err)
	}
	if err := database.EnsureJobSchema(); err != nil {
		t.Fatalf("EnsureJobSchema on legacy table = %v, want nil", err)
	}
	if err := database.CreateJob(Job{ID: "legacy", Type: "refresh_genre_stats", Priority: 5}); err != nil {
		t.Fatalf("CreateJob after migration = %v", err)
	}
}
