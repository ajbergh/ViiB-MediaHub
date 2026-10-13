package db

import (
	"errors"
	"time"
)

// BindSpotifyMetadataOwner records the authenticated profile identity of the
// current live context without changing its lifetime or reactivating old data.
func (d *DB) BindSpotifyMetadataOwner(epoch, provider, accountID, contextKey string) error {
	if epoch == "" || (provider != "oauth" && provider != "webplayer") || accountID == "" || len(accountID) > 256 || contextKey == "" || len(contextKey) > 256 {
		return errors.New("invalid Spotify owner identity")
	}
	result, err := d.conn.Exec(`INSERT INTO spotify_metadata_owner(singleton,provider,account_id,context_key,verified_at)
 SELECT 1,?,?,?,? WHERE ?=COALESCE((SELECT value FROM settings WHERE key='spotify_metadata_runtime_epoch'),'')
 AND ?=COALESCE((SELECT value FROM settings WHERE key='spotify_metadata_active_context'),'')
 ON CONFLICT(singleton) DO UPDATE SET provider=excluded.provider,account_id=excluded.account_id,context_key=excluded.context_key,verified_at=excluded.verified_at
 WHERE spotify_metadata_owner.context_key!=excluded.context_key
 OR (spotify_metadata_owner.provider=excluded.provider AND spotify_metadata_owner.account_id=excluded.account_id)`, provider, accountID, contextKey, time.Now().UnixMilli(), epoch, contextKey)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return ErrSpotifyMetadataRuntimeSuperseded
	}
	return nil
}
