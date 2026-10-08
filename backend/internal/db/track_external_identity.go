// Manages explicit external recording links tied to the current local source fingerprint.
package db

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"regexp"
	"time"
)

var spotifyRecordingID = regexp.MustCompile("^[A-Za-z0-9]{22}$")

// ValidSpotifyRecordingID accepts only a recording ID, never a URL or album ID.
func ValidSpotifyRecordingID(id string) bool { return spotifyRecordingID.MatchString(id) }

// SpotifySearchAllowed preserves explicit removals and stronger recording
// identities, including manual links to a previous source revision.
func (d *DB) SpotifySearchAllowed(songID, fingerprint string) (bool, error) {
	if err := d.EnsureExternalTrackAnalysisSchema(); err != nil {
		return false, err
	}
	var allowed bool
	err := d.conn.QueryRow(`SELECT NOT EXISTS (
 SELECT 1 FROM track_external_identity_suppression WHERE song_id=? AND source_fingerprint=?)
 AND NOT EXISTS (SELECT 1 FROM track_external_identity WHERE song_id=? AND provider='spotify' AND link_origin!='automatic_search')`, songID, fingerprint, songID).Scan(&allowed)
	return allowed, err
}

// SaveSpotifySearchMatch never upgrades an inferred match to manual confirmation.
func (d *DB) SaveSpotifySearchMatch(songID, id, fingerprint string) (bool, error) {
	if songID == "" || fingerprint == "" || !ValidSpotifyRecordingID(id) {
		return false, errors.New("recording ID and current source required")
	}
	if err := d.EnsureTrackAnalysisSchema(); err != nil {
		return false, err
	}
	if err := d.EnsureExternalTrackAnalysisSchema(); err != nil {
		return false, err
	}
	result, err := d.conn.Exec(`INSERT INTO track_external_identity
 (song_id,provider,external_id,link_origin,source_fingerprint,confirmed_at)
 SELECT ?,'spotify',?,'automatic_search',?,? WHERE EXISTS (
 SELECT 1 FROM track_analysis_source_revisions WHERE song_id=? AND source_fingerprint=?)
 AND NOT EXISTS (SELECT 1 FROM track_external_identity_suppression WHERE song_id=? AND source_fingerprint=?)
 ON CONFLICT(song_id,provider) DO UPDATE SET external_id=excluded.external_id,
 link_origin=excluded.link_origin,source_fingerprint=excluded.source_fingerprint,confirmed_at=excluded.confirmed_at
 WHERE track_external_identity.link_origin='automatic_search'`, songID, id, fingerprint, time.Now().UnixMilli(), songID, fingerprint, songID, fingerprint)
	if err != nil {
		return false, err
	}
	n, err := result.RowsAffected()
	return n == 1, err
}

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
	var suppressedFile downloadFileRevision
	var suppressedHash, suppressedCurrentPath string
	exactErr := d.conn.QueryRow(`SELECT p.file_path,p.content_sha256,p.file_size,p.mtime_ns,COALESCE(s.file_hash,''),s.file_path FROM spotify_download_suppression_bindings p JOIN songs s ON s.id=p.song_id WHERE p.song_id=? AND p.source_fingerprint=?`, songID, fingerprint).Scan(&suppressedFile.path, &suppressedFile.digest, &suppressedFile.size, &suppressedFile.mtime, &suppressedHash, &suppressedCurrentPath)
	if exactErr != nil && !errors.Is(exactErr, sql.ErrNoRows) {
		return false, exactErr
	}
	if exactErr == nil {
		if suppressedCurrentPath != suppressedFile.path {
			return false, nil
		}
		actual, err := readDownloadRevision(context.Background(), suppressedFile.path)
		if err != nil || actual != suppressedFile {
			return false, nil
		}
		info, err := os.Lstat(suppressedFile.path)
		if err != nil || LocalSourceFingerprint(Song{FilePath: suppressedFile.path, FileHash: suppressedHash}, info) != fingerprint {
			return false, nil
		}
	}
	tx, err := d.conn.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	if exactErr == nil {
		var current bool
		if err = tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM spotify_download_suppression_bindings p JOIN songs s ON s.id=p.song_id WHERE p.song_id=? AND p.source_fingerprint=? AND p.file_path=? AND p.content_sha256=? AND p.file_size=? AND p.mtime_ns=? AND s.file_path=? AND COALESCE(s.file_hash,'')=?)`, songID, fingerprint, suppressedFile.path, suppressedFile.digest, suppressedFile.size, suppressedFile.mtime, suppressedFile.path, suppressedHash).Scan(&current); err != nil {
			return false, err
		}
		if !current {
			return false, nil
		}
	}
	// A prior explicit removal is source-revision scoped. A later explicit
	// recording confirmation is the user's deliberate opt-in to relink this
	// exact revision, so clear only that revision's suppression atomically.
	if _, err := tx.Exec(`DELETE FROM track_external_identity_suppression WHERE song_id=? AND source_fingerprint=?`, songID, fingerprint); err != nil {
		return false, err
	}
	result, err := tx.Exec(`INSERT INTO track_external_identity
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
	if err != nil {
		return false, err
	}
	if n != 1 {
		return false, nil
	}
	if exactErr == nil {
		actual, verifyErr := readDownloadRevision(context.Background(), suppressedFile.path)
		if verifyErr != nil || actual != suppressedFile {
			return false, nil
		}
		if _, err = tx.Exec(`INSERT INTO spotify_download_import_bindings(song_id,source_fingerprint,spotify_id,file_path,content_sha256,file_size,mtime_ns)
 SELECT ?,?,?,?,?,?,? WHERE EXISTS(SELECT 1 FROM spotify_download_evidence WHERE file_path=? AND content_sha256=? AND file_size=? AND mtime_ns=? AND spotify_id=?)
 ON CONFLICT(song_id,source_fingerprint) DO UPDATE SET spotify_id=excluded.spotify_id,file_path=excluded.file_path,content_sha256=excluded.content_sha256,file_size=excluded.file_size,mtime_ns=excluded.mtime_ns`, songID, fingerprint, id, suppressedFile.path, suppressedFile.digest, suppressedFile.size, suppressedFile.mtime, suppressedFile.path, suppressedFile.digest, suppressedFile.size, suppressedFile.mtime, id); err != nil {
			return false, err
		}
		if _, err = tx.Exec(`DELETE FROM spotify_download_revision_suppression WHERE file_path=? AND content_sha256=? AND file_size=? AND mtime_ns=?`, suppressedFile.path, suppressedFile.digest, suppressedFile.size, suppressedFile.mtime); err != nil {
			return false, err
		}
		if _, err = tx.Exec(`DELETE FROM spotify_download_suppression_bindings WHERE song_id=? AND source_fingerprint=?`, songID, fingerprint); err != nil {
			return false, err
		}
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
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
	_, err = tx.Exec(`INSERT OR IGNORE INTO spotify_download_revision_suppression
 SELECT b.file_path,b.content_sha256,b.file_size,b.mtime_ns FROM spotify_download_import_bindings b
 JOIN track_external_identity_suppression s ON s.song_id=b.song_id AND s.source_fingerprint=b.source_fingerprint
 WHERE b.song_id=?`, songID)
	if err != nil {
		return err
	}
	if _, err = tx.Exec(`INSERT OR REPLACE INTO spotify_download_suppression_bindings
 SELECT b.song_id,b.source_fingerprint,b.file_path,b.content_sha256,b.file_size,b.mtime_ns FROM spotify_download_import_bindings b JOIN track_external_identity_suppression s ON s.song_id=b.song_id AND s.source_fingerprint=b.source_fingerprint WHERE b.song_id=?`, songID); err != nil {
		return err
	}
	_, err = tx.Exec("DELETE FROM track_external_identity WHERE song_id=? AND provider='spotify'", songID)
	if err != nil {
		return err
	}
	err = tx.Commit()
	return err
}
