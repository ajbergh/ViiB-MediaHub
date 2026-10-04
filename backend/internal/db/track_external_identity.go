package db

import (
	"database/sql"
	"errors"
	"regexp"
	"time"
)

var spotifyRecordingID = regexp.MustCompile("^[A-Za-z0-9]{22}$")

// ValidSpotifyRecordingID accepts only a recording ID, never a URL or album ID.
func ValidSpotifyRecordingID(id string) bool { return spotifyRecordingID.MatchString(id) }

type ExternalTrackIdentity struct {
	SongID            string `json:"songId"`
	Provider          string `json:"provider"`
	ExternalID        string `json:"externalId"`
	LinkOrigin        string `json:"linkOrigin"`
	SourceFingerprint string `json:"sourceFingerprint"`
	ConfirmedAt       int64  `json:"confirmedAt"`
}

// ConfirmSpotifyRecording requires an explicit recording confirmation and the
// latest resolved source revision. A stale confirmation cannot replace a link.
func (d *DB) ConfirmSpotifyRecording(songID, id, fingerprint string, confirmed bool) (bool, error) {
	if !confirmed || songID == "" || fingerprint == "" || !ValidSpotifyRecordingID(id) {
		return false, errors.New("explicit recording confirmation and source fingerprint required")
	}
	if err := d.EnsureTrackAnalysisSchema(); err != nil {
		return false, err
	}
	if err := d.EnsureExternalTrackAnalysisSchema(); err != nil {
		return false, err
	}
	result, err := d.conn.Exec(`INSERT INTO track_external_identity
 (song_id,provider,external_id,link_origin,source_fingerprint,confirmed_at)
 SELECT ?,'spotify',?,'manual_confirmation',?,? WHERE EXISTS (
 SELECT 1 FROM track_analysis_source_revisions WHERE song_id=? AND source_fingerprint=?)
 ON CONFLICT(song_id,provider) DO UPDATE SET external_id=excluded.external_id,
 link_origin=excluded.link_origin,source_fingerprint=excluded.source_fingerprint,confirmed_at=excluded.confirmed_at`,
		songID, id, fingerprint, time.Now().UnixMilli(), songID, fingerprint)
	if err != nil {
		return false, err
	}
	n, err := result.RowsAffected()
	return n == 1, err
}

// GetSpotifyRecording checks a freshly resolved fingerprint supplied by the
// caller; unavailable or changed audio never yields an active recording link.
func (d *DB) GetSpotifyRecording(songID, currentFingerprint string) (*ExternalTrackIdentity, error) {
	if err := d.EnsureExternalTrackAnalysisSchema(); err != nil {
		return nil, err
	}
	if currentFingerprint == "" {
		return nil, nil
	}
	var link ExternalTrackIdentity
	err := d.conn.QueryRow(`SELECT song_id,provider,external_id,link_origin,source_fingerprint,confirmed_at
 FROM track_external_identity WHERE song_id=? AND provider='spotify' AND source_fingerprint=?`, songID, currentFingerprint).
		Scan(&link.SongID, &link.Provider, &link.ExternalID, &link.LinkOrigin, &link.SourceFingerprint, &link.ConfirmedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &link, nil
}

func (d *DB) DeleteSpotifyRecording(songID string) error {
	if err := d.EnsureExternalTrackAnalysisSchema(); err != nil {
		return err
	}
	tx, err := d.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.Exec(`INSERT OR IGNORE INTO track_external_identity_suppression (song_id,source_fingerprint)
 SELECT song_id,source_fingerprint FROM track_external_identity WHERE song_id=? AND provider='spotify'`, songID)
	if err != nil {
		return err
	}
	_, err = tx.Exec("DELETE FROM track_external_identity WHERE song_id=? AND provider='spotify'", songID)
	if err != nil {
		return err
	}
	err = tx.Commit()
	return err
}
