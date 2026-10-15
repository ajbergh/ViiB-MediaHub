package db

import "database/sql"

type SpotifyMetadataFence struct {
	Epoch      string
	ContextKey string
}

// Take the SQLite writer reservation before reading account fences or prior
// snapshots. WAL readers cannot safely upgrade after another writer commits;
// acquiring here lets the busy timeout wait, then checks the current owner.
func (d *DB) beginSpotifyWriteTx() (*sql.Tx, error) {
	tx, err := d.conn.Begin()
	if err != nil {
		return nil, err
	}
	if _, err := tx.Exec(`UPDATE settings SET value=value WHERE 0`); err != nil {
		_ = tx.Rollback()
		return nil, err
	}
	return tx, nil
}

func checkSpotifyMetadataFenceTx(tx *sql.Tx, fence SpotifyMetadataFence) error {
	var epoch, contextKey string
	if err := tx.QueryRow(`SELECT COALESCE((SELECT value FROM settings WHERE key='spotify_metadata_runtime_epoch'),''), COALESCE((SELECT value FROM settings WHERE key='spotify_metadata_active_context'),'')`).Scan(&epoch, &contextKey); err != nil {
		return err
	}
	if fence.Epoch == "" || fence.ContextKey == "" || fence.Epoch != epoch || fence.ContextKey != contextKey {
		return ErrSpotifyMetadataRuntimeSuperseded
	}
	return nil
}

func (d *DB) PutSpotifyEntitySnapshotsForRuntime(fence SpotifyMetadataFence, snapshots []SpotifyEntitySnapshot) error {
	return d.putSpotifyEntitySnapshots(snapshots, nil, false, &fence)
}
