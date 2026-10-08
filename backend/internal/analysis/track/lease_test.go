package track

import (
	"context"
	"errors"
	"io"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/analysis"
	"github.com/ajbergh/viib-mediahub/internal/db"
	spotifyanalysis "github.com/ajbergh/viib-mediahub/internal/spotify/analysis"
)

func TestClaimHeartbeatRenewsAndCancelsOnOwnershipLoss(t *testing.T) {
	database, ids := runnerCatalog(t, 1)
	token, ok, err := database.ClaimTrackAnalysisLease(ids[0], "fp", AnalysisVersion, AlgorithmVersion)
	if err != nil || !ok {
		t.Fatal(ok, err)
	}
	first, err := database.GetTrackAnalysis(ids[0])
	if err != nil {
		t.Fatal(err)
	}
	ctx, stop := maintainTrackAnalysisLease(context.Background(), database, ids[0], "fp", token, time.Millisecond)
	defer stop()
	deadline := time.Now().Add(3 * time.Second)
	renewed := false
	for time.Now().Before(deadline) {
		got, err := database.GetTrackAnalysis(ids[0])
		if err != nil {
			t.Fatal(err)
		}
		if got.AnalyzedAt != nil && *got.AnalyzedAt > *first.AnalyzedAt {
			renewed = true
			break
		}
		time.Sleep(time.Millisecond)
	}
	if !renewed {
		t.Fatal("heartbeat did not renew")
	}
	if err := database.ReleaseTrackAnalysisLease(ids[0], token); err != nil {
		t.Fatal(err)
	}
	next, ok, err := database.ClaimTrackAnalysisLease(ids[0], "fp", AnalysisVersion, AlgorithmVersion)
	if err != nil || !ok {
		t.Fatal(ok, err)
	}
	select {
	case <-ctx.Done():
	case <-time.After(3 * time.Second):
		t.Fatal("lost ownership did not cancel work")
	}
	stop()
	stop()
	if err := database.RenewTrackAnalysisLease(ids[0], "fp", next); err != nil {
		t.Fatal("old heartbeat disturbed new claim", err)
	}
}

func TestClaimHeartbeatStopsWithParentCancellation(t *testing.T) {
	database, ids := runnerCatalog(t, 1)
	token, ok, err := database.ClaimTrackAnalysisLease(ids[0], "fp", AnalysisVersion, AlgorithmVersion)
	if err != nil || !ok {
		t.Fatal(ok, err)
	}
	parent, cancel := context.WithCancel(context.Background())
	ctx, stop := maintainTrackAnalysisLease(parent, database, ids[0], "fp", token, time.Hour)
	cancel()
	stop()
	if !errors.Is(ctx.Err(), context.Canceled) {
		t.Fatal(ctx.Err())
	}
	if err := database.ReleaseTrackAnalysisLease(ids[0], token); err != nil {
		t.Fatal(err)
	}
}

