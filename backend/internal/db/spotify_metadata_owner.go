package db

import (
	"database/sql"
	"errors"
	"time"
)

var ErrSpotifyMetadataRuntimeSuperseded = errors.New("Spotify metadata runtime superseded")

type SpotifyMetadataOwner struct {
	Provider   string
	AccountID  string
	ContextKey string
	VerifiedAt time.Time
}

// ReserveSpotifyMetadataRuntime fences the previous process before identity
// revalidation, retaining its owner and private rows for possible safe reuse.
func (d *DB) ReserveSpotifyMetadataRuntime(epoch string) (*SpotifyMetadataOwner, error) {
	if epoch == "" || len(epoch) > 256 {
		return nil, errors.New("invalid Spotify runtime epoch")
	}
	tx, err := d.conn.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	for key, value := range map[string]string{"spotify_metadata_runtime_epoch": epoch, "spotify_metadata_active_context": ""} {
		if _, err := tx.Exec(`INSERT INTO settings(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, value); err != nil {
			return nil, err
		}
	}
	var owner SpotifyMetadataOwner
	var verified int64
	err = tx.QueryRow(`SELECT provider,account_id,context_key,verified_at FROM spotify_metadata_owner WHERE singleton=1`).Scan(&owner.Provider, &owner.AccountID, &owner.ContextKey, &verified)
	if err != nil && err != sql.ErrNoRows {
		return nil, err
	}
	if commitErr := tx.Commit(); commitErr != nil {
		return nil, commitErr
	}
	if err == sql.ErrNoRows {
		return nil, nil
	}
	owner.VerifiedAt = time.UnixMilli(verified)
	return &owner, nil
}

// ConfirmSpotifyMetadataOwner accepts an authenticated stable profile ID, never
// a token-derived identifier. Matching identity reuses its random context;
// mismatched identity retires private caches in the same activation transaction.
func (d *DB) ConfirmSpotifyMetadataOwner(epoch, provider, accountID, newContext string) (string, error) {
	if epoch == "" || (provider != "oauth" && provider != "webplayer") || accountID == "" || len(accountID) > 256 || newContext == "" || len(newContext) > 256 {
		return "", errors.New("invalid Spotify owner identity")
	}
	tx, err := d.conn.Begin()
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	var activeEpoch string
	if err := tx.QueryRow(`SELECT COALESCE((SELECT value FROM settings WHERE key='spotify_metadata_runtime_epoch'),'')`).Scan(&activeEpoch); err != nil {
		return "", err
	}
	if epoch != activeEpoch {
		return "", ErrSpotifyMetadataRuntimeSuperseded
	}
	var previous SpotifyMetadataOwner
	err = tx.QueryRow(`SELECT provider,account_id,context_key FROM spotify_metadata_owner WHERE singleton=1`).Scan(&previous.Provider, &previous.AccountID, &previous.ContextKey)
	if err != nil && err != sql.ErrNoRows {
		return "", err
	}
	contextKey := newContext
	if err == nil && previous.Provider == provider && previous.AccountID == accountID && previous.ContextKey != "" {
		contextKey = previous.ContextKey
	} else {
		for _, table := range []string{"spotify_entity_relations", "spotify_entity_snapshots", "spotify_metadata_resource_status", "spotify_audio_artifacts", "spotify_playlist_traversals", "spotify_audio_field_attempts", "spotify_audio_observations", "external_track_analysis", "external_track_analysis_status"} {
			if _, err := tx.Exec("DELETE FROM " + table); err != nil {
				return "", err
			}
		}
	}
	if _, err := tx.Exec(`INSERT INTO spotify_metadata_owner(singleton,provider,account_id,context_key,verified_at) VALUES(1,?,?,?,?) ON CONFLICT(singleton) DO UPDATE SET provider=excluded.provider,account_id=excluded.account_id,context_key=excluded.context_key,verified_at=excluded.verified_at`, provider, accountID, contextKey, time.Now().UnixMilli()); err != nil {
		return "", err
	}
	if _, err := tx.Exec(`INSERT INTO settings(key,value) VALUES('spotify_metadata_active_context',?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, contextKey); err != nil {
		return "", err
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	return contextKey, nil
}
