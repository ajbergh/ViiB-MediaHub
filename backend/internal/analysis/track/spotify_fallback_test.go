package track

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/analysis"
	"github.com/ajbergh/viib-mediahub/internal/analysis/features"
	"github.com/ajbergh/viib-mediahub/internal/db"
	spotifyanalysis "github.com/ajbergh/viib-mediahub/internal/spotify/analysis"
)

// Seed the same current, fully prepared shape as the live library: provider
// timing, local energy/structure, and no local timing estimate. A new runtime
// then withholds the private provider bindings until ownership is confirmed.
func restartedProviderTrack(t *testing.T, legacy bool) (*db.DB, []string, spotifyanalysis.Observation) {
	t.Helper()
	database, ids := runnerCatalog(t, 1)
	if _, err := database.ReserveSpotifyMetadataRuntime("first"); err != nil {
		t.Fatal(err)
	}
	if _, err := database.ConfirmSpotifyMetadataOwner("first", "webplayer", "account", "context"); err != nil {
		t.Fatal(err)
	}
	bpm, tonic, mode := 109.724, 9, 0
	observation := spotifyanalysis.Observation{TrackID: strings.Repeat("A", 22), SourceEndpoint: "audio_features", AccountContext: "context", RetrievedAt: time.Now(), BPM: &bpm, Key: &tonic, Mode: &mode}
	progress, err := Run(t.Context(), database, analysis.NewDefaultDecoderRegistry(), ids, RunOptions{SpotifyFeatures: func(context.Context, analysis.ResolvedSource) *spotifyanalysis.Observation { return &observation }})
	if err != nil || progress.Analyzed != 1 {
		t.Fatalf("seed: %+v %v", progress, err)
	}
	record, err := database.GetTrackAnalysis(ids[0])
	if err != nil || record.Local == nil || record.Local.BPM != nil || record.Local.KeyTonic != nil {
		t.Fatalf("seed local timing: %+v %v", record.Local, err)
	}
	if legacy {
		record.Local.TempoAttempted, record.Local.KeyAttempted = nil, nil
		if err := database.UpsertTrackAnalysis(record); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := database.ReserveSpotifyMetadataRuntime("restarted"); err != nil {
		t.Fatal(err)
	}
	return database, ids, observation
}

func TestRestartedSpotifyTimingFallsBackOnce(t *testing.T) {
	for _, legacy := range []bool{false, true} {
		for _, provider := range []string{"unavailable", "bpm-only", "key-only"} {
			name := provider
			if legacy {
				name += "-legacy"
			}
			t.Run(name, func(t *testing.T) {
				database, ids, observation := restartedProviderTrack(t, legacy)
				before, err := database.GetTrackAnalysisArtifact(ids[0], features.ArtifactKind, features.FormatVersion, features.AlgorithmVersion)
				if err != nil {
					t.Fatal(err)
				}
				selection := db.AnalysisSelection{Mode: db.AnalysisSelectionMissing}
				selected, err := ExpandPreparationJobSelection(t.Context(), database, selection, nil)
				if err != nil || len(selected) != 1 {
					t.Fatalf("missing selection: %v %v", selected, err)
				}
				observation.AccountContext, observation.DurableImport = "", true
				if provider == "bpm-only" {
					observation.Key, observation.Mode = nil, nil
				} else if provider == "key-only" {
					observation.BPM = nil
				}
				opens := 0
				options := RunOptions{EnrichValid: true, ResolveSource: func(_ context.Context, id string) (analysis.ResolvedSource, error) {
					source, err := analysis.ResolveLocalSource(database, id)
					open := source.Open
					source.OpenStream = func() (io.ReadCloser, error) { opens++; return open() }
					return source, err
				}, SpotifyFeatures: func(context.Context, analysis.ResolvedSource) *spotifyanalysis.Observation {
					if provider == "unavailable" {
						return nil
					}
					return &observation
				}}
				progress, err := Run(t.Context(), database, analysis.NewDefaultDecoderRegistry(), selected, options)
				if err != nil || progress.Analyzed != 1 || opens != 1 {
					t.Fatalf("fallback: %+v opens=%d err=%v", progress, opens, err)
				}
				after, err := database.GetTrackAnalysis(ids[0])
				if err != nil || after.Local == nil || after.Local.TempoAttempted == nil || after.Local.KeyAttempted == nil || *after.Local.TempoAttempted != (provider != "bpm-only") || *after.Local.KeyAttempted != (provider != "key-only") {
					t.Fatalf("attempt markers: %+v %v", after.Local, err)
				}
				if bpm := db.ResolveEffectiveBPM(db.EffectiveBPMInputs{Analysis: &after}); bpm.Value == nil || (provider != "bpm-only" && bpm.Source != "measured") {
					t.Fatalf("missing fallback BPM: %+v", bpm)
				}
				retained, err := database.GetTrackAnalysisArtifact(ids[0], features.ArtifactKind, features.FormatVersion, features.AlgorithmVersion)
				if err != nil || !bytes.Equal(before.Data, retained.Data) {
					t.Fatal("scalar repair replaced structure")
				}
				selected, err = ExpandPreparationJobSelection(t.Context(), database, selection, nil)
				if err != nil || len(selected) != 0 {
					t.Fatalf("settled timing selected again: %v %v", selected, err)
				}
				if _, err := Run(t.Context(), database, analysis.NewDefaultDecoderRegistry(), ids, options); err != nil || opens != 1 {
					t.Fatalf("settled timing decoded again: opens=%d err=%v", opens, err)
				}
			})
		}
	}
}

func TestProviderRecoveryAvoidsTimingRepairDecode(t *testing.T) {
	database, ids, observation := restartedProviderTrack(t, false)
	observation.AccountContext, observation.DurableImport = "", true
	opens, lookups := 0, 0
	progress, err := Run(t.Context(), database, analysis.NewDefaultDecoderRegistry(), ids, RunOptions{
		ResolveSource: func(_ context.Context, id string) (analysis.ResolvedSource, error) {
			source, err := analysis.ResolveLocalSource(database, id)
			open := source.Open
			source.OpenStream = func() (io.ReadCloser, error) { opens++; return open() }
			return source, err
		}, SpotifyFeatures: func(context.Context, analysis.ResolvedSource) *spotifyanalysis.Observation {
			lookups++
			return &observation
		},
	})
	if err != nil || progress.Skipped != 1 || opens != 0 || lookups != 1 {
		t.Fatalf("recovery: %+v opens=%d lookups=%d err=%v", progress, opens, lookups, err)
	}
	selected, err := ExpandPreparationJobSelection(t.Context(), database, db.AnalysisSelection{Mode: db.AnalysisSelectionMissing}, nil)
	if err != nil || len(selected) != 0 {
		t.Fatalf("recovered provider selected again: %v %v", selected, err)
	}
}

func TestLegacyLocalAbstentionRemainsSettled(t *testing.T) {
	record := db.TrackAnalysis{Local: &db.LocalScalarObservation{}}
	if needsLocalTiming(record, nil) {
		t.Fatal("legacy local abstention treated as provider skip")
	}
	record.Local.TempoAttempted, record.Local.KeyAttempted = ptr(true), ptr(true)
	record.BPMSource, record.KeySource = ptr("spotify"), ptr("spotify")
	if needsLocalTiming(record, nil) {
		t.Fatal("explicit local abstention treated as provider skip")
	}
}

func TestTimingRepairProviderRecoveryRechecksSource(t *testing.T) {
	database, ids, observation := restartedProviderTrack(t, false)
	observation.AccountContext, observation.DurableImport = "", true
	resolutions := 0
	progress, err := Run(t.Context(), database, analysis.NewDefaultDecoderRegistry(), ids, RunOptions{
		ResolveSource: func(_ context.Context, id string) (analysis.ResolvedSource, error) {
			source, err := analysis.ResolveLocalSource(database, id)
			resolutions++
			if resolutions > 1 {
				source.Fingerprint = "changed"
			}
			return source, err
		}, SpotifyFeatures: func(context.Context, analysis.ResolvedSource) *spotifyanalysis.Observation { return &observation },
	})
	if err != nil || progress.Failed != 1 {
		t.Fatalf("source change accepted: %+v %v", progress, err)
	}
	record, err := database.GetTrackAnalysis(ids[0])
	if err != nil || record.Status != db.TrackAnalysisPending || record.SpotifyBindings.BPM.Durable {
		t.Fatalf("stale provider publication: %+v %v", record, err)
	}
}

func TestTimingReuseDoesNotBypassInvalidEnvelopeRepair(t *testing.T) {
	database, ids := runnerCatalog(t, 1)
	if _, err := Run(t.Context(), database, analysis.NewDefaultDecoderRegistry(), ids, RunOptions{}); err != nil {
		t.Fatal(err)
	}
	record, err := database.GetTrackAnalysis(ids[0])
	if err != nil {
		t.Fatal(err)
	}
	record.Local.AlgorithmVersion = "old"
	if err := database.UpsertTrackAnalysis(record); err != nil {
		t.Fatal(err)
	}
	progress, err := Run(t.Context(), database, analysis.NewDefaultDecoderRegistry(), ids, RunOptions{})
	if err != nil || progress.Analyzed != 1 {
		t.Fatalf("invalid envelope reused: %+v %v", progress, err)
	}
	record, err = database.GetTrackAnalysis(ids[0])
	if err != nil || record.Local.AlgorithmVersion != AlgorithmVersion {
		t.Fatalf("invalid envelope retained: %+v %v", record.Local, err)
	}
}
