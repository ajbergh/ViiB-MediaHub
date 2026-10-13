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

func TestSpotifyRESTSnapshotRelationsMergeOnlyOmittedFields(t *testing.T) {
	d, err := New(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	now := time.Now().UTC()
	trackID := strings.Repeat("T", 22)
	albumA, albumB, artistID := strings.Repeat("A", 22), strings.Repeat("B", 22), strings.Repeat("C", 22)
	key := SpotifySnapshotKey{EntityType: "track", SpotifyID: trackID, Resource: "rest:/v1/tracks:page:::related", ContextKey: "account"}
	base := SpotifyEntitySnapshot{
		SpotifySnapshotKey: key, SchemaVersion: 1, AdapterRevision: "fixture", RetrievedAt: now, ExpiresAt: now.Add(time.Hour),
		Payload:   []byte(`{"type":"track","album":{},"artists":[{}]}`),
		Relations: []SpotifyEntityRelation{{Kind: "album", Position: 0, ChildType: "album", ChildID: albumA}, {Kind: "artists", Position: 0, ChildType: "artist", ChildID: artistID}},
	}
	if err := d.PutSpotifyEntitySnapshot(base); err != nil {
		t.Fatal(err)
	}
	updated := base
	updated.RetrievedAt = now.Add(time.Second)
	updated.ExpiresAt = now.Add(time.Hour + time.Second)
	updated.Payload = []byte(`{"type":"track","name":"sparse"}`)
	updated.Relations = nil
	if err := d.PutSpotifyEntitySnapshot(updated); err != nil {
		t.Fatal(err)
	}
	got, err := d.GetSpotifyEntitySnapshot(key)
	if err != nil || got == nil || len(got.Relations) != 2 {
		t.Fatalf("sparse REST update lost relations: %+v %v", got, err)
	}

	updated.RetrievedAt = now.Add(2 * time.Second)
	updated.ExpiresAt = now.Add(time.Hour + 2*time.Second)
	updated.Payload = []byte(`{"type":"track","artists":[]}`)
	if err := d.PutSpotifyEntitySnapshot(updated); err != nil {
		t.Fatal(err)
	}
	got, err = d.GetSpotifyEntitySnapshot(key)
	if err != nil || got == nil || len(got.Relations) != 1 || got.Relations[0].Kind != "album" || got.Relations[0].ChildID != albumA {
		t.Fatalf("explicit empty artists did not replace only that relation family: %+v %v", got, err)
	}

	updated.RetrievedAt = now.Add(3 * time.Second)
	updated.ExpiresAt = now.Add(time.Hour + 3*time.Second)
	updated.Payload = []byte(`{"type":"track","album":{"id":"` + albumB + `"}}`)
	updated.Relations = []SpotifyEntityRelation{{Kind: "album", Position: 0, ChildType: "album", ChildID: albumB}}
	if err := d.PutSpotifyEntitySnapshot(updated); err != nil {
		t.Fatal(err)
	}
	got, err = d.GetSpotifyEntitySnapshot(key)
	if err != nil || got == nil || len(got.Relations) != 1 || got.Relations[0].ChildID != albumB || got.Relations[0].Unavailable {
		t.Fatalf("new authoritative album relation was not selected: %+v %v", got, err)
	}
}
