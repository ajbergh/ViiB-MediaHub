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
