// Tests and fixtures for embedding settings behavior.

package semantic

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/ajbergh/viib-mediahub/internal/db"
)

func TestManagedEmbeddingIdentityIgnoresChatAndLegacySettings(t *testing.T) {
	database := newEmbeddingSettingsTestDB(t)
	for _, provider := range []string{"openai", "anthropic", "gemini", "openrouter", "ollama", "xai"} {
		if err := database.SetSettingsBatch(map[string]string{
			"llm_provider": provider, "llm_model": "arbitrary-chat-model", "llm_api_key": "chat-secret",
			SemanticEmbeddingProviderSetting: provider, SemanticEmbeddingModelSetting: "legacy-embed-model",
			SemanticEmbeddingDimensionsSetting: "invalid-legacy-value", SemanticEmbeddingAPIKeySetting: "legacy-secret",
		}); err != nil {
			t.Fatal(err)
		}
		resolution, err := ResolveEmbeddingSettings(context.Background(), database, nil)
		if err != nil || resolution != ManagedEmbeddingResolution() {
			t.Fatalf("%s: %#v %v", provider, resolution, err)
		}
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

func newEmbeddingSettingsTestDB(t *testing.T) *db.DB {
	t.Helper()
	database, err := db.New(filepath.Join(t.TempDir(), "settings.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	return database
}
