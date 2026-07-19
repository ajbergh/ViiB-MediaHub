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
	"github.com/ajbergh/viib-mediahub/internal/validation"
	"github.com/ajbergh/viib-mediahub/internal/version"
	"github.com/go-chi/chi/v5"
)

// API provides the REST API handlers and dependencies for the ViiB MediaHub backend.
type API struct {
	db              *db.DB
	dataDir         string
	coverDir        string
	downloadManager *DownloadManager
	scanner         *scanner.Scanner
	lastfmClient    *lastfm.Client
	enrichRunning   int32
}

// New constructs a new API instance.
func New(database *db.DB, dataDir string) *API {
	logger.API("New: Starting with dataDir=%s", dataDir)

	coverDir := filepath.Join(dataDir, "covers")
	_ = os.MkdirAll(coverDir, 0700)

	downloadDir := filepath.Join(dataDir, "spotify_downloads")
	if customPath, err := database.GetSetting("spotify_download_path"); err == nil && customPath != "" {
		downloadDir = customPath
		logger.API("Using custom Spotify download path: %s", downloadDir)
	}
	_ = os.MkdirAll(downloadDir, 0700)

	logger.API("Creating scanner...")
	sc := scanner.New(database, dataDir)
	sc.SetSpotifyDownloadDir(downloadDir)

	logger.API("Creating download manager...")
	dm := NewDownloadManager(database, downloadDir)
	dm.SetScanner(sc)
	dm.Start()

	api := &API{
		db:              database,
		dataDir:         dataDir,
		coverDir:        coverDir,
		downloadManager: dm,
		scanner:         sc,
	}
	api.initLastFMClient()
	go api.scanOnStartup()
	go func() {
		if err := database.UpdateGenreStats(); err != nil {
			logger.API("Failed to update genre stats on startup: %v", err)
		}
	}()
	return api
}

