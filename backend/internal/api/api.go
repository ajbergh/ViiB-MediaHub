package api

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/audio"
	"github.com/ajbergh/viib-mediahub/internal/db"
	"github.com/go-chi/chi/v5"
)

type API struct {
	db       *db.DB
	dataDir  string
	coverDir string

	// Scanning state
	scanning     bool
	scanProgress string
	scanMutex    sync.RWMutex
}

func New(database *db.DB, dataDir string) *API {
	coverDir := filepath.Join(dataDir, "covers")
	os.MkdirAll(coverDir, 0755)

	return &API{
		db:       database,
		dataDir:  dataDir,
		coverDir: coverDir,
	}
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

	// File serving
	r.Get("/audio/*", a.serveAudio)
	r.Get("/cover/*", a.serveCover)

	// System
	r.Get("/health", a.healthCheck)
	r.Post("/browse", a.browseFolder)

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
	a.scanMutex.Lock()
	if a.scanning {
		a.scanMutex.Unlock()
		respondError(w, http.StatusConflict, "Scan already in progress")
		return
	}
	a.scanning = true
	a.scanProgress = "Starting scan..."
	a.scanMutex.Unlock()

	go a.performScan()

	respondJSON(w, map[string]string{"status": "started"})
}

func (a *API) getScanStatus(w http.ResponseWriter, r *http.Request) {
	a.scanMutex.RLock()
	defer a.scanMutex.RUnlock()

	respondJSON(w, map[string]interface{}{
		"scanning": a.scanning,
		"progress": a.scanProgress,
	})
}

func (a *API) performScan() {
	defer func() {
		a.scanMutex.Lock()
		a.scanning = false
		a.scanMutex.Unlock()
	}()

	folders, err := a.db.GetScanFolders()
	if err != nil {
		log.Printf("Failed to get scan folders: %v", err)
		return
	}

	if len(folders) == 0 {
		a.setProgress("No folders configured")
		return
	}

	totalSongs := 0
	for _, folder := range folders {
		a.setProgress(fmt.Sprintf("Scanning: %s", folder.Path))

		songs, err := a.scanFolder(folder.Path)
		if err != nil {
			log.Printf("Error scanning %s: %v", folder.Path, err)
			continue
		}

		if len(songs) > 0 {
			if err := a.db.SaveSongs(songs); err != nil {
				log.Printf("Error saving songs: %v", err)
			}
		}

		totalSongs += len(songs)
		a.db.UpdateScanFolder(folder.ID, time.Now().UnixMilli(), len(songs))
	}

	a.setProgress(fmt.Sprintf("Scan complete: %d songs found", totalSongs))
}

func (a *API) setProgress(msg string) {
	a.scanMutex.Lock()
	a.scanProgress = msg
	a.scanMutex.Unlock()
	log.Printf("Scan: %s", msg)
}

func (a *API) scanFolder(folderPath string) ([]db.Song, error) {
	var songs []db.Song

	audioExtensions := map[string]bool{
		".mp3":  true,
		".flac": true,
		".m4a":  true,
		".aac":  true,
		".ogg":  true,
		".opus": true,
		".wav":  true,
		".wma":  true,
	}

	err := filepath.Walk(folderPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors
		}

		if info.IsDir() {
			return nil
		}

		ext := strings.ToLower(filepath.Ext(path))
		if !audioExtensions[ext] {
			return nil
		}

		song, err := audio.ExtractMetadata(path)
		if err != nil {
			log.Printf("Failed to extract metadata from %s: %v", path, err)
			return nil
		}

		// Save cover art if present
		if song.CoverData != nil {
			coverPath := filepath.Join(a.coverDir, song.ID+".jpg")
			if err := os.WriteFile(coverPath, song.CoverData, 0644); err == nil {
				song.CoverPath = coverPath
			}
			song.CoverData = nil // Don't store in DB
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
			FilePath:    path,
			CoverPath:   song.CoverPath,
			AddedAt:     time.Now().UnixMilli(),
		}

		songs = append(songs, dbSong)
		return nil
	})

	return songs, err
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
	coverPath := filepath.Join(a.coverDir, songID+".jpg")

	if _, err := os.Stat(coverPath); os.IsNotExist(err) {
		respondError(w, http.StatusNotFound, "Cover not found")
		return
	}

	http.ServeFile(w, r, coverPath)
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
