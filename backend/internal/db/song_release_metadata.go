package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/releasedate"
)

type TrackReleaseMetadata struct {
	EditionDate    string `json:"editionDate,omitempty"`
	EditionSource  string `json:"editionSource,omitempty"`
	OriginalDate   string `json:"originalDate,omitempty"`
	OriginalSource string `json:"originalSource,omitempty"`
	RecordingID    string `json:"recordingId,omitempty"`
	State          string `json:"state"`
	CheckedAt      int64  `json:"checkedAt"`
	RetryAt        int64  `json:"retryAt"`
}

func (d *DB) ensureSongReleaseMetadataSchema() error {
	_, err := d.conn.Exec(`CREATE TABLE IF NOT EXISTS song_release_metadata (
	 song_id TEXT PRIMARY KEY REFERENCES songs(id) ON DELETE CASCADE,
	 title TEXT NOT NULL DEFAULT '',artist TEXT NOT NULL DEFAULT '',album TEXT NOT NULL DEFAULT '',file_hash TEXT NOT NULL DEFAULT '',
	 edition_date TEXT NOT NULL DEFAULT '',edition_source TEXT NOT NULL DEFAULT '',original_date TEXT NOT NULL DEFAULT '',original_source TEXT NOT NULL DEFAULT '',
	 recording_id TEXT NOT NULL DEFAULT '',state TEXT NOT NULL DEFAULT 'pending',checked_at INTEGER NOT NULL DEFAULT 0,retry_at INTEGER NOT NULL DEFAULT 0,manual_year INTEGER NOT NULL DEFAULT 0
	); CREATE INDEX IF NOT EXISTS idx_release_metadata_retry ON song_release_metadata(retry_at);`)
	return err
}

func (d *DB) GetTrackReleaseMetadata(id string) (TrackReleaseMetadata, error) {
	var value TrackReleaseMetadata
	err := d.conn.QueryRow(`SELECT r.edition_date,r.edition_source,r.original_date,r.original_source,r.recording_id,r.state,r.checked_at,r.retry_at FROM song_release_metadata r JOIN songs s ON s.id=r.song_id WHERE r.song_id=? AND (r.manual_year=1 OR (r.title=s.title AND r.artist=s.artist AND r.album=s.album AND r.file_hash=COALESCE(s.file_hash,'')))`, id).Scan(&value.EditionDate, &value.EditionSource, &value.OriginalDate, &value.OriginalSource, &value.RecordingID, &value.State, &value.CheckedAt, &value.RetryAt)
	if err == sql.ErrNoRows {
		value.State = "pending"
		err = nil
	}
	return value, err
}

