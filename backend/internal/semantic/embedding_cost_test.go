package semantic

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/ajbergh/viib-mediahub/internal/db"
)

func TestOpenAIEmbeddingCostEstimateUsesCurrentDocumentCountAndConfirmation(t *testing.T) {
	database := newEmbeddingSettingsTestDB(t)
	for _, song := range []db.Song{
		{ID: "one", Title: "First", Artist: "Artist", Album: "Album", FilePath: filepath.Join(t.TempDir(), "one.flac"), AddedAt: 1},
		{ID: "two", Title: "Second", Artist: "Artist", Album: "Album", FilePath: filepath.Join(t.TempDir(), "two.flac"), AddedAt: 1},
	} {
		if err := database.SaveSong(&song); err != nil {
			t.Fatal(err)
		}
	}
	settings := EmbeddingSettings{Model: DefaultOpenAIEmbeddingModel, Dimensions: DefaultOpenAIEmbeddingDimensions}
	estimate, err := EstimateOpenAIEmbeddingCost(context.Background(), database, settings)
	if err != nil {
		t.Fatal(err)
	}
	if estimate.Documents != 4 || estimate.TypicalInputTokens != 1000 || estimate.MaximumInputTokens != 4096 || estimate.USDPerMillionInputTokens != openAIEmbeddingSmallUSDPerMillionTokens || estimate.Confirmed {
		t.Fatalf("estimate=%#v", estimate)
	}
	if err := database.SetSetting(SemanticEmbeddingCloudConfirmationSetting, estimate.ConfirmationID()); err != nil {
		t.Fatal(err)
	}
	confirmed, err := OpenAIEmbeddingCostConfirmed(context.Background(), database, settings)
	if err != nil || !confirmed.Confirmed {
		t.Fatalf("confirmed=%#v err=%v", confirmed, err)
	}
}

func TestOpenAIEmbeddingCostEstimateRejectsUnpricedModel(t *testing.T) {
	if _, err := EstimateOpenAIEmbeddingCost(context.Background(), newEmbeddingSettingsTestDB(t), EmbeddingSettings{Model: "text-embedding-3-future"}); err == nil {
		t.Fatal("unpriced model accepted")
	}
}
