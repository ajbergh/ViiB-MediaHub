// v2_jobs_analysis.go registers track analysis on the durable job scheduler.
// The work list is re-expanded from the catalog on every run, so an
// interrupted multi-day analysis resumes by skipping tracks that are already
// valid rather than by recovering job state.
package api

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/analysis"
	"github.com/ajbergh/viib-mediahub/internal/analysis/track"
	"github.com/ajbergh/viib-mediahub/internal/db"
)

// JobTypeAnalyzeTracks is the durable job type for library track analysis.
const JobTypeAnalyzeTracks = "analyze_tracks"

// analysisProgressInterval throttles job-row writes. Progress is persisted at
// track granularity, but a 50,000-track run must not issue 50,000 UPDATEs
// faster than the SSE stream can report them.
const analysisProgressInterval = 500 * time.Millisecond

var analysisRegistryOnce sync.Once
var analysisRegistry *analysis.DecoderRegistry

// decoderRegistry returns the shared decoder registry. Registration is
// immutable after construction, so one instance is safe for concurrent jobs.
func decoderRegistry() *analysis.DecoderRegistry {
	analysisRegistryOnce.Do(func() {
		analysisRegistry = analysis.NewDefaultDecoderRegistry()
	})
	return analysisRegistry
}

// runAnalyzeTracksJob expands the recorded selection, analyzes the outstanding
// tracks, and records an aggregate result.
func (a *API) runAnalyzeTracksJob(job db.Job) {
	selection, err := db.ParseAnalysisSelection(job.Parameters)
	if err != nil {
		_ = a.db.FailJob(job.ID, "invalid_analysis_selection", err.Error())
		return
	}
	songIDs, err := a.db.ExpandAnalysisSelection(selection, track.AnalysisVersion, track.AlgorithmVersion)
	if err != nil {
		_ = a.db.FailJob(job.ID, "analysis_selection_failed", err.Error())
		return
	}
	if len(songIDs) == 0 {
		_ = a.db.CompleteJob(job.ID, map[string]any{"mode": selection.Mode, "total": 0, "analyzed": 0, "skipped": 0, "failed": 0},
			"No tracks matched the analysis selection")
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	_ = a.db.UpdateJobProgress(job.ID, 0, int64(len(songIDs)), fmt.Sprintf("Analyzing %d tracks", len(songIDs)))
	lastWrite := time.Now()

	progress, runErr := track.Run(ctx, a.db, decoderRegistry(), songIDs, track.RunOptions{
		Canceled: func() bool { return a.jobCancellationRequested(job.ID) },
		Progress: func(current track.RunProgress) {
			// Always persist the final track so the last update is not dropped.
			if current.Processed < current.Total && time.Since(lastWrite) < analysisProgressInterval {
				return
			}
			lastWrite = time.Now()
			_ = a.db.UpdateJobProgress(job.ID, int64(current.Processed), int64(current.Total),
				fmt.Sprintf("Analyzed %d of %d tracks (%d skipped, %d failed)", current.Analyzed, current.Total, current.Skipped, current.Failed))
		},
	})

	result := map[string]any{
		"mode":     selection.Mode,
		"total":    progress.Total,
		"analyzed": progress.Analyzed,
		"skipped":  progress.Skipped,
		"failed":   progress.Failed,
	}
	if runErr != nil {
		if errors.Is(runErr, context.Canceled) {
			// Outstanding tracks stay outstanding. Re-running the same
			// selection re-dispatches only what is still invalid.
			_ = a.db.CancelJob(job.ID, fmt.Sprintf("Canceled after %d of %d tracks", progress.Processed, progress.Total))
			return
		}
		_ = a.db.FailJob(job.ID, "analysis_run_failed", runErr.Error())
		return
	}
	_ = a.db.CompleteJob(job.ID, result,
		fmt.Sprintf("Analysis complete: %d analyzed, %d skipped, %d failed", progress.Analyzed, progress.Skipped, progress.Failed))
}
