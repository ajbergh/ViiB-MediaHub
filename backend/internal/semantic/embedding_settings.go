package semantic

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/ajbergh/viib-mediahub/internal/db"
	"github.com/ajbergh/viib-mediahub/internal/llm"
)

const (
	DefaultOpenAIEmbeddingModel      = "text-embedding-3-small"
	DefaultOpenAIEmbeddingDimensions = 512

	SemanticEmbeddingProviderSetting   = "semantic_embedding_provider"
	SemanticEmbeddingModelSetting      = "semantic_embedding_model"
	SemanticEmbeddingDimensionsSetting = "semantic_embedding_dimensions"
	SemanticEmbeddingBaseURLSetting    = "semantic_embedding_base_url"
	SemanticEmbeddingAPIKeySetting     = "semantic_embedding_api_key"

	embeddingResolutionReady              = "ready"
	embeddingResolutionDisabled           = "disabled"
	embeddingResolutionNeedsConfiguration = "needs_configuration"
)

// EmbeddingSettings holds only semantic-retrieval configuration. It is
// intentionally separate from chat settings so changing a chat model cannot
// silently change the vector space.
type EmbeddingSettings struct {
	Provider   string
	Model      string
	Dimensions int
	BaseURL    string
	APIKey     string
}

// EmbeddingResolution reports the selected embedding identity inputs or an
// actionable reason semantic retrieval is unavailable. A ready resolution is
// still validated by the provider test endpoint before a cloud rebuild starts.
type EmbeddingResolution struct {
	Settings EmbeddingSettings
	Status   string
	Source   string
	Reason   string
}

func (resolution EmbeddingResolution) Ready() bool {
	return resolution.Status == embeddingResolutionReady
}

type ollamaEmbeddingProbe func(context.Context, string, string, *http.Client) (bool, error)

// LoadEmbeddingSettings reads the dedicated semantic settings, including the
// API key through DB's encrypted setting path. Invalid dimensions are surfaced
// rather than silently changing an existing vector identity.
func LoadEmbeddingSettings(database *db.DB) (EmbeddingSettings, error) {
	values := make(map[string]string, 5)
	for _, key := range []string{
		SemanticEmbeddingProviderSetting,
		SemanticEmbeddingModelSetting,
		SemanticEmbeddingDimensionsSetting,
		SemanticEmbeddingBaseURLSetting,
		SemanticEmbeddingAPIKeySetting,
	} {
		value, err := database.GetSetting(key)
		if err != nil {
			return EmbeddingSettings{}, fmt.Errorf("read semantic setting %s: %w", key, err)
		}
		values[key] = value
	}
	settings := EmbeddingSettings{
		Provider: strings.ToLower(strings.TrimSpace(values[SemanticEmbeddingProviderSetting])),
		Model:    strings.TrimSpace(values[SemanticEmbeddingModelSetting]),
		BaseURL:  strings.TrimSpace(values[SemanticEmbeddingBaseURLSetting]),
		APIKey:   strings.TrimSpace(values[SemanticEmbeddingAPIKeySetting]),
	}
	if raw := strings.TrimSpace(values[SemanticEmbeddingDimensionsSetting]); raw != "" {
		dimensions, err := strconv.Atoi(raw)
		if err != nil || dimensions <= 0 {
			return EmbeddingSettings{}, fmt.Errorf("semantic embedding dimensions must be a positive integer")
		}
		settings.Dimensions = dimensions
	}
	return settings, nil
}

// ResolveEmbeddingSettings implements the documented auto-resolution order.
// The probe only reads Ollama's local tags endpoint; it never pulls a model.
func ResolveEmbeddingSettings(ctx context.Context, database *db.DB, client *http.Client) (EmbeddingResolution, error) {
	return resolveEmbeddingSettings(ctx, database, client, OllamaEmbeddingModelAvailable)
}

