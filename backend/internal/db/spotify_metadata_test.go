package db

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"
)

func TestSpotifySnapshotSameMillisecondReplacement(t *testing.T) {
	d, err := New(filepath.Join(t.TempDir(), "metadata.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	now := time.UnixMilli(1700000000000)
	snapshot := SpotifyEntitySnapshot{SpotifySnapshotKey: SpotifySnapshotKey{EntityType: "playlist", SpotifyID: referenceID, Resource: "playlist_traversal_partial_v1", ContextKey: "account-a"}, SchemaVersion: 1, AdapterRevision: "fixture", RetrievedAt: now, ExpiresAt: now.Add(time.Hour), Payload: []byte(`{"nextOffset":1}`)}
	if err := d.PutSpotifyEntitySnapshot(snapshot); err != nil {
		t.Fatal(err)
	}
	snapshot.RetrievedAt = now.Add(100 * time.Microsecond)
	snapshot.Payload = []byte(`{"complete":true}`)
	if err := d.PutSpotifyEntitySnapshot(snapshot); err != nil {
		t.Fatal(err)
	}
	got, err := d.GetSpotifyEntitySnapshot(snapshot.SpotifySnapshotKey)
	if err != nil || got == nil || !bytes.Contains(got.Payload, []byte(`"complete":true`)) {
		t.Fatal("same millisecond completion discarded", got, err)
	}
}

func TestSpotifyPartialProgressCannotRegress(t *testing.T) {
	d, err := New(filepath.Join(t.TempDir(), "metadata.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	now := time.Now()
	s := SpotifyEntitySnapshot{SpotifySnapshotKey: SpotifySnapshotKey{EntityType: "playlist", SpotifyID: referenceID, Resource: "playlist_traversal_partial_v1", ContextKey: "account-a"}, SchemaVersion: 1, AdapterRevision: "fixture", RetrievedAt: now, ExpiresAt: now.Add(time.Hour)}
	for i, raw := range []string{`{"revision":"r1","nextOffset":200}`, `{"revision":"r1","nextOffset":100}`, `{"revision":"r1","complete":true}`, `{"revision":"r1","nextOffset":300}`, `{"revision":"r2","nextOffset":100}`} {
		s.Payload = []byte(raw)
		s.RetrievedAt = now.Add(time.Duration(i) * time.Second)
		if err := d.PutSpotifyEntitySnapshot(s); err != nil {
			t.Fatal(err)
		}
		got, err := d.GetSpotifyEntitySnapshot(s.SpotifySnapshotKey)
		want := raw
		if i == 1 {
			want = `{"revision":"r1","nextOffset":200}`
		}
		if i == 3 {
			want = `{"revision":"r1","complete":true}`
		}
		var expected, actual any
		if err == nil && got != nil {
			json.Unmarshal([]byte(want), &expected)
			json.Unmarshal(got.Payload, &actual)
		}
		expectedJSON, _ := json.Marshal(expected)
		actualJSON, _ := json.Marshal(actual)
		if err != nil || got == nil || !bytes.Equal(actualJSON, expectedJSON) {
			t.Fatalf("write %d regressed: %+v %v", i, got, err)
		}
	}
}

func TestSpotifyMetadataMigrationRelationsAndIsolation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "metadata.db")
	d, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1700000000, 0).UTC()
	key := SpotifySnapshotKey{EntityType: "playlist", SpotifyID: referenceID, Resource: "playlist", ContextKey: "account-a"}
	snapshot := SpotifyEntitySnapshot{SpotifySnapshotKey: key, SchemaVersion: 1, AdapterRevision: "fixture", RetrievedAt: now, ExpiresAt: now.Add(time.Hour), Payload: []byte(`{"name":"Example","unknown":false,"access_token":"secret"}`), Relations: []SpotifyEntityRelation{
		{Kind: "items", Position: 0, ChildType: "track", ChildID: referenceID},
		{Kind: "items", Position: 1, ChildType: "unavailable", Unavailable: true},
		{Kind: "items", Position: 2, ChildType: "track", ChildID: referenceID},
	}}
	if err = d.PutSpotifyEntitySnapshot(snapshot); err != nil {
		d.Close()
		t.Fatal(err)
	}
	older := snapshot
	older.RetrievedAt = now.Add(-time.Hour)
	older.Relations = nil
	older.Payload = []byte(`{"name":"Old"}`)
	if err = d.PutSpotifyEntitySnapshot(older); err != nil {
		d.Close()
		t.Fatal(err)
	}
	failure := SpotifyMetadataResourceStatus{State: "unavailable", Reason: "not_found", CheckedAt: now.Add(time.Minute), RetryAt: now.Add(time.Hour)}
	if err = d.PutSpotifyMetadataResourceStatus(key, failure); err != nil {
		d.Close()
		t.Fatal(err)
	}
	d.Close()
	d, err = New(path)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if err = d.EnsureSpotifyMetadataSchema(); err != nil {
		t.Fatal(err)
	}
	got, err := d.GetSpotifyEntitySnapshot(key)
	if err != nil || got == nil || len(got.Relations) != 3 || !got.Relations[1].Unavailable || bytes.Contains(got.Payload, []byte("secret")) || !bytes.Contains(got.Payload, []byte(`"unknown":false`)) {
		t.Fatalf("snapshot lost: %+v %v", got, err)
	}
	other := key
	other.ContextKey = "account-b"
	if got, err = d.GetSpotifyEntitySnapshot(other); err != nil || got != nil {
		t.Fatal("account data leaked")
	}
	snapshot.Relations = append(snapshot.Relations, snapshot.Relations[0])
	snapshot.RetrievedAt = now.Add(time.Minute)
	if err = d.PutSpotifyEntitySnapshot(snapshot); err == nil {
		t.Fatal("duplicate position accepted")
	}
	got, err = d.GetSpotifyEntitySnapshot(key)
	if err != nil || len(got.Relations) != 3 {
		t.Fatal("failed write damaged last good snapshot")
	}
	status, err := d.GetSpotifyMetadataResourceStatus(key)
	if err != nil || status == nil || status.State != "unavailable" {
		t.Fatal("attempt status lost")
	}
	if err = d.RetireSpotifyMetadataContext("account-a"); err != nil {
		t.Fatal(err)
	}
	if got, err = d.GetSpotifyEntitySnapshot(key); err != nil || got != nil {
		t.Fatal("retired account snapshot retained")
	}
}
