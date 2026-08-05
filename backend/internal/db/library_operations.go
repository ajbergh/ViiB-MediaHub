package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
	CheckedAt               int64                     `json:"checkedAt"`
	Integrity                string                    `json:"integrity"`
	SongCount                int                       `json:"songCount"`
	SearchIndexCount         int                       `json:"searchIndexCount"`
	Revision                 int64                     `json:"revision"`
	RetainedChanges          int64                     `json:"retainedChanges"`
	MissingMedia             []MissingMedia            `json:"missingMedia"`
	BrokenPlaylistReferences []BrokenPlaylistReference `json:"brokenPlaylistReferences"`
	ScannerFailures          []ScannerFailure          `json:"scannerFailures"`
}

type SongMetadataPatch struct {
	Title       *string   `json:"title,omitempty"`
	Artist      *string   `json:"artist,omitempty"`
	Album       *string   `json:"album,omitempty"`
	AlbumArtist *string   `json:"albumArtist,omitempty"`
	TrackNumber *int      `json:"trackNumber,omitempty"`
	DiscNumber  *int      `json:"discNumber,omitempty"`
	Genre       *[]string `json:"genre,omitempty"`
	Year        *int      `json:"year,omitempty"`
}

func (d *DB) DatabasePath() (string, error) {
	rows, err := d.conn.Query(`PRAGMA database_list`)
	if err != nil { return "", err }
	defer rows.Close()
	for rows.Next() {
		var sequence int
		var name, path string
		if err := rows.Scan(&sequence, &name, &path); err != nil { return "", err }
		if name == "main" { return path, nil }
	}
	return "", fmt.Errorf("main SQLite database path was not found")
}

func (d *DB) CreateConsistentCopy(destination string) error {
	if err := d.CheckpointWAL(); err != nil { return err }
	if err := os.MkdirAll(filepath.Dir(destination), 0700); err != nil { return err }
	_ = os.Remove(destination)
	if _, err := d.conn.Exec(`VACUUM INTO ?`, destination); err == nil { return nil }
	escaped := strings.ReplaceAll(destination, "'", "''")
	_, err := d.conn.Exec(`VACUUM INTO '` + escaped + `'`)
	return err
}

func ValidateSQLiteCopy(path string) error {
	conn, err := sql.Open("sqlite3", "file:"+filepath.ToSlash(path)+"?mode=ro&_foreign_keys=on")
	if err != nil { return err }
	defer conn.Close()
	var integrity string
	if err := conn.QueryRow(`PRAGMA integrity_check`).Scan(&integrity); err != nil { return err }
	if integrity != "ok" { return fmt.Errorf("SQLite integrity check failed: %s", integrity) }
	return nil
}