func TestStaleEnrichmentCannotRestoreOrPublishOverNewWorker(t *testing.T) {
	for _, returned := range []bool{false, true} {
		t.Run(map[bool]string{false: "unavailable-restore", true: "returned-publication"}[returned], func(t *testing.T) {
			database, ids := runnerCatalog(t, 1)
			source, err := analysis.ResolveLocalSource(database, ids[0])
			if err != nil {
				t.Fatal(err)
			}
			bpm := 120.0
			measured := "measured"
			original := db.TrackAnalysis{SongID: ids[0], Status: db.TrackAnalysisPartial, SourceFingerprint: source.Fingerprint, AnalysisVersion: AnalysisVersion, AlgorithmVersion: AlgorithmVersion, BPM: &bpm, BPMSource: &measured}
			if err := database.UpsertTrackAnalysis(original); err != nil {
				t.Fatal(err)
			}
			entered, release, done := make(chan struct{}), make(chan struct{}), make(chan outcome, 1)
			go func() {
				done <- enrichCurrentScalars(context.Background(), database, source, func(context.Context, analysis.ResolvedSource) *spotifyanalysis.Observation {
					close(entered)
					<-release
					if returned {
						value := 130.0
						return &spotifyanalysis.Observation{BPM: &value}
					}
					return nil
				}, func(context.Context, string) (analysis.ResolvedSource, error) { return source, nil })
			}()
			select {
			case <-entered:
			case <-time.After(3 * time.Second):
				t.Fatal("enrichment did not enter")
			}
			// Simulate source invalidation/recovery allowing a newer preparation generation.
			if err := database.RefreshTrackAnalysisSourceRevision(ids[0], "replacement-source"); err != nil {
				close(release)
				t.Fatal(err)
			}
			next, ok, err := database.ClaimTrackAnalysisLease(ids[0], source.Fingerprint, AnalysisVersion, AlgorithmVersion)
			if err != nil || !ok {
				close(release)
				t.Fatal(ok, err)
			}
			close(release)
			select {
			case got := <-done:
				if got != outcomeFailed {
					t.Fatal("stale enrichment succeeded", got)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("enrichment stalled")
			}
			if err := database.RenewTrackAnalysisLease(ids[0], source.Fingerprint, next); err != nil {
				t.Fatal("new worker overwritten/released", err)
			}
			got, err := database.GetTrackAnalysis(ids[0])
			if err != nil || got.BPM == nil || *got.BPM != 120 || got.Status != db.TrackAnalysisRunning {
				t.Fatal(got, err)
			}
		})
	}
}

func TestStaleWaveformFailureCannotPublishStatusOrRestoreScalars(t *testing.T) {
	database, ids := runnerCatalog(t, 1)
	source, err := analysis.ResolveLocalSource(database, ids[0])
	if err != nil {
		t.Fatal(err)
	}
	original := db.TrackAnalysis{SongID: ids[0], Status: db.TrackAnalysisPartial, SourceFingerprint: source.Fingerprint, AnalysisVersion: AnalysisVersion, AlgorithmVersion: AlgorithmVersion}
	if err := database.UpsertTrackAnalysis(original); err != nil {
		t.Fatal(err)
	}
	entered, release, done := make(chan struct{}), make(chan struct{}), make(chan outcome, 1)
	source.OpenStream = func() (io.ReadCloser, error) {
		close(entered)
		<-release
		return nil, errors.New("late decode failure")
	}
	go func() {
		done <- repairCurrentWaveform(context.Background(), database, analysis.NewDefaultDecoderRegistry(), source, original, func(context.Context, string) (analysis.ResolvedSource, error) { return source, nil })
	}()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("waveform repair did not enter")
	}
	if err := database.RefreshTrackAnalysisSourceRevision(ids[0], "replacement-source"); err != nil {
		close(release)
		t.Fatal(err)
	}
	next, ok, err := database.ClaimTrackAnalysisLease(ids[0], source.Fingerprint, AnalysisVersion, AlgorithmVersion)
	if err != nil || !ok {
		close(release)
		t.Fatal(ok, err)
	}
	close(release)
	select {
	case got := <-done:
		if got != outcomeFailed {
			t.Fatal(got)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("waveform repair stalled")
	}
	if err := database.RenewTrackAnalysisLease(ids[0], source.Fingerprint, next); err != nil {
		t.Fatal("new waveform worker overwritten/released", err)
	}
	states, err := database.GetTrackCapabilityStatuses(ids[0], source.Fingerprint)
	if err != nil || len(states) != 0 {
		t.Fatal("stale failure capability escaped", states, err)
	}
}

func TestPreparationWaitsForLazyWaveformWithoutSkippingScalarWork(t *testing.T) {
	database, ids := runnerCatalog(t, 1)
	source, err := analysis.ResolveLocalSource(database, ids[0])
	if err != nil {
		t.Fatal(err)
	}
	token, ok, err := database.ClaimWaveformLease(ids[0], source.Fingerprint)
	if err != nil || !ok {
		t.Fatal(ok, err)
	}
	done := make(chan struct{})
	var progress RunProgress
	var runErr error
	go func() {
		defer close(done)
		progress, runErr = Run(t.Context(), database, analysis.NewDefaultDecoderRegistry(), ids, RunOptions{})
	}()
	select {
	case <-done:
		t.Fatal("preparation skipped active lazy work")
	case <-time.After(100 * time.Millisecond):
	}
	if err := database.ReleaseWaveformLease(ids[0], token); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("preparation did not resume")
	}
	if runErr != nil || progress.Skipped != 0 || progress.Analyzed != 1 {
		t.Fatal(progress, runErr)
	}
	if _, err := database.GetTrackAnalysis(ids[0]); err != nil {
		t.Fatal("scalar work lost", err)
	}
}
func TestPreparationWaveformWaitIsCancelable(t *testing.T) {
	database, ids := runnerCatalog(t, 1)
	token, ok, err := database.ClaimWaveformLease(ids[0], "fp")
	if err != nil || !ok {
		t.Fatal(ok, err)
	}
	defer database.ReleaseWaveformLease(ids[0], token)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, _, err := claimPreparation(ctx, database, ids[0], "fp"); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestSourceUnavailableRetainsLastGoodAndAttemptCooldown(t *testing.T) {
	database, ids := runnerCatalog(t, 1)
	if _, err := Run(t.Context(), database, analysis.NewDefaultDecoderRegistry(), ids, RunOptions{}); err != nil {
		t.Fatal(err)
	}
	before, err := database.GetTrackAnalysis(ids[0])
	if err != nil {
		t.Fatal(err)
	}
	source, err := analysis.ResolveLocalSource(database, ids[0])
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(source.Path, source.Path+".away"); err != nil {
		t.Fatal(err)
	}
	defer os.Rename(source.Path+".away", source.Path)
	first, err := Run(t.Context(), database, analysis.NewDefaultDecoderRegistry(), ids, RunOptions{})
	if err != nil || first.Failed != 1 {
		t.Fatal(first, err)
	}
	after, err := database.GetTrackAnalysis(ids[0])
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("last-good scalars changed", err)
	}
	states, err := database.GetTrackCapabilityStatuses(ids[0], before.SourceFingerprint)
	if err != nil {
		t.Fatal(err)
	}
	selected, selectionErr := database.ExpandAnalysisSelection(db.AnalysisSelection{Mode: db.AnalysisSelectionMissing}, AnalysisVersion, AlgorithmVersion)
	if selectionErr != nil || len(selected) != 0 {
		t.Fatal("cooldown entered missing queue", selected, selectionErr)
	}
	attempt := states["core_preparation"]
	if attempt.Reason != ErrorSourceUnavailable || attempt.RetryAt <= time.Now().UnixMilli() {
		t.Fatal(attempt)
	}
	again, err := Run(t.Context(), database, analysis.NewDefaultDecoderRegistry(), ids, RunOptions{})
	if err != nil || again.Skipped != 1 {
		t.Fatal(again, err)
	}
	retained, err := database.GetTrackCapabilityStatuses(ids[0], before.SourceFingerprint)
	if err != nil || !reflect.DeepEqual(states, retained) {
		t.Fatal("cooldown rewritten", err)
	}
	expired := []db.TrackCapabilityStatus{}
	for _, state := range retained {
		state.RetryAt = time.Now().Add(-time.Minute).UnixMilli()
		state.UpdatedAt = time.Now().Add(time.Millisecond).UnixMilli()
		expired = append(expired, state)
	}
	if err := database.PutTrackCapabilityStatuses(expired); err != nil {
		t.Fatal(err)
	}
	selected, selectionErr = database.ExpandAnalysisSelection(db.AnalysisSelection{Mode: db.AnalysisSelectionMissing}, AnalysisVersion, AlgorithmVersion)
	if selectionErr != nil || len(selected) != 1 || selected[0] != ids[0] {
		t.Fatal("expired source attempt not selected", selected, selectionErr)
	}
	retry, retryErr := Run(t.Context(), database, analysis.NewDefaultDecoderRegistry(), ids, RunOptions{})
	if retryErr != nil || retry.Failed != 1 {
		t.Fatal("expired attempt not retried", retry, retryErr)
	}
	final, err := database.GetTrackAnalysis(ids[0])
	if err != nil || !reflect.DeepEqual(before, final) {
		t.Fatal("retry lost last-good scalars", err)
	}

}
