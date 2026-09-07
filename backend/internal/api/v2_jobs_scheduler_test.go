package api

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/db"
)

func TestJobSchedulerDrainsQueuedJob(t *testing.T) {
	database, err := db.New(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	api := &API{db: database, jobWake: make(chan struct{}, 2)}
	if err := database.CreateJob(db.Job{ID: "refresh", Type: "refresh_genre_stats", Status: db.JobStatusQueued}); err != nil {
		t.Fatal(err)
	}
	api.wakeJobScheduler()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		job, getErr := database.GetJob("refresh")
		if getErr == nil && job.Status == db.JobStatusSucceeded {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	job, _ := database.GetJob("refresh")
	t.Fatalf("queued job was not drained: %#v", job)
}
