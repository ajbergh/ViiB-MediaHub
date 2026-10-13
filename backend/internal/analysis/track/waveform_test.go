package track

import (
	"bytes"
	"context"
	"io"
	"reflect"
	"testing"

	"github.com/ajbergh/viib-mediahub/internal/analysis"
	"github.com/ajbergh/viib-mediahub/internal/analysis/features"
	"github.com/ajbergh/viib-mediahub/internal/analysis/waveformartifact"
	"github.com/ajbergh/viib-mediahub/internal/db"
)

func TestWaveformOnlyRepairRetainsOtherPreparationAndSkipsNextPass(t *testing.T) {
	database, ids := runnerCatalog(t, 1)
	registry := analysis.NewDefaultDecoderRegistry()
	if _, err := Run(t.Context(), database, registry, ids, RunOptions{}); err != nil {
		t.Fatal(err)
	}
	source, err := analysis.ResolveLocalSource(database, ids[0])
	if err != nil {
		t.Fatal(err)
	}
	before, err := database.GetTrackAnalysis(ids[0])
	if err != nil {
		t.Fatal(err)
	}
	structure, _ := database.GetTrackAnalysisArtifact(ids[0], features.ArtifactKind, features.FormatVersion, features.AlgorithmVersion)
	loudness, _ := database.GetTrackAnalysisArtifact(ids[0], features.BS1770ArtifactKind, features.BS1770FormatVersion, features.BS1770AlgorithmVersion)
	cues, _ := database.GetDJHotCues(ids[0])
	wave, err := database.GetTrackAnalysisArtifact(ids[0], waveformartifact.Kind, waveformartifact.FormatVersion, waveformartifact.AlgorithmVersion)
	if err != nil {
		t.Fatal(err)
	}
	// An unbound legacy artifact must be selected and regenerated, never relabeled.
	wave.SourceFingerprint = ""
	if err = database.UpsertTrackAnalysisArtifact(wave); err != nil {
		t.Fatal(err)
	}
	selected, err := database.ExpandAnalysisSelection(db.AnalysisSelection{Mode: db.AnalysisSelectionMissing}, AnalysisVersion, AlgorithmVersion)
	if err != nil || len(selected) != 1 {
		t.Fatalf("missing waveform not selected: %v %v", selected, err)
	}
	opens := 0
	options := RunOptions{ResolveSource: func(ctx context.Context, id string) (analysis.ResolvedSource, error) {
		resolved, err := analysis.ResolveLocalSource(database, id)
		original := resolved.Open
		resolved.OpenStream = func() (io.ReadCloser, error) { opens++; return original() }
		return resolved, err
	}}
	progress, err := Run(t.Context(), database, registry, selected, options)
	if err != nil || progress.Analyzed != 1 || opens != 1 {
		t.Fatalf("repair: %+v %v opens=%d", progress, err, opens)
	}
	after, _ := database.GetTrackAnalysis(ids[0])
	if !reflect.DeepEqual(before, after) {
		t.Fatal("waveform repair changed scalar observations")
	}
	retained, _ := database.GetTrackAnalysisArtifact(ids[0], features.ArtifactKind, features.FormatVersion, features.AlgorithmVersion)
	if !bytes.Equal(structure.Data, retained.Data) || structure.CreatedAt != retained.CreatedAt {
		t.Fatal("structure recomputed")
	}
	retained, _ = database.GetTrackAnalysisArtifact(ids[0], features.BS1770ArtifactKind, features.BS1770FormatVersion, features.BS1770AlgorithmVersion)
	if !bytes.Equal(loudness.Data, retained.Data) || loudness.CreatedAt != retained.CreatedAt {
		t.Fatal("loudness recomputed")
	}
	afterCues, _ := database.GetDJHotCues(ids[0])
	if !reflect.DeepEqual(cues, afterCues) {
		t.Fatal("cues changed during waveform repair")
	}
	repaired, err := database.GetTrackAnalysisArtifact(ids[0], waveformartifact.Kind, waveformartifact.FormatVersion, waveformartifact.AlgorithmVersion)
	if err != nil || repaired.SourceFingerprint != source.Fingerprint {
		t.Fatal("waveform not bound to bytes")
	}
	selected, err = database.ExpandAnalysisSelection(db.AnalysisSelection{Mode: db.AnalysisSelectionMissing}, AnalysisVersion, AlgorithmVersion)
	if err != nil || len(selected) != 0 {
		t.Fatal("current waveform selected repeatedly")
	}
	progress, err = Run(t.Context(), database, registry, ids, options)
	if err != nil || progress.Skipped != 1 || opens != 1 {
		t.Fatal("current waveform decoded again")
	}
}

func TestSharedPreparationRejectsChangedSourceBeforePersistence(t *testing.T) {
	database, ids := runnerCatalog(t, 1)
	calls := 0
	options := RunOptions{ResolveSource: func(ctx context.Context, id string) (analysis.ResolvedSource, error) {
		source, err := analysis.ResolveLocalSource(database, id)
		calls++
		if calls > 1 {
			source.Fingerprint = "changed-source"
		}
		return source, err
	}}
	progress, err := Run(t.Context(), database, analysis.NewDefaultDecoderRegistry(), ids, options)
	if err != nil || progress.Failed != 1 {
		t.Fatalf("source fence: %+v %v", progress, err)
	}
	if _, err = database.GetTrackAnalysisArtifact(ids[0], waveformartifact.Kind, waveformartifact.FormatVersion, waveformartifact.AlgorithmVersion); err == nil {
		t.Fatal("stale source waveform published")
	}
	record, err := database.GetTrackAnalysis(ids[0])
	if err != nil || record.Status != db.TrackAnalysisPending {
		t.Fatal("source change failed to release claim")
	}
}
