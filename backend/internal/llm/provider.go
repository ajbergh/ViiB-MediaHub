// Package llm provides a unified LLM interface for ViiB MediaHub's AI DJ feature.
//
// This package abstracts LLM provider interactions using github.com/agentplexus/omnillm,
// enabling support for multiple providers including:
//
//   - Ollama (local-first, no API key required) - DEFAULT
//   - Google Gemini (current implementation)
//   - OpenAI (GPT-4o, GPT-4o-mini)
//   - Anthropic (Claude Opus, Sonnet, Haiku)
//   - X.AI (Grok-4, Grok-3)
//   - OpenRouter (access to its supported text models)
//   - AWS Bedrock (via external module)
//
// The package provides:
//
//   - Provider: Wraps omnillm with AI DJ-specific functionality
//   - PlaylistFilter: Structured output from natural language playlist requests
//   - Settings: Configuration for provider selection and API keys
//   - Factory functions for creating providers from stored settings
//
// Usage:
//
//	provider, err := llm.NewProvider(settings)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer provider.Close()
//
//	filter, err := provider.ParsePlaylistFilter(ctx, "upbeat jazz trios from the 90s")
//
// The package respects the existing PlaylistFilter schema from internal/gemini
// to maintain backward compatibility with the smart_playlist handler.
package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/agentplexus/omnillm"
	"github.com/ajbergh/viib-mediahub/internal/db"
	"github.com/ajbergh/viib-mediahub/internal/logger"
)

// Supported provider names as constants for type safety
const (
	ProviderOllama     = "ollama"
	ProviderGemini     = "gemini"
	ProviderOpenAI     = "openai"
	ProviderAnthropic  = "anthropic"
	ProviderXAI        = "xai"
	ProviderOpenRouter = "openrouter"
)

// Default configuration
const (
	DefaultOllamaEndpoint  = "http://localhost:11434"
	DefaultOllamaModel     = "llama3.2:8b"
	DefaultGeminiModel     = "gemini-3.6-flash"
	DefaultOpenAIModel     = "gpt-5-mini"
	DefaultAnthropicModel  = "claude-sonnet-5"
	DefaultXAIModel        = "grok-4.5"
	DefaultOpenRouterModel = "openrouter/auto"
	DefaultOpenRouterURL   = "https://openrouter.ai/api/v1"
)

// HTTP client timeouts per provider (local models need much longer timeouts)
const (
	OllamaTimeout  = 5 * time.Minute // Local models can be slow for large batches
	DefaultTimeout = 2 * time.Minute // Cloud APIs are faster
)

// Settings contains configuration for LLM provider.
// Stored encrypted in the settings table.
type Settings struct {
	Provider   string `json:"provider"`   // "ollama", "gemini", "openai", "anthropic", "xai", "openrouter"
	Model      string `json:"model"`      // Model name (e.g., "llama3.2:8b", "gemini-3.6-flash")
	APIKey     string `json:"apiKey"`     // API key (empty for Ollama)
	BaseURL    string `json:"baseURL"`    // Custom endpoint (default for Ollama: localhost:11434)
	MaxRetries int    `json:"maxRetries"` // Default: 3
}

// DefaultSettings returns sensible defaults for Ollama (local-first)
func DefaultSettings() Settings {
	return Settings{
		Provider:   ProviderOllama,
		Model:      DefaultOllamaModel,
		APIKey:     "",
		BaseURL:    DefaultOllamaEndpoint,
		MaxRetries: 3,
	}
}

// PlaylistFilter represents the structured output from intent parsing.
// This mirrors the PlaylistFilter in internal/gemini to maintain compatibility.
type PlaylistFilter struct {
	Genres       []string `json:"genres"`
	Artists      []string `json:"artists"`
	MinYear      int      `json:"minYear"`
	MaxYear      int      `json:"maxYear"`
	Description  string   `json:"description"`
	Mood         string   `json:"mood,omitempty"`         // e.g., "happy", "sad", "energetic", "chill"
	Energy       string   `json:"energy,omitempty"`       // "low", "medium", "high"
	Tempo        string   `json:"tempo,omitempty"`        // "slow", "medium", "fast"
	Occasion     string   `json:"occasion,omitempty"`     // "workout", "study", "party", etc.
	Instrumental bool     `json:"instrumental,omitempty"` // true for instrumental only
	FromCache    bool     `json:"fromCache,omitempty"`    // indicates if result was from cache
	FromProvider string   `json:"fromProvider,omitempty"` // which provider generated this
}

