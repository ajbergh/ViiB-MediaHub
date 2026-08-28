package semantic

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/ajbergh/viib-mediahub/internal/db"
)

const (
	SemanticEmbeddingCloudConfirmationSetting = "semantic_embedding_cloud_confirmation"
	openAIEmbeddingTypicalTokensPerDocument   = 250
	openAIEmbeddingMaximumTokensPerDocument   = 1024
	openAIEmbeddingSmallUSDPerMillionTokens   = 0.02
	openAIEmbeddingLargeUSDPerMillionTokens   = 0.13
)

// OpenAIEmbeddingCostEstimate is an intentionally conservative, one-time
// estimate for the current deterministic document catalog. The cost rate was
// verified against OpenAI's embedding-model documentation on 2026-08-26.
// ConfirmationID is persisted internally but never returned to the browser.
type OpenAIEmbeddingCostEstimate struct {
	Model                    string  `json:"model"`
	Dimensions               int     `json:"dimensions"`
	Documents                int     `json:"documents"`
	TypicalInputTokens       int     `json:"typicalInputTokens"`
	MaximumInputTokens       int     `json:"maximumInputTokens"`
	USDPerMillionInputTokens float64 `json:"usdPerMillionInputTokens"`
	TypicalUSD               float64 `json:"typicalUSD"`
	MaximumUSD               float64 `json:"maximumUSD"`
	Confirmed                bool    `json:"confirmed"`
	confirmationID           string
}

// CloudEmbeddingConfirmation protects cloud providers for which ViiB cannot
// derive a reliable local price estimate. It records the exact provider,
// model, dimensions, and current deterministic document count that the user
// acknowledged before document text is sent to that provider.
type CloudEmbeddingConfirmation struct {
	Provider       string `json:"provider"`
	Model          string `json:"model"`
	Dimensions     int    `json:"dimensions"`
	Documents      int    `json:"documents"`
	Confirmed      bool   `json:"confirmed"`
	confirmationID string
}

