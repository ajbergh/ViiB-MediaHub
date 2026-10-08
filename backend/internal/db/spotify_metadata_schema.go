package db

// EnsureSpotifyMetadataSchema installs account-scoped domain storage atomically.
func (d *DB) EnsureSpotifyMetadataSchema() error {
	tx, err := d.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.Exec(`
 CREATE TABLE IF NOT EXISTS spotify_download_lineage_staging (
 download_id TEXT NOT NULL REFERENCES spotify_downloads(id) ON DELETE CASCADE,
 context_key TEXT NOT NULL, origin_kind TEXT NOT NULL, origin_id TEXT NOT NULL,
 origin_revision TEXT NOT NULL, position INTEGER NOT NULL,
 PRIMARY KEY(download_id,context_key,origin_kind,origin_id,origin_revision,position));
 CREATE TABLE IF NOT EXISTS spotify_download_lineage_imports (
 file_path TEXT NOT NULL, content_sha256 TEXT NOT NULL, file_size INTEGER NOT NULL, mtime_ns INTEGER NOT NULL,
 recording_id TEXT NOT NULL, origin_kind TEXT NOT NULL, origin_id TEXT NOT NULL,
 origin_revision TEXT NOT NULL, position INTEGER NOT NULL,
 PRIMARY KEY(file_path,content_sha256,file_size,mtime_ns,recording_id,origin_kind,origin_id,origin_revision,position));
 CREATE TABLE IF NOT EXISTS spotify_download_lineage_status (
 file_path TEXT NOT NULL, content_sha256 TEXT NOT NULL, file_size INTEGER NOT NULL, mtime_ns INTEGER NOT NULL,
 recording_id TEXT NOT NULL, state TEXT NOT NULL, checked_at INTEGER NOT NULL,
 PRIMARY KEY(file_path,content_sha256,file_size,mtime_ns,recording_id));
 CREATE TABLE IF NOT EXISTS spotify_download_catalog_import_status (
 file_path TEXT NOT NULL, content_sha256 TEXT NOT NULL, file_size INTEGER NOT NULL, mtime_ns INTEGER NOT NULL,
 recording_id TEXT NOT NULL, state TEXT NOT NULL CHECK(state IN ('available','incomplete','oversized','not_available')),
 reason TEXT NOT NULL, checked_at INTEGER NOT NULL,
 PRIMARY KEY(file_path,content_sha256,file_size,mtime_ns,recording_id));
 CREATE TABLE IF NOT EXISTS spotify_download_catalog_relations (
 file_path TEXT NOT NULL, content_sha256 TEXT NOT NULL, file_size INTEGER NOT NULL, mtime_ns INTEGER NOT NULL,
 recording_id TEXT NOT NULL, resource TEXT NOT NULL, relation_kind TEXT NOT NULL, position INTEGER NOT NULL,
 child_type TEXT NOT NULL, child_id TEXT NOT NULL, unavailable INTEGER NOT NULL,
 metadata_json BLOB NOT NULL CHECK(length(metadata_json)<=65536),
 PRIMARY KEY(file_path,content_sha256,file_size,mtime_ns,recording_id,resource,relation_kind,position));
 CREATE TABLE IF NOT EXISTS spotify_download_catalog_imports (
 file_path TEXT NOT NULL, content_sha256 TEXT NOT NULL, file_size INTEGER NOT NULL, mtime_ns INTEGER NOT NULL,
 recording_id TEXT NOT NULL, entity_type TEXT NOT NULL, spotify_id TEXT NOT NULL, resource TEXT NOT NULL,
 schema_version INTEGER NOT NULL, adapter_revision TEXT NOT NULL,
 payload BLOB NOT NULL CHECK(length(payload)<=2097152), payload_hash TEXT NOT NULL,
 retrieved_at INTEGER NOT NULL, expires_at INTEGER NOT NULL,
 PRIMARY KEY(file_path,content_sha256,file_size,mtime_ns,recording_id,entity_type,spotify_id,resource));
 CREATE TABLE IF NOT EXISTS spotify_download_import_status (
 file_path TEXT NOT NULL, content_sha256 TEXT NOT NULL, file_size INTEGER NOT NULL, mtime_ns INTEGER NOT NULL,
 spotify_id TEXT NOT NULL, state TEXT NOT NULL CHECK(state IN ('available','not_available','oversized')),
 checked_at INTEGER NOT NULL,
 PRIMARY KEY(file_path,content_sha256,file_size,mtime_ns,spotify_id));
 CREATE TABLE IF NOT EXISTS spotify_download_import_bindings (
 song_id TEXT NOT NULL REFERENCES songs(id) ON DELETE CASCADE,
 source_fingerprint TEXT NOT NULL, spotify_id TEXT NOT NULL,
 file_path TEXT NOT NULL, content_sha256 TEXT NOT NULL, file_size INTEGER NOT NULL, mtime_ns INTEGER NOT NULL,
 PRIMARY KEY(song_id,source_fingerprint));
 CREATE TABLE IF NOT EXISTS spotify_download_audio_imports (
 file_path TEXT NOT NULL, content_sha256 TEXT NOT NULL, file_size INTEGER NOT NULL, mtime_ns INTEGER NOT NULL,
 spotify_id TEXT NOT NULL, resource TEXT NOT NULL, artifact_kind TEXT NOT NULL,
 schema_version INTEGER NOT NULL, adapter_revision TEXT NOT NULL, encoding TEXT NOT NULL,
 payload BLOB NOT NULL CHECK(length(payload)<=8388608), payload_hash TEXT NOT NULL,
 decoded_size INTEGER NOT NULL CHECK(decoded_size<=8388608), retrieved_at INTEGER NOT NULL, expires_at INTEGER NOT NULL,
 PRIMARY KEY(file_path,content_sha256,file_size,mtime_ns,spotify_id,resource,artifact_kind));
 CREATE TABLE IF NOT EXISTS spotify_download_scalar_imports (
 file_path TEXT NOT NULL, content_sha256 TEXT NOT NULL, file_size INTEGER NOT NULL, mtime_ns INTEGER NOT NULL,
 spotify_id TEXT NOT NULL, resource TEXT NOT NULL, field_key TEXT NOT NULL, metric TEXT NOT NULL, units TEXT NOT NULL, value_json TEXT NOT NULL,
 confidence REAL, schema_version INTEGER NOT NULL, adapter_revision TEXT NOT NULL,
 retrieved_at INTEGER NOT NULL, expires_at INTEGER NOT NULL,
 PRIMARY KEY(file_path,content_sha256,file_size,mtime_ns,spotify_id,resource,field_key));
 CREATE TABLE IF NOT EXISTS spotify_download_field_attempt_imports (
 file_path TEXT NOT NULL, content_sha256 TEXT NOT NULL, file_size INTEGER NOT NULL, mtime_ns INTEGER NOT NULL,
 spotify_id TEXT NOT NULL, field_key TEXT NOT NULL, endpoint TEXT NOT NULL, state TEXT NOT NULL, reason TEXT NOT NULL,
 checked_at INTEGER NOT NULL, adapter_revision TEXT NOT NULL,
 PRIMARY KEY(file_path,content_sha256,file_size,mtime_ns,spotify_id,field_key,endpoint));
 CREATE TABLE IF NOT EXISTS spotify_metadata_owner (
 singleton INTEGER PRIMARY KEY CHECK(singleton=1), provider TEXT NOT NULL,
 account_id TEXT NOT NULL, context_key TEXT NOT NULL, verified_at INTEGER NOT NULL);
 CREATE TABLE IF NOT EXISTS spotify_playlist_traversals (
 context_key TEXT NOT NULL, spotify_id TEXT NOT NULL, generation INTEGER NOT NULL,
 complete INTEGER NOT NULL DEFAULT 0 CHECK(complete IN (0,1)),
 PRIMARY KEY(context_key,spotify_id));
 CREATE TABLE IF NOT EXISTS spotify_audio_artifacts (
 spotify_id TEXT NOT NULL, resource TEXT NOT NULL, artifact_kind TEXT NOT NULL, context_key TEXT NOT NULL,
 schema_version INTEGER NOT NULL, adapter_revision TEXT NOT NULL, encoding TEXT NOT NULL,
 payload BLOB NOT NULL CHECK(length(payload)<=8388608), payload_hash TEXT NOT NULL,
 decoded_size INTEGER NOT NULL CHECK(decoded_size<=8388608), retrieved_at INTEGER NOT NULL, expires_at INTEGER NOT NULL,
 PRIMARY KEY(spotify_id,resource,artifact_kind,context_key));
 CREATE TABLE IF NOT EXISTS spotify_entity_snapshots (
 entity_type TEXT NOT NULL, spotify_id TEXT NOT NULL, resource TEXT NOT NULL, context_key TEXT NOT NULL,
 schema_version INTEGER NOT NULL, adapter_revision TEXT NOT NULL, payload BLOB NOT NULL,
 payload_hash TEXT NOT NULL, retrieved_at INTEGER NOT NULL, expires_at INTEGER NOT NULL,
 PRIMARY KEY(entity_type,spotify_id,resource,context_key),
 CHECK(length(payload)<=2097152));
 CREATE TABLE IF NOT EXISTS spotify_entity_relations (
 entity_type TEXT NOT NULL, spotify_id TEXT NOT NULL, resource TEXT NOT NULL, context_key TEXT NOT NULL,
 relation_kind TEXT NOT NULL, position INTEGER NOT NULL, child_type TEXT NOT NULL, child_id TEXT NOT NULL,
 unavailable INTEGER NOT NULL CHECK(unavailable IN (0,1)), metadata_json BLOB NOT NULL,
 PRIMARY KEY(entity_type,spotify_id,resource,context_key,relation_kind,position),
 FOREIGN KEY(entity_type,spotify_id,resource,context_key)
 REFERENCES spotify_entity_snapshots(entity_type,spotify_id,resource,context_key) ON DELETE CASCADE);
 CREATE TABLE IF NOT EXISTS spotify_metadata_resource_status (
 entity_type TEXT NOT NULL, spotify_id TEXT NOT NULL, resource TEXT NOT NULL, context_key TEXT NOT NULL,
 state TEXT NOT NULL, reason TEXT NOT NULL, checked_at INTEGER NOT NULL, retry_at INTEGER NOT NULL,
 PRIMARY KEY(entity_type,spotify_id,resource,context_key));
 `)
	if err != nil {
		return err
	}
	for _, table := range []string{"spotify_download_scalar_imports", "spotify_download_field_attempt_imports"} {
		if _, err := tx.Exec("CREATE INDEX IF NOT EXISTS idx_" + table + "_recording ON " + table + "(spotify_id,file_path,field_key)"); err != nil {
			return err
		}
	}
	if _, err := tx.Exec("CREATE INDEX IF NOT EXISTS idx_spotify_audio_observations_score_context ON spotify_audio_observations(context_key,field_key,spotify_id,endpoint)"); err != nil {
		return err
	}
	rows, err := tx.Query("PRAGMA table_info(spotify_audio_artifacts)")
	if err != nil {
		return err
	}
	found := false
	for rows.Next() {
		var cid, notnull, pk int
		var name, typ string
		var defaultValue any
		if err = rows.Scan(&cid, &name, &typ, &notnull, &defaultValue, &pk); err != nil {
			rows.Close()
			return err
		}
		if name == "provider_etag" {
			found = true
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	if !found {
		if _, err = tx.Exec("ALTER TABLE spotify_audio_artifacts ADD COLUMN provider_etag TEXT NOT NULL DEFAULT ''"); err != nil {
			return err
		}
	}
	if err = migrateDownloadedCatalogParents(tx); err != nil {
		return err
	}
	if err = migrateDownloadedCatalogScope(tx); err != nil {
		return err
	}
	if err = migrateDownloadLineageEntity(tx); err != nil {
		return err
	}
	if err = migrateSpotifyCaptureRevision(tx); err != nil {
		return err
	}
	if err = migrateDownloadCollections(tx); err != nil {
		return err
	}
	if err = migrateDownloadSuppression(tx); err != nil {
		return err
	}
	if err = migrateDownloadRetention(tx); err != nil {
		return err
	}
	return tx.Commit()
}
