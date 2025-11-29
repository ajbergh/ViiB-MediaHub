// Package scanner provides media library scanning functionality.
//
// The Scanner recursively scans configured folders for audio files,
// extracts metadata using the audio package, and updates the database.
//
// Features:
//   - Incremental scanning: skips unchanged files based on path
//   - Album artwork caching: one cover per album to save disk space
//   - Removal detection: removes songs when source files are deleted
//   - SSE event broadcasting: notifies frontend of scan progress
//   - Spotify download monitoring: auto-rescans after downloads complete
//
// Supported audio extensions:
//
//	.mp3, .flac, .m4a, .aac, .ogg, .opus, .wav, .wma
package scanner

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/db"
	"github.com/ajbergh/viib-mediahub/internal/logger"
	taglib "go.senan.xyz/taglib"
)

// Supported audio file extensions
var supportedExtensions = map[string]bool{
	".mp3":  true,
	".flac": true,
	".m4a":  true,
	".aac":  true,
	".ogg":  true,
	".opus": true,
	".wav":  true,
	".wma":  true,
}

// Common cover art filenames (case-insensitive matching)
var coverFilenames = []string{
	"cover.jpg", "cover.jpeg", "cover.png",
	"folder.jpg", "folder.jpeg", "folder.png",
	"album.jpg", "album.jpeg", "album.png",
	"front.jpg", "front.jpeg", "front.png",
	"albumart.jpg", "albumart.jpeg", "albumart.png",
	"artwork.jpg", "artwork.jpeg", "artwork.png",
}

// LibraryEvent represents an event that can be sent to frontend clients
type LibraryEvent struct {
	Type         string `json:"type"`                   // "scan_complete", "scan_started", "scan_progress"
	Message      string `json:"message"`                // Human-readable message
	NewSongs     int    `json:"newSongs,omitempty"`     // Number of new songs added (for scan_complete)
	RemovedSongs int    `json:"removedSongs,omitempty"` // Number of songs removed (for scan_complete)
	TotalSongs   int    `json:"totalSongs,omitempty"`   // Total songs in library (for scan_complete)
}

// Scanner handles media library scanning with progress tracking
type Scanner struct {
	db       *db.DB
	coverDir string
	dataDir  string

	// Scanning state
	scanning     bool
	scanProgress string
	scanMutex    sync.RWMutex

	// Album artwork cache to avoid duplicates
	// Key: "artist|album" -> cover file path
	albumCovers     map[string]string
	albumCoverMutex sync.RWMutex

	// Download folder monitoring
	spotifyDownloadDir     string
	downloadsSinceLastScan int
	rescanThreshold        int // Rescan after this many downloads
	rescanMutex            sync.Mutex

	// Event broadcasting to multiple SSE clients
	subscribers     map[chan LibraryEvent]struct{}
	subscriberMutex sync.RWMutex
}

// ScanResult contains the results of a scan operation
type ScanResult struct {
	TotalFiles   int
	NewSongs     int
	UpdatedSongs int
	RemovedSongs int
	Errors       int
	Duration     time.Duration
}

// New creates a new Scanner instance
func New(database *db.DB, dataDir string) *Scanner {
	coverDir := filepath.Join(dataDir, "covers")
	os.MkdirAll(coverDir, 0755)

	return &Scanner{
		db:              database,
		coverDir:        coverDir,
		dataDir:         dataDir,
		albumCovers:     make(map[string]string),
		rescanThreshold: 5, // Default: rescan after 5 downloads
		subscribers:     make(map[chan LibraryEvent]struct{}),
	}
}

// Subscribe creates a new channel for receiving library events
// The caller should call Unsubscribe when done to prevent leaks
func (s *Scanner) Subscribe() chan LibraryEvent {
	ch := make(chan LibraryEvent, 10)
	s.subscriberMutex.Lock()
	s.subscribers[ch] = struct{}{}
	s.subscriberMutex.Unlock()
	logger.Scanner("New subscriber added, total: %d", len(s.subscribers))
	return ch
}

