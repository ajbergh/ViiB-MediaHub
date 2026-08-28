package semantic

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGeminiEmbeddingProviderUsesBatchEndpointAndTaskPrefixes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/models/gemini-embedding-2:batchEmbedContents" || request.Method != http.MethodPost {
			t.Fatalf("request = %s %s", request.Method, request.URL.Path)
		}
		if request.Header.Get("x-goog-api-key") != "test-key" {
			t.Fatalf("API key = %q", request.Header.Get("x-goog-api-key"))
		}
		var payload struct {
			Requests []struct {
				Model                string `json:"model"`
				OutputDimensionality int    `json:"outputDimensionality"`
				Content              struct {
					Parts []struct {
						Text string `json:"text"`
					} `json:"parts"`
				} `json:"content"`
			} `json:"requests"`
		}
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if len(payload.Requests) != 2 || payload.Requests[0].Model != "models/gemini-embedding-2" || payload.Requests[0].OutputDimensionality != DefaultGeminiEmbeddingDimensions || payload.Requests[0].Content.Parts[0].Text != "title: ViiB music catalog | text: first" || payload.Requests[1].Content.Parts[0].Text != "title: ViiB music catalog | text: second" {
			t.Fatalf("payload=%#v", payload)
		}
		_ = json.NewEncoder(writer).Encode(map[string]any{"embeddings": []map[string]any{
			{"values": []float32{1, 0}},
			{"values": []float32{0, 1}},
		}})
	}))
	defer server.Close()
	provider, err := newGeminiEmbeddingProvider(server.URL, "test-key", "", 0, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	vectors, err := provider.EmbedDocuments(context.Background(), []string{"first", "second"})
	if err != nil || len(vectors) != 2 || vectors[0][0] != 1 || vectors[1][1] != 1 || provider.QueryPrefix() != "task: search result | query: " || provider.MaxBatchSize() != GeminiEmbeddingBatchSize {
		t.Fatalf("vectors=%v err=%v", vectors, err)
	}
}

func TestGeminiEmbeddingProviderRejectsUnsafeInputAndDimensions(t *testing.T) {
	if _, err := NewGeminiEmbeddingProvider("test-key", "gemini-embedding-2", 127, nil); err == nil {
		t.Fatal("unsupported dimensions accepted")
	}
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { requests++ }))
	defer server.Close()
	provider, err := newGeminiEmbeddingProvider(server.URL, "test-key", "", 0, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.EmbedDocuments(context.Background(), []string{strings.Repeat("a", geminiEmbeddingMaxInputBytes)}); err == nil {
		t.Fatal("unsafe input accepted")
	}
	if requests != 0 {
		t.Fatalf("requests=%d", requests)
	}
}
