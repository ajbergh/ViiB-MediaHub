package api

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/ajbergh/viib-mediahub/internal/analysis"
	"github.com/ajbergh/viib-mediahub/internal/db"
)

func TestPlaylistMetadataUsesCurrentManualFieldsAndFencesPublication(t *testing.T) {
	api, path := newBPMRouteTestAPI(t, true)
	source, err := analysis.ResolveLocalSource(api.db, "song")
	if err != nil {
		t.Fatal(err)
	}
	if err := api.db.RefreshTrackAnalysisSourceRevision("song", source.Fingerprint); err != nil {
		t.Fatal(err)
	}
	bpm, tonic, mode := 125.0, 9, "minor"
	if err := api.db.UpsertTrackAnalysisOverride(db.TrackAnalysisOverride{SongID: "song", BPM: &bpm, BPMLocked: true, BPMSourceFingerprint: source.Fingerprint, KeyTonic: &tonic, KeyMode: &mode, KeyLocked: true, KeySourceFingerprint: source.Fingerprint}); err != nil {
		t.Fatal(err)
	}
	if ok, err := api.db.SetTrackMetadataOverrideIfSourceCurrent("song", "local_energy_level", source.Fingerprint, json.RawMessage("8"), false); err != nil || !ok {
		t.Fatalf("manual energy: %v %v", ok, err)
	}
	snapshot := newPlaylistMetadataSnapshot(api)
	if err := snapshot.load(context.Background(), []db.Song{{ID: "song", Duration: 240}}); err != nil {
		t.Fatal(err)
	}
	value := snapshot.audio["song"]
	if value.BPM == nil || value.BPM.Value != 125 || value.BPM.Source != "manual" || value.EnergyLevel == nil || value.EnergyLevel.Value != 8 || value.Key == nil || value.Key.Camelot != "8A" {
		t.Fatalf("resolved=%+v", value)
	}
	emitted := false
	if err := snapshot.publish(context.Background(), func() error { emitted = true; return nil }); err != nil || !emitted {
		t.Fatalf("publication: %v", err)
	}
	if err := os.WriteFile(path, []byte("changed media with a different size"), 0o600); err != nil {
		t.Fatal(err)
	}
	emitted = false
	if err := snapshot.publish(context.Background(), func() error { emitted = true; return nil }); err == nil || emitted {
		t.Fatal("stale playlist published")
	}
	fresh := newPlaylistMetadataSnapshot(api)
	if err := fresh.load(context.Background(), []db.Song{{ID: "song"}}); err != nil {
		t.Fatal(err)
	}
	if value := fresh.audio["song"]; value.BPM != nil || value.Key != nil || value.EnergyLevel != nil {
		t.Fatalf("stale manual facts reused: %+v", value)
	}
}

func TestPlaylistMetadataKeepsProviderScoresSeparate(t *testing.T) {
	field := func(key, value, source string) db.EffectiveScalar {
		return db.EffectiveScalar{Key: key, Selected: &db.ScalarCandidate{SpotifyScalarField: db.SpotifyScalarField{Key: key, Value: json.RawMessage(value)}, Source: source}}
	}
	feature := TrackAnalysisFeatureResponse{EffectiveFields: []db.EffectiveScalar{
		field("spotify_energy_score", "0.9", "spotify_download_import"),
		field("spotify_danceability_score", "0.8", "spotify_download_import"),
		field("local_duration_seconds", "240", "manual"),
		field("duration_milliseconds", "100000", "spotify_download_import"),
	}}
	value := playlistAudioFeatures(feature, 200)
	if value.EnergyLevel != nil || value.DurationSeconds != 240 || value.ProviderScores["spotify_energy_score"].Value != .9 || value.ProviderScores["spotify_danceability_score"].Confidence != .65 {
		t.Fatalf("projection=%+v", value)
	}
}
