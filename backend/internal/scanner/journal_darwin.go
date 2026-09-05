//go:build darwin && cgo

// Package scanner provides media library scanning functionality.
package scanner

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/db"
	"github.com/ajbergh/viib-mediahub/internal/logger"
	"github.com/fsnotify/fsevents"
)

const fseventsHistoryTimeout = 10 * time.Second

var errFSEventsHistoryIncomplete = errors.New("macOS FSEvents history is incomplete; a directory scan is required")

// FSEventsDetector reads macOS's persistent FSEvents journal. It uses file
// events where available, then resolves the reported paths against the media
// metadata cache so callers still receive file-level changes.
type FSEventsDetector struct {
	scanner      *Scanner
	lastEventID  uint64
	lastScanTime time.Time
}

func newFSEventsDetector(s *Scanner) *FSEventsDetector {
	return &FSEventsDetector{scanner: s}
}

func (f *FSEventsDetector) Name() string {
	return "macOS FSEvents"
}

func (f *FSEventsDetector) IsAvailable() bool {
	// FSEvents is part of CoreServices on every supported macOS release. A
	// stream creation failure is reported by GetChangesSince and triggers the
	// signature-based fallback.
	return true
}

// GetChangesSince replays persistent FSEvents records after the saved cursor.
// The first run after upgrading from the mtime implementation derives a host
// event ID from the previous scan timestamp; Apple documents this conversion as
// conservative, so it may return harmless extra events but must not miss one.
func (f *FSEventsDetector) GetChangesSince(since time.Time, watchPaths []string) ([]FileChange, error) {
	paths, err := normalizeFSEventsWatchPaths(watchPaths)
	if err != nil {
		return nil, err
	}
	if len(paths) == 0 {
		return nil, nil
	}

	startID := f.lastEventID
	if startID == 0 {
		startID = fsevents.EventIDForDeviceBeforeTime(0, since)
	}

	stream := &fsevents.EventStream{
		Paths:   paths,
		Flags:   fsevents.FileEvents | fsevents.WatchRoot,
		Resume:  true,
		EventID: startID,
		Latency: 0,
		Events:  make(chan []fsevents.Event, 16),
	}
	if err := stream.Start(); err != nil {
		return nil, fmt.Errorf("start FSEvents stream: %w", err)
	}
	defer stream.Stop()

	changes := make([]FileChange, 0)
	timer := time.NewTimer(fseventsHistoryTimeout)
	defer timer.Stop()

	for {
		select {
		case events := <-stream.Events:
			historyDone := false
			for _, event := range events {
				if event.ID > f.lastEventID {
					f.lastEventID = event.ID
				}
				if fseventsEventRequiresFallback(event.Flags) {
					return nil, fmt.Errorf("%w (flags %#x for %s)", errFSEventsHistoryIncomplete, event.Flags, event.Path)
				}
				if event.Flags&fsevents.HistoryDone != 0 {
					historyDone = true
					continue
				}
				if event.Flags&fsevents.ItemIsDir != 0 || !pathIsWithinWatchPaths(event.Path, paths) {
					continue
				}
				change, err := f.changeForEvent(event.Path)
				if err != nil {
					return nil, err
				}
				if change != nil {
					changes = append(changes, *change)
				}
			}
			if historyDone {
				f.lastScanTime = time.Now()
				logger.Scanner("FSEvents: found %d file changes since event %d", len(changes), startID)
				return changes, nil
			}
		case <-timer.C:
			return nil, fmt.Errorf("timed out waiting for FSEvents history after event %d", startID)
		}
	}
}

func normalizeFSEventsWatchPaths(watchPaths []string) ([]string, error) {
	paths := make([]string, 0, len(watchPaths))
	seen := make(map[string]struct{}, len(watchPaths))
	for _, watchPath := range watchPaths {
		absPath, err := filepath.Abs(watchPath)
		if err != nil {
			return nil, fmt.Errorf("resolve FSEvents watch path %q: %w", watchPath, err)
		}
		resolvedPath, err := filepath.EvalSymlinks(absPath)
		if err != nil {
			return nil, fmt.Errorf("resolve FSEvents symlink %q: %w", absPath, err)
		}
		resolvedPath = filepath.Clean(resolvedPath)
		if _, exists := seen[resolvedPath]; exists {
			continue
		}
		seen[resolvedPath] = struct{}{}
		paths = append(paths, resolvedPath)
	}
	return paths, nil
}

func pathIsWithinWatchPaths(path string, watchPaths []string) bool {
	cleanPath := filepath.Clean(path)
	for _, watchPath := range watchPaths {
		rel, err := filepath.Rel(watchPath, cleanPath)
		if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
			return true
		}
	}
	return false
}

func (f *FSEventsDetector) changeForEvent(path string) (*FileChange, error) {
	path = filepath.Clean(path)
	if !supportedExtensions[strings.ToLower(filepath.Ext(path))] {
		return nil, nil
	}

	cached, cacheErr := f.scanner.db.GetFileMetadataCache(path)
	if cacheErr != nil && !errors.Is(cacheErr, sql.ErrNoRows) {
		return nil, fmt.Errorf("read metadata cache for %s: %w", path, cacheErr)
	}

	info, statErr := os.Stat(path)
	if statErr == nil {
		changeType := ChangeTypeModified
		if cached == nil {
			changeType = ChangeTypeCreated
		}
		change := &FileChange{
			Path:       path,
			ChangeType: changeType,
			NewMtime:   info.ModTime().UnixMilli(),
			NewSize:    info.Size(),
		}
		if cached != nil {
			change.OldMtime = cached.Mtime
			change.OldSize = cached.FileSize
		}
		return change, nil
	}
	if !errors.Is(statErr, os.ErrNotExist) {
		return nil, fmt.Errorf("stat FSEvents path %s: %w", path, statErr)
	}
	if cached == nil {
		return nil, nil
	}
	return &FileChange{
		Path:       path,
		ChangeType: ChangeTypeDeleted,
		OldMtime:   cached.Mtime,
		OldSize:    cached.FileSize,
	}, nil
}

func (f *FSEventsDetector) SaveState() error {
	state, err := f.scanner.db.GetScanState()
	if err != nil {
		state = &db.ScanState{LastScanTime: time.Now().UnixMilli()}
	}
	state.MacOSEventID = int64(f.lastEventID)
	return f.scanner.db.SaveScanState(*state)
}

func (f *FSEventsDetector) LoadState() error {
	state, err := f.scanner.db.GetScanState()
	if err != nil {
		return err
	}
	f.lastEventID = uint64(state.MacOSEventID)
	f.lastScanTime = time.UnixMilli(state.LastScanTime)
	return nil
}