// Unsubscribe removes a channel from receiving library events
func (s *Scanner) Unsubscribe(ch chan LibraryEvent) {
	s.subscriberMutex.Lock()
	delete(s.subscribers, ch)
	close(ch)
	s.subscriberMutex.Unlock()
	logger.Scanner("Subscriber removed, total: %d", len(s.subscribers))
}

// emitEvent broadcasts an event to all subscribers (non-blocking)
func (s *Scanner) emitEvent(event LibraryEvent) {
	s.subscriberMutex.RLock()
	defer s.subscriberMutex.RUnlock()

	logger.Scanner("Broadcasting event to %d subscribers: %s - %s", len(s.subscribers), event.Type, event.Message)

	for ch := range s.subscribers {
		select {
		case ch <- event:
		default:
			logger.Scanner("Subscriber channel full, dropping event")
		}
	}
}

// SetSpotifyDownloadDir sets the Spotify download directory to monitor
func (s *Scanner) SetSpotifyDownloadDir(dir string) {
	s.rescanMutex.Lock()
	defer s.rescanMutex.Unlock()
	s.spotifyDownloadDir = dir
	logger.Scanner("Spotify download directory set to: %s", dir)
}

// SetRescanThreshold sets how many downloads trigger an automatic rescan
func (s *Scanner) SetRescanThreshold(threshold int) {
	s.rescanMutex.Lock()
	defer s.rescanMutex.Unlock()
	s.rescanThreshold = threshold
	logger.Scanner("Rescan threshold set to: %d downloads", threshold)
}

// NotifyDownloadComplete is called when a Spotify download completes
// Returns true if a rescan was triggered
func (s *Scanner) NotifyDownloadComplete() bool {
	s.rescanMutex.Lock()

	s.downloadsSinceLastScan++
	downloads := s.downloadsSinceLastScan
	threshold := s.rescanThreshold
	downloadDir := s.spotifyDownloadDir

	s.rescanMutex.Unlock()

	logger.Scanner("Download complete notification: %d/%d downloads since last scan", downloads, threshold)

	// Check if we need to rescan
	if downloads >= threshold && downloadDir != "" {
		// Check if download dir is within a library folder
		folders, err := s.db.GetScanFolders()
		if err != nil {
			logger.Scanner("Error getting scan folders: %v", err)
			return false
		}

		for _, folder := range folders {
			// Check if spotify download dir is inside or is one of the library folders
			if isSubPath(folder.Path, downloadDir) || isSubPath(downloadDir, folder.Path) {
				logger.Scanner("Triggering automatic rescan (download dir %s is within library folder %s)", downloadDir, folder.Path)
				go s.ScanAll()

				s.rescanMutex.Lock()
				s.downloadsSinceLastScan = 0
				s.rescanMutex.Unlock()

				return true
			}
		}
	}

	return false
}

// isSubPath checks if child is a subdirectory of parent
func isSubPath(parent, child string) bool {
	parent = filepath.Clean(parent)
	child = filepath.Clean(child)

	// Normalize to lowercase for case-insensitive comparison on Windows
	parent = strings.ToLower(parent)
	child = strings.ToLower(child)

	return strings.HasPrefix(child, parent)
}

// IsScanning returns whether a scan is currently in progress
func (s *Scanner) IsScanning() bool {
	s.scanMutex.RLock()
	defer s.scanMutex.RUnlock()
	return s.scanning
}

// GetProgress returns the current scan progress message
func (s *Scanner) GetProgress() string {
	s.scanMutex.RLock()
	defer s.scanMutex.RUnlock()
	return s.scanProgress
}

// setProgress updates the scan progress message
func (s *Scanner) setProgress(msg string) {
	s.scanMutex.Lock()
	s.scanProgress = msg
	s.scanMutex.Unlock()
	logger.Scanner("%s", msg)
}

