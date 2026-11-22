// Tests preparation discovery across batches, artifact repairs, and cooperative throttling.
package track

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/analysis"
	"github.com/ajbergh/viib-mediahub/internal/analysis/features"
	"github.com/ajbergh/viib-mediahub/internal/analysis/waveformartifact"
	"github.com/ajbergh/viib-mediahub/internal/analysisbench"
	"github.com/ajbergh/viib-mediahub/internal/db"
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

func syntheticSettledCatalog(tb testing.TB, count int) (*db.DB, []string) {
	tb.Helper()
	directory := tb.TempDir()
	database, err := db.New(filepath.Join(directory, "library.db"))
	if err != nil {
		tb.Fatal(err)
	}
	tb.Cleanup(func() { database.Close() })

	fixture, err := analysisbench.NewClickTrack("clicks", 128, 3, 22050, 1)
	if err != nil {
		tb.Fatal(err)
	}
	var wav bytes.Buffer
	if err := analysisbench.WriteWAVPCM16(&wav, fixture); err != nil {
		tb.Fatal(err)
	}
	wavBytes := wav.Bytes()

	tmplID := "template-0000"
	tmplPath := filepath.Join(directory, tmplID+".wav")
	if err := os.WriteFile(tmplPath, wavBytes, 0600); err != nil {
		tb.Fatal(err)
	}
	if err := database.SaveSong(&db.Song{ID: tmplID, Title: tmplID, Artist: "Artist", Album: "Album", FilePath: tmplPath, AddedAt: 1}); err != nil {
		tb.Fatal(err)
	}
	if _, err := Run(tb.Context(), database, analysis.NewDefaultDecoderRegistry(), []string{tmplID}, RunOptions{}); err != nil {
		tb.Fatal(err)
	}
	tmplRecord, err := database.GetTrackAnalysis(tmplID)
	if err != nil {
		tb.Fatal(err)
	}
	tmplStates, err := database.GetTrackCapabilityStatuses(tmplID, tmplRecord.SourceFingerprint)
	if err != nil {
		tb.Fatal(err)
	}
	tmplArtifacts, err := database.GetTrackAnalysisArtifactsBounded(tmplID, preparationArtifactSpecs())
	if err != nil {
		tb.Fatal(err)
	}

	ids := make([]string, 0, count)
	for i := 0; i < count; i++ {
		id := fmt.Sprintf("track-%04d", i)
		path := filepath.Join(directory, id+".wav")
		if err := os.WriteFile(path, wavBytes, 0600); err != nil {
			tb.Fatal(err)
		}
		if err := database.SaveSong(&db.Song{ID: id, Title: id, Artist: "Artist", Album: "Album", FilePath: path, AddedAt: int64(i + 2)}); err != nil {
			tb.Fatal(err)
		}
		src, err := analysis.ResolveLocalSource(database, id)
		if err != nil {
			tb.Fatal(err)
		}
		rec := tmplRecord
		rec.SongID = id
		rec.SourceFingerprint = src.Fingerprint
		size := src.Size
		mtime := src.Mtime
		rec.SourceSize = &size
		rec.SourceMtime = &mtime
		if tmplRecord.Local != nil {
			localCopy := *tmplRecord.Local
			localCopy.SourceFingerprint = src.Fingerprint
			localCopy.Fields = append([]db.SpotifyScalarField(nil), tmplRecord.Local.Fields...)
			rec.Local = &localCopy
		}
		clonedStates := make([]db.TrackCapabilityStatus, 0, len(tmplStates))
		for _, s := range tmplStates {
			s.SongID = id
			s.SourceFingerprint = src.Fingerprint
			clonedStates = append(clonedStates, s)
		}
		clonedArtifacts := make([]db.TrackAnalysisArtifact, 0, len(tmplArtifacts))
		for _, lookup := range tmplArtifacts {
			if lookup.Err != nil {
				continue
			}
			a := lookup.Artifact
			a.ID = id + ":" + a.AlgorithmVersion
			a.SongID = id
			a.SourceFingerprint = src.Fingerprint
			clonedArtifacts = append(clonedArtifacts, a)
		}
		if err := database.PublishTrackPreparation(db.TrackPreparationPublication{
			Analysis:     rec,
			Artifacts:    clonedArtifacts,
			Capabilities: clonedStates,
		}); err != nil {
			tb.Fatal(err)
		}
		ids = append(ids, id)
	}
	return database, ids
}

