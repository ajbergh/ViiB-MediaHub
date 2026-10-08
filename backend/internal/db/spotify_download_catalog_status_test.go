package db

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDownloadCatalogOutcomesIndependentAndSourceBound(t *testing.T) {
	for _, mode := range []string{"available", "not_available", "missing_related", "related_only", "snapshot_count", "snapshot_bytes", "relation_count", "relation_bytes"} {
		t.Run(mode, func(t *testing.T) {
			d, path := evidenceFixture(t)
			if err := d.ActivateSpotifyMetadataContext("owner"); err != nil {
				t.Fatal(err)
			}
			now := time.Now()
			root := SpotifyEntitySnapshot{SpotifySnapshotKey: SpotifySnapshotKey{EntityType: "track", SpotifyID: referenceID, Resource: "track", ContextKey: "owner"}, SchemaVersion: 1, AdapterRevision: "fixture", Payload: []byte(`{"name":"retained"}`), RetrievedAt: now, ExpiresAt: now.Add(time.Hour)}
			if mode != "not_available" {
				if err := d.PutSpotifyEntitySnapshot(root); err != nil {
					t.Fatal(err)
				}
			}
			expected, reason := "available", "track_album_artist_graph_retained"
			switch mode {
			case "not_available":
				expected, reason = "not_available", "no_eligible_catalog"
			case "missing_related", "related_only":
				root.Relations = []SpotifyEntityRelation{{Kind: "artists", Position: 0, ChildType: "artist", ChildID: "AAAAAAAAAAAAAAAAAAAAAA"}}
				if mode == "related_only" {
					root.ExpiresAt = now.Add(time.Millisecond)
				}
				if err := d.PutSpotifyEntitySnapshot(root); err != nil {
					t.Fatal(err)
				}
				expected, reason = "incomplete", "related_entity_unavailable"
				if mode == "related_only" {
					child := root
					child.EntityType = "artist"
					child.SpotifyID = "AAAAAAAAAAAAAAAAAAAAAA"
					child.Relations = nil
					child.ExpiresAt = now.Add(time.Hour)
					if err := d.PutSpotifyEntitySnapshot(child); err != nil {
						t.Fatal(err)
					}
					if _, err := d.conn.Exec(`UPDATE spotify_entity_snapshots SET expires_at=retrieved_at WHERE entity_type='track'`); err != nil {
						t.Fatal(err)
					}
					reason = "root_track_unavailable"
				}
			case "snapshot_count", "snapshot_bytes":
				count, size := 256, 2
				if mode == "snapshot_bytes" {
					count, size = 4, 2097152
				}
				// Deliberately over-bound private rows are never promoted or decoded.
				for i := 0; i < count; i++ {
					if _, err := d.conn.Exec(`INSERT INTO spotify_entity_snapshots(entity_type,spotify_id,resource,context_key,schema_version,adapter_revision,payload,payload_hash,retrieved_at,expires_at) VALUES('track',?,?,'owner',1,'fixture',zeroblob(?),'fixture',?,?)`, referenceID, fmt.Sprintf("extra-%d", i), size, now.UnixMilli(), now.Add(time.Hour).UnixMilli()); err != nil {
						t.Fatal(err)
					}
				}
				expected, reason = "oversized", "snapshot_limit"
			case "relation_count", "relation_bytes":
				count, size := 20001, 2
				if mode == "relation_bytes" {
					count, size = 33, 65536
				}
				if _, err := d.conn.Exec(`WITH RECURSIVE n(i) AS (SELECT 0 UNION ALL SELECT i+1 FROM n WHERE i+1<?)
 INSERT INTO spotify_entity_relations(entity_type,spotify_id,resource,context_key,relation_kind,position,child_type,child_id,unavailable,metadata_json)
 SELECT 'track',?,'track','owner','items',i,'track','',1,zeroblob(?) FROM n`, count, referenceID, size); err != nil {
					t.Fatal(err)
				}
				expected, reason = "oversized", "relation_limit"
			}
			finishEvidence(t, d, path, "job", referenceID)
			fp := scanEvidence(t, d, path, "song")
			if _, err := d.DeleteCompletedDownloads(); err != nil {
				t.Fatal(err)
			}
			if err := d.RetireSpotifyMetadataContext("owner"); err != nil {
				t.Fatal(err)
			}
			d.Close()
			var err error
			d, err = New(filepath.Join(filepath.Dir(path), "library.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer d.Close()
			got, err := d.GetDownloadedSpotifyCatalog(t.Context(), "song", fp)
			if err != nil || got == nil || got.CatalogStatus == nil || got.CatalogStatus.State != expected || got.CatalogStatus.Reason != reason || got.CatalogStatus.Scope != "track_album_artist_graph_v2" || got.CatalogStatus.CheckedAt.IsZero() {
				t.Fatalf("%s outcome: %+v %v", mode, got, err)
			}
			var audio string
			if err := d.conn.QueryRow(`SELECT state FROM spotify_download_import_status`).Scan(&audio); err != nil || audio != "not_available" {
				t.Fatalf("catalog altered audio: %s %v", audio, err)
			}
			if mode == "snapshot_count" || mode == "snapshot_bytes" || mode == "not_available" {
				if len(got.Snapshots) != 0 || len(got.Relations) != 0 {
					t.Fatal("over-bound/absent material promoted")
				}
			}
			if mode == "relation_count" || mode == "relation_bytes" {
				if len(got.Snapshots) != 1 || len(got.Relations) != 0 {
					t.Fatal("bounded snapshots not preserved independently")
				}
			}
			if wrong, err := d.GetDownloadedSpotifyCatalog(t.Context(), "song", "changed"); err != nil || wrong != nil {
				t.Fatal("status escaped fingerprint fence", err)
			}
			if err := os.WriteFile(path, []byte("replacement"), 0600); err != nil {
				t.Fatal(err)
			}
			if wrong, err := d.GetDownloadedSpotifyCatalog(t.Context(), "song", fp); err != nil || wrong != nil {
				t.Fatal("status escaped physical fence", err)
			}
		})
	}
}

func TestDownloadCatalogAttemptPreservesLastGood(t *testing.T) {
	d, path := evidenceFixture(t)
	if err := d.ActivateSpotifyMetadataContext("owner"); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	root := SpotifyEntitySnapshot{SpotifySnapshotKey: SpotifySnapshotKey{EntityType: "track", SpotifyID: referenceID, Resource: "track", ContextKey: "owner"}, SchemaVersion: 1, AdapterRevision: "fixture", Payload: []byte(`{"name":"last good"}`), RetrievedAt: now, ExpiresAt: now.Add(time.Hour)}
	if err := d.PutSpotifyEntitySnapshot(root); err != nil {
		t.Fatal(err)
	}
	finishEvidence(t, d, path, "first", referenceID)
	fp := scanEvidence(t, d, path, "song")
	if err := d.RetireSpotifyMetadataContext("owner"); err != nil {
		t.Fatal(err)
	}
	finishEvidence(t, d, path, "second", referenceID)
	got, err := d.GetDownloadedSpotifyCatalog(t.Context(), "song", fp)
	if err != nil || got == nil || got.CatalogStatus.State != "not_available" || len(got.Snapshots) != 1 {
		t.Fatalf("retry erased last-good: %+v %v", got, err)
	}
	if err := d.ActivateSpotifyMetadataContext("owner"); err != nil {
		t.Fatal(err)
	}
	if err := d.PutSpotifyEntitySnapshot(root); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 256; i++ {
		if _, err := d.conn.Exec(`INSERT INTO spotify_entity_snapshots(entity_type,spotify_id,resource,context_key,schema_version,adapter_revision,payload,payload_hash,retrieved_at,expires_at) VALUES('track',?,?,'owner',1,'fixture',x'7b7d','fixture',?,?)`, referenceID, fmt.Sprintf("retry-%d", i), now.UnixMilli(), now.Add(time.Hour).UnixMilli()); err != nil {
			t.Fatal(err)
		}
	}
	finishEvidence(t, d, path, "third", referenceID)
	got, err = d.GetDownloadedSpotifyCatalog(t.Context(), "song", fp)
	if err != nil || got == nil || got.CatalogStatus.State != "oversized" || len(got.Snapshots) != 1 {
		t.Fatalf("oversized retry erased last-good: %+v %v", got, err)
	}
	if err := d.DeleteSpotifyRecording("song"); err != nil {
		t.Fatal(err)
	}
	if got, err := d.GetDownloadedSpotifyCatalog(t.Context(), "song", fp); err != nil || got != nil {
		t.Fatal("status escaped unlink", err)
	}
}

func TestDownloadCatalogOutcomeFailureRollsBackCompletion(t *testing.T) {
	d, path := evidenceFixture(t)
	if err := d.AddDownload(&SpotifyDownload{ID: "job", SpotifyID: referenceID, Type: "track", Title: "Fixture", Status: "queued", AddedAt: 1}); err != nil {
		t.Fatal(err)
	}
	if ok, err := d.MarkDownloadStarted("job"); err != nil || !ok {
		t.Fatal(ok, err)
	}
	if _, err := d.conn.Exec(`CREATE TRIGGER reject_catalog_outcome BEFORE INSERT ON spotify_download_catalog_import_status BEGIN SELECT RAISE(ABORT,'fixture'); END`); err != nil {
		t.Fatal(err)
	}
	if ok, err := d.MarkDownloadCompletedWithEvidence(t.Context(), "job", path); err == nil || ok {
		t.Fatal("completion succeeded without atomic outcome")
	}
	var state string
	var count int
	if err := d.conn.QueryRow(`SELECT status FROM spotify_downloads WHERE id='job'`).Scan(&state); err != nil || state != "downloading" {
		t.Fatal(state, err)
	}
	if err := d.conn.QueryRow(`SELECT COUNT(*) FROM spotify_download_evidence`).Scan(&count); err != nil || count != 0 {
		t.Fatal("partial evidence committed", count, err)
	}
	if _, err := d.conn.Exec(`DROP TRIGGER reject_catalog_outcome`); err != nil {
		t.Fatal(err)
	}
	if ok, err := d.MarkDownloadCompletedWithEvidence(t.Context(), "job", path); err != nil || !ok {
		t.Fatal("retry failed", ok, err)
	}
}

func TestDownloadCatalogCumulativeRetryLimits(t *testing.T) {
	for _, mode := range []string{"snapshots", "relations"} {
		t.Run(mode, func(t *testing.T) {
			d, path := evidenceFixture(t)
			now := time.Now()
			putBatch := func(attempt int) {
				t.Helper()
				if err := d.ActivateSpotifyMetadataContext("owner"); err != nil {
					t.Fatal(err)
				}
				count := 1
				if mode == "snapshots" {
					count = 128
					if attempt == 1 {
						count = 129
					}
				}
				for i := 0; i < count; i++ {
					root := SpotifyEntitySnapshot{SpotifySnapshotKey: SpotifySnapshotKey{EntityType: "track", SpotifyID: referenceID, Resource: fmt.Sprintf("batch-%d-%d", attempt, i), ContextKey: "owner"}, SchemaVersion: 1, AdapterRevision: "fixture", Payload: []byte(`{}`), RetrievedAt: now, ExpiresAt: now.Add(time.Hour)}
					if err := d.PutSpotifyEntitySnapshot(root); err != nil {
						t.Fatal(err)
					}
					if mode == "relations" {
						relCount := 10001
						if attempt == 1 {
							relCount = 10000
						}
						if _, err := d.conn.Exec(`WITH RECURSIVE n(i) AS (SELECT 0 UNION ALL SELECT i+1 FROM n WHERE i+1<?)
 INSERT INTO spotify_entity_relations(entity_type,spotify_id,resource,context_key,relation_kind,position,child_type,child_id,unavailable,metadata_json)
 SELECT 'track',?,?,'owner','items',i,'track','',1,x'7b7d' FROM n`, relCount, referenceID, root.Resource); err != nil {
							t.Fatal(err)
						}
					}
				}
			}
			putBatch(0)
			finishEvidence(t, d, path, "first", referenceID)
			fp := scanEvidence(t, d, path, "song")
			if err := d.RetireSpotifyMetadataContext("owner"); err != nil {
				t.Fatal(err)
			}
			putBatch(1)
			finishEvidence(t, d, path, "second", referenceID)
			got, err := d.GetDownloadedSpotifyCatalog(t.Context(), "song", fp)
			if err != nil || got == nil || got.CatalogStatus.State != "oversized" {
				t.Fatalf("cumulative bound not reported: %+v %v", got, err)
			}
			if mode == "snapshots" && len(got.Snapshots) != 128 {
				t.Fatal("snapshot retry overflowed/replaced last-good", len(got.Snapshots))
			}
			if mode == "relations" && (len(got.Snapshots) != 2 || len(got.Relations) != 10001) {
				t.Fatal("relation retry overflowed/replaced last-good", len(got.Snapshots), len(got.Relations))
			}
		})
	}
}
