package track

import (
	"github.com/ajbergh/viib-mediahub/internal/analysis"
	"github.com/ajbergh/viib-mediahub/internal/analysis/features"
	"github.com/ajbergh/viib-mediahub/internal/analysis/key"
	"github.com/ajbergh/viib-mediahub/internal/analysis/tempo"
	"github.com/ajbergh/viib-mediahub/internal/db"
	spotifyanalysis "github.com/ajbergh/viib-mediahub/internal/spotify/analysis"
	"reflect"
	"testing"
)

func TestPersistPreservesLocalKeyAndTempoUnderProviderProjection(t *testing.T) {
	database, ids := runnerCatalog(t, 1)
	source, err := analysis.ResolveLocalSource(database, ids[0])
	if err != nil {
		t.Fatal(err)
	}
	providerBPM, providerKey, providerMode := 130.0, 9, 0
	result := Result{SongID: ids[0], Source: source, Status: db.TrackAnalysisComplete,
		Tempo:   tempo.Estimate{Known: true, BPM: 120, Confidence: .9, Stability: .95},
		Key:     key.Estimate{Known: true, Tonic: 0, Mode: "major", Confidence: .8, Camelot: "8B", OpenKey: "1d"},
		Spotify: &spotifyanalysis.Observation{BPM: &providerBPM, Key: &providerKey, Mode: &providerMode}}
	if err := Persist(database, result); err != nil {
		t.Fatal(err)
	}
	got, err := database.GetTrackAnalysis(ids[0])
	if err != nil {
		t.Fatal(err)
	}
	if got.Local == nil || got.Local.KeyTonic == nil || *got.Local.KeyTonic != 0 || *got.Local.KeyMode != "major" || *got.Local.KeyConfidence != .8 || *got.Local.BPM != 120 || *got.Local.BPMConfidence != .9 {
		t.Fatalf("lost measured alternatives: %+v", got.Local)
	}
	if *got.BPM != 130 || *got.KeyTonic != 9 || *got.KeyMode != "minor" || got.BPMConfidence != nil || got.KeyConfidence != nil {
		t.Fatalf("provider projection: %+v", got)
	}
	// A later partial refresh may change one provider dimension while preserving
	// the coupled local key and all local measurement confidence/provenance.
	refreshed := spotifyanalysis.Observation{BPM: &providerBPM}
	db.ApplySpotifyScalars(&got, refreshed)
	if err := database.UpsertTrackAnalysis(got); err != nil {
		t.Fatal(err)
	}
	got, err = database.GetTrackAnalysis(ids[0])
	if err != nil || *got.Local.KeyTonic != 0 || *got.Local.KeyMode != "major" {
		t.Fatal("partial provider refresh lost local key")
	}
}

func TestPersistGenericLocalMetricsAndSelectiveRepair(t *testing.T) {
	database, ids := runnerCatalog(t, 1)
	source, err := analysis.ResolveLocalSource(database, ids[0])
	if err != nil {
		t.Fatal(err)
	}
	lufs, peak := -14.0, -1.0
	result := Result{SongID: ids[0], Source: source, Status: db.TrackAnalysisPartial, DurationSeconds: 12.5, EnergyLevel: &features.EnergyLevelEstimate{Level: 4, Confidence: .8, AlgorithmVersion: features.EnergyLevelAlgorithmVersion}}
	result.Loudness = &features.BS1770Result{IntegratedLUFS: &lufs, TruePeakDBTP: &peak, LoudnessStatus: "available", TruePeakStatus: "available", Standard: features.BS1770Standard, Algorithm: features.BS1770AlgorithmVersion, LoudnessAlgorithm: features.BS1770LoudnessAlgorithm, TruePeakAlgorithm: features.BS1770TruePeakAlgorithm}
	if err := Persist(database, result); err != nil {
		t.Fatal(err)
	}
	previous, err := database.GetTrackAnalysis(ids[0])
	if err != nil {
		t.Fatal(err)
	}
	if previous.Local == nil || len(previous.Local.Fields) != 4 {
		t.Fatalf("local fields: %+v", previous.Local)
	}
	result.DurationSeconds = 99
	result.EnergyLevel.Level = 7
	result.RepairPrevious = &previous
	result.RepairCapabilities = map[string]bool{"local_energy": true}
	if err := Persist(database, result); err != nil {
		t.Fatal(err)
	}
	got, err := database.GetTrackAnalysis(ids[0])
	if err != nil {
		t.Fatal(err)
	}
	resolved := db.ResolveAnalysisScalarFields(got, db.TrackAnalysisOverride{}, source.Fingerprint, nil)
	values := map[string]string{}
	for _, field := range resolved {
		if field.Selected != nil {
			values[field.Key] = string(field.Selected.Value)
		}
	}
	if values["local_duration_seconds"] != "12.5" || values["local_energy_level"] != "7" || values["integrated_lufs_bs1770"] != "-14" || values["true_peak_dbtp"] != "-1" {
		t.Fatalf("repair changed unrequested metrics: %+v", values)
	}
	if values["duration_seconds"] != "" || values["spotify_energy_score"] != "" {
		t.Fatalf("local substituted provider semantics: %+v", values)
	}
	if fields := db.ResolveAnalysisScalarFields(got, db.TrackAnalysisOverride{}, "replaced", nil); fields[0].Selected != nil {
		t.Fatal("old local duration admitted")
	}
}

func TestFailedScalarRepairPreservesLocalEvidence(t *testing.T) {
	database, ids := runnerCatalog(t, 1)
	source, err := analysis.ResolveLocalSource(database, ids[0])
	if err != nil {
		t.Fatal(err)
	}
	initial := Result{SongID: ids[0], Source: source, Status: db.TrackAnalysisPartial, DurationSeconds: 12.5, Tempo: tempo.Estimate{Known: true, BPM: 120, Confidence: .9}, EnergyLevel: &features.EnergyLevelEstimate{Level: 4, Confidence: .8, AlgorithmVersion: features.EnergyLevelAlgorithmVersion}}
	if err := Persist(database, initial); err != nil {
		t.Fatal(err)
	}
	before, err := database.GetTrackAnalysis(ids[0])
	if err != nil {
		t.Fatal(err)
	}
	failed := Result{SongID: ids[0], Source: source, Status: db.TrackAnalysisFailed, PreparationError: "decode_failed", RepairPrevious: &before, RepairCapabilities: map[string]bool{"local_scalars": true, "local_energy": true}}
	if err := Persist(database, failed); err != nil {
		t.Fatal(err)
	}
	after, err := database.GetTrackAnalysis(ids[0])
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before.Local, after.Local) || !reflect.DeepEqual(before.BPM, after.BPM) || !reflect.DeepEqual(before.EnergyLevel, after.EnergyLevel) {
		t.Fatalf("failed repair erased evidence: before=%+v after=%+v", before.Local, after.Local)
	}
	states, err := database.GetTrackCapabilityStatuses(ids[0], source.Fingerprint)
	if err != nil || states["local_scalars"].State == "available" {
		t.Fatalf("failed attempt misreported: %+v %v", states, err)
	}
}
