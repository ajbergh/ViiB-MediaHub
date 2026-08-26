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

// initSemanticService resolves configuration after API construction. It keeps
// startup non-blocking, and a missing provider only publishes a local status
// rather than changing the existing AI DJ behavior.
func (a *API) initSemanticService() {
	ctx, cancel := context.WithTimeout(context.Background(), semanticProviderTestTimeout)
	defer cancel()
	resolution, err := semantic.ResolveEmbeddingSettings(ctx, a.db, nil)
	if err != nil {
		a.storeSemanticUnavailable(semantic.EmbeddingResolution{Status: "error"}, err.Error())
		return
	}
	if !resolution.Ready() {
		a.storeSemanticUnavailable(resolution, "")
		return
	}
	provider, err := semantic.NewConfiguredEmbeddingProvider(resolution.Settings, nil)
	if err != nil {
		resolution.Status = "needs_configuration"
		resolution.Reason = err.Error()
		a.storeSemanticUnavailable(resolution, "")
		return
	}
	service, err := semantic.NewService(a.db, provider)
	if err != nil {
		_ = provider.Close()
		a.storeSemanticUnavailable(resolution, err.Error())
		return
	}
	if err := service.Start(context.Background()); err != nil {
		_ = service.Close()
		a.storeSemanticUnavailable(resolution, err.Error())
		return
	}
	a.semanticMu.Lock()
	if a.semanticClosed {
		a.semanticMu.Unlock()
		_ = service.Close()
		return
	}
	a.semanticService = service
	a.semanticState = resolution
	a.semanticError = ""
	a.semanticMu.Unlock()
}

func (a *API) storeSemanticUnavailable(resolution semantic.EmbeddingResolution, errMessage string) {
	a.semanticMu.Lock()
	defer a.semanticMu.Unlock()
	if a.semanticClosed {
		return
	}
	a.semanticService = nil
	a.semanticState = resolution
	a.semanticError = errMessage
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
	if total > 0 {
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
		respondError(w, http.StatusConflict, "semantic indexing is not configured")
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
		respondError(w, http.StatusConflict, "semantic indexing is not configured")
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
