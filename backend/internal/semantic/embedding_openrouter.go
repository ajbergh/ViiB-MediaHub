package semantic

import "net/http"

const openRouterEmbeddingsBaseURL = "https://openrouter.ai/api/v1"

// OpenRouterEmbeddingProvider uses OpenRouter's OpenAI-compatible embeddings
// endpoint. Model IDs remain provider-qualified (for example
// openai/text-embedding-3-small) so OpenRouter routes the request correctly.
type OpenRouterEmbeddingProvider struct {
	*OpenAIEmbeddingProvider
}

func NewOpenRouterEmbeddingProvider(apiKey, model string, dimensions int, client *http.Client) (*OpenRouterEmbeddingProvider, error) {
	return newOpenRouterEmbeddingProvider(openRouterEmbeddingsBaseURL, apiKey, model, dimensions, client)
}

func newOpenRouterEmbeddingProvider(baseURL, apiKey, model string, dimensions int, client *http.Client) (*OpenRouterEmbeddingProvider, error) {
	if model == "" {
		model = DefaultOpenRouterEmbeddingModel
	}
	if dimensions <= 0 {
		dimensions = DefaultOpenRouterEmbeddingDimensions
	}
	provider, err := newOpenAICompatibleEmbeddingProvider(
		EmbeddingProviderOpenRouter,
		"OpenRouter",
		openRouterEmbeddingsBaseURL,
		baseURL,
		apiKey,
		model,
		dimensions,
		OpenRouterEmbeddingBatchSize,
		false,
		client,
	)
	if err != nil {
		return nil, err
	}
	return &OpenRouterEmbeddingProvider{OpenAIEmbeddingProvider: provider}, nil
}

var _ EmbeddingProvider = (*OpenRouterEmbeddingProvider)(nil)
