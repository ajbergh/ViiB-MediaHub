package semantic

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOpenRouterEmbeddingProviderUsesOfficialCompatibleEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/embeddings" || request.Header.Get("Authorization") != "Bearer router-key" {
			t.Fatalf("request=%s authorization=%q", request.URL.Path, request.Header.Get("Authorization"))
		}
		var payload struct {
			Model      string   `json:"model"`
			Input      []string `json:"input"`
			Dimensions int      `json:"dimensions"`
		}
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload.Model != DefaultOpenRouterEmbeddingModel || payload.Dimensions != DefaultOpenRouterDimensions || len(payload.Input) != 2 {
			t.Fatalf("payload=%#v", payload)
		}
		_ = json.NewEncoder(writer).Encode(map[string]any{"data": []map[string]any{
			{"index": 0, "embedding": []float32{1, 0}},
			{"index": 1, "embedding": []float32{0, 1}},
		}})
	}))
	defer server.Close()
	provider, err := newOpenRouterEmbeddingProvider(server.URL, "router-key", "", 0, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	vectors, err := provider.EmbedDocuments(context.Background(), []string{"first", "second"})
	if err != nil || len(vectors) != 2 || provider.Name() != EmbeddingProviderOpenRouter {
		t.Fatalf("provider=%s vectors=%v err=%v", provider.Name(), vectors, err)
	}
}
