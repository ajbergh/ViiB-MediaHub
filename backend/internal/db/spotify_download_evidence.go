// Records and reconciles completed Spotify download evidence against validated local file identity.
package db

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"time"

	spotifyanalysis "github.com/ajbergh/viib-mediahub/internal/spotify/analysis"
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
	// Acquire the write lock before creating a read snapshot. Concurrent
	// completions otherwise race to upgrade stale WAL snapshots to writers.
	err = tx.QueryRowContext(ctx, `UPDATE spotify_downloads SET status='completed',progress=100,file_path=?,completed_at=?
 WHERE id=? AND status IN ('downloading','converting') RETURNING spotify_id`, revision.path, time.Now().Unix(), id).Scan(&recording)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !ValidSpotifyRecordingID(recording) {
		return false, errors.New("invalid recording ID in completed download")
	}
	_, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO spotify_download_evidence
 (file_path,content_sha256,file_size,mtime_ns,spotify_id,completed_at,features_json)
 VALUES (?,?,?,?,?,?,COALESCE((SELECT observation_json FROM external_track_analysis
 WHERE provider='spotify' AND external_id=? AND endpoint='audio_features' AND schema_version=? AND expires_at>? AND (COALESCE(account_context,'')<>'' AND account_context=(SELECT value FROM settings WHERE key='spotify_metadata_active_context'))),''))`,
		revision.path, revision.digest, revision.size, revision.mtime, recording, time.Now().UnixMilli(), recording, ExternalAnalysisSchemaVersion, time.Now().UnixMilli())
	if err != nil {
		return false, err
	}
	// Promote only this recording's fresh active-account artifacts. These
	// final-file imports are independent of private context cache retirement.
	_, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO spotify_download_audio_imports
 (file_path,content_sha256,file_size,mtime_ns,spotify_id,resource,artifact_kind,
 schema_version,adapter_revision,encoding,payload,payload_hash,decoded_size,retrieved_at,expires_at)
 SELECT ?,?,?,?,spotify_id,resource,artifact_kind,schema_version,adapter_revision,encoding,
 payload,payload_hash,decoded_size,retrieved_at,expires_at FROM spotify_audio_artifacts
 WHERE spotify_id=? AND schema_version=1 AND expires_at>? AND context_key<>''
 AND context_key=(SELECT value FROM settings WHERE key='spotify_metadata_active_context')
 AND resource IN ('audio_features','audio_analysis','three_band_waveform')
 AND (SELECT COALESCE(SUM(length(payload)),0) FROM spotify_audio_artifacts
 WHERE spotify_id=? AND context_key=(SELECT value FROM settings WHERE key='spotify_metadata_active_context'))<=33554432`,
		revision.path, revision.digest, revision.size, revision.mtime, recording, time.Now().UnixMilli(), recording)
	if err != nil {
		return false, err
	}
	_, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO spotify_download_scalar_imports
 (file_path,content_sha256,file_size,mtime_ns,spotify_id,resource,field_key,metric,units,value_json,confidence,schema_version,adapter_revision,retrieved_at,expires_at)
 SELECT ?,?,?,?,spotify_id,endpoint,field_key,metric,units,value_json,confidence,schema_version,adapter_revision,retrieved_at,expires_at
 FROM spotify_audio_observations WHERE spotify_id=? AND schema_version=1 AND context_key=(SELECT value FROM settings WHERE key='spotify_metadata_active_context')
	AND (SELECT COALESCE(SUM(length(value_json)),0) FROM spotify_audio_observations
 WHERE spotify_id=? AND context_key=(SELECT value FROM settings WHERE key='spotify_metadata_active_context'))<=65536`,
		revision.path, revision.digest, revision.size, revision.mtime, recording, recording)
	if err != nil {
		return false, err
	}
	_, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO spotify_download_field_attempt_imports
 (file_path,content_sha256,file_size,mtime_ns,spotify_id,field_key,endpoint,state,reason,checked_at,adapter_revision)
 SELECT ?,?,?,?,spotify_id,field_key,endpoint,state,reason,checked_at,adapter_revision
 FROM spotify_audio_field_attempts WHERE spotify_id=? AND context_key=(SELECT value FROM settings WHERE key='spotify_metadata_active_context')
 AND (SELECT COUNT(*) FROM spotify_audio_field_attempts WHERE spotify_id=? AND context_key=(SELECT value FROM settings WHERE key='spotify_metadata_active_context'))<=128`,
		revision.path, revision.digest, revision.size, revision.mtime, recording, recording)
	if err != nil {
		return false, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO spotify_download_import_status
 (file_path,content_sha256,file_size,mtime_ns,spotify_id,state,checked_at)
 VALUES(?,?,?,?,?,CASE
 WHEN (SELECT COALESCE(SUM(length(payload)),0) FROM spotify_audio_artifacts WHERE spotify_id=? AND context_key=(SELECT value FROM settings WHERE key='spotify_metadata_active_context'))>33554432
  AND NOT EXISTS(SELECT 1 FROM spotify_download_scalar_imports WHERE file_path=? AND content_sha256=? AND file_size=? AND mtime_ns=? AND spotify_id=?)
  AND NOT EXISTS(SELECT 1 FROM spotify_download_field_attempt_imports WHERE file_path=? AND content_sha256=? AND file_size=? AND mtime_ns=? AND spotify_id=?) THEN 'oversized'
 WHEN EXISTS(SELECT 1 FROM spotify_download_audio_imports WHERE file_path=? AND content_sha256=? AND file_size=? AND mtime_ns=? AND spotify_id=?)
  OR EXISTS(SELECT 1 FROM spotify_download_scalar_imports WHERE file_path=? AND content_sha256=? AND file_size=? AND mtime_ns=? AND spotify_id=?)
  OR EXISTS(SELECT 1 FROM spotify_download_field_attempt_imports WHERE file_path=? AND content_sha256=? AND file_size=? AND mtime_ns=? AND spotify_id=?) THEN 'available'
 ELSE 'not_available' END,?)
 ON CONFLICT(file_path,content_sha256,file_size,mtime_ns,spotify_id) DO UPDATE SET state=excluded.state,checked_at=excluded.checked_at`,
		revision.path, revision.digest, revision.size, revision.mtime, recording, recording,
		revision.path, revision.digest, revision.size, revision.mtime, recording,
		revision.path, revision.digest, revision.size, revision.mtime, recording,
		revision.path, revision.digest, revision.size, revision.mtime, recording,
		revision.path, revision.digest, revision.size, revision.mtime, recording,
		revision.path, revision.digest, revision.size, revision.mtime, recording, time.Now().UnixMilli())
	if err != nil {
		return false, err
	}
	catalogNow := time.Now().UnixMilli()
	_, err = tx.ExecContext(ctx, downloadedCatalogGraphCTE+`, retained AS (SELECT * FROM spotify_download_catalog_imports WHERE file_path=? AND content_sha256=? AND file_size=? AND mtime_ns=? AND recording_id=?),
 new_material AS (SELECT e.* FROM eligible e WHERE NOT EXISTS(SELECT 1 FROM retained r WHERE r.entity_type=e.entity_type AND r.spotify_id=e.spotify_id AND r.resource=e.resource))
 INSERT OR IGNORE INTO spotify_download_catalog_imports
 (file_path,content_sha256,file_size,mtime_ns,recording_id,entity_type,spotify_id,resource,schema_version,adapter_revision,payload,payload_hash,retrieved_at,expires_at)
 SELECT ?,?,?,?,?,entity_type,spotify_id,resource,schema_version,adapter_revision,payload,payload_hash,retrieved_at,expires_at FROM eligible
 WHERE (SELECT COALESCE(SUM(length(payload)),0) FROM retained)+(SELECT COALESCE(SUM(length(payload)),0) FROM new_material)<=8388608
 AND (SELECT COUNT(*) FROM retained)+(SELECT COUNT(*) FROM new_material)<=256`,
		catalogNow, recording, revision.path, revision.digest, revision.size, revision.mtime, recording, revision.path, revision.digest, revision.size, revision.mtime, recording)
	if err != nil {
		return false, err
	}
	_, err = tx.ExecContext(ctx, downloadedCatalogGraphCTE+`, eligible_relations AS (
 SELECT r.* FROM relations r WHERE EXISTS(SELECT 1 FROM spotify_download_catalog_imports s
 WHERE s.entity_type=r.entity_type AND s.spotify_id=r.spotify_id AND s.resource=r.resource
 AND s.file_path=? AND s.content_sha256=? AND s.file_size=? AND s.mtime_ns=? AND s.recording_id=?))
, retained AS (SELECT * FROM spotify_download_catalog_relations WHERE file_path=? AND content_sha256=? AND file_size=? AND mtime_ns=? AND recording_id=?),
 new_material AS (SELECT e.* FROM eligible_relations e WHERE NOT EXISTS(SELECT 1 FROM retained r WHERE r.parent_type=e.entity_type AND r.parent_id=e.spotify_id AND r.resource=e.resource AND r.relation_kind=e.relation_kind AND r.position=e.position))
 INSERT OR IGNORE INTO spotify_download_catalog_relations
 (file_path,content_sha256,file_size,mtime_ns,recording_id,parent_type,parent_id,resource,relation_kind,position,child_type,child_id,unavailable,metadata_json)
 SELECT ?,?,?,?,?,entity_type,spotify_id,resource,relation_kind,position,child_type,child_id,unavailable,metadata_json FROM eligible_relations
 WHERE (SELECT COUNT(*) FROM retained)+(SELECT COUNT(*) FROM new_material)<=20000
 AND (SELECT COALESCE(SUM(length(metadata_json)),0) FROM retained)+(SELECT COALESCE(SUM(length(metadata_json)),0) FROM new_material)<=2097152`,
		catalogNow, recording, revision.path, revision.digest, revision.size, revision.mtime, recording,
		revision.path, revision.digest, revision.size, revision.mtime, recording,
		revision.path, revision.digest, revision.size, revision.mtime, recording)
	if err != nil {
		return false, err
	}
	if err = recordDownloadedCatalogOutcome(ctx, tx, revision, recording, catalogNow); err != nil {
		return false, err
	}
	if err = promoteDownloadCollections(ctx, tx, id, revision, recording, catalogNow); err != nil {
		return false, err
	}
	if err = promoteDownloadLineage(ctx, tx, id, revision, recording, catalogNow); err != nil {
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
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() != revision.size || info.ModTime().UnixNano() != revision.mtime {
		return nil
	}
	fingerprint := LocalSourceFingerprint(song, info)
	suppressionRecording := recording
	if distinct != 1 {
		suppressionRecording = ""
	}
	if suppressed, err := d.restoreDownloadSuppression(ctx, song, fingerprint, suppressionRecording, revision); err != nil || suppressed {
		return err
	}
	if distinct != 1 || !ValidSpotifyRecordingID(recording) {
		// New conflicting evidence must also retire a prior automatic identity.
		// Preserve manual confirmation even when automatic evidence becomes invalid.
		_, err = d.conn.ExecContext(ctx, "DELETE FROM track_external_identity WHERE song_id=? AND provider='spotify' AND link_origin='download_completion'", song.ID)
		return err
	}
	publication, err := d.conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer publication.Rollback()
	_, err = publication.ExecContext(ctx, `INSERT INTO track_external_identity
 (song_id,provider,external_id,link_origin,source_fingerprint,confirmed_at)
 SELECT ?,'spotify',?,'download_completion',?,? WHERE EXISTS (
 SELECT 1 FROM songs WHERE id=? AND file_path=? AND COALESCE(file_hash,'')=?) AND NOT EXISTS (
 SELECT 1 FROM track_external_identity_suppression WHERE song_id=? AND source_fingerprint=?)
 ON CONFLICT(song_id,provider) DO UPDATE SET external_id=excluded.external_id, source_fingerprint=excluded.source_fingerprint, confirmed_at=excluded.confirmed_at WHERE track_external_identity.link_origin IN ('download_completion','automatic_search')`,
		song.ID, recording, fingerprint, time.Now().UnixMilli(), song.ID, path, song.FileHash, song.ID, fingerprint)
	if err != nil {
		return err
	}
	// Bind the retained bundle even when no usable BPM/key was returned.
	// Recheck canonical identity and suppression in the publication statement.
	bindingResult, err := publication.ExecContext(ctx, `INSERT INTO spotify_download_import_bindings
 (song_id,source_fingerprint,spotify_id,file_path,content_sha256,file_size,mtime_ns)
 SELECT ?,?,?,?,?,?,? WHERE EXISTS (
 SELECT 1 FROM songs s JOIN track_external_identity i ON i.song_id=s.id
 WHERE s.id=? AND s.file_path=? AND COALESCE(s.file_hash,'')=?
 AND i.provider='spotify' AND i.external_id=? AND i.source_fingerprint=?)
 AND NOT EXISTS (SELECT 1 FROM track_external_identity_suppression WHERE song_id=? AND source_fingerprint=?)
 ON CONFLICT(song_id,source_fingerprint) DO UPDATE SET spotify_id=excluded.spotify_id,
 file_path=excluded.file_path,content_sha256=excluded.content_sha256,file_size=excluded.file_size,mtime_ns=excluded.mtime_ns`,
		song.ID, fingerprint, recording, absolute, revision.digest, revision.size, revision.mtime,
		song.ID, path, song.FileHash, recording, fingerprint, song.ID, fingerprint)
	if err != nil {
		return err
	}
	bindingCount, err := bindingResult.RowsAffected()
	if err != nil {
		return err
	}
	if err = publication.Commit(); err != nil {
		return err
	}
	// Stronger manual links to another recording must not admit these scalars.
	if bindingCount == 0 {
		return nil
	}
	var encoded string
	err = d.conn.QueryRowContext(ctx, `SELECT features_json FROM spotify_download_evidence
 WHERE file_path=? AND content_sha256=? AND file_size=? AND mtime_ns=? AND spotify_id=?`,
		absolute, revision.digest, revision.size, revision.mtime, recording).Scan(&encoded)
	if err != nil {
		return err
	}
	if encoded == "" {
		return nil
	}
	var observation spotifyanalysis.Observation
	if json.Unmarshal([]byte(encoded), &observation) != nil || observation.TrackID != recording || spotifyanalysis.ValidateObservation(observation) != nil {
		return nil
	}
	if err := d.EnsureTrackAnalysisSchema(); err != nil {
		return err
	}
	tx, err := d.conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	existing, err := getTrackAnalysis(tx, song.ID)
	if err == nil && (existing.Status == TrackAnalysisRunning || existing.Status == TrackAnalysisPending) {
		return nil
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	record := TrackAnalysis{SongID: song.ID, Status: TrackAnalysisPartial, AnalysisVersion: 1,
		AlgorithmVersion: "spotify-features-v1", SourceFingerprint: fingerprint,
		SourceSize: &revision.size, SourceMtime: scalarPtr(info.ModTime().UnixMilli()), AnalyzedAt: scalarPtr(time.Now().UnixMilli())}
	sameSource := err == nil && existing.SourceFingerprint == fingerprint
	if sameSource {
		record = existing
		durable := func(b *SpotifyScalarBinding) bool {
			return b != nil && b.Durable && b.TrackID == recording && b.SourceFingerprint == fingerprint
		}
		if record.SpotifyBindings != nil && (observation.BPM == nil || (record.BPM != nil && record.BPMSource != nil && *record.BPMSource == "spotify" && durable(record.SpotifyBindings.BPM))) && (observation.Key == nil || observation.Mode == nil || (record.KeyTonic != nil && record.KeyMode != nil && record.KeySource != nil && *record.KeySource == "spotify" && durable(record.SpotifyBindings.Key))) {
			return nil
		}
	}
	observation.DurableImport = true
	ApplySpotifyScalars(&record, observation)
	if record.BPM == nil && record.KeyTonic == nil {
		return nil
	}
	if !sameSource && record.BPM != nil && record.KeyTonic != nil {
		record.Status = TrackAnalysisComplete
	}
	// New rows retain the download marker for local preparation; existing rows
	// retain their local completion identity. The transaction keeps the claim
	// check and merged publication in the same SQLite snapshot.
	if err := upsertTrackAnalysis(tx, record); err != nil {
		return err
	}
	return tx.Commit()
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
