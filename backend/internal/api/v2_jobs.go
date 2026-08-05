package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/db"
	"github.com/ajbergh/viib-mediahub/internal/scanner"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

const maxJobRequestBytes = 64 * 1024

type createJobRequest struct {
	Type       string          `json:"type"`
	Parameters json.RawMessage `json:"parameters,omitempty"`
}

func (a *API) V2JobRoutes() chi.Router {
	r := chi.NewRouter()
	if err := a.db.EnsureJobSchema(); err != nil {
		// Individual handlers will return a structured database error.
	}
	r.Get("/", a.listJobsV2)
	r.Post("/", a.createJobV2)
	r.Get("/events", a.jobEventsV2)
	r.Get("/{id}", a.getJobV2)
	r.Post("/{id}/cancel", a.cancelJobV2)
	r.Post("/{id}/retry", a.retryJobV2)
	return r
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

	job := db.Job{ID: uuid.NewString(), Type: request.Type, Status: db.JobStatusQueued, Parameters: request.Parameters, Message: "Queued"}
	if err := a.db.CreateJob(job); err != nil {
		respondV2Error(w, r, http.StatusInternalServerError, "job_create_failed", "Unable to create the operation job", true, nil)
		return
	}
	go a.runJob(job.ID)
	created, err := a.db.GetJob(job.ID)
	if err != nil { created = job }
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
		Parameters: original.Parameters, Message: "Queued as retry of " + original.ID,
	}
	if err := a.db.CreateJob(retry); err != nil {
		respondV2Error(w, r, http.StatusInternalServerError, "job_retry_failed", "Unable to create retry job", true, nil)
		return
	}
	go a.runJob(retry.ID)
	created, _ := a.db.GetJob(retry.ID)
	respondV2JSON(w, http.StatusAccepted, created)
}

func (a *API) runJob(id string) {
	job, err := a.db.GetJob(id)
	if err != nil { return }
	if err := a.db.StartJob(id, "Starting "+job.Type); err != nil { return }

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
	if deleteErr == nil { quick.ChangedFiles = append(quick.ChangedFiles, deleted...) }
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
		case <-r.Context().Done(): return
		case <-ticker.C:
			jobs, err := a.db.ListJobs(100, "")
			if err != nil { continue }
			payload, err := json.Marshal(map[string]any{"jobs": jobs})
			if err != nil || string(payload) == lastPayload { continue }
			lastPayload = string(payload)
			if _, err := fmt.Fprintf(w, "event: jobs\ndata: %s\n\n", payload); err != nil { return }
			flusher.Flush()
		case <-heartbeat.C:
			if _, err := fmt.Fprint(w, ": heartbeat\n\n"); err != nil { return }
			flusher.Flush()
		}
	}
}

// Keep scanner imported in generated API documentation builds where platform
// tags may eliminate a concrete result reference.
var _ = scanner.ScanResult{}
