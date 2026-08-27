// Package api provides REST API handlers for ViiB MediaHub.
//
// This file contains handlers for LLM (Large Language Model) configuration endpoints.
// These endpoints allow the frontend to configure which AI provider to use for
// features like AI DJ smart playlist generation.
//
// Endpoints:
//   - GET  /api/llm/settings   - Get current LLM settings
//   - PUT  /api/llm/settings   - Update LLM settings
//   - GET  /api/llm/providers  - List available providers and models
//   - POST /api/llm/test       - Test connection to current provider
package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/ajbergh/viib-mediahub/internal/llm"
	"github.com/ajbergh/viib-mediahub/internal/logger"
	"github.com/ajbergh/viib-mediahub/internal/semantic"
)

// LLMSettingsResponse is the API response for LLM settings.
// It includes the current provider configuration and available options.
type LLMSettingsResponse struct {
	// Current settings
	Provider string `json:"provider"`
	Model    string `json:"model"`
	APIKey   string `json:"apiKey,omitempty"` // Masked for security
	BaseURL  string `json:"baseURL,omitempty"`

	// Available options
	Providers []llm.ProviderInfo         `json:"providers"`
	Models    map[string][]llm.ModelInfo `json:"models"`
}

// LLMSettingsRequest is the request body for updating LLM settings.
type LLMSettingsRequest struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
	APIKey   string `json:"apiKey,omitempty"`
	BaseURL  string `json:"baseURL,omitempty"`
}

// LLMTestResponse is the response from testing the LLM connection.
type LLMTestResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// getLLMSettings returns the current LLM configuration.
// GET /api/llm/settings
func (a *API) getLLMSettings(w http.ResponseWriter, r *http.Request) {
	// Get current settings from database
	provider, _ := a.db.GetSetting("llm_provider")
	model, _ := a.db.GetSetting("llm_model")
	apiKey, _ := a.db.GetSetting("llm_api_key")
	baseURL, _ := a.db.GetSetting("llm_base_url")

	// Apply defaults if not set
	if provider == "" {
		provider = llm.ProviderOllama
	}
	if model == "" {
		// Get default model for the provider
		model = getDefaultModelForProvider(provider)
	}
	if baseURL == "" && provider == llm.ProviderOllama {
		baseURL = "http://localhost:11434"
	}

	// Mask API key for security (show only last 4 chars)
	maskedKey := ""
	if len(apiKey) > 4 {
		maskedKey = "****" + apiKey[len(apiKey)-4:]
	} else if apiKey != "" {
		maskedKey = "****"
	}

	models := llm.GetAvailableModels()
	if provider == llm.ProviderOpenRouter && apiKey != "" {
		openRouterModels, err := llm.GetOpenRouterModels(r.Context(), apiKey)
		if err != nil {
			// Keep the settings screen available and fall back to Auto Router if
			// the catalog cannot be refreshed (for example, while offline).
			logger.API("Unable to retrieve OpenRouter models: %v", err)
		} else {
			models[llm.ProviderOpenRouter] = openRouterModels
		}
	}

	response := LLMSettingsResponse{
		Provider:  provider,
		Model:     model,
		APIKey:    maskedKey,
		BaseURL:   baseURL,
		Providers: llm.GetAvailableProviders(),
		Models:    models,
	}

	respondJSON(w, response)
}

// updateLLMSettings updates the LLM configuration.
// PUT /api/llm/settings
func (a *API) updateLLMSettings(w http.ResponseWriter, r *http.Request) {
	var req LLMSettingsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Validate provider
	validProvider := false
	for _, p := range llm.GetAvailableProviders() {
		if p.ID == req.Provider {
			validProvider = true
			break
		}
	}
	if !validProvider {
		respondError(w, http.StatusBadRequest, "Invalid provider: "+req.Provider)
		return
	}

	// Save settings
	if err := a.db.SetSetting("llm_provider", req.Provider); err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to save provider: "+err.Error())
		return
	}

	if req.Model != "" {
		if err := a.db.SetSetting("llm_model", req.Model); err != nil {
			respondError(w, http.StatusInternalServerError, "Failed to save model: "+err.Error())
			return
		}
	}

	// Only update API key if provided (not masked value)
	if req.APIKey != "" && !ismaskedAPIKey(req.APIKey) {
		if err := a.db.SetSetting("llm_api_key", req.APIKey); err != nil {
			respondError(w, http.StatusInternalServerError, "Failed to save API key: "+err.Error())
			return
		}
		// Also sync to gemini_api_key if provider is Gemini for enrichment compatibility
		if req.Provider == "gemini" {
			if err := a.db.SetSetting("gemini_api_key", req.APIKey); err != nil {
				// Log but don't fail - enrichment key sync is best-effort
				logger.API("Warning: Failed to sync gemini_api_key: %v", err)
			}
		}
	}

	// Custom endpoints are only supported for Ollama. Clearing the persisted
	// value for cloud providers prevents a stale localhost URL from overriding
	// OpenRouter (or another cloud provider) after a provider switch.
	baseURL := req.BaseURL
	if req.Provider != llm.ProviderOllama {
		baseURL = ""
	}
	if err := a.db.SetSetting("llm_base_url", baseURL); err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to save base URL: "+err.Error())
		return
	}

	semanticChanged, err := a.initializeSemanticProviderFromLLM(req.Provider, baseURL)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to initialize semantic provider: "+err.Error())
		return
	}
	if semanticChanged {
		go a.restartSemanticService()
	}

	respondJSON(w, map[string]string{"status": "ok"})
}

