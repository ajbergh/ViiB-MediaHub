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
//   - Metadata enrichment: uses Last.FM or LLM based on user settings
//
// Supported audio extensions:
//
//	.mp3, .flac, .m4a, .aac, .ogg, .opus, .wav, .wma
package scanner

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/db"
	"github.com/ajbergh/viib-mediahub/internal/lastfm"
	"github.com/ajbergh/viib-mediahub/internal/llm"
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

// Directories to skip during scanning (case-insensitive prefix matching)
// These typically contain system files, snapshots, or hidden content
var skipDirectoryPrefixes = []string{
	"@", // Synology snapshots (@eaDir, @sharebin, @tmp, etc.)
	"$", // Windows system folders ($RECYCLE.BIN, $SysReset, etc.)
	".", // Hidden directories (.git, .svn, .cache, etc.)
	"#", // Temp/hash folders
}

// Directories to skip during scanning (exact name match, case-insensitive)
var skipDirectoryNames = map[string]bool{
	"$recycle.bin":              true,
	"system volume information": true,
	"recycler":                  true,
	"lost+found":                true,
	"node_modules":              true,
	"__pycache__":               true,
	"thumbs":                    true,
	".thumbnails":               true,
	".cache":                    true,
	".git":                      true,
	".svn":                      true,
	".hg":                       true,
	".ds_store":                 true,
	"desktop.ini":               true,
}

// shouldSkipDirectory checks if a directory should be skipped during scanning
func shouldSkipDirectory(dirName string) bool {
	lowerName := strings.ToLower(dirName)

	// Check exact name matches
	if skipDirectoryNames[lowerName] {
		return true
	}

	// Check prefix patterns
	for _, prefix := range skipDirectoryPrefixes {
		if strings.HasPrefix(lowerName, prefix) {
			return true
		}
	}

	return false
}

// pathContainsSkippedDirectory checks if a file path contains any directory that should be skipped.
// This is used to clean up songs that were added before skip logic was in place.
func pathContainsSkippedDirectory(filePath string) bool {
	// Split the path into directory components
	dir := filepath.Dir(filePath)
	for dir != "" && dir != "." && dir != "/" && dir != filepath.VolumeName(filePath)+"\\" {
		dirName := filepath.Base(dir)
		if shouldSkipDirectory(dirName) {
			return true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break // Reached root
		}
		dir = parent
	}
	return false
}

// LibraryEvent represents an event that can be sent to frontend clients
type LibraryEvent struct {
	Type         string                 `json:"type"`                   // "scan_complete", "scan_started", "scan_progress", "enrichment_started", "enrichment_progress", "enrichment_complete", "mood_started", "mood_progress", "mood_complete"
	Message      string                 `json:"message"`                // Human-readable message
	NewSongs     int                    `json:"newSongs,omitempty"`     // Number of new songs added (for scan_complete)
	UpdatedSongs int                    `json:"updatedSongs,omitempty"` // Number of songs updated (for library_updated)
	RemovedSongs int                    `json:"removedSongs,omitempty"` // Number of songs removed (for scan_complete)
	TotalSongs   int                    `json:"totalSongs,omitempty"`   // Total songs in library (for scan_complete)
	Data         map[string]interface{} `json:"data,omitempty"`         // Additional event-specific data
}

// EmitEvent allows external packages (like api) to emit events through the scanner's broadcast system
func (s *Scanner) EmitEvent(event LibraryEvent) {
	s.emitEvent(event)
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

	// Background enrichment
	enrichmentQueue        chan []db.Song
	enrichmentMutex        sync.Mutex
	enrichmentTotal        int
	enrichmentProcessed    int
	enrichmentBatchNum     int
	enrichmentTotalBatches int
	enrichmentActive       bool
	ctx                    context.Context
	cancel                 context.CancelFunc
	enrichmentWg           sync.WaitGroup
	closeOnce              sync.Once

	// Background scanner for incremental updates
	backgroundScanner *BackgroundScanner
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
	os.MkdirAll(coverDir, 0700)
	ctx, cancel := context.WithCancel(context.Background())

	s := &Scanner{
		db:              database,
		coverDir:        coverDir,
		dataDir:         dataDir,
		albumCovers:     make(map[string]string),
		rescanThreshold: 5, // Default: rescan after 5 downloads
		subscribers:     make(map[chan LibraryEvent]struct{}),
		enrichmentQueue: make(chan []db.Song, 1000), // Buffer for pending batches
		ctx:             ctx,
		cancel:          cancel,
	}

	// Initialize background scanner with workers scaled to CPU count
	numWorkers := runtime.NumCPU() / 2
	if numWorkers < 2 {
		numWorkers = 2
	}
	s.backgroundScanner = NewBackgroundScanner(s, numWorkers)
	s.backgroundScanner.Start()

	// Start background enrichment worker
	s.enrichmentWg.Add(1)
	go func() {
		defer s.enrichmentWg.Done()
		s.processEnrichmentQueue()
	}()

	return s
}

// Subscribe creates a new channel for receiving library events
// The caller should call Unsubscribe when done to prevent leaks
func (s *Scanner) Subscribe() chan LibraryEvent {
	ch := make(chan LibraryEvent, 10)
	s.subscriberMutex.Lock()
	s.subscribers[ch] = struct{}{}
	s.subscriberMutex.Unlock()
	logger.ScannerDebug("New subscriber added, total: %d", len(s.subscribers))
	return ch
}

