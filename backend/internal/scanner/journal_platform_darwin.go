//go:build darwin
// +build darwin

package scanner

// getPlatformDetectors returns the platform-specific change detectors for macOS.
// macOS supports FSEvents for efficient change detection.
func (s *Scanner) getPlatformDetectors() []JournalChangeDetector {
	return []JournalChangeDetector{
		newFSEventsDetector(s),
	}
}

// newMtimeDetector creates an mtime-based fallback detector for macOS.
func (s *Scanner) newMtimeDetector() JournalChangeDetector {
	return newMtimeChangeDetector(s)
}