// initializeSemanticProviderFromLLM gives a new installation a usable semantic
// configuration that matches its selected AI provider. It only replaces an
// unset/auto semantic provider, so a later explicit semantic choice remains
// independent from chat configuration.
func (a *API) initializeSemanticProviderFromLLM(provider, ollamaBaseURL string) (bool, error) {
	currentProvider, err := a.db.GetSetting(semantic.SemanticEmbeddingProviderSetting)
	if err != nil {
		return false, fmt.Errorf("read semantic provider: %w", err)
	}
	currentProvider = strings.ToLower(strings.TrimSpace(currentProvider))
	if currentProvider != "" && currentProvider != semantic.EmbeddingProviderAuto {
		return false, nil
	}

	model := ""
	dimensions := ""
	baseURL := ""
	switch provider {
	case llm.ProviderOllama:
		model = semantic.DefaultOllamaEmbeddingModel
		baseURL = strings.TrimSpace(ollamaBaseURL)
		if baseURL == "" {
			baseURL = llm.DefaultOllamaEndpoint
		}
	case llm.ProviderOpenAI:
		model = semantic.DefaultOpenAIEmbeddingModel
		dimensions = fmt.Sprint(semantic.DefaultOpenAIEmbeddingDimensions)
	case llm.ProviderOpenRouter:
		model = semantic.DefaultOpenRouterEmbeddingModel
		dimensions = fmt.Sprint(semantic.DefaultOpenRouterDimensions)
	case llm.ProviderGemini:
		model = semantic.DefaultGeminiEmbeddingModel
		dimensions = fmt.Sprint(semantic.DefaultGeminiEmbeddingDimensions)
	default:
		return false, nil
	}

	if err := a.db.SetSettingsBatch(map[string]string{
		semantic.SemanticEmbeddingProviderSetting:          provider,
		semantic.SemanticEmbeddingModelSetting:             model,
		semantic.SemanticEmbeddingDimensionsSetting:        dimensions,
		semantic.SemanticEmbeddingBaseURLSetting:           baseURL,
		semantic.SemanticEmbeddingAPIKeySetting:            "",
		semantic.SemanticEmbeddingCloudConfirmationSetting: "",
	}); err != nil {
		return false, fmt.Errorf("save matching semantic provider: %w", err)
	}
	return true, nil
}

// ismaskedAPIKey checks if the API key is a masked value (starts with ****)
func ismaskedAPIKey(key string) bool {
	return len(key) >= 4 && key[:4] == "****"
}

// getLLMProviders returns the list of available LLM providers and models.
// GET /api/llm/providers
func (a *API) getLLMProviders(w http.ResponseWriter, r *http.Request) {
	response := map[string]interface{}{
		"providers": llm.GetAvailableProviders(),
		"models":    llm.GetAvailableModels(),
	}
	respondJSON(w, response)
}

// testLLMConnection tests the connection to the configured LLM provider.
// POST /api/llm/test
func (a *API) testLLMConnection(w http.ResponseWriter, r *http.Request) {
	// Get current settings
	provider, _ := a.db.GetSetting("llm_provider")
	model, _ := a.db.GetSetting("llm_model")
	apiKey, _ := a.db.GetSetting("llm_api_key")
	baseURL, _ := a.db.GetSetting("llm_base_url")

	// Apply defaults
	if provider == "" {
		provider = llm.ProviderOllama
	}
	if model == "" {
		// Get default model for the provider
		model = getDefaultModelForProvider(provider)
	}
	if baseURL == "" && provider == llm.ProviderOllama {
		baseURL = "http://localhost:11434"
	}

	// Create provider instance
	settings := llm.Settings{
		Provider:   provider,
		Model:      model,
		APIKey:     apiKey,
		BaseURL:    baseURL,
		MaxRetries: 1,
	}

	llmProvider, err := llm.NewProvider(settings)
	if err != nil {
		respondJSON(w, LLMTestResponse{
			Success: false,
			Message: "Failed to create provider: " + err.Error(),
		})
		return
	}
	defer llmProvider.Close()

	// Test connection
	if err := llmProvider.TestConnection(r.Context()); err != nil {
		respondJSON(w, LLMTestResponse{
			Success: false,
			Message: "Connection failed: " + err.Error(),
		})
		return
	}

	respondJSON(w, LLMTestResponse{
		Success: true,
		Message: "Successfully connected to " + provider + " using model " + model,
	})
}

// getDefaultModelForProvider returns the default model for a given provider.
// For Ollama (freeform input), returns the default Ollama model.
// For other providers, returns the first model in their predefined list.
func getDefaultModelForProvider(provider string) string {
	// Check provider info for default model
	for _, p := range llm.GetAvailableProviders() {
		if p.ID == provider {
			return p.DefaultModel
		}
	}
	// Fallback to first model in list (for non-freeform providers)
	models := llm.GetAvailableModels()
	if providerModels, ok := models[provider]; ok && len(providerModels) > 0 {
		return providerModels[0].ID
	}
	return ""
}
