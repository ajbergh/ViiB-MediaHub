package db

import "database/sql"

type SpotifyMetadataFence struct {
	Epoch      string
	ContextKey string
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