func (d *DB) RunLibraryDiagnostics() (LibraryDiagnostics, error) {
	result := LibraryDiagnostics{
		CheckedAt: time.Now().UnixMilli(), MissingMedia: []MissingMedia{},
		BrokenPlaylistReferences: []BrokenPlaylistReference{}, ScannerFailures: []ScannerFailure{},
	}
	if err := d.conn.QueryRow(`PRAGMA integrity_check`).Scan(&result.Integrity); err != nil { return result, err }
	if err := d.conn.QueryRow(`SELECT COUNT(*) FROM songs WHERE COALESCE(ignored, 0) = 0`).Scan(&result.SongCount); err != nil { return result, err }
	if err := d.EnsureLibrarySyncSchema(); err != nil { return result, err }
	if err := d.conn.QueryRow(`SELECT COUNT(*) FROM song_search`).Scan(&result.SearchIndexCount); err != nil { return result, err }
	result.Revision, result.RetainedChanges, _ = d.LibrarySyncStats()

	rows, err := d.conn.Query(`SELECT id, title, file_path FROM songs WHERE COALESCE(ignored, 0) = 0`)
	if err != nil { return result, err }
	for rows.Next() {
		var item MissingMedia
		if err := rows.Scan(&item.SongID, &item.Title, &item.FilePath); err != nil { rows.Close(); return result, err }
		info, statErr := os.Stat(item.FilePath)
		if statErr != nil {
			item.Reason = statErr.Error()
			result.MissingMedia = append(result.MissingMedia, item)
		} else if info.IsDir() {
			item.Reason = "path resolves to a directory"
			result.MissingMedia = append(result.MissingMedia, item)
		}
	}
	if err := rows.Close(); err != nil { return result, err }

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
				playlistRows.Close(); return result, err
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
		if err != nil { return result, err }
		paths := make([]string, 0, len(diagnostics.MissingMedia))
		for _, missing := range diagnostics.MissingMedia { paths = append(paths, missing.FilePath) }
		if len(paths) > 0 {
			removed, err := d.DeleteSongsByFilePaths(paths)
			if err != nil { return result, err }
			result["removedMissing"] = removed
		}
	}

	tx, err := d.conn.Begin()
	if err != nil { return result, err }
	defer tx.Rollback()
	playlistRows, err := tx.Query(`SELECT id, song_ids FROM playlists`)
	if err != nil { return result, err }
	type playlistRepair struct { id string; ids []string }
	var repairs []playlistRepair
	for playlistRows.Next() {
		var id, raw string
		if err := playlistRows.Scan(&id, &raw); err != nil { playlistRows.Close(); return result, err }
		var ids []string
		if err := json.Unmarshal([]byte(raw), &ids); err != nil { continue }
		valid := make([]string, 0, len(ids))
		for _, songID := range ids {
			var exists int
			if err := tx.QueryRow(`SELECT 1 FROM songs WHERE id = ?`, songID).Scan(&exists); err == nil {
				valid = append(valid, songID)
			} else { result["removedPlaylistReferences"]++ }
		}
		repairs = append(repairs, playlistRepair{id: id, ids: valid})
	}
	_ = playlistRows.Close()
	for _, repair := range repairs {
		payload, _ := json.Marshal(repair.ids)
		if _, err := tx.Exec(`UPDATE playlists SET song_ids = ? WHERE id = ?`, string(payload), repair.id); err != nil { return result, err }
	}
	if err := tx.Commit(); err != nil { return result, err }
	if err := d.EnsureLibrarySyncSchema(); err != nil { return result, err }
	if err := d.conn.QueryRow(`SELECT COUNT(*) FROM song_search`).Scan(&result["rebuiltSearchRows"]); err != nil { return result, err }
	return result, nil
}

func (d *DB) UpdateSongMetadata(id string, patch SongMetadataPatch) (Song, error) {
	sets := make([]string, 0)
	args := make([]any, 0)
	add := func(column string, value any) { sets = append(sets, column+" = ?"); args = append(args, value) }
	if patch.Title != nil { value := strings.TrimSpace(*patch.Title); if value == "" { return Song{}, fmt.Errorf("title cannot be empty") }; add("title", value) }
	if patch.Artist != nil { value := strings.TrimSpace(*patch.Artist); if value == "" { return Song{}, fmt.Errorf("artist cannot be empty") }; add("artist", value) }
	if patch.Album != nil { value := strings.TrimSpace(*patch.Album); if value == "" { return Song{}, fmt.Errorf("album cannot be empty") }; add("album", value) }
	if patch.AlbumArtist != nil { add("album_artist", strings.TrimSpace(*patch.AlbumArtist)) }
	if patch.TrackNumber != nil { if *patch.TrackNumber < 0 { return Song{}, fmt.Errorf("trackNumber cannot be negative") }; add("track_number", *patch.TrackNumber) }
	if patch.DiscNumber != nil { if *patch.DiscNumber < 0 { return Song{}, fmt.Errorf("discNumber cannot be negative") }; add("disc_number", *patch.DiscNumber) }
	if patch.Year != nil { if *patch.Year < 0 || *patch.Year > 3000 { return Song{}, fmt.Errorf("year is outside the supported range") }; add("year", *patch.Year) }
	if patch.Genre != nil { normalized := NormalizeGenres(*patch.Genre); payload, _ := json.Marshal(normalized); add("genre", string(payload)) }
	if len(sets) == 0 { return d.getSongForOperation(id) }
	args = append(args, id)
	result, err := d.conn.Exec(`UPDATE songs SET `+strings.Join(sets, ", ")+` WHERE id = ?`, args...)
	if err != nil { return Song{}, err }
	rows, err := result.RowsAffected()
	if err != nil { return Song{}, err }
	if rows == 0 { return Song{}, sql.ErrNoRows }
	return d.getSongForOperation(id)
}

func (d *DB) getSongForOperation(id string) (Song, error) {
	row := d.conn.QueryRow(songSelect+` WHERE songs.id = ?`, id)
	return scanLibrarySong(row)
}
