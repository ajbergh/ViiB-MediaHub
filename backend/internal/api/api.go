package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/audio"
	"github.com/ajbergh/viib-mediahub/internal/db"
	"github.com/ajbergh/viib-mediahub/internal/logger"
	"github.com/ajbergh/viib-mediahub/internal/scanner"
	"github.com/go-chi/chi/v5"
)

type API struct {
	db              *db.DB
	dataDir         string
	coverDir        string
	downloadManager *DownloadManager
	scanner         *scanner.Scanner
}

func New(database *db.DB, dataDir string) *API {
	logger.API("New: Starting with dataDir=%s", dataDir)

	coverDir := filepath.Join(dataDir, "covers")
	os.MkdirAll(coverDir, 0755)

	// Get download directory from settings, or use default
	downloadDir := filepath.Join(dataDir, "spotify_downloads")
	if customPath, err := database.GetSetting("spotify_download_path"); err == nil && customPath != "" {
		downloadDir = customPath
		logger.API("Using custom Spotify download path: %s", downloadDir)
	}
	os.MkdirAll(downloadDir, 0755)

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

	// Trigger scan on startup (in background)
	go api.scanOnStartup()

	return api
}

func (a *API) Routes() chi.Router {
	r := chi.NewRouter()

	// Library endpoints
	r.Get("/songs", a.getSongs)
	r.Delete("/songs", a.clearSongs)
	r.Post("/songs/{id}/play", a.recordPlay)

	// Playlist endpoints
	r.Get("/playlists", a.getPlaylists)
	r.Post("/playlists", a.createPlaylist)
	r.Put("/playlists/{id}", a.updatePlaylist)
	r.Delete("/playlists/{id}", a.deletePlaylist)

	// Folder scanning
	r.Get("/folders", a.getScanFolders)
	r.Post("/folders", a.addScanFolder)
	r.Delete("/folders/{id}", a.removeScanFolder)
	r.Post("/scan", a.startScan)
	r.Get("/scan/status", a.getScanStatus)

	// Library events SSE
	r.Get("/library/events", a.libraryEventsSSE)

	// File serving
	r.Get("/audio/*", a.serveAudio)
	r.Get("/cover/*", a.serveCover)

	// System
	r.Get("/health", a.healthCheck)
	r.Post("/browse", a.browseFolder)

	// Spotify
	r.Get("/spotify/credentials", a.getSpotifyCredentials)
	r.Post("/spotify/credentials", a.saveSpotifyCredentials)
	r.Get("/spotify/search", a.spotifySearch)
	r.Get("/spotify/me", a.spotifyGetUserProfile)
	r.Get("/spotify/proxy", a.spotifyProxy)
	r.Post("/spotify/proxy", a.spotifyProxy)

	// Spotify Downloads
	r.Post("/spotify/download/track", a.downloadTrack)
	r.Post("/spotify/download/album", a.downloadAlbum)
	r.Post("/spotify/download/playlist", a.downloadPlaylist)
	r.Post("/spotify/download/url", a.downloadFromURL)
	r.Get("/spotify/downloads", a.getDownloads)
	r.Get("/spotify/downloads/{id}", a.getDownloadStatus)
	r.Delete("/spotify/downloads/{id}", a.deleteDownload)
	r.Post("/spotify/downloads/{id}/retry", a.retryDownload)
	r.Delete("/spotify/downloads/completed", a.clearCompletedDownloads)
	r.Get("/spotify/downloads/events", a.downloadProgressSSE)

	// Settings
	r.Get("/settings/{key}", a.getSetting)
	r.Post("/settings/{key}", a.setSetting)

	return r
}

// Helper functions

func respondJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func respondError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}

// Handlers

func (a *API) healthCheck(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, map[string]string{"status": "ok", "version": "1.0.0"})
}

func (a *API) getSongs(w http.ResponseWriter, r *http.Request) {
	songs, err := a.db.GetAllSongs()
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Transform file paths to API URLs
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

	// Clean up cover files
	os.RemoveAll(a.coverDir)
	os.MkdirAll(a.coverDir, 0755)

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

func (a *API) getScanStatus(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, map[string]interface{}{
		"scanning": a.scanner.IsScanning(),
		"progress": a.scanner.GetProgress(),
	})
}

// libraryEventsSSE streams library events (scan complete, etc.) to the frontend via SSE
func (a *API) libraryEventsSSE(w http.ResponseWriter, r *http.Request) {
	logger.API("libraryEventsSSE: Connection started")
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

	logger.API("libraryEventsSSE: Subscribed and entering event loop")
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
			logger.API("libraryEventsSSE: Sending event: %s", event.Type)
			data, _ := json.Marshal(event)
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}
	}
}

// scanOnStartup triggers a library scan when the application starts
func (a *API) scanOnStartup() {
	// Wait for application to fully initialize
	time.Sleep(3 * time.Second)

	folders, err := a.db.GetScanFolders()
	if err != nil {
		logger.API("Error getting scan folders for startup scan: %v", err)
		return
	}

	if len(folders) == 0 {
		logger.API("No folders configured, skipping startup scan")
		return
	}

	logger.API("Starting automatic library scan on startup...")
	result, err := a.scanner.ScanAll()
	if err != nil {
		logger.API("Startup scan error: %v", err)
		return
	}

	logger.API("Startup scan complete: %d files, %d new, %d updated (%s)",
		result.TotalFiles, result.NewSongs, result.UpdatedSongs, result.Duration)
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
	songID := strings.TrimPrefix(r.URL.Path, "/api/cover/")

	// First, try to get the song to find its cover path
	song, err := a.db.GetSongByID(songID)
	if err == nil && song.CoverPath != "" {
		// Song has a cover path set - serve that file
		if _, err := os.Stat(song.CoverPath); err == nil {
			http.ServeFile(w, r, song.CoverPath)
			return
		}
	}

	// Fallback: try legacy per-song cover file
	coverPath := filepath.Join(a.coverDir, songID+".jpg")
	if _, err := os.Stat(coverPath); err == nil {
		http.ServeFile(w, r, coverPath)
		return
	}

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

	entries, err := os.ReadDir(req.Path)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	type FolderEntry struct {
		Name  string `json:"name"`
		Path  string `json:"path"`
		IsDir bool   `json:"isDir"`
	}

	var result []FolderEntry

	// Add parent directory
	parent := filepath.Dir(req.Path)
	if parent != req.Path {
		result = append(result, FolderEntry{
			Name:  "..",
			Path:  parent,
			IsDir: true,
		})
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
	os.MkdirAll(tempDir, 0755)

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

	// Special handling for spotify_download_path - update download manager and scanner
	if key == "spotify_download_path" {
		downloadDir := body.Value
		if downloadDir == "" {
			// Use default if empty
			downloadDir = filepath.Join(a.dataDir, "spotify_downloads")
		}
		// Create the directory if it doesn't exist
		if err := os.MkdirAll(downloadDir, 0755); err != nil {
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
