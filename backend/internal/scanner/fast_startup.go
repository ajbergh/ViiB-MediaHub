// Package scanner provides media library scanning functionality.
// This file implements fast incremental startup detection.
package scanner

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/db"
	"github.com/ajbergh/viib-mediahub/internal/logger"
)

// FileChange represents a detected change to a file.
type FileChange struct {
	Path       string
	ChangeType ChangeType
	OldMtime   int64
	NewMtime   int64
	OldSize    int64
	NewSize    int64
}

// ChangeType indicates the type of file system change.
type ChangeType int

const (
	ChangeTypeCreated ChangeType = iota
	ChangeTypeModified
	ChangeTypeDeleted
)

func (c ChangeType) String() string {
	switch c {
	case ChangeTypeCreated:
		return "created"
	case ChangeTypeModified:
		return "modified"
	case ChangeTypeDeleted:
		return "deleted"
	default:
		return "unknown"
	}
}

// QuickStartupResult contains the results of a quick startup scan.
type QuickStartupResult struct {
	ChangedFiles   []FileChange
	UnchangedDirs  int
	CheckedDirs    int
	ScanDuration   time.Duration
	UsedSignatures bool
	UsedJournal    bool
	JournalMethod  string
	FallbackFull   bool
}

// QuickStartup performs incremental discovery using the fastest available
// detector, falling back to directory signatures when a journal is unavailable.
func (s *Scanner) QuickStartup() (*QuickStartupResult, error) {
	startTime := time.Now()
	result := &QuickStartupResult{}

	scanState, err := s.db.GetScanState()
	if err != nil {
		logger.Scanner("No previous scan state found, falling back to signature-based scan")
		return s.signatureBasedStartup()
	}

	lastScanTime := time.UnixMilli(scanState.LastScanTime)
	logger.Scanner("Last scan was at %s (%d ms ago)",
		lastScanTime.Format(time.RFC3339),
		time.Now().UnixMilli()-scanState.LastScanTime)

	folders, err := s.db.GetScanFolders()
	if err != nil || len(folders) == 0 {
		logger.Scanner("No scan folders configured")
		result.ScanDuration = time.Since(startTime)
		return result, nil
	}

	watchPaths := make([]string, 0, len(folders))
	for _, folder := range folders {
		watchPaths = append(watchPaths, folder.Path)
	}

	detector := s.getChangeDetector()
	if detector != nil && detector.IsAvailable() {
		logger.Scanner("Using %s for change detection", detector.Name())
		if err := detector.LoadState(); err != nil {
			logger.Scanner("Failed to load detector state: %v", err)
		}

		changes, detectErr := detector.GetChangesSince(lastScanTime, watchPaths)
		if detectErr == nil {
			result.ChangedFiles = coalesceFileChanges(changes)
			result.UsedJournal = true
			result.JournalMethod = detector.Name()
			result.ScanDuration = time.Since(startTime)
			if err := detector.SaveState(); err != nil {
				logger.Scanner("Failed to save detector state: %v", err)
			}
			logger.Scanner("Journal-based startup completed in %s: %d files changed using %s",
				result.ScanDuration, len(result.ChangedFiles), detector.Name())
			return result, nil
		}

		logger.Scanner("Journal detection failed: %v, trying signature-based scan", detectErr)
	}

	sigResult, err := s.signatureBasedStartup()
	if err != nil {
		logger.Scanner("Signature check failed: %v, falling back to full scan", err)
		result.FallbackFull = true
		result.ScanDuration = time.Since(startTime)
		return result, nil
	}

	result.ChangedFiles = coalesceFileChanges(sigResult.ChangedFiles)
	result.UnchangedDirs = sigResult.UnchangedDirs
	result.CheckedDirs = sigResult.CheckedDirs
	result.UsedSignatures = true
	result.ScanDuration = time.Since(startTime)
	logger.Scanner("Quick startup completed in %s: %d dirs checked, %d unchanged, %d files changed",
		result.ScanDuration, result.CheckedDirs, result.UnchangedDirs, len(result.ChangedFiles))
	return result, nil
}