// ScanAll scans all configured folders
func (s *Scanner) ScanAll() (*ScanResult, error) {
	s.scanMutex.Lock()
	if s.scanning {
		s.scanMutex.Unlock()
		return nil, fmt.Errorf("scan already in progress")
	}
	s.scanning = true
	s.scanMutex.Unlock()

	defer func() {
		s.scanMutex.Lock()
		s.scanning = false
		s.scanMutex.Unlock()
	}()

	// Emit scan started event
	s.emitEvent(LibraryEvent{
		Type:    "scan_started",
		Message: "Library scan started",
	})

	startTime := time.Now()
	s.setProgress("Loading configured folders...")

	folders, err := s.db.GetScanFolders()
	if err != nil {
		return nil, fmt.Errorf("failed to get scan folders: %w", err)
	}

	if len(folders) == 0 {
		s.setProgress("No folders configured")
		return &ScanResult{Duration: time.Since(startTime)}, nil
	}

	// Get all existing file paths from the database before scanning
	s.setProgress("Checking for removed files...")
	existingPaths, err := s.db.GetAllFilePaths()
	if err != nil {
		logger.Scanner("Failed to get existing file paths: %v", err)
		existingPaths = []string{}
	}

	// Build a set of paths that should exist (within configured folders)
	validFolderPaths := make(map[string]bool)
	for _, folder := range folders {
		validFolderPaths[strings.ToLower(filepath.Clean(folder.Path))] = true
	}

	// Track which existing paths are still valid (file exists and is within a scan folder)
	foundPaths := make(map[string]bool)

	// Clear the album cover cache for fresh scan
	s.albumCoverMutex.Lock()
	s.albumCovers = make(map[string]string)
	s.albumCoverMutex.Unlock()

	result := &ScanResult{}

	for _, folder := range folders {
		s.setProgress(fmt.Sprintf("Scanning: %s", folder.Path))

		folderResult, scannedPaths, err := s.ScanFolderWithPaths(folder.Path)
		if err != nil {
			logger.Scanner("Error scanning %s: %v", folder.Path, err)
			result.Errors++
			continue
		}

		// Mark all scanned paths as found
		for _, path := range scannedPaths {
			foundPaths[path] = true
		}

		result.TotalFiles += folderResult.TotalFiles
		result.NewSongs += folderResult.NewSongs
		result.UpdatedSongs += folderResult.UpdatedSongs
		result.Errors += folderResult.Errors

		// Update folder stats
		s.db.UpdateScanFolder(folder.ID, time.Now().UnixMilli(), folderResult.TotalFiles)
	}

	// Find and remove songs that no longer exist
	s.setProgress("Removing deleted files from library...")
	var pathsToRemove []string
	for _, existingPath := range existingPaths {
		// Check if the file was found during scanning
		if !foundPaths[existingPath] {
			// Also verify the file is within one of our configured scan folders
			// (don't remove songs from folders that were removed from config)
			pathLower := strings.ToLower(filepath.Clean(existingPath))
			isInScanFolder := false
			for folderPath := range validFolderPaths {
				if strings.HasPrefix(pathLower, folderPath) {
					isInScanFolder = true
					break
				}
			}
			if isInScanFolder {
				pathsToRemove = append(pathsToRemove, existingPath)
			}
		}
	}

	if len(pathsToRemove) > 0 {
		removed, err := s.db.DeleteSongsByFilePaths(pathsToRemove)
		if err != nil {
			logger.Scanner("Error removing deleted songs: %v", err)
		} else {
			result.RemovedSongs = removed
			logger.Scanner("Removed %d songs that no longer exist", removed)
		}
	}

	result.Duration = time.Since(startTime)
	s.setProgress(fmt.Sprintf("Scan complete: %d files found, %d new, %d updated, %d removed (%s)",
		result.TotalFiles, result.NewSongs, result.UpdatedSongs, result.RemovedSongs, result.Duration.Round(time.Millisecond)))

	// Reset download counter after scan
	s.rescanMutex.Lock()
	s.downloadsSinceLastScan = 0
	s.rescanMutex.Unlock()

	// Get total song count for the event
	totalSongs := 0
	if songs, err := s.db.GetAllSongs(); err == nil {
		totalSongs = len(songs)
	}

	// Emit scan complete event to notify frontend
	s.emitEvent(LibraryEvent{
		Type:         "scan_complete",
		Message:      fmt.Sprintf("Scan complete: %d new, %d removed", result.NewSongs, result.RemovedSongs),
		NewSongs:     result.NewSongs,
		RemovedSongs: result.RemovedSongs,
		TotalSongs:   totalSongs,
	})

	return result, nil
}

