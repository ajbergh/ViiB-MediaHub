package db

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func TestSpotifyTraversalLeaseMigrationAndReopen(t *testing.T) {
	d, path := evidenceFixture(t)
	if _, err := d.conn.Exec(`DROP TABLE spotify_playlist_traversal_generation; DROP TABLE spotify_playlist_traversals;
 CREATE TABLE spotify_playlist_traversals(context_key TEXT NOT NULL,spotify_id TEXT NOT NULL,generation INTEGER NOT NULL,complete INTEGER NOT NULL DEFAULT 0,PRIMARY KEY(context_key,spotify_id));
 INSERT INTO spotify_playlist_traversals VALUES('owner','legacy',42,0),('owner','finished',9,1)`); err != nil {
		t.Fatal(err)
	}
	before := time.Now().UnixMilli()
	if err := d.EnsureSpotifyMetadataSchema(); err != nil {
		t.Fatal(err)
	}
	var generation, touched int64
	var complete, expired bool
	read := func(database *DB, id string) {
		t.Helper()
		if err := database.conn.QueryRow(`SELECT generation,complete,last_touched_at,lease_expired FROM spotify_playlist_traversals WHERE spotify_id=?`, id).Scan(&generation, &complete, &touched, &expired); err != nil {
			t.Fatal(err)
		}
	}
	read(d, "legacy")
	if generation != 42 || complete || expired || touched < before || touched > time.Now().UnixMilli() {
		t.Fatal("legacy traversal not preserved", generation, complete, touched, expired)
	}
	firstTouch := touched
	if err := d.EnsureSpotifyMetadataSchema(); err != nil {
		t.Fatal(err)
	}
	read(d, "legacy")
	if touched != firstTouch {
		t.Fatal("repeated migration renewed legacy lease")
	}
	d.Close()
	reopened, err := New(filepath.Join(filepath.Dir(path), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	read(reopened, "legacy")
	if generation != 42 || touched != firstTouch {
		t.Fatal("reopen reset generation or renewed lease")
	}
	read(reopened, "finished")
	if generation != 9 || !complete || expired {
		t.Fatal("completed traversal changed during migration")
	}
	if err := reopened.ActivateSpotifyMetadataContext("owner"); err != nil {
		t.Fatal(err)
	}
	next, err := reopened.BeginSpotifyPlaylistTraversal("owner", collectionID)
	if err != nil || next.Generation <= 42 {
		t.Fatal("legacy maximum not reserved", next, err)
	}
}

func TestSpotifyTraversalLeaseTouchExpiryFenceAndReclaim(t *testing.T) {
	d, path := evidenceFixture(t)
	if err := d.ActivateSpotifyMetadataContext("owner"); err != nil {
		t.Fatal(err)
	}
	token, err := d.BeginSpotifyPlaylistTraversal("owner", collectionID)
	if err != nil {
		t.Fatal(err)
	}
	putCollectionFixture(t, d, "playlist", collectionID, "page", "v1", `{"complete":false,"nextOffset":100}`)
	agePrivateFixtures(t, d, time.Now().Add(-3*24*time.Hour))
	now := time.Now().Truncate(time.Millisecond)
	if _, err := d.conn.Exec(`UPDATE spotify_playlist_traversals SET last_touched_at=?`, now.Add(-SpotifyPlaylistTraversalLease).UnixMilli()); err != nil {
		t.Fatal(err)
	}
	if err := d.PutSpotifyPlaylistSnapshots(token, nil, false); err != nil {
		t.Fatal(err)
	}
	var touched int64
	if err := d.conn.QueryRow(`SELECT last_touched_at FROM spotify_playlist_traversals`).Scan(&touched); err != nil || touched < now.UnixMilli() {
		t.Fatal("partial publication did not renew lease", touched, err)
	}
	if got := privateSweep(t, d, now, 128); got.TraversalsExpired != 0 || got.Collected != 0 {
		t.Fatal("live traversal lost pin", got)
	}
	if _, err := d.conn.Exec(`UPDATE spotify_playlist_traversals SET last_touched_at=?`, now.Add(-SpotifyPlaylistTraversalLease).UnixMilli()); err != nil {
		t.Fatal(err)
	}
	if got := privateSweep(t, d, now.Add(-time.Millisecond), 128); got.TraversalsExpired != 0 || got.Collected != 0 {
		t.Fatal("lease expired early", got)
	}
	if got := privateSweep(t, d, now, 1); got.TraversalsExpired != 1 || got.Collected+got.StatusCollected != 0 {
		t.Fatal("expiry exceeded shared limit", got)
	}
	var complete, expired bool
	var generation int64
	if err := d.conn.QueryRow(`SELECT generation,complete,lease_expired FROM spotify_playlist_traversals`).Scan(&generation, &complete, &expired); err != nil || complete || !expired || generation != token.Generation {
		t.Fatal("expiry changed completion/generation", generation, complete, expired, err)
	}
	d.Close()
	reopened, err := New(filepath.Join(filepath.Dir(path), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if err := reopened.ActivateSpotifyMetadataContext("owner"); err != nil {
		t.Fatal(err)
	}
	if err := reopened.PutSpotifyPlaylistSnapshots(token, nil, true); !errors.Is(err, ErrSpotifyTraversalSuperseded) {
		t.Fatal("late completion accepted after restart", err)
	}
	if got := privateSweep(t, reopened, now, 128); got.Collected != 1 || got.TraversalsExpired != 0 {
		t.Fatal("expired pin not released", got)
	}
	next, err := reopened.BeginSpotifyPlaylistTraversal("owner", collectionID)
	if err != nil || next.Generation <= token.Generation {
		t.Fatal("reclaim reset generation", next, err)
	}
	if err := reopened.PutSpotifyPlaylistSnapshots(next, nil, false); err != nil {
		t.Fatal("reclaimed traversal could not publish", err)
	}
	if err := reopened.PutSpotifyPlaylistSnapshots(token, nil, false); !errors.Is(err, ErrSpotifyTraversalSuperseded) {
		t.Fatal("old token revived after reclaim", err)
	}
}

func TestSpotifyTraversalLeasePinsContextsAndRollback(t *testing.T) {
	for _, origin := range []SpotifyDownloadOrigin{{Kind: "playlist", ID: collectionID, Revision: "v1", Position: 0}, {Kind: "library", ID: "saved_playlists", EntityID: collectionID, Position: -1}, {Kind: "library", ID: "saved_playlists", Position: -1}} {
		t.Run(origin.Kind, func(t *testing.T) {
			d, _ := evidenceFixture(t)
			if err := d.ActivateSpotifyMetadataContext("owner"); err != nil {
				t.Fatal(err)
			}
			if _, err := d.BeginSpotifyPlaylistTraversal("owner", collectionID); err != nil {
				t.Fatal(err)
			}
			collectionPages(t, d)
			queueLineage(t, d, "pin", []SpotifyDownloadOrigin{origin})
			now := time.Now()
			old := now.Add(-40 * 24 * time.Hour)
			agePrivateFixtures(t, d, old)
			if _, err := d.conn.Exec(`UPDATE spotify_playlist_traversals SET last_touched_at=?;
 INSERT INTO spotify_playlist_traversals(context_key,spotify_id,generation,complete,last_touched_at,lease_expired) VALUES('other','other',4,0,1,0),('','legacy',4,0,1,0),('owner','untouched',4,0,0,0)`, old.UnixMilli()); err != nil {
				t.Fatal(err)
			}
			if got := privateSweep(t, d, now, 128); got.TraversalsExpired != 0 {
				t.Fatal("active origin traversal expired", got)
			}
			if _, err := d.conn.Exec(`UPDATE spotify_downloads SET status='completed'`); err != nil {
				t.Fatal(err)
			}
			if _, err := d.ReserveSpotifyMetadataRuntime("lease-reservation"); err != nil {
				t.Fatal(err)
			}
			if got := privateSweep(t, d, now, 128); got.TraversalsExpired != 0 {
				t.Fatal("reserved traversal expired", got)
			}
			if err := d.ActivateSpotifyMetadataContext("owner"); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(t.Context())
			cancel()
			if _, err := d.MaintainSpotifyPrivateCache(ctx, now, 128); !errors.Is(err, context.Canceled) {
				t.Fatal(err)
			}
			putOrphanStatusFixture(t, d, "resource", "rollback", "owner", "not_found", old, old)
			if _, err := d.conn.Exec(`CREATE TRIGGER fail_lease_cleanup BEFORE DELETE ON spotify_metadata_resource_status BEGIN SELECT RAISE(ABORT,'fixture'); END`); err != nil {
				t.Fatal(err)
			}
			if got, err := d.MaintainSpotifyPrivateCache(t.Context(), now, 128); err == nil || got.TraversalsExpired != 0 || got.Collected != 0 || got.StatusCollected != 0 {
				t.Fatal("expiry escaped failed cleanup", got, err)
			}
			var expired int
			if err := d.conn.QueryRow(`SELECT SUM(lease_expired) FROM spotify_playlist_traversals`).Scan(&expired); err != nil || expired != 0 {
				t.Fatal("lease mutation not rolled back", expired, err)
			}
			if _, err := d.conn.Exec(`DROP TRIGGER fail_lease_cleanup`); err != nil {
				t.Fatal(err)
			}
			if got := privateSweep(t, d, now, 1); got.TraversalsExpired != 1 || got.Collected+got.StatusCollected != 0 {
				t.Fatal("released lease not expired within limit", got)
			}
			if err := d.conn.QueryRow(`SELECT SUM(lease_expired) FROM spotify_playlist_traversals`).Scan(&expired); err != nil || expired != 1 {
				t.Fatal("other context or zero timestamp expired", expired, err)
			}
		})
	}
}
