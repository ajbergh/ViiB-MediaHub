// Package scanner provides media library scanning functionality.
// This file defines the interface for platform-specific filesystem journal detectors.
package scanner

import (
	"time"
)

// JournalChangeDetector is the interface for platform-specific filesystem journal implementations.
// Each platform (Windows/macOS/Linux) provides its own implementation that can detect
// file changes since a given timestamp using the most efficient method available.
type JournalChangeDetector interface {
	// GetChangesSince returns all filesystem changes since the given timestamp
	// that are relevant to audio files. The paths returned should be filtered
	// to only include files within the monitored directories.
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
