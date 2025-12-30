// Package scanner provides media library scanning functionality.
// This file implements fast incremental scanning using directory signatures
// and cheap change detection to minimize startup time.
package scanner

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/db"
	"github.com/ajbergh/viib-mediahub/internal/logger"
)

// FileChange represents a detected change to a file
type FileChange struct {
	Path       string
	ChangeType ChangeType
	OldMtime   int64
	NewMtime   int64
	OldSize    int64
	NewSize    int64
}

// ChangeType indicates the type of file system change
type ChangeType int

const (
	ChangeTypeCreated ChangeType = iota
	ChangeTypeModified
	ChangeTypeDeleted
)

// String returns a human-readable representation of the change type
func (c ChangeType) String() string {
	switch c {
	case ChangeTypeCreated:
		return "created"
	case ChangeTypeModified:
		return "modified"
	case ChangeTypeDeleted:
		return "deleted"
	default:
		return "unknown"
	}
}

// QuickStartupResult contains the results of a quick startup scan
type QuickStartupResult struct {
	ChangedFiles   []FileChange
	UnchangedDirs  int
	CheckedDirs    int
	ScanDuration   time.Duration
	UsedSignatures bool
	UsedJournal    bool
	JournalMethod  string
	FallbackFull   bool
}

// QuickStartup performs a fast incremental scan using stored signatures,
// filesystem journals, and cheap change detection. It returns quickly and
// queues any detected changes for processing.
//
// Detection priority:
// 1. Filesystem journal (Windows USN, macOS FSEvents) - fastest
// 2. Signature-based directory check - fast
// 3. mtime-based fallback - slower but universal
func (s *Scanner) QuickStartup() (*QuickStartupResult, error) {
	startTime := time.Now()
	result := &QuickStartupResult{}

	// Get last scan state
	scanState, err := s.db.GetScanState()
	if err != nil {
		logger.Scanner("No previous scan state found, falling back to signature-based scan")
		// No previous state, try signature-based check
		return s.signatureBasedStartup()
	}

	lastScanTime := time.UnixMilli(scanState.LastScanTime)
	logger.Scanner("Last scan was at %s (%d ms ago)",
		lastScanTime.Format(time.RFC3339),
		time.Now().UnixMilli()-scanState.LastScanTime)

	// Get configured scan folders
	folders, err := s.db.GetScanFolders()
	if err != nil || len(folders) == 0 {
		logger.Scanner("No scan folders configured")
		result.ScanDuration = time.Since(startTime)
		return result, nil
	}

	// Collect watch paths
	var watchPaths []string
	for _, folder := range folders {
		watchPaths = append(watchPaths, folder.Path)
	}

	// Try 1: Filesystem journal detection (fastest)
	detector := s.getChangeDetector()
	if detector != nil && detector.IsAvailable() {
		logger.Scanner("Using %s for change detection", detector.Name())

		// Load previous state
		if err := detector.LoadState(); err != nil {
			logger.Scanner("Failed to load detector state: %v", err)
		}

		changes, err := detector.GetChangesSince(lastScanTime, watchPaths)
		if err == nil {
			result.ChangedFiles = changes
			result.UsedJournal = true
			result.JournalMethod = detector.Name()
			result.ScanDuration = time.Since(startTime)

			// Save state for next startup
			if err := detector.SaveState(); err != nil {
				logger.Scanner("Failed to save detector state: %v", err)
			}

			logger.Scanner("Journal-based startup completed in %s: %d files changed using %s",
				result.ScanDuration, len(result.ChangedFiles), detector.Name())

			return result, nil
		}

		logger.Scanner("Journal detection failed: %v, trying signature-based scan", err)
	}

	// Try 2: Signature-based quick check
	sigResult, err := s.signatureBasedStartup()
	if err != nil {
		logger.Scanner("Signature check failed: %v, falling back to full scan", err)
		result.FallbackFull = true
		result.ScanDuration = time.Since(startTime)
		return result, nil
	}

	result.ChangedFiles = sigResult.ChangedFiles
	result.UnchangedDirs = sigResult.UnchangedDirs
	result.CheckedDirs = sigResult.CheckedDirs
	result.UsedSignatures = true
	result.ScanDuration = time.Since(startTime)

	logger.Scanner("Quick startup completed in %s: %d dirs checked, %d unchanged, %d files changed",
		result.ScanDuration, result.CheckedDirs, result.UnchangedDirs, len(result.ChangedFiles))

	return result, nil
}

