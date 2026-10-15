// Defines semantic functionality for package api.

package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/db"
	"github.com/ajbergh/viib-mediahub/internal/logger"
	"github.com/ajbergh/viib-mediahub/internal/semantic"
)

const semanticProviderTestTimeout = 30 * time.Second

// Kept at the application boundary so API unit tests need no model downloads.
var managedEmbeddingProvider = semantic.NewLocalEmbeddingProvider

type semanticStatusResponse struct {
	State             string                  `json:"state"`
	Reason            string                  `json:"reason,omitempty"`
	Provider          string                  `json:"provider,omitempty"`
	Model             string                  `json:"model,omitempty"`
	Dimensions        int                     `json:"dimensions,omitempty"`
	DocumentsByStatus map[string]int          `json:"documentsByStatus"`
	Indexes           []db.SemanticIndexState `json:"indexes"`
	Progress          float64                 `json:"progress"`
	LastError         string                  `json:"lastError,omitempty"`
}

type semanticRebuildRequest struct {
	Scope string `json:"scope"`
}

// initSemanticService owns initialization/download lifetime as well as retries.
func (a *API) initSemanticService() {
	a.semanticMu.Lock()
	if a.semanticClosed || a.semanticInitializing || a.semanticService != nil {
		a.semanticMu.Unlock()
		return
	}
	if a.semanticContext == nil {
		a.semanticContext, a.semanticCancel = context.WithCancel(context.Background())
	}
	if a.semanticWake == nil {
		a.semanticWake = make(chan struct{}, 1)
	}
	wake := a.semanticWake
	ctx := a.semanticContext
	generation := a.semanticGeneration
	a.semanticInitializing = true
	a.semanticWorkers.Add(1)
	a.semanticMu.Unlock()
	defer a.semanticWorkers.Done()
	defer func() { a.semanticMu.Lock(); a.semanticInitializing = false; a.semanticMu.Unlock() }()
	resolution := semantic.ManagedEmbeddingResolution()
	for {
		preparing := resolution
		preparing.Status = "preparing"
		preparing.Reason = "Preparing music matching on this device. Initial setup downloads a small model."
		a.storeSemanticUnavailable(generation, preparing, "")
		provider, err := managedEmbeddingProvider(ctx, a.dataDir, nil, nil)
		if err == nil {
			var service *semantic.Service
			service, err = semantic.NewService(a.db, provider)
			if err == nil {
				err = service.Start(ctx)
			}
			if err == nil {
				a.semanticMu.Lock()
				if a.semanticClosed || a.semanticGeneration != generation {
					a.semanticMu.Unlock()
					_ = service.Close()
					return
				}
				a.semanticService = service
				a.semanticState = resolution
				a.semanticError = ""
				a.semanticMu.Unlock()
				return
			}
			if service != nil {
				_ = service.Close()
			} else {
				_ = provider.Close()
			}
		}
		if ctx.Err() != nil {
			return
		}
		failed := resolution
		failed.Status = "error"
		failed.Reason = "Smart Playlist preparation will retry automatically. Standard matching remains available."
		a.storeSemanticUnavailable(generation, failed, err.Error())
		timer := time.NewTimer(time.Minute)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-wake:
			timer.Stop()
		case <-timer.C:
		}
	}
}

func (a *API) wakeSemanticPreparation() {
	a.semanticMu.RLock()
	wake := a.semanticWake
	a.semanticMu.RUnlock()
	if wake != nil {
		select {
		case wake <- struct{}{}:
		default:
		}
	}
	go a.initSemanticService()
}

func (a *API) storeSemanticUnavailable(generation uint64, resolution semantic.EmbeddingResolution, errMessage string) {
	a.semanticMu.Lock()
	defer a.semanticMu.Unlock()
	if a.semanticClosed || a.semanticGeneration != generation {
		return
	}
	a.semanticService = nil
	a.semanticState = resolution
	a.semanticError = errMessage
}

// The legacy settings endpoint is read-only so old clients cannot activate
// cloud embeddings. Legacy rows are ignored; migration happens by vector identity.
func (a *API) getSemanticSettings(w http.ResponseWriter, _ *http.Request) {
	resolution, _, _ := a.semanticSnapshot()
	settings := semantic.ManagedEmbeddingResolution().Settings
	respondJSON(w, map[string]any{"provider": settings.Provider, "model": settings.Model, "dimensions": settings.Dimensions, "managed": true, "status": resolution.Status})
}

