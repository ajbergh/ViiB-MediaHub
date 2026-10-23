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
	for _, phase := range []string{"initial", "discovery"} {
		t.Run(phase, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, "library.db")
			database, err := db.New(path)
			if err != nil {
				t.Fatal(err)
			}
			defer database.Close()
			library := filepath.Join(root, "Stems")
			writeStemLibraryTestPackage(t, filepath.Join(library, "test.viibstems"), []byte("source"))
			if err := database.SetStemLocations([]db.StemLocation{{ID: "root", Path: library, Enabled: true}}); err != nil {
				t.Fatal(err)
			}
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
			condition := ""
			if phase == "discovery" {
				condition = " WHEN NEW.message LIKE 'Validating stem package%'"
			}
			if _, err := conn.Exec(`CREATE TRIGGER fail_stem_progress BEFORE UPDATE OF progress_current ON operation_jobs` + condition + `
		BEGIN SELECT RAISE(FAIL, 'progress unavailable'); END`); err != nil {
				t.Fatal(err)
			}
			api.runStemLibraryScanJob(job)
			finished, err := database.GetJob(job.ID)
			if err != nil || finished.Status != db.JobStatusFailed || finished.ErrorCode != "stem_library_progress_failed" {
				t.Fatalf("progress failure stranded a running job: %+v err=%v", finished, err)
			}
		})
	}
}

func TestRetryStemScanReusesPendingWork(t *testing.T) {
	for _, pendingState := range []string{"queued", "running"} {
		t.Run(pendingState, func(t *testing.T) {
			database, err := db.New(filepath.Join(t.TempDir(), "library.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer database.Close()
			params := json.RawMessage(`{"locations":[]}`)
			if err := database.CreateJob(db.Job{ID: "failed", Type: "stem_library_scan", Status: db.JobStatusFailed, Parameters: params, Priority: 10}); err != nil {
				t.Fatal(err)
			}
			pendingID, err := database.QueueStemLibraryScan(db.Job{ID: "pending", Parameters: params, Priority: 10})
			if err != nil {
				t.Fatal(err)
			}
			if pendingState == "running" {
				if err := database.StartJob(pendingID, "Running"); err != nil {
					t.Fatal(err)
				}
			}
			api := &API{db: database, jobSchedulerOn: true, jobWake: make(chan struct{}, 2)}
			router := api.V2JobRoutes()
			for i := 0; i < 2; i++ {
				recorder := httptest.NewRecorder()
				router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/failed/retry", nil))
				var retried db.Job
				if err := json.Unmarshal(recorder.Body.Bytes(), &retried); err != nil || recorder.Code != http.StatusAccepted || retried.ID != pendingID || retried.Priority != analysisForegroundPriority {
					t.Fatalf("retry did not promote pending scan: %d %s err=%v", recorder.Code, recorder.Body.String(), err)
				}
			}
			jobs, err := database.ListJobs(10, "")
			if err != nil || len(jobs) != 2 {
				t.Fatalf("retry created duplicate work: %+v err=%v", jobs, err)
			}
		})
	}
}

func TestRetryStemScanRejectsChangedRoots(t *testing.T) {
	database, err := db.New(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := database.CreateJob(db.Job{ID: "failed", Type: "stem_library_scan", Status: db.JobStatusFailed, Parameters: json.RawMessage(`{"locations":["removed-root"]}`)}); err != nil {
		t.Fatal(err)
	}
	api := &API{db: database, jobSchedulerOn: true, jobWake: make(chan struct{}, 2)}
	recorder := httptest.NewRecorder()
	api.V2JobRoutes().ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/failed/retry", nil))
	if recorder.Code != http.StatusConflict {
		t.Fatalf("retry accepted old root configuration: %d %s", recorder.Code, recorder.Body.String())
	}
	jobs, err := database.ListJobs(10, "")
	if err != nil || len(jobs) != 1 {
		t.Fatalf("invalid retry queued work: %+v err=%v", jobs, err)
	}
}

func TestStartupStemScanRequiresSuccessfulCatalogCheck(t *testing.T) {
	for _, readFailure := range []bool{false, true} {
		t.Run(map[bool]string{false: "no-music-folders", true: "catalog-read-failed"}[readFailure], func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "library.db")
			database, err := db.New(path)
			if err != nil {
				t.Fatal(err)
			}
			defer database.Close()
			if readFailure {
				conn, err := sql.Open("sqlite", path)
				if err != nil {
					t.Fatal(err)
				}
				defer conn.Close()
				if _, err := conn.Exec(`ALTER TABLE scan_folders RENAME TO unavailable_scan_folders`); err != nil {
					t.Fatal(err)
				}
			}
			api := &API{db: database, jobSchedulerOn: true, jobWake: make(chan struct{}, 2)}
			api.scanOnStartup()
			jobs, err := database.ListJobs(10, "")
			want := 1
			if readFailure {
				want = 0
			}
			if err != nil || len(jobs) != want {
				t.Fatalf("startup queued wrong number of scans: jobs=%+v want=%d err=%v", jobs, want, err)
			}
		})
	}
}

func TestStemScanFinalProgressIncludesFailedTracks(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "library.db")
	database, err := db.New(path)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	source := filepath.Join(root, "track.ogg")
	if err := os.WriteFile(source, []byte("source"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := database.SaveSong(&db.Song{ID: "song", Title: "Track", FilePath: source, AddedAt: 1}); err != nil {
		t.Fatal(err)
	}
	if err := database.EnsureStemSchema(); err != nil {
		t.Fatal(err)
	}
	conn, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	// Force the last track's registry read to fail before its periodic update.
	if _, err := conn.Exec(`DROP TABLE track_stem_sets`); err != nil {
		t.Fatal(err)
	}
	api := &API{db: database, jobSchedulerOn: true, jobWake: make(chan struct{}, 2)}
	api.queueStartupStemScan()
	job, err := database.ClaimNextQueuedJob("Starting")
	if err != nil {
		t.Fatal(err)
	}
	api.runStemLibraryScanJob(job)
	finished, err := database.GetJob(job.ID)
	var result struct {
		Failed int `json:"failed"`
	}
	if err != nil || json.Unmarshal(finished.Result, &result) != nil || result.Failed != 1 || finished.Status != db.JobStatusSucceeded || finished.ProgressCurrent != 1 || finished.ProgressTotal != 1 {
		t.Fatalf("failed track left stale progress: %+v err=%v", finished, err)
	}
}
