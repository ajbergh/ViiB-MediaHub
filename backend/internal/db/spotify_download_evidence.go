// Records and reconciles completed Spotify download evidence against validated local file identity.
package db

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// LocalSourceFingerprint is shared with local analysis so recording links use
// precisely the same source revision as the analysis UI.
func LocalSourceFingerprint(song Song, info os.FileInfo) string {
	identity := song.FileHash
	if identity == "" {
		identity = "path:" + filepath.Clean(song.FilePath)
	}
	return identity + ":" + strconv.FormatInt(info.Size(), 10) + ":" + strconv.FormatInt(info.ModTime().UnixMilli(), 10)
}

type downloadFileRevision struct {
	path, digest string
	size, mtime  int64
}

func readDownloadRevision(ctx context.Context, path string) (downloadFileRevision, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return downloadFileRevision{}, err
	}
	absolute = filepath.Clean(absolute)
	before, err := os.Lstat(absolute)
	if err != nil {
		return downloadFileRevision{}, err
	}
	if !before.Mode().IsRegular() {
		return downloadFileRevision{}, errors.New("download artifact must be a regular file")
	}
	file, err := os.Open(absolute)
	if err != nil {
		return downloadFileRevision{}, err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil {
		return downloadFileRevision{}, err
	}
	if !os.SameFile(before, opened) {
		return downloadFileRevision{}, errors.New("download artifact changed before verification")
	}
	hash := sha256.New()
	buffer := make([]byte, 128*1024)
	var total int64
	for {
		if err := ctx.Err(); err != nil {
			return downloadFileRevision{}, err
		}
		n, err := file.Read(buffer)
		if n > 0 {
			hash.Write(buffer[:n])
			total += int64(n)
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return downloadFileRevision{}, err
		}
	}
	after, err := os.Lstat(absolute)
	if err != nil {
		return downloadFileRevision{}, err
	}
	if !os.SameFile(before, after) || before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) || total != after.Size() {
		return downloadFileRevision{}, errors.New("download artifact changed during verification")
	}
	return downloadFileRevision{absolute, hex.EncodeToString(hash.Sum(nil)), after.Size(), after.ModTime().UnixNano()}, nil
}

// MarkDownloadCompletedWithEvidence atomically records successful completion
// and durable evidence for the final tagged/converted artifact. Queue cleanup
// never deletes this evidence. Historical path-only rows are not trusted.
func (d *DB) MarkDownloadCompletedWithEvidence(ctx context.Context, id, path string) (bool, error) {
	if err := d.EnsureExternalTrackAnalysisSchema(); err != nil {
		return false, err
	}
	revision, err := readDownloadRevision(ctx, path)
	if err != nil {
		return false, fmt.Errorf("verify completed artifact: %w", err)
	}
	tx, err := d.conn.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	var recording string
	err = tx.QueryRowContext(ctx, "SELECT spotify_id FROM spotify_downloads WHERE id=? AND status IN ('downloading','converting')", id).Scan(&recording)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !ValidSpotifyRecordingID(recording) {
		return false, errors.New("invalid recording ID in completed download")
	}
	result, err := tx.ExecContext(ctx, `UPDATE spotify_downloads SET status='completed',progress=100,file_path=?,completed_at=?
 WHERE id=? AND status IN ('downloading','converting')`, revision.path, time.Now().Unix(), id)
	if err != nil {
		return false, err
	}
	count, err := result.RowsAffected()
	if err != nil || count != 1 {
		return false, err
	}
	_, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO spotify_download_evidence
 (file_path,content_sha256,file_size,mtime_ns,spotify_id,completed_at) VALUES (?,?,?,?,?,?)`,
		revision.path, revision.digest, revision.size, revision.mtime, recording, time.Now().UnixMilli())
	if err != nil {
		return false, err
	}
	if err = ctx.Err(); err != nil {
		return false, err
	}
	if err = tx.Commit(); err != nil {
		return false, err
	}
	// A library rescan may have finished before the worker completed.
	if err = d.ReconcileSpotifyDownload(ctx, revision.path); err != nil && ctx.Err() == nil {
		log.Printf("Spotify recording reconciliation failed: %v", err)
	}
	return true, nil
}

// ReconcileSpotifyDownload uses only durable completion evidence and the
// canonical persisted path identity. Manual links and explicit removals win.
func (d *DB) ReconcileSpotifyDownload(ctx context.Context, path string) error {
	if err := d.EnsureExternalTrackAnalysisSchema(); err != nil {
		return err
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	absolute = filepath.Clean(absolute)
	var candidates int
	if err = d.conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM spotify_download_evidence WHERE file_path=?", absolute).Scan(&candidates); err != nil {
		return err
	}
	if candidates == 0 {
		return nil
	} // No filesystem reads for unrelated library songs.
	var song Song
	err = d.conn.QueryRowContext(ctx, "SELECT id,file_path,COALESCE(file_hash,'') FROM songs WHERE file_path=?", path).Scan(&song.ID, &song.FilePath, &song.FileHash)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	revision, err := readDownloadRevision(ctx, path)
	if err != nil {
		return nil
	} // Unavailable/changing audio has no active automatic link.
	var recording string
	var distinct int
	err = d.conn.QueryRowContext(ctx, `SELECT COUNT(DISTINCT spotify_id),COALESCE(MIN(spotify_id),'')
 FROM spotify_download_evidence WHERE file_path=? AND content_sha256=? AND file_size=? AND mtime_ns=?`,
		absolute, revision.digest, revision.size, revision.mtime).Scan(&distinct, &recording)
	if err != nil {
		return err
	}
	if distinct != 1 || !ValidSpotifyRecordingID(recording) {
		// New conflicting evidence must also retire a prior automatic identity.
		// Preserve manual confirmation even when automatic evidence becomes invalid.
		_, err = d.conn.ExecContext(ctx, "DELETE FROM track_external_identity WHERE song_id=? AND provider='spotify' AND link_origin='download_completion'", song.ID)
		return err
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() != revision.size || info.ModTime().UnixNano() != revision.mtime {
		return nil
	}
	fingerprint := LocalSourceFingerprint(song, info)
	_, err = d.conn.ExecContext(ctx, `INSERT INTO track_external_identity
 (song_id,provider,external_id,link_origin,source_fingerprint,confirmed_at)
 SELECT ?,'spotify',?,'download_completion',?,? WHERE EXISTS (
 SELECT 1 FROM songs WHERE id=? AND file_path=? AND COALESCE(file_hash,'')=?) AND NOT EXISTS (
 SELECT 1 FROM track_external_identity_suppression WHERE song_id=? AND source_fingerprint=?)
 ON CONFLICT(song_id,provider) DO UPDATE SET external_id=excluded.external_id, source_fingerprint=excluded.source_fingerprint, confirmed_at=excluded.confirmed_at WHERE track_external_identity.link_origin='download_completion'`,
		song.ID, recording, fingerprint, time.Now().UnixMilli(), song.ID, path, song.FileHash, song.ID, fingerprint)
	return err
}

// RequeueConverting retires account-bound post-processing without restarting
// a completed artifact or changing another queue state.
func (d *DB) RequeueConverting(id string) (bool, error) {
	result, err := d.conn.Exec("UPDATE spotify_downloads SET status='queued',progress=0,error=NULL,started_at=NULL WHERE id=? AND status='converting'", id)
	if err != nil {
		return false, err
	}
	n, err := result.RowsAffected()
	return n == 1, err
}
