// Resolves embedding provider/model settings independently from chat configuration.

package semantic

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/ajbergh/viib-mediahub/internal/db"
)

const (
	DefaultOpenAIEmbeddingModel          = "text-embedding-3-small"
	DefaultOpenAIEmbeddingDimensions     = 512
	DefaultGeminiEmbeddingModel          = "gemini-embedding-2"
	DefaultGeminiEmbeddingDimensions     = 768
	DefaultOpenRouterEmbeddingModel      = "openai/text-embedding-3-small"
	DefaultOpenRouterEmbeddingDimensions = 512

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

// ResolveEmbeddingSettings always selects ViiB's managed local model. Legacy
// settings remain readable for compatibility but never select a runtime or send
// credentials to an embedding service.
func ResolveEmbeddingSettings(_ context.Context, _ *db.DB, _ *http.Client) (EmbeddingResolution, error) {
	return ManagedEmbeddingResolution(), nil
}
