package track

import (
	"bytes"
	"context"
	"errors"
	"github.com/ajbergh/viib-mediahub/internal/analysis"
	"github.com/ajbergh/viib-mediahub/internal/analysis/features"
	"github.com/ajbergh/viib-mediahub/internal/db"
	"github.com/ajbergh/viib-mediahub/internal/logger"
	spotifyanalysis "github.com/ajbergh/viib-mediahub/internal/spotify/analysis"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSpotifyEnrichesValidLocalAnalysisWithoutDecodingOrLosingArtifacts(t *testing.T) {
	for _, scenario := range []string{"key-only", "unavailable", "canceled"} {
		t.Run(scenario, func(t *testing.T) {
			logDirectory := t.TempDir()
			if err := logger.Init(logDirectory); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(logger.Close)
			database, ids := runnerCatalog(t, 1)
			if _, err := Run(t.Context(), database, analysis.NewDefaultDecoderRegistry(), ids, RunOptions{}); err != nil {
				t.Fatal(err)
			}
			before, _ := database.GetTrackAnalysis(ids[0])
			artifact, err := database.GetTrackAnalysisArtifact(ids[0], features.ArtifactKind, features.FormatVersion, features.AlgorithmVersion)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			opens := 0
			options := RunOptions{EnrichValid: true,
				ResolveSource: func(ctx context.Context, id string) (analysis.ResolvedSource, error) {
					source, err := analysis.ResolveLocalSource(database, id)
					source.OpenStream = func() (io.ReadCloser, error) { opens++; return nil, errors.New("must reuse local result") }
					return source, err
				},
				SpotifyFeatures: func(context.Context, analysis.ResolvedSource) *spotifyanalysis.Observation {
					if scenario == "canceled" {
						cancel()
						return nil
					}
					if scenario == "unavailable" {
						return nil
					}
					tonic, mode := 0, 1
					return &spotifyanalysis.Observation{Key: &tonic, Mode: &mode}
				},
			}
			progress, err := Run(ctx, database, analysis.NewDefaultDecoderRegistry(), ids, options)
			contents, readErr := os.ReadFile(filepath.Join(logDirectory, "scan.log"))
			if readErr != nil {
				t.Fatal(readErr)
			}
			if !strings.Contains(string(contents), "local_engine=\"reused\"") {
				t.Fatalf("missing reused analysis decision: %s", contents)
			}
			if scenario == "key-only" && !strings.Contains(string(contents), "action=\"spotify_enriched\"") {
				t.Fatalf("missing enrichment decision: %s", contents)
			}
			if scenario == "canceled" {
				if !errors.Is(err, context.Canceled) {
					t.Fatal(err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			if opens != 0 {
				t.Fatal("valid local audio decoded again")
			}
			after, err := database.GetTrackAnalysis(ids[0])
			if err != nil {
				t.Fatal(err)
			}
			if after.BPM == nil || *after.BPM != *before.BPM || *after.BPMSource != "measured" {
				t.Fatal("local BPM lost")
			}
			if scenario == "key-only" {
				if progress.Analyzed != 1 || *after.KeySource != "spotify" || *after.KeyMode != "major" {
					t.Fatalf("enrichment: %+v %+v", progress, after)
				}
			} else if after.Status != before.Status {
				t.Fatal("failed lookup lost prior status")
			}
			retained, err := database.GetTrackAnalysisArtifact(ids[0], features.ArtifactKind, features.FormatVersion, features.AlgorithmVersion)
			if err != nil || !bytes.Equal(artifact.Data, retained.Data) {
				t.Fatal("local artifacts lost")
			}
		})
	}
}

func TestSpotifyFirstAndLocalFallback(t *testing.T) {
	for _, scenario := range []string{"complete", "key-only", "bpm-only", "unavailable", "canceled"} {
		t.Run(scenario, func(t *testing.T) {
			logDirectory := t.TempDir()
			if err := logger.Init(logDirectory); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(logger.Close)
			database, ids := runnerCatalog(t, 1)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			opens := 0
			bpm, tonic, mode := 109.724, 9, 0 // A minor
			options := RunOptions{
				ResolveSource: func(ctx context.Context, id string) (analysis.ResolvedSource, error) {
					source, err := analysis.ResolveLocalSource(database, id)
					open := source.Open
					source.OpenStream = func() (io.ReadCloser, error) { opens++; return open() }
					return source, err
				},
				SpotifyFeatures: func(context.Context, analysis.ResolvedSource) *spotifyanalysis.Observation {
					if scenario == "unavailable" {
						return nil
					}
					if scenario == "canceled" {
						cancel()
						return nil
					}
					o := &spotifyanalysis.Observation{BPM: &bpm, Key: &tonic, Mode: &mode}
					if scenario == "key-only" {
						o.BPM = nil
					}
					if scenario == "bpm-only" {
						o.Key = nil
					}
					return o
				},
			}
			progress, err := Run(ctx, database, analysis.NewDefaultDecoderRegistry(), ids, options)
			contents, readErr := os.ReadFile(filepath.Join(logDirectory, "scan.log"))
			if readErr != nil {
				t.Fatal(readErr)
			}
			logText := string(contents)
			expected := map[string]string{"complete": "reason=\"spotify_complete\"", "key-only": "reason=\"spotify_missing_bpm\"", "bpm-only": "reason=\"spotify_missing_key\"", "unavailable": "reason=\"spotify_unavailable\"", "canceled": "analysis_canceled"}[scenario]
			if !strings.Contains(logText, expected) {
				t.Fatalf("missing decision %q: %s", expected, logText)
			}
			if scenario == "complete" && (!strings.Contains(logText, "bpm_source=\"spotify\"") || !strings.Contains(logText, "key_source=\"spotify\"") || !strings.Contains(logText, "local_engine=\"skipped\"")) {
				t.Fatalf("provider result log: %s", logText)
			}
			if scenario == "key-only" && (!strings.Contains(logText, "bpm_source=\"local\"") || !strings.Contains(logText, "key_source=\"spotify\"") || !strings.Contains(logText, "local_engine=\"run\"")) {
				t.Fatalf("mixed result log: %s", logText)
			}
			if scenario == "canceled" {
				if !errors.Is(err, context.Canceled) || opens != 0 {
					t.Fatalf("cancel: %v, opens %d", err, opens)
				}
				record, _ := database.GetTrackAnalysis(ids[0])
				if record.Status != db.TrackAnalysisPending {
					t.Fatalf("claim not released: %+v", record)
				}
				return
			}
			if err != nil || progress.Analyzed != 1 {
				t.Fatalf("run: %+v %v", progress, err)
			}
			record, err := database.GetTrackAnalysis(ids[0])
			if err != nil {
				t.Fatal(err)
			}
			if scenario == "complete" && opens != 0 {
				t.Fatal("complete provider result decoded audio")
			}
			if scenario != "complete" && opens == 0 {
				t.Fatal("missing provider dimension did not decode audio")
			}
			if scenario == "complete" || scenario == "bpm-only" {
				if record.BPM == nil || *record.BPM != bpm || *record.BPMSource != "spotify" || record.BPMConfidence != nil {
					t.Fatalf("provider BPM: %+v", record)
				}
			} else if record.BPM == nil || *record.BPMSource != "measured" {
				t.Fatalf("local BPM: %+v", record)
			}
			if scenario == "complete" || scenario == "key-only" {
				if record.KeyMode == nil || *record.KeyMode != "minor" || *record.KeyTonic != 9 || *record.KeySource != "spotify" || *record.CamelotKey != "8A" || record.KeyConfidence != nil {
					t.Fatalf("provider key: %+v", record)
				}
			}
		})
	}
}
