package db

import "database/sql"

// Scan folder operations

// GetScanFolders returns the configured scan folders for the library.
func (d *DB) GetScanFolders() ([]ScanFolder, error) {
	rows, err := d.conn.Query(`SELECT id, path, added_at, last_scan, song_count FROM scan_folders ORDER BY path`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var folders []ScanFolder
	for rows.Next() {
		var f ScanFolder
		var lastScan sql.NullInt64

		err := rows.Scan(&f.ID, &f.Path, &f.AddedAt, &lastScan, &f.SongCount)
		if err != nil {
			return nil, err
		}

		if lastScan.Valid {
			f.LastScan = lastScan.Int64
		}

		folders = append(folders, f)
	}

	return folders, rows.Err()
}

// AddScanFolder adds a new folder to be scanned for music files.
func (d *DB) AddScanFolder(f *ScanFolder) error {
	_, err := d.conn.Exec(`
		INSERT INTO scan_folders (id, path, added_at, song_count)
		VALUES (?, ?, ?, 0)
		ON CONFLICT(path) DO NOTHING
	`, f.ID, f.Path, f.AddedAt)
	return err
}

// UpdateScanFolder updates the scan timestamp and song count for a folder.
func (d *DB) UpdateScanFolder(id string, lastScan int64, songCount int) error {
	_, err := d.conn.Exec(`
		UPDATE scan_folders SET last_scan = ?, song_count = ? WHERE id = ?
	`, lastScan, songCount, id)
	return err
}

// RemoveScanFolder removes a configured scan folder.
func (d *DB) RemoveScanFolder(id string) error {
	_, err := d.conn.Exec("DELETE FROM scan_folders WHERE id = ?", id)
	return err
}
