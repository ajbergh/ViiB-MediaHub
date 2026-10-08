// v2_jobs_analysis.go registers track analysis on the durable job scheduler.
// The work list is re-expanded from the catalog on every run, so an
// interrupted multi-day analysis resumes by skipping tracks that are already
// valid rather than by recovering job state.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/analysis"
	"github.com/ajbergh/viib-mediahub/internal/analysis/track"
	"github.com/ajbergh/viib-mediahub/internal/db"
	"github.com/ajbergh/viib-mediahub/internal/logger"
	"github.com/google/uuid"
)

// JobTypeAnalyzeTracks is the durable job type for library track analysis.
const JobTypeAnalyzeTracks = "analyze_tracks"

// SettingAutoCueMode controls automatic generated cue persistence for newly
// created analysis jobs. It is installation-wide in the current settings store.
const SettingAutoCueMode = "analysis_auto_cue_mode"

// errAnalysisDeferred signals that a run must yield its worker rather than
// fail. It never reaches the caller of a route; the job returns to the queue.
var errAnalysisDeferred = errors.New("analysis deferred to reduce pressure during playback")

// analysisProgressInterval throttles job-row writes. Progress is persisted at
// track granularity, but a 50,000-track run must not issue 50,000 UPDATEs
// faster than the SSE stream can report them.
const analysisProgressInterval = 500 * time.Millisecond

// analysisDeferBackoff keeps a deferred job out of the queue long enough that
// the dispatcher does not immediately re-claim it and spin. Clearing playback
// pressure wakes the scheduler, so this is an upper bound on resume latency
// only when the frontend stops reporting without saying so.
const analysisDeferBackoff = 15 * time.Second

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
	ctx, cancel := a.analysisJobContext(job.ID)
	defer cancel()
	songIDs, err := track.ExpandPreparationJobSelection(ctx, a.db, selection, func(ctx context.Context, id string) (analysis.ResolvedSource, error) {
		if a.jobCancellationRequested(job.ID) {
			cancel()
			return analysis.ResolvedSource{}, context.Canceled
		}
		return a.resolveAnalysisSource(ctx, id)
	})
	if err != nil {
		if errors.Is(err, track.ErrPreparationBusy) {
			a.deferPreparationClaimJob(job.ID)
			return
		}
		if errors.Is(err, context.Canceled) {
			_ = a.db.CancelJob(job.ID, "Canceled during preparation discovery")
			return
		}
		_ = a.db.FailJob(job.ID, "analysis_selection_failed", err.Error())
		return
	}
	if len(songIDs) == 0 {
		_, _ = a.completeCancelableJob(job.ID, map[string]any{"mode": selection.Mode, "total": 0, "analyzed": 0, "skipped": 0, "failed": 0},
			"No tracks matched the analysis selection")
		return
	}
	// Yield before starting rather than holding a worker while playback runs.
	if a.analysisThrottled(job.Priority) {
		a.deferAnalysisJob(job.ID)
		return
	}
	logger.Analysis("job started job_id=%q mode=%q tracks=%d algorithm=%q", job.ID, selection.Mode, len(songIDs), track.AlgorithmVersion)

	_ = a.db.UpdateJobProgress(job.ID, 0, int64(len(songIDs)), fmt.Sprintf("Analyzing %d tracks", len(songIDs)))
	lastWrite := time.Now()

	progress, runErr := track.Run(ctx, a.db, decoderRegistry(), songIDs, track.RunOptions{
		ResolveSource:       a.resolveAnalysisSource,
		ProviderPreparation: a.spotifyPreparationForSource,
		EnrichValid:         a.spotifyFeaturesEnabled(),
		AutoCueMode:         selection.AutoCueMode,
		Canceled:            func() bool { return a.jobCancellationRequested(job.ID) },
		Throttle: func(context.Context) error {
			// Consulted between tracks. Yielding releases the worker so scans
			// and foreground analysis are not stuck behind a paused run.
			if a.analysisThrottled(job.Priority) {
				return errAnalysisDeferred
			}
			return nil
		},
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
		"mode":        selection.Mode,
		"source":      selection.Source,
		"autoCueMode": selection.AutoCueMode,
		"total":       progress.Total,
		"analyzed":    progress.Analyzed,
		"skipped":     progress.Skipped,
		"failed":      progress.Failed,
	}
	if runErr != nil {
		if errors.Is(runErr, errAnalysisDeferred) {
			// Outstanding tracks stay outstanding; the settled ones are skipped
			// when this job is claimed again.
			a.deferAnalysisJob(job.ID)
			return
		}
		if errors.Is(runErr, context.Canceled) {
			// Outstanding tracks stay outstanding. Re-running the same
			// selection re-dispatches only what is still invalid.
			_ = a.db.CancelJob(job.ID, fmt.Sprintf("Canceled after %d of %d tracks", progress.Processed, progress.Total))
			logger.Analysis("job canceled job_id=%q mode=%q processed=%d total=%d", job.ID, selection.Mode, progress.Processed, progress.Total)
			return
		}
		_ = a.db.FailJob(job.ID, "analysis_run_failed", runErr.Error())
		logger.Analysis("job failed job_id=%q mode=%q processed=%d total=%d error=%q", job.ID, selection.Mode, progress.Processed, progress.Total, runErr)
		return
	}
	if claimErr := track.CheckPreparationSelectionClaims(ctx, a.db, selection); claimErr != nil {
		if errors.Is(claimErr, track.ErrPreparationBusy) {
			a.deferPreparationClaimJob(job.ID)
			return
		}
		if errors.Is(claimErr, context.Canceled) {
			_ = a.db.CancelJob(job.ID, "Analysis canceled before completion")
			return
		}
		_ = a.db.FailJob(job.ID, "analysis_claim_check_failed", claimErr.Error())
		return
	}
	completed, completeErr := a.completeCancelableJob(job.ID, result,
		fmt.Sprintf("Analysis complete: %d analyzed, %d skipped, %d failed", progress.Analyzed, progress.Skipped, progress.Failed))
	if completeErr != nil {
		logger.Analysis("job completion persistence failed job_id=%q mode=%q error=%q", job.ID, selection.Mode, completeErr)
		return
	}
	if !completed {
		return
	}
	logger.Analysis("job completed job_id=%q mode=%q total=%d analyzed=%d skipped=%d failed=%d", job.ID, selection.Mode, progress.Total, progress.Analyzed, progress.Skipped, progress.Failed)
}

