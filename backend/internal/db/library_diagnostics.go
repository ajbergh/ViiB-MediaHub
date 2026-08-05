package db

import (
	"encoding/json"
	"os"
	"time"
)

type MissingMedia struct {
	SongID   string `json:"songId"`
	Title    string `json:"title"`
	FilePath string `json:"filePath"`
	Reason   string `json:"reason"`
}

type BrokenPlaylistReference struct {
	PlaylistID   string `json:"playlistId"`
	PlaylistName string `json:"playlistName"`
	SongID       string `json:"songId"`
}

type LibraryDiagnostics struct {
	CheckedAt                int64                     `json:"checkedAt"`
	Integrity                string                    `json:"integrity"`
	SongCount                int                       `json:"songCount"`
	SearchIndexCount         int                       `json:"searchIndexCount"`
	Revision                 int64                     `json:"revision"`
	RetainedChanges          int64                     `json:"retainedChanges"`
	MissingMedia             []MissingMedia            `json:"missingMedia"`
	BrokenPlaylistReferences []BrokenPlaylistReference `json:"brokenPlaylistReferences"`
	ScannerFailures          []ScannerFailure          `json:"scannerFailures"`
}

func (d *DB) RunLibraryDiagnostics() (LibraryDiagnostics, error) {
	result := LibraryDiagnostics{
		CheckedAt: time.Now().UnixMilli(), MissingMedia: []MissingMedia{},
		BrokenPlaylistReferences: []BrokenPlaylistReference{}, ScannerFailures: []ScannerFailure{},
	}
	if err := d.conn.QueryRow(`PRAGMA integrity_check`).Scan(&result.Integrity); err != nil {
		return result, err
	}
	if err := d.conn.QueryRow(`SELECT COUNT(*) FROM songs WHERE COALESCE(ignored, 0) = 0`).Scan(&result.SongCount); err != nil {
		return result, err
	}
	if err := d.EnsureLibrarySyncSchema(); err != nil {
		return result, err
	}
	if err := d.conn.QueryRow(`SELECT COUNT(*) FROM song_search`).Scan(&result.SearchIndexCount); err != nil {
		return result, err
	}
	result.Revision, result.RetainedChanges, _ = d.LibrarySyncStats()

	rows, err := d.conn.Query(`SELECT id, title, file_path FROM songs WHERE COALESCE(ignored, 0) = 0`)
	if err != nil {
		return result, err
	}
	for rows.Next() {
		var item MissingMedia
		if err := rows.Scan(&item.SongID, &item.Title, &item.FilePath); err != nil {
			rows.Close()
			return result, err
		}
		info, statErr := os.Stat(item.FilePath)
		if statErr != nil {
			item.Reason = statErr.Error()
			result.MissingMedia = append(result.MissingMedia, item)
		} else if info.IsDir() {
			item.Reason = "path resolves to a directory"
			result.MissingMedia = append(result.MissingMedia, item)
		}
	}
	if err := rows.Close(); err != nil {
		return result, err
	}

	playlistRows, err := d.conn.Query(`
		SELECT p.id, p.name, j.value
		FROM playlists p, json_each(p.song_ids) j
		LEFT JOIN songs s ON s.id = j.value
		WHERE s.id IS NULL
		ORDER BY p.name, j.key`)
	if err == nil {
		for playlistRows.Next() {
			var reference BrokenPlaylistReference
			if err := playlistRows.Scan(&reference.PlaylistID, &reference.PlaylistName, &reference.SongID); err != nil {
				playlistRows.Close()
				return result, err
			}
			result.BrokenPlaylistReferences = append(result.BrokenPlaylistReferences, reference)
		}
		_ = playlistRows.Close()
	}
	result.ScannerFailures, _ = d.ListScannerFailures(1000)
	return result, nil
}

func (d *DB) RepairLibraryIndexes(removeMissing bool) (map[string]int, error) {
	result := map[string]int{"removedMissing": 0, "removedPlaylistReferences": 0, "rebuiltSearchRows": 0}
	if removeMissing {
		diagnostics, err := d.RunLibraryDiagnostics()
		if err != nil {
			return result, err
		}
		paths := make([]string, 0, len(diagnostics.MissingMedia))
		for _, missing := range diagnostics.MissingMedia {
			paths = append(paths, missing.FilePath)
		}
		if len(paths) > 0 {
			removed, err := d.DeleteSongsByFilePaths(paths)
			if err != nil {
				return result, err
			}
			result["removedMissing"] = removed
		}
	}

	tx, err := d.conn.Begin()
	if err != nil {
		return result, err
	}
	defer tx.Rollback()
	playlistRows, err := tx.Query(`SELECT id, song_ids FROM playlists`)
	if err != nil {
		return result, err
	}
	type playlistRepair struct {
		id  string
		ids []string
	}
	var repairs []playlistRepair
	for playlistRows.Next() {
		var id, raw string
		if err := playlistRows.Scan(&id, &raw); err != nil {
			playlistRows.Close()
			return result, err
		}
		var ids []string
		if err := json.Unmarshal([]byte(raw), &ids); err != nil {
			continue
		}
		valid := make([]string, 0, len(ids))
		for _, songID := range ids {
			var exists int
			if err := tx.QueryRow(`SELECT 1 FROM songs WHERE id = ?`, songID).Scan(&exists); err == nil {
				valid = append(valid, songID)
			} else {
				result["removedPlaylistReferences"]++
			}
		}
		repairs = append(repairs, playlistRepair{id: id, ids: valid})
	}
	_ = playlistRows.Close()
	for _, repair := range repairs {
		payload, _ := json.Marshal(repair.ids)
		if _, err := tx.Exec(`UPDATE playlists SET song_ids = ? WHERE id = ?`, string(payload), repair.id); err != nil {
			return result, err
		}
	}
	if err := tx.Commit(); err != nil {
		return result, err
	}

	if err := d.RebuildLibrarySearchIndex(); err != nil {
		return result, err
	}
	var rebuiltRows int
	if err := d.conn.QueryRow(`SELECT COUNT(*) FROM song_search`).Scan(&rebuiltRows); err != nil {
		return result, err
	}
	result["rebuiltSearchRows"] = rebuiltRows
	return result, nil
}