func (d *DB) ReleaseMetadataCandidates(ctx context.Context, limit int) ([]Song, error) {
	limit = min(20, max(1, limit))
	rows, err := d.conn.QueryContext(ctx, songSelect+` WHERE COALESCE(songs.ignored,0)=0 AND songs.title<>'' AND songs.artist<>''
	 AND NOT EXISTS(SELECT 1 FROM song_release_metadata r WHERE r.song_id=songs.id AND
	 (r.manual_year=1 OR (r.title=songs.title AND r.artist=songs.artist AND r.album=songs.album AND r.file_hash=COALESCE(songs.file_hash,'') AND r.retry_at>?)))
	 ORDER BY songs.added_at DESC,songs.id LIMIT ?`, time.Now().UnixMilli(), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanLibrarySongRows(rows)
}

// Compare the exact catalog identity again inside the writer transaction.
// Background lookups never overwrite a manual year or a different track.
func (d *DB) ApplyTrackReleaseMetadata(ctx context.Context, target Song, value TrackReleaseMetadata) (bool, error) {
	if value.EditionDate != "" && releasedate.Year(value.EditionDate) == 0 || value.OriginalDate != "" && releasedate.Year(value.OriginalDate) == 0 {
		return false, fmt.Errorf("invalid release date")
	}
	if value.OriginalDate != "" && value.OriginalSource != "musicbrainz" && value.OriginalSource != "file_tag" {
		return false, fmt.Errorf("unverified original release source")
	}
	tx, err := d.conn.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `UPDATE settings SET value=value WHERE 0`); err != nil {
		return false, err
	}
	var current, manual bool
	err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM songs WHERE id=? AND title=? AND artist=? AND album=? AND COALESCE(file_hash,'')=?),EXISTS(SELECT 1 FROM song_release_metadata WHERE song_id=? AND manual_year=1)`, target.ID, target.Title, target.Artist, target.Album, target.FileHash, target.ID).Scan(&current, &manual)
	if err != nil || !current || manual {
		return false, err
	}
	// An old source's dates must not carry into a replacement track when this
	// lookup only records an unsuccessful attempt.
	if _, err = tx.ExecContext(ctx, `DELETE FROM song_release_metadata WHERE song_id=? AND manual_year=0 AND (title<>? OR artist<>? OR album<>? OR file_hash<>?)`, target.ID, target.Title, target.Artist, target.Album, target.FileHash); err != nil {
		return false, err
	}
	if value.CheckedAt == 0 {
		value.CheckedAt = time.Now().UnixMilli()
	}
	if value.RetryAt == 0 {
		value.RetryAt = time.Now().Add(30 * 24 * time.Hour).UnixMilli()
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO song_release_metadata(song_id,title,artist,album,file_hash,edition_date,edition_source,original_date,original_source,recording_id,state,checked_at,retry_at)
	 VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(song_id) DO UPDATE SET title=excluded.title,artist=excluded.artist,album=excluded.album,file_hash=excluded.file_hash,
	 edition_date=CASE WHEN excluded.edition_date<>'' THEN excluded.edition_date ELSE song_release_metadata.edition_date END,
	 edition_source=CASE WHEN excluded.edition_date<>'' THEN excluded.edition_source ELSE song_release_metadata.edition_source END,
	 original_date=CASE WHEN excluded.original_date<>'' THEN excluded.original_date ELSE song_release_metadata.original_date END,
	 original_source=CASE WHEN excluded.original_date<>'' THEN excluded.original_source ELSE song_release_metadata.original_source END,
	 recording_id=excluded.recording_id,state=excluded.state,checked_at=excluded.checked_at,retry_at=excluded.retry_at`, target.ID, target.Title, target.Artist, target.Album, target.FileHash, value.EditionDate, value.EditionSource, value.OriginalDate, value.OriginalSource, value.RecordingID, value.State, value.CheckedAt, value.RetryAt)
	if err != nil {
		return false, err
	}
	_, err = tx.ExecContext(ctx, `UPDATE songs SET year=CASE WHEN COALESCE(year,0)=0 AND ?>0 THEN ? ELSE year END,
	 original_year=CASE WHEN ?>0 THEN ? ELSE original_year END,
	 year_uncertain=CASE WHEN ?>0 THEN 0 ELSE year_uncertain END WHERE id=?`, releasedate.Year(value.EditionDate), releasedate.Year(value.EditionDate), releasedate.Year(value.OriginalDate), releasedate.Year(value.OriginalDate), releasedate.Year(value.OriginalDate), target.ID)
	if err != nil {
		return false, err
	}
	return true, tx.Commit()
}

func recordReleaseTagsTx(tx *sql.Tx, song Song) error {
	if _, err := tx.Exec(`DELETE FROM song_release_metadata WHERE song_id=? AND manual_year=0 AND (title<>? OR artist<>? OR album<>? OR file_hash<>?)`, song.ID, song.Title, song.Artist, song.Album, song.FileHash); err != nil {
		return err
	}

	date, _, _ := releasedate.Parse(song.ReleaseDate)
	original, _, _ := releasedate.Parse(song.OriginalReleaseDate)
	if date == "" && original == "" {
		return nil
	}
	editionSource, originalSource := "", ""
	if date != "" {
		editionSource = "file_tag"
	}
	if original != "" {
		originalSource = "file_tag"
	}
	_, err := tx.Exec(`INSERT INTO song_release_metadata(song_id,title,artist,album,file_hash,edition_date,edition_source,original_date,original_source)
	 VALUES(?,?,?,?,?,?,?, ?,?) ON CONFLICT(song_id) DO UPDATE SET title=excluded.title,artist=excluded.artist,album=excluded.album,file_hash=excluded.file_hash,
	 edition_date=CASE WHEN excluded.edition_date<>'' THEN excluded.edition_date ELSE song_release_metadata.edition_date END,
	 edition_source=CASE WHEN excluded.edition_date<>'' THEN excluded.edition_source ELSE song_release_metadata.edition_source END,
	 original_date=CASE WHEN excluded.original_date<>'' AND song_release_metadata.manual_year=0 THEN excluded.original_date ELSE song_release_metadata.original_date END,
	 original_source=CASE WHEN excluded.original_date<>'' AND song_release_metadata.manual_year=0 THEN excluded.original_source ELSE song_release_metadata.original_source END`, song.ID, song.Title, song.Artist, song.Album, song.FileHash, date, editionSource, original, originalSource)
	return err
}
