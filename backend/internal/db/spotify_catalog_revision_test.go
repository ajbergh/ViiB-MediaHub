package db

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSpotifyCaptureRevisionStorageAndMigration(t *testing.T) {
	d, path := evidenceFixture(t)
	now := time.Now().UTC()
	snapshot := SpotifyEntitySnapshot{SpotifySnapshotKey: SpotifySnapshotKey{EntityType: "playlist", SpotifyID: referenceID, Resource: "page", ContextKey: "owner"}, SchemaVersion: 1, AdapterRevision: "fixture", CaptureRevision: "version1", Payload: []byte(`{"name":"Original"}`), RetrievedAt: now, ExpiresAt: now.Add(time.Hour), Relations: []SpotifyEntityRelation{{Kind: "tracks", ChildType: "track", ChildID: referenceID, Position: 0}}}
	if err := d.PutSpotifyEntitySnapshot(snapshot); err != nil {
		t.Fatal(err)
	}
	d.Close()
	reopened, err := New(filepath.Join(filepath.Dir(path), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	got, err := reopened.GetSpotifyEntitySnapshot(snapshot.SpotifySnapshotKey)
	if err != nil || got == nil || got.CaptureRevision != "version1" || len(got.Relations) != 1 {
		t.Fatal(got, err)
	}
	hash := got.PayloadHash
	// Simulate the prior schema: its rows have no revision provenance.
	if _, err = reopened.conn.Exec(`ALTER TABLE spotify_entity_snapshots DROP COLUMN capture_revision`); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err = reopened.EnsureSpotifyMetadataSchema(); err != nil {
			t.Fatal(err)
		}
	}
	got, err = reopened.GetSpotifyEntitySnapshot(snapshot.SpotifySnapshotKey)
	if err != nil || got == nil || got.CaptureRevision != "" || got.PayloadHash != hash || len(got.Relations) != 1 {
		t.Fatal("migration invented provenance or lost data", got, err)
	}
	snapshot.CaptureRevision = "version2"
	snapshot.RetrievedAt = now.Add(time.Second)
	if err = reopened.PutSpotifyEntitySnapshot(snapshot); err != nil {
		t.Fatal(err)
	}
	// An older unbound response must not clear the current binding.
	old := snapshot
	old.CaptureRevision = ""
	old.RetrievedAt = now
	if err = reopened.PutSpotifyEntitySnapshot(old); err != nil {
		t.Fatal(err)
	}
	got, err = reopened.GetSpotifyEntitySnapshot(snapshot.SpotifySnapshotKey)
	if err != nil || got.CaptureRevision != "version2" {
		t.Fatal("older browse cleared binding", got, err)
	}
	// A newer independent browse is allowed, but it must clear the binding.
	old.RetrievedAt = now.Add(2 * time.Second)
	if err = reopened.PutSpotifyEntitySnapshot(old); err != nil {
		t.Fatal(err)
	}
	got, err = reopened.GetSpotifyEntitySnapshot(snapshot.SpotifySnapshotKey)
	if err != nil || got.CaptureRevision != "" {
		t.Fatal("new browse inherited stale binding", got, err)
	}
}

func TestSpotifyCaptureRevisionValidation(t *testing.T) {
	d, _ := evidenceFixture(t)
	now := time.Now()
	for _, revision := range []string{strings.Repeat("x", 257), "bad\nrevision"} {
		s := SpotifyEntitySnapshot{SpotifySnapshotKey: SpotifySnapshotKey{EntityType: "playlist", SpotifyID: referenceID, Resource: "page", ContextKey: "owner"}, SchemaVersion: 1, AdapterRevision: "fixture", CaptureRevision: revision, Payload: []byte(`{}`), RetrievedAt: now, ExpiresAt: now.Add(time.Hour)}
		if err := d.PutSpotifyEntitySnapshot(s); err == nil {
			t.Fatal("invalid revision accepted")
		}
	}
	s := SpotifyEntitySnapshot{SpotifySnapshotKey: SpotifySnapshotKey{EntityType: "track", SpotifyID: referenceID, Resource: "page", ContextKey: "owner"}, SchemaVersion: 1, AdapterRevision: "fixture", CaptureRevision: "playlist-revision", Payload: []byte(`{}`), RetrievedAt: now, ExpiresAt: now.Add(time.Hour)}
	if err := d.PutSpotifyEntitySnapshot(s); err == nil {
		t.Fatal("unrelated entity bound to playlist revision")
	}
}