// deferAnalysisJob returns a job to the durable queue so it resumes once
// playback pressure clears. A concurrent cancellation is finalized when the
// running-to-queued transition is refused.
func (a *API) deferAnalysisJob(id string) {
	if a.jobCancellationRequested(id) {
		_ = a.db.CancelJob(id, "Analysis canceled while waiting for playback")
		return
	}
	requeued, _ := a.db.RequeueJob(id, "Waiting for DJ playback to finish before analyzing", analysisDeferBackoff)
	if !requeued && a.jobCancellationRequested(id) {
		_ = a.db.CancelJob(id, "Analysis canceled while waiting for playback")
	}
}

// SettingAutoAnalyzeNewTracks enables queueing analysis for tracks a scan just
// added. It defaults to on because DJ deck loading relies on the durable
// analysis record; users can explicitly turn it off in Library Operations.
const SettingAutoAnalyzeNewTracks = "analysis_auto_analyze_new"

// autoAnalyzePriority keeps scan-triggered analysis below anything a user asked
// for directly, including the default priority of a manually created job.
const autoAnalyzePriority = -10

// queueAutoAnalysis enqueues background analysis for newly scanned tracks when
// the setting is enabled. It is a best-effort convenience: a failure to queue
// must never turn a successful scan into a failed one.
func (a *API) queueAutoAnalysis(trigger string) {
	value, err := a.db.GetSetting(SettingAutoAnalyzeNewTracks)
	if err != nil || (strings.TrimSpace(value) != "" && !isEnabledSetting(value)) {
		return
	}
	// Only an unstarted local missing-selection run covers newly scanned tracks.
	// A running job has already expanded its work list and needs one follow-up.
	job := db.Job{
		ID: uuid.NewString(), Type: JobTypeAnalyzeTracks, Status: db.JobStatusQueued,
		Parameters: autoAnalysisParameters(db.AnalysisSelection{Mode: db.AnalysisSelectionMissing, AutoCueMode: a.analysisAutoCueMode()}), Priority: autoAnalyzePriority,
		Message: "Queued automatically after " + trigger,
	}
	queued, err := a.db.QueueMissingAnalysisIfNeeded(job)
	if err != nil || !queued {
		return
	}
	a.wakeJobScheduler()
}

func (a *API) analysisAutoCueMode() db.AutomaticCuePointMode {
	value, err := a.db.GetSetting(SettingAutoCueMode)
	if err != nil {
		return db.AutomaticCuePointsFillEmpty
	}
	return db.NormalizeAutomaticCuePointMode(value)
}

func autoAnalysisParameters(selection db.AnalysisSelection) json.RawMessage {
	parameters, err := json.Marshal(selection)
	if err != nil {
		return json.RawMessage(`{"mode":"missing","autoCueMode":"fill-empty"}`)
	}
	return parameters
}

// isEnabledSetting accepts the several truthy spellings the settings store has
// accumulated rather than assuming one.
func isEnabledSetting(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on", "enabled":
		return true
	default:
		return false
	}
}

func (a *API) completeCancelableJob(id string, result any, message string) (bool, error) {
	completed, err := a.db.CompleteJobIfRunning(id, result, message)
	if err == nil && !completed && a.jobCancellationRequested(id) {
		return false, a.db.CancelJob(id, "Operation canceled before completion")
	}
	return completed, err
}

func (a *API) deferPreparationClaimJob(id string) {
	if a.jobCancellationRequested(id) {
		_ = a.db.CancelJob(id, "Analysis canceled while waiting for preparation ownership")
		return
	}
	queued, _ := a.db.RequeueJob(id, "Waiting for an existing preparation worker", time.Second)
	if queued {
		// The idle scheduler poll is 30s. Wake existing workers when this short
		// contention backoff ends instead of stranding otherwise-ready jobs.
		time.AfterFunc(time.Second, func() {
			a.jobSchedulerMu.Lock()
			wake := a.jobWake
			a.jobSchedulerMu.Unlock()
			if wake != nil {
				select {
				case wake <- struct{}{}:
				default:
				}
			}
		})
	}
	if !queued && a.jobCancellationRequested(id) {
		_ = a.db.CancelJob(id, "Analysis canceled while waiting for preparation ownership")
	}
}

// Cancellation must reach source reads and lease waits, not just track boundaries.
func (a *API) analysisJobContext(id string) (context.Context, func()) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if a.jobCancellationRequested(id) {
					cancel()
					return
				}
			}
		}
	}()
	var once sync.Once
	return ctx, func() { once.Do(func() { cancel(); <-done }) }
}
