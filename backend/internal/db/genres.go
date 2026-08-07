package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"unicode"
)

// GenreStat represents aggregated statistics for a single music genre.
// This data is cached in the genre_stats table and updated after library scans.
type GenreStat struct {
	Name       string   `json:"name"`               // Genre name (e.g., "Rock", "Jazz")
	Count      int      `json:"count"`              // Number of tracks tagged with this genre
	TopArtists []string `json:"topArtists"`         // Up to 3 most frequent artists in this genre
	CoverUrl   string   `json:"coverUrl,omitempty"` // API URL to representative album cover (/api/cover/{songId})
}

// specialCaseGenres maps lowercase genre names to their preferred capitalization.
// These are exceptions to the standard Title Case rule.
var specialCaseGenres = map[string]string{
	"r&b":              "R&B",
	"rnb":              "R&B",
	"edm":              "EDM",
	"dj":               "DJ",
	"uk":               "UK",
	"us":               "US",
	"lo-fi":            "Lo-Fi",
	"lofi":             "Lo-Fi",
	"lo fi":            "Lo-Fi",
	"hi-fi":            "Hi-Fi",
	"hifi":             "Hi-Fi",
	"dnb":              "DnB",
	"d&b":              "D&B",
	"idm":              "IDM",
	"ebm":              "EBM",
	"j-pop":            "J-Pop",
	"jpop":             "J-Pop",
	"k-pop":            "K-Pop",
	"kpop":             "K-Pop",
	"tv":               "TV",
	"ost":              "OST",
	"nyc":              "NYC",
	"la":               "LA",
	"nu-jazz":          "Nu-Jazz",
	"nu jazz":          "Nu-Jazz",
	"acid jazz":        "Acid Jazz",
	"trip-hop":         "Trip-Hop",
	"trip hop":         "Trip-Hop",
	"hip-hop":          "Hip-Hop",
	"hip hop":          "Hip-Hop",
	"neo-soul":         "Neo-Soul",
	"neo soul":         "Neo-Soul",
	"avant-garde":      "Avant-Garde",
	"post-punk":        "Post-Punk",
	"post punk":        "Post-Punk",
	"post-rock":        "Post-Rock",
	"post rock":        "Post-Rock",
	"drum and bass":    "Drum and Bass",
	"drum n bass":      "Drum and Bass",
	"drum & bass":      "Drum and Bass",
	"rock and roll":    "Rock and Roll",
	"rock n roll":      "Rock and Roll",
	"rock & roll":      "Rock and Roll",
	"rhythm and blues": "Rhythm and Blues",
}

// NormalizeGenre converts a genre name to a consistent capitalization format.
// It applies Title Case (capitalizing the first letter of each word) with
// special handling for known acronyms and genre-specific conventions.
//
// Examples:
//   - "acid jazz" → "Acid Jazz"
//   - "r&b" → "R&B"
//   - "90s rock" → "90s Rock"
//   - "hip-hop" → "Hip-Hop"
//   - "post punk" → "Post-Punk"
func NormalizeGenre(genre string) string {
	genre = strings.TrimSpace(genre)
	if genre == "" {
		return ""
	}

	lower := strings.ToLower(genre)

	// Check for exact special case match
	if special, ok := specialCaseGenres[lower]; ok {
		return special
	}

	// Apply Title Case with smart word handling
	words := strings.Fields(genre)
	for i, word := range words {
		wordLower := strings.ToLower(word)

		// Check if this word alone is a special case
		if special, ok := specialCaseGenres[wordLower]; ok {
			words[i] = special
			continue
		}

		// Preserve decade prefixes like "90s", "2000s"
		if len(word) >= 2 && unicode.IsDigit(rune(word[0])) {
			// Keep as-is if it looks like a decade (e.g., "90s", "2000s")
			words[i] = word
			continue
		}

		// Title case: capitalize first letter, lowercase rest
		if len(word) > 0 {
			runes := []rune(wordLower)
			runes[0] = unicode.ToUpper(runes[0])
			words[i] = string(runes)
		}
	}

	return strings.Join(words, " ")
}

