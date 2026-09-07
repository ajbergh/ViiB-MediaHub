package api

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/db"
)

func TestSchedulerWorkerCountIsConservativeAndBounded(t *testing.T) {
	for _, test := range []struct {
		cpuCount int
		want     int
	}{
		{cpuCount: 0, want: 1},
		{cpuCount: 1, want: 1},
		{cpuCount: 4, want: 1},
		{cpuCount: 8, want: 2},
		{cpuCount: 64, want: 2},
	} {
		if got := schedulerWorkerCount(test.cpuCount); got != test.want {
			t.Errorf("schedulerWorkerCount(%d) = %d, want %d", test.cpuCount, got, test.want)
		}
	}
}

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

func TestJobSchedulerDrainsQueuedJobAfterRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.db")
	database, err := db.New(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.CreateJob(db.Job{ID: "queued", Type: "refresh_genre_stats", Status: db.JobStatusQueued}); err != nil {
		t.Fatal(err)
	}
	database.Close()

	reopened, err := db.New(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	api := &API{db: reopened, jobWake: make(chan struct{}, 2)}
	api.V2JobRoutes()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		job, getErr := reopened.GetJob("queued")
		if getErr == nil && job.Status == db.JobStatusSucceeded {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	job, _ := reopened.GetJob("queued")
	t.Fatalf("queued job was not drained after restart: %#v", job)
}

func TestJobPauseResumeRoutes(t *testing.T) {
	database, err := db.New(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	api := &API{db: database, jobWake: make(chan struct{}, 2)}
	if err := database.CreateJob(db.Job{ID: "queued", Type: "refresh_genre_stats"}); err != nil {
		t.Fatal(err)
	}
	router := api.V2JobRoutes()
	for _, path := range []string{"/pause", "/resume"} {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, path, nil)
		router.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusAccepted {
			t.Fatalf("POST %s = %d, want %d", path, recorder.Code, http.StatusAccepted)
		}
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		job, getErr := database.GetJob("queued")
		if getErr == nil && job.Status == db.JobStatusSucceeded {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	job, err := database.GetJob("queued")
	t.Fatalf("resumed job = %#v, %v", job, err)
}
