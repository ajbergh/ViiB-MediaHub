package db

import (
	"database/sql"
	"errors"
	"os"
)

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
		var previousPath string
		err = d.conn.QueryRow(`SELECT id, file_path FROM songs WHERE file_hash = ? LIMIT 1`, fingerprint).Scan(&id, &previousPath)
		if err == nil {
			// Reuse identity only for a confirmed move. If the old path still exists,
			// this is a duplicate copy and must receive its own logical ID.
			if _, statErr := os.Stat(previousPath); errors.Is(statErr, os.ErrNotExist) {
				return id, nil
			}
			return proposedID, nil
		}
		if err != sql.ErrNoRows {
			return "", err
		}
	}
	return proposedID, nil
}
