// Package api provides REST API handlers for ViiB MediaHub.
//
// This package implements the complete REST API including:
//   - Library management: songs, playlists, folders
//   - Likes: song and album favorites with persistence
//   - Media serving: audio streaming, cover art
//   - Spotify integration: OAuth, search proxy, downloads, streaming
//   - Last.FM integration: settings, testing, enrichment endpoints
//   - Metadata enrichment: AI or Last.FM based on user preference
//   - Metadata caching: album and artist enrichment
//   - Settings: key-value configuration storage
//   - SSE endpoints: real-time download progress and library events
//
// All endpoints return JSON responses with consistent error handling.
// Audio and cover files are served with appropriate caching headers.
package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/audio"
	"github.com/ajbergh/viib-mediahub/internal/db"
	"github.com/ajbergh/viib-mediahub/internal/dj"
	"github.com/ajbergh/viib-mediahub/internal/lastfm"
	"github.com/ajbergh/viib-mediahub/internal/llm"
	"github.com/ajbergh/viib-mediahub/internal/logger"
	"github.com/ajbergh/viib-mediahub/internal/mediafetch"
	"github.com/ajbergh/viib-mediahub/internal/scanner"
	"github.com/ajbergh/viib-mediahub/internal/semantic"
	"github.com/ajbergh/viib-mediahub/internal/validation"
	"github.com/ajbergh/viib-mediahub/internal/version"
	"github.com/go-chi/chi/v5"
)

// API provides the REST API handlers and dependencies for the ViiB MediaHub backend.
// It contains references to the database, server logger, and other shared
// components required by handler implementations.
type API struct {
	db                 *db.DB
	dataDir            string
	coverDir           string
	downloadManager    *DownloadManager
	scanner            *scanner.Scanner
	lastfmClient       *lastfm.Client
	enrichRunning      int32 // atomic: 1 if enrichment goroutine is active
	semanticMu         sync.RWMutex
	semanticService    *semantic.Service
	semanticState      semantic.EmbeddingResolution
	semanticError      string
	semanticClosed     bool
	semanticGeneration uint64
}

// New constructs a new API instance using the given database and
// download manager dependencies. The returned API implements handler
// methods that are later bound to HTTP routes.
func New(database *db.DB, dataDir string) *API {
	logger.API("New: Starting with dataDir=%s", dataDir)

	coverDir := filepath.Join(dataDir, "covers")
	os.MkdirAll(coverDir, 0700)

	// Get download directory from settings, or use default
	downloadDir := filepath.Join(dataDir, "spotify_downloads")
	if customPath, err := database.GetSetting("spotify_download_path"); err == nil && customPath != "" {
		downloadDir = customPath
		logger.API("Using custom Spotify download path: %s", downloadDir)
	}
	os.MkdirAll(downloadDir, 0700)

	// Create scanner
	logger.API("Creating scanner...")
	sc := scanner.New(database, dataDir)

	// Set Spotify download directory for auto-rescan feature
	sc.SetSpotifyDownloadDir(downloadDir)

	logger.API("Creating download manager...")
	// Create download manager - it will get access token from database when needed
	dm := NewDownloadManager(database, downloadDir)

	// Set scanner reference for download notifications
	dm.SetScanner(sc)

	logger.API("Starting download manager...")
	dm.Start()
	logger.API("Download manager started")

	api := &API{
		db:              database,
		dataDir:         dataDir,
		coverDir:        coverDir,
		downloadManager: dm,
		scanner:         sc,
	}

	// Initialize Last.FM client if configured
	api.initLastFMClient()

	// Trigger scan on startup (in background)
	go api.scanOnStartup()

	// Ensure genre stats are populated on startup
	go func() {
		if err := database.UpdateGenreStats(); err != nil {
			logger.API("Failed to update genre stats on startup: %v", err)
		}
	}()

	// Provider probing and background indexing must never delay API startup.
	go api.initSemanticService()

	return api
}

// Routes registers all API HTTP routes and middleware on the provided chi Router.
func (a *API) Routes() chi.Router {
	r := chi.NewRouter()

	// Library endpoints
	r.Get("/songs", a.getSongs)
	r.Get("/genres", a.getGenres)
	r.Post("/genres/normalize", a.normalizeGenres) // Normalize genre capitalization
	r.Post("/smart-playlist", a.handleGenerateSmartPlaylist)
	r.Delete("/songs", a.clearSongs)
	r.Post("/library/enrich-genres", a.enrichGenres)
	r.Get("/library/duplicates", a.getDuplicateGroups)
	r.Get("/library/duplicates/ignored", a.getIgnoredSongs)
	r.Post("/library/duplicates/ignore", a.setDuplicateIgnored)
	r.Get("/library/enrich-genres/stream", a.enrichGenresStream)       // SSE streaming enrichment
	r.Get("/library/enrich-all/stream", a.enrichAllMetadataStream)     // SSE unified enrichment (genres+mood+years)
	r.Post("/library/enrich-mood", a.enrichMood)                       // Mood/energy enrichment
	r.Get("/library/enrich-mood/stream", a.enrichMoodStream)           // SSE streaming mood enrichment
	r.Post("/library/backfill-years", a.backfillSongYears)             // Backfill song years from album metadata
	r.Post("/library/detect-remasters", a.detectRemasters)             // Heuristic remaster detection
	r.Get("/library/enrich-years/stream", a.enrichOriginalYearsStream) // SSE streaming year enrichment
	r.Post("/songs/{id}/play", a.recordPlay)
	r.Post("/songs/{id}/listen-event", a.recordListeningEvent) // AI DJ preference learning
	r.Patch("/songs/{id}/duration", a.updateSongDuration)      // Update song duration from actual audio

	// Likes endpoints
	r.Post("/songs/{id}/like", a.toggleLike)    // Toggle like status for a song
	r.Get("/songs/liked", a.getLikedSongIDs)    // Get all liked song IDs
	r.Post("/songs/like/bulk", a.bulkLikeSongs) // Bulk like/unlike songs (for albums)

	// Album likes endpoints
	r.Post("/albums/{albumKey}/like", a.toggleAlbumLike) // Toggle like status for an album
	r.Get("/albums/liked", a.getLikedAlbumKeys)          // Get all liked album keys
	r.Get("/albums/liked/full", a.getLikedAlbums)        // Get all liked albums with metadata

	// Playlist endpoints
	r.Get("/playlists", a.getPlaylists)
	r.Post("/playlists/import/m3u", a.importPlaylistM3U)
	r.Get("/playlists/{id}/export.m3u", a.exportPlaylistM3U)
	r.Post("/playlists", a.createPlaylist)
	r.Put("/playlists/{id}", a.updatePlaylist)
	r.Delete("/playlists/{id}", a.deletePlaylist)

	// Folder scanning
	r.Get("/folders", a.getScanFolders)
	r.Post("/folders", a.addScanFolder)
	r.Delete("/folders/{id}", a.removeScanFolder)
	r.Post("/scan", a.startScan)
	r.Post("/scan/quick", a.startQuickScan)
	r.Get("/scan/status", a.getScanStatus)

	// Semantic indexing endpoints
	r.Get("/semantic/settings", a.getSemanticSettings)
	r.Put("/semantic/settings", a.updateSemanticSettings)
	r.Get("/semantic/status", a.getSemanticStatus)
	r.Post("/semantic/rebuild", a.rebuildSemanticIndex)
	r.Post("/semantic/retry-errors", a.retrySemanticErrors)
	r.Post("/semantic/test-embedding-provider", a.testSemanticEmbeddingProvider)

	// Library events SSE
	r.Get("/library/events", a.libraryEventsSSE)

	// DJ Mode endpoints
	r.Get("/dj/personas", a.getDJPersonas)
	r.Get("/dj/waveform/{id}", a.getDJWaveform) // Get or generate waveform data
	r.Get("/dj/hotcues/{id}", a.getDJHotCues)   // Get hot cues for a track
	r.Put("/dj/hotcues/{id}", a.saveDJHotCues)  // Save hot cues for a track

	// File serving
	r.Get("/audio/*", a.serveAudio)
	r.Get("/cover/*", a.serveCover)

	// System
	r.Get("/health", a.healthCheck)
	r.Post("/browse", a.browseFolder)
	r.Get("/settings/{key}", a.getSetting)
	r.Post("/settings/{key}", a.setSetting)

	// Spotify
	r.Get("/spotify/credentials", a.getSpotifyCredentials)
	r.Post("/spotify/credentials", a.saveSpotifyCredentials)
	r.Get("/spotify/search", a.spotifySearch)
	r.Get("/spotify/search/playlists", a.spotifySearchPlaylists)            // Fallback search for first-party playlists
	r.Get("/spotify/playlists/{id}/scrape", a.spotifyGetPlaylistByScraping) // Get first-party playlist details via scraping
	r.Get("/spotify/me", a.spotifyGetUserProfile)
	r.Get("/spotify/proxy", a.spotifyProxy)
	r.Post("/spotify/proxy", a.spotifyProxy)
	r.Get("/spotify/auth/status", a.getSpotifyAuthStatus)
	r.Post("/spotify/auth/refresh", a.refreshSpotifyAuth)

	// Spotify Downloads
	r.Post("/spotify/download/track", a.downloadTrack)
	r.Post("/spotify/download/album", a.downloadAlbum)
	r.Post("/spotify/download/playlist", a.downloadPlaylist)
	r.Post("/spotify/download/url", a.downloadFromURL)
	r.Get("/spotify/downloads", a.getDownloads)
	r.Get("/spotify/downloads/active-count", a.getActiveDownloadCount)
	r.Get("/spotify/downloads/{id}", a.getDownloadStatus)
	r.Delete("/spotify/downloads/{id}", a.deleteDownload)
	r.Post("/spotify/downloads/{id}/retry", a.retryDownload)
	r.Post("/spotify/downloads/{id}/force-restart", a.forceRestartDownload)
	r.Delete("/spotify/downloads/completed", a.clearCompletedDownloads)
	r.Get("/spotify/downloads/events", a.downloadProgressSSE)

	// Spotify Streaming
	r.Get("/spotify/stream/{id}", a.streamSpotifyTrack)

	// Album metadata cache
	r.Get("/albums/metadata", a.getAllAlbumMetadata)
	r.Get("/albums/metadata/expired", a.getExpiredAlbumMetadata)
	r.Get("/albums/metadata/unchecked", a.getUncheckedAlbumMetadata)
	r.Get("/albums/metadata/{key}", a.getAlbumMetadata)
	r.Post("/albums/metadata", a.saveAlbumMetadata)
	r.Post("/albums/metadata/batch", a.batchGetAlbumMetadata)
	r.Post("/albums/metadata/download-cover", a.downloadAlbumCover)
	r.Delete("/albums/metadata/{key}", a.resetAlbumMetadata)

	// Artist metadata cache
	r.Get("/artists/metadata", a.getAllArtistMetadata)
	r.Get("/artists/metadata/unchecked", a.getUncheckedArtistMetadata)
	r.Get("/artists/metadata/{name}", a.getArtistMetadata)
	r.Post("/artists/metadata", a.saveArtistMetadata)
	r.Post("/artists/metadata/download-image", a.downloadArtistImage)
	r.Delete("/artists/metadata/{name}", a.resetArtistMetadata)

	// LLM (AI) configuration
	r.Get("/llm/settings", a.getLLMSettings)
	r.Put("/llm/settings", a.updateLLMSettings)
	r.Get("/llm/providers", a.getLLMProviders)
	r.Post("/llm/test", a.testLLMConnection)

	// Last.FM integration
	r.Get("/lastfm/settings", a.handleGetLastFMSettings)
	r.Post("/lastfm/settings", a.handleSaveLastFMSettings)
	r.Get("/lastfm/status", a.handleLastFMStatus)
	r.Post("/lastfm/test", a.handleTestLastFMConnection)
	r.Post("/lastfm/authenticate", a.handleLastFMAuthenticate)
	r.Post("/lastfm/enrich/songs", a.handleLastFMEnrichSongs)
	r.Post("/lastfm/enrich/artists", a.handleLastFMEnrichArtists)
	r.Get("/lastfm/track", a.handleGetTrackLastFM)
	r.Get("/lastfm/similar", a.handleGetSimilarTracks)

	// Frontend Logging (writes to viib.log)
	r.Post("/log", a.handleFrontendLog)

	return r
}

