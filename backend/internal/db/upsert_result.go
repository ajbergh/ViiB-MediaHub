// Reports song inserts and updates during batch saves and reconciles Spotify download recording evidence.
package db

import (
	"context"
	"log"
	"strings"
)

// SongUpsertResult makes scan reporting distinguish inserts from updates.
type SongUpsertResult struct {
	Inserted int
	Updated  int
}

// SaveSongsWithResult records whether each path existed before the upsert. Scan
// batches are intentionally small, so the bounded IN query remains inexpensive.
func (d *DB) SaveSongsWithResult(songs []Song) (SongUpsertResult, error) {
	result := SongUpsertResult{}
	if len(songs) == 0 {
		return result, nil
	}

	placeholders := strings.TrimRight(strings.Repeat("?,", len(songs)), ",")
	args := make([]any, 0, len(songs))
	for _, song := range songs {
		args = append(args, song.FilePath)
	}

	existing := make(map[string]struct{}, len(songs))
	rows, err := d.conn.Query(`SELECT file_path FROM songs WHERE file_path IN (`+placeholders+`)`, args...)
	if err != nil {
		return result, err
	}
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err != nil {
			rows.Close()
			return result, err
		}
		existing[path] = struct{}{}
	}
	if err := rows.Close(); err != nil {
		return result, err
	}

	if err := d.SaveSongs(songs); err != nil {
		return result, err
	}
	paths := make([]string, 0, len(songs))
	for _, song := range songs {
		paths = append(paths, song.FilePath)
		if _, found := existing[song.FilePath]; found {
			result.Updated++
		} else {
			result.Inserted++
		}
	}
	if err := d.ReconcileSpotifyDownloads(context.Background(), paths); err != nil {
		log.Printf("Spotify recording reconciliation failed: %v", err)
	}
	return result, nil
}
