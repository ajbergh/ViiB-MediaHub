package track

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"testing"

	"github.com/ajbergh/viib-mediahub/internal/analysis"
	"github.com/ajbergh/viib-mediahub/internal/analysis/features"
	"github.com/ajbergh/viib-mediahub/internal/analysis/threeband"
	"github.com/ajbergh/viib-mediahub/internal/analysis/waveformartifact"
	"github.com/ajbergh/viib-mediahub/internal/db"
	spotifyanalysis "github.com/ajbergh/viib-mediahub/internal/spotify/analysis"
)

func TestHistoricalProviderBandClaimRequiresAdmission(t *testing.T) {
	for _, scenario := range []string{"unavailable", "shared-repair", "still-available"} {
		t.Run(scenario, func(t *testing.T) {
			database, ids := runnerCatalog(t, 1)
			provider := &spotifyanalysis.Observation{ProviderThreeBandAvailable: true}
			if _, err := Run(t.Context(), database, analysis.NewDefaultDecoderRegistry(), ids, RunOptions{SpotifyFeatures: func(context.Context, analysis.ResolvedSource) *spotifyanalysis.Observation { return provider }}); err != nil {
				t.Fatal(err)
			}
			record, err := database.GetTrackAnalysis(ids[0])
			if err != nil {
				t.Fatal(err)
			}
			if scenario == "shared-repair" {
				if err := database.DeleteTrackAnalysisArtifactForSource(ids[0], record.SourceFingerprint, waveformartifact.Kind, waveformartifact.FormatVersion, waveformartifact.AlgorithmVersion); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := database.ReserveSpotifyMetadataRuntime("restart"); err != nil {
				t.Fatal(err)
			}
			selected, err := ExpandPreparationJobSelection(t.Context(), database, db.AnalysisSelection{Mode: db.AnalysisSelectionMissing}, nil)
			if err != nil || len(selected) != 1 {
				t.Fatalf("provider claim hid missing artifact: %v %v", selected, err)
			}
			opens, lookups := 0, 0
			options := RunOptions{ResolveSource: func(_ context.Context, id string) (analysis.ResolvedSource, error) {
				source, err := analysis.ResolveLocalSource(database, id)
				open := source.Open
				source.OpenStream = func() (io.ReadCloser, error) { opens++; return open() }
				return source, err
			}, SpotifyFeatures: func(context.Context, analysis.ResolvedSource) *spotifyanalysis.Observation {
				lookups++
				if scenario == "still-available" {
					return provider
				}
				return nil
			}}
			progress, err := Run(t.Context(), database, analysis.NewDefaultDecoderRegistry(), selected, options)
			if err != nil || lookups != 1 {
				t.Fatalf("admission lookup: %+v opens=%d lookups=%d err=%v", progress, opens, lookups, err)
			}
			artifact, err := database.GetTrackAnalysisArtifact(ids[0], threeband.Kind, threeband.FormatVersion, threeband.AlgorithmVersion)
			states, stateErr := database.GetTrackCapabilityStatuses(ids[0], record.SourceFingerprint)
			if stateErr != nil {
				t.Fatal(stateErr)
			}
			if scenario == "still-available" {
				if opens != 0 || progress.Skipped != 1 || !errors.Is(err, sql.ErrNoRows) || states[threeband.Kind].Reason != "provider_three_band" {
					t.Fatalf("admitted provider duplicated local bands: %+v opens=%d err=%v", progress, opens, err)
				}
			} else {
				if opens != 1 || progress.Analyzed != 1 || err != nil || artifact.SourceFingerprint != record.SourceFingerprint || states[threeband.Kind].State != "available" || states[threeband.Kind].Reason != "" {
					t.Fatalf("local fallback lost: %+v opens=%d status=%+v err=%v", progress, opens, states[threeband.Kind], err)
				}
				selected, err = ExpandPreparationJobSelection(t.Context(), database, db.AnalysisSelection{Mode: db.AnalysisSelectionMissing}, nil)
				if err != nil || len(selected) != 0 {
					t.Fatalf("local fallback not settled: %v %v", selected, err)
				}
			}
			if _, err := Run(t.Context(), database, analysis.NewDefaultDecoderRegistry(), ids, options); err != nil || opens > 1 {
				t.Fatalf("repeated decode: opens=%d err=%v", opens, err)
			}
		})
	}
}

func TestProviderBandSelectionPublishesDuringEnrichmentAndSharedRepair(t *testing.T) {
	for _, scenario := range []string{"enrichment", "shared-repair"} {
		t.Run(scenario, func(t *testing.T) {
			database, ids := runnerCatalog(t, 1)
			if _, err := Run(t.Context(), database, analysis.NewDefaultDecoderRegistry(), ids, RunOptions{}); err != nil {
				t.Fatal(err)
			}
			record, err := database.GetTrackAnalysis(ids[0])
			if err != nil {
				t.Fatal(err)
			}
			if scenario == "shared-repair" {
				if err := database.DeleteTrackAnalysisArtifactForSource(ids[0], record.SourceFingerprint, features.BS1770ArtifactKind, features.BS1770FormatVersion, features.BS1770AlgorithmVersion); err != nil {
					t.Fatal(err)
				}
			}
			opens := 0
			progress, err := Run(t.Context(), database, analysis.NewDefaultDecoderRegistry(), ids, RunOptions{EnrichValid: true, ResolveSource: func(_ context.Context, id string) (analysis.ResolvedSource, error) {
				source, err := analysis.ResolveLocalSource(database, id)
				open := source.Open
				source.OpenStream = func() (io.ReadCloser, error) { opens++; return open() }
				return source, err
			}, SpotifyFeatures: func(context.Context, analysis.ResolvedSource) *spotifyanalysis.Observation {
				return &spotifyanalysis.Observation{ProviderThreeBandAvailable: true}
			}})
			wantOpens := 0
			if scenario == "shared-repair" {
				wantOpens = 1
			}
			if err != nil || progress.Analyzed != 1 || opens != wantOpens {
				t.Fatalf("provider selection: %+v opens=%d err=%v", progress, opens, err)
			}
			if _, err := database.GetTrackAnalysisArtifact(ids[0], threeband.Kind, threeband.FormatVersion, threeband.AlgorithmVersion); !errors.Is(err, sql.ErrNoRows) {
				t.Fatal("duplicate local output retained")
			}
			states, err := database.GetTrackCapabilityStatuses(ids[0], record.SourceFingerprint)
			if err != nil || states[threeband.Kind].Reason != "provider_three_band" || states[threeband.Kind].State != "available" {
				t.Fatalf("provider choice not published: %+v %v", states, err)
			}
		})
	}
}