// Helper functions

func respondJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func respondError(w http.ResponseWriter, status int, message string) {
	clientMessage := message
	// For server errors, log the detail but return a generic message to the client
	if status >= 500 {
		logger.API("Internal error (HTTP %d): %s", status, message)
		clientMessage = "Internal server error"
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": clientMessage})
}

// Handlers

func (a *API) healthCheck(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, map[string]string{"status": "ok", "version": version.Current})
}

// FrontendLogRequest represents a log message from the frontend
type FrontendLogRequest struct {
	Level     string `json:"level"`     // "debug", "info", "warn", "error"
	Message   string `json:"message"`   // Log message
	Component string `json:"component"` // Component name (e.g., "DJMode", "DJJogWheel")
	Data      any    `json:"data"`      // Optional additional data
}

// handleFrontendLog accepts log messages from the frontend and writes them to viib.log
func (a *API) handleFrontendLog(w http.ResponseWriter, r *http.Request) {
	var req FrontendLogRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid log request")
		return
	}

	// Validate level
	level := strings.ToLower(req.Level)
	if level != "debug" && level != "info" && level != "warn" && level != "error" {
		level = "info"
	}

	// Format the log message
	component := req.Component
	if component == "" {
		component = "Frontend"
	}

	// Build full message with optional data
	fullMessage := fmt.Sprintf("[%s] %s", component, req.Message)
	if req.Data != nil {
		if dataBytes, err := json.Marshal(req.Data); err == nil {
			fullMessage = fmt.Sprintf("%s | Data: %s", fullMessage, string(dataBytes))
		}
	}

	// Log using appropriate level
	switch level {
	case "debug":
		logger.Debug("FE", "%s", fullMessage)
	case "info":
		logger.Log("FE", "%s", fullMessage)
	case "warn":
		logger.Log("FE WARN", "%s", fullMessage)
	case "error":
		logger.Log("FE ERROR", "%s", fullMessage)
	}

	respondJSON(w, map[string]string{"status": "logged"})
}

// getDJPersonas returns all available DJ personas for the DJ mode feature.
func (a *API) getDJPersonas(w http.ResponseWriter, r *http.Request) {
	personas := dj.ListPersonas()
	respondJSON(w, personas)
}

func (a *API) getSongs(w http.ResponseWriter, r *http.Request) {
	songs, err := a.db.GetAllSongs()
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Transform file paths to API URLs
	for i := range songs {
		transformLibrarySongForAPI(&songs[i])
	}

	if songs == nil {
		songs = []db.Song{}
	}

	respondJSON(w, songs)
}

func (a *API) clearSongs(w http.ResponseWriter, r *http.Request) {
	if err := a.db.ClearSongs(); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Clean up cover files
	os.RemoveAll(a.coverDir)
	os.MkdirAll(a.coverDir, 0700)

	respondJSON(w, map[string]string{"status": "ok"})
}

func (a *API) recordPlay(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := a.db.UpdatePlayCount(id); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, map[string]string{"status": "ok"})
}

// recordListeningEvent records a listening event for AI DJ preference learning.
// This endpoint is called when a song finishes playing or is skipped.
// POST /api/songs/{id}/listen-event
// Request body: {
//
//	"playDuration": 15.5,      // seconds played
//	"songDuration": 180.0,     // total duration
//	"context": "ai_dj"         // playback context
//
// }
// Event type is automatically determined:
//   - play_complete: playDuration >= 90% of songDuration
//   - skip_early: playDuration < 10s
//   - skip_mid: playDuration 10-30s
//   - skip_late: playDuration > 30s but < 90%
func (a *API) recordListeningEvent(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		respondError(w, http.StatusBadRequest, "Missing song ID")
		return
	}

	var body struct {
		PlayDuration float64 `json:"playDuration"`
		SongDuration float64 `json:"songDuration"`
		Context      string  `json:"context"` // 'ai_dj', 'album', 'playlist', 'queue', 'search'
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Get song metadata for genre/mood/energy
	song, err := a.db.GetSongByID(id)
	if err != nil {
		respondError(w, http.StatusNotFound, "Song not found")
		return
	}

	if body.PlayDuration < 0 || body.SongDuration <= 0 || body.PlayDuration > 7*24*60*60 || body.SongDuration > 7*24*60*60 {
		respondError(w, http.StatusBadRequest, "Invalid listening durations")
		return
	}
	if body.PlayDuration > body.SongDuration {
		body.PlayDuration = body.SongDuration
	}
	validContexts := map[string]bool{"ai_dj": true, "album": true, "playlist": true, "queue": true, "search": true, "spotify": true}
	if !validContexts[body.Context] {
		body.Context = "queue"
	}

	// Determine event type based on play duration
	var eventType string
	completionRatio := body.PlayDuration / body.SongDuration
	switch {
	case completionRatio >= 0.9:
		eventType = "play_complete"
	case body.PlayDuration < 10:
		eventType = "skip_early"
	case body.PlayDuration < 30:
		eventType = "skip_mid"
	default:
		eventType = "skip_late"
	}

	// Get primary genre (first in list) for preference tracking
	primaryGenre := ""
	if len(song.Genre) > 0 {
		primaryGenre = song.Genre[0]
	}

	// Record the event
	event := db.ListeningEvent{
		SongID:       id,
		EventType:    eventType,
		PlayDuration: body.PlayDuration,
		SongDuration: body.SongDuration,
		Genre:        primaryGenre,
		Mood:         song.Mood,
		Energy:       song.Energy,
		Context:      body.Context,
	}
	if err := a.db.RecordListeningEvent(event); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Update skip count if this was a skip
	if eventType != "play_complete" {
		if err := a.db.UpdateSkipCount(id); err != nil {
			// Log but don't fail the request
			log.Printf("Failed to update skip count for %s: %v", id, err)
		}
	}

	// Update genre preferences for all genres on this song
	for _, genre := range song.Genre {
		if err := a.db.UpdateGenrePreferences(genre); err != nil {
			// Log but don't fail the request
			log.Printf("Failed to update genre preferences for %s: %v", genre, err)
		}
	}

	respondJSON(w, map[string]interface{}{
		"status":    "ok",
		"eventType": eventType,
	})
}

// updateSongDuration updates the duration of a song from the actual audio playback.
// This fixes cases where metadata extraction reports incorrect duration.
// PATCH /api/songs/{id}/duration
// Request body: { "duration": 173.45 }
// Response: { "status": "ok" }
func (a *API) updateSongDuration(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		respondError(w, http.StatusBadRequest, "Missing song ID")
		return
	}

	var body struct {
		Duration float64 `json:"duration"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Only update if duration is positive and reasonable (under 24 hours)
	if body.Duration <= 0 || body.Duration > 86400 {
		respondError(w, http.StatusBadRequest, "Invalid duration value")
		return
	}

	if err := a.db.UpdateSongDuration(id, body.Duration); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSON(w, map[string]string{"status": "ok"})
}

// toggleLike toggles the liked status of a song.
// POST /api/songs/{id}/like
// Response: { "liked": true, "likedAt": 1735123456789 }
func (a *API) toggleLike(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		respondError(w, http.StatusBadRequest, "Missing song ID")
		return
	}

	liked, likedAt, err := a.db.ToggleLike(id)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSON(w, map[string]interface{}{
		"id":      id,
		"liked":   liked,
		"likedAt": likedAt,
	})
}

// getLikedSongIDs returns the IDs of all liked songs.
// GET /api/songs/liked
// Response: { "ids": ["songId1", "songId2", ...] }
func (a *API) getLikedSongIDs(w http.ResponseWriter, r *http.Request) {
	ids, err := a.db.GetLikedSongIDs()
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if ids == nil {
		ids = []string{}
	}

	respondJSON(w, map[string]interface{}{
		"ids": ids,
	})
}

// bulkLikeSongs sets the liked status for multiple songs at once.
// Useful for liking/unliking all songs in an album.
// POST /api/songs/like/bulk
// Request: { "songIds": ["id1", "id2"], "liked": true }
// Response: { "updated": 5 }
func (a *API) bulkLikeSongs(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SongIDs []string `json:"songIds"`
		Liked   bool     `json:"liked"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}

	if len(req.SongIDs) == 0 {
		respondError(w, http.StatusBadRequest, "No song IDs provided")
		return
	}

	updated, err := a.db.BulkSetLike(req.SongIDs, req.Liked)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSON(w, map[string]interface{}{
		"updated": updated,
	})
}

// Album Like Handlers

// toggleAlbumLike toggles the liked status of an album.
// POST /api/albums/{albumKey}/like
// Response: { "albumKey": "...", "liked": true, "likedAt": 1735123456789 }
func (a *API) toggleAlbumLike(w http.ResponseWriter, r *http.Request) {
	albumKey := chi.URLParam(r, "albumKey")
	if albumKey == "" {
		respondError(w, http.StatusBadRequest, "Missing album key")
		return
	}

	// URL-decode the album key since it comes from a URL path parameter
	decodedKey, err := url.PathUnescape(albumKey)
	if err != nil {
		respondError(w, http.StatusBadRequest, "Invalid album key encoding")
		return
	}

	liked, likedAt, err := a.db.ToggleAlbumLike(decodedKey)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSON(w, map[string]interface{}{
		"albumKey": decodedKey,
		"liked":    liked,
		"likedAt":  likedAt,
	})
}

// getLikedAlbumKeys returns the album_keys of all liked albums.
// GET /api/albums/liked
// Response: { "albumKeys": ["albumKey1", "albumKey2", ...] }
func (a *API) getLikedAlbumKeys(w http.ResponseWriter, r *http.Request) {
	keys, err := a.db.GetLikedAlbumKeys()
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if keys == nil {
		keys = []string{}
	}

	respondJSON(w, map[string]interface{}{
		"albumKeys": keys,
	})
}

