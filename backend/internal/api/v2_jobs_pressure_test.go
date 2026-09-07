package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/db"
)

func TestAnalysisPressureRouteReportsAndClears(t *testing.T) {
	database, _, _ := analysisCatalog(t, 0)
	api := &API{db: database}
	api.jobSchedulerOn = true
	router := api.V2JobRoutes()

	post := func(body string) map[string]any {
		t.Helper()
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/analysis-pressure", strings.NewReader(body)))
		if recorder.Code != http.StatusAccepted {
			t.Fatalf("POST /analysis-pressure %s = %d, want %d", body, recorder.Code, http.StatusAccepted)
		}
		var payload map[string]any
		if err := json.NewDecoder(recorder.Result().Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		return payload
	}

	if payload := post(`{"active":true,"ttlSeconds":60}`); payload["active"] != true {
		t.Fatalf("report = %#v, want active", payload)
	}
	if !api.analysisThrottled(0) {
		t.Fatal("background analysis must be throttled while playback is active")
	}
	if api.analysisThrottled(analysisForegroundPriority) {
		t.Fatal("an explicit foreground request must not be throttled")
	}
	if payload := post(`{"active":false}`); payload["active"] != false {
		t.Fatalf("clear = %#v, want inactive", payload)
	}
	if api.analysisThrottled(0) {
		t.Fatal("throttling must stop once playback stops")
	}
}

// A stale report must expire on its own. Otherwise a closed or crashed UI would
// park the library queue forever.
func TestAnalysisPressureExpiresWithoutRenewal(t *testing.T) {
	var pressure playbackPressure
	pressure.report(30 * time.Millisecond)
	if !pressure.active() {
		t.Fatal("a fresh report must be active")
	}
	time.Sleep(60 * time.Millisecond)
	if pressure.active() {
		t.Fatal("an unrenewed report must expire")
	}
}

func TestAnalysisPressureTTLIsBounded(t *testing.T) {
	var pressure playbackPressure
	pressure.report(24 * time.Hour)
	if remaining := pressure.remaining(); remaining > maxPressureTTL {
		t.Fatalf("remaining = %v, want at most %v", remaining, maxPressureTTL)
	}
}

// Playback must reduce analysis pressure by releasing the worker, not by
// holding it while waiting: otherwise scans and foreground analysis would
// queue behind a paused run.
func TestAnalyzeTracksJobDefersDuringPlaybackAndResumesAfter(t *testing.T) {
	database, _, ids := analysisCatalog(t, 3)
	api := &API{db: database}
	router := api.V2JobRoutes()
	api.analysisPressure.report(time.Minute)

	if err := database.CreateJob(db.Job{ID: "deferred", Type: JobTypeAnalyzeTracks, Status: db.JobStatusQueued,
		Parameters: json.RawMessage(`{"mode":"missing"}`)}); err != nil {
		t.Fatal(err)
	}
	api.wakeJobScheduler()

	// The job must return to the queue rather than run or fail.
	deadline := time.Now().Add(10 * time.Second)
	requeued := false
	for time.Now().Before(deadline) {
		job, err := database.GetJob("deferred")
		if err == nil && job.Status == db.JobStatusQueued && job.Attempts > 0 {
			requeued = true
			break
		}
		if err == nil && (job.Status == db.JobStatusSucceeded || job.Status == db.JobStatusFailed) {
			t.Fatalf("job ran during playback: %#v", job)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !requeued {
		job, _ := database.GetJob("deferred")
		t.Fatalf("job was not deferred back to the queue: %#v", job)
	}
	for _, id := range ids {
		if _, err := database.GetTrackAnalysis(id); err == nil {
			t.Fatalf("%s: no track may be analyzed while playback is active", id)
		}
	}

	// Clearing playback resumes the deferred work without waiting out the backoff.
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/analysis-pressure", strings.NewReader(`{"active":false}`)))
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("clearing pressure = %d, want %d", recorder.Code, http.StatusAccepted)
	}
	finished := awaitJobStatus(t, database, "deferred", db.JobStatusSucceeded)
	var result struct {
		Analyzed int `json:"analyzed"`
	}
	if err := json.Unmarshal(finished.Result, &result); err != nil {
		t.Fatal(err)
	}
	if result.Analyzed != len(ids) {
		t.Fatalf("result = %#v, want all %d tracks analyzed after playback stopped", result, len(ids))
	}
}

// An explicit "analyze this now" request outranks playback pressure.
func TestForegroundAnalysisRunsDuringPlayback(t *testing.T) {
	database, _, ids := analysisCatalog(t, 2)
	api := &API{db: database}
	api.V2JobRoutes()
	api.analysisPressure.report(time.Minute)

	if err := database.CreateJob(db.Job{ID: "now", Type: JobTypeAnalyzeTracks, Status: db.JobStatusQueued,
		Priority: analysisForegroundPriority, Parameters: json.RawMessage(`{"mode":"missing"}`)}); err != nil {
		t.Fatal(err)
	}
	api.wakeJobScheduler()
	awaitJobStatus(t, database, "now", db.JobStatusSucceeded)
	for _, id := range ids {
		record, err := database.GetTrackAnalysis(id)
		if err != nil {
			t.Fatalf("%s: %v", id, err)
		}
		if record.BPM == nil {
			t.Fatalf("%s: foreground analysis did not measure the track", id)
		}
	}
}
