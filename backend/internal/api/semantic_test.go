package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/ajbergh/viib-mediahub/internal/db"
	"github.com/ajbergh/viib-mediahub/internal/semantic"
)

func TestSemanticStatusAndProviderTestEndpoints(t *testing.T) {
	database, err := db.New(filepath.Join(t.TempDir(), "semantic-api.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	service, err := semantic.NewService(database, apiEmbeddingProvider{})
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	api := &API{
		db:              database,
		semanticService: service,
		semanticState: semantic.EmbeddingResolution{
			Status: "ready",
			Settings: semantic.EmbeddingSettings{
				Provider: semantic.EmbeddingProviderOllama,
				Model:    "test-embedding-model",
			},
		},
	}
	routes := api.Routes()
	statusRecorder := httptest.NewRecorder()
	routes.ServeHTTP(statusRecorder, httptest.NewRequest(http.MethodGet, "/semantic/status", nil))
	if statusRecorder.Code != http.StatusOK {
		t.Fatalf("status code=%d body=%s", statusRecorder.Code, statusRecorder.Body.String())
	}
	var status semanticStatusResponse
	if err := json.NewDecoder(statusRecorder.Body).Decode(&status); err != nil {
		t.Fatal(err)
	}
	if status.State != "idle" || status.Provider != semantic.EmbeddingProviderOllama || status.Model != "test-embedding-model" {
		t.Fatalf("status=%#v", status)
	}
	testRecorder := httptest.NewRecorder()
	routes.ServeHTTP(testRecorder, httptest.NewRequest(http.MethodPost, "/semantic/test-embedding-provider", nil))
	if testRecorder.Code != http.StatusOK {
		t.Fatalf("test code=%d body=%s", testRecorder.Code, testRecorder.Body.String())
	}
	var providerTest map[string]any
	if err := json.NewDecoder(testRecorder.Body).Decode(&providerTest); err != nil {
		t.Fatal(err)
	}
	if providerTest["success"] != true || providerTest["dimensions"] != float64(2) {
		t.Fatalf("provider test=%#v", providerTest)
	}
}

func TestSemanticSettingsEndpointStoresDedicatedSettingsAndMasksAPIKey(t *testing.T) {
	database, err := db.New(filepath.Join(t.TempDir(), "semantic-settings-api.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	api := &API{db: database}
	routes := api.Routes()
	body := bytes.NewBufferString(`{"provider":"disabled","model":"text-embedding-3-small","dimensions":512,"baseURL":"https://example.invalid","apiKey":"secret-value"}`)
	updateRecorder := httptest.NewRecorder()
	routes.ServeHTTP(updateRecorder, httptest.NewRequest(http.MethodPut, "/semantic/settings", body))
	if updateRecorder.Code != http.StatusOK {
		t.Fatalf("update code=%d body=%s", updateRecorder.Code, updateRecorder.Body.String())
	}
	if got, err := database.GetSetting(semantic.SemanticEmbeddingAPIKeySetting); err != nil || got != "secret-value" {
		t.Fatalf("stored API key=%q err=%v", got, err)
	}
	settingsRecorder := httptest.NewRecorder()
	routes.ServeHTTP(settingsRecorder, httptest.NewRequest(http.MethodGet, "/semantic/settings", nil))
	if settingsRecorder.Code != http.StatusOK {
		t.Fatalf("settings code=%d body=%s", settingsRecorder.Code, settingsRecorder.Body.String())
	}
	var settings semanticSettingsResponse
	if err := json.NewDecoder(settingsRecorder.Body).Decode(&settings); err != nil {
		t.Fatal(err)
	}
	if settings.Provider != semantic.EmbeddingProviderDisabled || settings.Dimensions != 512 || !settings.APIKeyConfigured {
		t.Fatalf("settings=%#v", settings)
	}
	if bytes.Contains(settingsRecorder.Body.Bytes(), []byte("secret-value")) {
		t.Fatal("semantic settings response exposed the API key")
	}

	autoBody := bytes.NewBufferString(`{"provider":"auto","model":"nomic-embed-text","baseURL":""}`)
	autoRecorder := httptest.NewRecorder()
	routes.ServeHTTP(autoRecorder, httptest.NewRequest(http.MethodPut, "/semantic/settings", autoBody))
	if autoRecorder.Code != http.StatusOK {
		t.Fatalf("auto update code=%d body=%s", autoRecorder.Code, autoRecorder.Body.String())
	}
	loaded, err := semantic.LoadEmbeddingSettings(database)
	if err != nil {
		t.Fatalf("auto settings should be loadable: %v", err)
	}
	if loaded.Dimensions != 0 {
		t.Fatalf("auto dimensions=%d, want 0", loaded.Dimensions)
	}
}

type apiEmbeddingProvider struct{}

func (apiEmbeddingProvider) Name() string           { return semantic.EmbeddingProviderOllama }
func (apiEmbeddingProvider) Model() string          { return "test-embedding-model" }
func (apiEmbeddingProvider) DocumentPrefix() string { return "" }
func (apiEmbeddingProvider) QueryPrefix() string    { return "" }
func (apiEmbeddingProvider) MaxBatchSize() int      { return 32 }
func (apiEmbeddingProvider) Close() error           { return nil }
func (apiEmbeddingProvider) EmbedDocuments(_ context.Context, texts []string) ([][]float32, error) {
	vectors := make([][]float32, len(texts))
	for index := range texts {
		vectors[index] = []float32{1, 1}
	}
	return vectors, nil
}
func (apiEmbeddingProvider) EmbedQuery(_ context.Context, _ string) ([]float32, error) {
	return []float32{1, 1}, nil
}
