package track

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/ajbergh/viib-mediahub/internal/analysis"
	"github.com/ajbergh/viib-mediahub/internal/analysis/features"
	"github.com/ajbergh/viib-mediahub/internal/analysis/threeband"
	"github.com/ajbergh/viib-mediahub/internal/analysis/waveformartifact"
	"github.com/ajbergh/viib-mediahub/internal/analysisbench"
	"github.com/ajbergh/viib-mediahub/internal/db"
	spotifyanalysis "github.com/ajbergh/viib-mediahub/internal/spotify/analysis"
	"io"
	"os"
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
	// Repaired energy/loudness observations carry new measurement timestamps;
	// compare the remaining scalar dimensions, including decoded duration.
	retainUnrelated := func(local *db.LocalScalarObservation) *db.LocalScalarObservation {
		if local == nil {
			return nil
		}
		copy := *local
		copy.Fields = nil
		for _, field := range local.Fields {
			switch field.Key {
			case "local_energy_level", "integrated_lufs_bs1770", "true_peak_dbtp":
				continue
			}
			copy.Fields = append(copy.Fields, field)
		}
		return &copy
	}
	before.Local = retainUnrelated(before.Local)
	after.Local = retainUnrelated(after.Local)
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

func TestMissingLocalEnergyEnvelopeFieldIsSelectedAndRepaired(t *testing.T) {
	database, ids := runnerCatalog(t, 1)
	registry := analysis.NewDefaultDecoderRegistry()
	if progress, err := Run(t.Context(), database, registry, ids, RunOptions{}); err != nil || progress.Analyzed != 1 {
		t.Fatalf("initial preparation: %+v %v", progress, err)
	}
	record, err := database.GetTrackAnalysis(ids[0])
	if err != nil {
		t.Fatal(err)
	}
	if record.EnergyLevel == nil || record.Local == nil || !record.LocalScalarEnvelopePresent {
		t.Fatalf("expected explicit local energy envelope: %+v", record)
	}
	fields := record.Local.Fields[:0]
	for _, field := range record.Local.Fields {
		if field.Key != "local_energy_level" {
			fields = append(fields, field)
		}
	}
	record.Local.Fields = fields
	if err := database.UpsertTrackAnalysis(record); err != nil {
		t.Fatal(err)
	}
	record, err = database.GetTrackAnalysis(ids[0])
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range db.ResolveAnalysisScalarFields(record, db.TrackAnalysisOverride{}, record.SourceFingerprint, nil) {
		if field.Key == "local_energy_level" && field.Selected != nil {
			t.Fatalf("legacy energy columns bypassed the incomplete explicit envelope: %+v", field)
		}
	}
	selected, err := ExpandPreparationSelection(t.Context(), database, db.AnalysisSelection{Mode: db.AnalysisSelectionMissing}, nil)
	if err != nil || len(selected) != 1 || selected[0] != ids[0] {
		t.Fatalf("incomplete explicit local energy was not selected: %v %v", selected, err)
	}

	opens := 0
	options := RunOptions{ResolveSource: func(ctx context.Context, id string) (analysis.ResolvedSource, error) {
		source, err := analysis.ResolveLocalSource(database, id)
		open := source.Open
		source.OpenStream = func() (io.ReadCloser, error) { opens++; return open() }
		return source, err
	}}
	if progress, err := Run(t.Context(), database, registry, selected, options); err != nil || progress.Analyzed != 1 || opens != 1 {
		t.Fatalf("local energy repair: %+v %v opens=%d", progress, err, opens)
	}
	repaired, err := database.GetTrackAnalysis(ids[0])
	if err != nil {
		t.Fatal(err)
	}
	energyFound := false
	for _, field := range db.ResolveAnalysisScalarFields(repaired, db.TrackAnalysisOverride{}, repaired.SourceFingerprint, nil) {
		if field.Key == "local_energy_level" && field.Selected != nil && field.Selected.Source == "local" {
			energyFound = true
		}
	}
	if !energyFound {
		t.Fatal("repaired local energy was not available from the explicit envelope")
	}
	selected, err = ExpandPreparationSelection(t.Context(), database, db.AnalysisSelection{Mode: db.AnalysisSelectionMissing}, nil)
	if err != nil || len(selected) != 0 {
		t.Fatalf("repaired local energy repeated: %v %v", selected, err)
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

func TestMissingDecodedDurationRepairsOnce(t *testing.T) {
	database, ids := runnerCatalog(t, 1)
	registry := analysis.NewDefaultDecoderRegistry()
	if _, err := Run(t.Context(), database, registry, ids, RunOptions{}); err != nil {
		t.Fatal(err)
	}
	before, err := database.GetTrackAnalysis(ids[0])
	if err != nil {
		t.Fatal(err)
	}
	fields := before.Local.Fields
	before.Local.Fields = nil
	for _, field := range fields {
		if field.Key != "local_duration_seconds" {
			before.Local.Fields = append(before.Local.Fields, field)
		}
	}
	if err := database.UpsertTrackAnalysis(before); err != nil {
		t.Fatal(err)
	}
	selected, err := ExpandPreparationSelection(t.Context(), database, db.AnalysisSelection{Mode: db.AnalysisSelectionMissing}, nil)
	if err != nil || len(selected) != 1 {
		t.Fatalf("missing duration selection: %v %v", selected, err)
	}
	opens := 0
	options := RunOptions{ResolveSource: func(ctx context.Context, id string) (analysis.ResolvedSource, error) {
		source, err := analysis.ResolveLocalSource(database, id)
		open := source.Open
		source.OpenStream = func() (io.ReadCloser, error) { opens++; return open() }
		return source, err
	}}
	if progress, err := Run(t.Context(), database, registry, selected, options); err != nil || progress.Analyzed != 1 || opens != 1 {
		t.Fatalf("duration repair: %+v %v opens=%d", progress, err, opens)
	}
	selected, err = ExpandPreparationSelection(t.Context(), database, db.AnalysisSelection{Mode: db.AnalysisSelectionMissing}, nil)
	if err != nil || len(selected) != 0 {
		t.Fatalf("duration repair repeated: %v %v", selected, err)
	}
}

func TestNoDecodedDurationSettlesUnavailable(t *testing.T) {
	database, ids := runnerCatalog(t, 1)
	source, err := analysis.ResolveLocalSource(database, ids[0])
	if err != nil {
		t.Fatal(err)
	}
	result := Result{SongID: ids[0], Source: source, Status: db.TrackAnalysisPartial}
	if err := Persist(database, result); err != nil {
		t.Fatal(err)
	}
	states, err := database.GetTrackCapabilityStatuses(ids[0], source.Fingerprint)
	if err != nil || states["local_duration"].State != "unavailable" || states["local_duration"].Reason != "no_decoded_duration" || states["local_duration"].RetryAt != 0 {
		t.Fatalf("no PCM settlement: %+v %v", states, err)
	}
	record, err := database.GetTrackAnalysis(ids[0])
	if err != nil {
		t.Fatal(err)
	}
	missing, err := missingPreparation(database, source, record)
	if err != nil || missing["local_duration"] {
		t.Fatalf("settled no PCM repeatedly selected: %+v %v", missing, err)
	}
}

func TestProviderDurationCannotBecomeDecodedDuration(t *testing.T) {
	database, ids := runnerCatalog(t, 1)
	source, err := analysis.ResolveLocalSource(database, ids[0])
	if err != nil {
		t.Fatal(err)
	}
	var empty bytes.Buffer
	if err := analysisbench.WriteWAVPCM16(&empty, analysisbench.PCMFixture{SampleRate: 22050, Channels: 1}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source.Path, empty.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	bpm, duration := 120.0, 240.0
	options := RunOptions{SpotifyFeatures: func(context.Context, analysis.ResolvedSource) *spotifyanalysis.Observation {
		return &spotifyanalysis.Observation{BPM: &bpm, DurationSeconds: &duration}
	}}
	if _, err := Run(t.Context(), database, analysis.NewDefaultDecoderRegistry(), ids, options); err != nil {
		t.Fatal(err)
	}
	record, err := database.GetTrackAnalysis(ids[0])
	if err != nil {
		t.Fatal(err)
	}
	if record.Local != nil {
		for _, field := range record.Local.Fields {
			if field.Key == "local_duration_seconds" {
				t.Fatalf("provider duration fabricated local duration: %+v", field)
			}
		}
	}
	states, err := database.GetTrackCapabilityStatuses(ids[0], record.SourceFingerprint)
	if err != nil || states["local_duration"].State == "available" {
		t.Fatalf("provider duration settled local measurement: %+v %v", states, err)
	}
}

func TestLegacyThreeBandRemainsPreparedWithoutAudioDecode(t *testing.T) {
	database, ids := runnerCatalog(t, 1)
	registry := analysis.NewDefaultDecoderRegistry()
	if _, err := Run(t.Context(), database, registry, ids, RunOptions{}); err != nil {
		t.Fatal(err)
	}
	artifact, err := database.GetTrackAnalysisArtifact(ids[0], threeband.Kind, threeband.FormatVersion, threeband.AlgorithmVersion)
	if err != nil {
		t.Fatal(err)
	}
	o, err := threeband.Decode(artifact.Data)
	if err != nil {
		t.Fatal(err)
	}
	artifact.Data, err = json.Marshal(o)
	if err != nil {
		t.Fatal(err)
	}
	artifact.Encoding = threeband.LegacyEncoding
	if err := database.UpsertTrackAnalysisArtifact(artifact); err != nil {
		t.Fatal(err)
	}
	selected, err := database.ExpandAnalysisSelection(db.AnalysisSelection{Mode: db.AnalysisSelectionMissing}, AnalysisVersion, AlgorithmVersion)
	if err != nil || len(selected) != 0 {
		t.Fatalf("legacy selected for reanalysis: %v %v", selected, err)
	}
	options := RunOptions{ResolveSource: func(ctx context.Context, id string) (analysis.ResolvedSource, error) {
		source, err := analysis.ResolveLocalSource(database, id)
		source.OpenStream = func() (io.ReadCloser, error) {
			t.Error("legacy artifact caused audio decode")
			return nil, errors.New("unexpected decode")
		}
		return source, err
	}}
	if progress, err := Run(t.Context(), database, registry, ids, options); err != nil || progress.Skipped != 1 {
		t.Fatalf("legacy not current: %+v %v", progress, err)
	}
}
