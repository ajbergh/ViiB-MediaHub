package db

import "database/sql"

// Preserve legacy direct-track imports while widening relation identity to
// include the parent. This runs inside the schema installation transaction.
func migrateDownloadedCatalogParents(tx *sql.Tx) error {
	rows, err := tx.Query("PRAGMA table_info(spotify_download_catalog_relations)")
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
		if name == "parent_type" {
			found = true
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	if found {
		return nil
	}
	_, err = tx.Exec(`CREATE TABLE spotify_download_catalog_relations_next (
 file_path TEXT NOT NULL, content_sha256 TEXT NOT NULL, file_size INTEGER NOT NULL, mtime_ns INTEGER NOT NULL,
 recording_id TEXT NOT NULL, parent_type TEXT NOT NULL DEFAULT 'track', parent_id TEXT NOT NULL DEFAULT '',
 resource TEXT NOT NULL, relation_kind TEXT NOT NULL, position INTEGER NOT NULL,
 child_type TEXT NOT NULL, child_id TEXT NOT NULL, unavailable INTEGER NOT NULL,
 metadata_json BLOB NOT NULL CHECK(length(metadata_json)<=65536),
 PRIMARY KEY(file_path,content_sha256,file_size,mtime_ns,recording_id,parent_type,parent_id,resource,relation_kind,position));
 INSERT INTO spotify_download_catalog_relations_next
 (file_path,content_sha256,file_size,mtime_ns,recording_id,parent_type,parent_id,resource,relation_kind,position,child_type,child_id,unavailable,metadata_json)
 SELECT file_path,content_sha256,file_size,mtime_ns,recording_id,'track',recording_id,resource,relation_kind,position,child_type,child_id,unavailable,metadata_json
 FROM spotify_download_catalog_relations;
 DROP TABLE spotify_download_catalog_relations;
 ALTER TABLE spotify_download_catalog_relations_next RENAME TO spotify_download_catalog_relations;`)
	return err
}

func migrateDownloadedCatalogScope(tx *sql.Tx) error {
	rows, err := tx.Query("PRAGMA table_info(spotify_download_catalog_import_status)")
	if err != nil {
		return err
	}
	found := false
	for rows.Next() {
		var cid, notnull, pk int
		var name, typ string
		var def any
		if err = rows.Scan(&cid, &name, &typ, &notnull, &def, &pk); err != nil {
			rows.Close()
			return err
		}
		if name == "scope" {
			found = true
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	if found {
		return nil
	}
	_, err = tx.Exec("ALTER TABLE spotify_download_catalog_import_status ADD COLUMN scope TEXT NOT NULL DEFAULT 'track_and_direct_relations_v1'")
	return err
}
