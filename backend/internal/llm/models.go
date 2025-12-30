// Package llm - Model and provider configuration for AI DJ playlist generation
//
// This file contains the provider and model definitions for the LLM integration.
// For Ollama, users enter model names manually (e.g., "qwen3:4b", "llama3.2:8b")
// which are stored in the database. Other providers have predefined model lists.
package llm

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

// GetAvailableModels returns a map of provider to their available models.
// This is used by the Settings UI to populate model dropdowns.
// Note: Ollama is not included because users enter model names manually.
func GetAvailableModels() map[string][]ModelInfo {
	return map[string][]ModelInfo{
		// Ollama intentionally omitted - users enter model names manually
		// e.g., "qwen3:4b", "llama3.2:8b", "mistral:7b"
		ProviderGemini: {
			{ID: "gemini-3-flash-preview", Name: "Gemini 3 Flash Preview", Description: "Latest preview"},
			{ID: "gemini-2.5-flash-preview-09-2025", Name: "Gemini 2.5 Flash Preview", Description: "2.5 Flash preview"},
		},
		ProviderOpenAI: {
			{ID: "gpt-4o-mini", Name: "GPT-4o Mini", Description: "Fast, cost-effective (default)"},
			{ID: "gpt-4o", Name: "GPT-4o", Description: "Best quality, higher cost"},
			{ID: "gpt-4-turbo", Name: "GPT-4 Turbo", Description: "Previous flagship"},
			{ID: "gpt-3.5-turbo", Name: "GPT-3.5 Turbo", Description: "Legacy, cheapest"},
		},
		ProviderAnthropic: {
			{ID: "claude-3-5-haiku-latest", Name: "Claude 3.5 Haiku", Description: "Fast, cost-effective (default)"},
			{ID: "claude-3-5-sonnet-latest", Name: "Claude 3.5 Sonnet", Description: "Best quality"},
			{ID: "claude-3-opus-latest", Name: "Claude 3 Opus", Description: "Most capable"},
		},
		ProviderXAI: {
			{ID: "grok-2", Name: "Grok 2", Description: "Fast, good quality (default)"},
			{ID: "grok-3", Name: "Grok 3", Description: "Latest model"},
			{ID: "grok-3-mini", Name: "Grok 3 Mini", Description: "Smaller, faster"},
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
	}
}