// Unsubscribe removes a channel from receiving library events
func (s *Scanner) Unsubscribe(ch chan LibraryEvent) {
	s.subscriberMutex.Lock()
	delete(s.subscribers, ch)
	close(ch)
	s.subscriberMutex.Unlock()
	logger.ScannerDebug("Subscriber removed, total: %d", len(s.subscribers))
}

// emitEvent broadcasts an event to all subscribers (non-blocking)
func (s *Scanner) emitEvent(event LibraryEvent) {
	s.subscriberMutex.RLock()
	defer s.subscriberMutex.RUnlock()

	// Log library_updated and important events to main log for debugging
	if event.Type == "library_updated" || event.Type == "scan_started" || event.Type == "scan_complete" {
		logger.Scanner("SSE Broadcasting '%s' to %d subscribers: %s", event.Type, len(s.subscribers), event.Message)
	} else if event.Type != "background_progress" {
		logger.ScannerDebug("Broadcasting event to %d subscribers: %s - %s", len(s.subscribers), event.Type, event.Message)
	}

	for ch := range s.subscribers {
		select {
		case ch <- event:
		default:
			// Don't log dropped progress events to avoid log spam
			if event.Type != "background_progress" {
				logger.Scanner("Subscriber channel full, dropping event")
			}
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
				logger.Scanner("Triggering automatic quick scan (download dir %s is within library folder %s)", downloadDir, folder.Path)
				go s.performQuickScan()

				s.rescanMutex.Lock()
				s.downloadsSinceLastScan = 0
				s.rescanMutex.Unlock()

				return true
			}
		}
	}

	return false
}

// performQuickScan runs a quick scan with proper event handling for download completion
func (s *Scanner) performQuickScan() {
	if !s.TryBeginScan() {
		logger.Scanner("Quick scan skipped - scan already in progress")
		return
	}
	defer s.EndScan()

	s.emitEvent(LibraryEvent{
		Type:    "scan_started",
		Message: "Quick scan after download...",
	})

	quickResult, err := s.QuickStartup()
	if err != nil {
		logger.Scanner("Quick scan after download failed: %v", err)
		s.emitEvent(LibraryEvent{
			Type:    "scan_complete",
			Message: fmt.Sprintf("Quick scan failed: %v", err),
		})
		return
	}

	method := "signatures"
	if quickResult.UsedJournal {
		method = quickResult.JournalMethod
	}
	logger.Scanner("Quick scan complete in %s using %s: %d files changed",
		quickResult.ScanDuration, method, len(quickResult.ChangedFiles))

	// Also detect deleted files (files in cache that no longer exist on disk)
	s.emitEvent(LibraryEvent{
		Type:    "scan_progress",
		Message: "Checking for deleted files...",
	})
	deletedFiles, err := s.DetectDeletedFiles()
	if err != nil {
		logger.Scanner("Error detecting deleted files: %v", err)
	} else if len(deletedFiles) > 0 {
		logger.Scanner("Detected %d deleted files", len(deletedFiles))
		quickResult.ChangedFiles = append(quickResult.ChangedFiles, deletedFiles...)
	}

	if len(quickResult.ChangedFiles) > 0 {
		s.emitEvent(LibraryEvent{
			Type:    "scan_progress",
			Message: fmt.Sprintf("Processing %d changed files...", len(quickResult.ChangedFiles)),
		})

		result, err := s.ProcessChanges(quickResult.ChangedFiles)
		if err != nil {
			logger.Scanner("Error processing changes: %v", err)
		} else {
			logger.Scanner("Quick scan processed: %d added, %d updated, %d deleted",
				result.NewSongs, result.UpdatedSongs, result.RemovedSongs)
		}
	}

	s.emitEvent(LibraryEvent{
		Type:    "scan_complete",
		Message: fmt.Sprintf("Quick scan complete: %d files changed", len(quickResult.ChangedFiles)),
	})

	s.emitEvent(LibraryEvent{
		Type: "library_updated",
	})
}

