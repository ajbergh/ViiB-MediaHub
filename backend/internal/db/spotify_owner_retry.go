package db

import (
	"database/sql"
	"errors"
	"time"
)

// SpotifyOwnerValidationRetry contains only retry timing and a sanitized
// failure classification. It never contains provider error text or credentials.
type SpotifyOwnerValidationRetry struct {
	Provider   string
	RetryKey   string
	Failures   int
	CheckedAt  time.Time
	RetryAt    time.Time
	Outcome    string
	HTTPStatus int
}

func validSpotifyOwnerRetry(provider, retryKey string) bool {
	return (provider == "oauth" || provider == "webplayer") && retryKey != "" && len(retryKey) <= 256
}

// GetSpotifyOwnerValidationRetry only returns a retry for the exact active
// credential/owner key supplied by the caller.
func (d *DB) GetSpotifyOwnerValidationRetry(provider, retryKey string) (*SpotifyOwnerValidationRetry, error) {
	if !validSpotifyOwnerRetry(provider, retryKey) {
		return nil, errors.New("invalid Spotify owner retry key")
	}
	var retry SpotifyOwnerValidationRetry
	var checked, next int64
	err := d.conn.QueryRow(`SELECT provider,retry_key,failures,checked_at,retry_at,outcome,http_status
		FROM spotify_owner_validation_retry WHERE singleton=1 AND provider=? AND retry_key=?`, provider, retryKey).
		Scan(&retry.Provider, &retry.RetryKey, &retry.Failures, &checked, &next, &retry.Outcome, &retry.HTTPStatus)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	retry.CheckedAt = time.UnixMilli(checked).UTC()
	retry.RetryAt = time.UnixMilli(next).UTC()
	return &retry, nil
}

// SaveSpotifyOwnerValidationRetry is fenced by the current runtime epoch so a
// retired account cannot overwrite the replacement runtime's retry record.
func (d *DB) SaveSpotifyOwnerValidationRetry(epoch string, retry SpotifyOwnerValidationRetry) error {
	if epoch == "" || !validSpotifyOwnerRetry(retry.Provider, retry.RetryKey) || retry.Failures < 1 || retry.CheckedAt.IsZero() || !retry.RetryAt.After(retry.CheckedAt) {
		return errors.New("invalid Spotify owner retry state")
	}
	if (retry.Outcome != "network" && retry.Outcome != "http" && retry.Outcome != "invalid_response") || retry.HTTPStatus < 0 || retry.HTTPStatus > 599 {
		return errors.New("invalid Spotify owner retry classification")
	}
	tx, err := d.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var activeEpoch string
	if err := tx.QueryRow(`SELECT COALESCE((SELECT value FROM settings WHERE key='spotify_metadata_runtime_epoch'),'')`).Scan(&activeEpoch); err != nil {
		return err
	}
	if activeEpoch != epoch {
		return ErrSpotifyMetadataRuntimeSuperseded
	}
	if _, err := tx.Exec(`INSERT INTO spotify_owner_validation_retry(singleton,provider,retry_key,failures,checked_at,retry_at,outcome,http_status)
		VALUES(1,?,?,?,?,?,?,?) ON CONFLICT(singleton) DO UPDATE SET provider=excluded.provider,retry_key=excluded.retry_key,
		failures=excluded.failures,checked_at=excluded.checked_at,retry_at=excluded.retry_at,outcome=excluded.outcome,http_status=excluded.http_status`,
		retry.Provider, retry.RetryKey, retry.Failures, retry.CheckedAt.UnixMilli(), retry.RetryAt.UnixMilli(), retry.Outcome, retry.HTTPStatus); err != nil {
		return err
	}
	return tx.Commit()
}

// ClearSpotifyOwnerValidationRetry is likewise epoch-fenced and is used when
// the active login selection changes before the new owner is confirmed.
func (d *DB) ClearSpotifyOwnerValidationRetry(epoch string) error {
	if epoch == "" {
		return errors.New("invalid Spotify runtime epoch")
	}
	tx, err := d.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var activeEpoch string
	if err := tx.QueryRow(`SELECT COALESCE((SELECT value FROM settings WHERE key='spotify_metadata_runtime_epoch'),'')`).Scan(&activeEpoch); err != nil {
		return err
	}
	if activeEpoch != epoch {
		return ErrSpotifyMetadataRuntimeSuperseded
	}
	if _, err := tx.Exec(`DELETE FROM spotify_owner_validation_retry`); err != nil {
		return err
	}
	return tx.Commit()
}
