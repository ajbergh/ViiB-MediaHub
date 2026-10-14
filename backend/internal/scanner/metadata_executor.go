// metadata_executor.go bounds parser work, including operations whose callers timed out.
package scanner

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"sync/atomic"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/logger"
)

var errMetadataCapacity = errors.New("metadata parser capacity exhausted")
var errMetadataTimeout = errors.New("metadata parser timed out")

type metadataExecutor struct {
	slots  chan struct{}
	active int64
	peak   int64
}

var sharedMetadataExecutor = metadataExecutor{slots: make(chan struct{}, metadataConcurrency())}

func metadataConcurrency() int {
	return min(4, max(1, runtime.NumCPU()/4))
}

// extract retains the slot until the parser really exits. TagLib has no caller
// cancellation API; returning a timeout must never admit more abandoned work.
func (e *metadataExecutor) extract(ctx context.Context, timeout time.Duration, parse func() (*SongMetadata, error)) (*SongMetadata, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	acquire := time.NewTimer(min(timeout, time.Second))
	defer acquire.Stop()
	select {
	case e.slots <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-acquire.C:
		return nil, errMetadataCapacity
	}
	active := atomic.AddInt64(&e.active, 1)
	for {
		peak := atomic.LoadInt64(&e.peak)
		if active <= peak || atomic.CompareAndSwapInt64(&e.peak, peak, active) {
			break
		}
	}
	type result struct {
		metadata *SongMetadata
		err      error
	}
	done := make(chan result, 1)
	go func() {
		var r result
		defer func() {
			if recovered := recover(); recovered != nil {
				r = result{err: fmt.Errorf("metadata parser panic: %v", recovered)}
				logger.Scanner("%v\n%s", r.err, debug.Stack())
			}
			atomic.AddInt64(&e.active, -1)
			<-e.slots
			done <- r
		}()
		r.metadata, r.err = parse()
	}()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case r := <-done:
		return r.metadata, r.err
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-timer.C:
		return nil, errMetadataTimeout
	}
}

func (s *Scanner) extractMetadataBounded(filePath string) (*SongMetadata, error) {
	cleanPath := filepath.Clean(filePath)
	allowed, err := s.db.ScannerFailureRetryAllowed(cleanPath)
	if err != nil {
		logger.Scanner("Unable to read scanner failure state for %s: %v", cleanPath, err)
	} else if !allowed {
		return nil, fmt.Errorf("media file is temporarily quarantined: %s", cleanPath)
	}
	metadata, err := s.extractMetadata(cleanPath)
	if err != nil {
		// Capacity and cancellation are transient process conditions, not bad files.
		if !errors.Is(err, errMetadataCapacity) && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
			code := "metadata_error"
			if errors.Is(err, errMetadataTimeout) {
				code = "metadata_timeout"
			}
			_ = s.db.RecordScannerFailure(cleanPath, code, err.Error())
		}
		return nil, err
	}
	if metadata == nil || metadata.FileHash == "" {
		err := fmt.Errorf("metadata extraction did not produce a stable fingerprint")
		_ = s.db.RecordScannerFailure(cleanPath, "metadata_error", err.Error())
		return nil, err
	}
	if err := s.db.ClearScannerFailure(cleanPath); err != nil {
		logger.Scanner("Unable to clear scanner failure for %s: %v", cleanPath, err)
	}
	return metadata, nil
}

func MetadataExecutorStats() (limit int, active int64, peak int64) {
	return cap(sharedMetadataExecutor.slots), atomic.LoadInt64(&sharedMetadataExecutor.active), atomic.LoadInt64(&sharedMetadataExecutor.peak)
}
