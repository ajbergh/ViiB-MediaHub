// Adds external recording identity, reference cache, and refresh-status schema without replacing local analysis.
package db

import "strings"

// EnsureExternalTrackAnalysisSchema installs inert, additive reference storage.
// No credential material or raw provider responses belong in these tables.
func (d *DB) EnsureExternalTrackAnalysisSchema() error {
	d.externalSchemaMu.Lock()
	defer d.externalSchemaMu.Unlock()
	if d.externalSchemaReady {
		return nil
	}
	_, err := d.conn.Exec(`
 CREATE TABLE IF NOT EXISTS track_external_identity (
 song_id TEXT NOT NULL REFERENCES songs(id) ON DELETE CASCADE,
 provider TEXT NOT NULL CHECK(provider='spotify'),
 external_id TEXT NOT NULL,
 link_origin TEXT NOT NULL CHECK(link_origin IN ('manual_confirmation','download_completion')),
 source_fingerprint TEXT NOT NULL,
 confirmed_at INTEGER NOT NULL,
 PRIMARY KEY(song_id,provider));
 CREATE TABLE IF NOT EXISTS external_track_analysis (
 provider TEXT NOT NULL CHECK(provider='spotify'),
 external_id TEXT NOT NULL,
 endpoint TEXT NOT NULL CHECK(endpoint IN ('audio_analysis','audio_features')),
 schema_version INTEGER NOT NULL,
 observation_json TEXT NOT NULL CHECK(length(observation_json)<=16384),
 observation_hash TEXT NOT NULL,
 adapter_revision TEXT NOT NULL,
 retrieved_at INTEGER NOT NULL,
 expires_at INTEGER NOT NULL,
 PRIMARY KEY(provider,external_id,endpoint,schema_version));
 CREATE TABLE IF NOT EXISTS external_track_analysis_status (
 provider TEXT NOT NULL CHECK(provider='spotify'),
 external_id TEXT NOT NULL,
 endpoint TEXT NOT NULL CHECK(endpoint IN ('audio_analysis','audio_features')),
 schema_version INTEGER NOT NULL,
 code TEXT NOT NULL,
 checked_at INTEGER NOT NULL,
 retry_at INTEGER NOT NULL,
 PRIMARY KEY(provider,external_id,endpoint,schema_version));
 CREATE INDEX IF NOT EXISTS idx_external_analysis_cooldown ON external_track_analysis_status(provider,code,schema_version,retry_at);
 `)
	if err != nil {
		return err
	}
	var definition string
	if err = d.conn.QueryRow("SELECT sql FROM sqlite_master WHERE name='track_external_identity'").Scan(&definition); err != nil {
		return err
	}
	if !strings.Contains(definition, "download_completion") {
		tx, e := d.conn.Begin()
		if e != nil {
			return e
		}
		defer tx.Rollback()
		_, e = tx.Exec(`CREATE TABLE track_external_identity_next (
   song_id TEXT NOT NULL REFERENCES songs(id) ON DELETE CASCADE,
   provider TEXT NOT NULL CHECK(provider='spotify'), external_id TEXT NOT NULL,
   link_origin TEXT NOT NULL CHECK(link_origin IN ('manual_confirmation','download_completion')),
   source_fingerprint TEXT NOT NULL, confirmed_at INTEGER NOT NULL, PRIMARY KEY(song_id,provider));
   INSERT INTO track_external_identity_next SELECT * FROM track_external_identity;
   DROP TABLE track_external_identity;
   ALTER TABLE track_external_identity_next RENAME TO track_external_identity;`)
		if e != nil {
			return e
		}
		if e = tx.Commit(); e != nil {
			return e
		}
	}
	_, err = d.conn.Exec(`CREATE TABLE IF NOT EXISTS spotify_download_evidence (
  file_path TEXT NOT NULL, content_sha256 TEXT NOT NULL, file_size INTEGER NOT NULL,
  mtime_ns INTEGER NOT NULL, spotify_id TEXT NOT NULL, completed_at INTEGER NOT NULL,
  PRIMARY KEY(file_path,content_sha256,file_size,mtime_ns,spotify_id));
 CREATE TABLE IF NOT EXISTS track_external_identity_suppression (
  song_id TEXT NOT NULL REFERENCES songs(id) ON DELETE CASCADE,
  source_fingerprint TEXT NOT NULL, PRIMARY KEY(song_id,source_fingerprint));`)
	if err == nil {
		d.externalSchemaReady = true
	}
	return err
}
