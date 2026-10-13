package db

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const SpotifyDownloadOrphanGrace = 30 * 24 * time.Hour

var downloadRevisionTables = []string{
	"spotify_download_evidence", "spotify_download_audio_imports", "spotify_download_scalar_imports",
	"spotify_download_field_attempt_imports", "spotify_download_import_status", "spotify_download_catalog_imports",
	"spotify_download_catalog_relations", "spotify_download_catalog_import_status", "spotify_download_collection_status",
	"spotify_download_lineage_imports", "spotify_download_lineage_status",
}

func migrateDownloadRetention(tx *sql.Tx) error {
	if _, err := tx.Exec(`CREATE TABLE IF NOT EXISTS spotify_download_revision_retention (
 file_path TEXT NOT NULL, content_sha256 TEXT NOT NULL, file_size INTEGER NOT NULL, mtime_ns INTEGER NOT NULL,
 first_seen_at INTEGER NOT NULL, last_completed_ns INTEGER NOT NULL DEFAULT 0, pending_scan INTEGER NOT NULL DEFAULT 1 CHECK(pending_scan IN (0,1)),
 orphaned_at INTEGER, last_checked_at INTEGER NOT NULL DEFAULT 0,
 PRIMARY KEY(file_path,content_sha256,file_size,mtime_ns));
 CREATE INDEX IF NOT EXISTS idx_download_retention_check ON spotify_download_revision_retention(last_checked_at);
 CREATE INDEX IF NOT EXISTS idx_download_binding_revision ON spotify_download_import_bindings(file_path,content_sha256,file_size,mtime_ns);`); err != nil {
		return err
	}
	rows, err := tx.Query("PRAGMA table_info(spotify_download_revision_retention)")
	if err != nil {
		return err
	}
	hasCompletion := false
	for rows.Next() {
		var cid, notnull, pk int
		var name, typ string
		var def any
		if err = rows.Scan(&cid, &name, &typ, &notnull, &def, &pk); err != nil {
			rows.Close()
			return err
		}
		if name == "last_completed_ns" {
			hasCompletion = true
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	if !hasCompletion {
		if _, err = tx.Exec("ALTER TABLE spotify_download_revision_retention ADD COLUMN last_completed_ns INTEGER NOT NULL DEFAULT 0"); err != nil {
			return err
		}
	}
	// Legacy bundles receive pending protection until an actual successful scan.
	selects := make([]string, 0, len(downloadRevisionTables))
	for _, table := range downloadRevisionTables {
		selects = append(selects, "SELECT file_path,content_sha256,file_size,mtime_ns FROM "+table)
	}
	if _, err := tx.Exec(`INSERT OR IGNORE INTO spotify_download_revision_retention(file_path,content_sha256,file_size,mtime_ns,first_seen_at) SELECT file_path,content_sha256,file_size,mtime_ns,? FROM (`+strings.Join(selects, " UNION ")+`)`, time.Now().UnixMilli()); err != nil {
		return err
	}
	now := `CAST(strftime('%s','now') AS INTEGER)*1000`
	// Triggers cover all explicit/scanner/Plex removals, including FK cascades,
	// and same-fingerprint binding replacement without duplicating delete APIs.
	for _, event := range []struct{ name, clause, condition string }{
		{"song_delete", "BEFORE DELETE ON songs", "1"},
		{"song_source", "BEFORE UPDATE OF file_path,file_hash ON songs", `OLD.file_path!=NEW.file_path OR COALESCE(OLD.file_hash,'')!=COALESCE(NEW.file_hash,'')`},
	} {
		_, err := tx.Exec(`CREATE TRIGGER IF NOT EXISTS download_retention_` + event.name + ` ` + event.clause + ` WHEN ` + event.condition + ` BEGIN
 UPDATE spotify_download_revision_retention SET pending_scan=0,orphaned_at=` + now + ` WHERE (file_path,content_sha256,file_size,mtime_ns) IN (
 SELECT file_path,content_sha256,file_size,mtime_ns FROM spotify_download_import_bindings WHERE song_id=OLD.id
 UNION SELECT file_path,content_sha256,file_size,mtime_ns FROM spotify_download_suppression_bindings WHERE song_id=OLD.id);
 END;`)
		if err != nil {
			return err
		}
	}
	for _, event := range []struct{ name, clause, condition string }{
		{"binding_delete", "AFTER DELETE ON spotify_download_import_bindings", "1"},
		{"binding_replace", "BEFORE UPDATE ON spotify_download_import_bindings", `OLD.file_path!=NEW.file_path OR OLD.content_sha256!=NEW.content_sha256 OR OLD.file_size!=NEW.file_size OR OLD.mtime_ns!=NEW.mtime_ns`},
	} {
		_, err := tx.Exec(`CREATE TRIGGER IF NOT EXISTS download_retention_` + event.name + ` ` + event.clause + ` WHEN ` + event.condition + ` BEGIN
 UPDATE spotify_download_revision_retention SET pending_scan=0,orphaned_at=` + now + ` WHERE file_path=OLD.file_path AND content_sha256=OLD.content_sha256 AND file_size=OLD.file_size AND mtime_ns=OLD.mtime_ns;
 END;`)
		if err != nil {
			return err
		}
	}
	return nil
}

func retainCompletedDownload(tx *sql.Tx, file downloadFileRevision, now int64) error {
	_, err := tx.Exec(`INSERT INTO spotify_download_revision_retention(file_path,content_sha256,file_size,mtime_ns,first_seen_at,last_completed_ns,pending_scan)
 VALUES(?,?,?,?,?,?,1) ON CONFLICT(file_path,content_sha256,file_size,mtime_ns) DO UPDATE SET pending_scan=1,orphaned_at=NULL,last_checked_at=0,last_completed_ns=excluded.last_completed_ns`, file.path, file.digest, file.size, file.mtime, now/int64(time.Millisecond), now)
	return err
}

type DownloadRetentionResult struct{ Checked, Owned, Orphaned, Collected int }

// MaintainSpotifyDownloadRetention examines only successful full-scan roots or
// individually verified incremental paths. It never runs from queue/account
// cleanup. Groups are round-robin bounded, context-cancelable, and collected
// atomically across all recording candidates after a 30-day unowned grace.
func (d *DB) MaintainSpotifyDownloadRetention(ctx context.Context, roots, paths []string, now time.Time, limit int) (DownloadRetentionResult, error) {
	result := DownloadRetentionResult{}
	if limit < 1 || limit > 128 || len(roots) > 128 || len(paths) > 512 || now.IsZero() {
		return result, errors.New("invalid download retention bounds")
	}
	scope := []string{}
	args := []any{}
	for _, root := range roots {
		absolute, err := filepath.Abs(root)
		if err != nil {
			return result, err
		}
		absolute = filepath.Clean(absolute)
		// Readability is rechecked even after a successful traversal.
		if !readableRetentionRoot(absolute) {
			continue
		}
		prefix := strings.TrimRight(absolute, string(filepath.Separator)) + string(filepath.Separator)
		escaped := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(prefix)
		scope = append(scope, `file_path LIKE ? ESCAPE '\'`)
		args = append(args, escaped+"%")
	}
	for _, path := range paths {
		absolute, err := filepath.Abs(path)
		if err != nil {
			return result, err
		}
		scope = append(scope, "file_path=?")
		args = append(args, filepath.Clean(absolute))
	}
	if len(scope) == 0 {
		return result, nil
	}
	var incrementalFolders []ScanFolder
	if len(paths) > 0 {
		var e error
		incrementalFolders, e = d.GetScanFolders()
		if e != nil {
			return result, e
		}
	}

	args = append(args, limit)
	rows, err := d.conn.QueryContext(ctx, `SELECT file_path,content_sha256,file_size,mtime_ns FROM spotify_download_revision_retention WHERE (`+strings.Join(scope, " OR ")+`) ORDER BY last_checked_at,file_path,content_sha256 LIMIT ?`, args...)
	if err != nil {
		return result, err
	}
	files := []downloadFileRevision{}
	for rows.Next() {
		var file downloadFileRevision
		if err = rows.Scan(&file.path, &file.digest, &file.size, &file.mtime); err != nil {
			rows.Close()
			return result, err
		}
		files = append(files, file)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return result, err
	}
	touchDeferred := func(file downloadFileRevision, count bool) error {
		_, e := d.conn.ExecContext(ctx, `UPDATE spotify_download_revision_retention SET last_checked_at=? WHERE file_path=? AND content_sha256=? AND file_size=? AND mtime_ns=?`, now.UnixMilli(), file.path, file.digest, file.size, file.mtime)
		if e == nil && count {
			result.Checked++
		}
		return e
	}
	for _, file := range files {
		if err = ctx.Err(); err != nil {
			return result, err
		}
		// For incremental absence, independently verify accessible ownership roots.
		info, statErr := os.Lstat(file.path)
		fullSafe := false
		for _, root := range roots {
			if retentionPathWithinRoot(root, file.path) {
				if readableRetentionRoot(root) {
					fullSafe = true
					break
				}
			}
		}
		exactSafe := false
		for _, path := range paths {
			absolute, e := filepath.Abs(path)
			if e == nil && (filepath.Clean(absolute) == file.path || (runtime.GOOS == "windows" && strings.EqualFold(filepath.Clean(absolute), file.path))) {
				exactSafe = true
				break
			}
		}
		if !fullSafe && !exactSafe {
			if err = touchDeferred(file, true); err != nil {
				return result, err
			}
			continue
		}
		if statErr != nil && (!os.IsNotExist(statErr) || (!fullSafe && !d.ConfirmMissingLocalMedia(file.path))) {
			if err = touchDeferred(file, true); err != nil {
				return result, err
			}
			continue
		}
		var actual downloadFileRevision
		if statErr == nil {
			if !info.Mode().IsRegular() {
				if err = touchDeferred(file, true); err != nil {
					return result, err
				}
				continue
			}
			actual, err = readDownloadRevision(ctx, file.path)
			if err != nil {
				if ctx.Err() != nil {
					return result, ctx.Err()
				}
				if err = touchDeferred(file, true); err != nil {
					return result, err
				}
				continue
			}
		}
		tx, err := d.conn.BeginTx(ctx, nil)
		if err != nil {
			return result, err
		}
		processErr := func() error {
			defer tx.Rollback()
			revisionArgs := []any{file.path, file.digest, file.size, file.mtime}
			// Acquire writer ownership before querying; completion/rebind cannot race
			// a stale read snapshot into removing newly retained material.
			updateArgs := append([]any{now.UnixMilli()}, revisionArgs...)
			changed, e := tx.ExecContext(ctx, `UPDATE spotify_download_revision_retention SET last_checked_at=? WHERE file_path=? AND content_sha256=? AND file_size=? AND mtime_ns=?`, updateArgs...)
			if e != nil {
				return e
			}
			n, e := changed.RowsAffected()
			if e != nil || n == 0 {
				return e
			}
			result.Checked++
			var completedNS int64
			if e = tx.QueryRowContext(ctx, `SELECT last_completed_ns FROM spotify_download_revision_retention WHERE file_path=? AND content_sha256=? AND file_size=? AND mtime_ns=?`, revisionArgs...).Scan(&completedNS); e != nil {
				return e
			}
			// Coverage predates this completion/retry. Preserve pending state
			// until a later scan can actually establish its ownership.
			if completedNS >= now.UnixNano() {
				return tx.Commit()
			}
			var owned bool
			if statErr == nil && actual == file {
				if e = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM songs WHERE file_path=? OR (? AND file_path=? COLLATE NOCASE))`, file.path, runtime.GOOS == "windows", file.path).Scan(&owned); e != nil {
					return e
				}
			}
			if owned {
				if _, e = tx.ExecContext(ctx, `UPDATE spotify_download_revision_retention SET pending_scan=0,orphaned_at=NULL WHERE file_path=? AND content_sha256=? AND file_size=? AND mtime_ns=?`, revisionArgs...); e != nil {
					return e
				}
				result.Owned++
				return tx.Commit()
			}
			var pending bool
			var orphan sql.NullInt64
			if e = tx.QueryRowContext(ctx, `SELECT pending_scan,orphaned_at FROM spotify_download_revision_retention WHERE file_path=? AND content_sha256=? AND file_size=? AND mtime_ns=?`, revisionArgs...).Scan(&pending, &orphan); e != nil {
				return e
			}
			if pending || !orphan.Valid {
				if _, e = tx.ExecContext(ctx, `UPDATE spotify_download_revision_retention SET pending_scan=0,orphaned_at=? WHERE file_path=? AND content_sha256=? AND file_size=? AND mtime_ns=?`, append([]any{now.UnixMilli()}, revisionArgs...)...); e != nil {
					return e
				}
				result.Orphaned++
				return tx.Commit()
			}
			if orphan.Int64 > now.Add(-SpotifyDownloadOrphanGrace).UnixMilli() {
				return tx.Commit()
			}
			if statErr != nil {
				stillCovered := false
				for _, root := range roots {
					if readableRetentionRoot(root) && retentionPathWithinRoot(root, file.path) {
						stillCovered = true
						break
					}
				}
				if !stillCovered && (!exactSafe || !confirmMissingLocalMedia(file.path, incrementalFolders, os.Stat, os.ReadDir)) {
					return tx.Commit()
				}
			}
			// Recheck physical state inside the deletion transaction. Access failure
			// or a changed observation defers collection without discarding history.
			if statErr == nil {
				latest, e := readDownloadRevision(ctx, file.path)
				if e != nil {
					if ctx.Err() != nil {
						return ctx.Err()
					}
					return tx.Commit() // Defer this source race; continue other groups.
				}
				if latest != actual {
					return tx.Commit()
				}
			} else if _, e := os.Lstat(file.path); !os.IsNotExist(e) {
				return tx.Commit()
			}
			if e = retireCollectedDownloadScalars(tx, file); e != nil {
				return e
			}
			// Retire only automatic identities associated with the obsolete binding.
			if _, e = tx.ExecContext(ctx, `DELETE FROM track_external_identity WHERE provider='spotify' AND link_origin='download_completion' AND EXISTS(SELECT 1 FROM spotify_download_import_bindings b WHERE b.song_id=track_external_identity.song_id AND b.source_fingerprint=track_external_identity.source_fingerprint AND b.file_path=? AND b.content_sha256=? AND b.file_size=? AND b.mtime_ns=?)`, revisionArgs...); e != nil {
				return e
			}
			for _, table := range append(append([]string{}, downloadRevisionTables...), "spotify_download_import_bindings") {
				if _, e = tx.ExecContext(ctx, "DELETE FROM "+table+" WHERE file_path=? AND content_sha256=? AND file_size=? AND mtime_ns=?", revisionArgs...); e != nil {
					return fmt.Errorf("collect %s: %w", table, e)
				}
			}
			// Suppression projections are small source-bound state; exact tombstones
			// remain independent and continue gating automatic search after collection.
			if _, e = tx.ExecContext(ctx, `DELETE FROM spotify_download_revision_retention WHERE file_path=? AND content_sha256=? AND file_size=? AND mtime_ns=?`, revisionArgs...); e != nil {
				return e
			}
			if e = tx.Commit(); e != nil {
				return e
			}
			result.Collected++
			return nil
		}()
		if errors.Is(processErr, errDownloadRetentionBusy) {
			if err = touchDeferred(file, false); err != nil {
				return result, err
			}
			continue
		}
		if processErr != nil {
			return result, processErr
		}
	}
	return result, nil
}

// downloadRevisionIdentity qualifies materialized scalars when the ordinary
// source fingerprint collides across same-size/mtime byte replacements.
func downloadRevisionIdentity(file downloadFileRevision) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(fmt.Sprintf("%q:%s:%d:%d", file.path, file.digest, file.size, file.mtime))))
}

var errDownloadRetentionBusy = errors.New("download scalar projection owned by preparation")

func retireCollectedDownloadScalars(tx *sql.Tx, file downloadFileRevision) error {
	var hasAnalysis bool
	if err := tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM sqlite_master WHERE type='table' AND name='track_analysis')`).Scan(&hasAnalysis); err != nil {
		return err
	}
	if !hasAnalysis {
		return nil
	}

	rows, err := tx.Query(`SELECT song_id,source_fingerprint,spotify_id FROM spotify_download_import_bindings WHERE file_path=? AND content_sha256=? AND file_size=? AND mtime_ns=?
 UNION SELECT song_id,source_fingerprint,'' FROM track_analysis WHERE json_extract(CASE WHEN json_valid(spotify_bindings_json) THEN spotify_bindings_json ELSE '{}' END,'$.bpm.downloadRevision')=? OR json_extract(CASE WHEN json_valid(spotify_bindings_json) THEN spotify_bindings_json ELSE '{}' END,'$.key.downloadRevision')=?`, file.path, file.digest, file.size, file.mtime, downloadRevisionIdentity(file), downloadRevisionIdentity(file))
	if err != nil {
		return err
	}
	type bound struct{ song, fingerprint, recording string }
	bindings := []bound{}
	for rows.Next() {
		var b bound
		if err = rows.Scan(&b.song, &b.fingerprint, &b.recording); err != nil {
			rows.Close()
			return err
		}
		bindings = append(bindings, b)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, b := range bindings {
		record, err := getTrackAnalysis(tx, b.song)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return err
		}
		if record.SpotifyBindings == nil {
			continue
		}
		matches := func(v *SpotifyScalarBinding) bool {
			return v != nil && v.Durable && (v.DownloadRevision == downloadRevisionIdentity(file) || (v.DownloadRevision == "" && v.TrackID == b.recording && v.SourceFingerprint == b.fingerprint))
		}
		bpm, key := matches(record.SpotifyBindings.BPM), matches(record.SpotifyBindings.Key)
		if !bpm && !key {
			continue
		}
		if record.Status == TrackAnalysisRunning || record.Status == TrackAnalysisPending {
			return errDownloadRetentionBusy
		}
		local := CurrentLocalScalars(&record)
		if bpm {
			record.SpotifyBindings.BPM = nil
			if record.BPMSource != nil && *record.BPMSource == "spotify" {
				record.BPM = nil
				record.BPMSource = nil
				record.BPMConfidence = nil
				record.BPMAltCandidate = nil
				record.TempoStability = nil
				record.TempoKind = nil
				if local != nil && local.BPM != nil {
					record.BPM = local.BPM
					record.BPMSource = scalarPtr("measured")
					record.BPMConfidence = local.BPMConfidence
					record.BPMAltCandidate = local.BPMAltCandidate
					record.TempoStability = local.TempoStability
					record.TempoKind = local.TempoKind
				}
			}
		}
		if key {
			record.SpotifyBindings.Key = nil
			if record.KeySource != nil && *record.KeySource == "spotify" {
				record.KeyTonic = nil
				record.KeyMode = nil
				record.KeyConfidence = nil
				record.KeySource = nil
				record.CamelotKey = nil
				record.OpenKey = nil
				if local != nil && local.KeyTonic != nil {
					record.KeyTonic = local.KeyTonic
					record.KeyMode = local.KeyMode
					record.KeyConfidence = local.KeyConfidence
					record.KeySource = scalarPtr("measured")
				}
			}
		}
		if record.SpotifyBindings.BPM == nil && record.SpotifyBindings.Key == nil {
			record.SpotifyBindings = nil
		}
		if err = upsertTrackAnalysis(tx, record); err != nil {
			return err
		}
	}
	return nil
}

// A successful lexical root must also contain the resolved existing ancestor;
// symlinked descendants outside the traversed root are not coverage proof.
func retentionPathWithinRoot(root, path string) bool {
	if !PathWithinRoot(root, path) {
		return false
	}
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return false
	}
	ancestor := path
	for {
		resolved, err := filepath.EvalSymlinks(ancestor)
		if err == nil {
			return PathWithinRoot(resolvedRoot, resolved)
		}
		if !os.IsNotExist(err) {
			return false
		}
		parent := filepath.Dir(ancestor)
		if parent == ancestor {
			return false
		}
		ancestor = parent
	}
}

func readableRetentionRoot(root string) bool {
	directory, err := os.Open(root)
	if err != nil {
		return false
	}
	defer directory.Close()
	_, err = directory.ReadDir(1)
	return err == nil || errors.Is(err, io.EOF)
}
