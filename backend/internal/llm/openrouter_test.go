package llm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestParseEnrichmentResponseRequiresCompleteKnownValidatedResults(t *testing.T) {
	allowed := map[string]struct{}{"first": {}, "second": {}}
	response := `[
		{"id":"first","genres":["ambient","Ambient","post-rock","electronic","drone","extra"],"mood":"calm","energy":"low","tempo":"slow","bpm":65,"instrumental":true,"original_year":0},
		{"id":"second","genres":["rock"],"mood":"energetic","energy":"high","tempo":"fast","bpm":160,"instrumental":false,"original_year":1995}
	]`
	results, err := parseEnrichmentResponse(response, allowed)
	if err != nil {
		t.Fatalf("parseEnrichmentResponse() error = %v", err)
	}
	if got, want := results["first"].Mood, "peaceful"; got != want {
		t.Errorf("normalized mood = %q, want %q", got, want)
	}
	if got := len(results["first"].Genres); got != 5 {
		t.Errorf("normalized genre count = %d, want 5", got)
	}
	if _, err := parseEnrichmentResponse(`[{"id":"first"}]`, allowed); err == nil {
		t.Error("partial response was accepted")
	}
	if _, err := parseEnrichmentResponse(`[{"id":"first"},{"id":"unknown"}]`, allowed); err == nil {
		t.Error("response with unknown id was accepted")
	}
	if _, err := parseEnrichmentResponse(`[{"id":"first","mood":"surprised"},{"id":"second"}]`, allowed); err == nil {
		t.Error("response with an unsupported mood was accepted")
	}
}

func TestGetOpenRouterModelsUsesCatalogAndKeepsAutoRouter(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.Header.Get("Authorization"), "Bearer test-key"; got != want {
			t.Errorf("Authorization = %q, want %q", got, want)
		}
		if got, want := r.URL.Query().Get("output_modalities"), "text"; got != want {
			t.Errorf("output_modalities = %q, want %q", got, want)
		}
		if got, want := r.URL.Query().Get("limit"), "1000"; got != want {
			t.Errorf("limit = %q, want %q", got, want)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[
			{"id":"zeta/model","name":"Zeta Model","description":"Zeta description"},
			{"id":"alpha/model","name":"Alpha Model","description":"Alpha description"}
		]}`))
	}))
	defer server.Close()

	models, err := getOpenRouterModels(context.Background(), "test-key", server.URL, server.Client())
	if err != nil {
		t.Fatalf("getOpenRouterModels() error = %v", err)
	}

	if len(models) != 3 {
		t.Fatalf("model count = %d, want 3", len(models))
	}
	if got, want := models[0].ID, DefaultOpenRouterModel; got != want {
		t.Errorf("first model ID = %q, want %q", got, want)
	}
	if got, want := models[1].ID, "alpha/model"; got != want {
		t.Errorf("first catalog model ID = %q, want %q", got, want)
	}
	if got, want := models[2].ID, "zeta/model"; got != want {
		t.Errorf("second catalog model ID = %q, want %q", got, want)
	}
}

func TestNewProviderSupportsOpenRouter(t *testing.T) {
	provider, err := NewProvider(Settings{Provider: ProviderOpenRouter, APIKey: "test-key"})
	if err != nil {
		t.Fatalf("NewProvider() error = %v", err)
	}
	defer provider.Close()

	if got, want := provider.GetModel(), DefaultOpenRouterModel; got != want {
		t.Errorf("GetModel() = %q, want %q", got, want)
	}
	if got, want := provider.client.Provider().Name(), "openai"; got != want {
		t.Errorf("adapter name = %q, want %q", got, want)
	}
}

func TestCloudProvidersDoNotReuseOllamaBaseURL(t *testing.T) {
	tests := []struct {
		provider string
		want     string
	}{
		{ProviderOpenRouter, DefaultOpenRouterURL},
		{ProviderOpenAI, ""},
		{ProviderAnthropic, ""},
		{ProviderGemini, ""},
		{ProviderXAI, ""},
	}
	for _, test := range tests {
		if got := baseURLForProvider(test.provider, "http://localhost:11434"); got != test.want {
			t.Errorf("%s base URL = %q, want %q", test.provider, got, test.want)
		}
	}
	if got, want := baseURLForProvider(ProviderOllama, ""), DefaultOllamaEndpoint; got != want {
		t.Errorf("Ollama default base URL = %q, want %q", got, want)
	}
}

func TestGeminiModelsIncludeCurrentTextModels(t *testing.T) {
	models := GetAvailableModels()[ProviderGemini]
	wantIDs := map[string]bool{
		"gemini-3.6-flash":       false,
		"gemini-3.5-flash":       false,
		"gemini-3.5-flash-lite":  false,
		"gemini-3.1-flash-lite":  false,
		"gemini-3.1-pro-preview": false,
	}
	for _, model := range models {
		if _, exists := wantIDs[model.ID]; exists {
			wantIDs[model.ID] = true
		}
	}
	for id, found := range wantIDs {
		if !found {
			t.Errorf("Gemini model list is missing %q", id)
		}
	}
	if got, want := DefaultGeminiModel, "gemini-3.6-flash"; got != want {
		t.Errorf("DefaultGeminiModel = %q, want %q", got, want)
	}
}

func TestCloudProviderListsUseCurrentModels(t *testing.T) {
	tests := []struct {
		provider string
		model    string
		defaultM string
	}{
		{ProviderOpenAI, "gpt-5.2", DefaultOpenAIModel},
		{ProviderAnthropic, "claude-sonnet-5", DefaultAnthropicModel},
		{ProviderXAI, "grok-4.5", DefaultXAIModel},
	}

	modelsByProvider := GetAvailableModels()
	for _, test := range tests {
		found := false
		for _, model := range modelsByProvider[test.provider] {
			if model.ID == test.model {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("%s model list is missing %q", test.provider, test.model)
		}
		if got := getDefaultModel(test.provider); got != test.defaultM {
			t.Errorf("default model for %s = %q, want %q", test.provider, got, test.defaultM)
		}
	}
}

func TestAnthropicRequestsOmitTemperature(t *testing.T) {
	provider := &Provider{providerName: ProviderAnthropic}
	if got := provider.temperature(0.3); got != nil {
		t.Errorf("Anthropic temperature = %v, want nil", *got)
	}

	provider.providerName = ProviderOpenAI
	if got := provider.temperature(0.3); got == nil || *got != 0.3 {
		t.Errorf("OpenAI temperature = %v, want 0.3", got)
	}
}
