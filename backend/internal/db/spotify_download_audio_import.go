package db

import (
	"context"
	"database/sql"
	"errors"
	"os"
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
