package db

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/spotify/waveform"
	"google.golang.org/protobuf/encoding/protowire"
)

func waveformFixture(t *testing.T) waveform.Waveform {
	t.Helper()
	var raw []byte
	for _, field := range []struct {
		tag   protowire.Number
		value uint64
	}{{1, 44100}, {2, 20}, {3, 7}, {4, 8}, {5, 9}, {99, 10}} {
		raw = protowire.AppendTag(raw, field.tag, protowire.VarintType)
		raw = protowire.AppendVarint(raw, field.value)
	}
	w, err := waveform.DecodeDomain(raw)
	if err != nil {
		t.Fatal(err)
	}
	w.TrackID = referenceID
	w.ETag = "provider-etag"
	return w
}
func TestSpotifyWaveformPersistenceAndColumnUpgrade(t *testing.T) {
	path := filepath.Join(t.TempDir(), "waveform.db")
	d, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	// Recreate the immediately preceding schema to verify the additive upgrade.
	if _, err = d.conn.Exec("ALTER TABLE spotify_audio_artifacts DROP COLUMN provider_etag"); err != nil {
		d.Close()
		t.Fatal(err)
	}
	if err = d.EnsureSpotifyMetadataSchema(); err != nil {
		d.Close()
		t.Fatal(err)
	}
	w := waveformFixture(t)
	now := time.Unix(1700000000, 0).UTC()
	expiry := now.Add(time.Hour)
	if err = d.PutSpotifyWaveform(referenceID, "account-a", w, waveform.ContractRevision, now, expiry); err != nil {
		d.Close()
		t.Fatal(err)
	}
	invalid := w
	invalid.Lows = []int32{99}
	if err = d.PutSpotifyWaveform(referenceID, "account-a", invalid, waveform.ContractRevision, now.Add(time.Minute), expiry); err == nil {
		d.Close()
		t.Fatal("inconsistent waveform projection accepted")
	}
	d.Close()
	d, err = New(path)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	artifact, err := d.GetSpotifyAudioArtifact(referenceID, "three_band_waveform", "spotify_three_band", "account-a")
	if err != nil || artifact == nil || artifact.ProviderETag != w.ETag || artifact.AdapterRevision != waveform.ContractRevision {
		t.Fatalf("waveform lost: %+v %v", artifact, err)
	}
	restored, err := waveform.DecodeDomain(artifact.Payload)
	if err != nil || restored.Lows[0] != 7 || len(restored.Domain) != len(w.Domain) {
		t.Fatal("native waveform did not survive storage")
	}
	if artifact, err = d.GetSpotifyAudioArtifact(referenceID, "three_band_waveform", "spotify_three_band", "account-b"); err != nil || artifact != nil {
		t.Fatal("account waveform leaked")
	}
	if err = d.RetireSpotifyMetadataContext("account-a"); err != nil {
		t.Fatal(err)
	}
	if artifact, err = d.GetSpotifyAudioArtifact(referenceID, "three_band_waveform", "spotify_three_band", "account-a"); err != nil || artifact != nil {
		t.Fatal("retired waveform retained")
	}
}

func TestWaveformCooldownIsSharedWithScalarRequests(t *testing.T) {
	d, err := New(filepath.Join(t.TempDir(), "cooldown.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	now := time.Now().UTC()
	retry := now.Add(time.Hour)
	key := SpotifySnapshotKey{EntityType: "track", SpotifyID: referenceID, Resource: "three_band_waveform", ContextKey: "account-a"}
	if err = d.PutSpotifyMetadataResourceStatus(key, SpotifyMetadataResourceStatus{State: "cooldown", Reason: "rate_limited", CheckedAt: now, RetryAt: retry}); err != nil {
		t.Fatal(err)
	}
	got, err := d.GetExternalAnalysisCooldown()
	if err != nil || got.UnixMilli() != retry.UnixMilli() {
		t.Fatal("waveform cooldown not shared", got, err)
	}
}
