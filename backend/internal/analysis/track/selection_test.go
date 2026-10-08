package track

import (
	"context"
	"errors"
	"github.com/ajbergh/viib-mediahub/internal/analysis"
	"github.com/ajbergh/viib-mediahub/internal/analysis/features"
	"github.com/ajbergh/viib-mediahub/internal/analysis/waveformartifact"
	"github.com/ajbergh/viib-mediahub/internal/db"
	"io"
	"os"
	"testing"
	"time"
)

func TestMissingDiscoveryFindsSizedCorruptionAndRepairsWithoutAudioInspection(t *testing.T) {
	database, ids := runnerCatalog(t, 2)
	registry := analysis.NewDefaultDecoderRegistry()
	if _, err := Run(t.Context(), database, registry, ids, RunOptions{}); err != nil {
		t.Fatal(err)
	}
	opens := 0
	resolve := func(ctx context.Context, id string) (analysis.ResolvedSource, error) {
		s, err := analysis.ResolveLocalSource(database, id)
		open := s.Open
		s.OpenStream = func() (io.ReadCloser, error) { opens++; return open() }
		return s, err
	}
	selection := db.AnalysisSelection{Mode: db.AnalysisSelectionMissing}
	selected, err := ExpandPreparationSelection(t.Context(), database, selection, resolve)
	if err != nil || len(selected) != 0 {
		t.Fatal(selected, err)
	}
	a, err := database.GetTrackAnalysisArtifact(ids[0], features.ArtifactKind, features.FormatVersion, features.AlgorithmVersion)
	if err != nil {
		t.Fatal(err)
	}
	a.Data = []byte("nonempty invalid gzip payload")
	if err := database.UpsertTrackAnalysisArtifact(a); err != nil {
		t.Fatal(err)
	}
	coarse, err := database.ExpandAnalysisSelection(selection, AnalysisVersion, AlgorithmVersion)
	if err != nil || len(coarse) != 0 {
		t.Fatal("fixture did not bypass coarse metadata checks", coarse, err)
	}
	selected, err = ExpandPreparationSelection(t.Context(), database, selection, resolve)
	if err != nil || len(selected) != 1 || selected[0] != ids[0] || opens != 0 {
		t.Fatal(selected, err, opens)
	}
	if progress, err := Run(t.Context(), database, registry, selected, RunOptions{ResolveSource: resolve}); err != nil || progress.Analyzed != 1 || opens != 1 {
		t.Fatal(progress, err, opens)
	}
	selected, err = ExpandPreparationSelection(t.Context(), database, selection, resolve)
	if err != nil || len(selected) != 0 || opens != 1 {
		t.Fatal("repair did not settle", selected, err, opens)
	}
}
func TestMissingDiscoveryFindsUnobservedLocalReplacement(t *testing.T) {
	database, ids := runnerCatalog(t, 1)
	if _, err := Run(t.Context(), database, analysis.NewDefaultDecoderRegistry(), ids, RunOptions{}); err != nil {
		t.Fatal(err)
	}
	song, err := database.GetSongByID(ids[0])
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(song.FilePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(song.FilePath, info.ModTime().Add(time.Second), info.ModTime().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	selected, err := ExpandPreparationSelection(t.Context(), database, db.AnalysisSelection{Mode: db.AnalysisSelectionMissing}, nil)
	if err != nil || len(selected) != 1 {
		t.Fatal(selected, err)
	}
}
func TestMissingDiscoveryCancellationAndLiveClaim(t *testing.T) {
	database, ids := runnerCatalog(t, 1)
	source, err := analysis.ResolveLocalSource(database, ids[0])
	if err != nil {
		t.Fatal(err)
	}
	token, ok, err := database.ClaimTrackAnalysisLease(ids[0], source.Fingerprint, AnalysisVersion, AlgorithmVersion)
	if err != nil || !ok {
		t.Fatal(ok, err)
	}
	defer database.ReleaseTrackAnalysisLease(ids[0], token)
	selected, err := ExpandPreparationSelection(t.Context(), database, db.AnalysisSelection{Mode: db.AnalysisSelectionMissing}, nil)
	if err != nil || len(selected) != 0 {
		t.Fatal("live claim selected", selected, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := ExpandPreparationSelection(ctx, database, db.AnalysisSelection{Mode: db.AnalysisSelectionMissing}, nil); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestDurableSelectionDefersClaimArrivingAfterDiscovery(t *testing.T) {
	database, ids := runnerCatalog(t, 1)
	selection := db.AnalysisSelection{Mode: db.AnalysisSelectionMissing}
	if _, err := ExpandPreparationJobSelection(t.Context(), database, selection, nil); err != nil {
		t.Fatal(err)
	}
	source, err := analysis.ResolveLocalSource(database, ids[0])
	if err != nil {
		t.Fatal(err)
	}
	token, ok, err := database.ClaimTrackAnalysisLease(ids[0], source.Fingerprint, AnalysisVersion, AlgorithmVersion)
	if err != nil || !ok {
		t.Fatal(ok, err)
	}
	if err := CheckPreparationSelectionClaims(t.Context(), database, selection); !errors.Is(err, ErrPreparationBusy) {
		t.Fatal(err)
	}
	if err := database.ReleaseTrackAnalysisLease(ids[0], token); err != nil {
		t.Fatal(err)
	}
	if err := CheckPreparationSelectionClaims(t.Context(), database, selection); err != nil {
		t.Fatal(err)
	}
}

func TestDurableClaimCheckDistinguishesEnrichmentFromMissingCore(t *testing.T) {
	database, ids := runnerCatalog(t, 1)
	if _, err := Run(t.Context(), database, analysis.NewDefaultDecoderRegistry(), ids, RunOptions{}); err != nil {
		t.Fatal(err)
	}
	source, err := analysis.ResolveLocalSource(database, ids[0])
	if err != nil {
		t.Fatal(err)
	}
	token, ok, err := database.ClaimTrackAnalysisLease(ids[0], source.Fingerprint, AnalysisVersion, AlgorithmVersion)
	if err != nil || !ok {
		t.Fatal(ok, err)
	}
	defer database.ReleaseTrackAnalysisLease(ids[0], token)
	selection := db.AnalysisSelection{Mode: db.AnalysisSelectionMissing}
	if err := CheckPreparationSelectionClaims(t.Context(), database, selection); err != nil {
		t.Fatal("settled core blocked by optional owner", err)
	}
	if err := database.DeleteTrackAnalysisArtifact(ids[0], waveformartifact.Kind, waveformartifact.FormatVersion, waveformartifact.AlgorithmVersion); err != nil {
		t.Fatal(err)
	}
	if err := CheckPreparationSelectionClaims(t.Context(), database, selection); !errors.Is(err, ErrPreparationBusy) {
		t.Fatal("unfinished core claim not deferred", err)
	}
}
