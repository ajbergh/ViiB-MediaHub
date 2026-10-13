package db

import (
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func TestSpotifyMetadataOwnerRestartRevalidationAndReplacement(t *testing.T) {
	path := filepath.Join(t.TempDir(), "owner.db")
	d, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { d.Close() }()
	if owner, err := d.ReserveSpotifyMetadataRuntime("epoch1"); err != nil || owner != nil {
		t.Fatal(owner, err)
	}
	ctx, err := d.ConfirmSpotifyMetadataOwner("epoch1", "oauth", "accountA", "opaqueA")
	if err != nil || ctx != "opaqueA" {
		t.Fatal(ctx, err)
	}
	now := time.Now()
	s := SpotifyEntitySnapshot{SpotifySnapshotKey: SpotifySnapshotKey{EntityType: "playlist", SpotifyID: referenceID, Resource: "checkpoint", ContextKey: ctx}, SchemaVersion: 1, AdapterRevision: "fixture", RetrievedAt: now, ExpiresAt: now.Add(time.Hour), Payload: []byte(`{"retained":true}`)}
	if err := d.PutSpotifyEntitySnapshot(s); err != nil {
		t.Fatal(err)
	}
	d.Close()
	d, err = New(path)
	if err != nil {
		t.Fatal(err)
	}
	owner, err := d.ReserveSpotifyMetadataRuntime("epoch2")
	if err != nil || owner == nil || owner.ContextKey != ctx || owner.AccountID != "accountA" {
		t.Fatal("owner not retained", owner, err)
	}
	active, err := d.GetSetting("spotify_metadata_active_context")
	if err != nil || active != "" {
		t.Fatal("unverified owner active", active, err)
	}
	if _, err := d.ConfirmSpotifyMetadataOwner("epoch1", "oauth", "accountA", "stale"); !errors.Is(err, ErrSpotifyMetadataRuntimeSuperseded) {
		t.Fatal("old runtime reactivated", err)
	}
	ctx, err = d.ConfirmSpotifyMetadataOwner("epoch2", "oauth", "accountA", "unused")
	if err != nil || ctx != "opaqueA" {
		t.Fatal("same account not reused", ctx, err)
	}
	if got, err := d.GetSpotifyEntitySnapshot(s.SpotifySnapshotKey); err != nil || got == nil {
		t.Fatal("restart lost snapshot", got, err)
	}
	if err := d.PutExternalAnalysisStatus(referenceID, "audio_features", ExternalAnalysisStatus{Code: "rate_limited", CheckedAt: now, RetryAt: now.Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	if _, err := d.conn.Exec(`CREATE TRIGGER reject_owner_activation BEFORE UPDATE ON settings WHEN NEW.key='spotify_metadata_active_context' BEGIN SELECT RAISE(ABORT,'fixture'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := d.ConfirmSpotifyMetadataOwner("epoch2", "oauth", "accountB", "opaqueB"); err == nil {
		t.Fatal("failed activation succeeded")
	}
	if got, err := d.GetSpotifyEntitySnapshot(s.SpotifySnapshotKey); err != nil || got == nil {
		t.Fatal("failed activation purged retained data", got, err)
	}
	if _, err := d.conn.Exec(`DROP TRIGGER reject_owner_activation`); err != nil {
		t.Fatal(err)
	}
	if retained, err := d.ConfirmSpotifyMetadataOwner("epoch2", "oauth", "accountA", "unused"); err != nil || retained != "opaqueA" {
		t.Fatal("failed activation changed owner", retained, err)
	}
	ctx, err = d.ConfirmSpotifyMetadataOwner("epoch2", "oauth", "accountB", "opaqueB")
	if err != nil || ctx != "opaqueB" {
		t.Fatal("replacement context not isolated", ctx, err)
	}
	if got, err := d.GetSpotifyEntitySnapshot(s.SpotifySnapshotKey); err != nil || got != nil {
		t.Fatal("replacement retained private snapshot", got, err)
	}
	if status, err := d.GetExternalAnalysisStatus(referenceID, "audio_features"); err != nil || status != nil {
		t.Fatal("replacement retained old cooldown", status, err)
	}
	if err := d.PurgeSpotifyMetadata(); err != nil {
		t.Fatal(err)
	}
	if owner, err := d.ReserveSpotifyMetadataRuntime("epoch3"); err != nil || owner != nil {
		t.Fatal("logout retained owner", owner, err)
	}
}
