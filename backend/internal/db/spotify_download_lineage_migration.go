package db

import "database/sql"

// Collection identity is part of the origin key: two saved collections can
// request the same recording at the same unknown position. Legacy rows keep
// an empty identity and never claim a specific saved collection.
func migrateDownloadLineageEntity(tx *sql.Tx) error {
	for _, table := range []struct{ name, identity, definition string }{
		{"spotify_download_lineage_staging", "download_id,context_key", `download_id TEXT NOT NULL REFERENCES spotify_downloads(id) ON DELETE CASCADE, context_key TEXT NOT NULL`},
		{"spotify_download_lineage_imports", "file_path,content_sha256,file_size,mtime_ns,recording_id", `file_path TEXT NOT NULL, content_sha256 TEXT NOT NULL, file_size INTEGER NOT NULL, mtime_ns INTEGER NOT NULL, recording_id TEXT NOT NULL`},
	} {
		rows, err := tx.Query("PRAGMA table_info(" + table.name + ")")
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
			if name == "entity_id" {
				found = true
			}
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		if found {
			continue
		}
		columns := table.identity + ",origin_kind,origin_id,origin_revision,position"
		_, err = tx.Exec("CREATE TABLE " + table.name + "_next (" + table.definition + `, origin_kind TEXT NOT NULL, origin_id TEXT NOT NULL, origin_revision TEXT NOT NULL, position INTEGER NOT NULL, entity_id TEXT NOT NULL DEFAULT '', PRIMARY KEY(` + columns + `,entity_id));
 INSERT INTO ` + table.name + "_next (" + columns + ",entity_id) SELECT " + columns + ",'' FROM " + table.name + "; DROP TABLE " + table.name + "; ALTER TABLE " + table.name + "_next RENAME TO " + table.name + ";")
		if err != nil {
			return err
		}
	}
	return nil
}