func (s *Scanner) signatureBasedStartup() (*QuickStartupResult, error) {
	result := &QuickStartupResult{}
	storedSigs, err := s.db.GetAllDirectorySignatures()
	if err != nil {
		return nil, fmt.Errorf("failed to get directory signatures: %w", err)
	}
	logger.Scanner("Found %d stored directory signatures", len(storedSigs))

	sigMap := make(map[string]db.DirectorySignature, len(storedSigs))
	for _, sig := range storedSigs {
		sigMap[filepath.Clean(sig.Path)] = sig
	}

	folders, err := s.db.GetScanFolders()
	if err != nil {
		return nil, fmt.Errorf("failed to get scan folders: %w", err)
	}

	metadataCache, err := s.db.GetFileMetadataCacheMap()
	if err != nil {
		logger.Scanner("Failed to get metadata cache: %v", err)
		metadataCache = make(map[string]db.FileMetadataCache)
	}
	logger.Scanner("Found %d entries in file metadata cache", len(metadataCache))

	for _, folder := range folders {
		changes, checked, unchanged, err := s.checkDirectoryWithSignature(folder.Path, sigMap, metadataCache)
		if err != nil {
			logger.Scanner("Error checking directory %s: %v", folder.Path, err)
			continue
		}
		result.ChangedFiles = append(result.ChangedFiles, changes...)
		result.CheckedDirs += checked
		result.UnchangedDirs += unchanged
	}
	result.ChangedFiles = coalesceFileChanges(result.ChangedFiles)
	return result, nil
}

func (s *Scanner) checkDirectoryWithSignature(
	dirPath string,
	sigMap map[string]db.DirectorySignature,
	metadataCache map[string]db.FileMetadataCache,
) ([]FileChange, int, int, error) {
	var changes []FileChange
	dirsChecked := 0
	dirsUnchanged := 0

	err := filepath.Walk(dirPath, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if info.IsDir() {
			if shouldSkipDirectory(info.Name()) {
				return filepath.SkipDir
			}
			dirsChecked++
			if sig, exists := sigMap[filepath.Clean(path)]; exists {
				currentSig, err := s.computeQuickDirectorySignature(path)
				if err != nil {
					logger.Scanner("Error computing signature for %s: %v", path, err)
					return nil
				}
				if s.signaturesMatch(sig, currentSig) {
					dirsUnchanged++
					return filepath.SkipDir
				}
			}
			return nil
		}

		ext := strings.ToLower(filepath.Ext(path))
		if !supportedExtensions[ext] {
			return nil
		}
		if change := s.cheapFileChangeCheck(path, info, metadataCache); change != nil {
			changes = append(changes, *change)
		}
		return nil
	})

	return changes, dirsChecked, dirsUnchanged, err
}

func (s *Scanner) cheapFileChangeCheck(
	filePath string,
	info os.FileInfo,
	cache map[string]db.FileMetadataCache,
) *FileChange {
	cleanPath := filepath.Clean(filePath)
	cached, exists := cache[cleanPath]
	if !exists {
		cached, exists = cache[filePath]
	}
	if !exists {
		return &FileChange{
			Path:       cleanPath,
			ChangeType: ChangeTypeCreated,
			NewMtime:   info.ModTime().UnixMilli(),
			NewSize:    info.Size(),
		}
	}

	currentMtime := info.ModTime().UnixMilli()
	currentSize := info.Size()
	if cached.Mtime != currentMtime || cached.FileSize != currentSize {
		return &FileChange{
			Path:       cleanPath,
			ChangeType: ChangeTypeModified,
			OldMtime:   cached.Mtime,
			NewMtime:   currentMtime,
			OldSize:    cached.FileSize,
			NewSize:    currentSize,
		}
	}
	return nil
}

// coalesceFileChanges makes repeated watcher and journal notifications
// deterministic before they reach metadata extraction and persistence.
func coalesceFileChanges(changes []FileChange) []FileChange {
	byPath := make(map[string]FileChange, len(changes))
	order := make([]string, 0, len(changes))
	for _, change := range changes {
		path := filepath.Clean(change.Path)
		change.Path = path
		previous, exists := byPath[path]
		if !exists {
			byPath[path] = change
			order = append(order, path)
			continue
		}

		switch {
		case change.ChangeType == ChangeTypeDeleted:
			// A delete is terminal unless a later create for the same path appears.
			byPath[path] = change
		case previous.ChangeType == ChangeTypeDeleted && change.ChangeType == ChangeTypeCreated:
			// Atomic replace or delete/create becomes a modification.
			change.ChangeType = ChangeTypeModified
			byPath[path] = change
		case previous.ChangeType == ChangeTypeCreated:
			// A new file that is subsequently modified is still a create.
			previous.NewMtime = change.NewMtime
			previous.NewSize = change.NewSize
			byPath[path] = previous
		default:
			byPath[path] = change
		}
	}

	result := make([]FileChange, 0, len(byPath))
	for _, path := range order {
		if change, ok := byPath[path]; ok {
			result = append(result, change)
			delete(byPath, path)
		}
	}
	return result
}
