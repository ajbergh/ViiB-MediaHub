//go:build darwin && !cgo

package scanner

import (
	"fmt"
	"time"
)

// FSEventsDetector is deliberately unavailable when CGO is disabled: the
// CoreServices FSEvents API is a native macOS framework. The platform selector
// then uses the existing portable mtime detector rather than claiming that it
// is a journal-backed scan.
type FSEventsDetector struct{}

func newFSEventsDetector(*Scanner) *FSEventsDetector { return &FSEventsDetector{} }

func (*FSEventsDetector) Name() string { return "macOS FSEvents (requires CGO)" }

func (*FSEventsDetector) IsAvailable() bool { return false }

func (*FSEventsDetector) GetChangesSince(time.Time, []string) ([]FileChange, error) {
	return nil, fmt.Errorf("macOS FSEvents requires CGO")
}

func (*FSEventsDetector) SaveState() error { return nil }

func (*FSEventsDetector) LoadState() error { return nil }
