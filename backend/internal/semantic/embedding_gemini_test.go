package semantic

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGeminiEmbeddingProviderUsesBatchRetrievalTasks(t *testing.T) {
	taskCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/models/gemini-embedding-001:batchEmbedContents" || request.Header.Get("x-goog-api-key") != "gemini-key" {
			t.Fatalf("request=%s key=%q", request.URL.Path, request.Header.Get("x-goog-api-key"))
		}
		var payload struct {
			Requests []struct {
				Model                string `json:"model"`
				TaskType             string `json:"taskType"`
				OutputDimensionality int    `json:"outputDimensionality"`
			} `json:"requests"`
		}
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		expectedTask := geminiDocumentTask
		if taskCalls == 1 {
			expectedTask = geminiQueryTask
		}
		for _, item := range payload.Requests {
			if item.Model != "models/"+DefaultGeminiEmbeddingModel || item.TaskType != expectedTask || item.OutputDimensionality != DefaultGeminiEmbeddingDimensions {
				t.Fatalf("request=%#v expected task=%s", item, expectedTask)
			}
		}
		taskCalls++
		embeddings := make([]map[string]any, len(payload.Requests))
		for index := range embeddings {
			embeddings[index] = map[string]any{"values": []float32{1, float32(index)}}
		}
		_ = json.NewEncoder(writer).Encode(map[string]any{"embeddings": embeddings})
	}))
	defer server.Close()
	provider, err := newGeminiEmbeddingProvider(server.URL, "gemini-key", "", 0, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	documents, err := provider.EmbedDocuments(context.Background(), []string{"first", "second"})
	if err != nil || len(documents) != 2 {
		t.Fatalf("documents=%v err=%v", documents, err)
	}
	query, err := provider.EmbedQuery(context.Background(), "query")
	if err != nil || len(query) != 2 || taskCalls != 2 {
		t.Fatalf("query=%v calls=%d err=%v", query, taskCalls, err)
	}
}
