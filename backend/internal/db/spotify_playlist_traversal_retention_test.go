package db

import (
	"errors"
	"math"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestSpotifyTraversalAllocatorAcrossHandlesAndPurge(t *testing.T) {
	d, path := evidenceFixture(t)
	other, err := New(filepath.Join(filepath.Dir(path), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	if err := d.ActivateSpotifyMetadataContext("owner"); err != nil {
		t.Fatal(err)
	}
	type result struct {
		token SpotifyPlaylistTraversal
		err   error
	}
	results := make(chan result, 12)
	var workers sync.WaitGroup
	for i := 0; i < 12; i++ {
		workers.Add(1)
		go func(i int) {
			defer workers.Done()
			handle := []*DB{d, other}[i%2]
			token, err := handle.BeginSpotifyPlaylistTraversal("owner", collectionID)
			results <- result{token, err}
		}(i)
	}
	workers.Wait()
	close(results)
	seen := map[int64]bool{}
	var latest SpotifyPlaylistTraversal
	for got := range results {
		if got.err != nil || seen[got.token.Generation] {
			t.Fatal("concurrent allocation failed", got, seen)
		}
		seen[got.token.Generation] = true
		if got.token.Generation > latest.Generation {
			latest = got.token
		}
	}
	if err := d.PutSpotifyPlaylistSnapshots(latest, nil, true); err != nil {
		t.Fatal(err)
	}
	if err := d.PurgeSpotifyMetadata(); err != nil {
		t.Fatal(err)
	}
	if err := d.ActivateSpotifyMetadataContext("owner"); err != nil {
		t.Fatal(err)
	}
	next, err := other.BeginSpotifyPlaylistTraversal("owner", collectionID)
	if err != nil || next.Generation <= latest.Generation {
		t.Fatal("purge reused allocator", next, err)
	}
	if err := d.PutSpotifyPlaylistSnapshots(latest, nil, true); !errors.Is(err, ErrSpotifyTraversalSuperseded) {
		t.Fatal("purged token revived", err)
	}
}

func TestSpotifyTraversalDeletionPreservesActiveOrigins(t *testing.T) {
	for _, origin := range []SpotifyDownloadOrigin{{Kind: "playlist", ID: collectionID, Revision: "v1", Position: 0}, {Kind: "library", ID: "saved_playlists", EntityID: collectionID, Position: -1}, {Kind: "library", ID: "saved_playlists", Position: -1}} {
		t.Run(origin.Kind+origin.EntityID, func(t *testing.T) {
			d, _ := evidenceFixture(t)
			if err := d.ActivateSpotifyMetadataContext("owner"); err != nil {
				t.Fatal(err)
			}
			token, err := d.BeginSpotifyPlaylistTraversal("owner", collectionID)
			if err != nil {
				t.Fatal(err)
			}
			if err := d.PutSpotifyPlaylistSnapshots(token, nil, true); err != nil {
				t.Fatal(err)
			}
			queueLineage(t, d, "pin", []SpotifyDownloadOrigin{origin})
			now := time.Now()
			if _, err := d.conn.Exec(`UPDATE spotify_playlist_traversals SET last_touched_at=?`, now.Add(-SpotifyPlaylistTraversalRetention-time.Hour).UnixMilli()); err != nil {
				t.Fatal(err)
			}
			if got := privateSweep(t, d, now, 128); got.TraversalsDeleted != 0 {
				t.Fatal("active origin deleted", got)
			}
			if _, err := d.conn.Exec(`UPDATE spotify_downloads SET status='completed'`); err != nil {
				t.Fatal(err)
			}
			if got := privateSweep(t, d, now, 128); got.TraversalsDeleted != 1 {
				t.Fatal("released origin retained", got)
			}
		})
	}
}

func TestSpotifyTraversalCollectedGenerationNeverRevives(t *testing.T) {
	for _, complete := range []bool{false, true} {
		t.Run(map[bool]string{false: "expired", true: "complete"}[complete], func(t *testing.T) {
			d, _ := evidenceFixture(t)
			if err := d.ActivateSpotifyMetadataContext("owner"); err != nil {
				t.Fatal(err)
			}
			old, err := d.BeginSpotifyPlaylistTraversal("owner", collectionID)
			if err != nil {
				t.Fatal(err)
			}
			if complete {
				if err := d.PutSpotifyPlaylistSnapshots(old, nil, true); err != nil {
					t.Fatal(err)
				}
			}
			now := time.Now().Truncate(time.Millisecond)
			if _, err := d.conn.Exec(`UPDATE spotify_playlist_traversals SET last_touched_at=?`, now.Add(-SpotifyPlaylistTraversalLease).UnixMilli()); err != nil {
				t.Fatal(err)
			}
			if !complete {
				if got := privateSweep(t, d, now, 1); got.TraversalsExpired != 1 {
					t.Fatal(got)
				}
			} else {
				now = now.Add(-SpotifyPlaylistTraversalLease)
			}
			if got := privateSweep(t, d, now.Add(SpotifyPlaylistTraversalRetention-time.Millisecond), 1); got.TraversalsDeleted != 0 {
				t.Fatal("collected before grace", got)
			}
			if _, err := d.conn.Exec(`CREATE TRIGGER fail_traversal_delete BEFORE DELETE ON spotify_playlist_traversals BEGIN SELECT RAISE(ABORT,'fixture'); END`); err != nil {
				t.Fatal(err)
			}
			if got, err := d.MaintainSpotifyPrivateCache(t.Context(), now.Add(SpotifyPlaylistTraversalRetention), 1); err == nil || got.TraversalsDeleted != 0 {
				t.Fatal("failed collection escaped transaction", got, err)
			}
			if _, err := d.conn.Exec(`DROP TRIGGER fail_traversal_delete`); err != nil {
				t.Fatal(err)
			}
			if got := privateSweep(t, d, now.Add(SpotifyPlaylistTraversalRetention), 1); got.TraversalsDeleted != 1 || got.Collected+got.StatusCollected+got.TraversalsExpired != 0 {
				t.Fatal("shared deletion bound", got)
			}
			if err := d.PutSpotifyPlaylistSnapshots(old, nil, true); !errors.Is(err, ErrSpotifyTraversalSuperseded) {
				t.Fatal("deleted token published", err)
			}
			next, err := d.BeginSpotifyPlaylistTraversal("owner", collectionID)
			if err != nil || next.Generation <= old.Generation {
				t.Fatal("generation reused", next, err)
			}
			if err := d.PutSpotifyPlaylistSnapshots(old, nil, true); !errors.Is(err, ErrSpotifyTraversalSuperseded) {
				t.Fatal("old token revived", err)
			}
			if err := d.PutSpotifyPlaylistSnapshots(next, nil, true); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestSpotifyTraversalAllocatorRollbackAndExhaustion(t *testing.T) {
	d, _ := evidenceFixture(t)
	if err := d.ActivateSpotifyMetadataContext("owner"); err != nil {
		t.Fatal(err)
	}
	if _, err := d.conn.Exec(`CREATE TRIGGER fail_traversal_insert BEFORE INSERT ON spotify_playlist_traversals BEGIN SELECT RAISE(ABORT,'fixture'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := d.BeginSpotifyPlaylistTraversal("owner", collectionID); err == nil {
		t.Fatal("insert failure ignored")
	}
	var generation int64
	if err := d.conn.QueryRow(`SELECT last_generation FROM spotify_playlist_traversal_generation`).Scan(&generation); err != nil || generation != 0 {
		t.Fatal("allocation not rolled back", generation, err)
	}
	if _, err := d.conn.Exec(`DROP TRIGGER fail_traversal_insert; UPDATE spotify_playlist_traversal_generation SET last_generation=?`, int64(math.MaxInt64)); err != nil {
		t.Fatal(err)
	}
	if _, err := d.BeginSpotifyPlaylistTraversal("owner", collectionID); !errors.Is(err, ErrSpotifyTraversalGenerationExhausted) {
		t.Fatal("overflow not rejected", err)
	}
	if err := d.EnsureSpotifyMetadataSchema(); err != nil {
		t.Fatal(err)
	}
	if err := d.conn.QueryRow(`SELECT last_generation FROM spotify_playlist_traversal_generation`).Scan(&generation); err != nil || generation != math.MaxInt64 {
		t.Fatal("exhaustion counter reset", generation, err)
	}
}
