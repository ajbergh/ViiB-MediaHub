//go:build linux
// +build linux

package scanner

// getPlatformDetectors returns the platform-specific change detectors for Linux.
// Linux uses optimized mtime-based detection since inotify doesn't persist.
func (s *Scanner) getPlatformDetectors() []JournalChangeDetector {
	return []JournalChangeDetector{
		newLinuxMtimeDetector(s),
	}
}

// newMtimeDetector creates an mtime-based fallback detector for Linux.
func (s *Scanner) newMtimeDetector() JournalChangeDetector {
	return newMtimeChangeDetector(s)
}