// signatureBasedStartup uses stored directory signatures for quick change detection
func (s *Scanner) signatureBasedStartup() (*QuickStartupResult, error) {
	result := &QuickStartupResult{}

	// Get all stored directory signatures
	storedSigs, err := s.db.GetAllDirectorySignatures()
	if err != nil {
		return nil, fmt.Errorf("failed to get directory signatures: %w", err)
	}
	logger.Scanner("Found %d stored directory signatures", len(storedSigs))

	// Build a map for quick lookup
	sigMap := make(map[string]db.DirectorySignature)
	for _, sig := range storedSigs {
		sigMap[sig.Path] = sig
	}

	// Get configured scan folders
	folders, err := s.db.GetScanFolders()
	if err != nil {
		return nil, fmt.Errorf("failed to get scan folders: %w", err)
	}

	// Get file metadata cache for cheap change detection
	metadataCache, err := s.db.GetFileMetadataCacheMap()
	if err != nil {
		logger.Scanner("Failed to get metadata cache: %v", err)
		metadataCache = make(map[string]db.FileMetadataCache)
	}
	logger.Scanner("Found %d entries in file metadata cache", len(metadataCache))

	// Check each scan folder
	for _, folder := range folders {
		changes, checked, unchanged, err := s.checkDirectoryWithSignature(folder.Path, sigMap, metadataCache)
		if err != nil {
			logger.Scanner("Error checking directory %s: %v", folder.Path, err)
			continue
		}

		result.ChangedFiles = append(result.ChangedFiles, changes...)
		result.CheckedDirs += checked
		result.UnchangedDirs += unchanged
	}

	return result, nil
}

// checkDirectoryWithSignature checks a directory against its stored signature
func (s *Scanner) checkDirectoryWithSignature(
	dirPath string,
	sigMap map[string]db.DirectorySignature,
	metadataCache map[string]db.FileMetadataCache,
) ([]FileChange, int, int, error) {
	var changes []FileChange
	dirsChecked := 0
	dirsUnchanged := 0

	// Walk the directory tree
	err := filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors
		}

		if info.IsDir() {
			// Check if this directory should be skipped
			dirName := info.Name()
			if shouldSkipDirectory(dirName) {
				return filepath.SkipDir
			}

			dirsChecked++

			// Check if we have a stored signature for this directory
			sig, exists := sigMap[path]
			if exists {
				// Compute current quick stats
				currentSig, err := s.computeQuickDirectorySignature(path)
				if err != nil {
					logger.Scanner("Error computing signature for %s: %v", path, err)
					return nil
				}

				// Compare signatures
				if s.signaturesMatch(sig, currentSig) {
					dirsUnchanged++
					// Skip this entire subtree
					return filepath.SkipDir
				}
			}
			return nil
		}

		// It's a file - check if it's an audio file
		ext := strings.ToLower(filepath.Ext(path))
		if !supportedExtensions[ext] {
			return nil
		}

		// Cheap change detection: compare mtime and size
		change := s.cheapFileChangeCheck(path, info, metadataCache)
		if change != nil {
			changes = append(changes, *change)
		}

		return nil
	})

	return changes, dirsChecked, dirsUnchanged, err
}

