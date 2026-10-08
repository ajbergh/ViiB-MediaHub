package track

import (
	"context"
	"errors"
	"github.com/ajbergh/viib-mediahub/internal/db"
	"sync"
	"time"
)

// A failed renewal cancels local/provider work. Every publication and release
// still checks the durable token, so cancellation is not the ownership fence.
func maintainTrackAnalysisLease(parent context.Context, database *db.DB, songID, fingerprint, token string, interval time.Duration) (context.Context, func()) {
	ctx, cancel := context.WithCancel(parent)
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := database.RenewTrackAnalysisLease(songID, fingerprint, token); err != nil {
					cancel()
					return
				}
			}
		}
	}()
	var once sync.Once
	return ctx, func() { once.Do(func() { cancel(); <-done }) }
}

const claimHeartbeatInterval = time.Duration(db.TrackAnalysisLeaseMillis/3) * time.Millisecond

// A waveform owner must not cause a durable preparation job to count unfinished
// scalar work as skipped. Wait with cancellation, then claim normally.
func claimPreparation(ctx context.Context, database *db.DB, song, fp string) (string, bool, error) {
	for {
		token, claimed, err := database.ClaimTrackAnalysisLease(song, fp, AnalysisVersion, AlgorithmVersion)
		if !errors.Is(err, db.ErrWaveformLeaseBusy) {
			return token, claimed, err
		}
		select {
		case <-ctx.Done():
			return "", false, ctx.Err()
		case <-time.After(25 * time.Millisecond):
		}
	}
}
