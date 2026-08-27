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
)

const (
	openAIEmbeddingsBaseURL      = "https://api.openai.com/v1"
	openAIEmbeddingTimeout       = 2 * time.Minute
	openAIEmbeddingMaxInputBytes = 8192
	openAIEmbeddingMaxBatchBytes = 300000
)

// OpenAIEmbeddingProvider calls OpenAI's embeddings endpoint. The byte
// limits below are deliberately conservative guards: UTF-8 bytes upper-bound
// tokens, so no request near the documented token limits is sent accidentally.
type OpenAIEmbeddingProvider struct {
	providerName string
	errorLabel   string
	baseURL      string
	apiKey       string
	model        string
	dimensions   int
	batchSize    int
	client       *http.Client
}

func NewOpenAIEmbeddingProvider(apiKey, model string, dimensions int, client *http.Client) (*OpenAIEmbeddingProvider, error) {
	return newOpenAIEmbeddingProvider(openAIEmbeddingsBaseURL, apiKey, model, dimensions, client)
}

func newOpenAIEmbeddingProvider(baseURL, apiKey, model string, dimensions int, client *http.Client) (*OpenAIEmbeddingProvider, error) {
	return newOpenAICompatibleEmbeddingProvider(EmbeddingProviderOpenAI, "OpenAI", baseURL, apiKey, model, dimensions, OpenAIEmbeddingBatchSize, true, client)
}

// NewOpenRouterEmbeddingProvider uses OpenRouter's documented OpenAI-compatible
// embeddings endpoint while retaining a distinct vector identity.
func NewOpenRouterEmbeddingProvider(apiKey, model string, dimensions int, client *http.Client) (*OpenAIEmbeddingProvider, error) {
	return newOpenRouterEmbeddingProvider(DefaultOpenRouterBaseURL, apiKey, model, dimensions, client)
}

func newOpenRouterEmbeddingProvider(baseURL, apiKey, model string, dimensions int, client *http.Client) (*OpenAIEmbeddingProvider, error) {
	return newOpenAICompatibleEmbeddingProvider(EmbeddingProviderOpenRouter, "OpenRouter", baseURL, apiKey, model, dimensions, OpenRouterEmbeddingBatchSize, false, client)
}

func newOpenAICompatibleEmbeddingProvider(providerName, errorLabel, baseURL, apiKey, model string, dimensions, batchSize int, requireOpenAIModel bool, client *http.Client) (*OpenAIEmbeddingProvider, error) {
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return nil, fmt.Errorf("a %s API key is required for semantic retrieval", errorLabel)
	}
	model = strings.TrimSpace(model)
	if model == "" {
		if providerName == EmbeddingProviderOpenRouter {
			model = DefaultOpenRouterEmbeddingModel
		} else {
			model = DefaultOpenAIEmbeddingModel
		}
	}
	if requireOpenAIModel && !strings.HasPrefix(model, "text-embedding-3-") {
		return nil, fmt.Errorf("OpenAI semantic dimensions require a text-embedding-3 model, got %q", model)
	}
	if dimensions <= 0 {
		dimensions = DefaultOpenAIEmbeddingDimensions
	}
	if strings.TrimSpace(baseURL) == "" {
		baseURL = openAIEmbeddingsBaseURL
	}
	if client == nil {
		client = &http.Client{Timeout: openAIEmbeddingTimeout}
	}
	return &OpenAIEmbeddingProvider{
		providerName: providerName,
		errorLabel:   errorLabel,
		baseURL:      strings.TrimRight(baseURL, "/"),
		apiKey:       apiKey,
		model:        model,
		dimensions:   dimensions,
		batchSize:    batchSize,
		client:       client,
	}, nil
}

func (provider *OpenAIEmbeddingProvider) Name() string { return provider.providerName }

func (provider *OpenAIEmbeddingProvider) Model() string { return provider.model }

func (provider *OpenAIEmbeddingProvider) DocumentPrefix() string { return "" }