// computeQuickDirectorySignature computes a quick signature for a directory
// using only os.ReadDir (no file opening)
func (s *Scanner) computeQuickDirectorySignature(dirPath string) (db.DirectorySignature, error) {
	sig := db.DirectorySignature{
		Path:         dirPath,
		LastVerified: time.Now().UnixMilli(),
	}

	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return sig, err
	}

	var totalSize int64
	var latestMtime int64
	var fileCount int

	// Collect file info for content hash
	type fileInfo struct {
		name  string
		size  int64
		mtime int64
	}
	var files []fileInfo

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		// Only count supported audio files
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if !supportedExtensions[ext] {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		fileCount++
		totalSize += info.Size()
		mtime := info.ModTime().UnixMilli()
		if mtime > latestMtime {
			latestMtime = mtime
		}

		files = append(files, fileInfo{
			name:  entry.Name(),
			size:  info.Size(),
			mtime: mtime,
		})
	}

	sig.FileCount = fileCount
	sig.TotalSize = totalSize
	sig.LatestMtime = latestMtime

	// Compute content hash from sorted file list
	sort.Slice(files, func(i, j int) bool {
		return files[i].name < files[j].name
	})

	hasher := sha256.New()
	for _, f := range files {
		hasher.Write([]byte(fmt.Sprintf("%s|%d|%d|", f.name, f.size, f.mtime)))
	}
	sig.ContentHash = hex.EncodeToString(hasher.Sum(nil))[:16] // Use first 16 chars

	return sig, nil
}

// signaturesMatch compares two directory signatures
func (s *Scanner) signaturesMatch(stored, current db.DirectorySignature) bool {
	// Quick checks first (cheapest to most expensive)
	if stored.FileCount != current.FileCount {
		return false
	}
	if stored.TotalSize != current.TotalSize {
		return false
	}
	if stored.LatestMtime != current.LatestMtime {
		return false
	}
	// Content hash is the definitive check
	return stored.ContentHash == current.ContentHash
}

// cheapFileChangeCheck performs cheap mtime+size check for a file
func (s *Scanner) cheapFileChangeCheck(
	filePath string,
	info os.FileInfo,
	cache map[string]db.FileMetadataCache,
) *FileChange {
	cached, exists := cache[filePath]
	if !exists {
		// New file
		return &FileChange{
			Path:       filePath,
			ChangeType: ChangeTypeCreated,
			NewMtime:   info.ModTime().UnixMilli(),
			NewSize:    info.Size(),
		}
	}

	currentMtime := info.ModTime().UnixMilli()
	currentSize := info.Size()

	// Check if file changed
	if cached.Mtime != currentMtime || cached.FileSize != currentSize {
		return &FileChange{
			Path:       filePath,
			ChangeType: ChangeTypeModified,
			OldMtime:   cached.Mtime,
			NewMtime:   currentMtime,
			OldSize:    cached.FileSize,
			NewSize:    currentSize,
		}
	}

	// File unchanged
	return nil
}

// ComputeAndSaveDirectorySignatures computes and saves signatures for all directories
// in the scan folders. Should be called after a full scan.
func (s *Scanner) ComputeAndSaveDirectorySignatures() error {
	folders, err := s.db.GetScanFolders()
	if err != nil {
		return fmt.Errorf("failed to get scan folders: %w", err)
	}

	var signatures []db.DirectorySignature

	for _, folder := range folders {
		// Walk the directory and compute signatures
		err := filepath.Walk(folder.Path, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}

			if !info.IsDir() {
				return nil
			}

			// Check if this directory should be skipped
			dirName := info.Name()
			if shouldSkipDirectory(dirName) {
				return filepath.SkipDir
			}

			sig, err := s.computeQuickDirectorySignature(path)
			if err != nil {
				logger.Scanner("Error computing signature for %s: %v", path, err)
				return nil
			}

			// Only save if directory has audio files
			if sig.FileCount > 0 {
				signatures = append(signatures, sig)
			}

			return nil
		})

		if err != nil {
			logger.Scanner("Error walking %s: %v", folder.Path, err)
		}
	}

	// Save all signatures in batch
	if len(signatures) > 0 {
		logger.Scanner("Saving %d directory signatures", len(signatures))
		if err := s.db.SaveDirectorySignaturesBatch(signatures); err != nil {
			return fmt.Errorf("failed to save directory signatures: %w", err)
		}
	}

	return nil
}

// UpdateFileMetadataCache updates the metadata cache for processed files.
// Should be called after processing files to update their cached state.
func (s *Scanner) UpdateFileMetadataCache(files []string) error {
	var caches []db.FileMetadataCache
	now := time.Now().UnixMilli()

	for _, filePath := range files {
		info, err := os.Stat(filePath)
		if err != nil {
			continue
		}

		caches = append(caches, db.FileMetadataCache{
			FilePath:     filePath,
			FileSize:     info.Size(),
			Mtime:        info.ModTime().UnixMilli(),
			LastVerified: now,
		})
	}

	if len(caches) > 0 {
		return s.db.SaveFileMetadataCacheBatch(caches)
	}
	return nil
}

