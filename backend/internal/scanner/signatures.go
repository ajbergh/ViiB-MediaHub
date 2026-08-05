package scanner

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/db"
	"github.com/ajbergh/viib-mediahub/internal/logger"
)

// computeQuickDirectorySignature computes a directory signature without opening
// audio payloads. Only filename, size, and modification time are inspected.
func (s *Scanner) computeQuickDirectorySignature(dirPath string) (db.DirectorySignature, error) {
	sig := db.DirectorySignature{
		Path:         filepath.Clean(dirPath),
		LastVerified: time.Now().UnixMilli(),
	}

	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return sig, err
	}

	type fileInfo struct {
		name  string
		size  int64
		mtime int64
	}
	files := make([]fileInfo, 0, len(entries))

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if !supportedExtensions[ext] {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		mtime := info.ModTime().UnixMilli()
		sig.FileCount++
		sig.TotalSize += info.Size()
		if mtime > sig.LatestMtime {
			sig.LatestMtime = mtime
		}
		files = append(files, fileInfo{name: entry.Name(), size: info.Size(), mtime: mtime})
	}

	sort.Slice(files, func(i, j int) bool { return files[i].name < files[j].name })
	hasher := sha256.New()
	for _, file := range files {
		_, _ = fmt.Fprintf(hasher, "%s|%d|%d|", file.name, file.size, file.mtime)
	}
	sig.ContentHash = hex.EncodeToString(hasher.Sum(nil))[:16]
	return sig, nil
}

func (s *Scanner) signaturesMatch(stored, current db.DirectorySignature) bool {
	return stored.FileCount == current.FileCount &&
		stored.TotalSize == current.TotalSize &&
		stored.LatestMtime == current.LatestMtime &&
		stored.ContentHash == current.ContentHash
}

// ComputeAndSaveDirectorySignatures refreshes the signature cache after a full
// scan. Errors on individual directories are logged without discarding the
// successful signatures from the same root.
func (s *Scanner) ComputeAndSaveDirectorySignatures() error {
	folders, err := s.db.GetScanFolders()
	if err != nil {
		return fmt.Errorf("failed to get scan folders: %w", err)
	}

	var signatures []db.DirectorySignature
	for _, folder := range folders {
		walkErr := filepath.Walk(folder.Path, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			if !info.IsDir() {
				return nil
			}
			if shouldSkipDirectory(info.Name()) {
				return filepath.SkipDir
			}
			sig, err := s.computeQuickDirectorySignature(path)
			if err != nil {
				logger.Scanner("Error computing signature for %s: %v", path, err)
				return nil
			}
			if sig.FileCount > 0 {
				signatures = append(signatures, sig)
			}
			return nil
		})
		if walkErr != nil {
			logger.Scanner("Error walking %s: %v", folder.Path, walkErr)
		}
	}

	if len(signatures) == 0 {
		return nil
	}
	logger.Scanner("Saving %d directory signatures", len(signatures))
	if err := s.db.SaveDirectorySignaturesBatch(signatures); err != nil {
		return fmt.Errorf("failed to save directory signatures: %w", err)
	}
	return nil
}

// UpdateFileMetadataCache updates the inexpensive mtime/size cache used by
// signature fallback detection.
func (s *Scanner) UpdateFileMetadataCache(files []string) error {
	caches := make([]db.FileMetadataCache, 0, len(files))
	now := time.Now().UnixMilli()
	for _, filePath := range files {
		cleanPath := filepath.Clean(filePath)
		info, err := os.Stat(cleanPath)
		if err != nil {
			continue
		}
		caches = append(caches, db.FileMetadataCache{
			FilePath:     cleanPath,
			FileSize:     info.Size(),
			Mtime:        info.ModTime().UnixMilli(),
			LastVerified: now,
		})
	}
	if len(caches) == 0 {
		return nil
	}
	return s.db.SaveFileMetadataCacheBatch(caches)
}

func (s *Scanner) SaveScanState(result *ScanResult) error {
	return s.db.SaveScanState(db.ScanState{
		LastScanTime:   time.Now().UnixMilli(),
		ScanDurationMs: result.Duration.Milliseconds(),
		FilesScanned:   result.TotalFiles,
		FilesChanged:   result.NewSongs + result.UpdatedSongs,
	})
}

// DetectDeletedFiles finds cache entries whose files disappeared or whose paths
// now fall under ignored system directories.
func (s *Scanner) DetectDeletedFiles() ([]FileChange, error) {
	cache, err := s.db.GetAllFileMetadataCache()
	if err != nil {
		return nil, err
	}
	changes := make([]FileChange, 0)
	for _, cached := range cache {
		cleanPath := filepath.Clean(cached.FilePath)
		if pathContainsSkippedDirectory(cleanPath) {
			logger.Scanner("Marking file for removal (in skipped directory): %s", cleanPath)
			changes = append(changes, FileChange{
				Path: cleanPath, ChangeType: ChangeTypeDeleted,
				OldMtime: cached.Mtime, OldSize: cached.FileSize,
			})
			continue
		}
		if _, statErr := os.Stat(cleanPath); os.IsNotExist(statErr) {
			changes = append(changes, FileChange{
				Path: cleanPath, ChangeType: ChangeTypeDeleted,
				OldMtime: cached.Mtime, OldSize: cached.FileSize,
			})
		}
	}
	return coalesceFileChanges(changes), nil
}
