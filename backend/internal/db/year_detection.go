package db

import (
	"database/sql"
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// RemasterPatterns are common patterns that indicate a remaster or re-release
var remasterPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\bremaster(?:ed)?\b`),
	regexp.MustCompile(`(?i)\bre-?master(?:ed)?\b`),
	regexp.MustCompile(`(?i)\bdeluxe(?:\s+edition)?\b`),
	regexp.MustCompile(`(?i)\bexpanded(?:\s+edition)?\b`),
	regexp.MustCompile(`(?i)\banniversary(?:\s+edition)?\b`),
	regexp.MustCompile(`(?i)\b\d+th\s+anniversary\b`),
	regexp.MustCompile(`(?i)\bsuper\s+deluxe\b`),
	regexp.MustCompile(`(?i)\bcomplete\s+edition\b`),
	regexp.MustCompile(`(?i)\bdefinitive\s+edition\b`),
	regexp.MustCompile(`(?i)\bultimate\s+edition\b`),
	regexp.MustCompile(`(?i)\bspecial\s+edition\b`),
	regexp.MustCompile(`(?i)\bcollector'?s?\s+edition\b`),
	regexp.MustCompile(`(?i)\bbonus\s+track\b`),
	regexp.MustCompile(`(?i)\bbonus\s+disc\b`),
	regexp.MustCompile(`(?i)\b20\d{2}\s+re-?issue\b`),
	regexp.MustCompile(`(?i)\bre-?issue\b`),
	regexp.MustCompile(`(?i)\bre-?release\b`),
}

// yearExtractPatterns match year references in album names
var yearExtractPatterns = []*regexp.Regexp{
	// Match "Originally Released: 1991" or "Original Release: 1991"
	regexp.MustCompile(`(?i)original(?:ly)?\s+release[d]?[:\s]+(\d{4})`),
	// Match "(1991)" at end of album name
	regexp.MustCompile(`\((\d{4})\)\s*$`),
	// Match "from 1991" or "circa 1991"
	regexp.MustCompile(`(?i)(?:from|circa)\s+(\d{4})`),
	// Match "25th Anniversary" patterns to extract decade
	regexp.MustCompile(`(\d+)(?:st|nd|rd|th)\s+anniversary`),
}

// DetectRemasterPattern checks if album or title indicates a remaster
func DetectRemasterPattern(title, album string) bool {
	combined := title + " " + album
	for _, pattern := range remasterPatterns {
		if pattern.MatchString(combined) {
			return true
		}
	}
	return false
}

// ExtractOriginalYearFromAlbum attempts to extract original year from album name
// Returns 0 if no year found
func ExtractOriginalYearFromAlbum(album string, currentYear int) int {
	for _, pattern := range yearExtractPatterns {
		matches := pattern.FindStringSubmatch(album)
		if len(matches) > 1 {
			// Check if it's an anniversary pattern (e.g., "25th Anniversary")
			if strings.Contains(strings.ToLower(album), "anniversary") && len(matches) > 1 {
				annYears, err := strconv.Atoi(matches[1])
				if err == nil && annYears > 0 && currentYear > annYears {
					// Calculate original year from anniversary
					originalYear := currentYear - annYears
					if originalYear >= 1900 && originalYear < currentYear {
						return originalYear
					}
				}
			}

			// Direct year extraction
			year, err := strconv.Atoi(matches[1])
			if err == nil && year >= 1900 && year <= time.Now().Year() {
				// Only return if it's different from current year
				if year != currentYear {
					return year
				}
			}
		}
	}
	return 0
}

// CheckYearArtistMismatch checks if the song year seems too recent for the artist
// This is a heuristic that marks songs as uncertain if the year seems off
func CheckYearArtistMismatch(year int) bool {
	// If song year is in the last 10 years but no streaming ID, might be a remaster
	currentYear := time.Now().Year()
	return year >= currentYear-10
}

// AnalyzeSongYear analyzes a song's year data and determines if it's likely a remaster
// Returns: originalYear (0 if unknown), isUncertain (true if needs AI analysis)
func AnalyzeSongYear(title, album string, year int) (originalYear int, isUncertain bool) {
	// Check for remaster patterns
	if DetectRemasterPattern(title, album) {
		isUncertain = true

		// Try to extract original year from album name
		extractedYear := ExtractOriginalYearFromAlbum(album, year)
		if extractedYear > 0 && extractedYear < year {
			return extractedYear, true // Found likely original year
		}
	}

	return 0, isUncertain
}

