// Package api provides REST API handlers for ViiB MediaHub.
//
// This package implements the complete REST API including:
//   - Library management: songs, playlists, folders
//   - Media serving: audio streaming, cover art
//   - Spotify integration: OAuth, search proxy, downloads
//   - Metadata caching: album and artist enrichment
//   - Settings: key-value configuration storage
//   - SSE endpoints: real-time download progress and library events
//
// All endpoints return JSON responses with consistent error handling.
// Audio and cover files are served with appropriate caching headers.
package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
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
		isAllowed := strings.HasPrefix(strings.ToLower(normalizedPath), strings.ToLower(normalizedCoverDir))

		if !isAllowed {
			// Check if path is within any configured scan folder
			scanFolders, _ := a.db.GetScanFolders()
			for _, folder := range scanFolders {
				normalizedFolder := filepath.Clean(folder.Path)
				if strings.HasPrefix(strings.ToLower(normalizedPath), strings.ToLower(normalizedFolder)) {
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

	if req.AlbumKey == "" || req.ImageURL == "" {
		respondError(w, http.StatusBadRequest, "albumKey and imageUrl are required")
		return
	}

	// Parse album key to get album name and artist
	parts := strings.Split(req.AlbumKey, "::")
	if len(parts) != 2 {
		respondError(w, http.StatusBadRequest, "Invalid album key format (expected album::artist)")
		return
	}
	albumName := parts[0]
	// artistName := parts[1]

	// Find a song from this album to get the folder path
	songs, err := a.db.GetAllSongs()
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to get songs")
		return
	}

	var albumFolder string
	for _, song := range songs {
		if song.Album == albumName {
			albumFolder = filepath.Dir(song.FilePath)
			break
		}
	}

	if albumFolder == "" {
		respondError(w, http.StatusNotFound, "No songs found for this album")
		return
	}

	// Download the image
	resp, err := http.Get(req.ImageURL)
	if err != nil {
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to download image: %v", err))
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to download image: HTTP %d", resp.StatusCode))
		return
	}

	// Save as cover.jpg
	coverPath := filepath.Join(albumFolder, "cover.jpg")
	file, err := os.Create(coverPath)
	if err != nil {
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to create cover file: %v", err))
		return
	}
	defer file.Close()

	_, err = io.Copy(file, resp.Body)
	if err != nil {
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to write cover file: %v", err))
		return
	}

	// Update the database with the local cover path
	if err := a.db.UpdateAlbumLocalCover(req.AlbumKey, coverPath); err != nil {
		logger.API("Warning: Failed to update album local cover in database: %v", err)
	}

	respondJSON(w, map[string]interface{}{
		"status":    "ok",
		"coverPath": coverPath,
	})
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
	logger.API("getAllArtistMetadata: Returning %d artists (%d with local images)", len(metadata), withLocalImage)

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

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		logger.API("downloadArtistImage: Invalid request body: %v", err)
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.ArtistName == "" || req.ImageURL == "" {
		logger.API("downloadArtistImage: Missing required fields - artistName=%q, imageUrl=%q", req.ArtistName, req.ImageURL)
		respondError(w, http.StatusBadRequest, "artistName and imageUrl are required")
		return
	}

	logger.API("downloadArtistImage: Downloading image for artist %q from %s", req.ArtistName, req.ImageURL)

	// Create artists image directory
	artistImagesDir := filepath.Join(a.coverDir, "artists")
	if err := os.MkdirAll(artistImagesDir, 0755); err != nil {
		logger.API("downloadArtistImage: Failed to create artists directory: %v", err)
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to create artists directory: %v", err))
		return
	}

	// Generate filename from artist name (sanitize for filesystem)
	safeArtistName := strings.ReplaceAll(req.ArtistName, "/", "_")
	safeArtistName = strings.ReplaceAll(safeArtistName, "\\", "_")
	safeArtistName = strings.ReplaceAll(safeArtistName, ":", "_")
	imagePath := filepath.Join(artistImagesDir, safeArtistName+".jpg")

	// Download the image
	resp, err := http.Get(req.ImageURL)
	if err != nil {
		logger.API("downloadArtistImage: Failed to download image for %q: %v", req.ArtistName, err)
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to download image: %v", err))
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		logger.API("downloadArtistImage: HTTP error downloading image for %q: status %d", req.ArtistName, resp.StatusCode)
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to download image: HTTP %d", resp.StatusCode))
		return
	}

	// Save the image
	file, err := os.Create(imagePath)
	if err != nil {
		logger.API("downloadArtistImage: Failed to create image file for %q at %s: %v", req.ArtistName, imagePath, err)
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to create image file: %v", err))
		return
	}
	defer file.Close()

	bytesWritten, err := io.Copy(file, resp.Body)
	if err != nil {
		logger.API("downloadArtistImage: Failed to write image file for %q: %v", req.ArtistName, err)
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to write image file: %v", err))
		return
	}

	logger.API("downloadArtistImage: Successfully saved %d bytes for artist %q to %s", bytesWritten, req.ArtistName, imagePath)

	// Update the database with the local image path
	if err := a.db.UpdateArtistLocalImage(req.ArtistName, imagePath); err != nil {
		logger.API("downloadArtistImage: Failed to update artist local image in database for %q: %v", req.ArtistName, err)
	}

	respondJSON(w, map[string]interface{}{
		"status":    "ok",
		"imagePath": imagePath,
	})
}
