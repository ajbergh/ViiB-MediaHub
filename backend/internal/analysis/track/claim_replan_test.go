package track

import (
	"context"
	"errors"
	"io"
	"reflect"
	"testing"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/analysis"
	"github.com/ajbergh/viib-mediahub/internal/analysis/threeband"
	"github.com/ajbergh/viib-mediahub/internal/analysis/waveformartifact"
	"github.com/ajbergh/viib-mediahub/internal/db"
	spotifyanalysis "github.com/ajbergh/viib-mediahub/internal/spotify/analysis"
)

func TestClaimReplansAfterAnotherWorkerSettlesRepair(t *testing.T) {
	for _, kind := range []string{threeband.Kind, waveformartifact.Kind, "failed-cooldown"} {
		t.Run(kind, func(t *testing.T) {
			database, ids := runnerCatalog(t, 1)
			registry := analysis.NewDefaultDecoderRegistry()
			if _, err := Run(t.Context(), database, registry, ids, RunOptions{}); err != nil {
				t.Fatal(err)
			}
			source, err := analysis.ResolveLocalSource(database, ids[0])
			if err != nil {
				t.Fatal(err)
			}
			old, err := database.GetTrackAnalysis(ids[0])
			if err != nil {
				t.Fatal(err)
			}
			artifactKind, format, version := threeband.Kind, threeband.FormatVersion, threeband.AlgorithmVersion
			if kind == waveformartifact.Kind {
				artifactKind, format, version = waveformartifact.Kind, waveformartifact.FormatVersion, waveformartifact.AlgorithmVersion
			}
			if err := database.DeleteTrackAnalysisArtifactForSource(ids[0], source.Fingerprint, artifactKind, format, version); err != nil {
				t.Fatal(err)
			}
			stalePlan, err := missingPreparation(database, source, old)
			if err != nil || !stalePlan[artifactKind] {
				t.Fatal("missing initial repair", stalePlan, err)
			}
			// Worker A finishes after B has formed its repair plan, before B claims.
			if kind == "failed-cooldown" {
				old.Status = db.TrackAnalysisFailed
				code := ErrorDecodeFailed
				old.ErrorCode = &code
				analyzedAt := int64(12345)
				old.AnalyzedAt = &analyzedAt
				states := preparationStatuses(Result{SongID: ids[0], Source: source, PreparationError: code})
				if err := database.PublishTrackPreparation(db.TrackPreparationPublication{Analysis: old, Capabilities: states}); err != nil {
					t.Fatal(err)
				}
			} else {
				if progress, err := Run(t.Context(), database, registry, ids, RunOptions{}); err != nil || progress.Analyzed != 1 {
					t.Fatal(progress, err)
				}
			}
			latest, err := database.GetTrackAnalysis(ids[0])
			if err != nil {
				t.Fatal(err)
			}
			statesBefore, err := database.GetTrackCapabilityStatuses(ids[0], source.Fingerprint)
			if err != nil {
				t.Fatal(err)
			}
			token, snapshot, claimed, err := claimPreparationWithSnapshot(t.Context(), database, ids[0], source.Fingerprint)
			if err != nil || !claimed || snapshot == nil {
				t.Fatal(claimed, err)
			}
			freshPlan, err := claimedPreparationMissing(database, source, snapshot)
			if err != nil || freshPlan == nil || len(freshPlan) != 0 {
				t.Fatal("stale repair survived claim", freshPlan, err)
			}
			if got := settleClaimedPreparation(t.Context(), database, source, *snapshot, token, func(context.Context, string) (analysis.ResolvedSource, error) { return source, nil }); got != outcomeSkipped {
				t.Fatal(got)
			}
			got, err := database.GetTrackAnalysis(ids[0])
			if err != nil || !reflect.DeepEqual(got, latest) {
				t.Fatalf("new projection overwritten: %+v want=%+v err=%v", got, latest, err)
			}
			statesAfter, err := database.GetTrackCapabilityStatuses(ids[0], source.Fingerprint)
			if err != nil {
				t.Fatal(err)
			}
			if kind == "failed-cooldown" && !reflect.DeepEqual(statesBefore, statesAfter) {
				t.Fatal("cooldown changed", statesBefore, statesAfter)
			}
			opens := 0
			source.OpenStream = func() (io.ReadCloser, error) { opens++; t.Fatal("settled repair decoded again"); return nil, nil }
			if progress, err := Run(t.Context(), database, registry, ids, RunOptions{ResolveSource: func(context.Context, string) (analysis.ResolvedSource, error) { return source, nil }}); err != nil || progress.Skipped != 1 || opens != 0 {
				t.Fatal(progress, opens, err)
			}
			if kind == "failed-cooldown" && statesAfter[artifactKind].RetryAt <= time.Now().UnixMilli() {
				t.Fatal("cooldown lost")
			}
		})
	}
}

func TestClaimSnapshotDoesNotAdmitOldSourceOrAlgorithm(t *testing.T) {
	database, ids := runnerCatalog(t, 1)
	source, err := analysis.ResolveLocalSource(database, ids[0])
	if err != nil {
		t.Fatal(err)
	}
	for _, record := range []db.TrackAnalysis{
		{Status: db.TrackAnalysisPartial, SourceFingerprint: "old-source", AnalysisVersion: AnalysisVersion, AlgorithmVersion: AlgorithmVersion},
		{Status: db.TrackAnalysisPartial, SourceFingerprint: source.Fingerprint, AnalysisVersion: AnalysisVersion, AlgorithmVersion: "old-algorithm"},
		{Status: db.TrackAnalysisRunning, SourceFingerprint: source.Fingerprint, AnalysisVersion: AnalysisVersion, AlgorithmVersion: AlgorithmVersion},
	} {
		plan, err := claimedPreparationMissing(database, source, &record)
		if err != nil || plan != nil {
			t.Fatal("invalid prior projection admitted", plan, err)
		}
	}
}