// getLikedAlbums returns all liked albums with their metadata.
// GET /api/albums/liked/full
// Response: Array of AlbumMetadata objects
func (a *API) getLikedAlbums(w http.ResponseWriter, r *http.Request) {
	albums, err := a.db.GetLikedAlbums()
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if albums == nil {
		albums = []db.AlbumMetadata{}
	}

	respondJSON(w, albums)
}

func (a *API) getPlaylists(w http.ResponseWriter, r *http.Request) {
	playlists, err := a.db.GetAllPlaylists()
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if playlists == nil {
		playlists = []db.Playlist{}
	}

	respondJSON(w, playlists)
}

func (a *API) createPlaylist(w http.ResponseWriter, r *http.Request) {
	var p db.Playlist
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}

	p.ID = fmt.Sprintf("pl_%d", time.Now().UnixNano())
	p.CreatedAt = time.Now().UnixMilli()
	if p.SongIDs == nil {
		p.SongIDs = []string{}
	}

	if err := a.db.SavePlaylist(&p); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSON(w, p)
}

func (a *API) updatePlaylist(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var p db.Playlist
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}
	p.ID = id

	if err := a.db.SavePlaylist(&p); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSON(w, p)
}

func (a *API) deletePlaylist(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := a.db.DeletePlaylist(id); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, map[string]string{"status": "ok"})
}

func (a *API) getScanFolders(w http.ResponseWriter, r *http.Request) {
	folders, err := a.db.GetScanFolders()
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if folders == nil {
		folders = []db.ScanFolder{}
	}

	respondJSON(w, folders)
}

func (a *API) addScanFolder(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}

	// Validate path exists
	info, err := os.Stat(req.Path)
	if err != nil || !info.IsDir() {
		respondError(w, http.StatusBadRequest, "Invalid folder path")
		return
	}

	folder := &db.ScanFolder{
		ID:      fmt.Sprintf("folder_%d", time.Now().UnixNano()),
		Path:    req.Path,
		AddedAt: time.Now().UnixMilli(),
	}

	if err := a.db.AddScanFolder(folder); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSON(w, folder)
}

func (a *API) removeScanFolder(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := a.db.RemoveScanFolder(id); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, map[string]string{"status": "ok"})
}

func (a *API) startScan(w http.ResponseWriter, r *http.Request) {
	if a.scanner.IsScanning() {
		respondError(w, http.StatusConflict, "Scan already in progress")
		return
	}

	go func() {
		_, err := a.scanner.ScanAll()
		if err != nil {
			logger.API("Scan error: %v", err)
		}
	}()

	respondJSON(w, map[string]string{"status": "started"})
}

// startQuickScan performs a fast incremental scan using signatures and filesystem journals.
// Only processes changed/new/deleted files rather than rescanning the entire library.
func (a *API) startQuickScan(w http.ResponseWriter, r *http.Request) {
	if a.scanner.IsScanning() {
		respondError(w, http.StatusConflict, "Scan already in progress")
		return
	}

	// Set scanning state before starting goroutine
	a.scanner.SetScanning(true)
	a.scanner.SetProgress("Quick scan started...")

	go func() {
		// Ensure scanning is set to false when done
		defer func() {
			a.scanner.SetProgress("")
			a.scanner.SetScanning(false)
		}()

		// Emit scan started event
		a.scanner.EmitEvent(scanner.LibraryEvent{
			Type:    "scan_started",
			Message: "Quick scan started...",
		})

		logger.API("Starting quick scan...")
		quickResult, err := a.scanner.QuickStartup()
		if err != nil {
			logger.API("Quick scan failed: %v", err)
			a.scanner.EmitEvent(scanner.LibraryEvent{
				Type:    "scan_complete",
				Message: fmt.Sprintf("Quick scan failed: %v", err),
			})
			return
		}

		// Log results
		method := "signatures"
		if quickResult.UsedJournal {
			method = quickResult.JournalMethod
		}
		logger.API("Quick scan complete in %s using %s: %d dirs checked, %d unchanged, %d files changed",
			quickResult.ScanDuration, method, quickResult.CheckedDirs, quickResult.UnchangedDirs, len(quickResult.ChangedFiles))

		// Also detect deleted files (files in cache that no longer exist on disk)
		a.scanner.SetProgress("Checking for deleted files...")
		a.scanner.EmitEvent(scanner.LibraryEvent{
			Type:    "scan_progress",
			Message: "Checking for deleted files...",
		})
		deletedFiles, err := a.scanner.DetectDeletedFiles()
		if err != nil {
			logger.API("Error detecting deleted files: %v", err)
		} else if len(deletedFiles) > 0 {
			logger.API("Detected %d deleted files", len(deletedFiles))
			quickResult.ChangedFiles = append(quickResult.ChangedFiles, deletedFiles...)
		}

		// Process changes if any
		if len(quickResult.ChangedFiles) > 0 {
			msg := fmt.Sprintf("Processing %d changed files...", len(quickResult.ChangedFiles))
			a.scanner.SetProgress(msg)
			a.scanner.EmitEvent(scanner.LibraryEvent{
				Type:    "scan_progress",
				Message: msg,
			})

			result, err := a.scanner.ProcessChanges(quickResult.ChangedFiles)
			if err != nil {
				logger.API("Error processing changes: %v", err)
			} else {
				logger.API("Quick scan processed: %d added, %d updated, %d deleted",
					result.NewSongs, result.UpdatedSongs, result.RemovedSongs)
			}
		}

		// Emit completion event
		a.scanner.SetProgress("")
		a.scanner.EmitEvent(scanner.LibraryEvent{
			Type:    "scan_complete",
			Message: fmt.Sprintf("Quick scan complete: %d files changed", len(quickResult.ChangedFiles)),
		})

		// Notify UI to refresh
		a.scanner.EmitEvent(scanner.LibraryEvent{
			Type: "library_updated",
		})
	}()

	respondJSON(w, map[string]string{"status": "started", "type": "quick"})
}

func (a *API) getScanStatus(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, map[string]interface{}{
		"scanning": a.scanner.IsScanning(),
		"progress": a.scanner.GetProgress(),
	})
}

// libraryEventsSSE streams library events (scan complete, etc.) to the frontend via SSE
func (a *API) libraryEventsSSE(w http.ResponseWriter, r *http.Request) {
	logger.API("libraryEventsSSE: Connection started from client")
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	flusher, ok := w.(http.Flusher)
	if !ok {
		logger.API("libraryEventsSSE: Streaming not supported")
		respondError(w, http.StatusInternalServerError, "Streaming not supported")
		return
	}

	// Subscribe to library events
	eventChan := a.scanner.Subscribe()
	defer a.scanner.Unsubscribe(eventChan)

	logger.API("libraryEventsSSE: Subscribed to scanner events, entering event loop")
	for {
		select {
		case <-r.Context().Done():
			logger.API("libraryEventsSSE: Client disconnected")
			return
		case event, ok := <-eventChan:
			if !ok {
				logger.API("libraryEventsSSE: Event channel closed")
				return
			}
			// Log library_updated and important events
			if event.Type == "library_updated" || event.Type == "scan_started" || event.Type == "scan_complete" {
				logger.API("libraryEventsSSE: Sending event: %s - %s", event.Type, event.Message)
			}
			data, _ := json.Marshal(event)
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}
	}
}

// scanOnStartup triggers a library scan when the application starts.
// Uses the fast scan system (QuickStartup) to detect changes quickly,
// then processes any changes in the background. Falls back to full scan
// if quick startup detects many changes or is unavailable.
func (a *API) scanOnStartup() {
	folders, err := a.db.GetScanFolders()
	if err != nil {
		logger.API("Error getting scan folders for startup scan: %v", err)
		return
	}

	if len(folders) == 0 {
		logger.API("No folders configured, skipping startup scan")
		return
	}

	// Immediately report scanning so the frontend can show "Scanning..." on startup.
	// The actual scan work begins shortly after (startup delay helps the UI connect).
	a.scanner.SetScanning(true)
	a.scanner.SetProgress("Updating media library")
	defer func() {
		a.scanner.SetProgress("")
		a.scanner.SetScanning(false)
	}()

	a.scanner.EmitEvent(scanner.LibraryEvent{Type: "scan_started", Message: "Updating media library"})

	// Wait for application to fully initialize / UI to attach SSE.
	time.Sleep(3 * time.Second)

	// Try quick startup first (uses signatures and filesystem journals)
	logger.API("Starting quick startup scan...")
	quickResult, err := a.scanner.QuickStartup()
	if err != nil {
		logger.API("Quick startup failed: %v, falling back to full scan", err)
		// Fall back to full scan (which emits its own events)
		result, err := a.scanner.ScanAll()
		if err != nil {
			logger.API("Startup scan error: %v", err)
			return
		}
		logger.API("Full startup scan complete: %d files, %d new, %d updated (%s)",
			result.TotalFiles, result.NewSongs, result.UpdatedSongs, result.Duration)
		return
	}

	// Log quick startup results
	method := "signatures"
	if quickResult.UsedJournal {
		method = quickResult.JournalMethod
	}
	logger.API("Quick startup complete in %s using %s: %d dirs checked, %d unchanged, %d files changed",
		quickResult.ScanDuration, method, quickResult.CheckedDirs, quickResult.UnchangedDirs, len(quickResult.ChangedFiles))

	// Also detect deleted files (files in cache that no longer exist on disk)
	a.scanner.SetProgress("Updating media library")
	a.scanner.EmitEvent(scanner.LibraryEvent{Type: "scan_progress", Message: "Updating media library"})
	deletedFiles, err := a.scanner.DetectDeletedFiles()
	if err != nil {
		logger.API("Error detecting deleted files: %v", err)
	} else {
		logger.API("Detected %d deleted files", len(deletedFiles))
		if len(deletedFiles) > 0 {
			quickResult.ChangedFiles = append(quickResult.ChangedFiles, deletedFiles...)
		}
	}

	// If we detected changes, process them
	if len(quickResult.ChangedFiles) > 0 {
		// Update progress
		msg := fmt.Sprintf("Processing %d changed files...", len(quickResult.ChangedFiles))
		a.scanner.SetProgress(msg)
		a.scanner.EmitEvent(scanner.LibraryEvent{Type: "scan_progress", Message: msg})

		// Process all changes immediately during startup scan (don't use background queue)
		// This ensures the scan completes fully before we emit scan_complete
		logger.API("Processing %d changed files immediately...", len(quickResult.ChangedFiles))
		result, err := a.scanner.ProcessChanges(quickResult.ChangedFiles)
		if err != nil {
			logger.API("Error processing changes: %v", err)
		} else {
			logger.API("Processed changes: %d new, %d updated, %d removed",
				result.NewSongs, result.UpdatedSongs, result.RemovedSongs)

			// Emit scan complete with accurate counts
			a.scanner.SetProgress("")
			a.scanner.EmitEvent(scanner.LibraryEvent{
				Type:         "scan_complete",
				Message:      fmt.Sprintf("Startup scan complete: %d added, %d removed", result.NewSongs, result.RemovedSongs),
				NewSongs:     result.NewSongs,
				RemovedSongs: result.RemovedSongs,
			})

			// Notify UI to refresh
			a.scanner.EmitEvent(scanner.LibraryEvent{
				Type: "library_updated",
			})
			return
		}
	}

	// If quick startup required fallback or found too many changes, do a full scan
	if quickResult.FallbackFull {
		logger.API("Quick startup requested full scan fallback")
		result, err := a.scanner.ScanAll()
		if err != nil {
			logger.API("Fallback scan error: %v", err)
			// Emit scan complete even on error
			a.scanner.EmitEvent(scanner.LibraryEvent{
				Type:    "scan_complete",
				Message: "Scan failed",
			})
			return
		}
		logger.API("Fallback scan complete: %d files, %d new, %d updated (%s)",
			result.TotalFiles, result.NewSongs, result.UpdatedSongs, result.Duration)
		// ScanAll emits its own scan_complete event
		return
	}

	// Emit scan complete event for quick startup (no changes found)
	logger.API("Startup scan complete - no changes detected, emitting scan_complete event")
	a.scanner.SetProgress("")
	a.scanner.EmitEvent(scanner.LibraryEvent{
		Type:    "scan_complete",
		Message: "Library check complete - no changes detected",
	})
}

