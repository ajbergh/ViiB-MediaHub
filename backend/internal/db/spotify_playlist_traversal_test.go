package db

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"
)

func TestSpotifyPlaylistPublicationGenerationAndRollback(t *testing.T) {
	d, err := New(filepath.Join(t.TempDir(), "playlist.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if err := d.ActivateSpotifyMetadataContext("owner"); err != nil {
		t.Fatal(err)
	}
	old, err := d.BeginSpotifyPlaylistTraversal("owner", referenceID)
	if err != nil {
		t.Fatal(err)
	}
	current, err := d.BeginSpotifyPlaylistTraversal("owner", referenceID)
	if err != nil || current.Generation <= old.Generation {
		t.Fatal("generation did not advance", current, err)
	}
	now := time.Now()
	s := SpotifyEntitySnapshot{SpotifySnapshotKey: SpotifySnapshotKey{EntityType: "playlist", SpotifyID: referenceID, Resource: "page", ContextKey: "owner"}, SchemaVersion: 1, AdapterRevision: "fixture", RetrievedAt: now, ExpiresAt: now.Add(time.Hour), Payload: []byte(`{"revision":"new"}`)}
	if err := d.PutSpotifyPlaylistSnapshots(old, []SpotifyEntitySnapshot{s}, true); !errors.Is(err, ErrSpotifyTraversalSuperseded) {
		t.Fatal("stale publication allowed", err)
	}
	invalid := s
	invalid.Resource = "invalid"
	invalid.Payload = []byte(`bad-json`)
	if err := d.PutSpotifyPlaylistSnapshots(current, []SpotifyEntitySnapshot{s, invalid}, true); err == nil {
		t.Fatal("invalid batch committed")
	}
	if got, err := d.GetSpotifyEntitySnapshot(s.SpotifySnapshotKey); err != nil || got != nil {
		t.Fatal("failed batch published prefix", got, err)
	}
	marker := s
	marker.Resource = "playlist_traversal_partial_v1"
	marker.Payload = []byte(fmt.Sprintf(`{"revision":"new","generation":%d,"complete":true}`, current.Generation))
	if err := d.PutSpotifyPlaylistSnapshots(current, []SpotifyEntitySnapshot{s, marker}, true); err != nil {
		t.Fatal("failed batch advanced completion", err)
	}
	if err := d.PutSpotifyPlaylistSnapshots(current, []SpotifyEntitySnapshot{s}, false); !errors.Is(err, ErrSpotifyTraversalSuperseded) {
		t.Fatal("completed generation reopened", err)
	}
	next, err := d.BeginSpotifyPlaylistTraversal("owner", referenceID)
	if err != nil {
		t.Fatal(err)
	}
	marker.Payload = []byte(fmt.Sprintf(`{"revision":"new","generation":%d,"nextOffset":100}`, next.Generation))
	if err := d.PutSpotifyPlaylistSnapshots(next, []SpotifyEntitySnapshot{marker}, false); err != nil {
		t.Fatal("fresh generation could not resume", err)
	}
	got, err := d.GetSpotifyEntitySnapshot(marker.SpotifySnapshotKey)
	var prefix struct {
		Generation int64
		NextOffset int
		Complete   bool
	}
	if err != nil || got == nil || json.Unmarshal(got.Payload, &prefix) != nil || prefix.Generation != next.Generation || prefix.NextOffset != 100 || prefix.Complete {
		t.Fatal("fresh prefix not stored", got, err)
	}
	if err := d.ActivateSpotifyMetadataContext("replacement"); err != nil {
		t.Fatal(err)
	}
	if err := d.PutSpotifyPlaylistSnapshots(next, []SpotifyEntitySnapshot{s}, false); !errors.Is(err, ErrSpotifyTraversalSuperseded) {
		t.Fatal("retired account publication allowed", err)
	}
	if _, err := d.BeginSpotifyPlaylistTraversal("owner", referenceID); err == nil {
		t.Fatal("retired account acquired owner")
	}
}
