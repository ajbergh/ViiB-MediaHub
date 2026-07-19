package db

import (
	"database/sql"
	"encoding/json"
)

func (d *DB) GetPlaylistByID(id string) (*Playlist, error) {
	var playlist Playlist
	var songIDs string
	var coverPath sql.NullString
	if err := d.conn.QueryRow(`SELECT id, name, song_ids, cover_path, created_at FROM playlists WHERE id = ?`, id).Scan(
		&playlist.ID, &playlist.Name, &songIDs, &coverPath, &playlist.CreatedAt,
	); err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(songIDs), &playlist.SongIDs); err != nil {
		return nil, err
	}
	if coverPath.Valid {
		playlist.CoverPath = coverPath.String
	}
	return &playlist, nil
}