func (provider *OpenAIEmbeddingProvider) QueryPrefix() string { return "" }

func (provider *OpenAIEmbeddingProvider) MaxBatchSize() int { return provider.batchSize }

func (provider *OpenAIEmbeddingProvider) EmbedDocuments(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) > provider.MaxBatchSize() {
		return nil, fmt.Errorf("%s batch size %d exceeds maximum %d", provider.errorLabel, len(texts), provider.MaxBatchSize())
	}
	return provider.embed(ctx, texts)
}

func (provider *OpenAIEmbeddingProvider) EmbedQuery(ctx context.Context, text string) ([]float32, error) {
	vectors, err := provider.embed(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	if len(vectors) != 1 {
		return nil, fmt.Errorf("%s returned %d query embeddings", provider.errorLabel, len(vectors))
	}
	return vectors[0], nil
}

func (provider *OpenAIEmbeddingProvider) embed(ctx context.Context, inputs []string) ([][]float32, error) {
	if len(inputs) == 0 {
		return nil, fmt.Errorf("%s embedding input cannot be empty", provider.errorLabel)
	}
	inputBytes := 0
	for index, input := range inputs {
		if strings.TrimSpace(input) == "" {
			return nil, fmt.Errorf("%s embedding input %d is empty", provider.errorLabel, index)
		}
		bytes := len([]byte(input))
		if bytes > openAIEmbeddingMaxInputBytes {
			return nil, fmt.Errorf("%s embedding input %d is %d bytes, exceeds conservative %d-byte limit", provider.errorLabel, index, bytes, openAIEmbeddingMaxInputBytes)
		}
		inputBytes += bytes
		if inputBytes > openAIEmbeddingMaxBatchBytes {
			return nil, fmt.Errorf("%s embedding request is %d bytes, exceeds conservative %d-byte limit", provider.errorLabel, inputBytes, openAIEmbeddingMaxBatchBytes)
		}
	}
	payload, err := json.Marshal(struct {
		Model          string   `json:"model"`
		Input          []string `json:"input"`
		Dimensions     int      `json:"dimensions"`
		EncodingFormat string   `json:"encoding_format"`
	}{
		Model:          provider.model,
		Input:          inputs,
		Dimensions:     provider.dimensions,
		EncodingFormat: "float",
	})
	if err != nil {
		return nil, fmt.Errorf("encode %s embedding request: %w", provider.errorLabel, err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, provider.baseURL+"/embeddings", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", "Bearer "+provider.apiKey)
	request.Header.Set("Content-Type", "application/json")
	response, err := provider.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("request %s embeddings: %w", provider.errorLabel, err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 8<<10))
		return nil, fmt.Errorf("%s embeddings returned %s: %s", provider.errorLabel, response.Status, strings.TrimSpace(string(body)))
	}
	var result struct {
		Data []struct {
			Embedding []float32 `json:"embedding"`
			Index     int       `json:"index"`
		} `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 64<<20)).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode %s embeddings: %w", provider.errorLabel, err)
	}
	if len(result.Data) != len(inputs) {
		return nil, fmt.Errorf("%s returned %d embeddings for %d inputs", provider.errorLabel, len(result.Data), len(inputs))
	}
	vectors := make([][]float32, len(inputs))
	for _, datum := range result.Data {
		if datum.Index < 0 || datum.Index >= len(vectors) {
			return nil, fmt.Errorf("%s returned embedding index %d for %d inputs", provider.errorLabel, datum.Index, len(vectors))
		}
		if vectors[datum.Index] != nil {
			return nil, fmt.Errorf("%s returned duplicate embedding index %d", provider.errorLabel, datum.Index)
		}
		vectors[datum.Index] = datum.Embedding
	}
	return vectors, nil
}

func (provider *OpenAIEmbeddingProvider) Close() error { return nil }

var _ EmbeddingProvider = (*OpenAIEmbeddingProvider)(nil)