func (a *API) serveAudio(w http.ResponseWriter, r *http.Request) {
	// Get song ID from path
	songID := strings.TrimPrefix(r.URL.Path, "/api/audio/")

	// Get song from database by ID
	song, err := a.db.GetSongByID(songID)
	if err != nil {
		respondError(w, http.StatusNotFound, "Song not found")
		return
	}

	// Verify file exists
	if _, err := os.Stat(song.FilePath); os.IsNotExist(err) {
		respondError(w, http.StatusNotFound, "Audio file not found on disk")
		return
	}

	// Serve the actual file with range support for seeking
	http.ServeFile(w, r, song.FilePath)
}

func (a *API) serveCover(w http.ResponseWriter, r *http.Request) {
	pathOrID := strings.TrimPrefix(r.URL.Path, "/api/cover/")

	// URL-decode the path (handles %5C for backslashes, %3A for colons, etc.)
	decodedPath, err := url.PathUnescape(pathOrID)
	if err != nil {
		logger.API("Failed to URL-decode cover path: %s, error: %v", pathOrID, err)
		decodedPath = pathOrID // Fall back to original if decode fails
	}
	pathOrID = decodedPath

	// Check if the path looks like an absolute file path (for album/artist covers)
	// Windows paths start with a drive letter (e.g., C:) or UNC (\\)
	// Unix paths start with /
	isAbsolutePath := strings.HasPrefix(pathOrID, "/") ||
		(len(pathOrID) > 1 && pathOrID[1] == ':') ||
		strings.HasPrefix(pathOrID, "\\\\")

	if isAbsolutePath {
		// Normalize path separators for comparison
		normalizedPath := filepath.Clean(pathOrID)
		normalizedCoverDir := filepath.Clean(a.coverDir)

		// Security check: verify path is within allowed directories
		// 1. Check if within covers directory (for downloaded Spotify images)
		// 2. Check if within configured scan folders (for embedded album art)
		// NOTE: Append path separator to prevent prefix bypass (e.g., "covers-evil" matching "covers")
		isAllowed := strings.HasPrefix(strings.ToLower(normalizedPath), strings.ToLower(normalizedCoverDir)+string(filepath.Separator)) ||
			strings.EqualFold(normalizedPath, normalizedCoverDir)

		if !isAllowed {
			// Check if path is within any configured scan folder
			scanFolders, _ := a.db.GetScanFolders()
			for _, folder := range scanFolders {
				normalizedFolder := filepath.Clean(folder.Path)
				if strings.HasPrefix(strings.ToLower(normalizedPath), strings.ToLower(normalizedFolder)+string(filepath.Separator)) ||
					strings.EqualFold(normalizedPath, normalizedFolder) {
					isAllowed = true
					break
				}
			}
		}

		if isAllowed {
			if _, err := os.Stat(normalizedPath); err == nil {
				http.ServeFile(w, r, normalizedPath)
				return
			}
			logger.API("Cover file not found at path: %s", normalizedPath)
		} else {
			logger.API("Cover path security check failed - path %s not in allowed directories", normalizedPath)
		}
	}

	// Try as song ID - get the song to find its cover path
	song, err := a.db.GetSongByID(pathOrID)
	if err == nil && song.CoverPath != "" {
		// Song has a cover path set - serve that file
		if _, err := os.Stat(song.CoverPath); err == nil {
			http.ServeFile(w, r, song.CoverPath)
			return
		}
	}

	// Fallback: try legacy per-song cover file
	coverPath := filepath.Join(a.coverDir, pathOrID+".jpg")
	if _, err := os.Stat(coverPath); err == nil {
		http.ServeFile(w, r, coverPath)
		return
	}

	logger.API("Cover not found for: %s (isAbsPath=%v)", pathOrID, isAbsolutePath)
	respondError(w, http.StatusNotFound, "Cover not found")
}

func (a *API) browseFolder(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		req.Path = ""
	}

	// Default to user's home directory or common music folders
	if req.Path == "" {
		home, _ := os.UserHomeDir()
		req.Path = home
	}

	// FolderEntry represents a single folder entry returned by the browse API
	// used for selecting scan folders through the filesystem browser.
	type FolderEntry struct {
		Name  string `json:"name"`
		Path  string `json:"path"`
		IsDir bool   `json:"isDir"`
	}

	var result []FolderEntry

	// Check if we need to list available drives on Windows
	// This happens when path is "drives" or when on root and requesting drive list
	if req.Path == "drives" || req.Path == "/" || req.Path == "\\" {
		// List available drives on Windows
		if runtime.GOOS == "windows" {
			for drive := 'A'; drive <= 'Z'; drive++ {
				drivePath := string(drive) + ":"
				// Check if the drive is accessible
				if _, err := os.Stat(drivePath); err == nil {
					result = append(result, FolderEntry{
						Name:  drivePath,
						Path:  drivePath + "\\",
						IsDir: true,
					})
				}
			}
			respondJSON(w, map[string]interface{}{
				"currentPath": "Drives",
				"entries":     result,
			})
			return
		}
		// On non-Windows systems, default to home directory
		home, _ := os.UserHomeDir()
		req.Path = home
	}

	entries, err := os.ReadDir(req.Path)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Add parent directory (or drive list on Windows if at root)
	parent := filepath.Dir(req.Path)
	if parent != req.Path {
		// Check if parent is root on Windows
		if runtime.GOOS == "windows" && (parent == "\\" || parent == "/") {
			// Going up from root shows drive list
			result = append(result, FolderEntry{
				Name:  "Drives",
				Path:  "drives",
				IsDir: true,
			})
		} else {
			result = append(result, FolderEntry{
				Name:  "..",
				Path:  parent,
				IsDir: true,
			})
		}
	}

	for _, entry := range entries {
		// Only show directories for folder browsing
		if entry.IsDir() {
			result = append(result, FolderEntry{
				Name:  entry.Name(),
				Path:  filepath.Join(req.Path, entry.Name()),
				IsDir: true,
			})
		}
	}

	respondJSON(w, map[string]interface{}{
		"currentPath": req.Path,
		"entries":     result,
	})
}

// File upload for importing
//
//lint:ignore U1000 Retained for the planned local-file import endpoint.
func (a *API) uploadSong(w http.ResponseWriter, r *http.Request) {
	r.ParseMultipartForm(100 << 20) // 100MB max

	file, header, err := r.FormFile("file")
	if err != nil {
		respondError(w, http.StatusBadRequest, "No file provided")
		return
	}
	defer file.Close()

	// Save to temp location
	tempDir := filepath.Join(a.dataDir, "uploads")
	os.MkdirAll(tempDir, 0700)

	tempPath := filepath.Join(tempDir, header.Filename)
	dst, err := os.Create(tempPath)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer dst.Close()

	if _, err := io.Copy(dst, file); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Extract metadata and add to library
	song, err := audio.ExtractMetadata(tempPath)
	if err != nil {
		os.Remove(tempPath)
		respondError(w, http.StatusBadRequest, "Failed to extract metadata")
		return
	}

	dbSong := db.Song{
		ID:          song.ID,
		Title:       song.Title,
		Artist:      song.Artist,
		Album:       song.Album,
		AlbumArtist: song.AlbumArtist,
		TrackNumber: song.TrackNumber,
		DiscNumber:  song.DiscNumber,
		Genre:       song.Genre,
		Year:        song.Year,
		Duration:    song.Duration,
		FilePath:    tempPath,
		AddedAt:     time.Now().UnixMilli(),
	}

	if err := a.db.SaveSong(&dbSong); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSON(w, dbSong)
}

// Settings handlers

func (a *API) getSetting(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")
	if key == "" {
		respondError(w, http.StatusBadRequest, "Setting key is required")
		return
	}

	if !validation.IsValidSettingKey(key) {
		respondError(w, http.StatusBadRequest, "Invalid setting key")
		return
	}

	if validation.IsSensitiveSettingKey(key) {
		value, err := a.db.GetSetting(key)
		if err != nil {
			respondError(w, http.StatusInternalServerError, "Failed to get setting")
			return
		}
		respondJSON(w, map[string]interface{}{"key": key, "value": "", "configured": value != ""})
		return
	}

	value, err := a.db.GetSetting(key)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to get setting")
		return
	}

	respondJSON(w, map[string]string{"key": key, "value": value})
}

