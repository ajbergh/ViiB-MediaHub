package semantic

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	geminiEmbeddingsBaseURL      = "https://generativelanguage.googleapis.com/v1beta"
	geminiEmbeddingTimeout       = 2 * time.Minute
	geminiEmbeddingMaxInputBytes = 8192
	geminiEmbeddingMaxBatchBytes = 250000
)

// GeminiEmbeddingProvider uses Gemini's batch embeddings endpoint. Gemini
// recommends task-aware text prefixes for asymmetric text retrieval; prefixes
// are part of the persisted embedding identity and therefore force a safe
// rebuild when changed.
type GeminiEmbeddingProvider struct {
	baseURL    string
	apiKey     string
	model      string
	dimensions int
	client     *http.Client
}

func NewGeminiEmbeddingProvider(apiKey, model string, dimensions int, client *http.Client) (*GeminiEmbeddingProvider, error) {
	return newGeminiEmbeddingProvider(geminiEmbeddingsBaseURL, apiKey, model, dimensions, client)
}

func newGeminiEmbeddingProvider(baseURL, apiKey, model string, dimensions int, client *http.Client) (*GeminiEmbeddingProvider, error) {
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return nil, errors.New("a Gemini API key is required for semantic retrieval")
	}
	model = strings.TrimPrefix(strings.TrimSpace(model), "models/")
	if model == "" {
		model = DefaultGeminiEmbeddingModel
	}
	if !strings.HasPrefix(model, "gemini-embedding-") {
		return nil, fmt.Errorf("gemini semantic embeddings require a gemini-embedding model, got %q", model)
	}
	if dimensions <= 0 {
		dimensions = DefaultGeminiEmbeddingDimensions
	}
	if dimensions < 128 || dimensions > 3072 {
		return nil, fmt.Errorf("gemini embedding dimensions must be between 128 and 3072, got %d", dimensions)
	}
	if strings.TrimSpace(baseURL) == "" {
		baseURL = geminiEmbeddingsBaseURL
	}
	if client == nil {
		client = &http.Client{Timeout: geminiEmbeddingTimeout}
	}
	return &GeminiEmbeddingProvider{
		baseURL:    strings.TrimRight(baseURL, "/"),
		apiKey:     apiKey,
		model:      model,
		dimensions: dimensions,
		client:     client,
	}, nil
}

func (provider *GeminiEmbeddingProvider) Name() string { return EmbeddingProviderGemini }

func (provider *GeminiEmbeddingProvider) Model() string { return provider.model }

func (provider *GeminiEmbeddingProvider) DocumentPrefix() string {
	return "title: ViiB music catalog | text: "
}

func (provider *GeminiEmbeddingProvider) QueryPrefix() string {
	return "task: search result | query: "
}

func (provider *GeminiEmbeddingProvider) MaxBatchSize() int { return GeminiEmbeddingBatchSize }

func (provider *GeminiEmbeddingProvider) EmbedDocuments(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) > provider.MaxBatchSize() {
		return nil, fmt.Errorf("gemini batch size %d exceeds maximum %d", len(texts), provider.MaxBatchSize())
	}
	return provider.embed(ctx, texts, provider.DocumentPrefix())
}

func (provider *GeminiEmbeddingProvider) EmbedQuery(ctx context.Context, text string) ([]float32, error) {
	vectors, err := provider.embed(ctx, []string{text}, provider.QueryPrefix())
	if err != nil {
		return nil, err
	}
	if len(vectors) != 1 {
		return nil, fmt.Errorf("gemini returned %d query embeddings", len(vectors))
	}
	return vectors[0], nil
}

func (provider *GeminiEmbeddingProvider) embed(ctx context.Context, inputs []string, prefix string) ([][]float32, error) {
	if len(inputs) == 0 {
		return nil, errors.New("gemini embedding input cannot be empty")
	}
	type part struct {
		Text string `json:"text"`
	}
	type content struct {
		Parts []part `json:"parts"`
	}
	type requestBody struct {
		Model                string  `json:"model"`
		Content              content `json:"content"`
		OutputDimensionality int     `json:"outputDimensionality"`
	}
	requests := make([]requestBody, len(inputs))
	inputBytes := 0
	for index, input := range inputs {
		if strings.TrimSpace(input) == "" {
			return nil, fmt.Errorf("gemini embedding input %d is empty", index)
		}
		text := prefix + input
		byteCount := len([]byte(text))
		if byteCount > geminiEmbeddingMaxInputBytes {
			return nil, fmt.Errorf("gemini embedding input %d is %d bytes, exceeds conservative %d-byte limit", index, byteCount, geminiEmbeddingMaxInputBytes)
		}
		inputBytes += byteCount
		if inputBytes > geminiEmbeddingMaxBatchBytes {
			return nil, fmt.Errorf("gemini embedding request is %d bytes, exceeds conservative %d-byte limit", inputBytes, geminiEmbeddingMaxBatchBytes)
		}
		requests[index] = requestBody{
			Model:                "models/" + provider.model,
			Content:              content{Parts: []part{{Text: text}}},
			OutputDimensionality: provider.dimensions,
		}
	}
	payload, err := json.Marshal(struct {
		Requests []requestBody `json:"requests"`
	}{Requests: requests})
	if err != nil {
		return nil, fmt.Errorf("encode Gemini embedding request: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, provider.baseURL+"/models/"+provider.model+":batchEmbedContents", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	request.Header.Set("x-goog-api-key", provider.apiKey)
	request.Header.Set("Content-Type", "application/json")
	response, err := provider.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("request Gemini embeddings: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 8<<10))
		return nil, fmt.Errorf("gemini embeddings returned %s: %s", response.Status, strings.TrimSpace(string(body)))
	}
	var result struct {
		Embeddings []struct {
			Values []float32 `json:"values"`
		} `json:"embeddings"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 64<<20)).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode Gemini embeddings: %w", err)
	}
	if len(result.Embeddings) != len(inputs) {
		return nil, fmt.Errorf("gemini returned %d embeddings for %d inputs", len(result.Embeddings), len(inputs))
	}
	vectors := make([][]float32, len(result.Embeddings))
	for index, embedding := range result.Embeddings {
		vectors[index] = embedding.Values
	}
	return vectors, nil
}

func (provider *GeminiEmbeddingProvider) Close() error { return nil }

var _ EmbeddingProvider = (*GeminiEmbeddingProvider)(nil)
