package semantic

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOllamaEmbeddingProviderUsesBatchedCurrentEndpointAndPrefixes(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/api/embed" || request.Method != http.MethodPost {
			t.Fatalf("request = %s %s", request.Method, request.URL.Path)
		}
		requests++
		var payload struct {
			Model    string   `json:"model"`
			Input    []string `json:"input"`
			Truncate bool     `json:"truncate"`
		}
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload.Model != DefaultOllamaEmbeddingModel || payload.Truncate || len(payload.Input) != 2 || payload.Input[0] != "search_document: first" || payload.Input[1] != "search_document: second" {
			t.Fatalf("payload = %#v", payload)
		}
		_ = json.NewEncoder(writer).Encode(map[string]any{"embeddings": [][]float32{{1, 0}, {0, 1}}})
	}))
	defer server.Close()
	provider := NewOllamaEmbeddingProvider(server.URL, "", server.Client())
	vectors, err := provider.EmbedDocuments(context.Background(), []string{"first", "second"})
	if err != nil || len(vectors) != 2 || requests != 1 {
		t.Fatalf("vectors=%v requests=%d err=%v", vectors, requests, err)
	}
}

func TestOllamaEmbeddingProviderQueryPrefixAndModelProbe(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/api/embed":
			var payload struct {
				Input []string `json:"input"`
			}
			if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			if len(payload.Input) != 1 || payload.Input[0] != "search_query: late night drive" {
				t.Fatalf("input=%v", payload.Input)
			}
			_ = json.NewEncoder(writer).Encode(map[string]any{"embeddings": [][]float32{{0.5, 0.5}}})
		case "/api/tags":
			_ = json.NewEncoder(writer).Encode(map[string]any{"models": []map[string]string{{"name": DefaultOllamaEmbeddingModel}}})
		default:
			t.Fatalf("unexpected path %s", request.URL.Path)
		}
	}))
	defer server.Close()
	provider := NewOllamaEmbeddingProvider(server.URL, "", server.Client())
	vector, err := provider.EmbedQuery(context.Background(), "late night drive")
	if err != nil || len(vector) != 2 {
		t.Fatalf("vector=%v err=%v", vector, err)
	}
	available, err := OllamaEmbeddingModelAvailable(context.Background(), server.URL, DefaultOllamaEmbeddingModel, server.Client())
	if err != nil || !available {
		t.Fatalf("available=%v err=%v", available, err)
	}
}

func TestNormalizeEmbeddingBatchRejectsInvalidOutput(t *testing.T) {
	if _, err := NormalizeEmbeddingBatch([][]float32{{1, 0}}, 2, 0); err == nil {
		t.Fatal("count mismatch accepted")
	}
	if _, err := NormalizeEmbeddingBatch([][]float32{{0, 0}}, 1, 0); err == nil {
		t.Fatal("zero vector accepted")
	}
	if _, err := NormalizeEmbeddingBatch([][]float32{{float32(math.NaN())}}, 1, 0); err == nil {
		t.Fatal("NaN vector accepted")
	}
	vectors, err := NormalizeEmbeddingBatch([][]float32{{3, 4}}, 1, 2)
	if err != nil || math.Abs(float64(vectors[0][0]-0.6)) > 1e-6 {
		t.Fatalf("vectors=%v err=%v", vectors, err)
	}
}