func (a *API) setSetting(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")
	if key == "" {
		respondError(w, http.StatusBadRequest, "Setting key is required")
		return
	}

	if !validation.IsValidSettingKey(key) {
		respondError(w, http.StatusBadRequest, "Invalid setting key")
		return
	}

	var body struct {
		Value string `json:"value"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Special handling for concurrent_downloads - validate and update download manager
	if key == "concurrent_downloads" {
		n, err := strconv.Atoi(body.Value)
		if err != nil {
			respondError(w, http.StatusBadRequest, "Invalid value for concurrent_downloads")
			return
		}
		if n < MinConcurrentDownloads || n > MaxConcurrentDownloads {
			respondError(w, http.StatusBadRequest, fmt.Sprintf("concurrent_downloads must be between %d and %d", MinConcurrentDownloads, MaxConcurrentDownloads))
			return
		}
		if a.downloadManager != nil {
			if err := a.downloadManager.SetMaxConcurrent(n); err != nil {
				respondError(w, http.StatusInternalServerError, err.Error())
				return
			}
		}
	}

	// Conversion workers are independent from download slots and may be changed
	// while conversions are queued or running.
	if key == "spotify_conversion_workers" {
		n, err := strconv.Atoi(body.Value)
		if err != nil {
			respondError(w, http.StatusBadRequest, "Invalid value for spotify_conversion_workers")
			return
		}
		if n < MinConversionWorkers || n > MaxConversionWorkers {
			respondError(w, http.StatusBadRequest, fmt.Sprintf("spotify_conversion_workers must be between %d and %d", MinConversionWorkers, MaxConversionWorkers))
			return
		}
		if a.downloadManager != nil {
			if err := a.downloadManager.SetMaxConversionWorkers(n); err != nil {
				respondError(w, http.StatusInternalServerError, err.Error())
				return
			}
		}
	}

	// Special handling for spotify_download_path - update download manager and scanner
	if key == "spotify_download_path" {
		downloadDir := body.Value
		if downloadDir == "" {
			// Use default if empty
			downloadDir = filepath.Join(a.dataDir, "spotify_downloads")
		}
		// Create the directory if it doesn't exist
		if err := os.MkdirAll(downloadDir, 0700); err != nil {
			respondError(w, http.StatusBadRequest, fmt.Sprintf("Failed to create download directory: %v", err))
			return
		}
		// Update the download manager
		if a.downloadManager != nil {
			if err := a.downloadManager.SetDownloadDir(downloadDir); err != nil {
				respondError(w, http.StatusInternalServerError, err.Error())
				return
			}
		}
		logger.API("Updated Spotify download path to: %s", downloadDir)
	}

	if err := a.db.SetSetting(key, body.Value); err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to save setting")
		return
	}

	respondJSON(w, map[string]string{"status": "ok"})
}

// Album metadata handlers

func (a *API) getAllAlbumMetadata(w http.ResponseWriter, r *http.Request) {
	metadata, err := a.db.GetAllAlbumMetadata()
	if err != nil {
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to get album metadata: %v", err))
		return
	}
	if metadata == nil {
		metadata = []db.AlbumMetadata{}
	}
	respondJSON(w, metadata)
}

func (a *API) getAlbumMetadata(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")
	if key == "" {
		respondError(w, http.StatusBadRequest, "Album key is required")
		return
	}

	metadata, err := a.db.GetAlbumMetadata(key)
	if err != nil {
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to get album metadata: %v", err))
		return
	}
	if metadata == nil {
		respondError(w, http.StatusNotFound, "Album metadata not found")
		return
	}
	respondJSON(w, metadata)
}

func (a *API) saveAlbumMetadata(w http.ResponseWriter, r *http.Request) {
	var m db.AlbumMetadata
	if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if m.AlbumKey == "" || m.AlbumName == "" || m.ArtistName == "" {
		respondError(w, http.StatusBadRequest, "albumKey, albumName, and artistName are required")
		return
	}

	if err := a.db.SaveAlbumMetadata(&m); err != nil {
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to save album metadata: %v", err))
		return
	}

	respondJSON(w, map[string]string{"status": "ok"})
}

// downloadAlbumCover downloads album artwork from a URL and saves it as cover.jpg
// in the album's folder (determined from the first song in that album)
func (a *API) downloadAlbumCover(w http.ResponseWriter, r *http.Request) {
	var req struct {
		AlbumKey string `json:"albumKey"`
		ImageURL string `json:"imageUrl"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	parts := strings.Split(req.AlbumKey, "::")
	if len(parts) != 2 || req.ImageURL == "" {
		respondError(w, http.StatusBadRequest, "albumKey must be album::artist and imageUrl is required")
		return
	}
	albumName, artistName := parts[0], parts[1]
	songs, err := a.db.GetAllSongs()
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to get songs")
		return
	}
	var albumFolder string
	for _, song := range songs {
		songArtist := song.AlbumArtist
		if songArtist == "" {
			songArtist = song.Artist
		}
		if song.Album == albumName && songArtist == artistName {
			albumFolder = filepath.Dir(song.FilePath)
			break
		}
	}
	if albumFolder == "" {
		respondError(w, http.StatusNotFound, "No songs found for this album and artist")
		return
	}
	data, _, err := mediafetch.FetchImage(r.Context(), req.ImageURL, mediafetch.DefaultMaxBytes)
	if err != nil {
		respondError(w, http.StatusBadRequest, fmt.Sprintf("Failed to download artwork: %v", err))
		return
	}
	coverPath := filepath.Join(albumFolder, "cover.jpg")
	if err := os.WriteFile(coverPath, data, 0600); err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to save cover")
		return
	}
	if err := a.db.UpdateAlbumLocalCover(req.AlbumKey, coverPath); err != nil {
		logger.API("Warning: Failed to update album local cover in database: %v", err)
	}
	respondJSON(w, map[string]interface{}{"status": "ok", "coverPath": coverPath})
}

// resetAlbumMetadata resets the spotify_checked flag for an album to force re-fetch
func (a *API) resetAlbumMetadata(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")
	if key == "" {
		respondError(w, http.StatusBadRequest, "Album key is required")
		return
	}

	if err := a.db.ResetAlbumSpotifyCheck(key); err != nil {
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to reset album metadata: %v", err))
		return
	}

	respondJSON(w, map[string]string{"status": "ok"})
}

// batchGetAlbumMetadata returns metadata for multiple albums at once
func (a *API) batchGetAlbumMetadata(w http.ResponseWriter, r *http.Request) {
	var req struct {
		AlbumKeys []string `json:"albumKeys"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if len(req.AlbumKeys) == 0 {
		respondJSON(w, []db.AlbumMetadata{})
		return
	}

	// Limit batch size to prevent abuse
	if len(req.AlbumKeys) > 100 {
		respondError(w, http.StatusBadRequest, "Maximum 100 album keys per batch request")
		return
	}

	metadata, err := a.db.GetAlbumMetadataBatch(req.AlbumKeys)
	if err != nil {
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to get album metadata: %v", err))
		return
	}

	if metadata == nil {
		metadata = []db.AlbumMetadata{}
	}

	respondJSON(w, metadata)
}

// getExpiredAlbumMetadata returns albums that were checked more than 30 days ago and not found
func (a *API) getExpiredAlbumMetadata(w http.ResponseWriter, r *http.Request) {
	metadata, err := a.db.GetExpiredAlbumMetadata(30) // 30 days expiration
	if err != nil {
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to get expired album metadata: %v", err))
		return
	}

	if metadata == nil {
		metadata = []db.AlbumMetadata{}
	}

	respondJSON(w, metadata)
}

// getUncheckedAlbumMetadata returns albums that haven't been checked yet
func (a *API) getUncheckedAlbumMetadata(w http.ResponseWriter, r *http.Request) {
	metadata, err := a.db.GetAlbumsNeedingMetadata()
	if err != nil {
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to get unchecked album metadata: %v", err))
		return
	}

	if metadata == nil {
		metadata = []db.AlbumMetadata{}
	}

	respondJSON(w, metadata)
}

// Artist metadata handlers

func (a *API) getAllArtistMetadata(w http.ResponseWriter, r *http.Request) {
	metadata, err := a.db.GetAllArtistMetadata()
	if err != nil {
		logger.API("getAllArtistMetadata: Error fetching metadata: %v", err)
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to get artist metadata: %v", err))
		return
	}
	if metadata == nil {
		metadata = []db.ArtistMetadata{}
	}

	// Count how many have local images
	withLocalImage := 0
	for _, m := range metadata {
		if m.LocalImagePath != "" {
			withLocalImage++
		}
	}
	logger.APIDebug("getAllArtistMetadata: Returning %d artists (%d with local images)", len(metadata), withLocalImage)

	respondJSON(w, metadata)
}

func (a *API) getArtistMetadata(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if name == "" {
		respondError(w, http.StatusBadRequest, "Artist name is required")
		return
	}

	metadata, err := a.db.GetArtistMetadata(name)
	if err != nil {
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to get artist metadata: %v", err))
		return
	}
	if metadata == nil {
		respondError(w, http.StatusNotFound, "Artist metadata not found")
		return
	}
	respondJSON(w, metadata)
}

func (a *API) saveArtistMetadata(w http.ResponseWriter, r *http.Request) {
	var m db.ArtistMetadata
	if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if m.ArtistName == "" {
		respondError(w, http.StatusBadRequest, "artistName is required")
		return
	}

	if err := a.db.SaveArtistMetadata(&m); err != nil {
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to save artist metadata: %v", err))
		return
	}

	respondJSON(w, map[string]string{"status": "ok"})
}

func (a *API) resetArtistMetadata(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if name == "" {
		respondError(w, http.StatusBadRequest, "Artist name is required")
		return
	}

	if err := a.db.ResetArtistSpotifyCheck(name); err != nil {
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to reset artist metadata: %v", err))
		return
	}

	respondJSON(w, map[string]string{"status": "ok"})
}

func (a *API) getUncheckedArtistMetadata(w http.ResponseWriter, r *http.Request) {
	metadata, err := a.db.GetArtistsNeedingMetadata()
	if err != nil {
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to get unchecked artist metadata: %v", err))
		return
	}

	if metadata == nil {
		metadata = []db.ArtistMetadata{}
	}

	respondJSON(w, metadata)
}

// downloadArtistImage downloads artist image from URL and saves it locally
func (a *API) downloadArtistImage(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ArtistName string `json:"artistName"`
		ImageURL   string `json:"imageUrl"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ArtistName == "" || req.ImageURL == "" {
		respondError(w, http.StatusBadRequest, "artistName and imageUrl are required")
		return
	}
	data, _, err := mediafetch.FetchImage(r.Context(), req.ImageURL, mediafetch.DefaultMaxBytes)
	if err != nil {
		respondError(w, http.StatusBadRequest, fmt.Sprintf("Failed to download artwork: %v", err))
		return
	}
	artistImagesDir := filepath.Join(a.coverDir, "artists")
	if err := os.MkdirAll(artistImagesDir, 0700); err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to create artist image directory")
		return
	}
	safeArtistName := strings.NewReplacer("/", "_", "\\", "_", ":", "_").Replace(req.ArtistName)
	imagePath := filepath.Join(artistImagesDir, safeArtistName+".jpg")
	if err := os.WriteFile(imagePath, data, 0600); err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to save artist image")
		return
	}
	if err := a.db.UpdateArtistLocalImage(req.ArtistName, imagePath); err != nil {
		logger.API("Failed to update artist image for %q: %v", req.ArtistName, err)
	}
	respondJSON(w, map[string]interface{}{"status": "ok", "imagePath": imagePath})
}

type enrichGenresRequest struct {
	APIKey string `json:"apiKey"`
	Force  bool   `json:"force"`
	Offset int    `json:"offset"`
}

func (a *API) enrichGenres(w http.ResponseWriter, r *http.Request) {
	var req enrichGenresRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		// It's okay if body is empty, we might have key in settings
	}

	// If API key provided, save it as legacy gemini_api_key for backward compatibility
	if req.APIKey != "" {
		a.db.SetSetting("gemini_api_key", req.APIKey)
	}

	// Get LLM provider (checks unified settings first, then legacy gemini_api_key)
	provider, err := llm.GetConfiguredProvider(a.db)
	if err != nil {
		respondError(w, http.StatusBadRequest, "No LLM configured. Set AI Provider in Settings or provide a Gemini API key.")
		return
	}
	defer provider.Close()

	// Get songs for enrichment
	// Limit to provider's optimal batch size to avoid hitting token limits
	batchSize := provider.GetOptimalBatchSize()
	if batchSize > 50 {
		batchSize = 50 // Cap at 50 for this sync endpoint
	}
	songs, err := a.db.GetSongsForEnrichment(batchSize, req.Force, req.Offset)
	if err != nil {
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to get songs: %v", err))
		return
	}

	if len(songs) == 0 {
		respondJSON(w, map[string]interface{}{
			"status":  "ok",
			"message": "No songs found for enrichment",
			"count":   0,
		})
		return
	}

	genresMap, err := provider.EnrichGenres(r.Context(), songs)
	if err != nil {
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("LLM error: %v", err))
		return
	}

	updatedCount := 0
	for id, genres := range genresMap {
		if err := a.db.UpdateSongGenres(id, genres); err != nil {
			logger.API("Failed to update genres for song %s: %v", id, err)
			continue
		}
		updatedCount++
	}

	respondJSON(w, map[string]interface{}{
		"status":  "ok",
		"message": fmt.Sprintf("Successfully enriched %d songs using %s", updatedCount, provider.GetProviderName()),
		"count":   updatedCount,
	})
}

