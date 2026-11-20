//go:build darwin
// +build darwin

// Defines journal platform darwin functionality for package scanner.

package scanner

// Use the metadata cache as the checkpoint on macOS. A host-wide FSEvents ID
// cannot establish coverage for newly added folders, moved trees, or mounted
// volumes. Comparing with the committed cache also retries unprocessed changes.
func (s *Scanner) getPlatformDetectors() []JournalChangeDetector {
	return nil
}

// newMtimeDetector creates an mtime-based fallback detector for macOS.
func (s *Scanner) newMtimeDetector() JournalChangeDetector {
	return newMtimeChangeDetector(s)
}
