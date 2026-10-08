package db

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const collectionID = "PPPPPPPPPPPPPPPPPPPPPP"

func putCollectionFixture(t *testing.T, d *DB, typ, id, resource, revision, payload string, relations ...SpotifyEntityRelation) {
	t.Helper()
	now := time.Now().Add(-time.Minute)
	if err := d.PutSpotifyEntitySnapshot(SpotifyEntitySnapshot{SpotifySnapshotKey: SpotifySnapshotKey{EntityType: typ, SpotifyID: id, Resource: resource, ContextKey: "owner"}, CaptureRevision: revision, SchemaVersion: 1, AdapterRevision: "fixture", RetrievedAt: now, ExpiresAt: now.Add(time.Hour), Payload: []byte(payload), Relations: relations}); err != nil {
		t.Fatal(err)
	}
}
func collectionOrigins() []SpotifyDownloadOrigin {
	return []SpotifyDownloadOrigin{{Kind: "playlist", ID: collectionID, Revision: "v1", Position: 0}, {Kind: "playlist", ID: collectionID, Revision: "v1", Position: 2}, {Kind: "library", ID: "saved_playlists", EntityID: collectionID, Position: -1}}
}
func collectionPages(t *testing.T, d *DB) {
	t.Helper()
	putCollectionFixture(t, d, "track", referenceID, "track", "", `{"name":"Track"}`)
	putCollectionFixture(t, d, "playlist", collectionID, "root", "v1", `{"name":"Playlist","snapshot_id":"v1"}`, SpotifyEntityRelation{Kind: "tracks", Position: 0, ChildType: "track", ChildID: referenceID}, SpotifyEntityRelation{Kind: "tracks", Position: 1, Unavailable: true})
	putCollectionFixture(t, d, "playlist", collectionID, "page-2", "v1", `{"offset":2,"items":[]}`, SpotifyEntityRelation{Kind: "tracks", Position: 2, ChildType: "track", ChildID: referenceID})
	putCollectionFixture(t, d, "playlist", collectionID, "playlist_traversal_complete_v1", "v1", `{"revision":"v1","complete":true,"rowCount":3,"items":[{"track":{"id":"`+referenceID+`"}},null,{"track":{"id":"`+referenceID+`"}}]}`)
	putCollectionFixture(t, d, "library", "rest_saved_playlists", "saved-page", "", `{"items":[],"future_domain":false}`, SpotifyEntityRelation{Kind: "library_items", Position: 8, ChildType: "playlist", ChildID: collectionID}, SpotifyEntityRelation{Kind: "library_items", Position: 9, Unavailable: true})
}
func readCollections(t *testing.T, d *DB, path string) *DownloadedSpotifyCatalog {
	t.Helper()
	fp := scanEvidence(t, d, path, "song")
	got, err := d.GetDownloadedSpotifyCatalog(t.Context(), "song", fp)
	if err != nil || got == nil || got.CollectionStatus == nil {
		t.Fatalf("collection read: %+v %v", got, err)
	}
	return got
}