// EnrichmentProgress represents a progress update during genre enrichment
type EnrichmentProgress struct {
	Status         string `json:"status"` // "started", "processing", "batch_complete", "complete", "error"
	Message        string `json:"message"`
	TotalSongs     int    `json:"totalSongs"`
	ProcessedSongs int    `json:"processedSongs"`
	ChangedSongs   int    `json:"changedSongs,omitempty"`
	EmptyResults   int    `json:"emptyResults,omitempty"`
	CurrentBatch   int    `json:"currentBatch"`
	TotalBatches   int    `json:"totalBatches"`
	Error          string `json:"error,omitempty"`
}

// enrichGenresStream provides SSE streaming for genre enrichment progress
func (a *API) enrichGenresStream(w http.ResponseWriter, r *http.Request) {
	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	// Get parameters from query string
	force := r.URL.Query().Get("force") == "true"

	// Helper to send SSE event
	sendEvent := func(progress EnrichmentProgress) {
		data, _ := json.Marshal(progress)
		fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()
	}

	// Helper to send SSE keepalive comment (invisible to EventSource.onmessage)
	sendKeepalive := func() {
		fmt.Fprintf(w, ": keepalive %d\n\n", time.Now().Unix())
		flusher.Flush()
	}

	// Get LLM provider (checks unified settings first, then legacy gemini_api_key)
	provider, err := llm.GetConfiguredProvider(a.db)
	if err != nil {
		sendEvent(EnrichmentProgress{
			Status:  "error",
			Message: "No LLM configured",
			Error:   "Configure AI Provider in Settings or add a Gemini API key.",
		})
		return
	}
	defer provider.Close()

	// Get total songs needing enrichment
	allSongs, err := a.db.GetSongsForEnrichment(10000, force, 0) // Get all for counting
	if err != nil {
		sendEvent(EnrichmentProgress{
			Status:  "error",
			Message: "Failed to query songs",
			Error:   err.Error(),
		})
		return
	}

	totalSongs := len(allSongs)
	if totalSongs == 0 {
		sendEvent(EnrichmentProgress{
			Status:     "complete",
			Message:    "No songs need genre enrichment",
			TotalSongs: 0,
		})
		return
	}

	// Calculate batches - use provider's optimal batch size
	batchSize := provider.GetOptimalBatchSize()
	totalBatches := (totalSongs + batchSize - 1) / batchSize

	sendEvent(EnrichmentProgress{
		Status:       "started",
		Message:      fmt.Sprintf("Starting enrichment of %d songs in %d batches using %s", totalSongs, totalBatches, provider.GetProviderName()),
		TotalSongs:   totalSongs,
		TotalBatches: totalBatches,
	})

	processedSongs := 0
	updatedTotal := 0

	// Process in batches
	for batch := 0; batch < totalBatches; batch++ {
		// Check if client disconnected
		select {
		case <-r.Context().Done():
			logger.API("enrichGenresStream: Client disconnected")
			return
		default:
		}

		offset := batch * batchSize
		songs, err := a.db.GetSongsForEnrichment(batchSize, force, offset)
		if err != nil {
			sendEvent(EnrichmentProgress{
				Status:         "error",
				Message:        "Failed to get song batch",
				Error:          err.Error(),
				ProcessedSongs: processedSongs,
				CurrentBatch:   batch + 1,
				TotalBatches:   totalBatches,
			})
			return
		}

		if len(songs) == 0 {
			break
		}

		sendEvent(EnrichmentProgress{
			Status:         "processing",
			Message:        fmt.Sprintf("Processing batch %d of %d (%d songs)", batch+1, totalBatches, len(songs)),
			TotalSongs:     totalSongs,
			ProcessedSongs: processedSongs,
			CurrentBatch:   batch + 1,
			TotalBatches:   totalBatches,
		})

		logger.API("enrichGenresStream: Calling EnrichGenres for batch %d with %d songs", batch+1, len(songs))

		// Create a channel to signal when LLM call completes
		llmDone := make(chan struct{})
		var genresMap map[string][]string
		var llmErr error

		// Run LLM call in goroutine
		go func() {
			genresMap, llmErr = provider.EnrichGenres(r.Context(), songs)
			close(llmDone)
		}()

		// Send keepalive events every 5 seconds while waiting for LLM
		keepaliveTicker := time.NewTicker(5 * time.Second)
		defer keepaliveTicker.Stop()

	llmWait:
		for {
			select {
			case <-r.Context().Done():
				logger.API("enrichGenresStream: Client disconnected during LLM call")
				return
			case <-llmDone:
				break llmWait
			case <-keepaliveTicker.C:
				// Send SSE keepalive comment to prevent browser timeout
				logger.API("enrichGenresStream: Sending keepalive for batch %d", batch+1)
				sendKeepalive()
			}
		}
		keepaliveTicker.Stop()

		logger.API("enrichGenresStream: EnrichGenres returned, err=%v, resultCount=%d", llmErr, len(genresMap))

		if llmErr != nil {
			sendEvent(EnrichmentProgress{
				Status:         "error",
				Message:        "LLM API error",
				Error:          llmErr.Error(),
				ProcessedSongs: processedSongs,
				CurrentBatch:   batch + 1,
				TotalBatches:   totalBatches,
			})
			return
		}

		// Update songs in database
		logger.API("enrichGenresStream: Updating %d songs in database", len(genresMap))
		batchUpdated := 0
		for id, genres := range genresMap {
			if err := a.db.UpdateSongGenres(id, genres); err != nil {
				logger.API("Failed to update genres for song %s: %v", id, err)
				continue
			}
			batchUpdated++
		}
		logger.API("enrichGenresStream: Updated %d songs, sending batch_complete", batchUpdated)

		processedSongs += len(songs)
		updatedTotal += batchUpdated

		sendEvent(EnrichmentProgress{
			Status:         "batch_complete",
			Message:        fmt.Sprintf("Batch %d complete: enriched %d songs", batch+1, batchUpdated),
			TotalSongs:     totalSongs,
			ProcessedSongs: processedSongs,
			CurrentBatch:   batch + 1,
			TotalBatches:   totalBatches,
		})
		logger.API("enrichGenresStream: Batch %d complete, looping to next batch", batch+1)

		// Small delay between batches to avoid rate limiting
		if batch < totalBatches-1 {
			time.Sleep(500 * time.Millisecond)
		}
	}

	sendEvent(EnrichmentProgress{
		Status:         "complete",
		Message:        fmt.Sprintf("Enrichment complete! Updated genres for %d songs", updatedTotal),
		TotalSongs:     totalSongs,
		ProcessedSongs: processedSongs,
		TotalBatches:   totalBatches,
		CurrentBatch:   totalBatches,
	})
}

// backfillSongYears populates missing song year values from album_metadata.release_date
// This enables the AI DJ to correctly filter by decade (e.g., "90s hip hop")
func (a *API) backfillSongYears(w http.ResponseWriter, r *http.Request) {
	count, err := a.db.BackfillSongYearsFromAlbumMetadata()
	if err != nil {
		logger.API("backfillSongYears: Failed to backfill years: %v", err)
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to backfill years: %v", err))
		return
	}
	logger.API("backfillSongYears: Updated %d songs with year from album metadata", count)
	respondJSON(w, map[string]interface{}{
		"updated": count,
		"message": fmt.Sprintf("Updated %d songs with year from album metadata", count),
	})
}

// detectRemasters uses heuristics to identify songs that may be remasters
// It scans album/title for patterns like "Remastered", "Deluxe Edition", "Anniversary"
// and flags songs with year_uncertain for later AI analysis
func (a *API) detectRemasters(w http.ResponseWriter, r *http.Request) {
	processed, flagged, err := a.db.DetectRemasterSongs()
	if err != nil {
		logger.API("detectRemasters: Failed to detect remasters: %v", err)
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to detect remasters: %v", err))
		return
	}
	logger.API("detectRemasters: Processed %d songs, flagged %d as potential remasters", processed, flagged)
	respondJSON(w, map[string]interface{}{
		"processed": processed,
		"flagged":   flagged,
		"message":   fmt.Sprintf("Processed %d songs, flagged %d as potential remasters", processed, flagged),
	})
}

