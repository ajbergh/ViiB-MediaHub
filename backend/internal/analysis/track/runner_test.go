package track

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ajbergh/viib-mediahub/internal/analysis"
	"github.com/ajbergh/viib-mediahub/internal/analysisbench"
	"github.com/ajbergh/viib-mediahub/internal/db"
)

// runnerCatalog registers count click-track songs and returns their IDs.
func runnerCatalog(t *testing.T, count int) (*db.DB, []string) {
	t.Helper()
	directory := t.TempDir()
	database, err := db.New(filepath.Join(directory, "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })

	fixture, err := analysisbench.NewClickTrack("clicks", 128, 4, 22050, 1)
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
	return database, ids
}

func TestRunAnalyzesEveryTrackOnce(t *testing.T) {
	database, ids := runnerCatalog(t, 3)
	progress, err := Run(context.Background(), database, analysis.NewDefaultDecoderRegistry(), ids, RunOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if progress.Total != 3 || progress.Processed != 3 || progress.Analyzed != 3 {
		t.Fatalf("progress = %#v, want 3 analyzed", progress)
	}
	if progress.Skipped != 0 || progress.Failed != 0 {
		t.Fatalf("progress = %#v, want no skips or failures", progress)
	}
	for _, id := range ids {
		record, err := database.GetTrackAnalysis(id)
		if err != nil {
			t.Fatalf("%s: %v", id, err)
		}
		if record.BPM == nil {
			t.Fatalf("%s: no measured BPM persisted", id)
		}
	}
}

// Already-valid tracks must be skipped, which is what keeps a re-run cheap and
// makes an interrupted run resumable.
func TestRunSkipsAlreadyValidTracks(t *testing.T) {
	database, ids := runnerCatalog(t, 3)
	registry := analysis.NewDefaultDecoderRegistry()
	if _, err := Run(context.Background(), database, registry, ids, RunOptions{}); err != nil {
		t.Fatal(err)
	}
	second, err := Run(context.Background(), database, registry, ids, RunOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if second.Skipped != 3 || second.Analyzed != 0 {
		t.Fatalf("progress = %#v, want 3 skipped and 0 analyzed", second)
	}
}

// A source that changed on disk invalidates its record even though the
// analyzer version is unchanged.
func TestRunReanalyzesWhenSourceChanges(t *testing.T) {
	database, ids := runnerCatalog(t, 1)
	registry := analysis.NewDefaultDecoderRegistry()
	if _, err := Run(context.Background(), database, registry, ids, RunOptions{}); err != nil {
		t.Fatal(err)
	}
	source, err := analysis.ResolveLocalSource(database, ids[0])
	if err != nil {
		t.Fatal(err)
	}
	longer, err := analysisbench.NewClickTrack("clicks", 128, 8, 22050, 1)
	if err != nil {
		t.Fatal(err)
	}
	var wav bytes.Buffer
	if err := analysisbench.WriteWAVPCM16(&wav, longer); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source.Path, wav.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	again, err := Run(context.Background(), database, registry, ids, RunOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if again.Analyzed != 1 || again.Skipped != 0 {
		t.Fatalf("progress = %#v, want the changed source re-analyzed", again)
	}
}

// Cancellation stops between tracks and leaves the outstanding work
// outstanding, so a later run re-dispatches only what is unfinished.
func TestRunStopsOnCancellationAndLeavesRemainingWork(t *testing.T) {
	database, ids := runnerCatalog(t, 4)
	registry := analysis.NewDefaultDecoderRegistry()

	processed := 0
	progress, err := Run(context.Background(), database, registry, ids, RunOptions{
		Canceled: func() bool { return processed >= 2 },
		Progress: func(RunProgress) { processed++ },
	})
	if err != context.Canceled {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if progress.Processed != 2 {
		t.Fatalf("progress = %#v, want 2 processed before cancellation", progress)
	}

	resumed, err := Run(context.Background(), database, registry, ids, RunOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if resumed.Skipped != 2 || resumed.Analyzed != 2 {
		t.Fatalf("resumed = %#v, want 2 skipped and only the 2 unfinished tracks analyzed", resumed)
	}
}

// A cancellation delivered through the context must not persist a track-level
// failure for work that was merely interrupted.
func TestRunCanceledContextDoesNotRecordTrackFailure(t *testing.T) {
	database, ids := runnerCatalog(t, 2)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	progress, err := Run(ctx, database, analysis.NewDefaultDecoderRegistry(), ids, RunOptions{})
	if err == nil {
		t.Fatal("expected a cancellation error")
	}
	if progress.Processed != 0 || progress.Failed != 0 {
		t.Fatalf("progress = %#v, want nothing counted", progress)
	}
	for _, id := range ids {
		if _, err := database.GetTrackAnalysis(id); err == nil {
			t.Fatalf("%s: cancellation must not persist a record", id)
		}
	}
}

// A poison source is recorded as a durable failure so the queue does not
// re-decode it forever, and it is counted as failed rather than analyzed.
func TestRunRecordsFailureForUndecodableSource(t *testing.T) {
	directory := t.TempDir()
	database, err := db.New(filepath.Join(directory, "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	corrupt := filepath.Join(directory, "corrupt.wav")
	if err := os.WriteFile(corrupt, []byte("RIFFnope"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := database.SaveSong(&db.Song{ID: "corrupt", Title: "Corrupt", Artist: "Artist", Album: "Album", FilePath: corrupt, AddedAt: 1}); err != nil {
		t.Fatal(err)
	}
	if err := database.SaveSong(&db.Song{ID: "missing", Title: "Missing", Artist: "Artist", Album: "Album", FilePath: filepath.Join(directory, "absent.wav"), AddedAt: 2}); err != nil {
		t.Fatal(err)
	}

	progress, err := Run(context.Background(), database, analysis.NewDefaultDecoderRegistry(), []string{"corrupt", "missing"}, RunOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if progress.Failed != 2 || progress.Analyzed != 0 {
		t.Fatalf("progress = %#v, want 2 failures", progress)
	}
	record, err := database.GetTrackAnalysis("corrupt")
	if err != nil {
		t.Fatal(err)
	}
	if record.Status != db.TrackAnalysisFailed || record.ErrorCode == nil || *record.ErrorCode != ErrorDecodeFailed {
		t.Fatalf("corrupt record = %#v, want a durable decode_failed", record)
	}
	// The unresolvable source is recorded too, so the run does not retry it.
	unavailable, err := database.GetTrackAnalysis("missing")
	if err != nil {
		t.Fatal(err)
	}
	if unavailable.ErrorCode == nil || *unavailable.ErrorCode != ErrorSourceUnavailable {
		t.Fatalf("missing record = %#v, want source_unavailable", unavailable)
	}

	// A settled failure against unchanged bytes is skipped on the next pass.
	again, err := Run(context.Background(), database, analysis.NewDefaultDecoderRegistry(), []string{"corrupt"}, RunOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if again.Skipped != 1 {
		t.Fatalf("progress = %#v, want the settled failure skipped", again)
	}
}

// Throttling is how DJ playback reduces analysis pressure. It must be consulted
// before each track and must be able to abort the run.
func TestRunConsultsThrottleBeforeEachTrack(t *testing.T) {
	database, ids := runnerCatalog(t, 3)
	calls := 0
	progress, err := Run(context.Background(), database, analysis.NewDefaultDecoderRegistry(), ids, RunOptions{
		Throttle: func(context.Context) error {
			calls++
			if calls > 2 {
				return context.Canceled
			}
			return nil
		},
	})
	if err != context.Canceled {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if calls != 3 || progress.Processed != 2 {
		t.Fatalf("calls = %d, progress = %#v, want the throttle consulted per track", calls, progress)
	}
}
