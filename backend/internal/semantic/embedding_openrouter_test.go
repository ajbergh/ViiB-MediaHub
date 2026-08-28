package semantic

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOpenRouterEmbeddingProviderUsesOpenAICompatibleEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/embeddings" || request.Method != http.MethodPost {
			t.Fatalf("request=%s %s", request.Method, request.URL.Path)
		}
		if request.Header.Get("Authorization") != "Bearer router-key" {
			t.Fatalf("authorization=%q", request.Header.Get("Authorization"))
		}
		var payload struct {
			Model      string   `json:"model"`
			Input      []string `json:"input"`
			Dimensions int      `json:"dimensions"`
		}
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload.Model != DefaultOpenRouterEmbeddingModel || payload.Dimensions != DefaultOpenRouterEmbeddingDimensions || len(payload.Input) != 1 || payload.Input[0] != "playlist discovery" {
			t.Fatalf("payload=%#v", payload)
		}
		_ = json.NewEncoder(writer).Encode(map[string]any{"data": []map[string]any{{"index": 0, "embedding": []float32{1, 0}}}})
	}))
	defer server.Close()
	provider, err := newOpenRouterEmbeddingProvider(server.URL, "router-key", "", 0, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	vector, err := provider.EmbedQuery(context.Background(), "playlist discovery")
	if err != nil || len(vector) != 2 || provider.Name() != EmbeddingProviderOpenRouter || provider.Model() != DefaultOpenRouterEmbeddingModel || provider.MaxBatchSize() != OpenRouterEmbeddingBatchSize {
		t.Fatalf("vector=%v provider=%#v err=%v", vector, provider, err)
	}
}
