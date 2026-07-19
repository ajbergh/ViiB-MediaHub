package db

import "database/sql"

// DuplicateGroup represents active library entries with the same media fingerprint.
type DuplicateGroup struct {
	FileHash string `json:"fileHash"`
	Songs    []Song `json:"songs"`
}

func (d *DB) GetDuplicateGroups() ([]DuplicateGroup, error) {
	songs, err := d.GetAllSongs()
	if err != nil {
		return nil, err
	}
	grouped := make(map[string][]Song)
	for _, song := range songs {
		if song.FileHash != "" {
			grouped[song.FileHash] = append(grouped[song.FileHash], song)
		}
	}
	result := make([]DuplicateGroup, 0)
	for hash, candidates := range grouped {
		if len(candidates) > 1 {
			result = append(result, DuplicateGroup{FileHash: hash, Songs: candidates})
		}
	}
	return result, nil
}

func (d *DB) SetSongIgnored(id string, ignored bool) error {
	value := 0
	if ignored {
		value = 1
	}
	result, err := d.conn.Exec(`UPDATE songs SET ignored = ? WHERE id = ?`, value, id)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (d *DB) GetIgnoredSongs() ([]Song, error) {
	rows, err := d.conn.Query(`SELECT id FROM songs WHERE COALESCE(ignored, 0) = 1 ORDER BY artist, album, title`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	songs := make([]Song, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		song, err := d.GetSongByID(id)
		if err != nil {
			return nil, err
		}
		songs = append(songs, *song)
	}
	return songs, rows.Err()
}

func (d *DB) GetIgnoredFilePaths() ([]string, error) {
	rows, err := d.conn.Query(`SELECT file_path FROM songs WHERE COALESCE(ignored, 0) = 1`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	paths := make([]string, 0)
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err != nil {
			return nil, err
		}
		paths = append(paths, path)
	}
	return paths, rows.Err()
}
