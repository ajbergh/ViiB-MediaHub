// v2_jobs.go exposes persistent scan and aggregate-refresh jobs plus SSE.
package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"runtime"
	"strings"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/db"
	"github.com/ajbergh/viib-mediahub/internal/scanner"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

const (
	maxJobRequestBytes = 64 * 1024
	// schedulerIdlePollInterval bounds how long durable queued work can wait
	// when no explicit wake signal arrives.
	schedulerIdlePollInterval = 30 * time.Second
	minJobPriority            = -100
	maxJobPriority            = 100
)

func schedulerWorkerCount(cpuCount int) int {
	workers := cpuCount / 4
	if workers < 1 {
		return 1
	}
	if workers > 2 {
		return 2
	}
	return workers
}

type createJobRequest struct {
	Type       string          `json:"type"`
	Parameters json.RawMessage `json:"parameters,omitempty"`
	Priority   int             `json:"priority,omitempty"`
}

// V2JobRoutes returns routes for creating, observing, canceling, and retrying jobs.
func (a *API) V2JobRoutes() chi.Router {
	r := chi.NewRouter()
	if err := a.db.EnsureJobSchema(); err != nil {
		// Individual handlers will return a structured database error.
	} else {
		// Recoverable queued work may predate this process. Start the bounded
		// dispatcher while wiring routes so it is not dependent on a later API call.
		a.wakeJobScheduler()
	}
	r.Get("/", a.listJobsV2)
	r.Post("/", a.createJobV2)
	r.Get("/events", a.jobEventsV2)
	r.Get("/{id}", a.getJobV2)
	r.Post("/{id}/cancel", a.cancelJobV2)
	r.Post("/{id}/retry", a.retryJobV2)
	r.Post("/pause", a.pauseJobsV2)
	r.Post("/resume", a.resumeJobsV2)
	return r
}

func (a *API) pauseJobsV2(w http.ResponseWriter, r *http.Request) {
	count, err := a.db.PauseQueuedJobs()
	if err != nil {
		respondV2Error(w, r, http.StatusInternalServerError, "job_pause_failed", "Unable to pause queued jobs", true, nil)
		return
	}
	respondV2JSON(w, http.StatusAccepted, map[string]int64{"paused": count})
}

func (a *API) resumeJobsV2(w http.ResponseWriter, r *http.Request) {
	count, err := a.db.ResumePausedJobs()
	if err != nil {
		respondV2Error(w, r, http.StatusInternalServerError, "job_resume_failed", "Unable to resume queued jobs", true, nil)
		return
	}
	a.wakeJobScheduler()
	respondV2JSON(w, http.StatusAccepted, map[string]int64{"resumed": count})
}

func (a *API) listJobsV2(w http.ResponseWriter, r *http.Request) {
	jobs, err := a.db.ListJobs(parseBoundedInt(r.URL.Query().Get("limit"), 100, 500), r.URL.Query().Get("status"))
	if err != nil {
		respondV2Error(w, r, http.StatusInternalServerError, "jobs_unavailable", "Unable to list operation jobs", true, nil)
		return
	}
	respondV2JSON(w, http.StatusOK, map[string]any{"jobs": jobs})
}

func (a *API) getJobV2(w http.ResponseWriter, r *http.Request) {
	job, err := a.db.GetJob(chi.URLParam(r, "id"))
	if err != nil {
		respondV2Error(w, r, http.StatusNotFound, "job_not_found", "Operation job was not found", false, nil)
		return
	}
	respondV2JSON(w, http.StatusOK, job)
}