func (a *API) updateSemanticSettings(w http.ResponseWriter, _ *http.Request) {
	respondError(w, http.StatusMethodNotAllowed, "Smart Playlist indexing is managed automatically on this device")
}

func (a *API) getSemanticStatus(w http.ResponseWriter, r *http.Request) {
	resolution, service, errMessage := a.semanticSnapshot()
	response := semanticStatusResponse{
		State:             resolution.Status,
		Reason:            resolution.Reason,
		Provider:          resolution.Settings.Provider,
		Model:             resolution.Settings.Model,
		Dimensions:        resolution.Settings.Dimensions,
		DocumentsByStatus: map[string]int{},
		LastError:         errMessage,
	}
	if response.State == "" {
		response.State = "initializing"
	}
	if service != nil {
		status := service.Status()
		response.State = status.State
		response.LastError = status.LastError
	}
	stats, err := a.db.GetSemanticIndexStats(r.Context())
	if err != nil {
		response.LastError = err.Error()
		if service == nil {
			response.State = "error"
		}
		respondJSON(w, response)
		return
	}
	response.DocumentsByStatus = stats.DocumentsByStatus
	response.Indexes = stats.State
	for _, state := range stats.State {
		if state.Dimensions > response.Dimensions {
			response.Dimensions = state.Dimensions
		}
		if response.Provider == "" && state.EmbeddingProvider != "" {
			response.Provider = state.EmbeddingProvider
			response.Model = state.EmbeddingModel
		}
	}
	total := 0
	for _, count := range stats.DocumentsByStatus {
		total += count
	}
	if total > 0 && service != nil {
		response.Progress = float64(stats.DocumentsByStatus["ready"]) / float64(total)
	}
	respondJSON(w, response)
}

func (a *API) rebuildSemanticIndex(w http.ResponseWriter, r *http.Request) {
	var request semanticRebuildRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		respondError(w, http.StatusBadRequest, "invalid semantic rebuild request")
		return
	}
	service := a.currentSemanticService()
	if service == nil {
		a.wakeSemanticPreparation()
		respondJSON(w, map[string]string{"status": "accepted"})
		return
	}
	scope := strings.ToLower(strings.TrimSpace(request.Scope))
	switch scope {
	case "reindex":
		go func() {
			if err := service.Reindex(context.Background()); err != nil {
				logger.API("semantic reindex failed: %v", err)
			}
		}()
	case "reload":
		go func() {
			if err := service.LoadReadyIndexes(context.Background()); err != nil {
				logger.API("semantic index reload failed: %v", err)
			}
		}()
	default:
		respondError(w, http.StatusBadRequest, "semantic rebuild scope must be reindex or reload")
		return
	}
	respondJSON(w, map[string]string{"status": "accepted", "scope": scope})
}

func (a *API) retrySemanticErrors(w http.ResponseWriter, _ *http.Request) {
	service := a.currentSemanticService()
	if service == nil {
		a.wakeSemanticPreparation()
		respondJSON(w, map[string]string{"status": "accepted"})
		return
	}
	go func() {
		if _, err := service.RetryErrors(context.Background()); err != nil {
			logger.API("semantic error retry failed: %v", err)
		}
	}()
	respondJSON(w, map[string]string{"status": "accepted"})
}

func (a *API) testSemanticEmbeddingProvider(w http.ResponseWriter, r *http.Request) {
	service := a.currentSemanticService()
	if service == nil {
		respondError(w, http.StatusConflict, "semantic indexing is not configured")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), semanticProviderTestTimeout)
	defer cancel()
	identity, err := service.TestProvider(ctx)
	if err != nil {
		respondJSON(w, map[string]any{"success": false, "message": err.Error()})
		return
	}
	respondJSON(w, map[string]any{
		"success":    true,
		"provider":   identity.Provider,
		"model":      identity.Model,
		"dimensions": identity.Dimensions,
	})
}

func (a *API) semanticSnapshot() (semantic.EmbeddingResolution, *semantic.Service, string) {
	a.semanticMu.RLock()
	defer a.semanticMu.RUnlock()
	return a.semanticState, a.semanticService, a.semanticError
}

func (a *API) currentSemanticService() *semantic.Service {
	a.semanticMu.RLock()
	defer a.semanticMu.RUnlock()
	return a.semanticService
}
