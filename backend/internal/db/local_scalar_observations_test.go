package db

import (
	"math"
	"path/filepath"
	"reflect"
	"testing"
)

func TestIndependentLocalScalarsPersistResolveAndMigrate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.db")
	d, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = d.Close() }()
	if err := d.SaveSong(&Song{ID: "song", Title: "Song", Artist: "Artist", Album: "Album", FilePath: "song.wav", AddedAt: 1}); err != nil {
		t.Fatal(err)
	}
	if err := d.EnsureTrackAnalysisSchema(); err != nil {
		t.Fatal(err)
	}
	// Recreate the immediately preceding production schema by removing only the
	// new nullable column. Existing analysis and override rows must survive.
	if _, err := d.conn.Exec(`ALTER TABLE track_analysis DROP COLUMN local_scalar_json`); err != nil {
		t.Fatal(err)
	}
	trackAnalysisSchemas.Delete(d)
	bpm, confidence, provider := 120.0, .9, 130.0
	tonic, mode, measured, spotify := 0, "major", "measured", "spotify"
	original := TrackAnalysis{SongID: "song", Status: TrackAnalysisComplete, AnalysisVersion: 1, AlgorithmVersion: "local-v1", SourceFingerprint: "bytes1", BPM: &bpm, BPMSource: &measured, KeyTonic: &tonic, KeyMode: &mode, KeySource: &measured}
	if err := d.UpsertTrackAnalysis(original); err != nil {
		t.Fatal(err)
	}
	migrated, err := d.GetTrackAnalysis("song")
	if err != nil {
		t.Fatal(err)
	}
	if migrated.Local == nil || *migrated.Local.BPM != bpm || *migrated.Local.KeyTonic != 0 {
		t.Fatalf("measured legacy recovery: %+v", migrated.Local)
	}
	migrated.Local.BPMConfidence = &confidence
	migrated.BPM = &provider
	migrated.BPMSource = &spotify
	if err := d.UpsertTrackAnalysis(migrated); err != nil {
		t.Fatal(err)
	}
	local := migrated.Local
	if err := d.Close(); err != nil {
		t.Fatal(err)
	}
	d, err = New(path)
	if err != nil {
		t.Fatal(err)
	}
	got, err := d.GetTrackAnalysis("song")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(local, got.Local) {
		t.Fatalf("restart lost alternative: %+v", got.Local)
	}
	effective := ResolveEffectiveBPM(EffectiveBPMInputs{Analysis: &got})
	if effective.Source != measured || *effective.Value != bpm || !effective.SyncAllowed {
		t.Fatalf("unbound historical provider must fall back locally: %+v", effective)
	}
	got.BPM = nil
	got.BPMSource = nil
	effective = ResolveEffectiveBPM(EffectiveBPMInputs{Analysis: &got})
	if effective.Source != measured || *effective.Value != bpm || !effective.SyncAllowed {
		t.Fatalf("local fallback: %+v", effective)
	}
	manual := 140.0
	o := TrackAnalysisOverride{BPM: &manual, BPMLocked: true, BPMSourceFingerprint: "bytes1"}
	effective = ResolveEffectiveBPMForSource(EffectiveBPMInputs{Analysis: &got, Override: &o}, "bytes1")
	if effective.Source != "manual" || *effective.Value != manual {
		t.Fatal("manual precedence lost")
	}
	got.SourceFingerprint = "bytes2"
	got.KeyTonic = nil
	got.KeyMode = nil
	got.KeySource = nil
	if ResolveEffectiveBPM(EffectiveBPMInputs{Analysis: &got}).Value != nil || ResolveEffectiveKey(EffectiveKeyInputs{Analysis: &got}).Tonic != nil {
		t.Fatal("old local observation exposed for new bytes")
	}
	invalid := math.NaN()
	got.Local.BPM = &invalid
	if d.UpsertTrackAnalysis(got) == nil {
		t.Fatal("invalid alternative accepted")
	}
	stable, err := d.GetTrackAnalysis("song")
	if err != nil || stable.SourceFingerprint != "bytes1" || *stable.BPM != provider {
		t.Fatal("invalid observation replaced last good projection")
	}
}

func TestSpotifyLegacySlotsDoNotInventLocalMeasurements(t *testing.T) {
	bpm, source := 128.0, "spotify"
	a := TrackAnalysis{Status: TrackAnalysisComplete, SourceFingerprint: "bytes", AlgorithmVersion: "spotify-v1", BPM: &bpm, BPMSource: &source}
	tonic, mode := 0, "major"
	a.KeyTonic, a.KeyMode, a.KeySource = &tonic, &mode, &source
	if ResolveEffectiveBPM(EffectiveBPMInputs{Analysis: &a}).Value != nil || ResolveEffectiveKey(EffectiveKeyInputs{Analysis: &a}).Tonic != nil {
		t.Fatal("unbound historical provider exposed as current evidence")
	}
	if CurrentLocalScalars(&a) != nil {
		t.Fatal("provider legacy projection invented local measurement")
	}
}
