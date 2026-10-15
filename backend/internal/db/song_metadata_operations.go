// song_metadata_operations.go updates library database metadata without writing source tags.
package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ajbergh/viib-mediahub/internal/releasedate"
)

// SongMetadataPatch contains the database-backed metadata fields that may be edited.
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

// UpdateSongMetadata validates and persists a partial database metadata patch.
// It does not mutate the underlying media file.
func (d *DB) UpdateSongMetadata(id string, patch SongMetadataPatch) (Song, error) {
	sets := make([]string, 0)
	args := make([]any, 0)
	add := func(column string, value any) { sets = append(sets, column+" = ?"); args = append(args, value) }

	if patch.Title != nil {
		value := strings.TrimSpace(*patch.Title)
		if value == "" {
			return Song{}, fmt.Errorf("title cannot be empty")
		}
		add("title", value)
	}
	if patch.Artist != nil {
		value := strings.TrimSpace(*patch.Artist)
		if value == "" {
			return Song{}, fmt.Errorf("artist cannot be empty")
		}
		add("artist", value)
	}
	if patch.Album != nil {
		value := strings.TrimSpace(*patch.Album)
		if value == "" {
			return Song{}, fmt.Errorf("album cannot be empty")
		}
		add("album", value)
	}
	if patch.AlbumArtist != nil {
		add("album_artist", strings.TrimSpace(*patch.AlbumArtist))
	}
	if patch.TrackNumber != nil {
		if *patch.TrackNumber < 0 {
			return Song{}, fmt.Errorf("trackNumber cannot be negative")
		}
		add("track_number", *patch.TrackNumber)
	}
	if patch.DiscNumber != nil {
		if *patch.DiscNumber < 0 {
			return Song{}, fmt.Errorf("discNumber cannot be negative")
		}
		add("disc_number", *patch.DiscNumber)
	}
	if patch.Year != nil {
		if *patch.Year < 0 || *patch.Year > 3000 {
			return Song{}, fmt.Errorf("year is outside the supported range")
		}
		add("year", *patch.Year)
		add("original_year", *patch.Year)
	}
	if patch.Genre != nil {
		normalized := NormalizeGenres(*patch.Genre)
		payload, _ := json.Marshal(normalized)
		add("genre", string(payload))
	}
	if len(sets) == 0 {
		return d.getSongForOperation(id)
	}

	args = append(args, id)
	tx, err := d.conn.Begin()
	if err != nil {
		return Song{}, err
	}
	defer tx.Rollback()
	result, err := tx.Exec(`UPDATE songs SET `+strings.Join(sets, ", ")+` WHERE id = ?`, args...)
	if err != nil {
		return Song{}, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return Song{}, err
	}
	if rows == 0 {
		return Song{}, sql.ErrNoRows
	}
	if patch.Year == nil {
		// A corrected recording identity needs a fresh lookup. Manual years remain
		// authoritative until explicitly edited again.
		if _, err := tx.Exec(`UPDATE songs SET original_year=0 WHERE id=? AND EXISTS(SELECT 1 FROM song_release_metadata r WHERE r.song_id=songs.id AND r.manual_year=0 AND (r.title<>songs.title OR r.artist<>songs.artist OR r.album<>songs.album))`, id); err != nil {
			return Song{}, err
		}
	}
	if patch.Year != nil {
		_, err = tx.Exec(`INSERT INTO song_release_metadata(song_id,title,artist,album,file_hash,manual_year,original_date,original_source,state)
		 SELECT id,title,artist,album,COALESCE(file_hash,''),1,?,'manual','available' FROM songs WHERE id=?
		 ON CONFLICT(song_id) DO UPDATE SET manual_year=1,original_date=excluded.original_date,original_source='manual',state='available'`, releasedate.YearDate(*patch.Year), id)
		if err != nil {
			return Song{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return Song{}, err
	}
	return d.getSongForOperation(id)
}

func (d *DB) getSongForOperation(id string) (Song, error) {
	row := d.conn.QueryRow(songSelect+` WHERE songs.id = ?`, id)
	return scanLibrarySong(row)
}
