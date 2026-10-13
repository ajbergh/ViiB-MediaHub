package db

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
)

// Revision tombstones survive library removal. They express a user choice,
// independent of queue, account, and imported-payload retention lifetimes.
func migrateDownloadSuppression(tx *sql.Tx) error {
	_, err := tx.Exec(`CREATE TABLE IF NOT EXISTS spotify_download_revision_suppression (
 file_path TEXT NOT NULL, content_sha256 TEXT NOT NULL, file_size INTEGER NOT NULL, mtime_ns INTEGER NOT NULL,
 PRIMARY KEY(file_path,content_sha256,file_size,mtime_ns));
 CREATE TABLE IF NOT EXISTS spotify_download_suppression_bindings (
 song_id TEXT NOT NULL REFERENCES songs(id) ON DELETE CASCADE, source_fingerprint TEXT NOT NULL,
 file_path TEXT NOT NULL, content_sha256 TEXT NOT NULL, file_size INTEGER NOT NULL, mtime_ns INTEGER NOT NULL,
 PRIMARY KEY(song_id,source_fingerprint));
 INSERT OR IGNORE INTO spotify_download_suppression_bindings
 SELECT b.song_id,b.source_fingerprint,b.file_path,b.content_sha256,b.file_size,b.mtime_ns FROM spotify_download_import_bindings b
 JOIN track_external_identity_suppression s ON s.song_id=b.song_id AND s.source_fingerprint=b.source_fingerprint;
 INSERT OR IGNORE INTO spotify_download_revision_suppression
 SELECT b.file_path,b.content_sha256,b.file_size,b.mtime_ns FROM spotify_download_import_bindings b
 JOIN track_external_identity_suppression s ON s.song_id=b.song_id AND s.source_fingerprint=b.source_fingerprint;`)
	return err
}