func TestWaveformRepairUsesClaimSnapshotInsteadOfCallerProjection(t *testing.T) {
	database, ids := runnerCatalog(t, 1)
	registry := analysis.NewDefaultDecoderRegistry()
	if _, err := Run(t.Context(), database, registry, ids, RunOptions{}); err != nil {
		t.Fatal(err)
	}
	source, err := analysis.ResolveLocalSource(database, ids[0])
	if err != nil {
		t.Fatal(err)
	}
	stale, err := database.GetTrackAnalysis(ids[0])
	if err != nil {
		t.Fatal(err)
	}
	latest := stale
	bpm := 99.0
	latest.BPM = &bpm
	analyzedAt := int64(12345)
	latest.AnalyzedAt = &analyzedAt
	if err := database.UpsertTrackAnalysis(latest); err != nil {
		t.Fatal(err)
	}
	latest, err = database.GetTrackAnalysis(ids[0])
	if err != nil {
		t.Fatal(err)
	}
	source.OpenStream = func() (io.ReadCloser, error) { t.Fatal("current waveform decoded again"); return nil, nil }
	if got := repairCurrentWaveform(t.Context(), database, registry, source, stale, func(context.Context, string) (analysis.ResolvedSource, error) { return source, nil }); got != outcomeSkipped {
		t.Fatal(got)
	}
	got, err := database.GetTrackAnalysis(ids[0])
	if err != nil || !reflect.DeepEqual(got, latest) {
		t.Fatalf("stale caller replaced current scalars: %+v want=%+v err=%v", got, latest, err)
	}
}

func TestSettledClaimStillChecksSourceBeforePublication(t *testing.T) {
	database, ids := runnerCatalog(t, 1)
	if _, err := Run(t.Context(), database, analysis.NewDefaultDecoderRegistry(), ids, RunOptions{}); err != nil {
		t.Fatal(err)
	}
	source, err := analysis.ResolveLocalSource(database, ids[0])
	if err != nil {
		t.Fatal(err)
	}
	token, snapshot, claimed, err := claimPreparationWithSnapshot(t.Context(), database, ids[0], source.Fingerprint)
	if err != nil || !claimed || snapshot == nil {
		t.Fatal(claimed, err)
	}
	defer database.ReleaseTrackAnalysisLease(ids[0], token)
	changed := source
	changed.Fingerprint = "changed"
	if got := settleClaimedPreparation(t.Context(), database, source, *snapshot, token, func(context.Context, string) (analysis.ResolvedSource, error) { return changed, nil }); got != outcomeFailed {
		t.Fatal(got)
	}
	if err := database.RenewTrackAnalysisLease(ids[0], source.Fingerprint, token); err != nil {
		t.Fatal("source failure consumed claim", err)
	}
}

func TestDelayedSourceResolutionCannotClaimOverNewPublication(t *testing.T) {
	for _, scenario := range []string{"preparation", "source-unavailable", "enrichment", "waveform"} {
		t.Run(scenario, func(t *testing.T) {
			database, ids := runnerCatalog(t, 1)
			source, err := analysis.ResolveLocalSource(database, ids[0])
			if err != nil {
				t.Fatal(err)
			}
			old := db.TrackAnalysis{SongID: ids[0], Status: db.TrackAnalysisPartial, SourceFingerprint: source.Fingerprint, AnalysisVersion: AnalysisVersion, AlgorithmVersion: AlgorithmVersion}
			if err := database.UpsertTrackAnalysis(old); err != nil {
				t.Fatal(err)
			}
			latest := old
			latest.SourceFingerprint = "replacement-source"
			bpm := 123.0
			latest.BPM = &bpm
			published := false
			resolve := func(context.Context, string) (analysis.ResolvedSource, error) {
				if !published {
					if err := database.UpsertTrackAnalysis(latest); err != nil {
						t.Fatal(err)
					}
					latest, err = database.GetTrackAnalysis(ids[0])
					if err != nil {
						t.Fatal(err)
					}
					published = true
				}
				if scenario == "source-unavailable" {
					return analysis.ResolvedSource{}, errors.New("source unavailable")
				}
				return source, nil
			}
			source.OpenStream = func() (io.ReadCloser, error) { t.Fatal("stale source decoded"); return nil, nil }
			var got outcome
			switch scenario {
			case "enrichment":
				got = enrichCurrentScalars(t.Context(), database, source, func(context.Context, analysis.ResolvedSource) *spotifyanalysis.Observation {
					t.Fatal("stale source called provider")
					return nil
				}, resolve)
			case "waveform":
				got = repairCurrentWaveform(t.Context(), database, analysis.NewDefaultDecoderRegistry(), source, old, resolve)
			default:
				got = analyzeOne(t.Context(), database, analysis.NewDefaultDecoderRegistry(), ids[0], Options{}, db.AutomaticCuePointsOff, resolve, nil, false)
			}
			if got != outcomeSkipped {
				t.Fatal("stale work did not skip", got)
			}
			record, err := database.GetTrackAnalysis(ids[0])
			if err != nil || !reflect.DeepEqual(record, latest) {
				t.Fatal("newer source projection mutated", record, latest, err)
			}
			states, err := database.GetTrackCapabilityStatuses(ids[0], latest.SourceFingerprint)
			if err != nil || len(states) != 0 {
				t.Fatal("stale attempt published", states, err)
			}
		})
	}
}