func TestExpandPreparationSelectionBatchedAcrossChunks(t *testing.T) {
	const total = 650
	database, ids := syntheticSettledCatalog(t, total)

	// Verify baseline settled state returns nothing across multiple batches.
	selection := db.AnalysisSelection{Mode: db.AnalysisSelectionMissing}
	selected, err := ExpandPreparationSelection(t.Context(), database, selection, nil)
	if err != nil || len(selected) != 0 {
		t.Fatalf("settled catalog selected %v err=%v", selected, err)
	}

	// Introduce corrupt sized artifacts across chunk boundaries (indices 10, 305, 640).
	corruptIDs := []string{ids[10], ids[305], ids[640]}
	for _, id := range corruptIDs {
		a, err := database.GetTrackAnalysisArtifact(id, features.ArtifactKind, features.FormatVersion, features.AlgorithmVersion)
		if err != nil {
			t.Fatal(err)
		}
		a.Data = []byte("nonempty invalid gzip payload")
		if err := database.UpsertTrackAnalysisArtifact(a); err != nil {
			t.Fatal(err)
		}
	}

	// Introduce missing files (indices 15, 310).
	missingFileIDs := []string{ids[15], ids[310]}
	for _, id := range missingFileIDs {
		song, err := database.GetSongByID(id)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Remove(song.FilePath); err != nil {
			t.Fatal(err)
		}
	}

	// Introduce unanalyzed track (index 645) by adding a new song without track_analysis.
	newID := "track-new-0001"
	song0, err := database.GetSongByID(ids[0])
	if err != nil {
		t.Fatal(err)
	}
	newPath := filepath.Join(filepath.Dir(song0.FilePath), newID+".wav")
	wavData, err := os.ReadFile(song0.FilePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(newPath, wavData, 0600); err != nil {
		t.Fatal(err)
	}
	if err := database.SaveSong(&db.Song{ID: newID, Title: newID, Artist: "Artist", Album: "Album", FilePath: newPath, AddedAt: 9999}); err != nil {
		t.Fatal(err)
	}

	// Introduce a live claim on an unfinished track (index 300) -> skipped by discovery, caught by claim check.
	claimedID := ids[300]
	if err := database.DeleteTrackAnalysisArtifact(claimedID, waveformartifact.Kind, waveformartifact.FormatVersion, waveformartifact.AlgorithmVersion); err != nil {
		t.Fatal(err)
	}
	claimedSrc, err := analysis.ResolveLocalSource(database, claimedID)
	if err != nil {
		t.Fatal(err)
	}
	token, ok, err := database.ClaimTrackAnalysisLease(claimedID, claimedSrc.Fingerprint, AnalysisVersion, AlgorithmVersion)
	if err != nil || !ok {
		t.Fatalf("claim failed: ok=%v err=%v", ok, err)
	}

	selected, err = ExpandPreparationSelection(t.Context(), database, selection, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{
		corruptIDs[0]:     true,
		corruptIDs[1]:     true,
		corruptIDs[2]:     true,
		missingFileIDs[0]: true,
		missingFileIDs[1]: true,
		newID:             true,
	}
	if len(selected) != len(want) {
		t.Fatalf("selected %d tracks %v, want %d tracks %v", len(selected), selected, len(want), want)
	}
	for _, id := range selected {
		if !want[id] {
			t.Fatalf("unexpected selected track %q", id)
		}
	}

	if err := CheckPreparationSelectionClaims(t.Context(), database, selection); !errors.Is(err, ErrPreparationBusy) {
		t.Fatalf("expected ErrPreparationBusy, got %v", err)
	}
	if err := database.ReleaseTrackAnalysisLease(claimedID, token); err != nil {
		t.Fatal(err)
	}
	if err := CheckPreparationSelectionClaims(t.Context(), database, selection); err != nil {
		t.Fatalf("unexpected claim check error after release: %v", err)
	}
}

func TestExpandPreparationJobSelectionWithOptionsYieldsMidDiscoveryOnThrottle(t *testing.T) {
	const total = 650
	database, _ := syntheticSettledCatalog(t, total)
	errDeferred := errors.New("deferred mid-discovery")

	// Case 1: custom resolver triggers throttle mid-batch.
	throttled := false
	resolved := 0
	_, err := ExpandPreparationJobSelectionWithOptions(t.Context(), database, db.AnalysisSelection{Mode: db.AnalysisSelectionMissing}, SelectionOptions{
		ResolveSource: func(ctx context.Context, id string) (analysis.ResolvedSource, error) {
			resolved++
			if resolved >= 5 {
				throttled = true
			}
			return analysis.ResolveLocalSource(database, id)
		},
		Throttle: func(context.Context) error {
			if throttled {
				return errDeferred
			}
			return nil
		},
	})
	if !errors.Is(err, errDeferred) {
		t.Fatalf("got %v, want %v", err, errDeferred)
	}
	if resolved != 5 {
		t.Fatalf("resolved %d tracks before yielding, want 5", resolved)
	}

	// Case 2: nil resolver yields between 300-track chunks.
	checks := 0
	_, err = ExpandPreparationJobSelectionWithOptions(t.Context(), database, db.AnalysisSelection{Mode: db.AnalysisSelectionMissing}, SelectionOptions{
		Throttle: func(context.Context) error {
			checks++
			if checks >= 4 {
				return errDeferred
			}
			return nil
		},
	})
	if !errors.Is(err, errDeferred) {
		t.Fatalf("chunked throttle got %v, want %v", err, errDeferred)
	}
}

func BenchmarkExpandPreparationSelectionMissing1000(b *testing.B) {
	database, _ := syntheticSettledCatalog(b, 1000)
	selection := db.AnalysisSelection{Mode: db.AnalysisSelectionMissing}
	selected, err := ExpandPreparationSelection(b.Context(), database, selection, nil)
	if err != nil || len(selected) != 0 {
		b.Fatalf("setup invalid: selected=%v err=%v", selected, err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		got, err := ExpandPreparationSelection(b.Context(), database, selection, nil)
		if err != nil || len(got) != 0 {
			b.Fatalf("expand failed: got=%v err=%v", got, err)
		}
	}
}

func BenchmarkExpandPreparationSelectionMissing1000WithResolver(b *testing.B) {
	database, _ := syntheticSettledCatalog(b, 1000)
	selection := db.AnalysisSelection{Mode: db.AnalysisSelectionMissing}
	resolve := func(_ context.Context, id string) (analysis.ResolvedSource, error) {
		return analysis.ResolveLocalSource(database, id)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		got, err := ExpandPreparationSelection(b.Context(), database, selection, resolve)
		if err != nil || len(got) != 0 {
			b.Fatalf("expand failed: got=%v err=%v", got, err)
		}
	}
}

func BenchmarkCheckPreparationSelectionClaims1000(b *testing.B) {
	database, _ := syntheticSettledCatalog(b, 1000)
	selection := db.AnalysisSelection{Mode: db.AnalysisSelectionMissing}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := CheckPreparationSelectionClaims(b.Context(), database, selection); err != nil {
			b.Fatalf("claim check failed: %v", err)
		}
	}
}