// ScanFolder scans a single folder for audio files
func (s *Scanner) ScanFolder(folderPath string) (*ScanResult, error) {
	result, _, err := s.ScanFolderWithPaths(folderPath)
	return result, err
}

// ScanFolderWithPaths scans a single folder for audio files and returns the list of scanned file paths
func (s *Scanner) ScanFolderWithPaths(folderPath string) (*ScanResult, []string, error) {
	result := &ScanResult{}
	var songs []db.Song
	var scannedPaths []string

	err := filepath.Walk(folderPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors
		}

		if info.IsDir() {
			return nil
		}

		ext := strings.ToLower(filepath.Ext(path))
		if !supportedExtensions[ext] {
			return nil
		}

		result.TotalFiles++
		scannedPaths = append(scannedPaths, path)

		song, err := s.extractMetadata(path)
		if err != nil {
			logger.Scanner("Failed to extract metadata from %s: %v", path, err)
			result.Errors++
			return nil
		}

		// Get or create album cover
		coverPath := s.getAlbumCover(song.Artist, song.Album, filepath.Dir(path), path)
		song.CoverPath = coverPath

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

	if err != nil {
		return nil, nil, err
	}

	// Save songs to database
	if len(songs) > 0 {
		if err := s.db.SaveSongs(songs); err != nil {
			return nil, nil, fmt.Errorf("failed to save songs: %w", err)
		}
		result.NewSongs = len(songs) // SimpliSfied - SaveSongs handles upsert

		// Create album_metadata entries for new albums discovered during scan
		s.createAlbumMetadataEntries(songs)
	}

	return result, scannedPaths, nil
}

// SongMetadata holds extracted metadata from an audio file
type SongMetadata struct {
	ID          string
	Title       string
	Artist      string
	Album       string
	AlbumArtist string
	TrackNumber int
	DiscNumber  int
	Genre       []string
	Year        int
	Duration    float64
	FilePath    string
	CoverPath   string
	CoverData   []byte
}

