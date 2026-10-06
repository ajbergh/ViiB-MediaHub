package track

import (
	"context"
	"errors"
	"github.com/ajbergh/viib-mediahub/internal/analysis"
	"github.com/ajbergh/viib-mediahub/internal/analysis/features"
	"github.com/ajbergh/viib-mediahub/internal/analysis/waveformartifact"
	"github.com/ajbergh/viib-mediahub/internal/db"
	spotifyanalysis "github.com/ajbergh/viib-mediahub/internal/spotify/analysis"
	"io"
	"reflect"
	"testing"
)

func TestPreparationRepairsMultipleMissingCapabilitiesInOneDecode(t *testing.T) {
	database, ids := runnerCatalog(t, 1)
	registry := analysis.NewDefaultDecoderRegistry()
	if _, err := Run(t.Context(), database, registry, ids, RunOptions{}); err != nil {
		t.Fatal(err)
	}
	before, _ := database.GetTrackAnalysis(ids[0])
	wave, _ := database.GetTrackAnalysisArtifact(ids[0], waveformartifact.Kind, waveformartifact.FormatVersion, waveformartifact.AlgorithmVersion)
	cues, _ := database.GetDJHotCues(ids[0])
	for _, a := range []struct {
		kind      string
		version   int
		algorithm string
	}{{features.ArtifactKind, features.FormatVersion, features.AlgorithmVersion}, {features.BS1770ArtifactKind, features.BS1770FormatVersion, features.BS1770AlgorithmVersion}} {
		if err := database.DeleteTrackAnalysisArtifact(ids[0], a.kind, a.version, a.algorithm); err != nil {
			t.Fatal(err)
		}
	}
	before.EnergyLevel = nil
	before.EnergyLevelConfidence = nil
	before.EnergyAlgorithmVersion = nil
	if err := database.UpsertTrackAnalysis(before); err != nil {
		t.Fatal(err)
	}
	selected, err := database.ExpandAnalysisSelection(db.AnalysisSelection{Mode: db.AnalysisSelectionMissing}, AnalysisVersion, AlgorithmVersion)
	if err != nil || len(selected) != 1 {
		t.Fatalf("missing preparation not selected: %v %v", selected, err)
	}
	opens := 0
	options := RunOptions{ResolveSource: func(ctx context.Context, id string) (analysis.ResolvedSource, error) {
		s, e := analysis.ResolveLocalSource(database, id)
		open := s.Open
		s.OpenStream = func() (io.ReadCloser, error) { opens++; return open() }
		return s, e
	}}
	progress, err := Run(t.Context(), database, registry, selected, options)
	if err != nil || progress.Analyzed != 1 || opens != 1 {
		t.Fatalf("repair %+v %v opens %d", progress, err, opens)
	}
	after, _ := database.GetTrackAnalysis(ids[0])
	if after.EnergyLevel == nil {
		t.Fatal("energy not repaired")
	}
	after.EnergyLevel = nil
	after.EnergyLevelConfidence = nil
	after.EnergyAlgorithmVersion = nil
	if !reflect.DeepEqual(before, after) {
		t.Fatal("unrelated scalar observations changed")
	}
	retained, _ := database.GetTrackAnalysisArtifact(ids[0], waveformartifact.Kind, waveformartifact.FormatVersion, waveformartifact.AlgorithmVersion)
	if !reflect.DeepEqual(wave, retained) {
		t.Fatal("current amplitude artifact rewritten")
	}
	afterCues, _ := database.GetDJHotCues(ids[0])
	if !reflect.DeepEqual(cues, afterCues) {
		t.Fatal("existing cues changed")
	}
	states, err := database.GetTrackCapabilityStatuses(ids[0], before.SourceFingerprint)
	if err != nil || states["local_features"].State != "available" || states["local_loudness"].State != "available" || states["core_preparation"].State != "available" {
		t.Fatalf("capabilities not settled: %v %v", states, err)
	}
	selected, err = database.ExpandAnalysisSelection(db.AnalysisSelection{Mode: db.AnalysisSelectionMissing}, AnalysisVersion, AlgorithmVersion)
	if err != nil || len(selected) != 0 {
		t.Fatalf("repeat selection: %v %v", selected, err)
	}
	progress, err = Run(t.Context(), database, registry, ids, options)
	if err != nil || progress.Skipped != 1 || opens != 1 {
		t.Fatal("settled preparation decoded again")
	}
}

func TestProviderScalarsDoNotCauseRepeatedFailedLocalPreparation(t *testing.T) {
	database, ids := runnerCatalog(t, 1)
	opens := 0
	bpm, tonic, mode := 130.0, 9, 0
	options := RunOptions{ResolveSource: func(ctx context.Context, id string) (analysis.ResolvedSource, error) {
		s, e := analysis.ResolveLocalSource(database, id)
		s.OpenStream = func() (io.ReadCloser, error) { opens++; return nil, errors.New("temporary decode error") }
		return s, e
	}, SpotifyFeatures: func(context.Context, analysis.ResolvedSource) *spotifyanalysis.Observation {
		return &spotifyanalysis.Observation{BPM: &bpm, Key: &tonic, Mode: &mode}
	}}
	registry := analysis.NewDefaultDecoderRegistry()
	if p, err := Run(t.Context(), database, registry, ids, options); err != nil || p.Analyzed != 1 {
		t.Fatalf("initial provider success: %+v %v", p, err)
	}
	row, _ := database.GetTrackAnalysis(ids[0])
	if row.BPM == nil || *row.BPM != bpm {
		t.Fatal("local failure lost provider scalar")
	}
	states, err := database.GetTrackCapabilityStatuses(ids[0], row.SourceFingerprint)
	if err != nil || states["local_amplitude"].State != "failed" || states["local_amplitude"].RetryAt == 0 {
		t.Fatal("failure retry not retained")
	}
	selected, err := database.ExpandAnalysisSelection(db.AnalysisSelection{Mode: db.AnalysisSelectionMissing}, AnalysisVersion, AlgorithmVersion)
	if err != nil || len(selected) != 0 {
		t.Fatalf("retry suppression: %v %v", selected, err)
	}
	if p, err := Run(t.Context(), database, registry, ids, options); err != nil || p.Skipped != 1 || opens != 1 {
		t.Fatalf("repeated decode: %+v %v opens=%d", p, err, opens)
	}
	expired := []db.TrackCapabilityStatus{}
	for _, s := range states {
		s.RetryAt = 1
		s.UpdatedAt++
		expired = append(expired, s)
	}
	if err := database.PutTrackCapabilityStatuses(expired); err != nil {
		t.Fatal(err)
	}
	selected, err = database.ExpandAnalysisSelection(db.AnalysisSelection{Mode: db.AnalysisSelectionMissing}, AnalysisVersion, AlgorithmVersion)
	if err != nil || len(selected) != 1 {
		t.Fatal("expired failure not selected")
	}
	if _, err := Run(t.Context(), database, registry, selected, options); err != nil || opens != 2 {
		t.Fatal("expired failure not retried")
	}
}
