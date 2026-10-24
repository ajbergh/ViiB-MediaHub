// Package scanner provides media library scanning functionality.
// This file defines the interface for platform-specific filesystem journal detectors.
package scanner

import (
	"time"
)

// JournalChangeDetector is the interface for platform-specific filesystem journal implementations.
// Implementations use platform journal cursors or compare filesystem metadata
// against the stored cache; the supplied timestamp is not a universal replay boundary.
type JournalChangeDetector interface {
	// GetChangesSince reports audio-file changes within watchPaths relative to the
	// detector's cursor/cache, using since when supported by that implementation.
	// This is change detection, not an exhaustive timestamp-based event history.
	GetChangesSince(since time.Time, watchPaths []string) ([]FileChange, error)

	// IsAvailable returns true if this journal method is available on the current system.
	// For example, USN requires NTFS and admin privileges on Windows.
	IsAvailable() bool

	// Name returns the human-readable name of this detection method
	Name() string

	// SaveState saves the current journal cursor/position to the database
	// so it can be resumed on next startup.
	SaveState() error

	// LoadState loads the previously saved journal cursor from the database.
	LoadState() error
}

// getChangeDetector returns the appropriate journal change detector for the current platform.
// It tries the most efficient method first and falls back to less efficient ones.
func (s *Scanner) getChangeDetector() JournalChangeDetector {
	// Try platform-specific detectors in order of efficiency
	detectors := s.getPlatformDetectors()

	for _, detector := range detectors {
		if detector.IsAvailable() {
			return detector
		}
	}

	// Fallback to mtime-based detection (always available)
	return s.newMtimeDetector()
}
