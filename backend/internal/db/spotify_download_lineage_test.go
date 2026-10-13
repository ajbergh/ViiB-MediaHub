package db

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func queueLineage(t *testing.T, d *DB, id string, origins []SpotifyDownloadOrigin) string {
	t.Helper()
	ids, err := d.AddDownloads([]*SpotifyDownload{{ID: id, SpotifyID: referenceID, Type: "track", Title: "Fixture", Status: "queued", AddedAt: 1, Origins: origins}})
	if err != nil || len(ids) != 1 {
		t.Fatalf("queue: %v %v", ids, err)
	}
	return ids[0]
}
func completeLineage(t *testing.T, d *DB, path, id string) {
	t.Helper()
	if ok, err := d.MarkDownloadStarted(id); err != nil || !ok {
		t.Fatal(ok, err)
	}
	if ok, err := d.MarkDownloadCompletedWithEvidence(t.Context(), id, path); err != nil || !ok {
		t.Fatal(ok, err)
	}
}

func TestDownloadLineageLifecycleAndAccountFence(t *testing.T) {
	for _, retired := range []bool{false, true} {
		t.Run(fmt.Sprint(retired), func(t *testing.T) {
			d, path := evidenceFixture(t)
			if _, err := d.ReserveSpotifyMetadataRuntime("first-runtime"); err != nil {
				t.Fatal(err)
			}
			if _, err := d.ConfirmSpotifyMetadataOwner("first-runtime", "oauth", "account", "owner"); err != nil {
				t.Fatal(err)
			}
			origins := []SpotifyDownloadOrigin{{Kind: "playlist", ID: "AAAAAAAAAAAAAAAAAAAAAA", Revision: "snapshot", Position: 0}, {Kind: "playlist", ID: "AAAAAAAAAAAAAAAAAAAAAA", Revision: "snapshot", Position: 4}, {Kind: "library", ID: "saved_playlists", EntityID: "AAAAAAAAAAAAAAAAAAAAAA", Position: -1}}
			id := queueLineage(t, d, "first", origins)
			if reused := queueLineage(t, d, "duplicate", origins); reused != id {
				t.Fatal("dedup changed")
			}
			d.Close()
			reopened, err := New(filepath.Join(filepath.Dir(path), "library.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer reopened.Close()
			if _, err := reopened.ReserveSpotifyMetadataRuntime("second-runtime"); err != nil {
				t.Fatal(err)
			}
			account := "account"
			if retired {
				account = "replacement"
			}
			restored, err := reopened.ConfirmSpotifyMetadataOwner("second-runtime", "oauth", account, "replacement-context")
			if err != nil {
				t.Fatal(err)
			}
			if !retired && restored != "owner" {
				t.Fatal("same account lost its staging context")
			}

			completeLineage(t, reopened, path, id)
			fp := scanEvidence(t, reopened, path, "song")
			if _, err := reopened.DeleteCompletedDownloads(); err != nil {
				t.Fatal(err)
			}
			if !retired {
				if err := reopened.RetireSpotifyMetadataContext("owner"); err != nil {
					t.Fatal(err)
				}
			}
			reopened.Close()
			reopened, err = New(filepath.Join(filepath.Dir(path), "library.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer reopened.Close()

			got, err := reopened.GetDownloadedSpotifyCatalog(t.Context(), "song", fp)
			if err != nil || got == nil || got.LineageStatus == nil {
				t.Fatalf("read: %+v %v", got, err)
			}
			count := 3
			state := "available"
			if retired {
				count = 0
				state = "no_active_request_lineage"
			}
			if len(got.Origins) != count || got.LineageStatus.State != state {
				t.Fatalf("lineage: %+v %+v", got.Origins, got.LineageStatus)
			}
			if !retired {
				found := false
				for _, origin := range got.Origins {
					if origin.Kind == "library" {
						found = origin.EntityID == "AAAAAAAAAAAAAAAAAAAAAA"
					}
				}
				if !found {
					t.Fatal("collection identity lost", got.Origins)
				}
			}
			var pending int
			if err := reopened.conn.QueryRow("SELECT COUNT(*) FROM spotify_download_lineage_staging").Scan(&pending); err != nil || pending != 0 {
				t.Fatal(pending, err)
			}
			if wrong, err := reopened.GetDownloadedSpotifyCatalog(t.Context(), "song", "changed"); err != nil || wrong != nil {
				t.Fatal("fingerprint fence", err)
			}
			if err := os.WriteFile(path, []byte("changed media"), 0600); err != nil {
				t.Fatal(err)
			}
			if wrong, err := reopened.GetDownloadedSpotifyCatalog(t.Context(), "song", fp); err != nil || wrong != nil {
				t.Fatal("physical fence", err)
			}
		})
	}
}

func TestDownloadLineageValidationAndAtomicQueue(t *testing.T) {
	for _, origin := range []SpotifyDownloadOrigin{{Kind: "library", ID: "saved_albums", EntityID: "invalid", Position: -1}, {Kind: "album", ID: referenceID, EntityID: referenceID, Position: 0}, {Kind: "playlist", ID: "invalid", Position: 0}, {Kind: "library", ID: "account", Position: 0}, {Kind: "album", ID: referenceID, Position: -2}, {Kind: "playlist", ID: referenceID, Revision: strings.Repeat("x", 257), Position: 0}, {Kind: "playlist", ID: referenceID, Revision: "secret\n", Position: 0}} {
		d, _ := evidenceFixture(t)
		if err := d.ActivateSpotifyMetadataContext("owner"); err != nil {
			t.Fatal(err)
		}
		_, err := d.AddDownloads([]*SpotifyDownload{{ID: "good", SpotifyID: referenceID, Status: "queued", Metadata: "first"}, {ID: "invalid", SpotifyID: referenceID, Status: "queued", Origins: []SpotifyDownloadOrigin{origin}}})
		if err == nil {
			t.Fatal("invalid origin accepted", origin)
		}
		var count int
		if err := d.conn.QueryRow("SELECT COUNT(*) FROM spotify_downloads").Scan(&count); err != nil || count != 0 {
			t.Fatal("partial queue", count, err)
		}
	}
	d, _ := evidenceFixture(t)
	if _, err := d.AddDownloads([]*SpotifyDownload{{ID: "no-account", SpotifyID: referenceID, Status: "queued", Origins: []SpotifyDownloadOrigin{{Kind: "album", ID: referenceID, Position: 0}}}}); err == nil {
		t.Fatal("unowned origins accepted")
	}
}

func TestDownloadLineageCumulativeLimit(t *testing.T) {
	d, path := evidenceFixture(t)
	if err := d.ActivateSpotifyMetadataContext("owner"); err != nil {
		t.Fatal(err)
	}
	for batch := 0; batch < 3; batch++ {
		origins := make([]SpotifyDownloadOrigin, 32)
		for i := range origins {
			origins[i] = SpotifyDownloadOrigin{Kind: "playlist", ID: referenceID, Revision: fmt.Sprint(batch), Position: i}
		}
		id := queueLineage(t, d, fmt.Sprint(batch), origins)
		completeLineage(t, d, path, id)
	}
	fp := scanEvidence(t, d, path, "song")
	got, err := d.GetDownloadedSpotifyCatalog(t.Context(), "song", fp)
	if err != nil || got == nil || len(got.Origins) != 64 || got.LineageStatus.State != "oversized" {
		t.Fatalf("cumulative limit: %+v %v", got, err)
	}
	for _, origin := range got.Origins {
		if origin.Revision == "2" {
			t.Fatal("oversized attempt replaced last-good")
		}
	}
}

func TestDownloadLineagePromotionRollbackAndQueueDeletion(t *testing.T) {
	d, path := evidenceFixture(t)
	if err := d.ActivateSpotifyMetadataContext("owner"); err != nil {
		t.Fatal(err)
	}
	origins := []SpotifyDownloadOrigin{{Kind: "playlist", ID: referenceID, Position: 0}}
	id := queueLineage(t, d, "rollback", origins)
	if ok, err := d.MarkDownloadStarted(id); err != nil || !ok {
		t.Fatal(ok, err)
	}
	if _, err := d.conn.Exec(`CREATE TRIGGER reject_lineage_status BEFORE INSERT ON spotify_download_lineage_status BEGIN SELECT RAISE(ABORT,'fixture'); END`); err != nil {
		t.Fatal(err)
	}
	if ok, err := d.MarkDownloadCompletedWithEvidence(t.Context(), id, path); err == nil || ok {
		t.Fatal("completion committed without lineage status")
	}
	var staged, retained int
	if err := d.conn.QueryRow("SELECT COUNT(*) FROM spotify_download_lineage_staging").Scan(&staged); err != nil {
		t.Fatal(err)
	}
	if err := d.conn.QueryRow("SELECT COUNT(*) FROM spotify_download_lineage_imports").Scan(&retained); err != nil {
		t.Fatal(err)
	}
	row, err := d.GetDownload(id)
	if err != nil || row.Status != "downloading" || staged != 1 || retained != 0 {
		t.Fatal("partial completion", row, staged, retained, err)
	}
	if _, err := d.conn.Exec("DROP TRIGGER reject_lineage_status"); err != nil {
		t.Fatal(err)
	}
	if ok, err := d.MarkDownloadCompletedWithEvidence(t.Context(), id, path); err != nil || !ok {
		t.Fatal(ok, err)
	}
	pending := queueLineage(t, d, "delete", origins)
	if err := d.DeleteDownload(pending); err != nil {
		t.Fatal(err)
	}
	if err := d.conn.QueryRow("SELECT COUNT(*) FROM spotify_download_lineage_staging").Scan(&staged); err != nil || staged != 0 {
		t.Fatal("orphan staging", staged, err)
	}
}

func TestDownloadLineageQueueOriginLimit(t *testing.T) {
	d, _ := evidenceFixture(t)
	if err := d.ActivateSpotifyMetadataContext("owner"); err != nil {
		t.Fatal(err)
	}
	origins := make([]SpotifyDownloadOrigin, 32)
	for i := range origins {
		origins[i] = SpotifyDownloadOrigin{Kind: "playlist", ID: referenceID, Position: i}
	}
	id := queueLineage(t, d, "first", origins)
	_, err := d.AddDownloads([]*SpotifyDownload{{ID: "overflow", SpotifyID: referenceID, Status: "queued", Origins: []SpotifyDownloadOrigin{{Kind: "playlist", ID: referenceID, Position: 32}}}})
	if err == nil {
		t.Fatal("dedup accumulated more than 32 origins")
	}
	var count int
	if err := d.conn.QueryRow("SELECT COUNT(*) FROM spotify_download_lineage_staging WHERE download_id=?", id).Scan(&count); err != nil || count != 32 {
		t.Fatal(count, err)
	}
}

func TestLibraryOriginsKeepDistinctCollectionIdentities(t *testing.T) {
	d, path := evidenceFixture(t)
	if err := d.ActivateSpotifyMetadataContext("owner"); err != nil {
		t.Fatal(err)
	}
	a := SpotifyDownloadOrigin{Kind: "library", ID: "saved_albums", EntityID: "AAAAAAAAAAAAAAAAAAAAAA", Position: -1}
	b := a
	b.EntityID = "BBBBBBBBBBBBBBBBBBBBBB"
	legacy := a
	legacy.EntityID = ""
	id := queueLineage(t, d, "collections", []SpotifyDownloadOrigin{a, b, legacy, a})
	completeLineage(t, d, path, id)
	fp := scanEvidence(t, d, path, "song")
	got, err := d.GetDownloadedSpotifyCatalog(t.Context(), "song", fp)
	if err != nil || got == nil || len(got.Origins) != 3 {
		t.Fatalf("distinct collections: %+v %v", got, err)
	}
	identities := map[string]bool{}
	for _, o := range got.Origins {
		identities[o.EntityID] = true
	}
	if !identities[""] || !identities[a.EntityID] || !identities[b.EntityID] {
		t.Fatal(identities)
	}
}

func TestDownloadLineageCollectionMigration(t *testing.T) {
	d, _ := evidenceFixture(t)
	if err := d.ActivateSpotifyMetadataContext("owner"); err != nil {
		t.Fatal(err)
	}
	id := queueLineage(t, d, "legacy", []SpotifyDownloadOrigin{{Kind: "library", ID: "saved_albums", Position: -1}})
	// Recreate both previous-schema tables with retained rows.
	_, err := d.conn.Exec(`DROP TABLE spotify_download_lineage_staging;
 CREATE TABLE spotify_download_lineage_staging(download_id TEXT NOT NULL REFERENCES spotify_downloads(id) ON DELETE CASCADE,context_key TEXT NOT NULL,origin_kind TEXT NOT NULL,origin_id TEXT NOT NULL,origin_revision TEXT NOT NULL,position INTEGER NOT NULL,PRIMARY KEY(download_id,context_key,origin_kind,origin_id,origin_revision,position));
 DROP TABLE spotify_download_lineage_imports;
 CREATE TABLE spotify_download_lineage_imports(file_path TEXT NOT NULL,content_sha256 TEXT NOT NULL,file_size INTEGER NOT NULL,mtime_ns INTEGER NOT NULL,recording_id TEXT NOT NULL,origin_kind TEXT NOT NULL,origin_id TEXT NOT NULL,origin_revision TEXT NOT NULL,position INTEGER NOT NULL,PRIMARY KEY(file_path,content_sha256,file_size,mtime_ns,recording_id,origin_kind,origin_id,origin_revision,position));
 INSERT INTO spotify_download_lineage_imports VALUES('file','hash',1,2,'root','library','saved_albums','',-1);`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = d.conn.Exec(`INSERT INTO spotify_download_lineage_staging VALUES(?,'owner','library','saved_albums','',-1)`, id); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err = d.EnsureSpotifyMetadataSchema(); err != nil {
			t.Fatal(err)
		}
	}
	for _, table := range []string{"spotify_download_lineage_staging", "spotify_download_lineage_imports"} {
		var entity string
		if err = d.conn.QueryRow("SELECT entity_id FROM " + table).Scan(&entity); err != nil || entity != "" {
			t.Fatal(table, entity, err)
		}
	}
	if _, err = d.conn.Exec(`INSERT INTO spotify_download_lineage_staging(download_id,context_key,origin_kind,origin_id,origin_revision,position,entity_id) VALUES(?,'owner','library','saved_albums','',-1,'AAAAAAAAAAAAAAAAAAAAAA')`, id); err != nil {
		t.Fatal(err)
	}
	if _, err = d.conn.Exec(`INSERT INTO spotify_download_lineage_imports VALUES('file','hash',1,2,'root','library','saved_albums','',-1,'AAAAAAAAAAAAAAAAAAAAAA')`); err != nil {
		t.Fatal(err)
	}
	if _, err = d.conn.Exec(`DELETE FROM spotify_downloads WHERE id=?`, id); err != nil {
		t.Fatal(err)
	}
	var staging, retained int
	if err = d.conn.QueryRow(`SELECT COUNT(*) FROM spotify_download_lineage_staging`).Scan(&staging); err != nil {
		t.Fatal(err)
	}
	if err = d.conn.QueryRow(`SELECT COUNT(*) FROM spotify_download_lineage_imports`).Scan(&retained); err != nil {
		t.Fatal(err)
	}
	if staging != 0 || retained != 2 {
		t.Fatal("migration lost cascade or durable separation", staging, retained)
	}
}
