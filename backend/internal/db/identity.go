package db

import "database/sql"

// ResolveSongIdentity preserves the logical song ID when a file is rescanned or
// moved. Existing path identity wins during the first fingerprint migration;
// after that, the stable file hash reconciles renames and moves.
func (d *DB) ResolveSongIdentity(filePath, fingerprint, proposedID string) (string, error) {
	var id string
	err := d.conn.QueryRow(`SELECT id FROM songs WHERE file_path = ?`, filePath).Scan(&id)
	if err == nil {
		return id, nil
	}
	if err != sql.ErrNoRows {
		return "", err
	}

	if fingerprint != "" {
		err = d.conn.QueryRow(`SELECT id FROM songs WHERE file_hash = ? LIMIT 1`, fingerprint).Scan(&id)
		if err == nil {
			return id, nil
		}
		if err != sql.ErrNoRows {
			return "", err
		}
	}
	return proposedID, nil
}
