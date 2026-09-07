package db

import (
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"
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

// A deferred job must not be immediately re-claimable, otherwise a job that
// yields would be claimed again at once and spin the dispatcher.
func TestRequeueJobDelaysReclaim(t *testing.T) {
	database, err := New(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := database.CreateJob(Job{ID: "yield", Type: "refresh_genre_stats", Status: JobStatusQueued}); err != nil {
		t.Fatal(err)
	}
	if _, err := database.ClaimNextQueuedJob("running"); err != nil {
		t.Fatal(err)
	}
	requeued, err := database.RequeueJob("yield", "waiting", time.Minute)
	if err != nil || !requeued {
		t.Fatalf("RequeueJob = %v, %v", requeued, err)
	}
	if _, err := database.ClaimNextQueuedJob("running"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("claim of a backed-off job = %v, want sql.ErrNoRows", err)
	}

	// Clearing the backoff makes it claimable again without waiting.
	cleared, err := database.ClearJobBackoff()
	if err != nil {
		t.Fatal(err)
	}
	if cleared != 1 {
		t.Fatalf("ClearJobBackoff cleared %d jobs, want 1", cleared)
	}
	if _, err := database.ClaimNextQueuedJob("running"); err != nil {
		t.Fatalf("claim after clearing backoff = %v, want success", err)
	}
}

// Requeueing only applies to a running job; a canceled or completed job must
// not be resurrected.
func TestRequeueJobIgnoresNonRunningJobs(t *testing.T) {
	database, err := New(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := database.CreateJob(Job{ID: "settled", Type: "refresh_genre_stats", Status: JobStatusCanceled}); err != nil {
		t.Fatal(err)
	}
	requeued, err := database.RequeueJob("settled", "waiting", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if requeued {
		t.Fatal("a canceled job must not be requeued")
	}
}

// The legacy migration must also add available_at, and the claim query must
// work against an upgraded database.
func TestJobSchemaMigratesLegacyTableAvailableAt(t *testing.T) {
	database, err := New(filepath.Join(t.TempDir(), "legacy.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if _, err := database.conn.Exec(`
		CREATE TABLE operation_jobs (
			id TEXT PRIMARY KEY, type TEXT NOT NULL, status TEXT NOT NULL,
			progress_current INTEGER NOT NULL DEFAULT 0,
			progress_total INTEGER NOT NULL DEFAULT 0,
			priority INTEGER NOT NULL DEFAULT 0,
			message TEXT, parameters TEXT, result TEXT,
			error_code TEXT, error_message TEXT,
			attempts INTEGER NOT NULL DEFAULT 0,
			created_at INTEGER NOT NULL, started_at INTEGER,
			completed_at INTEGER, updated_at INTEGER NOT NULL
		);`); err != nil {
		t.Fatal(err)
	}
	if err := database.EnsureJobSchema(); err != nil {
		t.Fatalf("EnsureJobSchema on a pre-available_at table = %v, want nil", err)
	}
	if err := database.CreateJob(Job{ID: "job", Type: "refresh_genre_stats"}); err != nil {
		t.Fatal(err)
	}
	if _, err := database.ClaimNextQueuedJob("running"); err != nil {
		t.Fatalf("claim after migration = %v, want success", err)
	}
}
