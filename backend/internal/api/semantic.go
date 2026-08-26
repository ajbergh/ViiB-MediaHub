package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
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

type semanticSettingsResponse struct {
	Provider         string `json:"provider"`
	Model            string `json:"model"`
	Dimensions       int    `json:"dimensions"`
	BaseURL          string `json:"baseURL"`
	APIKeyConfigured bool   `json:"apiKeyConfigured"`
	Status           string `json:"status"`
	Reason           string `json:"reason,omitempty"`
}

type semanticSettingsRequest struct {
	Provider   string  `json:"provider"`
	Model      string  `json:"model"`
	Dimensions *int    `json:"dimensions"`
	BaseURL    string  `json:"baseURL"`
	APIKey     *string `json:"apiKey"`
}

// initSemanticService resolves configuration after API construction. It keeps
// startup non-blocking, and a missing provider only publishes a local status
// rather than changing the existing AI DJ behavior.
func (a *API) initSemanticService() {
	a.semanticMu.RLock()
	generation := a.semanticGeneration
	a.semanticMu.RUnlock()
	a.initSemanticServiceGeneration(generation)
}

func (a *API) initSemanticServiceGeneration(generation uint64) {
	ctx, cancel := context.WithTimeout(context.Background(), semanticProviderTestTimeout)
	defer cancel()
	resolution, err := semantic.ResolveEmbeddingSettings(ctx, a.db, nil)
	if err != nil {
		a.storeSemanticUnavailable(generation, semantic.EmbeddingResolution{Status: "error"}, err.Error())
		return
	}
	if !resolution.Ready() {
		a.storeSemanticUnavailable(generation, resolution, "")
		return
	}
	provider, err := semantic.NewConfiguredEmbeddingProvider(resolution.Settings, nil)
	if err != nil {
		resolution.Status = "needs_configuration"
		resolution.Reason = err.Error()
		a.storeSemanticUnavailable(generation, resolution, "")
		return
	}
	service, err := semantic.NewService(a.db, provider)
	if err != nil {
		_ = provider.Close()
		a.storeSemanticUnavailable(generation, resolution, err.Error())
		return
	}
	if err := service.Start(context.Background()); err != nil {
		_ = service.Close()
		a.storeSemanticUnavailable(generation, resolution, err.Error())
		return
	}
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

// restartSemanticService safely retires a previous provider before resolving
// the saved configuration. The generation prevents a slow Ollama probe from
// installing a stale service after a later settings update.
func (a *API) restartSemanticService() {
	a.semanticMu.Lock()
	if a.semanticClosed {
		a.semanticMu.Unlock()
		return
	}
	a.semanticGeneration++
	generation := a.semanticGeneration
	previous := a.semanticService
	a.semanticService = nil
	a.semanticState = semantic.EmbeddingResolution{Status: "initializing"}
	a.semanticError = ""
	a.semanticMu.Unlock()

	if previous != nil {
		if err := previous.Close(); err != nil {
			logger.API("close previous semantic service: %v", err)
		}
	}
	a.initSemanticServiceGeneration(generation)
}

func (a *API) getSemanticSettings(w http.ResponseWriter, _ *http.Request) {
	settings, err := semantic.LoadEmbeddingSettings(a.db)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to load semantic settings")
		return
	}
	resolution, _, _ := a.semanticSnapshot()
	provider := settings.Provider
	if provider == "" {
		provider = semantic.EmbeddingProviderAuto
	}
	response := semanticSettingsResponse{
		Provider:         provider,
		Model:            settings.Model,
		Dimensions:       settings.Dimensions,
		BaseURL:          settings.BaseURL,
		APIKeyConfigured: settings.APIKey != "",
		Status:           resolution.Status,
		Reason:           resolution.Reason,
	}
	if response.Status == "" {
		response.Status = "initializing"
	}
	respondJSON(w, response)
}

func (a *API) updateSemanticSettings(w http.ResponseWriter, r *http.Request) {
	var request semanticSettingsRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		respondError(w, http.StatusBadRequest, "invalid semantic settings request")
		return
	}
	existing, err := semantic.LoadEmbeddingSettings(a.db)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to load semantic settings")
		return
	}
	provider := strings.ToLower(strings.TrimSpace(request.Provider))
	if provider == "" {
		provider = semantic.EmbeddingProviderAuto
	}
	switch provider {
	case semantic.EmbeddingProviderAuto, semantic.EmbeddingProviderOllama, semantic.EmbeddingProviderOpenAI, semantic.EmbeddingProviderDisabled:
	default:
		respondError(w, http.StatusBadRequest, "semantic embedding provider must be auto, ollama, openai, or disabled")
		return
	}
	dimensions := existing.Dimensions
	if request.Dimensions != nil {
		dimensions = *request.Dimensions
	}
	if dimensions < 0 {
		respondError(w, http.StatusBadRequest, "semantic embedding dimensions must not be negative")
		return
	}
	// Ollama reports its vector size. Persisting an old cloud dimension here
	// would make the next local provider incorrectly reject valid vectors.
	if provider == semantic.EmbeddingProviderAuto || provider == semantic.EmbeddingProviderOllama {
		dimensions = 0
	}
	dimensionsValue := ""
	if dimensions > 0 {
		dimensionsValue = strconv.Itoa(dimensions)
	}
	values := map[string]string{
		semantic.SemanticEmbeddingProviderSetting:   provider,
		semantic.SemanticEmbeddingModelSetting:      strings.TrimSpace(request.Model),
		semantic.SemanticEmbeddingDimensionsSetting: dimensionsValue,
		semantic.SemanticEmbeddingBaseURLSetting:    strings.TrimSpace(request.BaseURL),
	}
	if request.APIKey != nil {
		values[semantic.SemanticEmbeddingAPIKeySetting] = strings.TrimSpace(*request.APIKey)
	}
	if err := a.db.SetSettingsBatch(values); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to save semantic settings")
		return
	}
	go a.restartSemanticService()
	respondJSON(w, map[string]string{"status": "accepted"})
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