// DetectRemasterSongs scans songs and flags likely remasters
// Updates year_uncertain and original_year columns based on heuristics
func (db *DB) DetectRemasterSongs() (processed, flagged int, err error) {
	// Get all songs that haven't been analyzed yet
	rows, err := db.conn.Query(`
		SELECT id, title, album, year 
		FROM songs 
		WHERE year IS NOT NULL 
		AND year_analyzed_at IS NULL
		LIMIT 10000
	`)
	if err != nil {
		return 0, 0, err
	}
	defer rows.Close()

	type songToUpdate struct {
		id           string
		originalYear int
		uncertain    bool
	}

	var updates []songToUpdate

	for rows.Next() {
		var id, title, album string
		var year int
		if err := rows.Scan(&id, &title, &album, &year); err != nil {
			continue
		}

		originalYear, isUncertain := AnalyzeSongYear(title, album, year)

		if isUncertain || originalYear > 0 {
			updates = append(updates, songToUpdate{
				id:           id,
				originalYear: originalYear,
				uncertain:    isUncertain,
			})
		}

		processed++
	}

	// Apply updates in a transaction
	tx, err := db.conn.Begin()
	if err != nil {
		return processed, 0, err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		UPDATE songs 
		SET original_year = CASE WHEN ? > 0 THEN ? ELSE original_year END,
		    year_uncertain = ?,
		    year_analyzed_at = ?
		WHERE id = ?
	`)
	if err != nil {
		return processed, 0, err
	}
	defer stmt.Close()

	now := time.Now().Unix()
	for _, u := range updates {
		_, err := stmt.Exec(u.originalYear, u.originalYear, u.uncertain, now, u.id)
		if err != nil {
			continue
		}
		if u.uncertain {
			flagged++
		}
	}

	// Mark all processed songs as analyzed (even if no changes)
	_, err = tx.Exec(`
		UPDATE songs 
		SET year_analyzed_at = ? 
		WHERE year IS NOT NULL 
		AND year_analyzed_at IS NULL
	`, now)
	if err != nil {
		return processed, flagged, err
	}

	if err := tx.Commit(); err != nil {
		return processed, flagged, err
	}

	return processed, flagged, nil
}

// GetUncertainYearSongs returns songs that need AI year analysis
func (db *DB) GetUncertainYearSongs(limit int) ([]Song, error) {
	query := `
		SELECT id, title, artist, album, album_artist, track_number, disc_number,
		       genre, year, original_year, year_uncertain, year_analyzed_at,
		       duration, file_path, cover_path, added_at, play_count, last_played, skip_count, liked, liked_at
		FROM songs
		WHERE year_uncertain = 1
		AND original_year IS NULL
		ORDER BY play_count DESC
		LIMIT ?
	`

	rows, err := db.conn.Query(query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var songs []Song
	for rows.Next() {
		var s Song
		var genreJSON sql.NullString
		var coverPath sql.NullString
		var albumArtist sql.NullString
		var trackNumber, discNumber sql.NullInt64
		var year, originalYear sql.NullInt64
		var yearUncertain sql.NullBool
		var yearAnalyzedAt sql.NullInt64
		var playCount, lastPlayed, skipCount sql.NullInt64
		var liked sql.NullBool
		var likedAt sql.NullInt64

		err := rows.Scan(
			&s.ID, &s.Title, &s.Artist, &s.Album, &albumArtist, &trackNumber, &discNumber,
			&genreJSON, &year, &originalYear, &yearUncertain, &yearAnalyzedAt,
			&s.Duration, &s.FilePath, &coverPath, &s.AddedAt, &playCount, &lastPlayed, &skipCount, &liked, &likedAt,
		)
		if err != nil {
			return nil, err
		}

		if albumArtist.Valid {
			s.AlbumArtist = albumArtist.String
		}
		if trackNumber.Valid {
			s.TrackNumber = int(trackNumber.Int64)
		}
		if discNumber.Valid {
			s.DiscNumber = int(discNumber.Int64)
		}
		if genreJSON.Valid {
			json.Unmarshal([]byte(genreJSON.String), &s.Genre)
		}
		if year.Valid {
			s.Year = int(year.Int64)
		}
		if originalYear.Valid {
			s.OriginalYear = int(originalYear.Int64)
		}
		if yearUncertain.Valid {
			s.YearUncertain = yearUncertain.Bool
		}
		if yearAnalyzedAt.Valid {
			s.YearAnalyzedAt = yearAnalyzedAt.Int64
		}
		if coverPath.Valid {
			s.CoverPath = coverPath.String
		}
		if playCount.Valid {
			s.PlayCount = int(playCount.Int64)
		}
		if lastPlayed.Valid {
			s.LastPlayed = lastPlayed.Int64
		}
		if skipCount.Valid {
			s.SkipCount = int(skipCount.Int64)
		}
		if liked.Valid {
			s.Liked = liked.Bool
		}
		if likedAt.Valid {
			s.LikedAt = likedAt.Int64
		}

		songs = append(songs, s)
	}

	return songs, nil
}

// SetOriginalYear updates a song's original year (used after AI analysis)
func (db *DB) SetOriginalYear(songID string, originalYear int) error {
	_, err := db.conn.Exec(`
		UPDATE songs 
		SET original_year = ?,
		    year_uncertain = 0,
		    year_analyzed_at = ?
		WHERE id = ?
	`, originalYear, time.Now().Unix(), songID)
	return err
}
