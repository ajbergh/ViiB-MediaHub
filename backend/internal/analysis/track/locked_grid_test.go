package track

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"github.com/ajbergh/viib-mediahub/internal/analysis"
	"github.com/ajbergh/viib-mediahub/internal/analysis/beatgrid"
	"github.com/ajbergh/viib-mediahub/internal/db"
	"io"
	"testing"
)

func TestLockedInvalidGridSettlesWithoutDecodingOrOverwriting(t *testing.T) {
	for _, scenario := range []string{"missing", "corrupt", "stale", "unbound"} {
		t.Run(scenario, func(t *testing.T) {
			database, ids := runnerCatalog(t, 1)
			registry := analysis.NewDefaultDecoderRegistry()
			if _, err := Run(t.Context(), database, registry, ids, RunOptions{}); err != nil {
				t.Fatal(err)
			}
			original, err := database.GetTrackAnalysisArtifact(ids[0], beatgrid.ArtifactKind, beatgrid.FormatVersion, beatgrid.AlgorithmVersion)
			if err != nil {
				t.Fatal(err)
			}
			changed := original
			switch scenario {
			case "missing":
				if err := database.DeleteTrackAnalysisArtifact(ids[0], beatgrid.ArtifactKind, beatgrid.FormatVersion, beatgrid.AlgorithmVersion); err != nil {
					t.Fatal(err)
				}
			case "corrupt":
				changed.Data = []byte("corrupt")
			case "stale":
				changed.SourceFingerprint = "old"
			case "unbound":
				changed.SourceFingerprint = ""
			}
			if scenario != "missing" {
				if err := database.UpsertTrackAnalysisArtifact(changed); err != nil {
					t.Fatal(err)
				}
			}
			if err := database.UpsertTrackAnalysisOverride(db.TrackAnalysisOverride{SongID: ids[0], BeatgridLocked: true, BeatgridArtifactID: &original.ID}); err != nil {
				t.Fatal(err)
			}
			selected, err := ExpandPreparationSelection(t.Context(), database, db.AnalysisSelection{Mode: db.AnalysisSelectionMissing}, nil)
			if err != nil || len(selected) != 1 {
				t.Fatal(selected, err)
			}
			opens := 0
			resolve := func(ctx context.Context, id string) (analysis.ResolvedSource, error) {
				s, err := analysis.ResolveLocalSource(database, id)
				s.OpenStream = func() (io.ReadCloser, error) { opens++; return nil, errors.New("must not decode") }
				return s, err
			}
			if _, err := Run(t.Context(), database, registry, selected, RunOptions{ResolveSource: resolve}); err != nil || opens != 0 {
				t.Fatal(err, opens)
			}
			source, err := analysis.ResolveLocalSource(database, ids[0])
			if err != nil {
				t.Fatal(err)
			}
			states, err := database.GetTrackCapabilityStatuses(ids[0], source.Fingerprint)
			if err != nil || states["local_beatgrid"].State != "unavailable" {
				t.Fatal(states, err)
			}
			stored, err := database.GetTrackAnalysisArtifact(ids[0], beatgrid.ArtifactKind, beatgrid.FormatVersion, beatgrid.AlgorithmVersion)
			if scenario == "missing" {
				if !errors.Is(err, sql.ErrNoRows) {
					t.Fatal(err)
				}
			} else if err != nil || !bytes.Equal(stored.Data, changed.Data) || stored.SourceFingerprint != changed.SourceFingerprint {
				t.Fatal("locked artifact overwritten", stored, err)
			}
			selected, err = ExpandPreparationSelection(t.Context(), database, db.AnalysisSelection{Mode: db.AnalysisSelectionMissing}, nil)
			if err != nil || len(selected) != 0 {
				t.Fatal("unresolved lock did not settle", selected, err)
			}
			override, err := database.GetTrackAnalysisOverride(ids[0])
			if err != nil || !override.BeatgridLocked {
				t.Fatal(override, err)
			}
		})
	}
}

func TestSourceReplacementPreparationKeepsManualGridButMarksItUnavailable(t *testing.T) {
	database, ids := runnerCatalog(t, 1)
	registry := analysis.NewDefaultDecoderRegistry()
	if _, err := Run(t.Context(), database, registry, ids, RunOptions{}); err != nil {
		t.Fatal(err)
	}
	original, err := database.GetTrackAnalysisArtifact(ids[0], beatgrid.ArtifactKind, beatgrid.FormatVersion, beatgrid.AlgorithmVersion)
	if err != nil {
		t.Fatal(err)
	}
	original.Provenance = "manual"
	if err := database.UpsertTrackAnalysisArtifact(original); err != nil {
		t.Fatal(err)
	}
	if err := database.UpsertTrackAnalysisOverride(db.TrackAnalysisOverride{SongID: ids[0], BeatgridLocked: true, BeatgridArtifactID: &original.ID}); err != nil {
		t.Fatal(err)
	}
	song, err := database.GetSongByID(ids[0])
	if err != nil {
		t.Fatal(err)
	}
	song.FileHash = "replacement-source-identity"
	if err := database.SaveSong(song); err != nil {
		t.Fatal(err)
	}
	if progress, err := Run(t.Context(), database, registry, ids, RunOptions{}); err != nil || progress.Analyzed != 1 {
		t.Fatal(progress, err)
	}
	source, err := analysis.ResolveLocalSource(database, ids[0])
	if err != nil {
		t.Fatal(err)
	}
	stored, err := database.GetTrackAnalysisArtifact(ids[0], beatgrid.ArtifactKind, beatgrid.FormatVersion, beatgrid.AlgorithmVersion)
	if err != nil || stored.SourceFingerprint != original.SourceFingerprint || !bytes.Equal(stored.Data, original.Data) {
		t.Fatal("manual grid overwritten", err)
	}
	states, err := database.GetTrackCapabilityStatuses(ids[0], source.Fingerprint)
	if err != nil || states["local_beatgrid"].State != "unavailable" || states["local_beatgrid"].Reason != "locked_beatgrid_source_mismatch" {
		t.Fatal("generated grid mislabeled as persisted", states, err)
	}
	selected, err := ExpandPreparationSelection(t.Context(), database, db.AnalysisSelection{Mode: db.AnalysisSelectionMissing}, nil)
	if err != nil || len(selected) != 0 {
		t.Fatal("locked stale grid causes repeated work", selected, err)
	}
}
