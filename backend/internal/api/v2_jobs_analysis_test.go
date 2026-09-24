package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/analysis/track"
	"github.com/ajbergh/viib-mediahub/internal/analysisbench"
	"github.com/ajbergh/viib-mediahub/internal/db"
)

// analysisCatalog writes count decodable click tracks into a fresh catalog and
// returns the database plus its directory so a restart can reopen it.
func analysisCatalog(t *testing.T, count int) (*db.DB, string, []string) {
	t.Helper()
	directory := t.TempDir()
	database, err := db.New(filepath.Join(directory, "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })

	fixture, err := analysisbench.NewClickTrack("clicks", 128, 3, 22050, 1)
	if err != nil {
		t.Fatal(err)
	}
	var wav bytes.Buffer
	if err := analysisbench.WriteWAVPCM16(&wav, fixture); err != nil {
		t.Fatal(err)
	}
	ids := make([]string, 0, count)
	for i := 0; i < count; i++ {
		id := string(rune('a' + i))
		path := filepath.Join(directory, id+".wav")
		if err := os.WriteFile(path, wav.Bytes(), 0600); err != nil {
			t.Fatal(err)
		}
		if err := database.SaveSong(&db.Song{ID: id, Title: id, Artist: "Artist", Album: "Album", FilePath: path, AddedAt: int64(i + 1)}); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	return database, directory, ids
}

func awaitJobStatus(t *testing.T, database *db.DB, id string, want string) db.Job {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	var last db.Job
	for time.Now().Before(deadline) {
		job, err := database.GetJob(id)
		if err == nil {
			last = job
			if job.Status == want {
				return job
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("job %s did not reach %q: %#v", id, want, last)
	return last
}

func TestAnalyzeTracksJobIsAcceptedAndDrained(t *testing.T) {
	database, _, ids := analysisCatalog(t, 3)
	api := &API{db: database}
	router := api.V2JobRoutes()

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"type":"analyze_tracks","parameters":{"mode":"missing"},"priority":10}`))
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("POST jobs = %d, want %d: %s", recorder.Code, http.StatusAccepted, recorder.Body.String())
	}
	var created db.Job
	if err := json.NewDecoder(recorder.Result().Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	if created.Type != JobTypeAnalyzeTracks || created.Priority != 10 {
		t.Fatalf("created = %#v, want an analyze_tracks job at priority 10", created)
	}

	finished := awaitJobStatus(t, database, created.ID, db.JobStatusSucceeded)
	var result struct {
		Total    int `json:"total"`
		Analyzed int `json:"analyzed"`
		Skipped  int `json:"skipped"`
		Failed   int `json:"failed"`
	}
	if err := json.Unmarshal(finished.Result, &result); err != nil {
		t.Fatal(err)
	}
	if result.Total != 3 || result.Analyzed != 3 || result.Failed != 0 {
		t.Fatalf("result = %#v, want 3 analyzed", result)
	}
	if finished.ProgressCurrent != 3 || finished.ProgressTotal != 3 {
		t.Fatalf("progress = %d/%d, want 3/3", finished.ProgressCurrent, finished.ProgressTotal)
	}
	for _, id := range ids {
		record, err := database.GetTrackAnalysis(id)
		if err != nil {
			t.Fatalf("%s: %v", id, err)
		}
		if record.BPM == nil {
			t.Fatalf("%s: job did not persist a measured BPM", id)
		}
	}
}

func TestCreateAnalysisJobSnapshotsAndValidatesAutomaticCueMode(t *testing.T) {
	database, _, _ := analysisCatalog(t, 1)
	api := &API{db: database, jobSchedulerOn: true}
	if err := database.SetSetting(SettingAutoCueMode, "replace-generated"); err != nil {
		t.Fatal(err)
	}
	create := func(parameters string) (int, db.Job) {
		t.Helper()
		request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"type":"analyze_tracks","parameters":`+parameters+`}`))
		recorder := httptest.NewRecorder()
		api.createJobV2(recorder, request)
		var created db.Job
		if recorder.Code == http.StatusAccepted {
			if err := json.Unmarshal(recorder.Body.Bytes(), &created); err != nil {
				t.Fatal(err)
			}
		}
		return recorder.Code, created
	}

	status, created := create(`{"mode":"missing"}`)
	if status != http.StatusAccepted {
		t.Fatalf("create analysis job = %d, want 202", status)
	}
	selection, err := db.ParseAnalysisSelection(created.Parameters)
	if err != nil || selection.AutoCueMode != db.AutomaticCuePointsReplaceGenerated {
		t.Fatalf("missing job mode did not snapshot setting: %#v err=%v", selection, err)
	}
	if err := database.SetSetting(SettingAutoCueMode, "off"); err != nil {
		t.Fatal(err)
	}
	status, created = create(`{"mode":"missing","autoCueMode":"suggest"}`)
	if status != http.StatusAccepted {
		t.Fatalf("create explicit analysis job = %d, want 202", status)
	}
	selection, err = db.ParseAnalysisSelection(created.Parameters)
	if err != nil || selection.AutoCueMode != db.AutomaticCuePointsSuggest {
		t.Fatalf("explicit job mode was not retained: %#v err=%v", selection, err)
	}
	if status, _ = create(`{"mode":"missing","autoCueMode":"refresh-all"}`); status != http.StatusBadRequest {
		t.Fatalf("invalid explicit mode create = %d, want 400", status)
	}
}

func TestAnalyzeTracksJobStreamsAvailablePlexSource(t *testing.T) {
	fixture, err := analysisbench.NewClickTrack("plex-clicks", 128, 3, 22050, 1)
	if err != nil {
		t.Fatal(err)
	}
	var wav bytes.Buffer
	if err := analysisbench.WriteWAVPCM16(&wav, fixture); err != nil {
		t.Fatal(err)
	}
	const token = "analysis-secret"
	var sawToken, leakedToken bool
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawToken = r.Header.Get("X-Plex-Token") == token
		leakedToken = r.URL.Query().Get("X-Plex-Token") != "" || strings.Contains(r.URL.RawQuery, token)
		w.Header().Set("Content-Type", "audio/wav")
		_, _ = w.Write(wav.Bytes())
	}))
	defer upstream.Close()

	database, api, plexTrack := setupPlexProxyTest(t, upstream.URL, "/audio", token, true)
	plexTrack.Container = "wav"
	if _, _, _, err := database.SyncPlexLibrary(plexTrack.SourceID, plexTrack.LibraryID, []db.PlexCatalogTrack{plexTrack}); err != nil {
		t.Fatal(err)
	}
	api.V2JobRoutes()
	if err := database.CreateJob(db.Job{ID: "plex-analysis", Type: JobTypeAnalyzeTracks, Status: db.JobStatusQueued,
		Parameters: json.RawMessage(`{"mode":"missing","source":"plex"}`)}); err != nil {
		t.Fatal(err)
	}
	api.wakeJobScheduler()
	finished := awaitJobStatus(t, database, "plex-analysis", db.JobStatusSucceeded)
	if sawToken == false || leakedToken {
		t.Fatalf("Plex authorization handling bad: token=%v leaked=%v", sawToken, leakedToken)
	}
	var result struct{ Analyzed, Failed int }
	if err := json.Unmarshal(finished.Result, &result); err != nil {
		t.Fatal(err)
	}
	if result.Analyzed != 1 || result.Failed != 0 {
		t.Fatalf("result = %#v, want one successful Plex analysis", result)
	}
	record, err := database.GetTrackAnalysis(plexTrack.SongID)
	if err != nil {
		t.Fatal(err)
	}
	if record.BPM == nil || record.SourceRevision == nil || record.SourceSize != nil || !strings.HasPrefix(*record.SourceRevision, "plex:") {
		t.Fatalf("persisted Plex analysis = %#v", record)
	}
}

