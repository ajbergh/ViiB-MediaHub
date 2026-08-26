package semantic

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOpenAIEmbeddingProviderUsesDocumentedRequestShapeAndResponseIndexes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/embeddings" || request.Method != http.MethodPost {
			t.Fatalf("request = %s %s", request.Method, request.URL.Path)
		}
		if request.Header.Get("Authorization") != "Bearer test-key" {
			t.Fatalf("authorization = %q", request.Header.Get("Authorization"))
		}
		var payload struct {
			Model          string   `json:"model"`
			Input          []string `json:"input"`
			Dimensions     int      `json:"dimensions"`
			EncodingFormat string   `json:"encoding_format"`
		}
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload.Model != DefaultOpenAIEmbeddingModel || payload.Dimensions != DefaultOpenAIEmbeddingDimensions || payload.EncodingFormat != "float" || strings.Join(payload.Input, ",") != "first,second" {
			t.Fatalf("payload = %#v", payload)
		}
		_ = json.NewEncoder(writer).Encode(map[string]any{"data": []map[string]any{
			{"index": 1, "embedding": []float32{0, 1}},
			{"index": 0, "embedding": []float32{1, 0}},
		}})
	}))
	defer server.Close()
	provider, err := newOpenAIEmbeddingProvider(server.URL, "test-key", "", 0, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	vectors, err := provider.EmbedDocuments(context.Background(), []string{"first", "second"})
	if err != nil || len(vectors) != 2 || vectors[0][0] != 1 || vectors[1][1] != 1 {
		t.Fatalf("vectors=%v err=%v", vectors, err)
	}
	if provider.DocumentPrefix() != "" || provider.QueryPrefix() != "" || provider.MaxBatchSize() != OpenAIEmbeddingBatchSize {
		t.Fatalf("provider prefixes or batch size are wrong")
	}
}

func TestOpenAIEmbeddingProviderRejectsUnsafeInputWithoutRequest(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { requests++ }))
	defer server.Close()
	provider, err := newOpenAIEmbeddingProvider(server.URL, "test-key", "", 0, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	tooMany := make([]string, OpenAIEmbeddingBatchSize+1)
	for index := range tooMany {
		tooMany[index] = "document"
	}
	for _, input := range [][]string{tooMany, {""}, {strings.Repeat("a", openAIEmbeddingMaxInputBytes+1)}} {
		if _, err := provider.EmbedDocuments(context.Background(), input); err == nil {
			t.Fatalf("unsafe input accepted: %d entries", len(input))
		}
	}
	if requests != 0 {
		t.Fatalf("requests = %d", requests)
	}
}

func TestOpenAIEmbeddingProviderReportsHTTPFailures(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusUnauthorized)
		_, _ = writer.Write([]byte(`{"error":{"message":"invalid key"}}`))
	}))
	defer server.Close()
	provider, err := newOpenAIEmbeddingProvider(server.URL, "test-key", "", 0, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.EmbedQuery(context.Background(), "late night drive"); err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("err = %v", err)
	}
}

func TestNewOpenAIEmbeddingProviderRejectsUnsupportedModel(t *testing.T) {
	if _, err := NewOpenAIEmbeddingProvider("test-key", "text-embedding-ada-002", 512, nil); err == nil {
		t.Fatal("unsupported model accepted")
	}
}
