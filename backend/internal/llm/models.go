// Package llm - Model and provider configuration for AI DJ playlist generation
//
// This file contains the provider and model definitions for the LLM integration.
// For Ollama, users enter model names manually (e.g., "qwen3:4b", "llama3.2:8b")
// which are stored in the database. Other providers have predefined model lists.
package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
)

const openRouterModelsEndpoint = DefaultOpenRouterURL + "/models"

// ModelInfo describes a model available for a provider
type ModelInfo struct {
	ID          string `json:"id"`          // Model identifier (e.g., "gpt-4o-mini")
	Name        string `json:"name"`        // Display name (e.g., "GPT-4o Mini")
	Description string `json:"description"` // Brief description
}

// ProviderInfo describes an LLM provider
type ProviderInfo struct {
	ID            string `json:"id"`            // Provider identifier (e.g., "ollama")
	Name          string `json:"name"`          // Display name (e.g., "Ollama (Local)")
	RequiresKey   bool   `json:"requiresKey"`   // Whether API key is required
	DefaultModel  string `json:"defaultModel"`  // Default model for this provider
	FreeformModel bool   `json:"freeformModel"` // Whether user can type any model name (Ollama)
}

type openRouterModelsResponse struct {
	Data []openRouterModel `json:"data"`
}

type openRouterModel struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// GetOpenRouterModels retrieves every currently available text model from
// OpenRouter. The catalog is fetched instead of hard-coded so the model
// dropdown stays current as OpenRouter adds and retires models.
func GetOpenRouterModels(ctx context.Context, apiKey string) ([]ModelInfo, error) {
	return getOpenRouterModels(ctx, apiKey, openRouterModelsEndpoint, &http.Client{Timeout: DefaultTimeout})
}

func getOpenRouterModels(ctx context.Context, apiKey, endpointURL string, client *http.Client) ([]ModelInfo, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("OpenRouter API key is required to retrieve models")
	}

	const pageSize = 1000
	models := []ModelInfo{{
		ID:          DefaultOpenRouterModel,
		Name:        "Auto Router",
		Description: "Automatically selects a suitable OpenRouter model",
	}}
	seen := map[string]struct{}{DefaultOpenRouterModel: {}}

	for offset := 0; ; offset += pageSize {
		endpoint, err := url.Parse(endpointURL)
		if err != nil {
			return nil, fmt.Errorf("invalid OpenRouter models endpoint: %w", err)
		}
		query := endpoint.Query()
		query.Set("output_modalities", "text")
		query.Set("limit", fmt.Sprintf("%d", pageSize))
		query.Set("offset", fmt.Sprintf("%d", offset))
		endpoint.RawQuery = query.Encode()

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
		if err != nil {
			return nil, fmt.Errorf("create OpenRouter models request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+apiKey)

		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("request OpenRouter models: %w", err)
		}

		if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
			resp.Body.Close()
			return nil, fmt.Errorf("OpenRouter models request failed with status %s", resp.Status)
		}

		var payload openRouterModelsResponse
		decodeErr := json.NewDecoder(io.LimitReader(resp.Body, 32<<20)).Decode(&payload)
		resp.Body.Close()
		if decodeErr != nil {
			return nil, fmt.Errorf("decode OpenRouter models response: %w", decodeErr)
		}

		for _, model := range payload.Data {
			if model.ID == "" {
				continue
			}
			if _, exists := seen[model.ID]; exists {
				continue
			}
			seen[model.ID] = struct{}{}
			name := model.Name
			if name == "" {
				name = model.ID
			}
			models = append(models, ModelInfo{ID: model.ID, Name: name, Description: model.Description})
		}

		if len(payload.Data) < pageSize {
			break
		}
	}

	// Keep Auto Router at the top, then present the live catalog predictably.
	sort.Slice(models[1:], func(i, j int) bool {
		return models[1:][i].Name < models[1:][j].Name
	})

	return models, nil
}