func (a *API) createJobV2(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxJobRequestBytes)
	var request createJobRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		respondV2Error(w, r, http.StatusBadRequest, "invalid_request", "The job request is not valid JSON", false, nil)
		return
	}
	request.Type = strings.ToLower(strings.TrimSpace(request.Type))
	if request.Type != "full_scan" && request.Type != "quick_scan" && request.Type != "refresh_genre_stats" {
		respondV2Error(w, r, http.StatusBadRequest, "unsupported_job_type", "Supported job types are full_scan, quick_scan, and refresh_genre_stats", false, map[string]any{"type": request.Type})
		return
	}
	if request.Priority < minJobPriority || request.Priority > maxJobPriority {
		respondV2Error(w, r, http.StatusBadRequest, "invalid_job_priority", "Job priority must be between -100 and 100", false, nil)
		return
	}

	job := db.Job{ID: uuid.NewString(), Type: request.Type, Status: db.JobStatusQueued, Parameters: request.Parameters, Priority: request.Priority, Message: "Queued"}
	if err := a.db.CreateJob(job); err != nil {
		respondV2Error(w, r, http.StatusInternalServerError, "job_create_failed", "Unable to create the operation job", true, nil)
		return
	}
	a.wakeJobScheduler()
	created, err := a.db.GetJob(job.ID)
	if err != nil {
		created = job
	}
	respondV2JSON(w, http.StatusAccepted, created)
}

func (a *API) cancelJobV2(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	changed, err := a.db.RequestJobCancellation(id)
	if err != nil {
		respondV2Error(w, r, http.StatusInternalServerError, "job_cancel_failed", "Unable to request cancellation", true, nil)
		return
	}
	if !changed {
		respondV2Error(w, r, http.StatusConflict, "job_not_cancelable", "The job is not in a cancelable state", false, nil)
		return
	}
	job, _ := a.db.GetJob(id)
	respondV2JSON(w, http.StatusAccepted, job)
}

func (a *API) retryJobV2(w http.ResponseWriter, r *http.Request) {
	original, err := a.db.GetJob(chi.URLParam(r, "id"))
	if err != nil {
		respondV2Error(w, r, http.StatusNotFound, "job_not_found", "Operation job was not found", false, nil)
		return
	}
	if original.Status != db.JobStatusFailed && original.Status != db.JobStatusInterrupted && original.Status != db.JobStatusCanceled {
		respondV2Error(w, r, http.StatusConflict, "job_not_retryable", "Only failed, interrupted, or canceled jobs can be retried", false, nil)
		return
	}
	retry := db.Job{
		ID: uuid.NewString(), Type: original.Type, Status: db.JobStatusQueued,
		Parameters: original.Parameters, Priority: original.Priority, Message: "Queued as retry of " + original.ID,
	}
	if err := a.db.CreateJob(retry); err != nil {
		respondV2Error(w, r, http.StatusInternalServerError, "job_retry_failed", "Unable to create retry job", true, nil)
		return
	}
	a.wakeJobScheduler()
	created, _ := a.db.GetJob(retry.ID)
	respondV2JSON(w, http.StatusAccepted, created)
}

// wakeJobScheduler starts the bounded worker pool on first use and signals it
// that claimable work may exist. It is called from concurrent HTTP handlers, so
// channel creation and pool startup are serialized under jobSchedulerMu and the
// channel is passed to workers rather than read from the struct.
func (a *API) wakeJobScheduler() {
	workers := schedulerWorkerCount(runtime.NumCPU())
	a.jobSchedulerMu.Lock()
	if a.jobWake == nil {
		// One slot per worker so a single wake can fan out to the whole pool.
		a.jobWake = make(chan struct{}, workers)
	}
	wake := a.jobWake
	if !a.jobSchedulerOn {
		a.jobSchedulerOn = true
		for i := 0; i < workers; i++ {
			go a.jobWorker(wake)
		}
		go a.jobSchedulerHeartbeat(wake)
	}
	a.jobSchedulerMu.Unlock()
	for i := 0; i < workers; i++ {
		select {
		case wake <- struct{}{}:
		default:
		}
	}
}

// jobSchedulerHeartbeat re-polls the durable queue so work is not stranded when
// a transient database error ends a drain pass between explicit wake signals.
func (a *API) jobSchedulerHeartbeat(wake chan struct{}) {
	ticker := time.NewTicker(schedulerIdlePollInterval)
	defer ticker.Stop()
	for range ticker.C {
		select {
		case wake <- struct{}{}:
		default:
		}
	}
}

func (a *API) jobWorker(wake <-chan struct{}) {
	for range wake {
		for {
			job, err := a.db.ClaimNextQueuedJob("Starting job")
			if errors.Is(err, sql.ErrNoRows) {
				break
			}
			if err != nil {
				break
			}
			a.runClaimedJob(job)
		}
	}
}

