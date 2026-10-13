package track

import (
	"context"
	"database/sql"
	"errors"
	"github.com/ajbergh/viib-mediahub/internal/analysis"
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
	token, _, claimed, err := claimPreparationWithSnapshot(ctx, database, song, fp)
	return token, claimed, err
}

func claimPreparationWithSnapshot(ctx context.Context, database *db.DB, song, fp string) (string, *db.TrackAnalysis, bool, error) {
	return claimPreparationSnapshot(ctx, database, song, fp, false, nil)
}

func observedPreparationFingerprint(database *db.DB, song string) (*string, error) {
	record, err := database.GetTrackAnalysis(song)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &record.SourceFingerprint, nil
}

func claimResolvedPreparation(ctx context.Context, database *db.DB, source analysis.ResolvedSource, resolve func(context.Context, string) (analysis.ResolvedSource, error)) (string, *db.TrackAnalysis, bool, error) {
	expected, err := observedPreparationFingerprint(database, source.SongID)
	if err != nil {
		return "", nil, false, err
	}
	current, err := resolve(ctx, source.SongID)
	if err != nil {
		return "", nil, false, err
	}
	if ctx.Err() != nil {
		return "", nil, false, ctx.Err()
	}
	if current.Fingerprint != source.Fingerprint {
		return "", nil, false, nil
	}
	return claimPreparationSnapshot(ctx, database, source.SongID, source.Fingerprint, true, expected)
}

func claimPreparationSnapshot(ctx context.Context, database *db.DB, song, fp string, compare bool, expected *string) (string, *db.TrackAnalysis, bool, error) {
	for {
		var token string
		var previous *db.TrackAnalysis
		var claimed bool
		var err error
		if compare {
			token, previous, claimed, err = database.ClaimTrackAnalysisLeaseIfUnchanged(song, fp, AnalysisVersion, AlgorithmVersion, expected)
		} else {
			token, previous, claimed, err = database.ClaimTrackAnalysisLeaseWithSnapshot(song, fp, AnalysisVersion, AlgorithmVersion)
		}
		if !errors.Is(err, db.ErrWaveformLeaseBusy) {
			return token, previous, claimed, err
		}
		select {
		case <-ctx.Done():
			return "", nil, false, ctx.Err()
		case <-time.After(25 * time.Millisecond):
		}
	}
}
