package scanner

import (
	"context"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/logger"
)

// Maintenance shares successful scan boundaries, never an account/queue event.
// One pass examines at most 16 revision groups with a five-second I/O budget.
func (s *Scanner) maintainDownloadedEvidence(roots, paths []string, coverage time.Time) {
	parent := s.ctx
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()
	remaining := 16
	for {
		batch := paths
		if len(batch) > 512 {
			batch = paths[:512]
		}
		result, err := s.db.MaintainSpotifyDownloadRetention(ctx, roots, batch, coverage, remaining)
		if err != nil {
			logger.Scanner("Downloaded evidence maintenance deferred: %v", err)
			return
		}
		if result.Collected > 0 {
			logger.Scanner("Collected %d expired downloaded evidence revision groups", result.Collected)
		}
		remaining -= result.Checked
		if len(paths) <= len(batch) || remaining <= 0 {
			return
		}
		paths = paths[len(batch):]
		roots = nil
	}
}

// Cache age cleanup is independent of filesystem ownership and has its own
// cancellation budget. It remains outside panel requests and startup.
func (s *Scanner) maintainPrivateSpotifyCache() {
	parent := s.ctx
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithTimeout(parent, 2*time.Second)
	defer cancel()
	result, err := s.db.MaintainSpotifyPrivateCache(ctx, time.Now(), 16)
	if err != nil {
		logger.Scanner("Private Spotify cache maintenance deferred: %v", err)
		return
	}
	if result.Collected > 0 {
		logger.Scanner("Collected %d expired private Spotify cache payload groups", result.Collected)
	}
}