// NormalizeGenres normalizes a slice of genre names.
func NormalizeGenres(genres []string) []string {
	normalized := make([]string, 0, len(genres))
	seen := make(map[string]bool)

	for _, g := range genres {
		n := NormalizeGenre(g)
		if n == "" {
			continue
		}
		// Deduplicate (different capitalizations become same after normalization)
		lower := strings.ToLower(n)
		if !seen[lower] {
			seen[lower] = true
			normalized = append(normalized, n)
		}
	}
	return normalized
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
	// Match the quoted genre name within the stored JSON array. The pattern
	// builder escapes SQL LIKE wildcards before binding the argument.
	query := `SELECT id, title, artist, album, genre, year, duration, file_path, cover_path, added_at, play_count, last_played
			  FROM songs
			  WHERE genre LIKE ? ESCAPE '\'
			  ORDER BY RANDOM() LIMIT 50`
	args := []interface{}{buildGenreLikePattern(genreName)}

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

// GetSongsByExactGenreWithYears returns songs that have the specified genre in their genre array,
// filtered to the specified year range. This is used by the AI DJ multi-genre blending to ensure
// decade-specific prompts like "90s hip hop" only return songs from that era.
// Uses COALESCE(original_year, year) to prefer original release year over remaster dates.
// Returns up to 50 random songs matching the criteria.
func (d *DB) GetSongsByExactGenreWithYears(genreName string, minYear, maxYear int) ([]Song, error) {
	// Build base query with genre matching
	// Use COALESCE to prefer original_year (for remasters) over embedded year
	query := `SELECT id, title, artist, album, genre, year, original_year, duration, file_path, cover_path, added_at, play_count, last_played 
			 FROM songs 
			 WHERE genre LIKE ? ESCAPE '\'`
	args := []interface{}{buildGenreLikePattern(genreName)}

	// Treat zero as unknown. Older enrichment runs may have written 0 rather
	// than NULL, and COALESCE(0, year) would otherwise make a valid song fail
	// every era query.
	if minYear > 0 {
		query += " AND COALESCE(NULLIF(original_year, 0), NULLIF(year, 0)) >= ?"
		args = append(args, minYear)
	}
	if maxYear > 0 {
		query += " AND COALESCE(NULLIF(original_year, 0), NULLIF(year, 0)) <= ?"
		args = append(args, maxYear)
	}

	query += " ORDER BY RANDOM() LIMIT 50"

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
		var originalYear sql.NullInt64

		err := rows.Scan(
			&s.ID, &s.Title, &s.Artist, &s.Album, &genreJSON, &s.Year, &originalYear, &s.Duration,
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
		if originalYear.Valid {
			s.OriginalYear = int(originalYear.Int64)
		}
		songs = append(songs, s)
	}
	return songs, rows.Err()
}

// GetSongsByExactGenreWithMood returns songs that have the specified genre in their genre array,
// filtered by mood/energy/tempo when provided. This combines exact genre matching with mood
// analysis for the AI DJ feature, enabling prompts like "upbeat jazz trios" to filter by
// both the specific genre and the mood characteristics.
// Uses JSON matching for exact genre names and optional mood/energy/tempo filters.
// Returns up to 50 random songs matching the criteria.
func (d *DB) GetSongsByExactGenreWithMood(genreName string, minYear, maxYear int, mood, energy, tempo string) ([]Song, error) {
	query := `SELECT id, title, artist, album, genre, year, original_year, duration, file_path, cover_path, added_at, play_count, last_played, mood, energy, tempo 
			 FROM songs 
			 WHERE genre LIKE ? ESCAPE '\'`
	args := []interface{}{buildGenreLikePattern(genreName)}

	// Treat zero as unknown; see GetSongsByExactGenreWithYears.
	if minYear > 0 {
		query += " AND COALESCE(NULLIF(original_year, 0), NULLIF(year, 0)) >= ?"
		args = append(args, minYear)
	}
	if maxYear > 0 {
		query += " AND COALESCE(NULLIF(original_year, 0), NULLIF(year, 0)) <= ?"
		args = append(args, maxYear)
	}

	// Add mood/energy/tempo filters when provided
	if mood != "" {
		query += " AND mood = ?"
		args = append(args, mood)
	}
	if energy != "" {
		query += " AND energy = ?"
		args = append(args, energy)
	}
	if tempo != "" {
		query += " AND tempo = ?"
		args = append(args, tempo)
	}

	query += " ORDER BY RANDOM() LIMIT 50"

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
		var originalYear sql.NullInt64
		var moodVal, energyVal, tempoVal sql.NullString

		err := rows.Scan(
			&s.ID, &s.Title, &s.Artist, &s.Album, &genreJSON, &s.Year, &originalYear, &s.Duration,
			&s.FilePath, &coverPath, &s.AddedAt, &s.PlayCount, &lastPlayed,
			&moodVal, &energyVal, &tempoVal,
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
		if originalYear.Valid {
			s.OriginalYear = int(originalYear.Int64)
		}
		if moodVal.Valid {
			s.Mood = moodVal.String
		}
		if energyVal.Valid {
			s.Energy = energyVal.String
		}
		if tempoVal.Valid {
			s.Tempo = tempoVal.String
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

// NormalizeAllGenres normalizes the capitalization of all genre names in the database.
// This is a one-time migration function that:
// 1. Fetches all songs with genre data
// 2. Normalizes each genre to consistent Title Case
// 3. Updates songs with normalized genres
// 4. Recalculates genre statistics
//
// Returns the number of songs updated and any error encountered.
func (d *DB) NormalizeAllGenres() (int, error) {
	// Fetch all songs with genres
	query := `SELECT id, genre FROM songs WHERE genre IS NOT NULL AND genre != '' AND genre != '[]'`
	rows, err := d.conn.Query(query)
	if err != nil {
		return 0, fmt.Errorf("failed to query songs: %w", err)
	}
	defer rows.Close()

	type songGenre struct {
		id     string
		genres []string
	}
	var songsToUpdate []songGenre

	for rows.Next() {
		var id string
		var genreJSON string
		if err := rows.Scan(&id, &genreJSON); err != nil {
			continue
		}

		var genres []string
		if err := json.Unmarshal([]byte(genreJSON), &genres); err != nil {
			continue
		}

		// Check if normalization would change anything
		normalized := NormalizeGenres(genres)
		normalizedJSON, _ := json.Marshal(normalized)
		if string(normalizedJSON) != genreJSON {
			songsToUpdate = append(songsToUpdate, songGenre{id: id, genres: normalized})
		}
	}

	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("error iterating songs: %w", err)
	}

	if len(songsToUpdate) == 0 {
		return 0, nil
	}

	// Update songs in a transaction
	tx, err := d.conn.Begin()
	if err != nil {
		return 0, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`UPDATE songs SET genre = ? WHERE id = ?`)
	if err != nil {
		return 0, fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()

	for _, sg := range songsToUpdate {
		genreJSON, _ := json.Marshal(sg.genres)
		if _, err := stmt.Exec(string(genreJSON), sg.id); err != nil {
			return 0, fmt.Errorf("failed to update song %s: %w", sg.id, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Recalculate genre stats after normalization
	if err := d.UpdateGenreStats(); err != nil {
		return len(songsToUpdate), fmt.Errorf("normalized %d songs but failed to update stats: %w", len(songsToUpdate), err)
	}

	return len(songsToUpdate), nil
}
