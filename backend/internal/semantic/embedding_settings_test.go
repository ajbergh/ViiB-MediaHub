package semantic

import (
	"context"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ajbergh/viib-mediahub/internal/db"
	"github.com/ajbergh/viib-mediahub/internal/llm"
)

func TestResolveEmbeddingSettingsUsesDedicatedAndChatConfiguration(t *testing.T) {
	database := newEmbeddingSettingsTestDB(t)
	if err := database.SetSettingsBatch(map[string]string{
		SemanticEmbeddingProviderSetting: EmbeddingProviderOllama,
		SemanticEmbeddingModelSetting:    "custom-embed",
		SemanticEmbeddingBaseURLSetting:  "http://127.0.0.1:11434/",
	}); err != nil {
		t.Fatal(err)
	}
	resolution, err := ResolveEmbeddingSettings(context.Background(), database, nil)
	if err != nil || !resolution.Ready() || resolution.Source != "setting" {
		t.Fatalf("resolution=%#v err=%v", resolution, err)
	}
	if resolution.Settings.Provider != EmbeddingProviderOllama || resolution.Settings.Model != "custom-embed" || resolution.Settings.BaseURL != "http://127.0.0.1:11434/" {
		t.Fatalf("settings=%#v", resolution.Settings)
	}

	if err := database.SetSettingsBatch(map[string]string{
		SemanticEmbeddingProviderSetting: "",
		SemanticEmbeddingAPIKeySetting:   "",
		"llm_provider":                   "openai",
		"llm_api_key":                    "chat-key",
	}); err != nil {
		t.Fatal(err)
	}
	resolution, err = ResolveEmbeddingSettings(context.Background(), database, nil)
	if err != nil || !resolution.Ready() || resolution.Source != "chat_provider" || resolution.Settings.Provider != EmbeddingProviderOpenAI || resolution.Settings.APIKey != "chat-key" || resolution.Settings.Dimensions != DefaultOpenAIEmbeddingDimensions {
		t.Fatalf("resolution=%#v err=%v", resolution, err)
	}
}

func TestResolveEmbeddingSettingsAutoProbeAndConfigurationMessage(t *testing.T) {
	database := newEmbeddingSettingsTestDB(t)
	called := false
	probe := func(_ context.Context, baseURL, model string, _ *http.Client) (bool, error) {
		called = true
		if baseURL != "http://localhost:11434" || model != DefaultOllamaEmbeddingModel {
			t.Fatalf("probe baseURL=%q model=%q", baseURL, model)
		}
		return true, nil
	}
	resolution, err := resolveEmbeddingSettings(context.Background(), database, nil, probe)
	if err != nil || !called || !resolution.Ready() || resolution.Source != "local_probe" || resolution.Settings.Provider != EmbeddingProviderOllama {
		t.Fatalf("resolution=%#v called=%v err=%v", resolution, called, err)
	}

	resolution, err = resolveEmbeddingSettings(context.Background(), database, nil, func(context.Context, string, string, *http.Client) (bool, error) { return false, nil })
	if err != nil || resolution.Status != embeddingResolutionNeedsConfiguration || resolution.Reason == "" {
		t.Fatalf("resolution=%#v err=%v", resolution, err)
	}
}

func TestResolveEmbeddingSettingsDocumentsNonNativeChatProviderFallback(t *testing.T) {
	providers := []string{llm.ProviderAnthropic, llm.ProviderXAI}
	for _, provider := range providers {
		t.Run(provider, func(t *testing.T) {
			database := newEmbeddingSettingsTestDB(t)
			if err := database.SetSetting("llm_provider", provider); err != nil {
				t.Fatal(err)
			}
			resolution, err := resolveEmbeddingSettings(context.Background(), database, nil, func(context.Context, string, string, *http.Client) (bool, error) {
				return false, nil
			})
			if err != nil || resolution.Status != embeddingResolutionNeedsConfiguration || !strings.Contains(resolution.Reason, provider+" chat has no native Phase 1 embedding adapter") {
				t.Fatalf("resolution=%#v err=%v", resolution, err)
			}
		})
	}
}

