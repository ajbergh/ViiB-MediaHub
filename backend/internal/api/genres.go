package api

import (
	"encoding/json"
	"net/http"

	"github.com/ajbergh/viib-mediahub/internal/logger"
)

// getGenres handles GET /api/genres requests.
// Returns aggregated genre statistics from the genre_stats table including:
// - Genre name
// - Track count for each genre
// - Top 3 artists in each genre
// - Representative cover image URL (/api/cover/{songId})
//
// The genre_stats table is populated and maintained by UpdateGenreStats, which:
// - Parses individual genres from each song's genre JSON array
// - Counts each genre individually (a song with ["Rock", "Alternative"] adds +1 to both)
// - Aggregates artist popularity per genre
// - Selects cover images from the most popular artist in each genre
//
// Response is sorted by track count (most popular genres first).
func (a *API) getGenres(w http.ResponseWriter, r *http.Request) {
	stats, err := a.db.GetAllGenreStats()
	if err != nil {
		logger.API("Failed to get genre stats: %v", err)
		http.Error(w, "Failed to get genres", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

// normalizeGenres handles POST /api/genres/normalize requests.
// Normalizes the capitalization of all genre names in the database to ensure consistency.
// This fixes issues like "acid jazz" and "Acid Jazz" being treated as different genres.
//
// All genres are converted to Title Case with special handling for:
// - Acronyms: "r&b" → "R&B", "edm" → "EDM"
// - Hyphenated genres: "trip-hop" → "Trip-Hop"
// - Decade prefixes: "90s rock" → "90s Rock"
//
// Response includes the number of songs that were updated.
func (a *API) normalizeGenres(w http.ResponseWriter, r *http.Request) {
	logger.API("Starting genre normalization...")

	count, err := a.db.NormalizeAllGenres()
	if err != nil {
		logger.API("Genre normalization failed: %v", err)
		http.Error(w, "Failed to normalize genres: "+err.Error(), http.StatusInternalServerError)
		return
	}

	logger.API("Genre normalization complete: %d songs updated", count)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":       "ok",
		"songsUpdated": count,
		"message":      "Genre normalization complete",
	})
}
