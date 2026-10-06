package db

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSpotifySnapshotBatchRollbackAndCommit(t *testing.T) {
	d, err := New(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	now := time.Now().UTC()
	first := SpotifyEntitySnapshot{SpotifySnapshotKey: SpotifySnapshotKey{EntityType: "playlist", SpotifyID: strings.Repeat("P", 22), Resource: "page:0", ContextKey: "account"}, SchemaVersion: 1, AdapterRevision: "fixture", RetrievedAt: now, ExpiresAt: now.Add(time.Hour), Payload: []byte(`{"revision":"old"}`)}
	if err := d.PutSpotifyEntitySnapshot(first); err != nil {
		t.Fatal(err)
	}
	updated := first
	updated.RetrievedAt = now.Add(time.Second)
	updated.Payload = []byte(`{"revision":"new"}`)
	second := updated
	second.Resource = "page:100"
	second.Relations = []SpotifyEntityRelation{{Kind: "tracks", Position: -1}}
	if err := d.PutSpotifyEntitySnapshots([]SpotifyEntitySnapshot{updated, second}); err == nil {
		t.Fatal("invalid second entity accepted")
	}
	got, err := d.GetSpotifyEntitySnapshot(first.SpotifySnapshotKey)
	if err != nil || got == nil || !strings.Contains(string(got.Payload), "old") {
		t.Fatal("failed batch changed earlier snapshot", err)
	}
	got, err = d.GetSpotifyEntitySnapshot(second.SpotifySnapshotKey)
	if err != nil || got != nil {
		t.Fatal("failed batch created later snapshot", err)
	}
	second.Relations = nil
	if err := d.PutSpotifyEntitySnapshots([]SpotifyEntitySnapshot{updated, second}); err != nil {
		t.Fatal(err)
	}
	for _, value := range []SpotifyEntitySnapshot{updated, second} {
		got, err := d.GetSpotifyEntitySnapshot(value.SpotifySnapshotKey)
		if err != nil || got == nil || !strings.Contains(string(got.Payload), "new") {
			t.Fatal("successful batch incomplete", err)
		}
	}
}
