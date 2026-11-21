// Coordinates manual and scheduled Plex catalog sync for the selected source.
package api

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/db"
	"github.com/ajbergh/viib-mediahub/internal/logger"
)

const plexAutoSyncSetting = "plex_auto_sync"
const plexAutoSyncInterval = time.Hour

var (
	errPlexSourceRequired  = errors.New("Plex server required")
	errPlexLibraryRequired = errors.New("Plex library required")
	errPlexSyncRunning     = errors.New("Plex synchronization is already running")
)

type plexSyncRun struct {
	id     string
	source *db.PlexSource
	ctx    context.Context
	cancel context.CancelFunc
	tracks atomic.Int64
	total  atomic.Int64
}

func (a *API) beginPlexSync(ctx context.Context, automatic bool) (*plexSyncRun, error) {
	a.plexSyncMu.Lock()
	defer a.plexSyncMu.Unlock()
	if a.plexSyncClosed {
		return nil, context.Canceled
	}
	if a.plexSync != nil {
		return nil, errPlexSyncRunning
	}
	source, err := a.db.GetActivePlexSource()
	if err != nil {
		return nil, err
	}
	if source == nil {
		return nil, errPlexSourceRequired
	}
	if source.LibraryID == "" {
		return nil, errPlexLibraryRequired
	}
	now := time.Now()
	if automatic && (now.Sub(time.UnixMilli(source.LastSyncAt)) < plexAutoSyncInterval ||
		(a.plexLastAttemptSource == source.ID && now.Sub(a.plexLastAttemptAt) < plexAutoSyncInterval)) {
		return nil, nil
	}
	runID := rand.Text()
	if err := a.db.StartPlexSync(ctx, *source, runID); err != nil {
		return nil, err
	}
	runCtx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	run := &plexSyncRun{id: runID, source: source, ctx: runCtx, cancel: cancel}
	a.plexSync = run
	a.plexLastAttemptSource = source.ID
	a.plexLastAttemptAt = now
	a.plexSyncWorkers.Add(1)
	return run, nil
}

func (a *API) plexSyncSource() (*db.PlexSource, error) {
	a.plexSyncMu.Lock()
	defer a.plexSyncMu.Unlock()
	source, err := a.db.GetActivePlexSource()
	if source != nil && source.LastSyncStatus == "running" && a.plexSync == nil {
		source.LastSyncStatus = "interrupted"
		source.LastSyncError = "The previous Plex sync was interrupted. Synchronize to try again."
	}
	if source != nil && a.plexSync != nil && source.ID == a.plexSync.source.ID && source.LastSyncStatus == "running" {
		source.SyncTracks = a.plexSync.tracks.Load()
		source.SyncTotal = a.plexSync.total.Load()
	}
	return source, err
}

// Source and library changes invalidate the old worker's ownership before it
// can apply a snapshot or publish an error for the new selection.
func (a *API) cancelPlexSyncLocked() {
	if a.plexSync != nil {
		a.plexSync.cancel()
		a.plexSync = nil
	}
}

func (a *API) cancelPlexSync(w http.ResponseWriter, r *http.Request) {
	a.plexSyncMu.Lock()
	defer a.plexSyncMu.Unlock()
	if run := a.plexSync; run != nil {
		if err := a.db.FailPlexSync(run.source.ID, run.id, "interrupted", "Plex synchronization canceled.", nil); err != nil {
			logger.API("Failed to cancel Plex sync: %v", err)
			respondPlexAPIMessage(w, r, http.StatusServiceUnavailable, "plex_sync_cancel_failed", "ViiB could not cancel the Plex sync. Try again shortly.", true)
			return
		}
		a.cancelPlexSyncLocked()
	}
	respondV2JSON(w, http.StatusOK, map[string]string{"status": "stopped"})
}

func (a *API) finishPlexSync(run *plexSyncRun) {
	run.cancel()
	a.plexSyncMu.Lock()
	if a.plexSync == run {
		a.plexSync = nil
	}
	a.plexSyncMu.Unlock()
	a.plexSyncWorkers.Done()
}

func (a *API) completePlexSync(run *plexSyncRun, catalog []db.PlexCatalogTrack) (int, int, int, error) {
	a.plexSyncMu.Lock()
	defer a.plexSyncMu.Unlock()
	if a.plexSync != run {
		return 0, 0, 0, context.Canceled
	}
	added, updated, removed, err := a.db.CompletePlexSync(run.ctx, run.source.ID, run.source.LibraryID, run.id, catalog)
	if err == nil {
		a.plexSync = nil
	}
	return added, updated, removed, err
}

func (a *API) plexAutoSyncEnabled() bool {
	value, err := a.db.GetSetting(plexAutoSyncSetting)
	return err == nil && (value == "" || value == "true")
}

func (a *API) setPlexSyncSchedule(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Enabled *bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil || request.Enabled == nil {
		respondPlexAPIMessage(w, r, http.StatusBadRequest, "plex_invalid_schedule", "Choose whether automatic Plex synchronization is enabled.", false)
		return
	}
	if err := a.db.SetSetting(plexAutoSyncSetting, strconv.FormatBool(*request.Enabled)); err != nil {
		respondPlexAPIMessage(w, r, http.StatusInternalServerError, "plex_schedule_save_failed", "ViiB could not save the Plex sync schedule.", true)
		return
	}
	respondV2JSON(w, http.StatusOK, map[string]bool{"enabled": *request.Enabled})
}

func (a *API) startPlexSyncScheduler() {
	ctx, cancel := context.WithCancel(context.Background())
	a.plexScheduleCancel = cancel
	a.plexSyncWorkers.Add(1)
	go func() {
		defer a.plexSyncWorkers.Done()
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if !a.plexAutoSyncEnabled() {
					continue
				}
				run, err := a.beginPlexSync(ctx, true)
				if run != nil {
					go a.runPlexSync(run)
				} else if err != nil && !errors.Is(err, errPlexSyncRunning) &&
					!errors.Is(err, errPlexLibraryRequired) && !errors.Is(err, errPlexSourceRequired) && ctx.Err() == nil {
					logger.API("Could not start automatic Plex sync: %v", err)
				}
			}
		}
	}()
}

func (a *API) closePlexSync() {
	a.plexSyncMu.Lock()
	a.plexSyncClosed = true
	a.cancelPlexSyncLocked()
	if a.plexScheduleCancel != nil {
		a.plexScheduleCancel()
	}
	a.plexSyncMu.Unlock()
	a.plexSyncWorkers.Wait()
}
