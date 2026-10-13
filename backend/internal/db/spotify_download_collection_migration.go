package db

import "database/sql"

func migrateDownloadCollections(tx *sql.Tx) error {
	_, err := tx.Exec(`CREATE TABLE IF NOT EXISTS spotify_download_collection_status(file_path TEXT NOT NULL,content_sha256 TEXT NOT NULL,file_size INTEGER NOT NULL,mtime_ns INTEGER NOT NULL,recording_id TEXT NOT NULL,state TEXT NOT NULL,reason TEXT NOT NULL,checked_at INTEGER NOT NULL,PRIMARY KEY(file_path,content_sha256,file_size,mtime_ns,recording_id))`)
	if err != nil {
		return err
	}
	for _, column := range []string{"captured_resource", "capture_revision"} {
		rows, e := tx.Query("PRAGMA table_info(spotify_download_catalog_imports)")
		if e != nil {
			return e
		}
		found := false
		for rows.Next() {
			var cid, notnull, pk int
			var name, typ string
			var def any
			if e = rows.Scan(&cid, &name, &typ, &notnull, &def, &pk); e != nil {
				rows.Close()
				return e
			}
			if name == column {
				found = true
			}
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return e
		}
		if found {
			continue
		}
		if _, e = tx.Exec("ALTER TABLE spotify_download_catalog_imports ADD COLUMN " + column + " TEXT NOT NULL DEFAULT ''"); e != nil {
			return e
		}
	}
	return nil
}
