// Tests and fixtures for semantic behavior.

package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
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

func TestSemanticSettingsAreManagedAndReadOnly(t *testing.T) {
	database, err := db.New(filepath.Join(t.TempDir(), "settings.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	api := &API{db: database, semanticClosed: true}
	routes := api.Routes()
	update := httptest.NewRecorder()
	routes.ServeHTTP(update, httptest.NewRequest(http.MethodPut, "/semantic/settings", bytes.NewBufferString(`{"provider":"openai","apiKey":"secret"}`)))
	if update.Code != http.StatusMethodNotAllowed {
		t.Fatalf("update=%d %s", update.Code, update.Body)
	}
	result := httptest.NewRecorder()
	routes.ServeHTTP(result, httptest.NewRequest(http.MethodGet, "/semantic/settings", nil))
	var settings struct {
		Provider string
		Model    string
		Managed  bool
	}
	if err := json.Unmarshal(result.Body.Bytes(), &settings); err != nil {
		t.Fatal(err)
	}
	if settings.Provider != semantic.EmbeddingProviderLocal || settings.Model != semantic.LocalEmbeddingModel || !settings.Managed {
		t.Fatalf("%+v", settings)
	}
	if key, _ := database.GetSetting(semantic.SemanticEmbeddingAPIKeySetting); key != "" {
		t.Fatal("legacy update persisted a cloud credential")
	}
}

func TestChatProviderSwitchKeepsSemanticServiceAndIndex(t *testing.T) {
	database, err := db.New(filepath.Join(t.TempDir(), "switch.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := database.SaveSong(&db.Song{ID: "track", Title: "Quiet piano", Artist: "Artist", FilePath: filepath.Join(t.TempDir(), "song.flac")}); err != nil {
		t.Fatal(err)
	}
	service, err := semantic.NewService(database, apiEmbeddingProvider{})
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	if err := service.Reindex(context.Background()); err != nil {
		t.Fatal(err)
	}
	before, err := database.GetSemanticIndexStats(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	api := &API{db: database, semanticService: service, semanticState: semantic.ManagedEmbeddingResolution()}
	routes := api.Routes()
	for _, body := range []string{
		`{"provider":"openai","model":"chat-model","apiKey":"openai-secret"}`,
		`{"provider":"anthropic","model":"other-model","apiKey":"****masked"}`,
		`{"provider":"anthropic","model":"other-model","apiKey":"anthropic-secret"}`,
		`{"provider":"ollama","model":"local-chat","baseURL":"http://localhost:11434"}`,
	} {
		rec := httptest.NewRecorder()
		routes.ServeHTTP(rec, httptest.NewRequest(http.MethodPut, "/llm/settings", strings.NewReader(body)))
		if rec.Code != http.StatusOK {
			t.Fatalf("save=%d %s", rec.Code, rec.Body)
		}
		if api.currentSemanticService() != service || api.semanticGeneration != 0 {
			t.Fatal("chat switch restarted semantic service")
		}
		if strings.Contains(body, "masked") {
			if key, _ := database.GetSetting("llm_api_key"); key != "" {
				t.Fatal("provider switch reused old key")
			}
		}
	}
	after, err := database.GetSemanticIndexStats(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatal("chat switch changed durable vectors")
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

func (apiEmbeddingProvider) Dimensions() int { return 2 }

func TestProviderSwitchDoesNotSilentlyFallBackToLegacyGemini(t *testing.T) {
	database, err := db.New(filepath.Join(t.TempDir(), "legacy-switch.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := database.SetSettingsBatch(map[string]string{"gemini_api_key": "legacy-gemini-key", "llm_provider": "anthropic"}); err != nil {
		t.Fatal(err)
	}
	api := &API{db: database}
	settings := getLLMSettings(api)
	if settings.Provider != "anthropic" || settings.APIKey != "" || settings.Model != getDefaultModelForProvider("anthropic") {
		t.Fatalf("unexpected provider after switch: %#v", settings)
	}
	rec := httptest.NewRecorder()
	api.Routes().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/smart-playlist", strings.NewReader(`{"prompt":"a soundtrack for a completely imaginary journey","source":"local"}`)))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("unexpected fallback: %d %s", rec.Code, rec.Body)
	}
	if err := database.SetSetting("llm_provider", ""); err != nil {
		t.Fatal(err)
	}
	if settings := getLLMSettings(api); settings.Provider != "gemini" || settings.APIKey != "legacy-gemini-key" {
		t.Fatal("legacy installation fallback was lost")
	}
}
