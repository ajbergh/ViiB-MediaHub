package db

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

// Simulate historical rows from a connection that did not enforce foreign keys.
// Production queue deletion normally cascades these rows immediately.
func orphanLineage(t *testing.T, d *DB, id, owner string, staged time.Time) {
	t.Helper()
	c, err := d.conn.Conn(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if _, err := c.ExecContext(t.Context(), `PRAGMA foreign_keys=OFF`); err != nil {
		t.Fatal(err)
	}
	defer c.ExecContext(context.Background(), `PRAGMA foreign_keys=ON`)
	if _, err := c.ExecContext(t.Context(), `INSERT INTO spotify_download_lineage_staging(download_id,context_key,origin_kind,origin_id,origin_revision,position,entity_id,staged_at) VALUES(?,?,'playlist',?,'v1',0,'',?)`, id, owner, collectionID, staged.UnixMilli()); err != nil {
		t.Fatal(err)
	}
}

func TestDownloadLineageOrphansGraceFenceLimitAndRollback(t *testing.T) {
	d, _ := evidenceFixture(t)
	if err := d.ActivateSpotifyMetadataContext("owner"); err != nil {
		t.Fatal(err)
	}
	now := time.Now().Truncate(time.Millisecond)
	old := now.Add(-SpotifyLineageOrphanGrace)
	orphanLineage(t, d, "old", "owner", old)
	orphanLineage(t, d, "fresh", "owner", now)
	orphanLineage(t, d, "other", "other", old)
	orphanLineage(t, d, "legacy", "", old)
	if got := privateSweep(t, d, now.Add(-time.Millisecond), 128); got.StagingCollected != 0 {
		t.Fatal("early collection", got)
	}
	putOrphanStatusFixture(t, d, "resource", "rollback", "owner", "not_found", old.Add(-time.Hour), old.Add(-time.Hour))
	if _, err := d.conn.Exec(`CREATE TRIGGER fail_staging_delete BEFORE DELETE ON spotify_download_lineage_staging BEGIN SELECT RAISE(ABORT,'fixture'); END`); err != nil {
		t.Fatal(err)
	}
	if got, err := d.MaintainSpotifyPrivateCache(t.Context(), now, 128); err == nil || got != (PrivateCacheRetentionResult{}) {
		t.Fatal("partial cleanup escaped", got, err)
	}
	var count int
	if err := d.conn.QueryRow(`SELECT COUNT(*) FROM spotify_metadata_resource_status WHERE spotify_id='rollback'`).Scan(&count); err != nil || count != 1 {
		t.Fatal("prior delete not rolled back", count, err)
	}
	if _, err := d.conn.Exec(`DROP TRIGGER fail_staging_delete`); err != nil {
		t.Fatal(err)
	}
	if got := privateSweep(t, d, now, 1); got.StatusCollected != 1 || got.StagingCollected != 0 {
		t.Fatal("shared limit", got)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := d.MaintainSpotifyPrivateCache(ctx, now, 128); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err := d.ReserveSpotifyMetadataRuntime("reservation"); err != nil {
		t.Fatal(err)
	}
	if got := privateSweep(t, d, now, 128); got.StagingCollected != 0 {
		t.Fatal("reserved owner collected", got)
	}
	if err := d.ActivateSpotifyMetadataContext("owner"); err != nil {
		t.Fatal(err)
	}
	if got := privateSweep(t, d, now, 128); got.StagingCollected != 1 {
		t.Fatal("exact grace collection", got)
	}
	if err := d.conn.QueryRow(`SELECT COUNT(*) FROM spotify_download_lineage_staging`).Scan(&count); err != nil || count != 3 {
		t.Fatal("foreign/legacy/fresh staging changed", count, err)
	}
}

func TestDownloadLineageRetentionPreservesAllQueueStatesAndRetry(t *testing.T) {
	for _, state := range []string{"queued", "downloading", "converting", "failed", "completed"} {
		t.Run(state, func(t *testing.T) {
			d, _ := evidenceFixture(t)
			if err := d.ActivateSpotifyMetadataContext("owner"); err != nil {
				t.Fatal(err)
			}
			id := queueLineage(t, d, "queue", []SpotifyDownloadOrigin{{Kind: "playlist", ID: collectionID, Revision: "v1", Position: 0}})
			now := time.Now()
			if _, err := d.conn.Exec(`UPDATE spotify_downloads SET status=?; UPDATE spotify_download_lineage_staging SET staged_at=?`, state, now.Add(-365*24*time.Hour).UnixMilli()); err != nil {
				t.Fatal(err)
			}
			if got := privateSweep(t, d, now, 128); got.StagingCollected != 0 {
				t.Fatal("queue lineage expired", got)
			}
			if state == "failed" {
				if err := d.ResetDownloadForRetry(id); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := d.ResetDownloadForForceRestart(id); err != nil {
					t.Fatal(err)
				}
			}
			var count int
			if err := d.conn.QueryRow(`SELECT COUNT(*) FROM spotify_download_lineage_staging WHERE download_id=?`, id).Scan(&count); err != nil || count != 1 {
				t.Fatal("retry lost lineage", count, err)
			}
			if err := d.DeleteDownload(id); err != nil {
				t.Fatal(err)
			}
			if err := d.conn.QueryRow(`SELECT COUNT(*) FROM spotify_download_lineage_staging`).Scan(&count); err != nil || count != 0 {
				t.Fatal("queue deletion did not cascade", count, err)
			}
		})
	}
}

func TestDownloadLineageRetentionMigrationDoesNotRenewOnReopen(t *testing.T) {
	d, path := evidenceFixture(t)
	if err := d.ActivateSpotifyMetadataContext("owner"); err != nil {
		t.Fatal(err)
	}
	queueLineage(t, d, "queue", []SpotifyDownloadOrigin{{Kind: "playlist", ID: collectionID, Revision: "v1", Position: 0}})
	if _, err := d.conn.Exec(`DROP INDEX idx_spotify_lineage_staged_at; ALTER TABLE spotify_download_lineage_staging DROP COLUMN staged_at`); err != nil {
		t.Fatal(err)
	}
	before := time.Now().UnixMilli()
	if err := d.EnsureSpotifyMetadataSchema(); err != nil {
		t.Fatal(err)
	}
	var first, again int64
	if err := d.conn.QueryRow(`SELECT staged_at FROM spotify_download_lineage_staging`).Scan(&first); err != nil || first < before {
		t.Fatal("legacy grace missing", first, err)
	}
	d.Close()
	reopened, err := New(filepath.Join(filepath.Dir(path), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if err := reopened.conn.QueryRow(`SELECT staged_at FROM spotify_download_lineage_staging`).Scan(&again); err != nil || again != first {
		t.Fatal("reopen renewed grace", first, again, err)
	}
}
