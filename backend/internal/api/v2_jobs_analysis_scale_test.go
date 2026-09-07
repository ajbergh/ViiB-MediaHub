package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/analysisbench"
	"github.com/ajbergh/viib-mediahub/internal/db"
)

// TestAnalyzeTracksJobHandlesLargeCatalogWithControlledFailures is the Phase 4
// integration test: a synthetic catalog containing decodable audio, corrupt
// audio, an unsupported codec, and a missing file must run to completion with
// every track settled and every failure classified. It is skipped under -short
// because it decodes the whole catalog.
func TestAnalyzeTracksJobHandlesLargeCatalogWithControlledFailures(t *testing.T) {
	if testing.Short() {
		t.Skip("decodes a whole synthetic catalog")
	}
	// The roadmap specifies a 1k synthetic catalog for this phase.
	const (
		decodable   = 900
		corrupt     = 40
		unsupported = 30
		absent      = 30
	)
	total := decodable + corrupt + unsupported + absent

	directory := t.TempDir()
	database, err := db.New(filepath.Join(directory, "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	fixture, err := analysisbench.NewClickTrack("clicks", 128, 2, 22050, 1)
	if err != nil {
		t.Fatal(err)
	}
	var wav bytes.Buffer
	if err := analysisbench.WriteWAVPCM16(&wav, fixture); err != nil {
		t.Fatal(err)
	}

	save := func(id, path string) {
		t.Helper()
		if err := database.SaveSong(&db.Song{ID: id, Title: id, Artist: "Artist", Album: "Album", FilePath: path, AddedAt: 1}); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < decodable; i++ {
		id := fmt.Sprintf("ok-%03d", i)
		path := filepath.Join(directory, id+".wav")
		if err := os.WriteFile(path, wav.Bytes(), 0600); err != nil {
			t.Fatal(err)
		}
		save(id, path)
	}
	for i := 0; i < corrupt; i++ {
		id := fmt.Sprintf("corrupt-%03d", i)
		path := filepath.Join(directory, id+".wav")
		if err := os.WriteFile(path, []byte("RIFF not really audio"), 0600); err != nil {
			t.Fatal(err)
		}
		save(id, path)
	}
	for i := 0; i < unsupported; i++ {
		id := fmt.Sprintf("opus-%03d", i)
		path := filepath.Join(directory, id+".opus")
		if err := os.WriteFile(path, []byte("opus placeholder"), 0600); err != nil {
			t.Fatal(err)
		}
		save(id, path)
	}
	for i := 0; i < absent; i++ {
		id := fmt.Sprintf("gone-%03d", i)
		save(id, filepath.Join(directory, id+".wav"))
	}

	api := &API{db: database}
	api.V2JobRoutes()
	if err := database.CreateJob(db.Job{ID: "scale", Type: JobTypeAnalyzeTracks, Status: db.JobStatusQueued,
		Parameters: json.RawMessage(`{"mode":"all"}`)}); err != nil {
		t.Fatal(err)
	}
	api.wakeJobScheduler()

	started := time.Now()
	finished := awaitLongJobStatus(t, database, "scale", db.JobStatusSucceeded, 5*time.Minute)
	t.Logf("analyzed %d tracks in %s", total, time.Since(started).Round(time.Millisecond))

	var result struct {
		Total    int `json:"total"`
		Analyzed int `json:"analyzed"`
		Skipped  int `json:"skipped"`
		Failed   int `json:"failed"`
	}
	if err := json.Unmarshal(finished.Result, &result); err != nil {
		t.Fatal(err)
	}
	if result.Total != total {
		t.Fatalf("total = %d, want %d", result.Total, total)
	}
	if result.Analyzed != decodable {
		t.Fatalf("analyzed = %d, want %d", result.Analyzed, decodable)
	}
	if result.Failed != corrupt+unsupported+absent {
		t.Fatalf("failed = %d, want %d", result.Failed, corrupt+unsupported+absent)
	}
	// Every track must be settled: a run that leaves rows behind is not
	// resumable, because the next run cannot tell finished from abandoned.
	if finished.ProgressCurrent != int64(total) {
		t.Fatalf("progress = %d/%d, want every track accounted for", finished.ProgressCurrent, total)
	}

	codes := map[string]int{}
	for i := 0; i < corrupt; i++ {
		codes[requireErrorCode(t, database, fmt.Sprintf("corrupt-%03d", i))]++
	}
	for i := 0; i < unsupported; i++ {
		codes[requireErrorCode(t, database, fmt.Sprintf("opus-%03d", i))]++
	}
	for i := 0; i < absent; i++ {
		codes[requireErrorCode(t, database, fmt.Sprintf("gone-%03d", i))]++
	}
	if codes["decode_failed"] != corrupt {
		t.Errorf("decode_failed = %d, want %d", codes["decode_failed"], corrupt)
	}
	if codes["unsupported_codec"] != unsupported {
		t.Errorf("unsupported_codec = %d, want %d", codes["unsupported_codec"], unsupported)
	}
	if codes["source_unavailable"] != absent {
		t.Errorf("source_unavailable = %d, want %d", codes["source_unavailable"], absent)
	}

	// A second identical run must skip everything, including the failures, so a
	// poison file is not re-decoded on every pass.
	if err := database.CreateJob(db.Job{ID: "rerun", Type: JobTypeAnalyzeTracks, Status: db.JobStatusQueued,
		Parameters: json.RawMessage(`{"mode":"all"}`)}); err != nil {
		t.Fatal(err)
	}
	api.wakeJobScheduler()
	rerun := awaitLongJobStatus(t, database, "rerun", db.JobStatusSucceeded, time.Minute)
	var second struct {
		Skipped  int `json:"skipped"`
		Analyzed int `json:"analyzed"`
	}
	if err := json.Unmarshal(rerun.Result, &second); err != nil {
		t.Fatal(err)
	}
	// The missing-source tracks cannot be fingerprinted, so they are retried
	// rather than skipped. Everything that could be settled must be skipped.
	if second.Analyzed != 0 {
		t.Fatalf("second run analyzed %d tracks, want 0", second.Analyzed)
	}
	if second.Skipped != decodable+corrupt+unsupported {
		t.Fatalf("second run skipped %d, want %d", second.Skipped, decodable+corrupt+unsupported)
	}
}

func requireErrorCode(t *testing.T, database *db.DB, songID string) string {
	t.Helper()
	record, err := database.GetTrackAnalysis(songID)
	if err != nil {
		t.Fatalf("%s: %v", songID, err)
	}
	if record.ErrorCode == nil {
		t.Fatalf("%s: failure was persisted without an error code: %#v", songID, record)
	}
	return *record.ErrorCode
}

func awaitLongJobStatus(t *testing.T, database *db.DB, id, want string, timeout time.Duration) db.Job {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var last db.Job
	for time.Now().Before(deadline) {
		job, err := database.GetJob(id)
		if err == nil {
			last = job
			if job.Status == want {
				return job
			}
			if job.Status == db.JobStatusFailed {
				t.Fatalf("job %s failed: %s / %s", id, job.ErrorCode, job.ErrorMessage)
			}
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("job %s did not reach %q within %s: %#v", id, want, timeout, last)
	return last
}