// SaveScanState saves the current scan state after a scan completes
func (s *Scanner) SaveScanState(result *ScanResult) error {
	state := db.ScanState{
		LastScanTime:   time.Now().UnixMilli(),
		ScanDurationMs: result.Duration.Milliseconds(),
		FilesScanned:   result.TotalFiles,
		FilesChanged:   result.NewSongs + result.UpdatedSongs,
	}

	return s.db.SaveScanState(state)
}

// DetectDeletedFiles finds files that were in the cache but no longer exist,
// or are in directories that should be skipped (like @eaDir, $RECYCLE.BIN, etc.)
func (s *Scanner) DetectDeletedFiles() ([]FileChange, error) {
	cache, err := s.db.GetAllFileMetadataCache()
	if err != nil {
		return nil, err
	}

	var changes []FileChange
	for _, cached := range cache {
		// Check if file is in a directory that should be skipped
		// This cleans up files that were added before skip logic was implemented
		if pathContainsSkippedDirectory(cached.FilePath) {
			logger.Scanner("Marking file for removal (in skipped directory): %s", cached.FilePath)
			changes = append(changes, FileChange{
				Path:       cached.FilePath,
				ChangeType: ChangeTypeDeleted,
				OldMtime:   cached.Mtime,
				OldSize:    cached.FileSize,
			})
			continue
		}

		// Check if file no longer exists
		if _, err := os.Stat(cached.FilePath); os.IsNotExist(err) {
			changes = append(changes, FileChange{
				Path:       cached.FilePath,
				ChangeType: ChangeTypeDeleted,
				OldMtime:   cached.Mtime,
				OldSize:    cached.FileSize,
			})
		}
	}

	return changes, nil
}

// ProcessChanges processes detected file changes (add/update/delete songs)
func (s *Scanner) ProcessChanges(changes []FileChange) (*ScanResult, error) {
	result := &ScanResult{}
	startTime := time.Now()

	var filesToAdd []string
	var filesToUpdate []string
	var filesToDelete []string

	for _, change := range changes {
		switch change.ChangeType {
		case ChangeTypeCreated:
			filesToAdd = append(filesToAdd, change.Path)
		case ChangeTypeModified:
			filesToUpdate = append(filesToUpdate, change.Path)
		case ChangeTypeDeleted:
			filesToDelete = append(filesToDelete, change.Path)
		}
	}

	// Process additions and updates
	allFilesToProcess := append(filesToAdd, filesToUpdate...)
	if len(allFilesToProcess) > 0 {
		// Only log when processing multiple files to reduce log noise
		if len(allFilesToProcess) > 1 {
			logger.Scanner("Processing %d files (%d new, %d updated)",
				len(allFilesToProcess), len(filesToAdd), len(filesToUpdate))
		}

		// Process each file individually
		var songs []db.Song
		for _, filePath := range allFilesToProcess {
			metadata, err := s.extractMetadata(filePath)
			if err != nil {
				logger.Scanner("Error extracting metadata from %s: %v", filePath, err)
				result.Errors++
				continue
			}

			// Get or create album cover
			coverPath := s.getAlbumCover(metadata.Artist, metadata.Album, filepath.Dir(filePath), filePath)

			dbSong := db.Song{
				ID:          metadata.ID,
				Title:       metadata.Title,
				Artist:      metadata.Artist,
				Album:       metadata.Album,
				AlbumArtist: metadata.AlbumArtist,
				TrackNumber: metadata.TrackNumber,
				DiscNumber:  metadata.DiscNumber,
				Genre:       metadata.Genre,
				Year:        metadata.Year,
				Duration:    metadata.Duration,
				FilePath:    filePath,
				CoverPath:   coverPath,
				AddedAt:     time.Now().UnixMilli(),
				FileHash:    metadata.ID,
			}

			songs = append(songs, dbSong)
			result.TotalFiles++
		}

		// Save all songs
		if len(songs) > 0 {
			if err := s.db.SaveSongs(songs); err != nil {
				logger.Scanner("Error saving songs: %v", err)
				result.Errors++
			} else {
				result.NewSongs = len(filesToAdd)
				result.UpdatedSongs = len(filesToUpdate)

				// Create album metadata entries
				s.createAlbumMetadataEntries(songs)

				// Emit library_updated event so UI refreshes
				s.emitEvent(LibraryEvent{
					Type:         "library_updated",
					Message:      fmt.Sprintf("Added %d songs", len(songs)),
					NewSongs:     len(filesToAdd),
					UpdatedSongs: len(filesToUpdate),
				})

				// Update genre stats
				go s.db.UpdateGenreStats()
			}
		}

		// Update metadata cache for processed files
		if err := s.UpdateFileMetadataCache(allFilesToProcess); err != nil {
			logger.Scanner("Error updating metadata cache: %v", err)
		}
	}

	// Process deletions
	if len(filesToDelete) > 0 {
		logger.Scanner("Removing %d deleted files from library", len(filesToDelete))
		removed, err := s.db.DeleteSongsByFilePaths(filesToDelete)
		if err != nil {
			logger.Scanner("Error removing songs: %v", err)
		} else {
			result.RemovedSongs = removed

			// Emit library_updated event so UI refreshes
			if removed > 0 {
				s.emitEvent(LibraryEvent{
					Type:         "library_updated",
					Message:      fmt.Sprintf("Removed %d songs", removed),
					RemovedSongs: removed,
				})

				// Update genre stats
				go s.db.UpdateGenreStats()
			}
		}

		// Clean up metadata cache
		if err := s.db.DeleteFileMetadataCacheBatch(filesToDelete); err != nil {
			logger.Scanner("Error cleaning metadata cache: %v", err)
		}
	}

	result.Duration = time.Since(startTime)
	return result, nil
}