// EstimateOpenAIEmbeddingCost counts the exact Phase 1 document set produced
// for the current catalog. It contains no API call and does not persist data.
func EstimateOpenAIEmbeddingCost(ctx context.Context, database *db.DB, settings EmbeddingSettings) (OpenAIEmbeddingCostEstimate, error) {
	if database == nil {
		return OpenAIEmbeddingCostEstimate{}, fmt.Errorf("semantic database is required")
	}
	if err := ctx.Err(); err != nil {
		return OpenAIEmbeddingCostEstimate{}, err
	}
	model := strings.TrimSpace(settings.Model)
	if model == "" {
		model = DefaultOpenAIEmbeddingModel
	}
	price, err := openAIEmbeddingPrice(model)
	if err != nil {
		return OpenAIEmbeddingCostEstimate{}, err
	}
	dimensions := settings.Dimensions
	if dimensions <= 0 {
		dimensions = DefaultOpenAIEmbeddingDimensions
	}
	songs, err := database.GetAllSongs()
	if err != nil {
		return OpenAIEmbeddingCostEstimate{}, fmt.Errorf("load catalog for OpenAI embedding estimate: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return OpenAIEmbeddingCostEstimate{}, err
	}
	documents := BuildDocuments(songs, DocumentContext{})
	estimate := OpenAIEmbeddingCostEstimate{
		Model:                    model,
		Dimensions:               dimensions,
		Documents:                len(documents),
		TypicalInputTokens:       len(documents) * openAIEmbeddingTypicalTokensPerDocument,
		MaximumInputTokens:       len(documents) * openAIEmbeddingMaximumTokensPerDocument,
		USDPerMillionInputTokens: price,
	}
	estimate.TypicalUSD = float64(estimate.TypicalInputTokens) * price / 1_000_000
	estimate.MaximumUSD = float64(estimate.MaximumInputTokens) * price / 1_000_000
	identity := fmt.Sprintf("%s\x00%d\x00%d\x00%d\x00%d\x00%.6f", estimate.Model, estimate.Dimensions, estimate.Documents, estimate.TypicalInputTokens, estimate.MaximumInputTokens, estimate.USDPerMillionInputTokens)
	sum := sha256.Sum256([]byte(identity))
	estimate.confirmationID = hex.EncodeToString(sum[:])
	return estimate, nil
}

// OpenAIEmbeddingCostConfirmed validates that the user explicitly accepted a
// current catalog/model/dimensions estimate before any cloud indexing starts.
func OpenAIEmbeddingCostConfirmed(ctx context.Context, database *db.DB, settings EmbeddingSettings) (OpenAIEmbeddingCostEstimate, error) {
	estimate, err := EstimateOpenAIEmbeddingCost(ctx, database, settings)
	if err != nil {
		return OpenAIEmbeddingCostEstimate{}, err
	}
	confirmed, err := database.GetSetting(SemanticEmbeddingCloudConfirmationSetting)
	if err != nil {
		return OpenAIEmbeddingCostEstimate{}, fmt.Errorf("read OpenAI embedding confirmation: %w", err)
	}
	estimate.Confirmed = strings.TrimSpace(confirmed) == estimate.confirmationID
	return estimate, nil
}

func (estimate OpenAIEmbeddingCostEstimate) ConfirmationID() string { return estimate.confirmationID }

// EstimateCloudEmbeddingConfirmation counts the current document set without
// contacting the provider. Gemini and OpenRouter pricing/model availability is
// account- and route-dependent, so this is intentionally an acknowledgement,
// not a local currency estimate.
func EstimateCloudEmbeddingConfirmation(ctx context.Context, database *db.DB, settings EmbeddingSettings) (CloudEmbeddingConfirmation, error) {
	if database == nil {
		return CloudEmbeddingConfirmation{}, fmt.Errorf("semantic database is required")
	}
	if err := ctx.Err(); err != nil {
		return CloudEmbeddingConfirmation{}, err
	}
	provider := strings.TrimSpace(settings.Provider)
	if provider != EmbeddingProviderGemini && provider != EmbeddingProviderOpenRouter {
		return CloudEmbeddingConfirmation{}, fmt.Errorf("cloud embedding confirmation supports Gemini or OpenRouter, got %q", provider)
	}
	model := strings.TrimSpace(settings.Model)
	dimensions := settings.Dimensions
	if provider == EmbeddingProviderGemini {
		if model == "" {
			model = DefaultGeminiEmbeddingModel
		}
		if dimensions <= 0 {
			dimensions = DefaultGeminiEmbeddingDimensions
		}
	} else {
		if model == "" {
			model = DefaultOpenRouterEmbeddingModel
		}
		if dimensions <= 0 {
			dimensions = DefaultOpenRouterEmbeddingDimensions
		}
	}
	songs, err := database.GetAllSongs()
	if err != nil {
		return CloudEmbeddingConfirmation{}, fmt.Errorf("load catalog for %s embedding confirmation: %w", provider, err)
	}
	if err := ctx.Err(); err != nil {
		return CloudEmbeddingConfirmation{}, err
	}
	documents := BuildDocuments(songs, DocumentContext{})
	confirmation := CloudEmbeddingConfirmation{
		Provider:   provider,
		Model:      model,
		Dimensions: dimensions,
		Documents:  len(documents),
	}
	identity := fmt.Sprintf("cloud-embedding\x00%s\x00%s\x00%d\x00%d", confirmation.Provider, confirmation.Model, confirmation.Dimensions, confirmation.Documents)
	sum := sha256.Sum256([]byte(identity))
	confirmation.confirmationID = hex.EncodeToString(sum[:])
	return confirmation, nil
}

// CloudEmbeddingConfirmationConfirmed reports whether the current Gemini or
// OpenRouter document set has been explicitly acknowledged.
func CloudEmbeddingConfirmationConfirmed(ctx context.Context, database *db.DB, settings EmbeddingSettings) (CloudEmbeddingConfirmation, error) {
	confirmation, err := EstimateCloudEmbeddingConfirmation(ctx, database, settings)
	if err != nil {
		return CloudEmbeddingConfirmation{}, err
	}
	stored, err := database.GetSetting(SemanticEmbeddingCloudConfirmationSetting)
	if err != nil {
		return CloudEmbeddingConfirmation{}, fmt.Errorf("read cloud embedding confirmation: %w", err)
	}
	confirmation.Confirmed = strings.TrimSpace(stored) == confirmation.confirmationID
	return confirmation, nil
}

func (confirmation CloudEmbeddingConfirmation) ConfirmationID() string {
	return confirmation.confirmationID
}

func openAIEmbeddingPrice(model string) (float64, error) {
	switch model {
	case DefaultOpenAIEmbeddingModel:
		return openAIEmbeddingSmallUSDPerMillionTokens, nil
	case "text-embedding-3-large":
		return openAIEmbeddingLargeUSDPerMillionTokens, nil
	default:
		return 0, fmt.Errorf("OpenAI cloud-cost confirmation supports text-embedding-3-small or text-embedding-3-large, got %q", model)
	}
}
