package db

import (
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func TestSpotifyRuntimeEpochFencesSameAccountSnapshotAndTraversal(t *testing.T) {
	d, err := New(filepath.Join(t.TempDir(), "fence.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if _, err := d.ReserveSpotifyMetadataRuntime("epoch1"); err != nil {
		t.Fatal(err)
	}
	if _, err := d.ConfirmSpotifyMetadataOwner("epoch1", "oauth", "account", "context"); err != nil {
		t.Fatal(err)
	}
	old := SpotifyMetadataFence{Epoch: "epoch1", ContextKey: "context"}
	traversal, err := d.BeginSpotifyPlaylistTraversalForRuntime(old, referenceID)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	s := SpotifyEntitySnapshot{SpotifySnapshotKey: SpotifySnapshotKey{EntityType: "playlist", SpotifyID: referenceID, Resource: "page", ContextKey: "context"}, SchemaVersion: 1, AdapterRevision: "fixture", RetrievedAt: now, ExpiresAt: now.Add(time.Hour), Payload: []byte(`{"current":true}`)}
	if err := d.PutSpotifyEntitySnapshotsForRuntime(old, []SpotifyEntitySnapshot{s}); err != nil {
		t.Fatal(err)
	}
	w := waveformFixture(t)
	if err := d.PutSpotifyWaveformForRuntime(old, referenceID, w, "fixture", now, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err := d.ReserveSpotifyMetadataRuntime("epoch2"); err != nil {
		t.Fatal(err)
	}
	if ctx, err := d.ConfirmSpotifyMetadataOwner("epoch2", "oauth", "account", "unused"); err != nil || ctx != "context" {
		t.Fatal(ctx, err)
	}
	s.Payload = []byte(`{"stale":true}`)
	s.RetrievedAt = now.Add(time.Minute)
	if err := d.PutSpotifyEntitySnapshotsForRuntime(old, []SpotifyEntitySnapshot{s}); !errors.Is(err, ErrSpotifyMetadataRuntimeSuperseded) {
		t.Fatal("old runtime snapshot allowed", err)
	}
	if err := d.PutSpotifyPlaylistSnapshots(traversal, []SpotifyEntitySnapshot{s}, true); !errors.Is(err, ErrSpotifyMetadataRuntimeSuperseded) {
		t.Fatal("old epoch traversal allowed", err)
	}
	if _, err := d.BeginSpotifyPlaylistTraversalForRuntime(old, referenceID); err == nil {
		t.Fatal("old runtime acquired traversal")
	}
	if err := d.PutSpotifyWaveformForRuntime(old, referenceID, w, "stale", now.Add(time.Minute), now.Add(time.Hour)); !errors.Is(err, ErrSpotifyMetadataRuntimeSuperseded) {
		t.Fatal("old waveform write allowed", err)
	}
	statusKey := SpotifySnapshotKey{EntityType: "track", SpotifyID: referenceID, Resource: "three_band_waveform", ContextKey: "context"}
	if err := d.PutSpotifyMetadataResourceStatusForRuntime(old, statusKey, SpotifyMetadataResourceStatus{State: "cooldown", CheckedAt: now.Add(time.Minute), RetryAt: now.Add(time.Hour)}); !errors.Is(err, ErrSpotifyMetadataRuntimeSuperseded) {
		t.Fatal("old status write allowed", err)
	}
	status, err := d.GetSpotifyMetadataResourceStatus(statusKey)
	if err != nil || status == nil || status.State != "available" {
		t.Fatal("old status replaced current availability", status, err)
	}
	got, err := d.GetSpotifyEntitySnapshot(s.SpotifySnapshotKey)
	if err != nil || got == nil || string(got.Payload) != `{"current":true}` {
		t.Fatal("old runtime replaced snapshot", got, err)
	}
	fresh := SpotifyMetadataFence{Epoch: "epoch2", ContextKey: "context"}
	foreign := s
	foreign.ContextKey = "different-account"
	if err := d.PutSpotifyEntitySnapshotsForRuntime(fresh, []SpotifyEntitySnapshot{s, foreign}); !errors.Is(err, ErrSpotifyMetadataRuntimeSuperseded) {
		t.Fatal("mixed context batch allowed", err)
	}
	got, err = d.GetSpotifyEntitySnapshot(s.SpotifySnapshotKey)
	if err != nil || got == nil || string(got.Payload) != `{"current":true}` {
		t.Fatal("mixed context batch wrote valid prefix", got, err)
	}
	if err := d.PutSpotifyEntitySnapshotsForRuntime(fresh, []SpotifyEntitySnapshot{s}); err != nil {
		t.Fatal("current runtime rejected", err)
	}
}