// GetAvailableModels returns a map of provider to their available models.
// This is used by the Settings UI to populate model dropdowns.
// Note: Ollama is not included because users enter model names manually.
func GetAvailableModels() map[string][]ModelInfo {
	return map[string][]ModelInfo{
		// Ollama intentionally omitted - users enter model names manually
		// e.g., "qwen3:4b", "llama3.2:8b", "mistral:7b"
		ProviderGemini: {
			{ID: "gemini-3.6-flash", Name: "Gemini 3.6 Flash", Description: "Latest production-ready Flash model (default)"},
			{ID: "gemini-3.5-flash", Name: "Gemini 3.5 Flash", Description: "Frontier intelligence for complex agentic tasks"},
			{ID: "gemini-3.5-flash-lite", Name: "Gemini 3.5 Flash-Lite", Description: "Fast, cost-effective high-throughput model"},
			{ID: "gemini-3.1-flash-lite", Name: "Gemini 3.1 Flash-Lite", Description: "Low-latency, cost-effective multimodal model"},
			{ID: "gemini-3.1-pro-preview", Name: "Gemini 3.1 Pro Preview", Description: "Preview model for advanced reasoning and coding"},
		},
		ProviderOpenAI: {
			{ID: "gpt-5-mini", Name: "GPT-5 Mini", Description: "Fast, cost-efficient GPT-5 model (default)"},
			{ID: "gpt-5.2", Name: "GPT-5.2", Description: "Latest flagship GPT model"},
			{ID: "gpt-5.1", Name: "GPT-5.1", Description: "Advanced reasoning and agentic tasks"},
			{ID: "gpt-5", Name: "GPT-5", Description: "Previous flagship reasoning model"},
			{ID: "gpt-5-nano", Name: "GPT-5 Nano", Description: "Fastest, lowest-cost GPT-5 model"},
			{ID: "gpt-4.1", Name: "GPT-4.1", Description: "High-capability non-reasoning model"},
			{ID: "gpt-4.1-mini", Name: "GPT-4.1 Mini", Description: "Smaller, fast GPT-4.1 model"},
		},
		ProviderAnthropic: {
			{ID: "claude-sonnet-5", Name: "Claude Sonnet 5", Description: "Best balance of speed and intelligence (default)"},
			{ID: "claude-fable-5", Name: "Claude Fable 5", Description: "Highest capability for long-running agentic work"},
			{ID: "claude-opus-5", Name: "Claude Opus 5", Description: "Complex reasoning and enterprise work"},
			{ID: "claude-haiku-4-5", Name: "Claude Haiku 4.5", Description: "Fastest model for high-volume, low-latency tasks"},
		},
		ProviderXAI: {
			{ID: "grok-4.5", Name: "Grok 4.5", Description: "Latest flagship for code, chat, and knowledge work (default)"},
			{ID: "grok-latest", Name: "Grok Latest", Description: "Automatically tracks xAI's latest Grok release"},
			{ID: "grok-4.3", Name: "Grok 4.3", Description: "Current general-purpose Grok model"},
			{ID: "grok-420-reasoning", Name: "Grok 4.20 Reasoning", Description: "Extended reasoning model"},
		},
		ProviderOpenRouter: {
			{ID: DefaultOpenRouterModel, Name: "Auto Router", Description: "Automatically selects a suitable OpenRouter model (default)"},
		},
	}
}

// GetAvailableProviders returns information about all supported providers
func GetAvailableProviders() []ProviderInfo {
	return []ProviderInfo{
		{
			ID:            ProviderOllama,
			Name:          "Ollama (Local)",
			RequiresKey:   false,
			DefaultModel:  DefaultOllamaModel,
			FreeformModel: true, // User types model name manually
		},
		{
			ID:            ProviderGemini,
			Name:          "Google Gemini",
			RequiresKey:   true,
			DefaultModel:  DefaultGeminiModel,
			FreeformModel: false,
		},
		{
			ID:            ProviderOpenAI,
			Name:          "OpenAI",
			RequiresKey:   true,
			DefaultModel:  DefaultOpenAIModel,
			FreeformModel: false,
		},
		{
			ID:            ProviderAnthropic,
			Name:          "Anthropic (Claude)",
			RequiresKey:   true,
			DefaultModel:  DefaultAnthropicModel,
			FreeformModel: false,
		},
		{
			ID:            ProviderXAI,
			Name:          "X.AI (Grok)",
			RequiresKey:   true,
			DefaultModel:  DefaultXAIModel,
			FreeformModel: false,
		},
		{
			ID:            ProviderOpenRouter,
			Name:          "OpenRouter",
			RequiresKey:   true,
			DefaultModel:  DefaultOpenRouterModel,
			FreeformModel: false,
		},
	}
}
