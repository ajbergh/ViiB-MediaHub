package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// GenreStat represents aggregated statistics for a single music genre.
// This data is cached in the genre_stats table and updated after library scans.
type GenreStat struct {
	Name       string   `json:"name"`               // Genre name (e.g., "Rock", "Jazz")
	Count      int      `json:"count"`              // Number of tracks tagged with this genre
	TopArtists []string `json:"topArtists"`         // Up to 3 most frequent artists in this genre
	CoverUrl   string   `json:"coverUrl,omitempty"` // API URL to representative album cover (/api/cover/{songId})
}

// UpdateGenreStats recalculates genre statistics from the songs table and updates the genre_stats table.
// This function:
// 1. Queries all songs with genre metadata (stored as JSON arrays in the genre column)
// 2. Parses each song's genre array and counts individual genres
// 3. Tracks artist frequency per genre to identify top artists
// 4. Selects a representative cover image (from the most popular artist in each genre)
// 5. Replaces all entries in genre_stats with the newly calculated data
//
// Genre Handling: Songs can have multiple genres (e.g., ["Rock", "Alternative Rock"]).
// Each genre is counted individually, so a song with 3 genres contributes +1 to each.
//
// Cover Selection: For each genre, the cover is selected from a song by the most popular
// artist in that genre, ensuring visual representation matches the genre's top content.
//
// This function is called automatically:
// - On application startup (in api.New)
// - After each library scan completes (in scanner.Scan)
// - After individual file additions (in scanner.AddFile)
func (d *DB) UpdateGenreStats() error {
	// 1. Fetch all songs with genres, including song IDs for cover URL generation
	query := `SELECT id, artist, genre, cover_path FROM songs WHERE genre IS NOT NULL AND genre != '' AND genre != '[]'`
	rows, err := d.conn.Query(query)
	if err != nil {
		return fmt.Errorf("failed to query songs: %w", err)
	}
	defer rows.Close()

	// 2. Aggregate genre statistics in memory
	type songCover struct {
		songID    string // Song ID for generating API cover URL
		coverPath string // File path for fallback
	}
	type genreData struct {
		count   int                  // Total number of tracks with this genre
		artists map[string]int       // Artist name -> track count mapping
		covers  map[string]songCover // Artist name -> cover info (for selecting representative cover)
	}
	stats := make(map[string]*genreData)

	for rows.Next() {
		var songID string
		var artist string
		var genreJSON string
		var coverPath sql.NullString

		if err := rows.Scan(&songID, &artist, &genreJSON, &coverPath); err != nil {
			continue
		}

		// Parse genre JSON array (e.g., ["Rock", "Alternative Rock"])
		var genres []string
		if err := json.Unmarshal([]byte(genreJSON), &genres); err != nil {
			continue
		}

		// Process each individual genre tag
		for _, g := range genres {
			g = strings.TrimSpace(g)
			if g == "" {
				continue
			}

			// Initialize genre data structure if this is the first occurrence
			if _, exists := stats[g]; !exists {
				stats[g] = &genreData{
					artists: make(map[string]int),
					covers:  make(map[string]songCover),
				}
			}

			entry := stats[g]
			entry.count++
			entry.artists[artist]++

			// Store cover info for this artist (will use the most popular artist's cover later)
			// Only update if we don't have a cover for this artist yet or if this song has a cover
			if coverPath.Valid && coverPath.String != "" {
				if _, hascover := entry.covers[artist]; !hascover {
					entry.covers[artist] = songCover{
						songID:    songID,
						coverPath: coverPath.String,
					}
				}
			}
		}
	}

	// 3. Prepare database transaction for updating genre_stats table
	tx, err := d.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Clear existing stats (full refresh approach for simplicity and consistency)
	if _, err := tx.Exec("DELETE FROM genre_stats"); err != nil {
		return err
	}

	stmt, err := tx.Prepare("INSERT INTO genre_stats (name, count, artists, cover_url) VALUES (?, ?, ?, ?)")
	if err != nil {
		return err
	}
	defer stmt.Close()

	// Insert computed statistics for each genre
	for name, data := range stats {
		// Sort artists by their track count in descending order to find top contributors
		type artistCount struct {
			Name  string
			Count int
		}
		var acs []artistCount
		for a, c := range data.artists {
			acs = append(acs, artistCount{a, c})
		}
		sort.Slice(acs, func(i, j int) bool {
			return acs[i].Count > acs[j].Count
		})

		// Select up to 3 top artists for display in the UI
		topArtists := make([]string, 0)
		for i := 0; i < len(acs) && i < 3; i++ {
			topArtists = append(topArtists, acs[i].Name)
		}
		artistsJSON, _ := json.Marshal(topArtists)

		// Select cover from the most popular artist in this genre
		// This ensures the genre's visual representation matches its most prominent content
		coverUrl := ""
		if len(acs) > 0 {
			topArtist := acs[0].Name
			if cover, exists := data.covers[topArtist]; exists {
				// Use song ID-based URL for proper API routing through /api/cover/{songId}
				// This works with the serveCover endpoint which handles song ID lookups
				coverUrl = "/api/cover/" + cover.songID
			}
		}

		if _, err := stmt.Exec(name, data.count, string(artistsJSON), coverUrl); err != nil {
			return err
		}
	}

	return tx.Commit()
}