// enrichAllMetadataStream provides SSE streaming for unified metadata enrichment.
// This is the recommended endpoint - it enriches genres, mood, energy, tempo, BPM,
// instrumental detection, and original year in a single efficient API call per batch.
// Uses a strict JSON contract to keep malformed model output from reaching storage.
// Supports any configured LLM provider (Ollama, Gemini, OpenAI, Anthropic, X.AI).
func (a *API) enrichAllMetadataStream(w http.ResponseWriter, r *http.Request) {
	logger.API("enrichAllMetadataStream: Starting unified metadata enrichment")

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	// Get parameters from query string
	force := r.URL.Query().Get("force") == "true"

	// Helper to send SSE event
	sendEvent := func(progress EnrichmentProgress) {
		data, _ := json.Marshal(progress)
		fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()
	}

	// Helper to send SSE keepalive comment (invisible to EventSource.onmessage)
	sendKeepalive := func() {
		fmt.Fprintf(w, ": keepalive %d\n\n", time.Now().Unix())
		flusher.Flush()
	}

	// Get LLM provider (checks unified settings first, then legacy gemini_api_key)
	provider, err := llm.GetConfiguredProvider(a.db)
	if err != nil {
		sendEvent(EnrichmentProgress{
			Status:  "error",
			Message: "No LLM configured",
			Error:   "Configure AI Provider in Settings or add a Gemini API key.",
		})
		return
	}
	defer provider.Close()

	// Snapshot every song needing any enrichment before writing results. This
	// prevents pagination from skipping rows as individual fields are filled.
	logger.API("enrichAllMetadataStream: Querying songs for enrichment (force=%v)", force)
	allSongs := make([]db.Song, 0)
	const queryPageSize = 500
	for offset := 0; ; offset += queryPageSize {
		page, queryErr := a.db.GetSongsForAIEnrichment(queryPageSize, force, offset)
		if queryErr != nil {
			sendEvent(EnrichmentProgress{Status: "error", Message: "Failed to query songs", Error: queryErr.Error()})
			return
		}
		allSongs = append(allSongs, page...)
		if len(page) < queryPageSize {
			break
		}
	}
	logger.API("enrichAllMetadataStream: Got %d songs", len(allSongs))

	totalSongs := len(allSongs)
	if totalSongs == 0 {
		sendEvent(EnrichmentProgress{
			Status:     "complete",
			Message:    "No songs need enrichment",
			TotalSongs: 0,
		})
		return
	}

	// Use provider's optimal batch size
	batchSize := provider.GetOptimalBatchSize()
	totalBatches := (totalSongs + batchSize - 1) / batchSize

	// Concurrency settings - adapt based on provider type
	// Cloud APIs can handle parallel requests, but local Ollama should be single-threaded
	// to avoid overloading the GPU/CPU and causing timeouts
	concurrency := provider.GetOptimalConcurrency()
	if totalBatches < concurrency {
		concurrency = totalBatches
	}

	sendEvent(EnrichmentProgress{
		Status:       "started",
		Message:      fmt.Sprintf("Starting unified enrichment of %d songs in %d batches using %s", totalSongs, totalBatches, provider.GetProviderName()),
		TotalSongs:   totalSongs,
		TotalBatches: totalBatches,
	})

	// Thread-safe counters
	var mu sync.Mutex
	processedSongs := 0
	changedSongs := 0
	emptyResults := 0
	genresUpdated := 0
	moodUpdated := 0
	yearsUpdated := 0
	completedBatches := 0
	failedBatches := 0 // Track failed batches for reporting

	// Create batch job channel
	type batchJob struct {
		batchNum int
		songs    []db.Song
	}
	jobs := make(chan batchJob, totalBatches)
	results := make(chan struct{}, totalBatches)

	// Worker function
	workerFunc := func(workerID int) {
		for job := range jobs {
			// Check if client disconnected
			select {
			case <-r.Context().Done():
				return
			default:
			}

			logger.API("enrichAllMetadataStream: Worker %d processing batch %d with %d songs", workerID, job.batchNum+1, len(job.songs))

			// Call LLM API
			unified, err := provider.EnrichAllMetadata(r.Context(), job.songs)

			if err != nil {
				logger.API("enrichAllMetadataStream: Worker %d batch %d error: %v", workerID, job.batchNum+1, err)
				mu.Lock()
				failedBatches++
				completedBatches++ // Count as completed (attempted) even if failed
				mu.Unlock()
				// Continue processing other batches - don't stop everything for one failure
				results <- struct{}{}
				continue
			}

			logger.API("enrichAllMetadataStream: Worker %d batch %d returned %d results", workerID, job.batchNum+1, len(unified))

			updates := make([]db.AIEnrichmentUpdate, 0, len(unified))
			batchEmptyResults := 0
			for _, song := range job.songs {
				meta := unified[song.ID]
				if meta == nil {
					batchEmptyResults++
					continue
				}
				if !meta.HasMetadata() {
					batchEmptyResults++
				}
				updates = append(updates, db.AIEnrichmentUpdate{SongID: song.ID, Genres: meta.Genres, Mood: meta.Mood, Energy: meta.Energy, Tempo: meta.Tempo, BPM: meta.BPM, Instrumental: meta.Instrumental, OriginalYear: meta.OriginalYear})
			}
			applied, updateErr := a.db.ApplyAIEnrichmentBatch(updates, force)
			if updateErr != nil {
				logger.API("enrichAllMetadataStream: Failed to apply batch %d atomically: %v", job.batchNum+1, updateErr)
				mu.Lock()
				failedBatches++
				completedBatches++
				mu.Unlock()
				results <- struct{}{}
				continue
			}
			batchGenres, batchMood, batchYears := applied.Genres, applied.Mood, applied.Years
			if batchGenres > 0 {
				if aggregateErr := a.db.UpdateGenreStats(); aggregateErr != nil {
					logger.API("enrichAllMetadataStream: Batch %d committed but genre aggregate refresh failed: %v", job.batchNum+1, aggregateErr)
				}
			}

			// Update totals thread-safely
			mu.Lock()
			processedSongs += len(job.songs)
			changedSongs += applied.Songs
			emptyResults += batchEmptyResults
			genresUpdated += batchGenres
			moodUpdated += batchMood
			yearsUpdated += batchYears
			completedBatches++
			currentCompleted := completedBatches
			currentProcessed := processedSongs
			currentChanged := changedSongs
			currentEmpty := emptyResults
			mu.Unlock()

			logger.API("enrichAllMetadataStream: Batch %d complete - changed_songs=%d no_metadata=%d genres=%d mood=%d years=%d (total: %d/%d batches)",
				job.batchNum+1, applied.Songs, batchEmptyResults, batchGenres, batchMood, batchYears, currentCompleted, totalBatches)

			// Broadcast after both the song transaction and derived genre aggregate
			// refresh. The revision stream supplies the changed rows; this legacy
			// event also wakes pages with independent derived-data queries.
			if a.scanner != nil {
				a.scanner.EmitEvent(scanner.LibraryEvent{
					Type:         "library_updated",
					Message:      fmt.Sprintf("AI enrichment batch %d committed", job.batchNum+1),
					UpdatedSongs: applied.Songs,
					Data: map[string]interface{}{
						"currentBatch": job.batchNum + 1, "totalBatches": totalBatches,
						"processedSongs": currentProcessed, "totalSongs": totalSongs,
						"changedSongs": applied.Songs, "emptyResults": batchEmptyResults,
					},
				})
			}

			// Send progress event
			mu.Lock()
			sendEvent(EnrichmentProgress{
				Status:         "batch_complete",
				Message:        fmt.Sprintf("Batch %d: changed=%d, no metadata=%d, genres=%d, mood=%d, years=%d (%d/%d batches)", job.batchNum+1, applied.Songs, batchEmptyResults, batchGenres, batchMood, batchYears, currentCompleted, totalBatches),
				TotalSongs:     totalSongs,
				ProcessedSongs: currentProcessed,
				ChangedSongs:   currentChanged,
				EmptyResults:   currentEmpty,
				CurrentBatch:   currentCompleted,
				TotalBatches:   totalBatches,
			})
			mu.Unlock()

			results <- struct{}{}
		}
	}

	// Start workers
	var wg sync.WaitGroup
	for w := 0; w < concurrency; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			workerFunc(workerID)
		}(w)
	}

	// Start keepalive goroutine
	keepaliveDone := make(chan struct{})
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-keepaliveDone:
				return
			case <-r.Context().Done():
				return
			case <-ticker.C:
				mu.Lock()
				logger.API("enrichAllMetadataStream: Keepalive - %d/%d batches complete", completedBatches, totalBatches)
				sendKeepalive()
				mu.Unlock()
			}
		}
	}()

	// Submit all batch jobs
	for batch := 0; batch < totalBatches; batch++ {
		start := batch * batchSize
		end := start + batchSize
		if end > totalSongs {
			end = totalSongs
		}
		jobs <- batchJob{batchNum: batch, songs: allSongs[start:end]}
	}
	close(jobs)

	// Wait for all results
	for i := 0; i < totalBatches; i++ {
		select {
		case <-r.Context().Done():
			close(keepaliveDone)
			logger.API("enrichAllMetadataStream: Client disconnected, cancelling")
			return
		case <-results:
			// Batch completed
		}
	}

	// Stop keepalive and wait for workers
	close(keepaliveDone)
	wg.Wait()

	// Build completion message
	completionMsg := fmt.Sprintf("Enrichment complete! Changed songs: %d, no metadata: %d, Genres: %d, Mood: %d, Years: %d", changedSongs, emptyResults, genresUpdated, moodUpdated, yearsUpdated)
	if failedBatches > 0 {
		completionMsg = fmt.Sprintf("%s (%d batches failed)", completionMsg, failedBatches)
	}
	logger.API("enrichAllMetadataStream: Complete - changed_songs=%d no_metadata=%d genres=%d mood=%d years=%d failed_batches=%d", changedSongs, emptyResults, genresUpdated, moodUpdated, yearsUpdated, failedBatches)

	sendEvent(EnrichmentProgress{
		Status:         "complete",
		Message:        completionMsg,
		TotalSongs:     totalSongs,
		ProcessedSongs: processedSongs,
		ChangedSongs:   changedSongs,
		EmptyResults:   emptyResults,
		TotalBatches:   totalBatches,
		CurrentBatch:   totalBatches,
	})
}

// enrichOriginalYearsStream provides SSE streaming for original year enrichment
// Uses LLM AI to determine the original release year of songs that may have remaster dates
func (a *API) enrichOriginalYearsStream(w http.ResponseWriter, r *http.Request) {
	logger.API("enrichOriginalYearsStream: Starting original year analysis stream")

	// Get LLM provider (checks unified settings first, then legacy gemini_api_key)
	provider, err := llm.GetConfiguredProvider(a.db)
	if err != nil {
		logger.API("enrichOriginalYearsStream: No LLM configured")
		http.Error(w, "No LLM configured. Set AI Provider in Settings.", http.StatusServiceUnavailable)
		return
	}
	defer provider.Close()

	// Setup SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "SSE not supported", http.StatusInternalServerError)
		return
	}

	// Get songs flagged as uncertain (potential remasters) that need AI analysis
	songs, err := a.db.GetUncertainYearSongs(500)
	if err != nil {
		logger.API("enrichOriginalYearsStream: Failed to get songs: %v", err)
		jsonData, _ := json.Marshal(EnrichmentProgress{
			Status:  "error",
			Message: "Failed to get songs",
			Error:   err.Error(),
		})
		fmt.Fprintf(w, "data: %s\n\n", jsonData)
		flusher.Flush()
		return
	}

	totalSongs := len(songs)
	if totalSongs == 0 {
		logger.API("enrichOriginalYearsStream: No songs need year analysis")
		jsonData, _ := json.Marshal(EnrichmentProgress{
			Status:  "complete",
			Message: "No songs need year analysis. Run 'Detect Remasters' first to flag songs.",
		})
		fmt.Fprintf(w, "data: %s\n\n", jsonData)
		flusher.Flush()
		return
	}

	batchSize := 20 // Smaller batches for year analysis
	totalBatches := (totalSongs + batchSize - 1) / batchSize

	logger.API("enrichOriginalYearsStream: Starting analysis of %d songs in %d batches", totalSongs, totalBatches)

	// Send initial event to client
	jsonData, _ := json.Marshal(EnrichmentProgress{
		Status:       "started",
		Message:      fmt.Sprintf("Starting year analysis of %d songs in %d batches", totalSongs, totalBatches),
		TotalSongs:   totalSongs,
		TotalBatches: totalBatches,
	})
	fmt.Fprintf(w, "data: %s\n\n", jsonData)
	flusher.Flush()

	// Create channels for communication
	type progressUpdate struct {
		data EnrichmentProgress
		done bool
	}
	progressChan := make(chan progressUpdate, 100)

	// Start background goroutine for processing
	go func() {
		defer close(progressChan)

		processedSongs := 0
		updatedTotal := 0

		for batch := 0; batch < totalBatches; batch++ {
			start := batch * batchSize
			end := start + batchSize
			if end > totalSongs {
				end = totalSongs
			}
			batchSongs := songs[start:end]

			logger.API("enrichOriginalYearsStream: Processing batch %d/%d (%d songs)", batch+1, totalBatches, len(batchSongs))

			// Send progress update
			select {
			case progressChan <- progressUpdate{
				data: EnrichmentProgress{
					Status:         "processing",
					Message:        fmt.Sprintf("Analyzing years for batch %d of %d (%d songs)", batch+1, totalBatches, len(batchSongs)),
					TotalSongs:     totalSongs,
					ProcessedSongs: processedSongs,
					CurrentBatch:   batch + 1,
					TotalBatches:   totalBatches,
				},
			}:
			default:
			}

			// Call LLM API
			results, err := provider.AnalyzeOriginalYear(context.Background(), batchSongs)
			if err != nil {
				logger.API("enrichOriginalYearsStream: LLM API error on batch %d: %v", batch+1, err)
				select {
				case progressChan <- progressUpdate{
					data: EnrichmentProgress{
						Status:         "error",
						Message:        fmt.Sprintf("API error on batch %d: %v", batch+1, err),
						TotalSongs:     totalSongs,
						ProcessedSongs: processedSongs,
						CurrentBatch:   batch + 1,
						TotalBatches:   totalBatches,
						Error:          err.Error(),
					},
				}:
				default:
				}
				continue
			}

			// Update database with results
			batchUpdated := 0
			for id, analysis := range results {
				if analysis != nil && analysis.OriginalYear > 0 {
					if err := a.db.SetOriginalYear(id, analysis.OriginalYear); err != nil {
						logger.API("enrichOriginalYearsStream: Failed to update year for song %s: %v", id, err)
						continue
					}
					batchUpdated++
					updatedTotal++
				}
			}
			processedSongs += len(batchSongs)

			logger.API("enrichOriginalYearsStream: Batch %d complete - analyzed %d songs (total: %d/%d)", batch+1, batchUpdated, processedSongs, totalSongs)

			// Send batch complete update
			select {
			case progressChan <- progressUpdate{
				data: EnrichmentProgress{
					Status:         "batch_complete",
					Message:        fmt.Sprintf("Batch %d complete - analyzed %d songs", batch+1, batchUpdated),
					TotalSongs:     totalSongs,
					ProcessedSongs: processedSongs,
					CurrentBatch:   batch + 1,
					TotalBatches:   totalBatches,
				},
			}:
			default:
			}
		}

		logger.API("enrichOriginalYearsStream: Year analysis complete - analyzed %d songs total", updatedTotal)

		// Send final complete message
		select {
		case progressChan <- progressUpdate{
			data: EnrichmentProgress{
				Status:         "complete",
				Message:        fmt.Sprintf("Year analysis complete! Analyzed %d songs, found original years for %d", totalSongs, updatedTotal),
				TotalSongs:     totalSongs,
				ProcessedSongs: totalSongs,
				TotalBatches:   totalBatches,
				CurrentBatch:   totalBatches,
			},
			done: true,
		}:
		default:
		}
	}()

	// Stream updates to client
	for update := range progressChan {
		jsonData, _ := json.Marshal(update.data)
		fmt.Fprintf(w, "data: %s\n\n", jsonData)
		flusher.Flush()
	}
}