// Provider wraps omnillm with AI DJ-specific functionality.
// It provides methods for generating playlist filters from natural language prompts.
type Provider struct {
	client       *omnillm.ChatClient
	providerName string
	model        string
	settings     Settings
}

// NewProvider creates a new LLM provider from settings.
// Returns an error if the provider cannot be initialized.
func NewProvider(settings Settings) (*Provider, error) {
	// Validate provider name
	providerName := mapProviderName(settings.Provider)
	if providerName == "" {
		return nil, fmt.Errorf("unsupported provider: %s", settings.Provider)
	}

	// Determine appropriate timeout for this provider
	// Local models (Ollama) need much longer timeouts for large batch processing
	timeout := DefaultTimeout
	if settings.Provider == ProviderOllama {
		timeout = OllamaTimeout
	}

	// Build omnillm config with custom HTTP client for timeout control
	config := omnillm.ClientConfig{
		Provider: omnillm.ProviderName(providerName),
		APIKey:   settings.APIKey,
		HTTPClient: &http.Client{
			Timeout: timeout,
		},
	}

	config.BaseURL = baseURLForProvider(settings.Provider, settings.BaseURL)

	// Create omnillm client
	client, err := omnillm.NewClient(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create LLM client: %w", err)
	}

	// Determine model to use
	model := settings.Model
	if model == "" {
		model = getDefaultModel(settings.Provider)
	}

	logger.API("LLM Provider initialized: provider=%s model=%s timeout=%v", settings.Provider, model, timeout)

	return &Provider{
		client:       client,
		providerName: settings.Provider,
		model:        model,
		settings:     settings,
	}, nil
}

// baseURLForProvider prevents an old Ollama endpoint from being reused after a
// user switches to a cloud provider. The UI only supports custom endpoints for
// Ollama; OpenRouter always uses its OpenAI-compatible API endpoint.
func baseURLForProvider(provider, configuredBaseURL string) string {
	if provider == ProviderOllama {
		if configuredBaseURL != "" {
			return configuredBaseURL
		}
		return DefaultOllamaEndpoint
	}
	if provider == ProviderOpenRouter {
		return DefaultOpenRouterURL
	}
	return ""
}

// NewProviderFromDB creates a provider using settings stored in the database.
// This reads the llm_provider, llm_model, llm_api_key, and llm_base_url settings.
func NewProviderFromDB(database *db.DB) (*Provider, error) {
	provider, _ := database.GetSetting("llm_provider")
	model, _ := database.GetSetting("llm_model")
	apiKey, _ := database.GetSetting("llm_api_key")
	baseURL, _ := database.GetSetting("llm_base_url")

	if provider == "" {
		provider = ProviderOllama // Default to Ollama
	}
	// A cloud provider must never inherit the prior local Ollama endpoint.
	if provider != ProviderOllama {
		baseURL = ""
	}

	settings := Settings{
		Provider:   provider,
		Model:      model,
		APIKey:     apiKey,
		BaseURL:    baseURL,
		MaxRetries: 3,
	}

	return NewProvider(settings)
}

// GetConfiguredProvider returns a provider using settings from the database.
// This is the primary entry point for both AI DJ and enrichment features.
//
// The function attempts providers in this order:
//  1. Unified LLM settings (llm_provider, llm_model, etc.)
//  2. Legacy gemini_api_key (for backward compatibility)
//
// Returns an error if no LLM is configured.
func GetConfiguredProvider(database *db.DB) (*Provider, error) {
	// Try unified LLM settings first
	llmProvider, _ := database.GetSetting("llm_provider")

	if llmProvider != "" {
		// Use unified LLM settings
		return NewProviderFromDB(database)
	}

	// Fallback: Check for legacy gemini_api_key
	geminiKey, err := database.GetSetting("gemini_api_key")
	if err == nil && geminiKey != "" {
		logger.API("LLM GetConfiguredProvider: Using legacy gemini_api_key")
		return NewProvider(Settings{
			Provider: ProviderGemini,
			Model:    DefaultGeminiModel,
			APIKey:   geminiKey,
		})
	}

	return nil, fmt.Errorf("no LLM configured: set AI Provider in Settings or provide a Gemini API key")
}

