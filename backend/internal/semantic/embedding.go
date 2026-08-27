package semantic

import (
	"context"
	"errors"
	"fmt"
)

const (
	EmbeddingProviderAuto       = "auto"
	EmbeddingProviderOllama     = "ollama"
	EmbeddingProviderOpenAI     = "openai"
	EmbeddingProviderOpenRouter = "openrouter"
	EmbeddingProviderGemini     = "gemini"
	EmbeddingProviderDisabled   = "disabled"

	DefaultOllamaEmbeddingModel  = "nomic-embed-text"
	OllamaEmbeddingBatchSize     = 32
	OpenAIEmbeddingBatchSize     = 128
	OpenRouterEmbeddingBatchSize = 128
	GeminiEmbeddingBatchSize     = 32
)

// EmbeddingProvider keeps provider-specific task prefixes and transport details
// outside stored semantic documents. Document and query embeddings have
// separate methods because retrieval models can require asymmetric prompts.
type EmbeddingProvider interface {
	Name() string
	Model() string
	DocumentPrefix() string
	QueryPrefix() string
	EmbedDocuments(ctx context.Context, texts []string) ([][]float32, error)
	EmbedQuery(ctx context.Context, text string) ([]float32, error)
	MaxBatchSize() int
	Close() error
}

// EmbeddingIdentity is persisted in semantic_index_state. Every member affects
// vector comparability; a caller must not mix identities in one arena.
type EmbeddingIdentity struct {
	Provider       string
	Model          string
	Dimensions     int
	DocumentPrefix string
	QueryPrefix    string
}

func (identity EmbeddingIdentity) Valid() error {
	if identity.Provider == "" || identity.Model == "" || identity.Dimensions <= 0 {
		return errors.New("embedding identity requires provider, model, and dimensions")
	}
	return nil
}

// NormalizeEmbeddingBatch validates provider output shape and produces the
// normalized vectors that every durable and in-memory write path consumes.
func NormalizeEmbeddingBatch(vectors [][]float32, expectedCount, expectedDimensions int) ([][]float32, error) {
	if len(vectors) != expectedCount {
		return nil, fmt.Errorf("embedding provider returned %d vectors for %d inputs", len(vectors), expectedCount)
	}
	if expectedCount == 0 {
		return [][]float32{}, nil
	}
	dimensions := expectedDimensions
	if dimensions == 0 {
		dimensions = len(vectors[0])
	}
	if dimensions <= 0 {
		return nil, errors.New("embedding provider returned an empty vector")
	}
	normalized := make([][]float32, len(vectors))
	for index, vector := range vectors {
		if len(vector) != dimensions {
			return nil, fmt.Errorf("embedding %d dimensions %d do not match %d", index, len(vector), dimensions)
		}
		var err error
		normalized[index], err = NormalizeL2(vector)
		if err != nil {
			return nil, fmt.Errorf("invalid embedding %d: %w", index, err)
		}
	}
	return normalized, nil
}
