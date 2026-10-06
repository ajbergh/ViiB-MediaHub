package db

import (
	spotifyanalysis "github.com/ajbergh/viib-mediahub/internal/spotify/analysis"
	"path/filepath"
	"testing"
	"time"
)

func TestSpotifyBindingsRetirePerFieldAndPreserveDurableFacts(t *testing.T) {
	d, err := New(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if err := d.SaveSong(&Song{ID: "song", Title: "Song", Artist: "Artist", Album: "Album", FilePath: "song.wav", AddedAt: 1}); err != nil {
		t.Fatal(err)
	}
	if err := d.ActivateSpotifyMetadataContext("account-a"); err != nil {
		t.Fatal(err)
	}
	localBPM, providerBPM := 120.0, 130.0
	tonic, mode := 0, 1
	localMode := "major"
	a := TrackAnalysis{SongID: "song", Status: TrackAnalysisComplete, SourceFingerprint: "bytes1", AnalysisVersion: 1, AlgorithmVersion: "local-v1", Local: &LocalScalarObservation{SourceFingerprint: "bytes1", AlgorithmVersion: "local-v1", BPM: &localBPM, KeyTonic: &tonic, KeyMode: &localMode}}
	o := spotifyanalysis.Observation{TrackID: "0123456789abcdefghijkl", SourceEndpoint: "audio_features", RetrievedAt: time.Now(), AccountContext: "account-a", BPM: &providerBPM, Key: &tonic, Mode: &mode}
	ApplySpotifyScalars(&a, o)
	if err := d.UpsertTrackAnalysis(a); err != nil {
		t.Fatal(err)
	}
	got, err := d.GetTrackAnalysis("song")
	if err != nil {
		t.Fatal(err)
	}
	if b := ResolveEffectiveBPM(EffectiveBPMInputs{Analysis: &got}); b.Source != "spotify" || *b.Value != providerBPM || b.SyncAllowed {
		t.Fatal("current provider eligibility/sync")
	}
	if err := d.ActivateSpotifyMetadataContext("account-b"); err != nil {
		t.Fatal(err)
	}
	got, err = d.GetTrackAnalysis("song")
	if err != nil {
		t.Fatal(err)
	}
	if b := ResolveEffectiveBPM(EffectiveBPMInputs{Analysis: &got}); b.Source != "measured" || *b.Value != localBPM {
		t.Fatal("retired provider did not fall back to local tempo")
	}
	if k := ResolveEffectiveKey(EffectiveKeyInputs{Analysis: &got}); k.Source != "measured" || *k.Tonic != tonic {
		t.Fatal("retired provider did not fall back to local key")
	}
	if d.UpsertTrackAnalysis(a) == nil {
		t.Fatal("late retired-account write accepted")
	}
	// Only the newly returned tempo becomes account-b eligible. Key remains
	// independently bound to account-a and falls back to its local alternative.
	o.AccountContext = "account-b"
	o.Key = nil
	o.Mode = nil
	ApplySpotifyScalars(&got, o)
	if err := d.UpsertTrackAnalysis(got); err != nil {
		t.Fatal(err)
	}
	if got.KeySource == nil || *got.KeySource != "measured" || got.SpotifyBindings.Key != nil {
		t.Fatal("partial replacement retained retired key")
	}
	got, err = d.GetTrackAnalysis("song")
	if err != nil {
		t.Fatal(err)
	}
	if b := ResolveEffectiveBPM(EffectiveBPMInputs{Analysis: &got}); b.Source != "spotify" {
		t.Fatal("new-account tempo unavailable")
	}
	if err := d.ActivateSpotifyMetadataContext(""); err != nil {
		t.Fatal(err)
	}
	if err := d.RetirePrivateSpotifyScalarBindings(); err != nil {
		t.Fatal(err)
	}
	got, err = d.GetTrackAnalysis("song")
	if err != nil {
		t.Fatal(err)
	}
	if got.SpotifyBindings != nil || got.BPMSource == nil || *got.BPMSource != "measured" || *got.BPM != localBPM {
		t.Fatal("private projections not purged to local fallback")
	}
	// Verified import evidence bypasses the session cache lifecycle.
	o.DurableImport = true
	o.Key = &tonic
	o.Mode = &mode
	ApplySpotifyScalars(&got, o)
	if err := d.UpsertTrackAnalysis(got); err != nil {
		t.Fatal(err)
	}
	if err := d.ActivateSpotifyMetadataContext(""); err != nil {
		t.Fatal(err)
	}
	if err := d.RetirePrivateSpotifyScalarBindings(); err != nil {
		t.Fatal(err)
	}
	got, err = d.GetTrackAnalysis("song")
	if err != nil {
		t.Fatal(err)
	}
	if b := ResolveEffectiveBPMForSource(EffectiveBPMInputs{Analysis: &got}, "bytes1"); b.Source != "spotify" || b.SyncAllowed {
		t.Fatal("durable fact lost after retirement")
	}
	if b := ResolveEffectiveBPMForSource(EffectiveBPMInputs{Analysis: &got}, "bytes2"); b.Value != nil {
		t.Fatal("durable fact applied to changed bytes")
	}
}

func TestExternalScalarCacheRetainsAccountOwnership(t *testing.T) {
	d, err := New(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if err := d.ActivateSpotifyMetadataContext("account-a"); err != nil {
		t.Fatal(err)
	}
	bpm := 120.0
	now := time.Now().UTC()
	o := spotifyanalysis.Observation{TrackID: "0123456789abcdefghijkl", Source: "spotify_internal", SourceEndpoint: "audio_features", RetrievedAt: now, AccountContext: "account-a", BPM: &bpm}
	if err := d.PutExternalAnalysis(o, "revision", now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	cache, err := d.GetExternalAnalysis(o.TrackID, o.SourceEndpoint)
	if err != nil || cache == nil || cache.Observation.AccountContext != o.AccountContext {
		t.Fatalf("cache ownership lost: %+v %v", cache, err)
	}
	if err := d.ActivateSpotifyMetadataContext("account-b"); err != nil {
		t.Fatal(err)
	}
	cache, err = d.GetExternalAnalysis(o.TrackID, o.SourceEndpoint)
	if err != nil || cache != nil {
		t.Fatal("retired-account cache exposed")
	}

}

func TestMixedPreparationScalarBindings(t *testing.T) {
	bpm, confidence := 123.0, 0.8
	key, mode := 5, 0
	first := time.Unix(1700000000, 0)
	features := spotifyanalysis.Observation{TrackID: "0123456789abcdefghijkl", AccountContext: "account", SourceEndpoint: "audio_features", RetrievedAt: first, BPM: &bpm}
	detailed := spotifyanalysis.Observation{TrackID: features.TrackID, AccountContext: features.AccountContext, SourceEndpoint: "audio_analysis", RetrievedAt: first.Add(time.Hour), Key: &key, Mode: &mode, KeyConfidence: &confidence}
	projected := spotifyanalysis.PreparationScalars(&features, &detailed)
	record := TrackAnalysis{SourceFingerprint: "bytes"}
	ApplySpotifyScalars(&record, *projected)
	if record.SpotifyBindings.BPM.Endpoint != "audio_features" || record.SpotifyBindings.Key.Endpoint != "audio_analysis" || record.SpotifyBindings.Key.RetrievedAt != detailed.RetrievedAt.UnixMilli() || record.BPMConfidence != nil || record.KeyConfidence != &confidence {
		t.Fatal("mixed origins lost")
	}
	if features.Key != nil || detailed.BPM != nil {
		t.Fatal("original provider observations changed")
	}
	features.BPM = nil
	features.Key, features.Mode = &key, &mode
	detailed.BPM, detailed.BPMConfidence = &bpm, &confidence
	detailed.Key, detailed.Mode = nil, nil
	projected = spotifyanalysis.PreparationScalars(&features, &detailed)
	ApplySpotifyScalars(&record, *projected)
	if record.SpotifyBindings.BPM.Endpoint != "audio_analysis" || record.SpotifyBindings.Key.Endpoint != "audio_features" || record.BPMConfidence != &confidence || record.KeyConfidence != nil {
		t.Fatal("reverse mixed origins lost")
	}
}

func TestExternalCacheRejectsMixedPreparationProjection(t *testing.T) {
	d, err := New(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if err := d.ActivateSpotifyMetadataContext("fixture-account"); err != nil {
		t.Fatal(err)
	}
	bpm := 125.0
	key, mode := 4, 1
	now := time.Now().UTC()
	features := spotifyanalysis.Observation{TrackID: "0123456789abcdefghijkl", AccountContext: "fixture-account", Source: "spotify_internal", SourceEndpoint: "audio_features", RetrievedAt: now, BPM: &bpm}
	detailed := features
	detailed.SourceEndpoint = "audio_analysis"
	detailed.BPM = nil
	detailed.Key, detailed.Mode = &key, &mode
	camelot := "12B"
	detailed.Camelot = &camelot
	projection := spotifyanalysis.PreparationScalars(&features, &detailed)
	if err := d.PutExternalAnalysis(*projection, "fixture", now.Add(time.Hour)); err == nil {
		t.Fatal("mixed projection persisted as provider observation")
	}
	for _, original := range []spotifyanalysis.Observation{features, detailed} {
		if err := d.PutExternalAnalysis(original, "fixture", now.Add(time.Hour)); err != nil {
			t.Fatal(err)
		}
	}
	cached, err := d.GetExternalAnalysis(features.TrackID, features.SourceEndpoint)
	if err != nil || cached == nil || cached.Observation.Key != nil {
		t.Fatalf("original feature cache changed: %+v %v", cached, err)
	}
}
