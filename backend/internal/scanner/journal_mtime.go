// Package scanner provides media library scanning functionality.
// This file implements a universal mtime-based change detector that works on all platforms.
package scanner

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/db"
	"github.com/ajbergh/viib-mediahub/internal/logger"
)

// MtimeChangeDetector implements JournalChangeDetector using mtime-based detection.
// This is the universal fallback that works on all platforms.
type MtimeChangeDetector struct {
	scanner      *Scanner
	lastScanTime time.Time
}

// newMtimeChangeDetector creates a new mtime-based change detector
func newMtimeChangeDetector(s *Scanner) *MtimeChangeDetector {
	return &MtimeChangeDetector{
		scanner: s,
	}
}

// Name returns the name of this detector
func (m *MtimeChangeDetector) Name() string {
	return "mtime fallback"
}

// IsAvailable always returns true for mtime detection
func (m *MtimeChangeDetector) IsAvailable() bool {
	return true
}

// GetChangesSince returns all filesystem changes since the given timestamp
func (m *MtimeChangeDetector) GetChangesSince(since time.Time, watchPaths []string) ([]FileChange, error) {
	var changes []FileChange
	_ = since // Used for logging, actual comparison uses metadata cache

	// Load metadata cache for comparison
	metadataCache, err := m.scanner.db.GetFileMetadataCacheMap()
	if err != nil {
		logger.Scanner("Cannot load metadata cache: %v", err)
		metadataCache = make(map[string]db.FileMetadataCache)
	}

	// Track seen files to detect deletions
	seenFiles := make(map[string]bool)

	for _, watchPath := range watchPaths {
		err := filepath.Walk(watchPath, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil // Skip errors
			}

			if info.IsDir() {
				// Check if this directory should be skipped
				dirName := info.Name()
				if shouldSkipDirectory(dirName) {
					return filepath.SkipDir
				}
				return nil
			}

			// Check if it's an audio file
			ext := strings.ToLower(filepath.Ext(path))
			if !supportedExtensions[ext] {
				return nil
			}

			seenFiles[path] = true

			// Get cached metadata
			cached, hasCached := metadataCache[path]
			mtime := info.ModTime().UnixMilli()
			size := info.Size()

			if !hasCached {
				// New file
				changes = append(changes, FileChange{
					Path:       path,
					ChangeType: ChangeTypeCreated,
					NewMtime:   mtime,
					NewSize:    size,
				})
			} else if cached.Mtime != mtime || cached.FileSize != size {
				// Modified file
				changes = append(changes, FileChange{
					Path:       path,
					ChangeType: ChangeTypeModified,
					OldMtime:   cached.Mtime,
					NewMtime:   mtime,
					OldSize:    cached.FileSize,
					NewSize:    size,
				})
			}
			// If mtime and size match, file is unchanged

			return nil
		})

		if err != nil {
			logger.Scanner("Error walking %s: %v", watchPath, err)
		}
	}

	// Detect deleted files - files in cache but not seen during walk
	for path := range metadataCache {
		if !seenFiles[path] {
			// Check if the path is within our watch paths
			for _, watchPath := range watchPaths {
				if strings.HasPrefix(strings.ToLower(path), strings.ToLower(watchPath)) {
					cached := metadataCache[path]
					changes = append(changes, FileChange{
						Path:       path,
						ChangeType: ChangeTypeDeleted,
						OldMtime:   cached.Mtime,
						OldSize:    cached.FileSize,
					})
					break
				}
			}
		}
	}

	m.lastScanTime = time.Now()
	logger.Scanner("mtime fallback: found %d changes since %s", len(changes), since.Format(time.RFC3339))
	return changes, nil
}

// SaveState saves the current scan timestamp to the database
func (m *MtimeChangeDetector) SaveState() error {
	state, err := m.scanner.db.GetScanState()
	if err != nil {
		state = &db.ScanState{
			LastScanTime: time.Now().UnixMilli(),
		}
	}

	state.LastScanTime = m.lastScanTime.UnixMilli()
	return m.scanner.db.SaveScanState(*state)
}

// LoadState loads the previously saved scan timestamp
func (m *MtimeChangeDetector) LoadState() error {
	state, err := m.scanner.db.GetScanState()
	if err != nil {
		return err
	}

	m.lastScanTime = time.UnixMilli(state.LastScanTime)
	return nil
}