// extractMetadata extracts metadata from an audio file using taglib
func (s *Scanner) extractMetadata(filePath string) (*SongMetadata, error) {
	// Get file info for ID generation
	info, err := os.Stat(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to stat file: %w", err)
	}

	// Generate ID from file path and size
	hash := sha256.Sum256([]byte(fmt.Sprintf("%s:%d", filePath, info.Size())))
	id := hex.EncodeToString(hash[:8])

	// Try to read tags with taglib
	tags, err := taglib.ReadTags(filePath)
	if err != nil {
		// If we can't read tags, create minimal metadata from filename
		baseName := filepath.Base(filePath)
		title := strings.TrimSuffix(baseName, filepath.Ext(baseName))

		return &SongMetadata{
			ID:       id,
			Title:    title,
			Artist:   "Unknown Artist",
			Album:    "Unknown Album",
			FilePath: filePath,
		}, nil
	}

	// Read properties for duration
	props, err := taglib.ReadProperties(filePath)
	if err != nil {
		// Continue without duration
		props = taglib.Properties{}
	}

	// Helper to get first tag value
	getTag := func(key string) string {
		if vals, ok := tags[key]; ok && len(vals) > 0 {
			return vals[0]
		}
		return ""
	}

	// Get basic metadata
	song := &SongMetadata{
		ID:       id,
		Title:    getTag(taglib.Title),
		Artist:   getTag(taglib.Artist),
		Album:    getTag(taglib.Album),
		Duration: float64(props.Length) / float64(time.Second),
		FilePath: filePath,
	}

	// Year
	if yearStr := getTag(taglib.Date); yearStr != "" {
		// Parse year from date (might be YYYY or YYYY-MM-DD)
		if len(yearStr) >= 4 {
			if y, err := strconv.Atoi(yearStr[:4]); err == nil {
				song.Year = y
			}
		}
	}

	// Fallback title to filename
	if song.Title == "" {
		baseName := filepath.Base(filePath)
		song.Title = strings.TrimSuffix(baseName, filepath.Ext(baseName))
	}

	// Fallback artist/album
	if song.Artist == "" {
		song.Artist = "Unknown Artist"
	}
	if song.Album == "" {
		song.Album = "Unknown Album"
	}

	// Album artist
	if aa := getTag(taglib.AlbumArtist); aa != "" {
		song.AlbumArtist = aa
	}

	// Track number
	if trackStr := getTag(taglib.TrackNumber); trackStr != "" {
		// Handle "1/12" format
		if idx := strings.Index(trackStr, "/"); idx > 0 {
			trackStr = trackStr[:idx]
		}
		if t, err := strconv.Atoi(trackStr); err == nil {
			song.TrackNumber = t
		}
	}

	// Disc number
	if discStr := getTag(taglib.DiscNumber); discStr != "" {
		// Handle "1/2" format
		if idx := strings.Index(discStr, "/"); idx > 0 {
			discStr = discStr[:idx]
		}
		if d, err := strconv.Atoi(discStr); err == nil {
			song.DiscNumber = d
		}
	}

	// Genre
	if genre := getTag(taglib.Genre); genre != "" {
		song.Genre = []string{genre}
	}

	return song, nil
}

// getAlbumCover gets or creates album artwork for a song
// It checks for cached covers first, then looks for local cover files,
// then tries to extract embedded artwork from the audio file
func (s *Scanner) getAlbumCover(artist, album, folderPath, audioFilePath string) string {
	// Create album key for caching
	albumKey := fmt.Sprintf("%s|%s", strings.ToLower(artist), strings.ToLower(album))

	// Check if we already have a cover for this album
	s.albumCoverMutex.RLock()
	if coverPath, ok := s.albumCovers[albumKey]; ok {
		s.albumCoverMutex.RUnlock()
		return coverPath
	}
	s.albumCoverMutex.RUnlock()

	// Generate a unique cover filename based on album
	coverHash := sha256.Sum256([]byte(albumKey))
	coverFileName := hex.EncodeToString(coverHash[:8]) + ".jpg"
	coverPath := filepath.Join(s.coverDir, coverFileName)

	// Check if cover already exists on disk
	if _, err := os.Stat(coverPath); err == nil {
		s.albumCoverMutex.Lock()
		s.albumCovers[albumKey] = coverPath
		s.albumCoverMutex.Unlock()
		return coverPath
	}

	// Try to find local cover art file in the audio file's folder
	localCover := s.findLocalCover(folderPath)
	if localCover != "" {
		// Copy local cover to our cover cache
		if err := s.copyCoverFile(localCover, coverPath); err == nil {
			s.albumCoverMutex.Lock()
			s.albumCovers[albumKey] = coverPath
			s.albumCoverMutex.Unlock()
			return coverPath
		}
	}

	// Try to extract embedded artwork
	embeddedCover := s.extractEmbeddedArtwork(audioFilePath)
	if embeddedCover != nil {
		if err := os.WriteFile(coverPath, embeddedCover, 0644); err == nil {
			s.albumCoverMutex.Lock()
			s.albumCovers[albumKey] = coverPath
			s.albumCoverMutex.Unlock()
			return coverPath
		}
	}

	// No cover found
	return ""
}