// Routes registers all API HTTP routes.
func (a *API) Routes() chi.Router {
	r := chi.NewRouter()

	r.Get("/songs", a.getSongs)
	r.Get("/genres", a.getGenres)
	r.Post("/genres/normalize", a.normalizeGenres)
	r.Post("/smart-playlist", a.handleGenerateSmartPlaylist)
	r.Delete("/songs", a.clearSongs)
	r.Post("/library/enrich-genres", a.enrichGenres)
	r.Get("/library/enrich-genres/stream", a.enrichGenresStream)
	r.Get("/library/enrich-all/stream", a.enrichAllMetadataStream)
	r.Post("/library/enrich-mood", a.enrichMood)
	r.Get("/library/enrich-mood/stream", a.enrichMoodStream)
	r.Post("/library/backfill-years", a.backfillSongYears)
	r.Post("/library/detect-remasters", a.detectRemasters)
	r.Get("/library/enrich-years/stream", a.enrichOriginalYearsStream)
	r.Post("/songs/{id}/play", a.recordPlay)
	r.Post("/songs/{id}/listen-event", a.recordListeningEvent)
	r.Patch("/songs/{id}/duration", a.updateSongDuration)

	r.Post("/songs/{id}/like", a.toggleLike)
	r.Get("/songs/liked", a.getLikedSongIDs)
	r.Post("/songs/like/bulk", a.bulkLikeSongs)
	r.Post("/albums/{albumKey}/like", a.toggleAlbumLike)
	r.Get("/albums/liked", a.getLikedAlbumKeys)
	r.Get("/albums/liked/full", a.getLikedAlbums)

	r.Get("/playlists", a.getPlaylists)
	r.Post("/playlists", a.createPlaylist)
	r.Put("/playlists/{id}", a.updatePlaylist)
	r.Delete("/playlists/{id}", a.deletePlaylist)

	r.Get("/folders", a.getScanFolders)
	r.Post("/folders", a.addScanFolder)
	r.Delete("/folders/{id}", a.removeScanFolder)
	r.Post("/scan", a.startScan)
	r.Post("/scan/quick", a.startQuickScan)
	r.Get("/scan/status", a.getScanStatus)
	r.Get("/library/events", a.libraryEventsSSE)

	r.Get("/dj/personas", a.getDJPersonas)
	r.Get("/dj/waveform/{id}", a.getDJWaveform)
	r.Get("/dj/hotcues/{id}", a.getDJHotCues)
	r.Put("/dj/hotcues/{id}", a.saveDJHotCues)

	r.Get("/audio/*", a.serveAudio)
	r.Get("/cover/*", a.serveCover)

	r.Get("/health", a.healthCheck)
	r.Post("/browse", a.browseFolder)
	r.Get("/settings/{key}", a.getSetting)
	r.Post("/settings/{key}", a.setSetting)

	r.Get("/spotify/credentials", a.getSpotifyCredentials)
	r.Post("/spotify/credentials", a.saveSpotifyCredentials)
	r.Get("/spotify/search", a.spotifySearch)
	r.Get("/spotify/search/playlists", a.spotifySearchPlaylists)
	r.Get("/spotify/playlists/{id}/scrape", a.spotifyGetPlaylistByScraping)
	r.Get("/spotify/me", a.spotifyGetUserProfile)
	r.Get("/spotify/proxy", a.spotifyProxy)
	r.Post("/spotify/proxy", a.spotifyProxy)
	r.Get("/spotify/auth/status", a.getSpotifyAuthStatus)
	r.Post("/spotify/auth/refresh", a.refreshSpotifyAuth)

	r.Post("/spotify/download/track", a.downloadTrack)
	r.Post("/spotify/download/album", a.downloadAlbum)
	r.Post("/spotify/download/playlist", a.downloadPlaylist)
	r.Post("/spotify/download/url", a.downloadFromURL)
	r.Get("/spotify/downloads", a.getDownloads)
	r.Get("/spotify/downloads/{id}", a.getDownloadStatus)
	r.Delete("/spotify/downloads/{id}", a.deleteDownload)
	r.Post("/spotify/downloads/{id}/retry", a.retryDownload)
	r.Post("/spotify/downloads/{id}/force-restart", a.forceRestartDownload)
	r.Delete("/spotify/downloads/completed", a.clearCompletedDownloads)
	r.Get("/spotify/downloads/events", a.downloadProgressSSE)
	r.Get("/spotify/stream/{id}", a.streamSpotifyTrack)

	r.Get("/albums/metadata", a.getAllAlbumMetadata)
	r.Get("/albums/metadata/expired", a.getExpiredAlbumMetadata)
	r.Get("/albums/metadata/unchecked", a.getUncheckedAlbumMetadata)
	r.Get("/albums/metadata/{key}", a.getAlbumMetadata)
	r.Post("/albums/metadata", a.saveAlbumMetadata)
	r.Post("/albums/metadata/batch", a.batchGetAlbumMetadata)
	r.Post("/albums/metadata/download-cover", a.downloadAlbumCover)
	r.Delete("/albums/metadata/{key}", a.resetAlbumMetadata)

	r.Get("/artists/metadata", a.getAllArtistMetadata)
	r.Get("/artists/metadata/unchecked", a.getUncheckedArtistMetadata)
	r.Get("/artists/metadata/{name}", a.getArtistMetadata)
	r.Post("/artists/metadata", a.saveArtistMetadata)
	r.Post("/artists/metadata/download-image", a.downloadArtistImage)
	r.Delete("/artists/metadata/{name}", a.resetArtistMetadata)

	r.Get("/llm/settings", a.getLLMSettings)
	r.Put("/llm/settings", a.updateLLMSettings)
	r.Get("/llm/providers", a.getLLMProviders)
	r.Post("/llm/test", a.testLLMConnection)

	r.Get("/lastfm/settings", a.handleGetLastFMSettings)
	r.Post("/lastfm/settings", a.handleSaveLastFMSettings)
	r.Get("/lastfm/status", a.handleLastFMStatus)
	r.Post("/lastfm/test", a.handleTestLastFMConnection)
	r.Post("/lastfm/authenticate", a.handleLastFMAuthenticate)
	r.Post("/lastfm/enrich/songs", a.handleLastFMEnrichSongs)
	r.Post("/lastfm/enrich/artists", a.handleLastFMEnrichArtists)
	r.Get("/lastfm/track", a.handleGetTrackLastFM)
	r.Get("/lastfm/similar", a.handleGetSimilarTracks)

	r.Post("/log", a.handleFrontendLog)
	return r
}

func respondJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(data)
}

func respondError(w http.ResponseWriter, status int, message string) {
	clientMessage := message
	if status >= 500 {
		logger.API("Internal error (HTTP %d): %s", status, message)
		clientMessage = "Internal server error"
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": clientMessage})
}

func (a *API) healthCheck(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, map[string]string{"status": "ok", "version": version.Current})
}

// FrontendLogRequest represents a log message from the frontend.
type FrontendLogRequest struct {
	Level     string `json:"level"`
	Message   string `json:"message"`
	Component string `json:"component"`
	Data      any    `json:"data"`
}

func (a *API) handleFrontendLog(w http.ResponseWriter, r *http.Request) {
	var req FrontendLogRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid log request")
		return
	}
	level := strings.ToLower(req.Level)
	if level != "debug" && level != "info" && level != "warn" && level != "error" {
		level = "info"
	}
	component := req.Component
	if component == "" {
		component = "Frontend"
	}
	fullMessage := fmt.Sprintf("[%s] %s", component, req.Message)
	if req.Data != nil {
		if dataBytes, err := json.Marshal(req.Data); err == nil {
			fullMessage = fmt.Sprintf("%s | Data: %s", fullMessage, string(dataBytes))
		}
	}
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

func (a *API) getDJPersonas(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, dj.ListPersonas())
}

func (a *API) getSongs(w http.ResponseWriter, r *http.Request) {
	songs, err := a.db.GetAllSongs()
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	for i := range songs {
		songs[i].FilePath = "/api/audio/" + songs[i].ID
		if songs[i].CoverPath != "" {
			songs[i].CoverPath = "/api/cover/" + songs[i].ID
		}
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
	_ = os.RemoveAll(a.coverDir)
	_ = os.MkdirAll(a.coverDir, 0700)
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

func (a *API) recordListeningEvent(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		respondError(w, http.StatusBadRequest, "Missing song ID")
		return
	}
	var body struct {
		PlayDuration float64 `json:"playDuration"`
		SongDuration float64 `json:"songDuration"`
		Context      string  `json:"context"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	song, err := a.db.GetSongByID(id)
	if err != nil {
		respondError(w, http.StatusNotFound, "Song not found")
		return
	}
	if body.PlayDuration < 0 || body.SongDuration <= 0 || body.PlayDuration > 7*24*60*60 || body.SongDuration > 7*24*60*60 {
		respondError(w, http.StatusBadRequest, "Invalid listening durations")
		return
	}
	completionRatio := body.PlayDuration / body.SongDuration
	var eventType string
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
	primaryGenre := ""
	if len(song.Genre) > 0 {
		primaryGenre = song.Genre[0]
	}
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
	if eventType != "play_complete" {
		if err := a.db.UpdateSkipCount(id); err != nil {
			log.Printf("Failed to update skip count for %s: %v", id, err)
		}
	}
	for _, genre := range song.Genre {
		if err := a.db.UpdateGenrePreferences(genre); err != nil {
			log.Printf("Failed to update genre preferences for %s: %v", genre, err)
		}
	}
	respondJSON(w, map[string]interface{}{"status": "ok", "eventType": eventType})
}

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

// The remainder of this file contains the existing handlers for likes,
// playlists, scanning, media serving, settings, metadata, and enrichment.