func TestAnalyzeTracksJobRejectsUnexpandableSelection(t *testing.T) {
	database, _, _ := analysisCatalog(t, 1)
	api := &API{db: database}
	// Keep the pool from starting so nothing is dispatched during the test.
	api.jobSchedulerOn = true

	for _, payload := range []string{
		`{"type":"analyze_tracks","parameters":{"mode":"ids"}}`,
		`{"type":"analyze_tracks","parameters":{"mode":"playlist"}}`,
		`{"type":"analyze_tracks","parameters":{"mode":"nonsense"}}`,
	} {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(payload))
		api.createJobV2(recorder, request)
		if recorder.Code != http.StatusBadRequest {
			t.Errorf("POST %s = %d, want %d", payload, recorder.Code, http.StatusBadRequest)
		}
	}
}

// The Phase 4 acceptance criterion: a run interrupted partway through must
// resume and re-dispatch only the unfinished tracks.
func TestAnalyzeTracksJobResumesOnlyUnfinishedWorkAfterRestart(t *testing.T) {
	database, directory, ids := analysisCatalog(t, 6)

	// Simulate a run that settled the first two tracks before the process died.
	registry := decoderRegistry()
	done, err := track.Run(t.Context(), database, registry, ids[:2], track.RunOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if done.Analyzed != 2 {
		t.Fatalf("setup analyzed %d tracks, want 2", done.Analyzed)
	}
	if err := database.CreateJob(db.Job{ID: "resumed", Type: JobTypeAnalyzeTracks, Status: db.JobStatusQueued,
		Parameters: json.RawMessage(`{"mode":"missing"}`)}); err != nil {
		t.Fatal(err)
	}
	database.Close()

	// Restart: the queued job survives and the work list is re-expanded.
	reopened, err := db.New(filepath.Join(directory, "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	api := &API{db: reopened}
	api.V2JobRoutes()

	finished := awaitJobStatus(t, reopened, "resumed", db.JobStatusSucceeded)
	var result struct {
		Total    int `json:"total"`
		Analyzed int `json:"analyzed"`
		Skipped  int `json:"skipped"`
	}
	if err := json.Unmarshal(finished.Result, &result); err != nil {
		t.Fatal(err)
	}
	// "missing" expansion excludes the two settled tracks entirely, so the
	// resumed run must cover exactly the four that were never started.
	if result.Total != 4 || result.Analyzed != 4 {
		t.Fatalf("result = %#v, want the 4 unfinished tracks analyzed", result)
	}
	for _, id := range ids {
		record, err := reopened.GetTrackAnalysis(id)
		if err != nil {
			t.Fatalf("%s: %v", id, err)
		}
		if record.BPM == nil {
			t.Fatalf("%s: track was left unanalyzed after resume", id)
		}
	}
}

// A resumed "all" selection still visits every track but must skip the ones
// that are already valid rather than re-decoding them.
func TestAnalyzeTracksJobSkipsAlreadyValidTracks(t *testing.T) {
	database, _, ids := analysisCatalog(t, 4)
	api := &API{db: database}
	api.V2JobRoutes()

	if err := database.CreateJob(db.Job{ID: "first", Type: JobTypeAnalyzeTracks, Status: db.JobStatusQueued,
		Parameters: json.RawMessage(`{"mode":"all"}`)}); err != nil {
		t.Fatal(err)
	}
	api.wakeJobScheduler()
	awaitJobStatus(t, database, "first", db.JobStatusSucceeded)

	if err := database.CreateJob(db.Job{ID: "second", Type: JobTypeAnalyzeTracks, Status: db.JobStatusQueued,
		Parameters: json.RawMessage(`{"mode":"all"}`)}); err != nil {
		t.Fatal(err)
	}
	api.wakeJobScheduler()
	finished := awaitJobStatus(t, database, "second", db.JobStatusSucceeded)

	var result struct {
		Total    int `json:"total"`
		Analyzed int `json:"analyzed"`
		Skipped  int `json:"skipped"`
	}
	if err := json.Unmarshal(finished.Result, &result); err != nil {
		t.Fatal(err)
	}
	if result.Total != len(ids) || result.Skipped != len(ids) || result.Analyzed != 0 {
		t.Fatalf("result = %#v, want every track skipped on the second run", result)
	}
}

// A selection that matches nothing must complete rather than fail, so
// "Analyze Missing" on a fully analyzed library is not an error.
func TestAnalyzeTracksJobCompletesWithEmptySelection(t *testing.T) {
	database, _, _ := analysisCatalog(t, 0)
	api := &API{db: database}
	api.V2JobRoutes()
	if err := database.CreateJob(db.Job{ID: "empty", Type: JobTypeAnalyzeTracks, Status: db.JobStatusQueued,
		Parameters: json.RawMessage(`{"mode":"missing"}`)}); err != nil {
		t.Fatal(err)
	}
	api.wakeJobScheduler()
	finished := awaitJobStatus(t, database, "empty", db.JobStatusSucceeded)
	if finished.ProgressTotal != 0 {
		t.Fatalf("progress total = %d, want 0", finished.ProgressTotal)
	}
}

// Concurrency must stay bounded by the worker policy. Submitting many jobs at
// once must not run them all at the same time.
func TestAnalyzeTracksJobsStayWithinWorkerBound(t *testing.T) {
	database, _, _ := analysisCatalog(t, 4)
	api := &API{db: database}
	api.V2JobRoutes()

	const submitted = 20
	for i := 0; i < submitted; i++ {
		if err := database.CreateJob(db.Job{ID: "job-" + string(rune('a'+i)), Type: JobTypeAnalyzeTracks,
			Status: db.JobStatusQueued, Parameters: json.RawMessage(`{"mode":"all"}`)}); err != nil {
			t.Fatal(err)
		}
	}
	api.wakeJobScheduler()

	bound := schedulerWorkerCount(runtime.NumCPU())
	peak := 0
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		running, err := database.ListJobs(500, db.JobStatusRunning)
		if err == nil {
			if len(running) > peak {
				peak = len(running)
			}
			if len(running) > bound {
				t.Fatalf("%d jobs running at once, want at most %d", len(running), bound)
			}
		}
		// Do not combine independent queued/running snapshots as a completion
		// signal. A worker can claim a job between those reads, yielding an
		// impossible-looking zero/zero observation while that job is running.
		succeeded, err := database.ListJobs(500, db.JobStatusSucceeded)
		if err == nil && len(succeeded) == submitted {
			break
		}
		time.Sleep(2 * time.Millisecond)
	}
	if peak == 0 {
		t.Fatal("no job was observed running; the bound was not exercised")
	}
	succeeded, err := database.ListJobs(500, db.JobStatusSucceeded)
	if err != nil {
		t.Fatal(err)
	}
	if len(succeeded) != submitted {
		t.Fatalf("%d jobs succeeded, want %d", len(succeeded), submitted)
	}
}