// findLocalCover looks for cover art files in a directory
func (s *Scanner) findLocalCover(folderPath string) string {
	entries, err := os.ReadDir(folderPath)
	if err != nil {
		return ""
	}

	// Create a map of lowercase filenames to actual paths
	files := make(map[string]string)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		files[strings.ToLower(name)] = filepath.Join(folderPath, name)
	}

	// Check for common cover filenames
	for _, coverName := range coverFilenames {
		if path, ok := files[coverName]; ok {
			return path
		}
	}

	return ""
}

// copyCoverFile copies a cover image to the cache directory
func (s *Scanner) copyCoverFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	_, err = io.Copy(dstFile, srcFile)
	return err
}

// extractEmbeddedArtwork extracts cover art embedded in the audio file
func (s *Scanner) extractEmbeddedArtwork(filePath string) []byte {
	// Use taglib.ReadImage to get embedded artwork
	imageData, err := taglib.ReadImage(filePath)
	if err != nil || len(imageData) == 0 {
		return nil
	}
	return imageData
}

// CleanOrphanedCovers removes cover files that are no longer referenced by any song
func (s *Scanner) CleanOrphanedCovers() (int, error) {
	songs, err := s.db.GetAllSongs()
	if err != nil {
		return 0, err
	}

	// Build set of used cover paths
	usedCovers := make(map[string]bool)
	for _, song := range songs {
		if song.CoverPath != "" {
			usedCovers[song.CoverPath] = true
		}
	}

	// List all cover files
	entries, err := os.ReadDir(s.coverDir)
	if err != nil {
		return 0, err
	}

	deleted := 0
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		coverPath := filepath.Join(s.coverDir, entry.Name())
		if !usedCovers[coverPath] {
			os.Remove(coverPath)
			deleted++
		}
	}

	return deleted, nil
}

// createAlbumMetadataEntries creates album_metadata entries for albums found during scan
// This populates the cache with album info and local cover paths for later Spotify enrichment
func (s *Scanner) createAlbumMetadataEntries(songs []db.Song) {
	// Track albums we've already processed in this batch
	processedAlbums := make(map[string]bool)

	for _, song := range songs {
		// Create album key in the same format as frontend: "album::artist"
		albumKey := fmt.Sprintf("%s::%s", song.Album, song.Artist)

		// Skip if already processed in this batch
		if processedAlbums[albumKey] {
			continue
		}
		processedAlbums[albumKey] = true

		// Check if album_metadata entry already exists
		existing, _ := s.db.GetAlbumMetadata(albumKey)
		if existing != nil {
			// Update local_cover_path if we found a local cover and it's not set
			if song.CoverPath != "" && existing.LocalCoverPath == "" {
				// Find the original local cover file path
				localCover := s.findLocalCoverForSong(song.FilePath)
				if localCover != "" {
					if err := s.db.UpdateAlbumLocalCover(albumKey, localCover); err != nil {
						logger.Scanner("Failed to update local cover path for %s: %v", albumKey, err)
					}
				}
			}
			continue
		}

		// Find local cover file path (the original file, not the cached copy)
		localCoverPath := ""
		if song.CoverPath != "" {
			// Check if there's a local cover.jpg/folder.jpg in the song's directory
			localCoverPath = s.findLocalCoverForSong(song.FilePath)
		}

		// Create new album_metadata entry with local info only
		metadata := &db.AlbumMetadata{
			AlbumKey:       albumKey,
			AlbumName:      song.Album,
			ArtistName:     song.Artist,
			LocalCoverPath: localCoverPath,
			SpotifyChecked: false, // Not yet checked Spotify
			SpotifyFound:   false,
			FetchedAt:      0, // Not fetched from Spotify yet
		}

		if err := s.db.SaveAlbumMetadata(metadata); err != nil {
			logger.Scanner("Failed to save album metadata for %s: %v", albumKey, err)
		} else {
			logger.Scanner("Created album_metadata entry: %s (local cover: %s)", albumKey, localCoverPath)
		}
	}
}

// findLocalCoverForSong finds the original local cover file path for a song
func (s *Scanner) findLocalCoverForSong(audioFilePath string) string {
	folderPath := filepath.Dir(audioFilePath)
	return s.findLocalCover(folderPath)
}