// Restore suppression and its evidence association under the new canonical ID.
// The caller has just verified the actual physical revision and fingerprint.
func (d *DB) restoreDownloadSuppression(ctx context.Context, song Song, fingerprint, recording string, file downloadFileRevision) (bool, error) {
	tx, err := d.conn.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	// Retire only a projection whose exact provenance no longer matches. Legacy
	// unqualified song suppressions retain their existing source-token semantics.
	if _, err = tx.ExecContext(ctx, `DELETE FROM track_external_identity_suppression WHERE song_id=? AND source_fingerprint=? AND EXISTS(
 SELECT 1 FROM spotify_download_suppression_bindings p WHERE p.song_id=? AND p.source_fingerprint=?
 AND (p.file_path!=? OR p.content_sha256!=? OR p.file_size!=? OR p.mtime_ns!=?))`, song.ID, fingerprint, song.ID, fingerprint, file.path, file.digest, file.size, file.mtime); err != nil {
		return false, err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM spotify_download_suppression_bindings WHERE song_id=? AND source_fingerprint=? AND (file_path!=? OR content_sha256!=? OR file_size!=? OR mtime_ns!=?)`, song.ID, fingerprint, file.path, file.digest, file.size, file.mtime); err != nil {
		return false, err
	}
	_, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO track_external_identity_suppression(song_id,source_fingerprint)
 SELECT ?,? WHERE EXISTS(SELECT 1 FROM spotify_download_revision_suppression WHERE file_path=? AND content_sha256=? AND file_size=? AND mtime_ns=?)
 AND EXISTS(SELECT 1 FROM songs WHERE id=? AND file_path=? AND COALESCE(file_hash,'')=?)`, song.ID, fingerprint, file.path, file.digest, file.size, file.mtime, song.ID, song.FilePath, song.FileHash)
	if err != nil {
		return false, err
	}
	var suppressed bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM track_external_identity_suppression WHERE song_id=? AND source_fingerprint=?)`, song.ID, fingerprint).Scan(&suppressed); err != nil {
		return false, err
	}
	if suppressed {
		if _, err = tx.ExecContext(ctx, `DELETE FROM track_external_identity WHERE song_id=? AND provider='spotify' AND source_fingerprint=?`, song.ID, fingerprint); err != nil {
			return false, err
		}
		_, err = tx.ExecContext(ctx, `INSERT OR REPLACE INTO spotify_download_suppression_bindings(song_id,source_fingerprint,file_path,content_sha256,file_size,mtime_ns)
 SELECT ?,?,?,?,?,? WHERE EXISTS(SELECT 1 FROM spotify_download_revision_suppression WHERE file_path=? AND content_sha256=? AND file_size=? AND mtime_ns=?)`, song.ID, fingerprint, file.path, file.digest, file.size, file.mtime, file.path, file.digest, file.size, file.mtime)
		if err != nil {
			return false, err
		}
	}
	if suppressed && ValidSpotifyRecordingID(recording) {
		_, err = tx.ExecContext(ctx, `INSERT INTO spotify_download_import_bindings(song_id,source_fingerprint,spotify_id,file_path,content_sha256,file_size,mtime_ns)
 SELECT ?,?,?,?,?,?,? WHERE EXISTS(SELECT 1 FROM songs WHERE id=? AND file_path=? AND COALESCE(file_hash,'')=?)
 ON CONFLICT(song_id,source_fingerprint) DO UPDATE SET spotify_id=excluded.spotify_id,file_path=excluded.file_path,content_sha256=excluded.content_sha256,file_size=excluded.file_size,mtime_ns=excluded.mtime_ns`, song.ID, fingerprint, recording, file.path, file.digest, file.size, file.mtime, song.ID, song.FilePath, song.FileHash)
		if err != nil {
			return false, err
		}
	}
	if err = tx.Commit(); err != nil {
		return false, err
	}
	return suppressed, nil
}

// currentDownloadSuppression discovers retained user choices independently of
// scanner restoration. Unrelated paths require no filesystem reads. A path
// with retained choices is hashed so coarse fingerprint collisions cannot
// suppress a different physical revision.
func (d *DB) currentDownloadSuppression(ctx context.Context, songID, fingerprint string) (Song, downloadFileRevision, bool, error) {
	var song Song
	var file downloadFileRevision
	err := d.conn.QueryRowContext(ctx, `SELECT id,file_path,COALESCE(file_hash,'') FROM songs WHERE id=?`, songID).Scan(&song.ID, &song.FilePath, &song.FileHash)
	if err != nil {
		return song, file, false, err
	}
	absolute, err := filepath.Abs(song.FilePath)
	if err != nil {
		return song, file, false, err
	}
	file.path = filepath.Clean(absolute)
	var candidate bool
	if err = d.conn.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM spotify_download_revision_suppression WHERE file_path=?)`, file.path).Scan(&candidate); err != nil {
		return song, file, false, err
	}
	if !candidate {
		return song, file, false, nil
	}
	actual, err := readDownloadRevision(ctx, file.path)
	if err != nil {
		return song, file, true, err
	}
	info, err := os.Lstat(file.path)
	if err != nil {
		return song, file, true, err
	}
	if info.Size() != actual.size || info.ModTime().UnixNano() != actual.mtime || LocalSourceFingerprint(song, info) != fingerprint {
		return song, file, true, nil
	}
	file = actual
	var blocked bool
	err = d.conn.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM spotify_download_revision_suppression WHERE file_path=? AND content_sha256=? AND file_size=? AND mtime_ns=?)`, file.path, file.digest, file.size, file.mtime).Scan(&blocked)
	return song, file, blocked, err
}

// Automatic admission may retire an obsolete projection after verifying new
// bytes. Confirmation retains it until its stricter stale-choice check runs.
func (d *DB) currentAutomaticDownloadSuppression(ctx context.Context, songID, fingerprint string) (Song, downloadFileRevision, bool, error) {
	song, file, blocked, err := d.currentDownloadSuppression(ctx, songID, fingerprint)
	if err == nil && !blocked && file.digest != "" {
		var obsolete bool
		err = d.conn.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM spotify_download_suppression_bindings WHERE song_id=? AND source_fingerprint=? AND (file_path!=? OR content_sha256!=? OR file_size!=? OR mtime_ns!=?))`, songID, fingerprint, file.path, file.digest, file.size, file.mtime).Scan(&obsolete)
		if err == nil && obsolete {
			_, err = d.restoreDownloadSuppression(ctx, song, fingerprint, "", file)
		}
	}
	return song, file, blocked, err
}
