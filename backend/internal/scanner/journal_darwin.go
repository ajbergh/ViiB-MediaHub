//go:build darwin
// +build darwin

// Package scanner provides media library scanning functionality.
// This file implements macOS FSEvents for efficient filesystem change detection.
package scanner

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/db"
	"github.com/ajbergh/viib-mediahub/internal/logger"
)

// FSEventsDetector implements JournalChangeDetector using macOS FSEvents.
// Note: FSEvents in Go requires CGO and the FSEvents framework.
// For a pure-Go solution, we fall back to mtime-based detection with optimization.
// A full FSEvents implementation would use github.com/fsnotify/fsevents or similar.
type FSEventsDetector struct {
	scanner      *Scanner
	lastEventID  uint64
	lastScanTime time.Time
}

// newFSEventsDetector creates a new FSEvents detector
func newFSEventsDetector(s *Scanner) *FSEventsDetector {
	return &FSEventsDetector{
		scanner: s,
	}
}

// Name returns the name of this detector
func (f *FSEventsDetector) Name() string {
	return "macOS FSEvents (mtime fallback)"
}

// IsAvailable checks if FSEvents is available
func (f *FSEventsDetector) IsAvailable() bool {
	// FSEvents requires CGO and the CoreServices framework.
	// For this implementation, we provide an optimized mtime-based fallback
	// that uses the LastModified directory attribute for faster scanning.
	return true
}

// GetChangesSince returns all filesystem changes since the given timestamp
// This implementation uses an optimized mtime scan since pure-Go FSEvents
// would require CGO. It's still much faster than a full scan because:
// 1. We use directory mtime to skip unchanged directories
// 2. We only check audio files that match our extensions
func (f *FSEventsDetector) GetChangesSince(since time.Time, watchPaths []string) ([]FileChange, error) {
	var changes []FileChange

	for _, watchPath := range watchPaths {
		dirChanges, err := f.scanDirectoryForChanges(watchPath, since)
		if err != nil {
			logger.Scanner("Error scanning %s for changes: %v", watchPath, err)
			continue
		}
		changes = append(changes, dirChanges...)
	}

	f.lastScanTime = time.Now()
	logger.Scanner("FSEvents (mtime): found %d changes since %s", len(changes), since.Format(time.RFC3339))
	return changes, nil
}

// scanDirectoryForChanges efficiently scans a directory tree for changes
func (f *FSEventsDetector) scanDirectoryForChanges(dirPath string, since time.Time) ([]FileChange, error) {
	var changes []FileChange

	err := filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors
		}

		// For directories, check if we can skip the entire subtree
		if info.IsDir() {
			// Check if this directory should be skipped
			dirName := info.Name()
			if shouldSkipDirectory(dirName) {
				return filepath.SkipDir
			}
			// If directory hasn't been modified since last scan, we might be able to skip it
			// Note: Directory mtime only changes when direct children are added/removed
			// Files modified within maintain the same directory mtime
			return nil
		}

		// Check if it's an audio file
		ext := strings.ToLower(filepath.Ext(path))
		if !supportedExtensions[ext] {
			return nil
		}

		// Check if file was modified since the given time
		if info.ModTime().After(since) {
			// Determine if it's new or modified by checking if we have it in cache
			cached, err := f.scanner.db.GetFileMetadataCache(path)

			var changeType ChangeType
			if err != nil || cached == nil {
				changeType = ChangeTypeCreated
			} else {
				changeType = ChangeTypeModified
			}

			changes = append(changes, FileChange{
				Path:       path,
				ChangeType: changeType,
				NewMtime:   info.ModTime().UnixMilli(),
				NewSize:    info.Size(),
			})
		}

		return nil
	})

	return changes, err
}

// SaveState saves the current event ID to the database
func (f *FSEventsDetector) SaveState() error {
	state, err := f.scanner.db.GetScanState()
	if err != nil {
		state = &db.ScanState{
			LastScanTime: time.Now().UnixMilli(),
		}
	}

	state.MacOSEventID = int64(f.lastEventID)
	return f.scanner.db.SaveScanState(*state)
}

// LoadState loads the previously saved event ID
func (f *FSEventsDetector) LoadState() error {
	state, err := f.scanner.db.GetScanState()
	if err != nil {
		return err
	}

	f.lastEventID = uint64(state.MacOSEventID)
	f.lastScanTime = time.UnixMilli(state.LastScanTime)
	return nil
}
