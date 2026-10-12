package db

import (
	"database/sql"
	"errors"
	"math"
	"time"
)

// SpotifyPlaylistTraversalLease is a local inactivity policy, not a provider TTL.
// Checkpoint publication renews it; expiry fences writes without claiming completion.
const SpotifyPlaylistTraversalLease = 24 * time.Hour

// Completed/expired ownership rows are cache state. The singleton allocator
// survives their collection, including explicit private-cache purge/retirement.
const SpotifyPlaylistTraversalRetention = 30 * 24 * time.Hour

func migrateSpotifyPlaylistTraversalLease(tx *sql.Tx) error {
	rows, err := tx.Query(`PRAGMA table_info(spotify_playlist_traversals)`)
	if err != nil {
		return err
	}
	columns := map[string]bool{}
	for rows.Next() {
		var cid, notnull, pk int
		var name, typ string
		var defaultValue any
		if err := rows.Scan(&cid, &name, &typ, &notnull, &defaultValue, &pk); err != nil {
			rows.Close()
			return err
		}
		columns[name] = true
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	if !columns["last_touched_at"] {
		if _, err := tx.Exec(`ALTER TABLE spotify_playlist_traversals ADD COLUMN last_touched_at INTEGER NOT NULL DEFAULT 0`); err != nil {
			return err
		}
		// Legacy work gets a full lease window from migration, preserving its
		// generation and actual completion state. Reopen must not renew it again.
		if _, err := tx.Exec(`UPDATE spotify_playlist_traversals SET last_touched_at=?`, time.Now().UnixMilli()); err != nil {
			return err
		}
	}
	if !columns["lease_expired"] {
		if _, err := tx.Exec(`ALTER TABLE spotify_playlist_traversals ADD COLUMN lease_expired INTEGER NOT NULL DEFAULT 0 CHECK(lease_expired IN (0,1))`); err != nil {
			return err
		}
	}
	if !columns["lease_expired_at"] {
		if _, err := tx.Exec(`ALTER TABLE spotify_playlist_traversals ADD COLUMN lease_expired_at INTEGER NOT NULL DEFAULT 0`); err != nil {
			return err
		}
		if _, err := tx.Exec(`UPDATE spotify_playlist_traversals SET lease_expired_at=? WHERE lease_expired=1`, time.Now().UnixMilli()); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(`CREATE TABLE IF NOT EXISTS spotify_playlist_traversal_generation(
 singleton INTEGER PRIMARY KEY CHECK(singleton=1),last_generation INTEGER NOT NULL CHECK(typeof(last_generation)='integer' AND last_generation>=0));
 INSERT INTO spotify_playlist_traversal_generation(singleton,last_generation)
 SELECT 1,MAX(0,COALESCE(MAX(generation),0)) FROM spotify_playlist_traversals WHERE 1
 ON CONFLICT(singleton) DO UPDATE SET last_generation=MAX(last_generation,excluded.last_generation)`); err != nil {
		return err
	}
	_, err = tx.Exec(`CREATE INDEX IF NOT EXISTS idx_spotify_traversal_lease ON spotify_playlist_traversals(context_key,complete,lease_expired,last_touched_at)`)
	return err
}

var ErrSpotifyTraversalSuperseded = errors.New("Spotify playlist traversal superseded")
var ErrSpotifyTraversalGenerationExhausted = errors.New("Spotify playlist traversal generation exhausted")

type SpotifyPlaylistTraversal struct {
	ContextKey   string
	SpotifyID    string
	Generation   int64
	RuntimeEpoch string
}

// BeginSpotifyPlaylistTraversal advances the publication owner before fetching
// a root. Checkpoint content can be reused, but prior workers lose write access.
func (d *DB) BeginSpotifyPlaylistTraversal(contextKey, id string) (SpotifyPlaylistTraversal, error) {
	return d.beginSpotifyPlaylistTraversal(contextKey, id, "")
}

func (d *DB) BeginSpotifyPlaylistTraversalForRuntime(fence SpotifyMetadataFence, id string) (SpotifyPlaylistTraversal, error) {
	if fence.Epoch == "" {
		return SpotifyPlaylistTraversal{}, ErrSpotifyMetadataRuntimeSuperseded
	}
	return d.beginSpotifyPlaylistTraversal(fence.ContextKey, id, fence.Epoch)
}

func (d *DB) beginSpotifyPlaylistTraversal(contextKey, id, epoch string) (SpotifyPlaylistTraversal, error) {
	token := SpotifyPlaylistTraversal{ContextKey: contextKey, SpotifyID: id, RuntimeEpoch: epoch}
	if !validSnapshotKey(SpotifySnapshotKey{EntityType: "playlist", SpotifyID: id, Resource: "traversal", ContextKey: contextKey}) {
		return token, errors.New("invalid playlist traversal identity")
	}
	tx, err := d.conn.Begin()
	if err != nil {
		return token, err
	}
	defer tx.Rollback()
	// Reserve the writer before checking ownership, so account changes and
	// independent handles cannot race generation allocation or row recreation.
	if _, err := tx.Exec(`UPDATE settings SET value=value WHERE key='spotify_metadata_active_context'`); err != nil {
		return token, err
	}
	var active string
	if err := tx.QueryRow(`SELECT COALESCE((SELECT value FROM settings WHERE key='spotify_metadata_active_context'),'')`).Scan(&active); err != nil {
		return token, err
	}
	if active != contextKey {
		return token, ErrSpotifyTraversalSuperseded
	}
	if epoch != "" {
		if err := checkSpotifyMetadataFenceTx(tx, SpotifyMetadataFence{Epoch: epoch, ContextKey: contextKey}); err != nil {
			return token, err
		}
	}
	err = tx.QueryRow(`UPDATE spotify_playlist_traversal_generation SET last_generation=last_generation+1 WHERE singleton=1 AND last_generation<? RETURNING last_generation`, int64(math.MaxInt64)).Scan(&token.Generation)
	if errors.Is(err, sql.ErrNoRows) {
		return token, ErrSpotifyTraversalGenerationExhausted
	}
	if err != nil {
		return token, err
	}
	if _, err := tx.Exec(`INSERT INTO spotify_playlist_traversals(context_key,spotify_id,generation,complete,last_touched_at,lease_expired,lease_expired_at)
 VALUES(?,?,?,0,?,0,0) ON CONFLICT(context_key,spotify_id) DO UPDATE SET generation=excluded.generation,complete=0,last_touched_at=excluded.last_touched_at,lease_expired=0,lease_expired_at=0`, contextKey, id, token.Generation, time.Now().UnixMilli()); err != nil {
		return token, err
	}
	return token, tx.Commit()
}

// Publication ownership and every snapshot/relation update share one transaction.
func (d *DB) PutSpotifyPlaylistSnapshots(token SpotifyPlaylistTraversal, snapshots []SpotifyEntitySnapshot, complete bool) error {
	var fence *SpotifyMetadataFence
	if token.RuntimeEpoch != "" {
		fence = &SpotifyMetadataFence{Epoch: token.RuntimeEpoch, ContextKey: token.ContextKey}
	}
	return d.putSpotifyEntitySnapshots(snapshots, &token, complete, fence)
}