// ==============================================================================
// Background Scanner Control Methods
// ==============================================================================

// PauseBackgroundWork pauses the background scanner
func (s *Scanner) PauseBackgroundWork() {
	if s.backgroundScanner != nil {
		s.backgroundScanner.Pause()
	}
}

// ResumeBackgroundWork resumes the background scanner
func (s *Scanner) ResumeBackgroundWork() {
	if s.backgroundScanner != nil {
		s.backgroundScanner.Resume()
	}
}

// IsBackgroundPaused returns whether background work is paused
func (s *Scanner) IsBackgroundPaused() bool {
	if s.backgroundScanner != nil {
		return s.backgroundScanner.IsPaused()
	}
	return true
}

// GetBackgroundStats returns statistics about background processing
func (s *Scanner) GetBackgroundStats() (processed, errors int64, queueSize int, activeWorkers int32) {
	if s.backgroundScanner != nil {
		return s.backgroundScanner.Stats()
	}
	return 0, 0, 0, 0
}

// QueueChangesForBackground queues detected changes for background processing
func (s *Scanner) QueueChangesForBackground(changes []FileChange) {
	if s.backgroundScanner == nil || len(changes) == 0 {
		return
	}

	// Determine priority based on change count
	priority := PriorityHigh
	if len(changes) > 100 {
		priority = PriorityNormal
	}

	s.backgroundScanner.QueueFileTasks(changes, priority)
}

// StartBackgroundValidation starts a background validation of the library
func (s *Scanner) StartBackgroundValidation() {
	if s.backgroundScanner == nil {
		return
	}

	// Get a random sample of files to validate
	sample, err := s.db.GetRandomFileSample(100)
	if err != nil {
		logger.Scanner("Cannot get file sample for validation: %v", err)
		return
	}

	logger.Scanner("Starting background validation of %d files", len(sample))

	for _, file := range sample {
		s.backgroundScanner.QueueTask(&BackgroundTask{
			Type:     TaskValidateFile,
			Priority: PriorityLow,
			FilePath: file.FilePath,
		})
	}
}

// StopBackgroundWork stops the background scanner
func (s *Scanner) StopBackgroundWork() {
	if s.backgroundScanner != nil {
		s.backgroundScanner.Stop()
	}
}
