//go:build windows
// +build windows

package scanner

// getPlatformDetectors returns the platform-specific change detectors for Windows.
// Windows supports USN journal for efficient change detection.
func (s *Scanner) getPlatformDetectors() []JournalChangeDetector {
	return []JournalChangeDetector{
		newWindowsUSNDetector(s),
	}
}

// newMtimeDetector creates an mtime-based fallback detector for Windows.
func (s *Scanner) newMtimeDetector() JournalChangeDetector {
	return newMtimeChangeDetector(s)
}
