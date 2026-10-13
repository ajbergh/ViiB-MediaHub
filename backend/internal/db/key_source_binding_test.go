package db

import (
	"path/filepath"
	"testing"
)

func TestSourceBoundKeyResolverAndConditionalWrites(t *testing.T) {
	d, err := New(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if err := d.SaveSong(&Song{ID: "song", Title: "Song", Artist: "Artist", Album: "Album", FilePath: "song.wav", AddedAt: 1}); err != nil {
		t.Fatal(err)
	}
	tonic, manualTonic, mode, source := 0, 9, "major", "measured"
	a := TrackAnalysis{SongID: "song", Status: TrackAnalysisComplete, SourceFingerprint: "v1", KeyTonic: &tonic, KeyMode: &mode, KeySource: &source}
	o := TrackAnalysisOverride{SongID: "song", KeyTonic: &manualTonic, KeyMode: &mode, KeyLocked: true}
	if k := ResolveEffectiveKeyForSource(EffectiveKeyInputs{Analysis: &a, Override: &o}, "v1"); k.Source != "measured" {
		t.Fatal("legacy unbound key became current")
	}
	o.KeySourceFingerprint = "v1"
	if k := ResolveEffectiveKeyForSource(EffectiveKeyInputs{Analysis: &a, Override: &o}, "v1"); k.Source != "manual" {
		t.Fatal("bound manual key ignored")
	}
	if k := ResolveEffectiveKeyForSource(EffectiveKeyInputs{Analysis: &a, Override: &o}, "v2"); k.Tonic != nil {
		t.Fatal("old key reused for new source")
	}
	bpm := 128.0
	grid := "grid"
	o.BPM = &bpm
	o.BPMLocked = true
	o.BPMSourceFingerprint = "v1"
	o.BeatgridArtifactID = &grid
	o.BeatgridLocked = true
	if err := d.UpsertTrackAnalysisOverride(o); err != nil {
		t.Fatal(err)
	}
	if err := d.RefreshTrackAnalysisSourceRevision("song", "v2"); err != nil {
		t.Fatal(err)
	}
	if wrote, err := d.SetTrackAnalysisKeyOverrideIfSourceCurrent("song", 7, "minor", "v1"); err != nil || wrote {
		t.Fatal("stale concurrent key write accepted")
	}
	if wrote, err := d.SetTrackAnalysisKeyOverrideIfSourceCurrent("song", 7, "minor", "v2"); err != nil || !wrote {
		t.Fatal("current key write rejected")
	}
	got, err := d.GetTrackAnalysisOverride("song")
	if err != nil || got.KeySourceFingerprint != "v2" || *got.BPM != bpm || !got.BeatgridLocked {
		t.Fatal("key-only update replaced other overrides")
	}
	if reset, err := d.ResetTrackAnalysisKeyOverrideIfSourceCurrent("song", "v1"); err != nil || reset {
		t.Fatal("stale reset accepted")
	}
	if reset, err := d.ResetTrackAnalysisKeyOverrideIfSourceCurrent("song", "v2"); err != nil || !reset {
		t.Fatal("current reset rejected")
	}
	got, err = d.GetTrackAnalysisOverride("song")
	if err != nil || got.KeyTonic != nil || got.KeySourceFingerprint != "" || got.KeyLocked || *got.BPM != bpm || !got.BeatgridLocked {
		t.Fatal("key reset changed other overrides")
	}
}

func TestKeyFingerprintMigrationPreservesUnboundLegacyOverride(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.db")
	d, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = d.Close() }()
	if err := d.SaveSong(&Song{ID: "song", Title: "Song", Artist: "Artist", Album: "Album", FilePath: "song.wav", AddedAt: 1}); err != nil {
		t.Fatal(err)
	}
	tonic, mode, bpm := 9, "minor", 128.0
	if err := d.UpsertTrackAnalysisOverride(TrackAnalysisOverride{SongID: "song", KeyTonic: &tonic, KeyMode: &mode, KeyLocked: true, BPM: &bpm, BPMLocked: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := d.conn.Exec(`ALTER TABLE track_analysis_overrides DROP COLUMN key_source_fingerprint`); err != nil {
		t.Fatal(err)
	}
	trackAnalysisSchemas.Delete(d)
	got, err := d.GetTrackAnalysisOverride("song")
	if err != nil || got.KeySourceFingerprint != "" || !got.KeyLocked || *got.KeyTonic != tonic || *got.BPM != bpm {
		t.Fatal("migration lost legacy manual values")
	}
	if key := ResolveEffectiveKeyForSource(EffectiveKeyInputs{Override: &got}, "current"); key.Tonic != nil {
		t.Fatal("migration invented legacy source binding")
	}
	if err := d.RefreshTrackAnalysisSourceRevision("song", "current"); err != nil {
		t.Fatal(err)
	}
	if ok, err := d.SetTrackAnalysisKeyOverrideIfSourceCurrent("song", tonic, mode, "current"); err != nil || !ok {
		t.Fatal("legacy key cannot be verified")
	}
	if err := d.Close(); err != nil {
		t.Fatal(err)
	}
	d, err = New(path)
	if err != nil {
		t.Fatal(err)
	}
	got, err = d.GetTrackAnalysisOverride("song")
	if err != nil || got.KeySourceFingerprint != "current" || *got.BPM != bpm {
		t.Fatal("key source binding lost on restart")
	}
}
