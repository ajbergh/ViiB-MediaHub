package db

import (
	"bytes"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
)

func TestBoundedArtifactReadsRejectReplacementAndVisitAcrossChunks(t *testing.T) {
	d, err := New(filepath.Join(t.TempDir(), "bounded.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	put := func(id string, size int) {
		t.Helper()
		if err := d.SaveSong(&Song{ID: id, Title: id, FilePath: id + ".wav", AddedAt: 1}); err != nil {
			t.Fatal(err)
		}
		if err := d.UpsertTrackAnalysisArtifact(TrackAnalysisArtifact{ID: id, SongID: id, Kind: "fixture", FormatVersion: 1, AlgorithmVersion: "v1", Encoding: "raw", SourceFingerprint: "source", Data: bytes.Repeat([]byte{1}, size)}); err != nil {
			t.Fatal(err)
		}
	}
	put("replaced", 8)
	got, err := d.GetTrackAnalysisArtifactBounded("replaced", "fixture", 1, "v1", 8)
	if err != nil || len(got.Data) != 8 || got.SourceFingerprint != "source" {
		t.Fatalf("exact limit: %+v, %v", got, err)
	}
	// Simulate a replacement after a consumer has selected eligible metadata.
	metadata, err := d.ListTrackAnalysisArtifactMetadata("fixture", 1, "v1")
	if err != nil || len(metadata) != 1 || metadata[0].PayloadBytes != 8 {
		t.Fatalf("metadata: %+v, %v", metadata, err)
	}
	put("replaced", 9)
	got, err = d.GetTrackAnalysisArtifactBounded("replaced", "fixture", 1, "v1", 8)
	if !errors.Is(err, ErrTrackAnalysisArtifactTooLarge) || len(got.Data) != 0 {
		t.Fatalf("oversized replacement returned payload: %+v, %v", got, err)
	}
	if _, err := d.GetTrackAnalysisArtifactBounded("missing", "fixture", 1, "v1", 8); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("missing: %v", err)
	}
	ids := []string{"", "replaced", "missing"}
	for i := 0; i < 501; i++ {
		id := fmt.Sprintf("song-%03d", i)
		put(id, 8)
		ids = append(ids, id, id)
	}
	seen := map[string]bool{}
	err = d.VisitTrackAnalysisArtifactPayloadsBounded(ids, "fixture", 1, "v1", 8, func(id, fingerprint string, payload []byte) error {
		if seen[id] || id == "replaced" || fingerprint != "source" || len(payload) != 8 {
			t.Fatalf("unexpected visitor payload: %s %s %d", id, fingerprint, len(payload))
		}
		seen[id] = true
		return nil
	})
	if err != nil || len(seen) != 501 {
		t.Fatalf("chunked visit: count=%d err=%v", len(seen), err)
	}
	stop := errors.New("stop")
	if err := d.VisitTrackAnalysisArtifactPayloadsBounded(ids, "fixture", 1, "v1", 8, func(string, string, []byte) error { return stop }); !errors.Is(err, stop) {
		t.Fatalf("callback error: %v", err)
	}
	// Callback failure must release the query for subsequent reads.
	if _, err := d.GetTrackAnalysisArtifactBounded("song-000", "fixture", 1, "v1", 8); err != nil {
		t.Fatal(err)
	}
	for _, limit := range []int64{0, -1} {
		if _, err := d.GetTrackAnalysisArtifactBounded("song-000", "fixture", 1, "v1", limit); err == nil {
			t.Fatal("accepted invalid getter limit")
		}
		if err := d.VisitTrackAnalysisArtifactPayloadsBounded(ids, "fixture", 1, "v1", limit, func(string, string, []byte) error { return nil }); err == nil {
			t.Fatal("accepted invalid visitor limit")
		}
	}
}
