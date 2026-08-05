package db

import (
	"database/sql"
	"path/filepath"
	"testing"
)

func TestJobLifecycleDoesNotInterruptOnRead(t *testing.T) {
	database, err := New(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer database.Close()

	job := Job{ID: "job-1", Type: "quick_scan", Status: JobStatusQueued, Message: "Queued"}
	if err := database.CreateJob(job); err != nil {
		t.Fatalf("create job: %v", err)
	}
	if err := database.StartJob(job.ID, "Running"); err != nil {
		t.Fatalf("start job: %v", err)
	}

	// Repeated schema checks and reads must not run restart recovery again.
	if err := database.EnsureJobSchema(); err != nil {
		t.Fatalf("ensure schema again: %v", err)
	}
	current, err := database.GetJob(job.ID)
	if err != nil {
		t.Fatalf("get job: %v", err)
	}
	if current.Status != JobStatusRunning {
		t.Fatalf("expected running job, got %s", current.Status)
	}

	if err := database.UpdateJobProgress(job.ID, 2, 5, "Working"); err != nil {
		t.Fatalf("update progress: %v", err)
	}
	if err := database.CompleteJob(job.ID, map[string]int{"changed": 3}, "Done"); err != nil {
		t.Fatalf("complete job: %v", err)
	}
	current, err = database.GetJob(job.ID)
	if err != nil {
		t.Fatalf("get completed job: %v", err)
	}
	if current.Status != JobStatusSucceeded || current.ProgressCurrent != 2 || len(current.Result) == 0 {
		t.Fatalf("unexpected completed job: %#v", current)
	}
}

func TestJobCancellationAndRetryStates(t *testing.T) {
	database, err := New(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer database.Close()

	job := Job{ID: "job-2", Type: "full_scan", Status: JobStatusQueued}
	if err := database.CreateJob(job); err != nil {
		t.Fatalf("create job: %v", err)
	}
	changed, err := database.RequestJobCancellation(job.ID)
	if err != nil || !changed {
		t.Fatalf("request cancellation: changed=%v err=%v", changed, err)
	}
	current, err := database.GetJob(job.ID)
	if err != nil || current.Status != JobStatusCanceled || current.CompletedAt == 0 {
		t.Fatalf("queued cancellation must be terminal immediately: %#v err=%v", current, err)
	}
	if err := database.StartJob(job.ID, "must not start"); err != sql.ErrNoRows {
		t.Fatalf("canceled queued job unexpectedly started: %v", err)
	}

	running := Job{ID: "job-3", Type: "quick_scan", Status: JobStatusQueued}
	if err := database.CreateJob(running); err != nil {
		t.Fatalf("create running job: %v", err)
	}
	if err := database.StartJob(running.ID, "Running"); err != nil {
		t.Fatalf("start running job: %v", err)
	}
	changed, err = database.RequestJobCancellation(running.ID)
	if err != nil || !changed {
		t.Fatalf("request running cancellation: changed=%v err=%v", changed, err)
	}
	current, err = database.GetJob(running.ID)
	if err != nil || current.Status != JobStatusCanceling {
		t.Fatalf("running job should enter canceling: %#v err=%v", current, err)
	}
}