// enrichMood triggers mood/energy/tempo analysis for songs without mood data
func (a *API) enrichMood(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(map[string]string{
		"message": "Use /api/library/enrich-mood/stream for streaming progress",
	})
}

// enrichMoodStream provides SSE streaming for mood enrichment progress
// The analysis runs in a background goroutine and continues even if the client disconnects
func (a *API) enrichMoodStream(w http.ResponseWriter, r *http.Request) {
	logger.API("enrichMoodStream: Starting mood analysis stream")

	// Get LLM provider (checks unified settings first, then legacy gemini_api_key)
	provider, err := llm.GetConfiguredProvider(a.db)
	if err != nil {
		logger.API("enrichMoodStream: No LLM configured")
		http.Error(w, "No LLM configured. Set AI Provider in Settings.", http.StatusServiceUnavailable)
		return
	}

	// Setup SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "SSE not supported", http.StatusInternalServerError)
		return
	}

	// Get songs without mood analysis
	songs, err := a.db.GetSongsWithoutMood(1000)
	if err != nil {
		provider.Close()
		logger.API("enrichMoodStream: Failed to get songs: %v", err)
		jsonData, _ := json.Marshal(EnrichmentProgress{
			Status:  "error",
			Message: "Failed to get songs",
			Error:   err.Error(),
		})
		fmt.Fprintf(w, "data: %s\n\n", jsonData)
		flusher.Flush()
		return
	}

	totalSongs := len(songs)
	if totalSongs == 0 {
		provider.Close()
		logger.API("enrichMoodStream: All songs already have mood analysis")
		jsonData, _ := json.Marshal(EnrichmentProgress{
			Status:  "complete",
			Message: "All songs already have mood analysis",
		})
		fmt.Fprintf(w, "data: %s\n\n", jsonData)
		flusher.Flush()
		return
	}

	batchSize := 50
	totalBatches := (totalSongs + batchSize - 1) / batchSize

	logger.API("enrichMoodStream: Starting analysis of %d songs in %d batches", totalSongs, totalBatches)

	// Send initial event to client
	jsonData, _ := json.Marshal(EnrichmentProgress{
		Status:       "started",
		Message:      fmt.Sprintf("Starting mood analysis of %d songs in %d batches", totalSongs, totalBatches),
		TotalSongs:   totalSongs,
		TotalBatches: totalBatches,
	})
	fmt.Fprintf(w, "data: %s\n\n", jsonData)
	flusher.Flush()

	// Broadcast to sidebar
	a.scanner.EmitEvent(scanner.LibraryEvent{
		Type:    "mood_started",
		Message: fmt.Sprintf("Analyzing mood for %d songs", totalSongs),
		Data: map[string]interface{}{
			"processedSongs": 0,
			"totalSongs":     totalSongs,
			"currentBatch":   0,
			"totalBatches":   totalBatches,
		},
	})

	// Create channels for communication between goroutine and SSE sender
	type progressUpdate struct {
		data EnrichmentProgress
		done bool
	}
	progressChan := make(chan progressUpdate, 100)

	// Start background goroutine for processing (continues even if client disconnects)
	go func() {
		defer provider.Close()
		defer close(progressChan)

		processedSongs := 0
		updatedTotal := 0

		// broadcastMoodEvent emits to the scanner's event system for sidebar updates
		broadcastMoodEvent := func(eventType, message string, processed, total, currentBatch, batches int) {
			a.scanner.EmitEvent(scanner.LibraryEvent{
				Type:    eventType,
				Message: message,
				Data: map[string]interface{}{
					"processedSongs": processed,
					"totalSongs":     total,
					"currentBatch":   currentBatch,
					"totalBatches":   batches,
				},
			})
		}

		for batch := 0; batch < totalBatches; batch++ {
			start := batch * batchSize
			end := start + batchSize
			if end > totalSongs {
				end = totalSongs
			}
			batchSongs := songs[start:end]

			logger.API("enrichMoodStream: Processing batch %d/%d (%d songs)", batch+1, totalBatches, len(batchSongs))

			// Send progress update (non-blocking)
			select {
			case progressChan <- progressUpdate{
				data: EnrichmentProgress{
					Status:         "processing",
					Message:        fmt.Sprintf("Analyzing mood for batch %d of %d (%d songs)", batch+1, totalBatches, len(batchSongs)),
					TotalSongs:     totalSongs,
					ProcessedSongs: processedSongs,
					CurrentBatch:   batch + 1,
					TotalBatches:   totalBatches,
				},
			}:
			default:
				// Channel full, skip SSE update but continue processing
			}

			// Always broadcast to sidebar
			broadcastMoodEvent("mood_progress", fmt.Sprintf("Analyzing batch %d/%d", batch+1, totalBatches), processedSongs, totalSongs, batch+1, totalBatches)

			moodMap, err := provider.AnalyzeSongMood(context.Background(), batchSongs)
			if err != nil {
				logger.API("enrichMoodStream: LLM API error on batch %d: %v", batch+1, err)
				select {
				case progressChan <- progressUpdate{
					data: EnrichmentProgress{
						Status:         "error",
						Message:        "LLM API error",
						Error:          err.Error(),
						ProcessedSongs: processedSongs,
						CurrentBatch:   batch + 1,
						TotalBatches:   totalBatches,
					},
					done: true,
				}:
				default:
				}
				broadcastMoodEvent("mood_complete", fmt.Sprintf("Mood analysis error: %v", err), processedSongs, totalSongs, batch+1, totalBatches)
				return
			}

			batchUpdated := 0
			for id, analysis := range moodMap {
				if err := a.db.UpdateSongMood(id, analysis.Mood, analysis.Energy, analysis.Tempo, analysis.BPM, analysis.Instrumental); err != nil {
					logger.API("enrichMoodStream: Failed to update mood for song %s: %v", id, err)
					continue
				}
				batchUpdated++
			}

			processedSongs += len(batchSongs)
			updatedTotal += batchUpdated

			logger.API("enrichMoodStream: Batch %d complete - analyzed %d songs (total: %d/%d)", batch+1, batchUpdated, processedSongs, totalSongs)

			// Send batch completion (non-blocking)
			select {
			case progressChan <- progressUpdate{
				data: EnrichmentProgress{
					Status:         "batch_complete",
					Message:        fmt.Sprintf("Batch %d complete: analyzed %d songs", batch+1, batchUpdated),
					TotalSongs:     totalSongs,
					ProcessedSongs: processedSongs,
					CurrentBatch:   batch + 1,
					TotalBatches:   totalBatches,
				},
			}:
			default:
			}

			// Always broadcast to sidebar
			broadcastMoodEvent("mood_progress", fmt.Sprintf("Batch %d complete: %d songs analyzed", batch+1, batchUpdated), processedSongs, totalSongs, batch+1, totalBatches)

			if batch < totalBatches-1 {
				time.Sleep(500 * time.Millisecond)
			}
		}

		logger.API("enrichMoodStream: Mood analysis complete - analyzed %d songs total", updatedTotal)

		// Send completion (non-blocking)
		select {
		case progressChan <- progressUpdate{
			data: EnrichmentProgress{
				Status:         "complete",
				Message:        fmt.Sprintf("Mood analysis complete! Analyzed %d songs", updatedTotal),
				TotalSongs:     totalSongs,
				ProcessedSongs: processedSongs,
				TotalBatches:   totalBatches,
				CurrentBatch:   totalBatches,
			},
			done: true,
		}:
		default:
		}

		// Always broadcast completion to sidebar
		broadcastMoodEvent("mood_complete", fmt.Sprintf("Mood analysis complete! %d songs", updatedTotal), processedSongs, totalSongs, totalBatches, totalBatches)
	}()

	// SSE event loop - reads from progress channel until done or client disconnects
	for {
		select {
		case <-r.Context().Done():
			logger.API("enrichMoodStream: Client disconnected, but background analysis continues")
			return
		case update, ok := <-progressChan:
			if !ok {
				// Channel closed, processing complete
				return
			}
			jsonData, _ := json.Marshal(update.data)
			fmt.Fprintf(w, "data: %s\n\n", jsonData)
			flusher.Flush()
			if update.done {
				return
			}
		}
	}
}