func (a *API) runClaimedJob(job db.Job) {
	id := job.ID

	switch job.Type {
	case "full_scan":
		a.runFullScanJob(id)
	case "quick_scan":
		a.runQuickScanJob(id)
	case "refresh_genre_stats":
		if err := a.db.UpdateGenreStats(); err != nil {
			_ = a.db.FailJob(id, "genre_stats_failed", err.Error())
			return
		}
		_ = a.db.CompleteJob(id, map[string]string{"status": "refreshed"}, "Genre statistics refreshed")
	default:
		_ = a.db.FailJob(id, "unsupported_job_type", "Unsupported job type")
	}
}

func (a *API) runFullScanJob(id string) {
	updatesDone := make(chan struct{})
	subscription := a.scanner.Subscribe()
	go func() {
		defer close(updatesDone)
		for event := range subscription {
			if event.Type == "scan_progress" || event.Type == "scan_started" {
				_ = a.db.UpdateJobProgress(id, 0, 0, event.Message)
			}
		}
	}()

	result, err := a.scanner.ScanAll()
	a.scanner.Unsubscribe(subscription)
	<-updatesDone
	if err != nil {
		_ = a.db.FailJob(id, "scan_failed", err.Error())
		return
	}
	if a.jobCancellationRequested(id) {
		_ = a.db.CancelJob(id, "Cancellation completed after the current scan operation")
		return
	}
	_ = a.db.CompleteJob(id, result, fmt.Sprintf("Scan complete: %d new, %d updated, %d removed", result.NewSongs, result.UpdatedSongs, result.RemovedSongs))
}

func (a *API) runQuickScanJob(id string) {
	if !a.scanner.TryBeginScan() {
		_ = a.db.FailJob(id, "scan_in_progress", "Another scan is already running")
		return
	}
	defer a.scanner.EndScan()
	_ = a.db.UpdateJobProgress(id, 0, 0, "Detecting filesystem changes")
	quick, err := a.scanner.QuickStartup()
	if err != nil {
		_ = a.db.FailJob(id, "quick_scan_failed", err.Error())
		return
	}
	deleted, deleteErr := a.scanner.DetectDeletedFiles()
	if deleteErr == nil {
		quick.ChangedFiles = append(quick.ChangedFiles, deleted...)
	}
	_ = a.db.UpdateJobProgress(id, 0, int64(len(quick.ChangedFiles)), "Processing changed files")
	result, err := a.scanner.ProcessChanges(quick.ChangedFiles)
	if err != nil {
		_ = a.db.FailJob(id, "quick_scan_failed", err.Error())
		return
	}
	if a.jobCancellationRequested(id) {
		_ = a.db.CancelJob(id, "Cancellation completed after the current quick-scan batch")
		return
	}
	_ = a.db.CompleteJob(id, map[string]any{"detection": quick, "result": result}, fmt.Sprintf("Quick scan complete: %d changes", len(quick.ChangedFiles)))
}

func (a *API) jobCancellationRequested(id string) bool {
	job, err := a.db.GetJob(id)
	return err == nil && job.Status == db.JobStatusCanceling
}

func (a *API) jobEventsV2(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		respondV2Error(w, r, http.StatusInternalServerError, "streaming_unsupported", "Streaming is not supported", false, nil)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("X-Accel-Buffering", "no")

	ticker := time.NewTicker(time.Second)
	heartbeat := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	defer heartbeat.Stop()
	lastPayload := ""
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			jobs, err := a.db.ListJobs(100, "")
			if err != nil {
				continue
			}
			payload, err := json.Marshal(map[string]any{"jobs": jobs})
			if err != nil || string(payload) == lastPayload {
				continue
			}
			lastPayload = string(payload)
			if _, err := fmt.Fprintf(w, "event: jobs\ndata: %s\n\n", payload); err != nil {
				return
			}
			flusher.Flush()
		case <-heartbeat.C:
			if _, err := fmt.Fprint(w, ": heartbeat\n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

// Keep scanner imported in generated API documentation builds where platform
// tags may eliminate a concrete result reference.
var _ = scanner.ScanResult{}