// ParsePlaylistFilter converts a natural language prompt into a structured filter.
// This is the primary method used by the AI DJ feature.
func (p *Provider) ParsePlaylistFilter(ctx context.Context, prompt string) (*PlaylistFilter, error) {
	startTime := time.Now()

	// Create chat completion request
	resp, err := p.client.CreateChatCompletion(ctx, &omnillm.ChatCompletionRequest{
		Model: p.model,
		Messages: []omnillm.Message{
			{Role: omnillm.RoleSystem, Content: PlaylistFilterSystemPrompt},
			{Role: omnillm.RoleUser, Content: prompt},
		},
		Temperature: p.temperature(0.3), // Low temperature for structured output when supported
	})
	if err != nil {
		return nil, fmt.Errorf("LLM request failed: %w", err)
	}

	// Extract response content
	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("empty response from LLM")
	}

	content := resp.Choices[0].Message.Content

	// Parse JSON response into PlaylistFilter
	filter, err := parseFilterFromResponse(content)
	if err != nil {
		return nil, fmt.Errorf("failed to parse LLM response: %w (content: %s)", err, truncateString(content, 200))
	}

	// Mark provider source
	filter.FromProvider = p.providerName

	duration := time.Since(startTime)
	logger.API("LLM ParsePlaylistFilter: provider=%s model=%s duration=%v genres=%v",
		p.providerName, p.model, duration, filter.Genres)

	return filter, nil
}

// ParsePlaylistFilterWithContext converts a natural language prompt into a structured filter,
// using the user's available genres to guide the LLM toward genres that actually exist in their library.
// This produces significantly better results than the generic ParsePlaylistFilter.
//
// Parameters:
//   - ctx: Request context for cancellation
//   - prompt: User's natural language playlist request (e.g., "90s alt rock")
//   - availableGenres: List of genre names from the user's library (most popular first)
//
// The LLM will prefer selecting from availableGenres when possible, falling back to
// generic genre names only when no match exists.
func (p *Provider) ParsePlaylistFilterWithContext(ctx context.Context, prompt string, availableGenres []string) (*PlaylistFilter, error) {
	startTime := time.Now()

	// Format the available genres for the prompt (limit to top 100 to stay within token limits)
	maxGenres := 100
	if len(availableGenres) > maxGenres {
		availableGenres = availableGenres[:maxGenres]
	}
	genresList := strings.Join(availableGenres, ", ")

	// Create context-aware system prompt
	systemPrompt := fmt.Sprintf(PlaylistFilterContextPromptTemplate, genresList)

	// Log the request for debugging
	logger.API("AI DJ Request: provider=%s model=%s prompt='%s' availableGenres=%d",
		p.providerName, p.model, prompt, len(availableGenres))

	// Create chat completion request
	resp, err := p.client.CreateChatCompletion(ctx, &omnillm.ChatCompletionRequest{
		Model: p.model,
		Messages: []omnillm.Message{
			{Role: omnillm.RoleSystem, Content: systemPrompt},
			{Role: omnillm.RoleUser, Content: prompt},
		},
		Temperature: p.temperature(0.3), // Low temperature for structured output when supported
	})
	if err != nil {
		return nil, fmt.Errorf("LLM request failed: %w", err)
	}

	// Extract response content
	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("empty response from LLM")
	}

	content := resp.Choices[0].Message.Content
	logger.API("AI DJ Raw LLM Response: %s", truncateString(content, 500))

	// Parse JSON response into PlaylistFilter
	filter, err := parseFilterFromResponse(content)
	if err != nil {
		return nil, fmt.Errorf("failed to parse LLM response: %w (content: %s)", err, truncateString(content, 200))
	}

	// Mark provider source
	filter.FromProvider = p.providerName

	duration := time.Since(startTime)
	logger.API("AI DJ Parsed Filter: provider=%s model=%s duration=%v genres=%v mood=%s energy=%s years=%d-%d",
		p.providerName, p.model, duration, filter.Genres, filter.Mood, filter.Energy, filter.MinYear, filter.MaxYear)

	return filter, nil
}

