package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"time"
)

// GetDownloadedSpotifyAudioArtifact reads durable imports independently of account
// caches. Physical file identity is verified against final download evidence.
func (d *DB) GetDownloadedSpotifyAudioArtifact(ctx context.Context, songID, fingerprint, resource, kind string) (*SpotifyAudioArtifact, error) {
	if songID == "" || fingerprint == "" {
		return nil, errors.New("current source required")
	}
	var expected downloadFileRevision
	var fileHash string
	var value SpotifyAudioArtifact
	value.Resource = resource
	value.Kind = kind
	var encoding string
	var encoded []byte
	var size int
	var retrieved, expires int64
	err := d.conn.QueryRowContext(ctx, `SELECT a.spotify_id,a.schema_version,a.adapter_revision,a.encoding,a.payload,a.payload_hash,a.decoded_size,a.retrieved_at,a.expires_at,b.file_path,b.content_sha256,b.file_size,b.mtime_ns,COALESCE(s.file_hash,'')
 FROM spotify_download_import_bindings b JOIN spotify_download_audio_imports a
 ON a.file_path=b.file_path AND a.content_sha256=b.content_sha256 AND a.file_size=b.file_size AND a.mtime_ns=b.mtime_ns AND a.spotify_id=b.spotify_id
 JOIN songs s ON s.id=b.song_id AND s.file_path=b.file_path
 JOIN track_external_identity i ON i.song_id=b.song_id AND i.provider='spotify' AND i.external_id=b.spotify_id AND i.source_fingerprint=b.source_fingerprint
 WHERE b.song_id=? AND b.source_fingerprint=? AND a.resource=? AND a.artifact_kind=? AND a.schema_version=1
 AND NOT EXISTS (SELECT 1 FROM track_external_identity_suppression x WHERE x.song_id=b.song_id AND x.source_fingerprint=b.source_fingerprint)`, songID, fingerprint, resource, kind).Scan(&value.TrackID, &value.SchemaVersion, &value.AdapterRevision, &encoding, &encoded, &value.PayloadHash, &size, &retrieved, &expires, &expected.path, &expected.digest, &expected.size, &expected.mtime, &fileHash)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	actual, err := readDownloadRevision(ctx, expected.path)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, nil
	}
	if actual != expected {
		return nil, nil
	}
	info, err := os.Lstat(expected.path)
	if err != nil || !info.Mode().IsRegular() || info.Size() != expected.size || info.ModTime().UnixNano() != expected.mtime {
		return nil, nil
	}
	if LocalSourceFingerprint(Song{FilePath: expected.path, FileHash: fileHash}, info) != fingerprint {
		return nil, nil
	}
	var admitted int
	err = d.conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM songs s JOIN track_external_identity i ON i.song_id=s.id
 WHERE s.id=? AND s.file_path=? AND COALESCE(s.file_hash,'')=? AND i.provider='spotify' AND i.external_id=? AND i.source_fingerprint=?
 AND NOT EXISTS (SELECT 1 FROM track_external_identity_suppression x WHERE x.song_id=s.id AND x.source_fingerprint=?)`, songID, expected.path, fileHash, value.TrackID, fingerprint, fingerprint).Scan(&admitted)
	if err != nil {
		return nil, err
	}
	if admitted != 1 {
		return nil, nil
	}
	return decodeSpotifyAudioArtifact(value, encoding, encoded, size, retrieved, expires)
}

// GetDownloadedSpotifyScalarCandidates returns final-file-bound provider scalar
// facts and latest endpoint attempts, independent of the current Spotify account.
func (d *DB) GetDownloadedSpotifyScalarCandidates(ctx context.Context, songID, fingerprint string) ([]SpotifyScalarField, []SpotifyFieldAttempt, string, error) {
	if songID == "" || fingerprint == "" {
		return nil, nil, "", errors.New("current source required")
	}
	var expected downloadFileRevision
	var recording, fileHash string
	err := d.conn.QueryRowContext(ctx, `SELECT b.spotify_id,b.file_path,b.content_sha256,b.file_size,b.mtime_ns,COALESCE(s.file_hash,'')
 FROM spotify_download_import_bindings b JOIN songs s ON s.id=b.song_id AND s.file_path=b.file_path
 JOIN track_external_identity i ON i.song_id=b.song_id AND i.provider='spotify' AND i.external_id=b.spotify_id AND i.source_fingerprint=b.source_fingerprint
 WHERE b.song_id=? AND b.source_fingerprint=? AND NOT EXISTS (
 SELECT 1 FROM track_external_identity_suppression x WHERE x.song_id=b.song_id AND x.source_fingerprint=b.source_fingerprint)`, songID, fingerprint).
		Scan(&recording, &expected.path, &expected.digest, &expected.size, &expected.mtime, &fileHash)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil, "", nil
	}
	if err != nil {
		return nil, nil, "", err
	}
	actual, err := readDownloadRevision(ctx, expected.path)
	if err != nil {
		if ctx.Err() != nil {
			return nil, nil, "", ctx.Err()
		}
		return nil, nil, "", nil
	}
	if actual != expected {
		return nil, nil, "", nil
	}
	info, err := os.Lstat(expected.path)
	if err != nil || !info.Mode().IsRegular() || info.Size() != expected.size || info.ModTime().UnixNano() != expected.mtime || LocalSourceFingerprint(Song{FilePath: expected.path, FileHash: fileHash}, info) != fingerprint {
		return nil, nil, "", nil
	}
	var admitted int
	err = d.conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM songs s JOIN track_external_identity i ON i.song_id=s.id
 WHERE s.id=? AND s.file_path=? AND COALESCE(s.file_hash,'')=? AND i.provider='spotify' AND i.external_id=? AND i.source_fingerprint=?
 AND NOT EXISTS (SELECT 1 FROM track_external_identity_suppression x WHERE x.song_id=s.id AND x.source_fingerprint=?)`, songID, expected.path, fileHash, recording, fingerprint, fingerprint).Scan(&admitted)
	if err != nil {
		return nil, nil, "", err
	}
	if admitted != 1 {
		return nil, nil, "", nil
	}
	fields, err := d.queryDownloadedSpotifyScalarFields(ctx, expected, recording, time.Now())
	if err != nil {
		return nil, nil, "", err
	}
	attempts, err := d.queryDownloadedSpotifyFieldAttempts(ctx, expected, recording)
	if err != nil {
		return nil, nil, "", err
	}
	finalRevision, err := readDownloadRevision(ctx, expected.path)
	if err != nil {
		if ctx.Err() != nil {
			return nil, nil, "", ctx.Err()
		}
		return nil, nil, "", nil
	}
	if finalRevision != expected {
		return nil, nil, "", nil
	}
	var finalAdmitted int
	err = d.conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM songs s JOIN track_external_identity i ON i.song_id=s.id
 WHERE s.id=? AND s.file_path=? AND COALESCE(s.file_hash,'')=? AND i.provider='spotify' AND i.external_id=? AND i.source_fingerprint=?
 AND NOT EXISTS (SELECT 1 FROM track_external_identity_suppression x WHERE x.song_id=s.id AND x.source_fingerprint=?)`, songID, expected.path, fileHash, recording, fingerprint, fingerprint).Scan(&finalAdmitted)
	if err != nil {
		return nil, nil, "", err
	}
	if finalAdmitted != 1 {
		return nil, nil, "", nil
	}
	return fields, attempts, recording, nil
}

func (d *DB) queryDownloadedSpotifyScalarFields(ctx context.Context, revision downloadFileRevision, recording string, now time.Time) ([]SpotifyScalarField, error) {
	rows, err := d.conn.QueryContext(ctx, `SELECT field_key,metric,units,value_json,confidence,resource,schema_version,adapter_revision,retrieved_at,expires_at
	FROM spotify_download_scalar_imports WHERE file_path=? AND content_sha256=? AND file_size=? AND mtime_ns=? AND spotify_id=? ORDER BY field_key,resource`, revision.path, revision.digest, revision.size, revision.mtime, recording)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]SpotifyScalarField, 0)
	for rows.Next() {
		if len(result) >= 64 {
			return nil, errors.New("downloaded scalar field limit exceeded")
		}
		var field SpotifyScalarField
		var raw string
		var confidence sql.NullFloat64
		var retrieved, expires int64
		if err := rows.Scan(&field.Key, &field.Metric, &field.Units, &raw, &confidence, &field.Endpoint, &field.SchemaVersion, &field.AdapterRevision, &retrieved, &expires); err != nil {
			return nil, err
		}
		if !json.Valid([]byte(raw)) || raw == "null" {
			return nil, errors.New("invalid downloaded scalar value")
		}
		field.Value = json.RawMessage(raw)
		if confidence.Valid {
			value := confidence.Float64
			field.Confidence = &value
		}
		field.RetrievedAt = time.UnixMilli(retrieved).UTC()
		field.ExpiresAt = time.UnixMilli(expires).UTC()
		field.Stale = !now.Before(field.ExpiresAt)
		field.DurableImport = true
		result = append(result, field)
	}
	return result, rows.Err()
}

func (d *DB) queryDownloadedSpotifyFieldAttempts(ctx context.Context, revision downloadFileRevision, recording string) ([]SpotifyFieldAttempt, error) {
	rows, err := d.conn.QueryContext(ctx, `SELECT field_key,endpoint,state,reason,checked_at,adapter_revision
 FROM spotify_download_field_attempt_imports WHERE file_path=? AND content_sha256=? AND file_size=? AND mtime_ns=? AND spotify_id=? ORDER BY field_key,endpoint`, revision.path, revision.digest, revision.size, revision.mtime, recording)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]SpotifyFieldAttempt, 0)
	for rows.Next() {
		if len(result) >= 64 {
			return nil, errors.New("downloaded field attempt limit exceeded")
		}
		var attempt SpotifyFieldAttempt
		var checked int64
		if err := rows.Scan(&attempt.Key, &attempt.Endpoint, &attempt.State, &attempt.Reason, &checked, &attempt.AdapterRevision); err != nil {
			return nil, err
		}
		attempt.CheckedAt = time.UnixMilli(checked).UTC()
		attempt.DurableImport = true
		result = append(result, attempt)
	}
	return result, rows.Err()
}
