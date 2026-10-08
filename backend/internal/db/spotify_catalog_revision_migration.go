package db

import "database/sql"

// Existing independent catalog observations remain explicitly unbound.
func migrateSpotifyCaptureRevision(tx *sql.Tx) error {
	rows, err := tx.Query("PRAGMA table_info(spotify_entity_snapshots)")
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
		if name == "capture_revision" {
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
	_, err = tx.Exec("ALTER TABLE spotify_entity_snapshots ADD COLUMN capture_revision TEXT NOT NULL DEFAULT ''")
	return err
}