// TestConnection verifies that the provider is accessible and working.
// Returns nil if the connection is successful, or an error describing the issue.
func (p *Provider) TestConnection(ctx context.Context) error {
	// Send a simple test request
	resp, err := p.client.CreateChatCompletion(ctx, &omnillm.ChatCompletionRequest{
		Model: p.model,
		Messages: []omnillm.Message{
			{Role: omnillm.RoleUser, Content: "Reply with only the word: OK"},
		},
		MaxTokens:   intPtr(10),
		Temperature: p.temperature(0),
	})
	if err != nil {
		return fmt.Errorf("connection test failed: %w", err)
	}

	if len(resp.Choices) == 0 {
		return fmt.Errorf("no response received from provider")
	}

	return nil
}

// GetProviderName returns the name of the current provider.
func (p *Provider) GetProviderName() string {
	return p.providerName
}

// GetModel returns the model being used.
func (p *Provider) GetModel() string {
	return p.model
}

// Close releases resources held by the provider.
func (p *Provider) Close() error {
	if p.client != nil {
		return p.client.Close()
	}
	return nil
}

// mapProviderName converts our provider constants to omnillm's expected names
func mapProviderName(provider string) string {
	switch strings.ToLower(provider) {
	case ProviderOllama:
		return "ollama"
	case ProviderGemini:
		return "gemini"
	case ProviderOpenAI:
		return "openai"
	case ProviderAnthropic:
		return "anthropic"
	case ProviderXAI:
		return "xai"
	case ProviderOpenRouter:
		// omnillm's OpenAI adapter is compatible with OpenRouter when pointed at
		// OpenRouter's API base URL.
		return "openai"
	default:
		return ""
	}
}

// getDefaultModel returns the default model for a provider
func getDefaultModel(provider string) string {
	switch strings.ToLower(provider) {
	case ProviderOllama:
		return DefaultOllamaModel
	case ProviderGemini:
		return DefaultGeminiModel
	case ProviderOpenAI:
		return DefaultOpenAIModel
	case ProviderAnthropic:
		return DefaultAnthropicModel
	case ProviderXAI:
		return DefaultXAIModel
	case ProviderOpenRouter:
		return DefaultOpenRouterModel
	default:
		return ""
	}
}

// parseFilterFromResponse extracts a PlaylistFilter from LLM response text
func parseFilterFromResponse(content string) (*PlaylistFilter, error) {
	// Try to extract JSON from the response
	content = strings.TrimSpace(content)

	// Remove markdown code fences if present
	if strings.HasPrefix(content, "```json") {
		content = strings.TrimPrefix(content, "```json")
		content = strings.TrimSuffix(content, "```")
		content = strings.TrimSpace(content)
	} else if strings.HasPrefix(content, "```") {
		content = strings.TrimPrefix(content, "```")
		content = strings.TrimSuffix(content, "```")
		content = strings.TrimSpace(content)
	}

	// Find JSON object in the response
	startIdx := strings.Index(content, "{")
	endIdx := strings.LastIndex(content, "}")
	if startIdx == -1 || endIdx == -1 || endIdx <= startIdx {
		return nil, fmt.Errorf("no JSON object found in response")
	}
	jsonStr := content[startIdx : endIdx+1]

	// Parse JSON
	var filter PlaylistFilter
	if err := json.Unmarshal([]byte(jsonStr), &filter); err != nil {
		return nil, fmt.Errorf("JSON parse error: %w", err)
	}

	return &filter, nil
}

// Generate sends a system prompt and user prompt to the LLM and returns the response text.
// This is a generic method for use cases that need custom prompt handling.
func (p *Provider) Generate(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	resp, err := p.client.CreateChatCompletion(ctx, &omnillm.ChatCompletionRequest{
		Model: p.model,
		Messages: []omnillm.Message{
			{Role: omnillm.RoleSystem, Content: systemPrompt},
			{Role: omnillm.RoleUser, Content: userPrompt},
		},
		Temperature: p.temperature(0.4), // Slightly higher temp for creative generation when supported
	})
	if err != nil {
		return "", fmt.Errorf("LLM request failed: %w", err)
	}

	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("empty response from LLM")
	}

	return resp.Choices[0].Message.Content, nil
}

// Helper functions
func floatPtr(f float64) *float64 { return &f }
func intPtr(i int) *int           { return &i }

// temperature returns nil for providers and models that reject explicit
// sampling controls. Current Claude models require their default sampling
// behavior, so the field must be omitted entirely.
func (p *Provider) temperature(value float64) *float64 {
	if p.providerName == ProviderAnthropic {
		return nil
	}
	return floatPtr(value)
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
