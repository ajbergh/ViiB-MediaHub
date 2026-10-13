package db

import (
	"errors"
	spotifyanalysis "github.com/ajbergh/viib-mediahub/internal/spotify/analysis"
	"path/filepath"
	"testing"
	"time"
)

func TestRetainedArtifactReadDoesNotActivateOwner(t *testing.T) {
	d, err := New(filepath.Join(t.TempDir(), "read.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	d.ReserveSpotifyMetadataRuntime("first")
	if _, err := d.ConfirmSpotifyMetadataOwner("first", "oauth", "account", "retained"); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	if err := d.PutSpotifyWaveformForRuntime(SpotifyMetadataFence{Epoch: "first", ContextKey: "retained"}, referenceID, waveformFixture(t), "fixture", now, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	observation, err := spotifyanalysis.ValidateDomainPayload(referenceID, "audio_features", []byte(`{"id":"`+referenceID+`","tempo":123}`))
	if err != nil {
		t.Fatal(err)
	}
	observation.AccountContext = "retained"
	observation.RetrievedAt = now
	if err := d.PutExternalAnalysis(observation, "fixture", now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	failure := ExternalAnalysisStatus{Code: spotifyanalysis.RateLimited, CheckedAt: now, RetryAt: now.Add(time.Minute)}
	if err := d.PutExternalAnalysisStatusForRuntime(SpotifyMetadataFence{Epoch: "first", ContextKey: "retained"}, referenceID, "audio_features", failure); err != nil {
		t.Fatal(err)
	}
	if err := d.PutExternalAnalysisStatus(referenceID, "audio_analysis", failure); err != nil {
		t.Fatal(err)
	}
	if _, err := d.ReserveSpotifyMetadataRuntime("restart"); err != nil {
		t.Fatal(err)
	}
	fence := SpotifyMetadataReadFence{Epoch: "restart", ContextKey: "retained", Provider: "oauth", Pending: true}
	status, err := d.GetExternalAnalysisStatusForRuntime(fence, referenceID, "audio_features")
	if err != nil || status == nil || status.Code != spotifyanalysis.RateLimited {
		t.Fatal(status, err)
	}
	if status, err := d.GetExternalAnalysisStatusForRuntime(fence, referenceID, "audio_analysis"); err != nil || status != nil {
		t.Fatal("unscoped failure exposed", status, err)
	}
	fields, attempts, err := d.GetSpotifyScalarCandidatesForRuntime(fence, referenceID, now)
	if err != nil || len(fields) == 0 || len(attempts) == 0 {
		t.Fatal("retained candidates", fields, attempts, err)
	}
	if active, err := d.GetSpotifyScalarFields(referenceID, now); err != nil || len(active) != 0 {
		t.Fatal("retained fields became active", active, err)
	}
	cache, err := d.GetExternalAnalysisForRuntime(fence, referenceID, "audio_features")
	if err != nil || cache == nil || cache.Observation.BPM == nil || *cache.Observation.BPM != 123 {
		t.Fatal(cache, err)
	}
	if cache, err := d.GetExternalAnalysis(referenceID, "audio_features"); err != nil || cache != nil {
		t.Fatal("retained cache became active", cache, err)
	}
	artifact, err := d.GetSpotifyAudioArtifactForRuntime(fence, referenceID, "three_band_waveform", "spotify_three_band")
	if err != nil || artifact == nil {
		t.Fatal(artifact, err)
	}
	if err := d.PutSpotifyWaveformForRuntime(SpotifyMetadataFence{Epoch: "restart", ContextKey: "retained"}, referenceID, waveformFixture(t), "fixture", now, now.Add(time.Hour)); !errors.Is(err, ErrSpotifyMetadataRuntimeSuperseded) {
		t.Fatal("read activated writes", err)
	}
	for _, bad := range []SpotifyMetadataReadFence{
		{Epoch: "first", ContextKey: "retained", Provider: "oauth", Pending: true},
		{Epoch: "restart", ContextKey: "other", Provider: "oauth", Pending: true},
		{Epoch: "restart", ContextKey: "retained", Provider: "webplayer", Pending: true},
		{Epoch: "restart", ContextKey: "retained", Provider: "oauth"},
	} {
		if got, err := d.GetSpotifyAudioArtifactForRuntime(bad, referenceID, "three_band_waveform", "spotify_three_band"); got != nil || !errors.Is(err, ErrSpotifyMetadataRuntimeSuperseded) {
			t.Fatal("invalid read accepted", bad, got, err)
		}
	}
	if _, err := d.ConfirmSpotifyMetadataOwner("restart", "oauth", "account", "unused"); err != nil {
		t.Fatal(err)
	}
	if _, err := d.GetSpotifyAudioArtifactForRuntime(fence, referenceID, "three_band_waveform", "spotify_three_band"); !errors.Is(err, ErrSpotifyMetadataRuntimeSuperseded) {
		t.Fatal("pending read survived activation", err)
	}
	fence.Pending = false
	if got, err := d.GetSpotifyAudioArtifactForRuntime(fence, referenceID, "three_band_waveform", "spotify_three_band"); err != nil || got == nil {
		t.Fatal(got, err)
	}
	if err := d.RetireSpotifyMetadataContext("retained"); err != nil {
		t.Fatal(err)
	}
	if got, err := d.GetSpotifyAudioArtifactForRuntime(fence, referenceID, "three_band_waveform", "spotify_three_band"); got != nil || !errors.Is(err, ErrSpotifyMetadataRuntimeSuperseded) {
		t.Fatal("retired read accepted", got, err)
	}
}

func TestActiveArtifactReadRejectsSupersededEpochBeforeProfileCapture(t *testing.T) {
	d, err := New(filepath.Join(t.TempDir(), "active-read.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if _, err := d.ReserveSpotifyMetadataRuntime("first"); err != nil {
		t.Fatal(err)
	}
	if err := d.ActivateSpotifyMetadataContext("fresh"); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	fence := SpotifyMetadataFence{Epoch: "first", ContextKey: "fresh"}
	if err := d.PutSpotifyWaveformForRuntime(fence, referenceID, waveformFixture(t), "fixture", now, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if got, err := d.GetSpotifyActiveAudioArtifactForRuntime(fence, referenceID, "three_band_waveform", "spotify_three_band"); err != nil || got == nil {
		t.Fatal(got, err)
	}
	if _, err := d.ReserveSpotifyMetadataRuntime("second"); err != nil {
		t.Fatal(err)
	}
	if err := d.ActivateSpotifyMetadataContext("fresh"); err != nil {
		t.Fatal(err)
	}
	if got, err := d.GetSpotifyActiveAudioArtifactForRuntime(fence, referenceID, "three_band_waveform", "spotify_three_band"); got != nil || !errors.Is(err, ErrSpotifyMetadataRuntimeSuperseded) {
		t.Fatal("stale read accepted", got, err)
	}
	fence.Epoch = "second"
	if got, err := d.GetSpotifyActiveAudioArtifactForRuntime(fence, referenceID, "three_band_waveform", "spotify_three_band"); err != nil || got == nil {
		t.Fatal(got, err)
	}
}

func TestRuntimeCooldownExcludesForeignAndUnknownOwner(t *testing.T) {
	d, err := New(filepath.Join(t.TempDir(), "cooldown-read.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if _, err := d.ReserveSpotifyMetadataRuntime("epoch"); err != nil {
		t.Fatal(err)
	}
	if _, err := d.ConfirmSpotifyMetadataOwner("epoch", "oauth", "account", "owner"); err != nil {
		t.Fatal(err)
	}
	fence := SpotifyMetadataFence{Epoch: "epoch", ContextKey: "owner"}
	now := time.Now()
	status := ExternalAnalysisStatus{Code: spotifyanalysis.RateLimited, CheckedAt: now, RetryAt: now.Add(time.Minute)}
	if err := d.PutExternalAnalysisStatusForRuntime(fence, referenceID, "audio_features", status); err != nil {
		t.Fatal(err)
	}
	status.RetryAt = now.Add(time.Hour)
	if err := d.PutExternalAnalysisStatus(referenceID, "audio_analysis", status); err != nil {
		t.Fatal(err)
	}
	for _, owner := range []string{"owner", "foreign"} {
		delay := 2 * time.Minute
		if owner == "foreign" {
			delay = 2 * time.Hour
		}
		key := SpotifySnapshotKey{EntityType: "track", SpotifyID: referenceID, Resource: "three_band_waveform", ContextKey: owner}
		if err := d.PutSpotifyMetadataResourceStatus(key, SpotifyMetadataResourceStatus{State: "cooldown", Reason: "rate_limited", CheckedAt: now, RetryAt: now.Add(delay)}); err != nil {
			t.Fatal(err)
		}
	}
	if got, err := d.GetExternalAnalysisCooldownForRuntime(fence); err != nil || got.UnixMilli() != now.Add(2*time.Minute).UnixMilli() {
		t.Fatal(got, err)
	}
	if got, err := d.GetExternalAnalysisCooldown(); err != nil || got.UnixMilli() != now.Add(2*time.Hour).UnixMilli() {
		t.Fatal("legacy compatibility", got, err)
	}
	if _, err := d.ReserveSpotifyMetadataRuntime("next"); err != nil {
		t.Fatal(err)
	}
	if _, err := d.GetExternalAnalysisCooldownForRuntime(fence); !errors.Is(err, ErrSpotifyMetadataRuntimeSuperseded) {
		t.Fatal("superseded cooldown admitted", err)
	}
}

func TestScoreSummaryRequiresCurrentSourceAndPreservesZero(t *testing.T) {
	d, err := New(filepath.Join(t.TempDir(), "summary.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if _, err := d.ReserveSpotifyMetadataRuntime("epoch"); err != nil {
		t.Fatal(err)
	}
	if _, err := d.ConfirmSpotifyMetadataOwner("epoch", "oauth", "account", "owner"); err != nil {
		t.Fatal(err)
	}
	if err := d.SaveSong(&Song{ID: "song", Title: "Song", Artist: "Artist", Album: "Album", AddedAt: 1}); err != nil {
		t.Fatal(err)
	}
	if err := d.RefreshTrackAnalysisSourceRevision("song", "fp"); err != nil {
		t.Fatal(err)
	}
	if ok, err := d.ConfirmSpotifyRecording("song", referenceID, "fp", true); err != nil || !ok {
		t.Fatal(ok, err)
	}
	zero := 0.0
	now := time.Now()
	if err := d.PutExternalAnalysis(spotifyanalysis.Observation{TrackID: referenceID, AccountContext: "owner", Source: "spotify_internal", SourceEndpoint: "audio_features", RetrievedAt: now, Energy: &zero}, "fixture", now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	bpm, loudness, duration := 123.0, -9.0, 180.0
	tonic, mode, meter := 0, 0, 4
	camelot := "5A"
	if err := d.PutExternalAnalysis(spotifyanalysis.Observation{TrackID: referenceID, AccountContext: "owner", Source: "spotify_internal", SourceEndpoint: "audio_analysis", RetrievedAt: now, BPM: &bpm, Key: &tonic, Mode: &mode, Camelot: &camelot, LoudnessDB: &loudness, DurationSeconds: &duration, TimeSignature: &meter}, "fixture", now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	fence := SpotifyMetadataReadFence{Epoch: "epoch", ContextKey: "owner"}
	got, err := d.GetSpotifyScoreSummariesForRuntime(fence, map[string]string{"song": "fp"}, now)
	if err != nil || len(got["song"]) != 1 || got["song"]["spotify_energy_score"].Value != 0 {
		t.Fatal(got, err)
	}
	if got["song"]["spotify_energy_score"].ExpiresAt.UnixMilli() != now.Add(time.Hour).UnixMilli() {
		t.Fatal("summary expiry lost", got)
	}
	batch, err := d.GetSpotifyScalarCandidateBatchForRuntime(fence, map[string]string{"song": "fp"}, now)
	if err != nil || len(batch["song"]) != 6 {
		t.Fatalf("all scalar batch: %+v %v", batch, err)
	}
	keys := map[string]bool{}
	for _, field := range batch["song"] {
		keys[field.Key] = true
		if field.AdapterRevision != "fixture" {
			t.Fatal("lost adapter provenance")
		}
	}
	for _, key := range []string{"tempo_bpm", "key_mode", "provider_loudness_db", "time_signature", "duration_seconds", "spotify_energy_score"} {
		if !keys[key] {
			t.Fatalf("missing %s", key)
		}
	}
	got, err = d.GetSpotifyScoreSummariesForRuntime(fence, map[string]string{"song": "changed"}, now)
	if err != nil || len(got) != 0 {
		t.Fatal("old source score exposed", got, err)
	}
	captured := batch
	if err := d.DeleteSpotifyRecording("song"); err != nil {
		t.Fatal(err)
	}
	filtered, err := d.RevalidateSpotifyScalarCandidateBatch(map[string]string{"song": "fp"}, captured)
	if err != nil || len(filtered) != 0 {
		t.Fatalf("captured unlink evidence applied: %+v %v", filtered, err)
	}
	if ok, err := d.ConfirmSpotifyRecording("song", "AAAAAAAAAAAAAAAAAAAAAA", "fp", true); err != nil || !ok {
		t.Fatal(ok, err)
	}
	filtered, err = d.RevalidateSpotifyScalarCandidateBatch(map[string]string{"song": "fp"}, captured)
	if err != nil || len(filtered) != 0 {
		t.Fatalf("captured old recording borrowed same source: %+v %v", filtered, err)
	}
	if ok, err := d.ConfirmSpotifyRecording("song", referenceID, "fp", true); err != nil || !ok {
		t.Fatal(ok, err)
	}
	filtered, err = d.RevalidateSpotifyScalarCandidateBatch(map[string]string{"song": "fp"}, captured)
	if err != nil || len(filtered["song"]) != 6 {
		t.Fatalf("restored explicit recording lost: %+v %v", filtered, err)
	}
	filtered, err = d.RevalidateSpotifyScalarCandidateBatch(map[string]string{"song": "replaced"}, captured)
	if err != nil || len(filtered) != 0 {
		t.Fatalf("captured old source applied: %+v %v", filtered, err)
	}

}

func TestFreshTrackCatalogRequiresFullResourceAndActiveRuntime(t *testing.T) {
	for _, resource := range []string{"getTrack:related", "queryTrack:page:0:10", "getTrack:page:0:0", "rest:/v1/tracks/" + referenceID + ":page::"} {
		t.Run(resource, func(t *testing.T) {
			d, err := New(filepath.Join(t.TempDir(), "catalog.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer d.Close()
			if _, err = d.ReserveSpotifyMetadataRuntime("epoch"); err != nil {
				t.Fatal(err)
			}
			if _, err = d.ConfirmSpotifyMetadataOwner("epoch", "oauth", "account", "context"); err != nil {
				t.Fatal(err)
			}
			now := time.Now()
			expiry := now.Add(time.Hour)
			fence := SpotifyMetadataFence{Epoch: "epoch", ContextKey: "context"}
			snapshot := SpotifyEntitySnapshot{SpotifySnapshotKey: SpotifySnapshotKey{EntityType: "track", SpotifyID: referenceID, Resource: resource, ContextKey: "context"}, SchemaVersion: 1, AdapterRevision: "fixture", Payload: []byte(`{"id":"` + referenceID + `"}`), RetrievedAt: now, ExpiresAt: expiry}
			if err = d.PutSpotifyEntitySnapshot(snapshot); err != nil {
				t.Fatal(err)
			}
			fresh, err := d.HasFreshSpotifyTrackCatalogForRuntime(fence, referenceID, now)
			want := resource == "getTrack:page:0:0" || resource == "rest:/v1/tracks/"+referenceID+":page::"
			if err != nil || fresh != want {
				t.Fatalf("resource admission %v %v want %v", fresh, err, want)
			}
			if fresh, err = d.HasFreshSpotifyTrackCatalogForRuntime(fence, referenceID, expiry); err != nil || fresh {
				t.Fatalf("deadline admission %v %v", fresh, err)
			}
			if _, err = d.ReserveSpotifyMetadataRuntime("replacement"); err != nil {
				t.Fatal(err)
			}
			if fresh, err = d.HasFreshSpotifyTrackCatalogForRuntime(fence, referenceID, now); !errors.Is(err, ErrSpotifyMetadataRuntimeSuperseded) || fresh {
				t.Fatalf("retired runtime admission %v %v", fresh, err)
			}
		})
	}
}
