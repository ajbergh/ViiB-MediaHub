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
	baseURL      string
	apiKey       string
	model        string
	dimensions   int
	name         string
	label        string
	maxBatchSize int
	client       *http.Client
}

func NewOpenAIEmbeddingProvider(apiKey, model string, dimensions int, client *http.Client) (*OpenAIEmbeddingProvider, error) {
	return newOpenAIEmbeddingProvider(openAIEmbeddingsBaseURL, apiKey, model, dimensions, client)
}

func newOpenAIEmbeddingProvider(baseURL, apiKey, model string, dimensions int, client *http.Client) (*OpenAIEmbeddingProvider, error) {
	return newOpenAICompatibleEmbeddingProvider(EmbeddingProviderOpenAI, "OpenAI", openAIEmbeddingsBaseURL, baseURL, apiKey, model, dimensions, OpenAIEmbeddingBatchSize, true, client)
}

// newOpenAICompatibleEmbeddingProvider supports services that expose the
// OpenAI embeddings wire format. The OpenAI adapter keeps its stricter model
// validation; OpenRouter deliberately accepts its catalog-qualified model IDs.
func newOpenAICompatibleEmbeddingProvider(name, label, defaultBaseURL, baseURL, apiKey, model string, dimensions, maxBatchSize int, requireTextEmbedding3 bool, client *http.Client) (*OpenAIEmbeddingProvider, error) {
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return nil, fmt.Errorf("an %s API key is required for semantic retrieval", label)
	}
	model = strings.TrimSpace(model)
	if model == "" {
		model = DefaultOpenAIEmbeddingModel
	}
	if requireTextEmbedding3 && !strings.HasPrefix(model, "text-embedding-3-") {
		return nil, fmt.Errorf("%s semantic dimensions require a text-embedding-3 model, got %q", label, model)
	}
	if dimensions <= 0 {
		dimensions = DefaultOpenAIEmbeddingDimensions
	}
	if strings.TrimSpace(baseURL) == "" {
		baseURL = defaultBaseURL
	}
	if client == nil {
		client = &http.Client{Timeout: openAIEmbeddingTimeout}
	}
	return &OpenAIEmbeddingProvider{
		baseURL:      strings.TrimRight(baseURL, "/"),
		apiKey:       apiKey,
		model:        model,
		dimensions:   dimensions,
		name:         name,
		label:        label,
		maxBatchSize: maxBatchSize,
		client:       client,
	}, nil
}

func (provider *OpenAIEmbeddingProvider) Name() string { return provider.name }

func (provider *OpenAIEmbeddingProvider) Model() string { return provider.model }

func (provider *OpenAIEmbeddingProvider) DocumentPrefix() string { return "" }

func (provider *OpenAIEmbeddingProvider) QueryPrefix() string { return "" }

func (provider *OpenAIEmbeddingProvider) MaxBatchSize() int { return provider.maxBatchSize }

func (provider *OpenAIEmbeddingProvider) EmbedDocuments(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) > provider.MaxBatchSize() {
		return nil, fmt.Errorf("%s batch size %d exceeds maximum %d", provider.label, len(texts), provider.MaxBatchSize())
	}
	return provider.embed(ctx, texts)
}

func (provider *OpenAIEmbeddingProvider) EmbedQuery(ctx context.Context, text string) ([]float32, error) {
	vectors, err := provider.embed(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	if len(vectors) != 1 {
		return nil, fmt.Errorf("%s returned %d query embeddings", provider.label, len(vectors))
	}
	return vectors[0], nil
}

func (provider *OpenAIEmbeddingProvider) embed(ctx context.Context, inputs []string) ([][]float32, error) {
	if len(inputs) == 0 {
		return nil, fmt.Errorf("%s embedding input cannot be empty", provider.label)
	}
	inputBytes := 0
	for index, input := range inputs {
		if strings.TrimSpace(input) == "" {
			return nil, fmt.Errorf("%s embedding input %d is empty", provider.label, index)
		}
		bytes := len([]byte(input))
		if bytes > openAIEmbeddingMaxInputBytes {
			return nil, fmt.Errorf("%s embedding input %d is %d bytes, exceeds conservative %d-byte limit", provider.label, index, bytes, openAIEmbeddingMaxInputBytes)
		}
		inputBytes += bytes
		if inputBytes > openAIEmbeddingMaxBatchBytes {
			return nil, fmt.Errorf("%s embedding request is %d bytes, exceeds conservative %d-byte limit", provider.label, inputBytes, openAIEmbeddingMaxBatchBytes)
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
		return nil, fmt.Errorf("encode %s embedding request: %w", provider.label, err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, provider.baseURL+"/embeddings", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", "Bearer "+provider.apiKey)
	request.Header.Set("Content-Type", "application/json")
	response, err := provider.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("request %s embeddings: %w", provider.label, err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 8<<10))
		return nil, fmt.Errorf("%s embeddings returned %s: %s", provider.label, response.Status, strings.TrimSpace(string(body)))
	}
	var result struct {
		Data []struct {
			Embedding []float32 `json:"embedding"`
			Index     int       `json:"index"`
		} `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 64<<20)).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode %s embeddings: %w", provider.label, err)
	}
	if len(result.Data) != len(inputs) {
		return nil, fmt.Errorf("%s returned %d embeddings for %d inputs", provider.label, len(result.Data), len(inputs))
	}
	vectors := make([][]float32, len(inputs))
	for _, datum := range result.Data {
		if datum.Index < 0 || datum.Index >= len(vectors) {
			return nil, fmt.Errorf("%s returned embedding index %d for %d inputs", provider.label, datum.Index, len(vectors))
		}
		if vectors[datum.Index] != nil {
			return nil, fmt.Errorf("%s returned duplicate embedding index %d", provider.label, datum.Index)
		}
		vectors[datum.Index] = datum.Embedding
	}
	return vectors, nil
}

func (provider *OpenAIEmbeddingProvider) Close() error { return nil }

var _ EmbeddingProvider = (*OpenAIEmbeddingProvider)(nil)
