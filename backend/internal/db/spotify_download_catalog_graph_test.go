package db

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// A tiny reachable graph in a large unrelated catalog must use identity-keyed
// lookups. Previously SQLite scanned relations by entity_type alone, making a
// completion hold the writer for tens of seconds despite retaining a few rows.
func TestDownloadedCatalogGraphUsesTargetedRelationLookups(t *testing.T) {
	d, path := evidenceFixture(t)
	if err := d.ActivateSpotifyMetadataContext("owner"); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	album, artist := "AAAAAAAAAAAAAAAAAAAAAA", "BBBBBBBBBBBBBBBBBBBBBB"
	root := SpotifyEntitySnapshot{SpotifySnapshotKey: SpotifySnapshotKey{EntityType: "track", SpotifyID: referenceID, Resource: "catalog", ContextKey: "owner"}, SchemaVersion: 1, AdapterRevision: "fixture", Payload: []byte(`{}`), RetrievedAt: now, ExpiresAt: now.Add(time.Hour)}
	root.Relations = []SpotifyEntityRelation{{Kind: "album", ChildType: "album", ChildID: album}, {Kind: "artist", ChildType: "artist", ChildID: artist}}
	if err := d.PutSpotifyEntitySnapshot(root); err != nil {
		t.Fatal(err)
	}
	for _, node := range []struct{ kind, id, childKind, childID string }{{"album", album, "artist", artist}, {"artist", artist, "album", album}} {
		snapshot := root
		snapshot.EntityType, snapshot.SpotifyID = node.kind, node.id
		snapshot.Relations = []SpotifyEntityRelation{{Kind: "related", ChildType: node.childKind, ChildID: node.childID}}
		if err := d.PutSpotifyEntitySnapshot(snapshot); err != nil {
			t.Fatal(err)
		}
	}
	tx, err := d.conn.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	// Use set-based fixture seeding to represent a production-sized catalog
	// without spending the test's time constructing provider responses.
	_, err = tx.Exec(`WITH RECURSIVE n(i) AS (SELECT 1 UNION ALL SELECT i+1 FROM n WHERE i<50000)
	 INSERT INTO spotify_entity_snapshots(entity_type,spotify_id,resource,context_key,schema_version,adapter_revision,payload,payload_hash,retrieved_at,expires_at)
	 SELECT 'track',printf('%022d',i),'catalog','owner',1,'fixture',X'7B7D','fixture',?,? FROM n`, now.UnixMilli(), now.Add(time.Hour).UnixMilli())
	if err != nil {
		t.Fatal(err)
	}
	_, err = tx.Exec(`INSERT INTO spotify_entity_relations(entity_type,spotify_id,resource,context_key,relation_kind,position,child_type,child_id,unavailable,metadata_json)
	 SELECT entity_type,spotify_id,resource,context_key,'artist',0,'artist','AAAAAAAAAAAAAAAAAAAAAA',0,X'7B7D'
	 FROM spotify_entity_snapshots WHERE entity_type='track' AND spotify_id<>?`, referenceID)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	query := downloadedCatalogGraphCTE + ` SELECT (SELECT COUNT(*) FROM eligible),(SELECT COUNT(*) FROM relations)`
	rows, err := d.conn.Query("EXPLAIN QUERY PLAN "+query, now.UnixMilli(), referenceID)
	if err != nil {
		t.Fatal(err)
	}
	targeted := 0
	for rows.Next() {
		var id, parent, unused int
		var detail string
		if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
			rows.Close()
			t.Fatal(err)
		}
		if detail == "SCAN r" || strings.HasPrefix(detail, "SCAN r ") || strings.HasPrefix(detail, "SEARCH r ") {
			for _, key := range []string{"entity_type=?", "spotify_id=?", "resource=?", "context_key=?"} {
				if !strings.Contains(detail, key) {
					rows.Close()
					t.Fatalf("broad relation traversal: %s", detail)
				}
			}
			targeted++
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		t.Fatal(err)
	}
	rows.Close()
	if targeted < 2 {
		t.Fatalf("expected keyed recursive and retained relation lookups, got %d", targeted)
	}
	if err := d.AddDownload(&SpotifyDownload{ID: "large-catalog", SpotifyID: referenceID, Type: "track", Status: "queued"}); err != nil {
		t.Fatal(err)
	}
	if changed, err := d.MarkDownloadStarted("large-catalog"); err != nil || !changed {
		t.Fatalf("start: %v %v", changed, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	started := time.Now()
	if changed, err := d.MarkDownloadCompletedWithEvidence(ctx, "large-catalog", path); err != nil || !changed {
		t.Fatalf("completion: %v %v", changed, err)
	}
	t.Logf("completion with 50,000 unrelated catalog rows: %v", time.Since(started))
	var imports, relations int
	if err := d.conn.QueryRow(`SELECT count(*) FROM spotify_download_catalog_imports WHERE recording_id=?`, referenceID).Scan(&imports); err != nil {
		t.Fatal(err)
	}
	if err := d.conn.QueryRow(`SELECT count(*) FROM spotify_download_catalog_relations WHERE recording_id=?`, referenceID).Scan(&relations); err != nil {
		t.Fatal(err)
	}
	if imports != 3 || relations != 4 {
		t.Fatalf("unrelated catalog retained: imports=%d relations=%d", imports, relations)
	}
}

func TestDownloadedCatalogReachableGraphLifecycle(t *testing.T) {
	for _, mode := range []string{"cycle", "missing", "unavailable"} {
		t.Run(mode, func(t *testing.T) {
			d, path := evidenceFixture(t)
			if err := d.ActivateSpotifyMetadataContext("owner"); err != nil {
				t.Fatal(err)
			}
			now := time.Now()
			album := "AAAAAAAAAAAAAAAAAAAAAA"
			artist := "BBBBBBBBBBBBBBBBBBBBBB"
			sibling := "CCCCCCCCCCCCCCCCCCCCCC"
			missing := "DDDDDDDDDDDDDDDDDDDDDD"
			edge := func(kind, typ, id string, pos int) SpotifyEntityRelation {
				return SpotifyEntityRelation{Kind: kind, ChildType: typ, ChildID: id, Position: pos}
			}
			put := func(typ, id, resource string, expired bool, relations ...SpotifyEntityRelation) {
				t.Helper()
				expires := now.Add(time.Hour)
				if expired {
					expires = now.Add(-time.Minute)
				}
				if err := d.PutSpotifyEntitySnapshot(SpotifyEntitySnapshot{SpotifySnapshotKey: SpotifySnapshotKey{EntityType: typ, SpotifyID: id, Resource: resource, ContextKey: "owner"}, SchemaVersion: 1, AdapterRevision: "fixture", Payload: []byte(`{"name":"graph"}`), RetrievedAt: now.Add(-time.Hour), ExpiresAt: expires, Relations: relations}); err != nil {
					t.Fatal(err)
				}
			}
			put("track", referenceID, "catalog", false, edge("members", "album", album, 0), edge("members", "artist", artist, 1))
			nested := edge("missing", "artist", missing, 2)
			if mode == "unavailable" {
				nested.Unavailable = true
			}
			albumEdges := []SpotifyEntityRelation{edge("members", "artist", artist, 0), edge("tracks", "track", sibling, 0)}
			typedUnavailable := edge("items", "episode", "", 3)
			typedUnavailable.Unavailable = true
			albumEdges = append(albumEdges, typedUnavailable)
			if mode != "cycle" {
				albumEdges = append(albumEdges, nested)
			}
			put("album", album, "catalog", false, albumEdges...)
			put("artist", artist, "catalog", false, edge("members", "album", album, 0))
			// An expired resource on a reachable node must not introduce another child.
			put("artist", artist, "expired", true, edge("members", "artist", missing, 0))
			put("track", sibling, "catalog", false, edge("members", "artist", missing, 0))
			put("artist", missing, "unrelated", false)
			if mode == "missing" {
				if _, err := d.conn.Exec("DELETE FROM spotify_entity_snapshots WHERE entity_type='artist' AND spotify_id=?", missing); err != nil {
					t.Fatal(err)
				}
			}
			put("playlist", sibling, "catalog", false, edge("members", "track", referenceID, 0))
			finishEvidence(t, d, path, "job", referenceID)
			fp := scanEvidence(t, d, path, "song")
			if _, err := d.DeleteCompletedDownloads(); err != nil {
				t.Fatal(err)
			}
			if err := d.RetireSpotifyMetadataContext("owner"); err != nil {
				t.Fatal(err)
			}
			d.Close()
			reopened, err := New(filepath.Join(filepath.Dir(path), "library.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer reopened.Close()
			got, err := reopened.GetDownloadedSpotifyCatalog(t.Context(), "song", fp)
			if err != nil || got == nil {
				t.Fatalf("read: %+v %v", got, err)
			}
			state := "available"
			if mode == "missing" {
				state = "incomplete"
			}
			if got.CatalogStatus == nil || got.CatalogStatus.State != state || got.CatalogStatus.Scope != downloadedCatalogGraphScope {
				t.Fatalf("outcome: %+v", got.CatalogStatus)
			}
			if len(got.Snapshots) != 3 {
				t.Fatalf("unrelated or stale nodes captured: %+v", got.Snapshots)
			}
			expected := 6
			if mode != "cycle" {
				expected++
			}
			if len(got.Relations) != expected {
				t.Fatalf("relations=%d expected=%d", len(got.Relations), expected)
			}
			parents := map[string]bool{}
			for _, r := range got.Relations {
				if r.Kind == "members" && r.Position == 0 {
					parents[r.ParentType] = true
				}
			}
			if len(parents) != 3 {
				t.Fatalf("parent identities collided: %+v", parents)
			}
			foundUnavailableEpisode := false
			for _, relation := range got.Relations {
				if relation.ParentType == "album" && relation.ParentID == album && relation.Kind == "items" && relation.Position == 3 && relation.ChildType == "episode" && relation.ChildID == "" && relation.Unavailable {
					foundUnavailableEpisode = true
				}
			}
			if !foundUnavailableEpisode {
				t.Fatalf("typed unavailable catalog relation was dropped: %+v", got.Relations)
			}
		})
	}
}

func TestDownloadedCatalogScopeMigration(t *testing.T) {
	d, _ := evidenceFixture(t)
	_, err := d.conn.Exec(`DROP TABLE spotify_download_catalog_import_status;
 CREATE TABLE spotify_download_catalog_import_status (
 file_path TEXT NOT NULL,content_sha256 TEXT NOT NULL,file_size INTEGER NOT NULL,mtime_ns INTEGER NOT NULL,
 recording_id TEXT NOT NULL,state TEXT NOT NULL,reason TEXT NOT NULL,checked_at INTEGER NOT NULL,
 PRIMARY KEY(file_path,content_sha256,file_size,mtime_ns,recording_id));
 INSERT INTO spotify_download_catalog_import_status VALUES('file','hash',1,2,'root','available','direct_track_bundle_retained',3);`)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := d.EnsureSpotifyMetadataSchema(); err != nil {
			t.Fatal(err)
		}
	}
	var scope, reason string
	if err := d.conn.QueryRow("SELECT scope,reason FROM spotify_download_catalog_import_status").Scan(&scope, &reason); err != nil {
		t.Fatal(err)
	}
	if scope != "track_and_direct_relations_v1" || reason != "direct_track_bundle_retained" {
		t.Fatalf("legacy outcome relabeled: %s %s", scope, reason)
	}
}

func TestDownloadedCatalogNestedLimitsPreserveLastGood(t *testing.T) {
	for _, mode := range []string{"snapshots", "relations"} {
		t.Run(mode, func(t *testing.T) {
			d, path := evidenceFixture(t)
			if err := d.ActivateSpotifyMetadataContext("owner"); err != nil {
				t.Fatal(err)
			}
			now := time.Now()
			album := "AAAAAAAAAAAAAAAAAAAAAA"
			root := SpotifyEntitySnapshot{SpotifySnapshotKey: SpotifySnapshotKey{EntityType: "track", SpotifyID: referenceID, Resource: "track", ContextKey: "owner"}, SchemaVersion: 1, AdapterRevision: "fixture", Payload: []byte(`{}`), RetrievedAt: now, ExpiresAt: now.Add(time.Hour)}
			if err := d.PutSpotifyEntitySnapshot(root); err != nil {
				t.Fatal(err)
			}
			finishEvidence(t, d, path, "first", referenceID)
			fp := scanEvidence(t, d, path, "song")
			root.Relations = []SpotifyEntityRelation{{Kind: "album", ChildType: "album", ChildID: album}}
			if err := d.PutSpotifyEntitySnapshot(root); err != nil {
				t.Fatal(err)
			}
			child := root
			child.EntityType = "album"
			child.SpotifyID = album
			child.Resource = "album"
			child.Relations = nil
			if err := d.PutSpotifyEntitySnapshot(child); err != nil {
				t.Fatal(err)
			}
			var err error
			reason := "snapshot_limit"
			if mode == "snapshots" {
				_, err = d.conn.Exec(`WITH RECURSIVE n(i) AS (SELECT 0 UNION ALL SELECT i+1 FROM n WHERE i<254)
 INSERT INTO spotify_entity_snapshots(entity_type,spotify_id,resource,context_key,schema_version,adapter_revision,payload,payload_hash,retrieved_at,expires_at)
 SELECT 'album',?,'extra-'||i,'owner',1,'fixture',x'7b7d','unused',?,? FROM n`, album, now.UnixMilli(), now.Add(time.Hour).UnixMilli())
			} else {
				reason = "relation_limit"
				_, err = d.conn.Exec(`WITH RECURSIVE n(i) AS (SELECT 0 UNION ALL SELECT i+1 FROM n WHERE i<20000)
 INSERT INTO spotify_entity_relations(entity_type,spotify_id,resource,context_key,relation_kind,position,child_type,child_id,unavailable,metadata_json)
 SELECT 'album',?,'album','owner','tracks',i,'track','',1,x'7b7d' FROM n`, album)
			}
			if err != nil {
				t.Fatal(err)
			}
			finishEvidence(t, d, path, "retry", referenceID)
			got, err := d.GetDownloadedSpotifyCatalog(t.Context(), "song", fp)
			if err != nil || got == nil || got.CatalogStatus.State != "oversized" || got.CatalogStatus.Reason != reason {
				t.Fatalf("bound: %+v %v", got, err)
			}
			if mode == "snapshots" && len(got.Snapshots) != 1 {
				t.Fatalf("last-good lost: %d", len(got.Snapshots))
			}
			expectedRelations := 0
			if mode == "snapshots" {
				expectedRelations = 1
			} // Root remains retained; nested parents do not.
			if len(got.Relations) != expectedRelations {
				t.Fatalf("relations=%d expected=%d", len(got.Relations), expectedRelations)
			}
		})
	}
}
