//go:build linux
// +build linux

// Package scanner provides media library scanning functionality.
// This file implements Linux mtime-based change detection with optimizations.
package scanner

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/db"
	"github.com/ajbergh/viib-mediahub/internal/logger"
)

// LinuxMtimeDetector implements JournalChangeDetector using mtime-based detection.
// Linux doesn't have persistent filesystem events like Windows USN or macOS FSEvents,
// so we use an optimized mtime scan that leverages:
// 1. Directory ctime/mtime to skip unchanged directory subtrees
// 2. Inode change time (ctime) for more accurate change detection
type LinuxMtimeDetector struct {
	scanner      *Scanner
	lastScanTime time.Time
}

// newLinuxMtimeDetector creates a new Linux mtime detector
func newLinuxMtimeDetector(s *Scanner) *LinuxMtimeDetector {
	return &LinuxMtimeDetector{
		scanner: s,
	}
}

// Name returns the name of this detector
func (l *LinuxMtimeDetector) Name() string {
	return "Linux mtime (optimized)"
}

// IsAvailable always returns true for Linux mtime detection
func (l *LinuxMtimeDetector) IsAvailable() bool {
	return true
}

// GetChangesSince returns all filesystem changes since the given timestamp
// Uses optimized mtime scanning with directory-level skipping.
func (l *LinuxMtimeDetector) GetChangesSince(since time.Time, watchPaths []string) ([]FileChange, error) {
	var changes []FileChange
	sinceUnix := since.Unix()

	// Load stored directory signatures for quick skipping
	signatures, err := l.scanner.db.GetAllDirectorySignatures()
	if err != nil {
		logger.Scanner("Cannot load directory signatures: %v", err)
	}

	sigMap := make(map[string]db.DirectorySignature)
	for _, sig := range signatures {
		sigMap[sig.Path] = sig
	}

	for _, watchPath := range watchPaths {
		dirChanges, err := l.scanDirectoryForChanges(watchPath, sinceUnix, sigMap)
		if err != nil {
			logger.Scanner("Error scanning %s for changes: %v", watchPath, err)
			continue
		}
		changes = append(changes, dirChanges...)
	}

	l.lastScanTime = time.Now()
	logger.Scanner("Linux mtime: found %d changes since %s", len(changes), since.Format(time.RFC3339))
	return changes, nil
}

// scanDirectoryForChanges efficiently scans a directory tree for changes
func (l *LinuxMtimeDetector) scanDirectoryForChanges(
	dirPath string,
	sinceUnix int64,
	sigMap map[string]db.DirectorySignature,
) ([]FileChange, error) {
	var changes []FileChange

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

			// Check if we can skip this directory using stored signature
			if sig, ok := sigMap[path]; ok {
				// Get current directory stats
				stat, ok := info.Sys().(*syscall.Stat_t)
				if ok {
					// Use ctime (inode change time) for better accuracy
					dirCtime := stat.Ctim.Sec

					// If directory hasn't changed since signature was computed, skip it
					if dirCtime <= sig.LastVerified/1000 && sig.LatestMtime/1000 <= sinceUnix {
						logger.Scanner("Skipping unchanged directory: %s", path)
						return filepath.SkipDir
					}
				}
			}
			return nil
		}

		// Check if it's an audio file
		ext := strings.ToLower(filepath.Ext(path))
		if !supportedExtensions[ext] {
			return nil
		}

		// Get detailed file stats
		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok {
			// Fallback to basic mtime check
			if info.ModTime().Unix() > sinceUnix {
				changes = append(changes, l.createFileChange(path, info))
			}
			return nil
		}

		// Use the most recent of mtime and ctime for change detection
		// ctime catches metadata changes, mtime catches content changes
		mtime := stat.Mtim.Sec
		ctime := stat.Ctim.Sec
		latestChange := mtime
		if ctime > mtime {
			latestChange = ctime
		}

		if latestChange > sinceUnix {
			changes = append(changes, l.createFileChange(path, info))
		}

		return nil
	})

	return changes, err
}

// createFileChange creates a FileChange from file info
func (l *LinuxMtimeDetector) createFileChange(path string, info os.FileInfo) FileChange {
	// Check if file is new or modified
	cached, err := l.scanner.db.GetFileMetadataCache(path)

	var changeType ChangeType
	var oldMtime, oldSize int64

	if err != nil || cached == nil {
		changeType = ChangeTypeCreated
	} else {
		changeType = ChangeTypeModified
		oldMtime = cached.Mtime
		oldSize = cached.FileSize
	}

	return FileChange{
		Path:       path,
		ChangeType: changeType,
		OldMtime:   oldMtime,
		NewMtime:   info.ModTime().UnixMilli(),
		OldSize:    oldSize,
		NewSize:    info.Size(),
	}
}

// SaveState saves the current scan timestamp to the database
func (l *LinuxMtimeDetector) SaveState() error {
	state, err := l.scanner.db.GetScanState()
	if err != nil {
		state = &db.ScanState{
			LastScanTime: time.Now().UnixMilli(),
		}
	}

	state.LinuxLastMtime = l.lastScanTime.UnixMilli()
	return l.scanner.db.SaveScanState(*state)
}

// LoadState loads the previously saved scan timestamp
func (l *LinuxMtimeDetector) LoadState() error {
	state, err := l.scanner.db.GetScanState()
	if err != nil {
		return err
	}

	l.lastScanTime = time.UnixMilli(state.LinuxLastMtime)
	return nil
}
