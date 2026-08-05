// metadata_executor.go bounds concurrent native metadata extraction globally.
package scanner

import (
	"fmt"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"

	"github.com/ajbergh/viib-mediahub/internal/logger"
)

var (
	metadataLimitOnce sync.Once
	metadataSlots     chan struct{}
	metadataActive    int64
	metadataPeak      int64
)

func metadataConcurrency() int {
	workers := runtime.NumCPU() / 4
	if workers < 1 { workers = 1 }
	if workers > 4 { workers = 4 }
	return workers
}

func initializeMetadataSlots() {
	metadataSlots = make(chan struct{}, metadataConcurrency())
}

// extractMetadataBounded prevents a background scanner from opening and parsing
// an unbounded number of media files concurrently. Files with repeated native
// parser timeouts are quarantined with exponential retry delay.
func (s *Scanner) extractMetadataBounded(filePath string) (*SongMetadata, error) {
	metadataLimitOnce.Do(initializeMetadataSlots)
	cleanPath := filepath.Clean(filePath)
	allowed, err := s.db.ScannerFailureRetryAllowed(cleanPath)
	if err != nil {
		logger.Scanner("Unable to read scanner failure state for %s: %v", cleanPath, err)
	} else if !allowed {
		return nil, fmt.Errorf("media file is temporarily quarantined: %s", cleanPath)
	}

	select {
	case metadataSlots <- struct{}{}:
	case <-s.ctx.Done():
		return nil, s.ctx.Err()
	}
	active := atomic.AddInt64(&metadataActive, 1)
	for {
		peak := atomic.LoadInt64(&metadataPeak)
		if active <= peak || atomic.CompareAndSwapInt64(&metadataPeak, peak, active) { break }
	}
	defer func() {
		atomic.AddInt64(&metadataActive, -1)
		<-metadataSlots
	}()

	metadata, err := s.extractMetadata(cleanPath)
	if err != nil {
		_ = s.db.RecordScannerFailure(cleanPath, "metadata_error", err.Error())
		return nil, err
	}
	// The legacy timeout fallback intentionally returns minimal metadata with an
	// empty stable fingerprint. Treat that as a quarantinable timeout rather than
	// persisting a path-only identity.
	if metadata == nil || metadata.FileHash == "" {
		timeoutErr := fmt.Errorf("metadata extraction did not produce a stable fingerprint")
		_ = s.db.RecordScannerFailure(cleanPath, "metadata_timeout", timeoutErr.Error())
		return nil, timeoutErr
	}
	if err := s.db.ClearScannerFailure(cleanPath); err != nil {
		logger.Scanner("Unable to clear scanner failure for %s: %v", cleanPath, err)
	}
	return metadata, nil
}

// MetadataExecutorStats reports the configured concurrency and current/peak use.
func MetadataExecutorStats() (limit int, active int64, peak int64) {
	metadataLimitOnce.Do(initializeMetadataSlots)
	return cap(metadataSlots), atomic.LoadInt64(&metadataActive), atomic.LoadInt64(&metadataPeak)
}
