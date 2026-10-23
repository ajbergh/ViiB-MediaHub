package api

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/ajbergh/viib-mediahub/internal/db"
)

func TestStartupStemScanDiscoversAndRemovesPackages(t *testing.T) {
	for _, adjacentOnly := range []bool{false, true} {
		t.Run(map[bool]string{false: "configured-library", true: "adjacent-only"}[adjacentOnly], func(t *testing.T) {
			root := t.TempDir()
			database, err := db.New(filepath.Join(root, "library.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer database.Close()
			library := filepath.Join(root, "Stems")
			if err := os.MkdirAll(library, 0o700); err != nil {
				t.Fatal(err)
			}
			if !adjacentOnly {
				if err := database.SetStemLocations([]db.StemLocation{{ID: "root", Path: library, Enabled: true}}); err != nil {
					t.Fatal(err)
				}
			}
			for _, id := range []string{"new", "deleted"} {
				source := filepath.Join(root, id+".ogg")
				if err := os.WriteFile(source, []byte(id), 0o600); err != nil {
					t.Fatal(err)
				}
				if err := database.SaveSong(&db.Song{ID: id, Title: id, FilePath: source, AddedAt: 1}); err != nil {
					t.Fatal(err)
				}
			}
			packageRoot := library
			if adjacentOnly {
				packageRoot = root
			}
			writeStemLibraryTestPackage(t, filepath.Join(packageRoot, "new.viibstems"), []byte("new"))
			if err := database.UpsertStemSet(db.StemSet{ID: "removed", SongID: "deleted", PackagePath: filepath.Join(packageRoot, "deleted.viibstems"), Status: "unavailable"}); err != nil {
				t.Fatal(err)
			}
			api := &API{db: database, jobSchedulerOn: true, jobWake: make(chan struct{}, 2)}
			api.queueStartupStemScan()
			api.queueStartupStemScan()
			jobs, err := database.ListJobs(10, "")
			if err != nil || len(jobs) != 1 || jobs[0].Priority != 10 {
				t.Fatalf("expected one background startup scan: jobs=%+v err=%v", jobs, err)
			}
			job, err := database.ClaimNextQueuedJob("Starting")
			if err != nil {
				t.Fatal(err)
			}
			api.runStemLibraryScanJob(job)
			finished, err := database.GetJob(job.ID)
			if err != nil || finished.Status != db.JobStatusSucceeded {
				t.Fatalf("startup scan did not finish: %+v err=%v", finished, err)
			}
			statuses, err := database.ListStemStatuses([]string{"new", "deleted"})
			if err != nil || statuses["new"] != "ready" || statuses["deleted"] != "none" {
				t.Fatalf("startup reconciliation failed: %v err=%v", statuses, err)
			}
		})
	}
}

func TestManualStemScanPromotesPendingStartupScan(t *testing.T) {
	root := t.TempDir()
	database, err := db.New(filepath.Join(root, "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	library := filepath.Join(root, "Stems")
	if err := os.MkdirAll(library, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := database.SetStemLocations([]db.StemLocation{{ID: "root", Path: library, Enabled: true}}); err != nil {
		t.Fatal(err)
	}
	api := &API{db: database, jobSchedulerOn: true, jobWake: make(chan struct{}, 2)}
	api.queueStartupStemScan()
	job, err := database.ClaimNextQueuedJob("Starting")
	if err != nil {
		t.Fatal(err)
	}
	api.analysisPressure.report(defaultPressureTTL)
	api.runStemLibraryScanJob(job) // Background work yields to playback.
	recorder := httptest.NewRecorder()
	api.V2StemRoutes().ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/scan", nil))
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("manual scan rejected: %s", recorder.Body.String())
	}
	var accepted struct {
		JobID string `json:"jobId"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &accepted); err != nil || accepted.JobID != job.ID {
		t.Fatalf("manual request duplicated startup scan: %s err=%v", recorder.Body.String(), err)
	}
	job, err = database.ClaimNextQueuedJob("Starting manual scan")
	if err != nil || job.Priority != analysisForegroundPriority {
		t.Fatalf("manual scan not released from playback backoff: %+v err=%v", job, err)
	}
	api.runStemLibraryScanJob(job)
	finished, err := database.GetJob(job.ID)
	if err != nil || finished.Status != db.JobStatusSucceeded {
		t.Fatalf("manual scan deferred during playback: %+v err=%v", finished, err)
	}
}

func TestStemScanProgressFailureSettlesJob(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "library.db")
	database, err := db.New(path)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	api := &API{db: database, jobSchedulerOn: true, jobWake: make(chan struct{}, 2)}
	api.queueStartupStemScan()
	job, err := database.ClaimNextQueuedJob("Starting")
	if err != nil {
		t.Fatal(err)
	}
	// Simulate a progress-write failure while allowing terminal updates.
	conn, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err := conn.Exec(`CREATE TRIGGER fail_stem_progress BEFORE UPDATE OF progress_current ON operation_jobs
		BEGIN SELECT RAISE(FAIL, 'progress unavailable'); END`); err != nil {
		t.Fatal(err)
	}
	api.runStemLibraryScanJob(job)
	finished, err := database.GetJob(job.ID)
	if err != nil || finished.Status != db.JobStatusFailed || finished.ErrorCode != "stem_library_progress_failed" {
		t.Fatalf("progress failure stranded a running job: %+v err=%v", finished, err)
	}
}