func TestDownloadedCollectionsLifecycle(t *testing.T) {
	d, path := evidenceFixture(t)
	if err := d.ActivateSpotifyMetadataContext("owner"); err != nil {
		t.Fatal(err)
	}
	collectionPages(t, d)
	putCollectionFixture(t, d, "playlist", collectionID, "unbound", "", `{"name":"Unbound"}`)
	putCollectionFixture(t, d, "playlist", collectionID, "wrong-revision", "v2", `{"name":"Other revision"}`)
	putCollectionFixture(t, d, "playlist", "AAAAAAAAAAAAAAAAAAAAAA", "unrelated", "v1", `{"name":"Other playlist"}`)
	putCollectionFixture(t, d, "library", "rest_saved_playlists", "unrelated-library", "", `{}`, SpotifyEntityRelation{Kind: "library_items", Position: 0, ChildType: "playlist", ChildID: "AAAAAAAAAAAAAAAAAAAAAA"})
	id := queueLineage(t, d, "collections", collectionOrigins())
	completeLineage(t, d, path, id)
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
	if err != nil || got == nil || got.CollectionStatus == nil || got.CollectionStatus.State != "available" || got.CollectionStatus.Scope != requestedCollectionScope {
		t.Fatal(got, err)
	}
	if len(got.Snapshots) != 5 || len(got.Relations) != 5 {
		t.Fatal("unexpected collection material", len(got.Snapshots), len(got.Relations))
	}
	resources := map[string]bool{}
	positions := map[int]bool{}
	for _, s := range got.Snapshots {
		resources[s.CapturedResource] = true
		if s.CapturedResource != "" && !strings.HasPrefix(s.Resource, "request:") {
			t.Fatal(s)
		}
		if s.EntityType == "playlist" && s.CaptureRevision != "v1" {
			t.Fatal(s)
		}
	}
	for _, name := range []string{"root", "page-2", "playlist_traversal_complete_v1", "saved-page"} {
		if !resources[name] {
			t.Fatal("missing page", name)
		}
	}
	if resources["unbound"] || resources["wrong-revision"] || resources["unrelated-library"] {
		t.Fatal("unrequested data retained", resources)
	}
	for _, r := range got.Relations {
		if r.ParentType == "playlist" {
			positions[r.Position] = true
		}
		if r.Position == 1 && !r.Unavailable {
			t.Fatal("unavailable row lost")
		}
	}
	if !positions[0] || !positions[1] || !positions[2] {
		t.Fatal(positions)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	changed := append([]byte{}, original...)
	changed[0] ^= 1
	if err = os.WriteFile(path, changed, 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.Chtimes(path, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
	if wrong, e := reopened.GetDownloadedSpotifyCatalog(t.Context(), "song", fp); e != nil || wrong != nil {
		t.Fatal("changed physical bytes admitted", wrong, e)
	}
}

func TestDownloadedCollectionsIncompleteEvidence(t *testing.T) {
	for _, mode := range []string{"missing_page", "wrong_revision", "unbound_page", "legacy_library", "missing_library", "wrong_entity", "expired", "unknown_revision", "conflicting_position", "checkpoint_ids", "checkpoint_available", "checkpoint_order", "retired"} {
		t.Run(mode, func(t *testing.T) {
			d, path := evidenceFixture(t)
			if err := d.ActivateSpotifyMetadataContext("owner"); err != nil {
				t.Fatal(err)
			}
			collectionPages(t, d)
			origins := collectionOrigins()
			var err error
			switch mode {
			case "missing_page":
				_, err = d.conn.Exec(`DELETE FROM spotify_entity_snapshots WHERE resource='page-2'`)
			case "wrong_revision":
				_, err = d.conn.Exec(`UPDATE spotify_entity_snapshots SET capture_revision='v2' WHERE resource='page-2'`)
			case "unbound_page":
				_, err = d.conn.Exec(`UPDATE spotify_entity_snapshots SET capture_revision='' WHERE resource='page-2'`)
			case "legacy_library":
				origins[2].EntityID = ""
			case "missing_library":
				_, err = d.conn.Exec(`DELETE FROM spotify_entity_snapshots WHERE entity_type='library'`)
			case "wrong_entity":
				origins[2].EntityID = "AAAAAAAAAAAAAAAAAAAAAA"
			case "expired":
				_, err = d.conn.Exec(`UPDATE spotify_entity_snapshots SET expires_at=retrieved_at+1 WHERE resource='page-2'`)
			case "unknown_revision":
				origins[0].Revision = ""
				origins[1].Revision = ""
			case "checkpoint_ids":
				putCollectionFixture(t, d, "playlist", collectionID, "playlist_traversal_complete_v1", "v1", `{"revision":"v1","complete":true,"rowCount":3,"items":[{"track":{"id":"AAAAAAAAAAAAAAAAAAAAAA"}},null,{"track":{"id":"`+referenceID+`"}}]}`)
			case "checkpoint_available":
				putCollectionFixture(t, d, "playlist", collectionID, "playlist_traversal_complete_v1", "v1", `{"revision":"v1","complete":true,"rowCount":3,"items":[null,null,null]}`)
			case "checkpoint_order":
				putCollectionFixture(t, d, "playlist", collectionID, "playlist_traversal_complete_v1", "v1", `{"revision":"v1","complete":true,"rowCount":3,"items":[null,{"track":{"id":"`+referenceID+`"}},{"track":{"id":"`+referenceID+`"}}]}`)
			case "conflicting_position":
				putCollectionFixture(t, d, "playlist", collectionID, "conflict", "v1", `{}`, SpotifyEntityRelation{Kind: "tracks", Position: 0, ChildType: "track", ChildID: "AAAAAAAAAAAAAAAAAAAAAA"})
			}
			if err != nil {
				t.Fatal(err)
			}
			id := queueLineage(t, d, "missing", origins)
			if mode == "retired" {
				if err = d.RetireSpotifyMetadataContext("owner"); err != nil {
					t.Fatal(err)
				}
			}
			completeLineage(t, d, path, id)
			got := readCollections(t, d, path)
			expected := "incomplete"
			if mode == "retired" {
				expected = "not_available"
			}
			if got.CollectionStatus.State != expected {
				t.Fatalf("%s: %+v", mode, got.CollectionStatus)
			}
			if mode == "retired" {
				for _, s := range got.Snapshots {
					if s.CapturedResource != "" {
						t.Fatal("retired private pages promoted")
					}
				}
			}
		})
	}
}

func TestDownloadedCollectionsImmutableAndRollback(t *testing.T) {
	d, path := evidenceFixture(t)
	if err := d.ActivateSpotifyMetadataContext("owner"); err != nil {
		t.Fatal(err)
	}
	collectionPages(t, d)
	id := queueLineage(t, d, "rollback", collectionOrigins())
	if ok, err := d.MarkDownloadStarted(id); err != nil || !ok {
		t.Fatal(ok, err)
	}
	if _, err := d.conn.Exec(`CREATE TRIGGER reject_collection_status BEFORE INSERT ON spotify_download_collection_status BEGIN SELECT RAISE(ABORT,'fixture');END`); err != nil {
		t.Fatal(err)
	}
	if ok, err := d.MarkDownloadCompletedWithEvidence(t.Context(), id, path); err == nil || ok {
		t.Fatal("partial completion committed")
	}
	var retained, staged int
	if err := d.conn.QueryRow(`SELECT COUNT(*) FROM spotify_download_catalog_imports`).Scan(&retained); err != nil {
		t.Fatal(err)
	}
	if err := d.conn.QueryRow(`SELECT COUNT(*) FROM spotify_download_lineage_staging`).Scan(&staged); err != nil {
		t.Fatal(err)
	}
	if retained != 0 || staged != 3 {
		t.Fatal("rollback lost staging or committed material", retained, staged)
	}
	if _, err := d.conn.Exec(`DROP TRIGGER reject_collection_status`); err != nil {
		t.Fatal(err)
	}
	if ok, err := d.MarkDownloadCompletedWithEvidence(t.Context(), id, path); err != nil || !ok {
		t.Fatal(ok, err)
	}
	first := readCollections(t, d, path)
	if len(first.Snapshots) != 5 {
		t.Fatal(len(first.Snapshots))
	}
	id = queueLineage(t, d, "retry", collectionOrigins())
	completeLineage(t, d, path, id)
	if got := readCollections(t, d, path); len(got.Snapshots) != 5 {
		t.Fatal("same observation duplicated", len(got.Snapshots))
	}
	putCollectionFixture(t, d, "playlist", collectionID, "root", "v2", `{"name":"Later"}`, SpotifyEntityRelation{Kind: "tracks", Position: 0, ChildType: "track", ChildID: referenceID})
	putCollectionFixture(t, d, "playlist", collectionID, "playlist_traversal_complete_v1", "v2", `{"revision":"v2","complete":true,"rowCount":1,"items":[{"track":{"id":"`+referenceID+`"}}]}`)
	id = queueLineage(t, d, "later", []SpotifyDownloadOrigin{{Kind: "playlist", ID: collectionID, Revision: "v2", Position: 0}})
	completeLineage(t, d, path, id)
	got := readCollections(t, d, path)
	if got.CollectionStatus.State != "available" || len(got.Snapshots) != 7 {
		t.Fatal(got.CollectionStatus, len(got.Snapshots))
	}
	old, new := false, false
	for _, s := range got.Snapshots {
		if s.CapturedResource == "root" {
			old = old || s.CaptureRevision == "v1"
			new = new || s.CaptureRevision == "v2"
		}
	}
	if !old || !new {
		t.Fatal("immutable revision history lost", old, new)
	}
	// Valid-looking provenance edits are still detected by the observation key.
	if _, err := d.conn.Exec(`UPDATE spotify_download_catalog_imports SET capture_revision='tampered' WHERE captured_resource='root'`); err != nil {
		t.Fatal(err)
	}
	fp := scanEvidence(t, d, path, "song")
	if _, err := d.GetDownloadedSpotifyCatalog(t.Context(), "song", fp); err == nil {
		t.Fatal("tampered capture provenance admitted")
	}
}

func TestDownloadedCollectionsCumulativeLimits(t *testing.T) {
	for _, mode := range []string{"snapshots", "relations", "bytes"} {
		t.Run(mode, func(t *testing.T) {
			d, path := evidenceFixture(t)
			if err := d.ActivateSpotifyMetadataContext("owner"); err != nil {
				t.Fatal(err)
			}
			collectionPages(t, d)
			id := queueLineage(t, d, "first", collectionOrigins())
			completeLineage(t, d, path, id)
			baseline := readCollections(t, d, path)
			switch mode {
			case "snapshots":
				for i := 0; i < 252; i++ {
					putCollectionFixture(t, d, "playlist", collectionID, fmt.Sprintf("extra-%d", i), "v1", `{}`)
				}
			case "relations":
				relations := make([]SpotifyEntityRelation, 20000)
				for i := range relations {
					relations[i] = SpotifyEntityRelation{Kind: "other", Position: i, Unavailable: true}
				}
				putCollectionFixture(t, d, "playlist", collectionID, "many-relations", "v1", `{}`, relations...)
			case "bytes":
				for i := 0; i < 4; i++ {
					putCollectionFixture(t, d, "playlist", collectionID, fmt.Sprintf("large-%d", i), "v1", `{"data":"`+strings.Repeat("x", 2097141)+`"}`)
				}
			}
			id = queueLineage(t, d, "oversized", collectionOrigins())
			completeLineage(t, d, path, id)
			got := readCollections(t, d, path)
			if got.CollectionStatus.State != "oversized" || len(got.Snapshots) != len(baseline.Snapshots) || len(got.Relations) != len(baseline.Relations) {
				t.Fatal("oversized attempt replaced last-good", got.CollectionStatus, len(got.Snapshots), len(got.Relations))
			}
		})
	}
}

func TestDownloadedSavedLibraryScopeSelection(t *testing.T) {
	for _, fixture := range []struct{ scope, origin, typ, id string }{
		{"rest_saved_tracks", "saved_tracks", "track", referenceID},
		{"rest_saved_albums", "saved_albums", "album", "AAAAAAAAAAAAAAAAAAAAAA"},
		{"rest_saved_playlists", "saved_playlists", "playlist", collectionID},
		{"libraryV3_Albums", "saved_albums", "album", "AAAAAAAAAAAAAAAAAAAAAA"},
		{"libraryV3_Playlists", "saved_playlists", "playlist", collectionID},
	} {
		for _, mode := range []string{"matched", "position_mismatch", "unavailable"} {
			t.Run(fixture.scope+"/"+mode, func(t *testing.T) {
				d, path := evidenceFixture(t)
				if err := d.ActivateSpotifyMetadataContext("owner"); err != nil {
					t.Fatal(err)
				}
				putCollectionFixture(t, d, "library", fixture.scope, "selected-page", "", `{}`, SpotifyEntityRelation{Kind: "library_items", Position: 4, ChildType: fixture.typ, ChildID: fixture.id, Unavailable: mode == "unavailable"})
				putCollectionFixture(t, d, "library", fixture.scope, "other-page", "", `{}`, SpotifyEntityRelation{Kind: "library_items", Position: 0, ChildType: fixture.typ, ChildID: "ZZZZZZZZZZZZZZZZZZZZZZ"})
				origin := SpotifyDownloadOrigin{Kind: "library", ID: fixture.origin, EntityID: fixture.id, Position: -1}
				if mode == "position_mismatch" {
					origin.Position = 3
				}
				id := queueLineage(t, d, "scope", []SpotifyDownloadOrigin{origin})
				completeLineage(t, d, path, id)
				got := readCollections(t, d, path)
				if mode == "matched" {
					if got.CollectionStatus.State != "available" || len(got.Snapshots) != 1 || got.Snapshots[0].CapturedResource != "selected-page" {
						t.Fatal(got.CollectionStatus, got.Snapshots)
					}
				} else if got.CollectionStatus.State != "incomplete" || len(got.Snapshots) != 0 {
					t.Fatal("unverified library page retained", got.CollectionStatus, got.Snapshots)
				}
			})
		}
	}
}
