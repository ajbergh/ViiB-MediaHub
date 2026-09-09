package track

import (
	"context"

	"github.com/ajbergh/viib-mediahub/internal/analysis"
	"github.com/ajbergh/viib-mediahub/internal/db"
	"github.com/ajbergh/viib-mediahub/internal/logger"
)

// RunProgress is the aggregate state of one analysis run. Counts are derived
// from per-track outcomes, not from a single opaque counter, so a partially
// finished run can be reported honestly.
type RunProgress struct {
	Total     int
	Processed int
	Analyzed  int
	Skipped   int
	Failed    int
	SongID    string
}

// Done reports how many tracks are settled, which is what the job row's
// progress counter tracks.
func (p RunProgress) Done() int { return p.Processed }

// RunOptions configures one pass over a work list.
type RunOptions struct {
	Analysis Options
	// Canceled is consulted between tracks. Cancellation is cooperative at
	// track granularity: suspending DSP mid-track would buy a few seconds of
	// latency at the cost of a much harder invariant.
	Canceled func() bool
	// Progress is called after each track settles.
	Progress func(RunProgress)
	// Throttle is consulted before each track and may block, which is how DJ
	// playback reduces background analysis pressure without an audio dropout.
	Throttle func(context.Context) error
}

// Run analyzes each song in the work list, skipping tracks whose durable
// record already came from the current analyzer and the current source bytes.
// It returns the final progress; a canceled run returns what it completed
// along with context.Canceled.
func Run(ctx context.Context, database *db.DB, registry *analysis.DecoderRegistry, songIDs []string, opts RunOptions) (RunProgress, error) {
	progress := RunProgress{Total: len(songIDs)}
	if opts.Analysis == (Options{}) {
		opts.Analysis = DefaultOptions()
	}
	for _, songID := range songIDs {
		if err := ctx.Err(); err != nil {
			return progress, err
		}
		if opts.Canceled != nil && opts.Canceled() {
			return progress, context.Canceled
		}
		if opts.Throttle != nil {
			if err := opts.Throttle(ctx); err != nil {
				return progress, err
			}
		}
		settled := analyzeOne(ctx, database, registry, songID, opts.Analysis)
		// A cancellation that arrived mid-decode leaves the track outstanding
		// rather than failed, so it must not be counted before returning.
		if err := ctx.Err(); err != nil {
			return progress, err
		}
		progress.Processed++
		progress.SongID = songID
		switch settled {
		case outcomeSkipped:
			progress.Skipped++
		case outcomeFailed:
			progress.Failed++
		default:
			progress.Analyzed++
		}
		if opts.Progress != nil {
			opts.Progress(progress)
		}
	}
	return progress, nil
}

type outcome int

const (
	outcomeAnalyzed outcome = iota
	outcomeSkipped
	outcomeFailed
)

// analyzeOne settles exactly one track. Every terminal condition is persisted,
// so a source that cannot be analyzed is not retried on the next run.
func analyzeOne(ctx context.Context, database *db.DB, registry *analysis.DecoderRegistry, songID string, opts Options) outcome {
	source, err := analysis.ResolveLocalSource(database, songID)
	if err != nil {
		code, message := ClassifyError(err)
		if persistErr := PersistFailure(database, songID, analysis.ResolvedSource{}, code, message); persistErr != nil {
			logger.Analysis("track failed song_id=%q code=%q error=%q persist_error=%q", songID, code, message, persistErr)
		} else {
			logger.Analysis("track failed song_id=%q code=%q error=%q", songID, code, message)
		}
		return outcomeFailed
	}
	valid, err := database.TrackAnalysisValid(songID, source.Fingerprint, AnalysisVersion, AlgorithmVersion)
	if err == nil && valid {
		return outcomeSkipped
	}
	if err != nil {
		logger.Analysis("validity check failed song_id=%q path=%q error=%q", songID, source.Path, err)
	}
	// Two overlapping jobs can expand the same selection. The claim ensures only
	// one of them decodes the file; the loser treats the track as another
	// worker's responsibility rather than duplicating the work.
	claimed, err := database.ClaimTrackAnalysis(songID, source.Fingerprint, AnalysisVersion, AlgorithmVersion)
	if err != nil {
		logger.Analysis("track claim failed song_id=%q path=%q error=%q", songID, source.Path, err)
		return outcomeFailed
	}
	if !claimed {
		return outcomeSkipped
	}

	result, err := Analyze(ctx, database, registry, songID, opts)
	if err != nil {
		if ctx.Err() != nil {
			// Do not persist a cancellation as a track-level failure; the work
			// is still outstanding, so the claim has to be given back or an
			// immediate resume would skip it for the whole lease window.
			_ = database.ReleaseTrackAnalysis(songID)
			return outcomeFailed
		}
		code, message := ClassifyError(err)
		if persistErr := PersistFailure(database, songID, source, code, message); persistErr != nil {
			logger.Analysis("track failed song_id=%q path=%q code=%q error=%q persist_error=%q", songID, source.Path, code, message, persistErr)
		} else {
			logger.Analysis("track failed song_id=%q path=%q code=%q error=%q", songID, source.Path, code, message)
		}
		return outcomeFailed
	}
	if err := Persist(database, result); err != nil {
		_ = database.ReleaseTrackAnalysis(songID)
		logger.Analysis("track persistence failed song_id=%q path=%q status=%q error=%q", songID, source.Path, result.Status, err)
		return outcomeFailed
	}
	if code, message := resultIssue(result); code != "" {
		logger.Analysis("track incomplete song_id=%q path=%q status=%q code=%q error=%q tempo_known=%t tempo_crest=%.3f key_known=%t key_flatness=%.6f",
			songID, source.Path, result.Status, code, message, result.Tempo.Known, result.Tempo.OnsetCrestFactor, result.Key.Known, result.Key.Flatness)
	}
	if result.Status == db.TrackAnalysisFailed {
		return outcomeFailed
	}
	return outcomeAnalyzed
}
