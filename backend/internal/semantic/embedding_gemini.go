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
	DefaultGeminiEmbeddingBaseURL    = "https://generativelanguage.googleapis.com/v1beta"
	DefaultGeminiEmbeddingModel      = "gemini-embedding-001"
	DefaultGeminiEmbeddingDimensions = 768
	geminiEmbeddingTimeout           = 2 * time.Minute
	geminiDocumentTask               = "RETRIEVAL_DOCUMENT"
	geminiQueryTask                  = "RETRIEVAL_QUERY"
)

// GeminiEmbeddingProvider uses Google's synchronous batchEmbedContents REST
// API. Retrieval task types are part of the persisted vector identity because
// Gemini deliberately produces asymmetric document and query embeddings.
type GeminiEmbeddingProvider struct {
	baseURL    string
	apiKey     string
	model      string
	dimensions int
	client     *http.Client
}

func NewGeminiEmbeddingProvider(apiKey, model string, dimensions int, client *http.Client) (*GeminiEmbeddingProvider, error) {
	return newGeminiEmbeddingProvider(DefaultGeminiEmbeddingBaseURL, apiKey, model, dimensions, client)
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
	if strings.ContainsAny(model, "/:") {
		return nil, fmt.Errorf("invalid Gemini embedding model %q", model)
	}
	if dimensions <= 0 {
		dimensions = DefaultGeminiEmbeddingDimensions
	}
	if strings.TrimSpace(baseURL) == "" {
		baseURL = DefaultGeminiEmbeddingBaseURL
	}
	if client == nil {
		client = &http.Client{Timeout: geminiEmbeddingTimeout}
	}
	return &GeminiEmbeddingProvider{
		baseURL: strings.TrimRight(baseURL, "/"), apiKey: apiKey, model: model,
		dimensions: dimensions, client: client,
	}, nil
}

func (provider *GeminiEmbeddingProvider) Name() string { return EmbeddingProviderGemini }

func (provider *GeminiEmbeddingProvider) Model() string { return provider.model }

func (provider *GeminiEmbeddingProvider) DocumentPrefix() string { return geminiDocumentTask }

func (provider *GeminiEmbeddingProvider) QueryPrefix() string { return geminiQueryTask }

func (provider *GeminiEmbeddingProvider) MaxBatchSize() int { return GeminiEmbeddingBatchSize }

func (provider *GeminiEmbeddingProvider) EmbedDocuments(ctx context.Context, texts []string) ([][]float32, error) {
	return provider.embed(ctx, texts, geminiDocumentTask)
}

func (provider *GeminiEmbeddingProvider) EmbedQuery(ctx context.Context, text string) ([]float32, error) {
	vectors, err := provider.embed(ctx, []string{text}, geminiQueryTask)
	if err != nil {
		return nil, err
	}
	if len(vectors) != 1 {
		return nil, fmt.Errorf("gemini returned %d query embeddings", len(vectors))
	}
	return vectors[0], nil
}

func (provider *GeminiEmbeddingProvider) embed(ctx context.Context, texts []string, taskType string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, errors.New("gemini embedding input cannot be empty")
	}
	if len(texts) > provider.MaxBatchSize() {
		return nil, fmt.Errorf("gemini batch size %d exceeds maximum %d", len(texts), provider.MaxBatchSize())
	}
	type part struct {
		Text string `json:"text"`
	}
	type requestContent struct {
		Parts []part `json:"parts"`
	}
	type embedRequest struct {
		Model                string         `json:"model"`
		Content              requestContent `json:"content"`
		TaskType             string         `json:"taskType"`
		OutputDimensionality int            `json:"outputDimensionality"`
	}
	payload := struct {
		Requests []embedRequest `json:"requests"`
	}{Requests: make([]embedRequest, len(texts))}
	for index, text := range texts {
		if strings.TrimSpace(text) == "" {
			return nil, fmt.Errorf("gemini embedding input %d is empty", index)
		}
		payload.Requests[index] = embedRequest{
			Model:                "models/" + provider.model,
			Content:              requestContent{Parts: []part{{Text: text}}},
			TaskType:             taskType,
			OutputDimensionality: provider.dimensions,
		}
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("encode Gemini embedding request: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, provider.baseURL+"/models/"+provider.model+":batchEmbedContents", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("x-goog-api-key", provider.apiKey)
	response, err := provider.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("request Gemini embeddings: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		responseBody, _ := io.ReadAll(io.LimitReader(response.Body, 8<<10))
		return nil, fmt.Errorf("gemini embeddings returned %s: %s", response.Status, strings.TrimSpace(string(responseBody)))
	}
	var result struct {
		Embeddings []struct {
			Values []float32 `json:"values"`
		} `json:"embeddings"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 64<<20)).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode Gemini embeddings: %w", err)
	}
	if len(result.Embeddings) != len(texts) {
		return nil, fmt.Errorf("gemini returned %d embeddings for %d inputs", len(result.Embeddings), len(texts))
	}
	vectors := make([][]float32, len(result.Embeddings))
	for index, embedding := range result.Embeddings {
		vectors[index] = embedding.Values
	}
	return vectors, nil
}

func (provider *GeminiEmbeddingProvider) Close() error { return nil }

var _ EmbeddingProvider = (*GeminiEmbeddingProvider)(nil)
