package semantic

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/llm"
)

const ollamaEmbeddingTimeout = 5 * time.Minute

// OllamaEmbeddingProvider uses the current batched /api/embed endpoint. It
// never pulls a model; availability validation is deliberately separate.
type OllamaEmbeddingProvider struct {
	baseURL string
	model   string
	client  *http.Client
}

func NewOllamaEmbeddingProvider(baseURL, model string, client *http.Client) *OllamaEmbeddingProvider {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = llm.DefaultOllamaEndpoint
	}
	if strings.TrimSpace(model) == "" {
		model = DefaultOllamaEmbeddingModel
	}
	if client == nil {
		client = &http.Client{Timeout: ollamaEmbeddingTimeout}
	}
	return &OllamaEmbeddingProvider{baseURL: strings.TrimRight(baseURL, "/"), model: model, client: client}
}

func (provider *OllamaEmbeddingProvider) Name() string  { return EmbeddingProviderOllama }
func (provider *OllamaEmbeddingProvider) Model() string { return provider.model }
func (provider *OllamaEmbeddingProvider) MaxBatchSize() int {
	return OllamaEmbeddingBatchSize
}

func (provider *OllamaEmbeddingProvider) DocumentPrefix() string {
	if provider.model == DefaultOllamaEmbeddingModel {
		return "search_document: "
	}
	return ""
}

func (provider *OllamaEmbeddingProvider) QueryPrefix() string {
	if provider.model == DefaultOllamaEmbeddingModel {
		return "search_query: "
	}
	return ""
}

func (provider *OllamaEmbeddingProvider) EmbedDocuments(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) > provider.MaxBatchSize() {
		return nil, fmt.Errorf("ollama batch size %d exceeds maximum %d", len(texts), provider.MaxBatchSize())
	}
	inputs := make([]string, len(texts))
	for index, text := range texts {
		inputs[index] = provider.DocumentPrefix() + text
	}
	return provider.embed(ctx, inputs)
}

func (provider *OllamaEmbeddingProvider) EmbedQuery(ctx context.Context, text string) ([]float32, error) {
	vectors, err := provider.embed(ctx, []string{provider.QueryPrefix() + text})
	if err != nil {
		return nil, err
	}
	if len(vectors) != 1 {
		return nil, fmt.Errorf("ollama returned %d query embeddings", len(vectors))
	}
	return vectors[0], nil
}

func (provider *OllamaEmbeddingProvider) embed(ctx context.Context, inputs []string) ([][]float32, error) {
	payload, err := json.Marshal(struct {
		Model    string   `json:"model"`
		Input    []string `json:"input"`
		Truncate bool     `json:"truncate"`
	}{Model: provider.model, Input: inputs, Truncate: false})
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, provider.baseURL+"/api/embed", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := provider.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("request ollama embeddings: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 8<<10))
		return nil, fmt.Errorf("ollama embeddings returned %s: %s", response.Status, strings.TrimSpace(string(body)))
	}
	var result struct {
		Embeddings [][]float32 `json:"embeddings"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 32<<20)).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode ollama embeddings: %w", err)
	}
	return result.Embeddings, nil
}

func (provider *OllamaEmbeddingProvider) Close() error { return nil }

// OllamaEmbeddingModelAvailable probes only the local model list. It does not
// call /api/pull or otherwise alter the user's Ollama installation.
func OllamaEmbeddingModelAvailable(ctx context.Context, baseURL, model string, client *http.Client) (bool, error) {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = llm.DefaultOllamaEndpoint
	}
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(baseURL, "/")+"/api/tags", nil)
	if err != nil {
		return false, err
	}
	response, err := client.Do(request)
	if err != nil {
		return false, fmt.Errorf("probe ollama models: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return false, fmt.Errorf("ollama model probe returned %s", response.Status)
	}
	var tags struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 4<<20)).Decode(&tags); err != nil {
		return false, fmt.Errorf("decode ollama model probe: %w", err)
	}
	for _, candidate := range tags.Models {
		if candidate.Name == model {
			return true, nil
		}
	}
	return false, nil
}

var _ EmbeddingProvider = (*OllamaEmbeddingProvider)(nil)
