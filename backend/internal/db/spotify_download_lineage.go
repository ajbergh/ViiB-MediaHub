package db

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
	"unicode"
)

// Explicit request provenance, not inferred playlist or saved-library membership.
// Position is zero-based; -1 represents an origin without a known position.
type SpotifyDownloadOrigin struct {
	EntityID string `json:"entityId,omitempty"`
	Kind     string `json:"kind"`
	ID       string `json:"id"`
	Revision string `json:"revision,omitempty"`
	Position int    `json:"position"`
}

type DownloadedLineageStatus struct {
	State     string    `json:"state"`
	CheckedAt time.Time `json:"checkedAt"`
}

func validDownloadOrigin(o SpotifyDownloadOrigin) bool {
	if o.Position < -1 || o.Position > 1000000 || len(o.Revision) > 256 || strings.IndexFunc(o.Revision, unicode.IsControl) >= 0 {
		return false
	}
	switch o.Kind {
	case "playlist", "album":
		return o.EntityID == "" && ValidSpotifyRecordingID(o.ID)
	case "library":
		return (o.EntityID == "" || ValidSpotifyRecordingID(o.EntityID)) && (o.ID == "saved_tracks" || o.ID == "saved_albums" || o.ID == "saved_playlists")
	}
	return false
}

func stageDownloadOrigins(tx *sql.Tx, id string, origins []SpotifyDownloadOrigin) error {
	if len(origins) == 0 {
		return nil
	}
	if len(origins) > 32 {
		return errors.New("too many download origins")
	}
	var account string
	if err := tx.QueryRow("SELECT COALESCE((SELECT value FROM settings WHERE key='spotify_metadata_active_context'),'')").Scan(&account); err != nil {
		return err
	}
	if account == "" {
		return errors.New("active account required for download origins")
	}
	for _, origin := range origins {
		if !validDownloadOrigin(origin) {
			return errors.New("invalid download origin")
		}
		if _, err := tx.Exec(`INSERT OR IGNORE INTO spotify_download_lineage_staging(download_id,context_key,origin_kind,origin_id,origin_revision,position,entity_id) VALUES(?,?,?,?,?,?,?)`, id, account, origin.Kind, origin.ID, origin.Revision, origin.Position, origin.EntityID); err != nil {
			return err
		}
	}
	var count int
	if err := tx.QueryRow("SELECT COUNT(*) FROM spotify_download_lineage_staging WHERE download_id=?", id).Scan(&count); err != nil {
		return err
	}
	if count > 32 {
		return errors.New("too many cumulative download origins")
	}
	return nil
}

func promoteDownloadLineage(ctx context.Context, tx *sql.Tx, id string, r downloadFileRevision, recording string, now int64) error {
	args := []any{r.path, r.digest, r.size, r.mtime, recording, id}
	const selection = `WITH retained AS (SELECT origin_kind,origin_id,origin_revision,position,entity_id FROM spotify_download_lineage_imports WHERE file_path=? AND content_sha256=? AND file_size=? AND mtime_ns=? AND recording_id=?),
 staged AS (SELECT origin_kind,origin_id,origin_revision,position,entity_id FROM spotify_download_lineage_staging WHERE download_id=? AND context_key<>'' AND context_key=(SELECT value FROM settings WHERE key='spotify_metadata_active_context')),
 combined AS (SELECT * FROM retained UNION SELECT * FROM staged)`
	var count, staged int
	if err := tx.QueryRowContext(ctx, selection+` SELECT (SELECT COUNT(*) FROM combined),(SELECT COUNT(*) FROM staged)`, args...).Scan(&count, &staged); err != nil {
		return err
	}
	state := "available"
	if count > 64 {
		state = "oversized"
	} else {
		if staged == 0 {
			state = "no_active_request_lineage"
		}
		insertArgs := append(append([]any{}, args...), r.path, r.digest, r.size, r.mtime, recording)
		if _, err := tx.ExecContext(ctx, selection+` INSERT OR IGNORE INTO spotify_download_lineage_imports(file_path,content_sha256,file_size,mtime_ns,recording_id,origin_kind,origin_id,origin_revision,position,entity_id) SELECT ?,?,?,?,?,origin_kind,origin_id,origin_revision,position,entity_id FROM staged`, insertArgs...); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO spotify_download_lineage_status(file_path,content_sha256,file_size,mtime_ns,recording_id,state,checked_at) VALUES(?,?,?,?,?,?,?) ON CONFLICT(file_path,content_sha256,file_size,mtime_ns,recording_id) DO UPDATE SET state=excluded.state,checked_at=excluded.checked_at`, r.path, r.digest, r.size, r.mtime, recording, state, now); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, "DELETE FROM spotify_download_lineage_staging WHERE download_id=?", id)
	return err
}

func readDownloadLineage(ctx context.Context, tx *sql.DB, r downloadFileRevision, recording string, result *DownloadedSpotifyCatalog) error {
	var status DownloadedLineageStatus
	var checked int64
	err := tx.QueryRowContext(ctx, `SELECT state,checked_at FROM spotify_download_lineage_status WHERE file_path=? AND content_sha256=? AND file_size=? AND mtime_ns=? AND recording_id=?`, r.path, r.digest, r.size, r.mtime, recording).Scan(&status.State, &checked)
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	if err == nil {
		if (status.State != "available" && status.State != "oversized" && status.State != "no_active_request_lineage") || checked < 0 {
			return errors.New("invalid download lineage status")
		}
		status.CheckedAt = time.UnixMilli(checked).UTC()
		result.LineageStatus = &status
	}
	rows, err := tx.QueryContext(ctx, `SELECT origin_kind,origin_id,origin_revision,position,entity_id FROM spotify_download_lineage_imports WHERE file_path=? AND content_sha256=? AND file_size=? AND mtime_ns=? AND recording_id=? ORDER BY origin_kind,origin_id,origin_revision,position,entity_id LIMIT 65`, r.path, r.digest, r.size, r.mtime, recording)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var origin SpotifyDownloadOrigin
		if err := rows.Scan(&origin.Kind, &origin.ID, &origin.Revision, &origin.Position, &origin.EntityID); err != nil {
			return err
		}
		if !validDownloadOrigin(origin) || len(result.Origins) >= 64 {
			return errors.New("invalid retained download origin")
		}
		result.Origins = append(result.Origins, origin)
	}
	return rows.Err()
}

func ValidateSpotifyDownloadOrigins(origins []SpotifyDownloadOrigin) error {
	if len(origins) > 32 {
		return errors.New("too many download origins")
	}
	for _, o := range origins {
		if !validDownloadOrigin(o) {
			return errors.New("invalid download origin")
		}
	}
	return nil
}