// GetGenreNames returns all known genre names from the genre_stats table.
// This is useful for local genre matching before calling external AI APIs.
func (d *DB) GetGenreNames() ([]string, error) {
	query := `SELECT name FROM genre_stats ORDER BY count DESC`
	rows, err := d.conn.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		names = append(names, name)
	}
	return names, rows.Err()
}

// GetSongsByExactGenre returns songs that have the specified genre in their genre array.
// Uses JSON matching to find songs where the genre array contains the exact genre name.
// Returns up to 50 random songs matching the genre.
func (d *DB) GetSongsByExactGenre(genreName string) ([]Song, error) {
	// Match the genre name within the JSON array - handles exact matches like ["90s Alternative", "Rock"]
	// We use LIKE with proper JSON escaping for reliability
	query := `SELECT id, title, artist, album, genre, year, duration, file_path, cover_path, added_at, play_count, last_played 
			  FROM songs 
			  WHERE genre LIKE ? ESCAPE '\\' OR genre LIKE ? ESCAPE '\\' OR genre LIKE ? ESCAPE '\\'
			  ORDER BY RANDOM() LIMIT 50`

	// Match three patterns:
	// 1. '["GenreName"' - genre is first in array
	// 2. ', "GenreName"' - genre is in middle of array
	// 3. '"GenreName"]' - genre is last in array (and might be only element)
	args := []interface{}{
		buildGenreLikePattern(genreName), // General match within JSON (with escaping)
	}
	// Actually, simpler approach: just use a single LIKE that matches the quoted genre name
	// Use ESCAPE clause to handle any wildcards (%, _) in genre names safely
	query = `SELECT id, title, artist, album, genre, year, duration, file_path, cover_path, added_at, play_count, last_played 
			 FROM songs 
			 WHERE genre LIKE ? ESCAPE '\\'
			 ORDER BY RANDOM() LIMIT 50`
	args = []interface{}{buildGenreLikePattern(genreName)}

	rows, err := d.conn.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var songs []Song
	for rows.Next() {
		var s Song
		var genreJSON sql.NullString
		var coverPath sql.NullString
		var lastPlayed sql.NullInt64

		err := rows.Scan(
			&s.ID, &s.Title, &s.Artist, &s.Album, &genreJSON, &s.Year, &s.Duration,
			&s.FilePath, &coverPath, &s.AddedAt, &s.PlayCount, &lastPlayed,
		)
		if err != nil {
			return nil, err
		}

		if genreJSON.Valid && genreJSON.String != "" {
			json.Unmarshal([]byte(genreJSON.String), &s.Genre)
		}
		if coverPath.Valid {
			s.CoverPath = coverPath.String
		}
		if lastPlayed.Valid {
			s.LastPlayed = lastPlayed.Int64
		}
		songs = append(songs, s)
	}
	return songs, rows.Err()
}

// GetAllGenreStats returns all cached genre statistics ordered by track count (most popular first).
//
// This function queries the pre-computed genre_stats table which is populated by UpdateGenreStats.
// The returned data includes:
// - Genre name
// - Total number of tracks tagged with this genre
// - Top 3 most frequent artists in the genre
// - API URL to a representative cover image (/api/cover/{songId})
//
// Returns an empty slice if no genres are found or if genre_stats hasn't been populated yet.
// The data is sorted by count in descending order, so the most popular genres appear first.
func (d *DB) GetAllGenreStats() ([]GenreStat, error) {
	query := `SELECT name, count, artists, cover_url FROM genre_stats ORDER BY count DESC`
	rows, err := d.conn.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// Initialize as empty slice to ensure JSON serializes as [] not null
	stats := make([]GenreStat, 0)
	for rows.Next() {
		var s GenreStat
		var artistsJSON string
		var coverUrl sql.NullString

		if err := rows.Scan(&s.Name, &s.Count, &artistsJSON, &coverUrl); err != nil {
			return nil, err
		}

		// Parse top artists JSON array
		if err := json.Unmarshal([]byte(artistsJSON), &s.TopArtists); err != nil {
			s.TopArtists = []string{} // Fallback to empty array on parse error
		}
		// Ensure TopArtists is never nil
		if s.TopArtists == nil {
			s.TopArtists = []string{}
		}

		// Cover URL is already in API format (/api/cover/{songId}) from UpdateGenreStats
		if coverUrl.Valid {
			s.CoverUrl = coverUrl.String
		}

		stats = append(stats, s)
	}

	return stats, nil
}