// isSubPath checks if child is a subdirectory of parent
func isSubPath(parent, child string) bool {
	parent = filepath.Clean(parent)
	child = filepath.Clean(child)
	rel, err := filepath.Rel(parent, child)
	if err != nil || filepath.IsAbs(rel) {
		return false
	}
	if rel == "." {
		return true
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// IsScanning returns whether a scan is currently in progress
func (s *Scanner) IsScanning() bool {
	s.scanMutex.RLock()
	defer s.scanMutex.RUnlock()
	return s.scanning
}

// SetScanning sets the scanning state (used by quick scan)
func (s *Scanner) SetScanning(scanning bool) {
	s.scanMutex.Lock()
	s.scanning = scanning
	s.scanMutex.Unlock()
}

// TryBeginScan atomically acquires scan ownership.
func (s *Scanner) TryBeginScan() bool {
	s.scanMutex.Lock()
	defer s.scanMutex.Unlock()
	if s.scanning {
		return false
	}
	s.scanning = true
	return true
}

// EndScan releases scan ownership.
func (s *Scanner) EndScan() {
	s.SetScanning(false)
}

// GetProgress returns the current scan progress message
func (s *Scanner) GetProgress() string {
	s.scanMutex.RLock()
	defer s.scanMutex.RUnlock()
	return s.scanProgress
}

// SetProgress updates the progress string returned by /scan/status.
// It does not emit an SSE event (call emitEvent/setProgress separately if needed).
func (s *Scanner) SetProgress(msg string) {
	s.scanMutex.Lock()
	s.scanProgress = msg
	s.scanMutex.Unlock()
}

// setProgress updates the scan progress message
func (s *Scanner) setProgress(msg string) {
	s.scanMutex.Lock()
	s.scanProgress = msg
	s.scanMutex.Unlock()
	logger.Scanner("%s", msg)

	s.emitEvent(LibraryEvent{
		Type:    "scan_progress",
		Message: msg,
	})
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

	// A root is eligible for destructive reconciliation only after a complete,
	// error-free traversal. Temporary NAS, USB, permission, or mount failures
	// must never delete valid library records.
	deletionSafeFolderPaths := make(map[string]bool)

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
		if folderResult.Errors == 0 {
			deletionSafeFolderPaths[filepath.Clean(folder.Path)] = true
		} else {
			logger.Scanner("Skipping destructive reconciliation for %s because traversal reported %d errors", folder.Path, folderResult.Errors)
		}

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
			isInScanFolder := false
			for folderPath := range deletionSafeFolderPaths {
				if isSubPath(folderPath, existingPath) {
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

	// Check enrichment source setting to determine enrichment method
	enrichmentSource, _ := s.db.GetSetting("enrichment_source")
	if enrichmentSource == "" {
		enrichmentSource = "ai" // Default to AI
	}

	logger.Scanner("Enrichment source configured: %s", enrichmentSource)

	// Handle Last.FM enrichment
	if enrichmentSource == "lastfm" || enrichmentSource == "hybrid" {
		// Check if Last.FM is enabled and configured
		lastfmEnabled, _ := s.db.GetSetting("lastfm_enabled")
		lastfmAPIKey, _ := s.db.GetSetting("lastfm_api_key")

		if lastfmEnabled == "true" && lastfmAPIKey != "" {
			logger.Scanner("Using Last.FM for metadata enrichment...")
			s.enrichWithLastFM()
			if enrichmentSource == "hybrid" {
				// Last.FM only updates tracks it can match. Re-query afterward so
				// the LLM receives just unresolved or under-enriched tracks.
				logger.Scanner("Running AI fallback for unresolved Last.FM tracks...")
				s.enrichWithLLM()
			}
		} else if enrichmentSource == "hybrid" {
			logger.Scanner("Last.FM not configured, falling back to AI enrichment...")
			s.enrichWithLLM()
		} else {
			logger.Scanner("Last.FM enrichment selected but not configured")
		}
	} else {
		// Use AI/LLM enrichment
		s.enrichWithLLM()
	}

	// Reset download counter after scan
	s.rescanMutex.Lock()
	s.downloadsSinceLastScan = 0
	s.rescanMutex.Unlock()

	// Update genre stats cache
	s.setProgress("Updating genre statistics...")
	if err := s.db.UpdateGenreStats(); err != nil {
		logger.Scanner("Failed to update genre stats: %v", err)
	}

	// Get total song count for the event
	totalSongs := 0
	if songs, err := s.db.GetAllSongs(); err == nil {
		totalSongs = len(songs)
	}

	// Save scan state for fast startup
	s.setProgress("Saving scan state for fast startup...")
	if err := s.SaveScanState(result); err != nil {
		logger.Scanner("Failed to save scan state: %v", err)
	}

	// Compute and save directory signatures for fast change detection
	s.setProgress("Computing directory signatures...")
	if err := s.ComputeAndSaveDirectorySignatures(); err != nil {
		logger.Scanner("Failed to save directory signatures: %v", err)
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
	ignoredPathSet := make(map[string]struct{})
	if ignoredPaths, err := s.db.GetIgnoredFilePaths(); err == nil {
		for _, ignoredPath := range ignoredPaths {
			ignoredPathSet[filepath.Clean(ignoredPath)] = struct{}{}
		}
	}
	fileCount := 0
	const batchSize = 50

	err := filepath.Walk(folderPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			// Continue discovery, but mark this root unsafe for deletion reconciliation.
			logger.Scanner("Error accessing %s: %v", path, err)
			result.Errors++
			return nil
		}

		if info.IsDir() {
			// Check if this directory should be skipped
			dirName := info.Name()
			if shouldSkipDirectory(dirName) {
				logger.Scanner("Skipping directory: %s", path)
				return filepath.SkipDir
			}
			return nil
		}

		ext := strings.ToLower(filepath.Ext(path))
		if !supportedExtensions[ext] {
			return nil
		}
		if _, ignored := ignoredPathSet[filepath.Clean(path)]; ignored {
			return nil
		}

		result.TotalFiles++
		fileCount++

		// Log progress every 50 files
		if fileCount%50 == 0 {
			logger.Scanner("Scanned %d files so far in %s", fileCount, folderPath)
		}

		scannedPaths = append(scannedPaths, path)

		song, err := s.extractMetadata(path)
		if err != nil {
			logger.Scanner("Failed to extract metadata from %s: %v", path, err)
			result.Errors++
			return nil
		}
		resolvedID, err := s.db.ResolveSongIdentity(path, song.FileHash, song.ID)
		if err != nil {
			logger.Scanner("Failed to resolve stable identity for %s: %v", path, err)
			result.Errors++
			return nil
		}
		song.ID = resolvedID

		// Get or create album cover
		coverArtist := song.AlbumArtist
		if coverArtist == "" {
			coverArtist = song.Artist
		}
		coverPath := s.getAlbumCover(coverArtist, song.Album, filepath.Dir(path), path)
		song.CoverPath = coverPath

		dbSong := db.Song{
			ID:           song.ID,
			Title:        song.Title,
			Artist:       song.Artist,
			Album:        song.Album,
			AlbumArtist:  song.AlbumArtist,
			TrackNumber:  song.TrackNumber,
			DiscNumber:   song.DiscNumber,
			Genre:        song.Genre,
			Year:         song.Year,
			Duration:     song.Duration,
			ReplayGainDB: song.ReplayGainDB,
			ReplayPeak:   song.ReplayPeak,
			FilePath:     path,
			CoverPath:    song.CoverPath,
			AddedAt:      time.Now().UnixMilli(),
			FileHash:     song.FileHash,
		}

		songs = append(songs, dbSong)

		// Save batch if we reached the limit
		if len(songs) >= batchSize {
			logger.Scanner("Saving batch of %d songs...", len(songs))
			upsert, err := s.db.SaveSongsWithResult(songs)
			if err != nil {
				logger.Scanner("ERROR saving batch to database: %v", err)
				result.Errors++
				// Continue discovery, but make the root ineligible for deletion reconciliation.
			} else {
				result.NewSongs += upsert.Inserted
				result.UpdatedSongs += upsert.Updated
				s.createAlbumMetadataEntries(songs)

				// Note: Genre enrichment is handled after scan completes in ScanAll
				// by querying the database for songs with missing genres

				// Emit update event for real-time UI updates
				s.emitEvent(LibraryEvent{
					Type:         "library_updated",
					Message:      fmt.Sprintf("Library updated: %d added, %d updated", upsert.Inserted, upsert.Updated),
					NewSongs:     upsert.Inserted,
					UpdatedSongs: upsert.Updated,
				})

				// Update genre stats periodically during scan
				go s.db.UpdateGenreStats()
			}
			// Reset batch
			songs = []db.Song{}
		}

		return nil
	})

	if err != nil {
		logger.Scanner("Error during folder walk of %s: %v", folderPath, err)
		return nil, nil, fmt.Errorf("failed to walk folder: %w", err)
	}

	// Save any remaining songs
	if len(songs) > 0 {
		logger.Scanner("Saving final batch of %d songs...", len(songs))
		upsert, err := s.db.SaveSongsWithResult(songs)
		if err != nil {
			logger.Scanner("ERROR saving final batch to database: %v", err)
			return nil, nil, fmt.Errorf("failed to save songs batch: %w", err)
		}
		result.NewSongs += upsert.Inserted
		result.UpdatedSongs += upsert.Updated
		s.createAlbumMetadataEntries(songs)

		// Note: Genre enrichment is handled after scan completes in ScanAll
		// by querying the database for songs with missing genres

		// Emit final update
		s.emitEvent(LibraryEvent{
			Type:         "library_updated",
			Message:      fmt.Sprintf("Library updated: %d added, %d updated", upsert.Inserted, upsert.Updated),
			NewSongs:     upsert.Inserted,
			UpdatedSongs: upsert.Updated,
		})
	}

	// Update file metadata cache for fast startup
	if len(scannedPaths) > 0 {
		if err := s.UpdateFileMetadataCache(scannedPaths); err != nil {
			logger.Scanner("Failed to update metadata cache for %s: %v", folderPath, err)
		}
	}

	logger.Scanner("Completed scan of %s: %d files, %d errors", folderPath, fileCount, result.Errors)
	return result, scannedPaths, nil
}

// enrichWithLastFM enriches songs using Last.FM API.
// Gets songs needing enrichment and uses Last.FM tags to update genres.
func (s *Scanner) enrichWithLastFM() {
	// Get songs needing enrichment (those without genres or Last.FM data)
	songsToEnrich, err := s.db.GetSongsWithMissingGenres(10000)
	if err != nil {
		logger.Scanner("Error getting songs for Last.FM enrichment: %v", err)
		return
	}

	if len(songsToEnrich) == 0 {
		logger.Scanner("No songs need enrichment")
		return
	}

	// Get Last.FM API key
	apiKey, _ := s.db.GetSetting("lastfm_api_key")
	if apiKey == "" {
		logger.Scanner("Last.FM API key not configured")
		return
	}

	// Create Last.FM client
	client := lastfm.NewClient(apiKey, "")
	enricher := lastfm.NewEnricher(client, s.db)

	totalToEnrich := len(songsToEnrich)
	batchSize := 50 // Process in batches for progress updates
	totalBatches := (totalToEnrich + batchSize - 1) / batchSize

	logger.Scanner("Starting Last.FM enrichment for %d songs (%d batches)...", totalToEnrich, totalBatches)

	// Emit enrichment started event
	s.emitEvent(LibraryEvent{
		Type:    "enrichment_started",
		Message: fmt.Sprintf("Starting Last.FM enrichment for %d songs", totalToEnrich),
		Data: map[string]interface{}{
			"totalSongs":   totalToEnrich,
			"totalBatches": totalBatches,
		},
	})

	totalEnriched := 0

	for batch := 0; batch < totalBatches; batch++ {
		start := batch * batchSize
		end := start + batchSize
		if end > totalToEnrich {
			end = totalToEnrich
		}

		batchSongs := songsToEnrich[start:end]

		// Emit progress event
		s.emitEvent(LibraryEvent{
			Type:    "enrichment_progress",
			Message: fmt.Sprintf("Last.FM enrichment batch %d of %d", batch+1, totalBatches),
			Data: map[string]interface{}{
				"currentBatch":   batch + 1,
				"totalBatches":   totalBatches,
				"processedSongs": totalEnriched,
				"totalSongs":     totalToEnrich,
			},
		})

		// Enrich batch with Last.FM
		result, err := enricher.EnrichSongs(context.Background(), batchSongs, lastfm.EnrichOptions{
			MinTagCount:    30,
			MaxConcurrency: 3,
			FetchSimilar:   false, // Don't fetch similar tracks during scan for speed
		})
		if err != nil {
			logger.Scanner("Last.FM enrichment error for batch %d: %v", batch+1, err)
			continue
		}

		totalEnriched += result.Enriched
		logger.Scanner("Batch %d/%d: Enriched %d songs via Last.FM (total: %d/%d)",
			batch+1, totalBatches, result.Enriched, totalEnriched, totalToEnrich)

		// Emit progress after batch
		s.emitEvent(LibraryEvent{
			Type:    "enrichment_progress",
			Message: fmt.Sprintf("Completed Last.FM batch %d of %d (%d songs)", batch+1, totalBatches, result.Enriched),
			Data: map[string]interface{}{
				"currentBatch":   batch + 1,
				"totalBatches":   totalBatches,
				"processedSongs": totalEnriched,
				"totalSongs":     totalToEnrich,
			},
		})
	}

	if totalEnriched > 0 {
		s.emitEvent(LibraryEvent{
			Type:    "enrichment_complete",
			Message: fmt.Sprintf("Last.FM enriched %d songs", totalEnriched),
			Data: map[string]interface{}{
				"enrichedSongs": totalEnriched,
				"totalSongs":    totalToEnrich,
			},
		})
		logger.Scanner("Last.FM enrichment complete: %d songs enriched", totalEnriched)
		s.db.UpdateGenreStats()
	}
}

// enrichWithLLM enriches songs using AI/LLM provider.
// Falls back gracefully if no LLM is configured.
func (s *Scanner) enrichWithLLM() {
	provider, err := llm.GetConfiguredProvider(s.db)
	if err != nil {
		logger.Scanner("No LLM provider configured for enrichment: %v", err)
		return
	}
	defer provider.Close()

	// Snapshot every track that needs any AI field before applying updates.
	// Paging after writes would skip rows as they stop matching this query.
	allSongsToEnrich := make([]db.Song, 0)
	const queryPageSize = 500
	for offset := 0; ; offset += queryPageSize {
		page, queryErr := s.db.GetSongsForAIEnrichment(queryPageSize, false, offset)
		if queryErr != nil {
			logger.Scanner("Failed to query songs for unified AI enrichment: %v", queryErr)
			return
		}
		allSongsToEnrich = append(allSongsToEnrich, page...)
		if len(page) < queryPageSize {
			break
		}
	}
	if len(allSongsToEnrich) == 0 {
		logger.Scanner("No songs need LLM enrichment")
		return
	}

	totalToEnrich := len(allSongsToEnrich)
	batchSize := provider.GetOptimalBatchSize()
	totalBatches := (totalToEnrich + batchSize - 1) / batchSize

	logger.Scanner("Starting unified LLM enrichment for %d songs (%d batches) using %s...",
		totalToEnrich, totalBatches, provider.GetProviderName())
	s.setProgress(fmt.Sprintf("AI enrichment: preparing %d songs with %s", totalToEnrich, provider.GetProviderName()))

	// Emit enrichment started event
	s.emitEvent(LibraryEvent{
		Type:    "enrichment_started",
		Message: fmt.Sprintf("AI enrichment: preparing %d songs with %s", totalToEnrich, provider.GetProviderName()),
		Data: map[string]interface{}{
			"totalSongs":   totalToEnrich,
			"totalBatches": totalBatches,
		},
	})

	processedSongs := 0
	changedSongs := 0
	emptyResults := 0
	genresUpdated := 0
	moodUpdated := 0
	yearsUpdated := 0
	failedBatches := 0

	for batch := 0; batch < totalBatches; batch++ {
		start := batch * batchSize
		end := start + batchSize
		if end > totalToEnrich {
			end = totalToEnrich
		}

		songsToEnrich := allSongsToEnrich[start:end]

		progressMessage := fmt.Sprintf("AI enrichment: batch %d of %d using %s", batch+1, totalBatches, provider.GetProviderName())
		s.setProgress(progressMessage)

		// Emit progress event before processing.
		s.emitEvent(LibraryEvent{
			Type:    "enrichment_progress",
			Message: progressMessage,
			Data: map[string]interface{}{
				"currentBatch":   batch + 1,
				"totalBatches":   totalBatches,
				"processedSongs": processedSongs,
				"totalSongs":     totalToEnrich,
			},
		})

		unified, err := provider.EnrichAllMetadata(context.Background(), songsToEnrich)
		if err != nil {
			failedBatches++
			logger.Scanner("Unified LLM enrichment failed for batch %d: %v", batch+1, err)
			continue
		}

		updates := make([]db.AIEnrichmentUpdate, 0, len(unified))
		batchEmptyResults := 0
		for _, song := range songsToEnrich {
			metadata := unified[song.ID]
			if metadata == nil {
				batchEmptyResults++
				continue
			}
			if !metadata.HasMetadata() {
				batchEmptyResults++
			}
			updates = append(updates, db.AIEnrichmentUpdate{
				SongID: song.ID, Genres: metadata.Genres, Mood: metadata.Mood,
				Energy: metadata.Energy, Tempo: metadata.Tempo, BPM: metadata.BPM,
				Instrumental: metadata.Instrumental, OriginalYear: metadata.OriginalYear,
			})
		}
		applied, applyErr := s.db.ApplyAIEnrichmentBatch(updates, false)
		if applyErr != nil {
			failedBatches++
			logger.Scanner("Failed to apply unified AI batch %d: %v", batch+1, applyErr)
			continue
		}

		processedSongs += len(songsToEnrich)
		changedSongs += applied.Songs
		emptyResults += batchEmptyResults
		genresUpdated += applied.Genres
		moodUpdated += applied.Mood
		yearsUpdated += applied.Years

		if applied.Genres > 0 {
			if err := s.db.UpdateGenreStats(); err != nil {
				logger.Scanner("Failed to refresh genre aggregates after AI batch %d: %v", batch+1, err)
			}
		}

		completedMessage := fmt.Sprintf("AI batch %d/%d complete: changed=%d, no metadata=%d, genres=%d, mood/BPM=%d, years=%d", batch+1, totalBatches, applied.Songs, batchEmptyResults, applied.Genres, applied.Mood, applied.Years)
		s.setProgress(completedMessage)
		logger.Scanner("%s (processed: %d/%d)", completedMessage, processedSongs, totalToEnrich)
		s.emitEvent(LibraryEvent{
			Type:         "library_updated",
			Message:      fmt.Sprintf("AI enrichment batch %d committed", batch+1),
			UpdatedSongs: applied.Songs,
			Data: map[string]interface{}{
				"currentBatch": batch + 1, "totalBatches": totalBatches,
				"processedSongs": processedSongs, "totalSongs": totalToEnrich,
				"changedSongs": applied.Songs, "emptyResults": batchEmptyResults,
			},
		})

		// Emit progress after the transaction commits so the renderer can safely
		// reload the newly enriched song rows immediately.
		s.emitEvent(LibraryEvent{
			Type:    "enrichment_progress",
			Message: completedMessage,
			Data: map[string]interface{}{
				"currentBatch":   batch + 1,
				"totalBatches":   totalBatches,
				"processedSongs": processedSongs,
				"totalSongs":     totalToEnrich,
				"changedSongs":   changedSongs,
				"emptyResults":   emptyResults,
			},
		})
	}

	completionMessage := fmt.Sprintf("AI enrichment complete: changed=%d, no metadata=%d, genres=%d, mood/BPM=%d, years=%d", changedSongs, emptyResults, genresUpdated, moodUpdated, yearsUpdated)
	if failedBatches > 0 {
		completionMessage += fmt.Sprintf(" (%d batches failed)", failedBatches)
	}
	s.setProgress(completionMessage)
	s.emitEvent(LibraryEvent{
		Type:    "enrichment_complete",
		Message: completionMessage,
		Data: map[string]interface{}{
			"enrichedSongs":  processedSongs,
			"processedSongs": processedSongs,
			"totalSongs":     totalToEnrich,
			"currentBatch":   totalBatches,
			"totalBatches":   totalBatches,
			"changedSongs":   changedSongs,
			"emptyResults":   emptyResults,
		},
	})
	logger.Scanner("Unified LLM enrichment complete via %s: changed=%d, no metadata=%d, genres=%d, mood=%d, years=%d, failed batches=%d", provider.GetProviderName(), changedSongs, emptyResults, genresUpdated, moodUpdated, yearsUpdated, failedBatches)
}

// SongMetadata holds extracted metadata from an audio file
type SongMetadata struct {
	ID           string
	Title        string
	Artist       string
	Album        string
	AlbumArtist  string
	TrackNumber  int
	DiscNumber   int
	Genre        []string
	Year         int
	Duration     float64
	ReplayGainDB float64
	ReplayPeak   float64
	FilePath     string
	CoverPath    string
	CoverData    []byte
	FileHash     string
}

// extractMetadata extracts metadata from an audio file using taglib
// Uses a timeout to prevent hanging on inaccessible or problematic files
func (s *Scanner) extractMetadata(filePath string) (*SongMetadata, error) {
	// Use a channel to handle timeouts
	type metadataResult struct {
		metadata *SongMetadata
		err      error
	}
	resultChan := make(chan metadataResult, 1)

	// Run metadata extraction in a goroutine with timeout
	go func() {
		// Get file info for ID generation
		info, err := os.Stat(filePath)
		if err != nil {
			resultChan <- metadataResult{nil, fmt.Errorf("failed to stat file: %w", err)}
			return
		}

		fingerprint, err := computeMediaFingerprint(filePath, info)
		if err != nil {
			resultChan <- metadataResult{nil, fmt.Errorf("failed to fingerprint file: %w", err)}
			return
		}
		id := proposedSongID(fingerprint, filePath)

		// Try to read tags with taglib
		tags, err := taglib.ReadTags(filePath)
		if err != nil {
			// If we can't read tags, create minimal metadata from filename
			baseName := filepath.Base(filePath)
			title := strings.TrimSuffix(baseName, filepath.Ext(baseName))

			resultChan <- metadataResult{&SongMetadata{
				ID:       id,
				Title:    title,
				Artist:   "Unknown Artist",
				Album:    "Unknown Album",
				FilePath: filePath,
				FileHash: fingerprint,
			}, nil}
			return
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

		getTagFold := func(keys ...string) string {
			for actualKey, values := range tags {
				for _, key := range keys {
					if strings.EqualFold(actualKey, key) && len(values) > 0 {
						return values[0]
					}
				}
			}
			return ""
		}
		parseReplayValue := func(value string) float64 {
			value = strings.TrimSpace(strings.TrimSuffix(strings.TrimSuffix(value, " dB"), "dB"))
			parsed, _ := strconv.ParseFloat(value, 64)
			return parsed
		}

		// Get basic metadata
		song := &SongMetadata{
			ID:       id,
			FileHash: fingerprint,
			Title:    getTag(taglib.Title),
			Artist:   getTag(taglib.Artist),
			Album:    getTag(taglib.Album),
			Duration: float64(props.Length) / float64(time.Second),
			FilePath: filePath,
		}

		if value := getTagFold("REPLAYGAIN_TRACK_GAIN"); value != "" {
			song.ReplayGainDB = parseReplayValue(value)
		} else if value := getTagFold("R128_TRACK_GAIN"); value != "" {
			// Opus R128 gain is stored in Q7.8 dB units.
			song.ReplayGainDB = parseReplayValue(value) / 256
		}
		if value := getTagFold("REPLAYGAIN_TRACK_PEAK"); value != "" {
			song.ReplayPeak = parseReplayValue(value)
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

		// Genre - split comma-separated genres into individual elements
		// Many audio files store genres as "Rock, Alternative Rock, Grunge" in a single tag
		// We split these into separate array elements for proper individual genre tracking
		if genre := getTag(taglib.Genre); genre != "" {
			// Split by comma and trim whitespace from each genre
			genreParts := strings.Split(genre, ",")
			genres := make([]string, 0, len(genreParts))
			for _, g := range genreParts {
				trimmed := strings.TrimSpace(g)
				if trimmed != "" {
					genres = append(genres, trimmed)
				}
			}
			if len(genres) > 0 {
				song.Genre = genres
			}
		}

		resultChan <- metadataResult{song, nil}
	}()

	// Wait for result or timeout (30 seconds per file)
	select {
	case result := <-resultChan:
		return result.metadata, result.err
	case <-time.After(30 * time.Second):
		logger.Scanner("Timeout reading metadata from %s (30s), skipping file", filePath)
		// Return minimal metadata to allow scan to continue
		baseName := filepath.Base(filePath)
		title := strings.TrimSuffix(baseName, filepath.Ext(baseName))
		hash := sha256.Sum256([]byte(fmt.Sprintf("%s:timeout", filePath)))
		id := hex.EncodeToString(hash[:8])
		return &SongMetadata{
			ID:       id,
			Title:    title,
			Artist:   "Unknown Artist",
			Album:    "Unknown Album",
			FilePath: filePath,
		}, nil
	}
}

// getAlbumCover gets or creates album artwork for a song
// It uses content-based hashing to deduplicate identical cover art.
// Multiple albums with the same cover art will share a single cached file.
func (s *Scanner) getAlbumCover(artist, album, folderPath, audioFilePath string) string {
	// Create album key for caching (to avoid re-processing the same album)
	albumKey := fmt.Sprintf("%s|%s", strings.ToLower(artist), strings.ToLower(album))

	// Check if we already have a cover path cached for this album
	s.albumCoverMutex.RLock()
	if coverPath, ok := s.albumCovers[albumKey]; ok {
		s.albumCoverMutex.RUnlock()
		return coverPath
	}
	s.albumCoverMutex.RUnlock()

	// Try to find local cover art file in the audio file's folder
	localCover := s.findLocalCover(folderPath)
	if localCover != "" {
		// Get content-based path (deduplicates identical covers)
		coverPath := s.saveCoverWithContentHash(localCover, nil)
		if coverPath != "" {
			s.albumCoverMutex.Lock()
			s.albumCovers[albumKey] = coverPath
			s.albumCoverMutex.Unlock()
			return coverPath
		}
	}

	// Try to extract embedded artwork
	embeddedCover := s.extractEmbeddedArtwork(audioFilePath)
	if embeddedCover != nil {
		// Get content-based path (deduplicates identical covers)
		coverPath := s.saveCoverWithContentHash("", embeddedCover)
		if coverPath != "" {
			s.albumCoverMutex.Lock()
			s.albumCovers[albumKey] = coverPath
			s.albumCoverMutex.Unlock()
			return coverPath
		}
	}

	// No cover found
	return ""
}

// saveCoverWithContentHash saves cover art using a content-based hash as filename.
// This deduplicates identical covers - if the same image is used by multiple albums,
// only one copy is stored. Pass either srcPath (file path) or data (raw bytes), not both.
func (s *Scanner) saveCoverWithContentHash(srcPath string, data []byte) string {
	var coverData []byte
	var err error
	if srcPath != "" {
		coverData, err = os.ReadFile(srcPath)
		if err != nil {
			logger.Scanner("Failed to read cover file %s: %v", srcPath, err)
			return ""
		}
	} else if len(data) > 0 {
		coverData = data
	} else {
		return ""
	}

	contentType := http.DetectContentType(coverData)
	extension := ""
	switch contentType {
	case "image/jpeg":
		extension = ".jpg"
	case "image/png":
		extension = ".png"
	case "image/gif":
		extension = ".gif"
	case "image/webp":
		extension = ".webp"
	default:
		logger.Scanner("Unsupported cover content type %s", contentType)
		return ""
	}

	contentHash := sha256.Sum256(coverData)
	coverPath := filepath.Join(s.coverDir, hex.EncodeToString(contentHash[:8])+extension)
	if _, err := os.Stat(coverPath); err == nil {
		return coverPath
	}
	if err := os.WriteFile(coverPath, coverData, 0o600); err != nil {
		logger.Scanner("Failed to write cover file %s: %v", coverPath, err)
		return ""
	}
	return coverPath
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

// extractEmbeddedArtwork extracts cover art embedded in the audio file
func (s *Scanner) extractEmbeddedArtwork(filePath string) []byte {
	// Use taglib.ReadImage to get embedded artwork
	imageData, err := taglib.ReadImage(filePath)
	if err != nil || len(imageData) == 0 {
		return nil
	}
	return imageData
}

// createAlbumMetadataEntries creates album_metadata entries for albums found during scan
// This populates the cache with album info and local cover paths for later Spotify enrichment
func (s *Scanner) createAlbumMetadataEntries(songs []db.Song) {
	// Track albums we've already processed in this batch
	processedAlbums := make(map[string]bool)

	for _, song := range songs {
		// Create album key in the same format as frontend: "album::albumArtist".
		albumArtist := song.AlbumArtist
		if albumArtist == "" {
			albumArtist = song.Artist
		}
		albumKey := fmt.Sprintf("%s::%s", song.Album, albumArtist)

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
			ArtistName:     albumArtist,
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

// processEnrichmentQueue handles background enrichment of songs.
func (s *Scanner) processEnrichmentQueue() {
	logger.ScannerDebug("Enrichment worker started")
	for {
		var batch []db.Song
		select {
		case <-s.ctx.Done():
			return
		case batch = <-s.enrichmentQueue:
		}

		provider, err := llm.GetConfiguredProvider(s.db)
		if err != nil {
			continue
		}

		s.enrichmentMutex.Lock()
		s.enrichmentBatchNum++
		currentBatch := s.enrichmentBatchNum
		totalBatches := s.enrichmentTotalBatches
		totalSongs := s.enrichmentTotal
		isFirstBatch := currentBatch == 1
		s.enrichmentMutex.Unlock()

		if isFirstBatch {
			logger.Scanner("Starting enrichment of %d songs needing genres across %d batches using %s", totalSongs, totalBatches, provider.GetProviderName())
			s.emitEvent(LibraryEvent{
				Type:    "enrichment_started",
				Message: fmt.Sprintf("Enriching %d songs", totalSongs),
				Data:    map[string]interface{}{"totalSongs": totalSongs, "totalBatches": totalBatches},
			})
		}

		for attempt := 0; attempt < 3; attempt++ {
			enrichedGenres, enrichErr := provider.EnrichGenres(s.ctx, batch)
			if enrichErr == nil {
				count := 0
				for songID, genres := range enrichedGenres {
					if err := s.db.UpdateSongGenres(songID, genres); err == nil {
						count++
					}
				}
				if count > 0 {
					s.enrichmentMutex.Lock()
					s.enrichmentProcessed += count
					processedNow := s.enrichmentProcessed
					isComplete := s.enrichmentBatchNum >= s.enrichmentTotalBatches
					if isComplete {
						s.enrichmentActive = false
					}
					s.enrichmentMutex.Unlock()
					if err := s.db.UpdateGenreStats(); err != nil {
						logger.Scanner("Failed to update genre stats after enrichment: %v", err)
					}
					s.emitEvent(LibraryEvent{
						Type:    "enrichment_progress",
						Message: fmt.Sprintf("Enriched batch %d/%d (%d songs)", currentBatch, totalBatches, count),
						Data: map[string]interface{}{
							"processedSongs": processedNow,
							"totalSongs":     totalSongs,
							"currentBatch":   currentBatch,
							"totalBatches":   totalBatches,
						},
					})
					if isComplete {
						s.emitEvent(LibraryEvent{
							Type:    "enrichment_complete",
							Message: fmt.Sprintf("Genre enrichment complete: %d songs", processedNow),
							Data:    map[string]interface{}{"processedSongs": processedNow, "totalSongs": totalSongs},
						})
					}
				}
				break
			}

			if s.ctx.Err() != nil {
				provider.Close()
				return
			}
			if strings.Contains(enrichErr.Error(), "429") || strings.Contains(strings.ToLower(enrichErr.Error()), "quota") {
				logger.Scanner("LLM rate limit hit, waiting 30s (attempt %d/3)...", attempt+1)
				select {
				case <-s.ctx.Done():
					provider.Close()
					return
				case <-time.After(30 * time.Second):
				}
				continue
			}
			logger.Scanner("LLM enrichment failed: %v", enrichErr)
			break
		}
		provider.Close()
		select {
		case <-s.ctx.Done():
			return
		case <-time.After(2 * time.Second):
		}
	}
}