func resolveEmbeddingSettings(ctx context.Context, database *db.DB, client *http.Client, probe ollamaEmbeddingProbe) (EmbeddingResolution, error) {
	settings, err := LoadEmbeddingSettings(database)
	if err != nil {
		return EmbeddingResolution{}, err
	}
	provider := settings.Provider
	if provider == "" {
		provider = EmbeddingProviderAuto
	}
	switch provider {
	case EmbeddingProviderDisabled:
		return EmbeddingResolution{Settings: settings, Status: embeddingResolutionDisabled, Source: "setting", Reason: "semantic retrieval is disabled"}, nil
	case EmbeddingProviderOllama:
		return readyOllamaResolution(settings, "setting"), nil
	case EmbeddingProviderOpenAI:
		return readyOpenAIResolution(settings, "setting"), nil
	case EmbeddingProviderAuto:
	default:
		return EmbeddingResolution{Settings: settings, Status: embeddingResolutionNeedsConfiguration, Source: "setting", Reason: "semantic embedding provider must be auto, ollama, openai, or disabled"}, nil
	}

	chatProvider, err := database.GetSetting("llm_provider")
	if err != nil {
		return EmbeddingResolution{}, fmt.Errorf("read chat provider: %w", err)
	}
	chatProvider = strings.ToLower(strings.TrimSpace(chatProvider))
	if chatProvider == "" {
		legacyGeminiKey, err := database.GetSetting("gemini_api_key")
		if err != nil {
			return EmbeddingResolution{}, fmt.Errorf("read legacy Gemini API key: %w", err)
		}
		if strings.TrimSpace(legacyGeminiKey) != "" {
			chatProvider = llm.ProviderGemini
		}
	}
	if chatProvider == llm.ProviderOllama {
		if settings.BaseURL == "" {
			settings.BaseURL, err = database.GetSetting("llm_base_url")
			if err != nil {
				return EmbeddingResolution{}, fmt.Errorf("read chat base URL: %w", err)
			}
		}
		return readyOllamaResolution(settings, "chat_provider"), nil
	}
	if chatProvider == llm.ProviderOpenAI {
		if settings.APIKey == "" {
			settings.APIKey, err = database.GetSetting("llm_api_key")
			if err != nil {
				return EmbeddingResolution{}, fmt.Errorf("read chat API key: %w", err)
			}
		}
		return readyOpenAIResolution(settings, "chat_provider"), nil
	}

	if settings.BaseURL == "" {
		settings.BaseURL = llm.DefaultOllamaEndpoint
	}
	if settings.Model == "" {
		settings.Model = DefaultOllamaEmbeddingModel
	}
	available, probeErr := probe(ctx, settings.BaseURL, settings.Model, client)
	if probeErr == nil && available {
		return readyOllamaResolution(settings, "local_probe"), nil
	}
	reason := autoConfigurationReason(chatProvider, settings.Model)
	if probeErr != nil {
		reason += "; local Ollama probe failed: " + probeErr.Error()
	}
	return EmbeddingResolution{Settings: settings, Status: embeddingResolutionNeedsConfiguration, Source: "auto", Reason: reason}, nil
}

// autoConfigurationReason is intentionally explicit about chat providers that
// do not have a native Phase 1 embedding adapter. Their chat credentials must
// never be sent to an unrelated embedding API; an already-pulled local Ollama
// model remains the supported automatic fallback.
func autoConfigurationReason(chatProvider, ollamaModel string) string {
	base := fmt.Sprintf("pull %q in Ollama or configure an OpenAI API key for semantic embeddings", ollamaModel)
	switch chatProvider {
	case llm.ProviderGemini, llm.ProviderAnthropic, llm.ProviderXAI, llm.ProviderOpenRouter:
		return fmt.Sprintf("%s chat has no native Phase 1 embedding adapter; %s", chatProvider, base)
	default:
		return base
	}
}

func readyOllamaResolution(settings EmbeddingSettings, source string) EmbeddingResolution {
	settings.Provider = EmbeddingProviderOllama
	if settings.Model == "" {
		settings.Model = DefaultOllamaEmbeddingModel
	}
	if settings.BaseURL == "" {
		settings.BaseURL = llm.DefaultOllamaEndpoint
	}
	return EmbeddingResolution{Settings: settings, Status: embeddingResolutionReady, Source: source}
}

func readyOpenAIResolution(settings EmbeddingSettings, source string) EmbeddingResolution {
	settings.Provider = EmbeddingProviderOpenAI
	if settings.Model == "" {
		settings.Model = DefaultOpenAIEmbeddingModel
	}
	if settings.Dimensions == 0 {
		settings.Dimensions = DefaultOpenAIEmbeddingDimensions
	}
	if settings.APIKey == "" {
		return EmbeddingResolution{Settings: settings, Status: embeddingResolutionNeedsConfiguration, Source: source, Reason: "an OpenAI API key is required for semantic retrieval"}
	}
	return EmbeddingResolution{Settings: settings, Status: embeddingResolutionReady, Source: source}
}

// NewConfiguredEmbeddingProvider constructs the selected semantic embedding
// adapter without changing the configured chat provider or model.
func NewConfiguredEmbeddingProvider(settings EmbeddingSettings, client *http.Client) (EmbeddingProvider, error) {
	switch settings.Provider {
	case EmbeddingProviderOllama:
		return NewOllamaEmbeddingProvider(settings.BaseURL, settings.Model, client), nil
	case EmbeddingProviderOpenAI:
		return NewOpenAIEmbeddingProvider(settings.APIKey, settings.Model, settings.Dimensions, client)
	default:
		return nil, fmt.Errorf("unsupported semantic embedding provider %q", settings.Provider)
	}
}