func TestResolveEmbeddingSettingsDirectsSupportedCloudChatProvidersToExplicitConfirmation(t *testing.T) {
	providers := []string{llm.ProviderGemini, llm.ProviderOpenRouter}
	for _, provider := range providers {
		t.Run(provider, func(t *testing.T) {
			database := newEmbeddingSettingsTestDB(t)
			if err := database.SetSettingsBatch(map[string]string{"llm_provider": provider, "llm_api_key": "chat-key"}); err != nil {
				t.Fatal(err)
			}
			resolution, err := resolveEmbeddingSettings(context.Background(), database, nil, func(context.Context, string, string, *http.Client) (bool, error) { return false, nil })
			if err != nil || resolution.Status != embeddingResolutionNeedsConfiguration || resolution.Settings.Provider != provider || !strings.Contains(resolution.Reason, "embeddings are supported") {
				t.Fatalf("resolution=%#v err=%v", resolution, err)
			}
		})
	}
}

func TestResolveEmbeddingSettingsRecognizesLegacyGeminiFallback(t *testing.T) {
	database := newEmbeddingSettingsTestDB(t)
	if err := database.SetSetting("gemini_api_key", "legacy-key"); err != nil {
		t.Fatal(err)
	}
	resolution, err := resolveEmbeddingSettings(context.Background(), database, nil, func(context.Context, string, string, *http.Client) (bool, error) {
		return false, nil
	})
	if err != nil || resolution.Status != embeddingResolutionNeedsConfiguration || !strings.Contains(resolution.Reason, "gemini embeddings are supported") {
		t.Fatalf("resolution=%#v err=%v", resolution, err)
	}
}

func TestExplicitOpenRouterAndGeminiReuseMatchingChatCredentials(t *testing.T) {
	for _, test := range []struct {
		provider, model string
		dimensions      int
	}{
		{EmbeddingProviderOpenRouter, DefaultOpenRouterEmbeddingModel, DefaultOpenRouterDimensions},
		{EmbeddingProviderGemini, DefaultGeminiEmbeddingModel, DefaultGeminiEmbeddingDimensions},
	} {
		t.Run(test.provider, func(t *testing.T) {
			database := newEmbeddingSettingsTestDB(t)
			if err := database.SetSettingsBatch(map[string]string{
				SemanticEmbeddingProviderSetting: test.provider,
				"llm_provider":                   test.provider,
				"llm_api_key":                    "chat-key",
			}); err != nil {
				t.Fatal(err)
			}
			resolution, err := ResolveEmbeddingSettings(context.Background(), database, nil)
			if err != nil || !resolution.Ready() || resolution.Settings.APIKey != "chat-key" || resolution.Settings.Model != test.model || resolution.Settings.Dimensions != test.dimensions {
				t.Fatalf("resolution=%#v err=%v", resolution, err)
			}
		})
	}
}

func TestLoadEmbeddingSettingsRejectsInvalidDimensions(t *testing.T) {
	database := newEmbeddingSettingsTestDB(t)
	if err := database.SetSetting(SemanticEmbeddingDimensionsSetting, "0"); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadEmbeddingSettings(database); err == nil {
		t.Fatal("zero dimensions accepted")
	}
}

func TestNewConfiguredEmbeddingProviderCreatesOpenAIAdapter(t *testing.T) {
	provider, err := NewConfiguredEmbeddingProvider(EmbeddingSettings{
		Provider:   EmbeddingProviderOpenAI,
		Model:      DefaultOpenAIEmbeddingModel,
		Dimensions: DefaultOpenAIEmbeddingDimensions,
		APIKey:     "test-key",
	}, nil)
	if err != nil || provider.Name() != EmbeddingProviderOpenAI || provider.Model() != DefaultOpenAIEmbeddingModel {
		t.Fatalf("provider=%T err=%v", provider, err)
	}
}

func TestNewConfiguredEmbeddingProviderCreatesSupportedCloudAdapters(t *testing.T) {
	for _, settings := range []EmbeddingSettings{
		{Provider: EmbeddingProviderOpenRouter, APIKey: "test-key"},
		{Provider: EmbeddingProviderGemini, APIKey: "test-key"},
	} {
		provider, err := NewConfiguredEmbeddingProvider(settings, nil)
		if err != nil || provider.Name() != settings.Provider {
			t.Fatalf("settings=%#v provider=%T err=%v", settings, provider, err)
		}
		_ = provider.Close()
	}
}

func newEmbeddingSettingsTestDB(t *testing.T) *db.DB {
	t.Helper()
	database, err := db.New(filepath.Join(t.TempDir(), "settings.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	return database
}
