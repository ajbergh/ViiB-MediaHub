package db

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestManualMetadataOverrideReopenAndSourceGuard(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.db")
	d, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := d.SaveSong(&Song{ID: "song", Title: "Song", AddedAt: 1}); err != nil {
		t.Fatal(err)
	}
	if err := d.RefreshTrackAnalysisSourceRevision("song", "fp"); err != nil {
		t.Fatal(err)
	}
	for key, value := range map[string]string{"time_signature": "3", "local_energy_level": "7"} {
		if ok, err := d.SetTrackMetadataOverrideIfSourceCurrent("song", key, "fp", json.RawMessage(value), false); err != nil || !ok {
			t.Fatal(ok, err)
		}
	}
	if err := d.Close(); err != nil {
		t.Fatal(err)
	}
	d, err = New(path)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	fields, err := d.GetTrackMetadataOverrides("song")
	if err != nil || len(fields) != 2 {
		t.Fatalf("reopen: %+v %v", fields, err)
	}
	provider := ScalarCandidate{SpotifyScalarField: SpotifyScalarField{Key: "time_signature", Metric: "measured_meter", Units: "beats_per_bar", Value: json.RawMessage("4")}, Source: "spotify_private", SourceFingerprint: "fp"}
	candidates := append(fields, provider)
	if got := ResolveEffectiveScalar("time_signature", "fp", candidates); got.Selected == nil || got.Selected.Source != "manual" || string(got.Selected.Value) != "3" {
		t.Fatalf("manual precedence: %+v", got)
	}
	if err := d.RefreshTrackAnalysisSourceRevision("song", "new"); err != nil {
		t.Fatal(err)
	}
	if ok, err := d.SetTrackMetadataOverrideIfSourceCurrent("song", "time_signature", "fp", nil, true); err != nil || ok {
		t.Fatal("stale reset accepted", ok, err)
	}
	if ok, err := d.SetTrackMetadataOverrideIfSourceCurrent("song", "time_signature", "fp", json.RawMessage("4"), false); err != nil || ok {
		t.Fatal("stale write accepted", ok, err)
	}
	fields, err = d.GetTrackMetadataOverrides("song")
	if err != nil || len(fields) != 2 {
		t.Fatal("stale writes changed retained values")
	}
}
