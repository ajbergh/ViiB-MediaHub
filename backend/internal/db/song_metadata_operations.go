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
	songs, err := d.updateSongsMetadata([]string{id}, patch)
	if err != nil {
		return Song{}, err
	}
	return songs[0], nil
}

// UpdateAlbumMetadata atomically applies shared album fields to the specified tracks.
func (d *DB) UpdateAlbumMetadata(ids []string, patch SongMetadataPatch) ([]Song, error) {
	if len(ids) == 0 {
		return nil, fmt.Errorf("at least one track is required")
	}
	if patch.Title != nil || patch.TrackNumber != nil || patch.DiscNumber != nil {
		return nil, fmt.Errorf("track titles and numbers must be edited individually")
	}
	if patch.Album == nil && patch.AlbumArtist == nil && patch.Artist == nil && patch.Year == nil && patch.Genre == nil {
		return nil, fmt.Errorf("at least one album field is required")
	}
	return d.updateSongsMetadata(ids, patch)
}

func songMetadataAssignments(patch SongMetadataPatch) ([]string, []any, error) {
	sets := make([]string, 0)
	args := make([]any, 0)
	add := func(column string, value any) { sets = append(sets, column+" = ?"); args = append(args, value) }

	if patch.Title != nil {
		value := strings.TrimSpace(*patch.Title)
		if value == "" {
			return nil, nil, fmt.Errorf("title cannot be empty")
		}
		add("title", value)
	}
	if patch.Artist != nil {
		value := strings.TrimSpace(*patch.Artist)
		if value == "" {
			return nil, nil, fmt.Errorf("artist cannot be empty")
		}
		add("artist", value)
	}
	if patch.Album != nil {
		value := strings.TrimSpace(*patch.Album)
		if value == "" {
			return nil, nil, fmt.Errorf("album cannot be empty")
		}
		add("album", value)
	}
	if patch.AlbumArtist != nil {
		add("album_artist", strings.TrimSpace(*patch.AlbumArtist))
	}
	if patch.TrackNumber != nil {
		if *patch.TrackNumber < 0 {
			return nil, nil, fmt.Errorf("trackNumber cannot be negative")
		}
		add("track_number", *patch.TrackNumber)
	}
	if patch.DiscNumber != nil {
		if *patch.DiscNumber < 0 {
			return nil, nil, fmt.Errorf("discNumber cannot be negative")
		}
		add("disc_number", *patch.DiscNumber)
	}
	if patch.Year != nil {
		if *patch.Year < 0 || *patch.Year > 3000 {
			return nil, nil, fmt.Errorf("year is outside the supported range")
		}
		add("year", *patch.Year)
		add("original_year", *patch.Year)
	}
	if patch.Genre != nil {
		normalized := NormalizeGenres(*patch.Genre)
		payload, _ := json.Marshal(normalized)
		add("genre", string(payload))
	}
	return sets, args, nil
}

func (d *DB) updateSongsMetadata(ids []string, patch SongMetadataPatch) ([]Song, error) {
	sets, args, err := songMetadataAssignments(patch)
	if err != nil {
		return nil, err
	}
	tx, err := d.conn.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	songs := make([]Song, 0, len(ids))
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if id == "" {
			return nil, fmt.Errorf("track ID cannot be empty")
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		if len(sets) != 0 {
			if err := updateTrackMetadataTx(tx, id, sets, args, patch.Year); err != nil {
				return nil, err
			}
		}
		song, err := scanLibrarySong(tx.QueryRow(songSelect+` WHERE songs.id = ?`, id))
		if err != nil {
			return nil, err
		}
		songs = append(songs, song)
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return songs, nil
}

func updateTrackMetadataTx(tx *sql.Tx, id string, sets []string, args []any, year *int) error {
	result, err := tx.Exec(`UPDATE songs SET `+strings.Join(sets, ", ")+` WHERE id = ?`, append(args, id)...)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return sql.ErrNoRows
	}
	if year == nil {
		// A corrected recording identity needs a fresh lookup. Manual years remain
		// authoritative until explicitly edited again.
		if _, err := tx.Exec(`UPDATE songs SET original_year=0 WHERE id=? AND EXISTS(SELECT 1 FROM song_release_metadata r WHERE r.song_id=songs.id AND r.manual_year=0 AND (r.title<>songs.title OR r.artist<>songs.artist OR r.album<>songs.album))`, id); err != nil {
			return err
		}
	}
	if year != nil {
		_, err = tx.Exec(`INSERT INTO song_release_metadata(song_id,title,artist,album,file_hash,manual_year,original_date,original_source,state)
		 SELECT id,title,artist,album,COALESCE(file_hash,''),1,?,'manual','available' FROM songs WHERE id=?
		 ON CONFLICT(song_id) DO UPDATE SET manual_year=1,original_date=excluded.original_date,original_source='manual',state='available'`, releasedate.YearDate(*year), id)
		if err != nil {
			return err
		}
	}
	return nil
}

func (d *DB) getSongForOperation(id string) (Song, error) {
	row := d.conn.QueryRow(songSelect+` WHERE songs.id = ?`, id)
	return scanLibrarySong(row)
}
