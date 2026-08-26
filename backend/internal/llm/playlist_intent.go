package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

const (
	maxSemanticIntentTextLength = 512
	maxSemanticIntentListLength = 50
)

// PlaylistIntent is the bounded, retrieval-oriented interpretation of a music
// request. SemanticQuery describes meaning rather than trying to match the
// user's incomplete local metadata taxonomy.
type PlaylistIntent struct {
	IntentSummary         string   `json:"intentSummary"`
	SemanticQuery         string   `json:"semanticQuery"`
	NegativeSemanticQuery string   `json:"negativeSemanticQuery,omitempty"`
	IncludeArtists        []string `json:"includeArtists,omitempty"`
	ExcludeArtists        []string `json:"excludeArtists,omitempty"`
	PreferredGenres       []string `json:"preferredGenres,omitempty"`
	MinYear               int      `json:"minYear,omitempty"`
	MaxYear               int      `json:"maxYear,omitempty"`
	YearConstraintHard    bool     `json:"yearConstraintHard,omitempty"`
	InstrumentalOnly      bool     `json:"instrumentalOnly,omitempty"`
	DiscoveryBias         float64  `json:"discoveryBias,omitempty"`
	FamiliarityBias       float64  `json:"familiarityBias,omitempty"`
}

// ParsePlaylistIntent asks the LLM to compile retrieval intent without
// receiving the user's full genre catalog or any track-level library data.
func (p *Provider) ParsePlaylistIntent(ctx context.Context, prompt string) (PlaylistIntent, error) {
	response, err := p.Generate(ctx, PlaylistIntentSystemPrompt, prompt)
	if err != nil {
		return PlaylistIntent{}, fmt.Errorf("LLM playlist intent request failed: %w", err)
	}
	intent, err := ParsePlaylistIntentResponse(response, prompt)
	if err != nil {
		return FallbackPlaylistIntent(prompt), fmt.Errorf("parse LLM playlist intent: %w", err)
	}
	return intent, nil
}

// ParsePlaylistIntentResponse parses strict JSON and applies the fixed bounds
// that protect query embedding, filters, and downstream API responses.
func ParsePlaylistIntentResponse(response, rawPrompt string) (PlaylistIntent, error) {
	jsonObject := extractJSONObject(response)
	if jsonObject == "" {
		return PlaylistIntent{}, fmt.Errorf("no JSON object found in playlist intent response")
	}
	decoder := json.NewDecoder(bytes.NewBufferString(jsonObject))
	decoder.DisallowUnknownFields()
	var intent PlaylistIntent
	if err := decoder.Decode(&intent); err != nil {
		return PlaylistIntent{}, err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return PlaylistIntent{}, fmt.Errorf("playlist intent response contains additional JSON values")
	}
	return normalizePlaylistIntent(intent, rawPrompt), nil
}

// FallbackPlaylistIntent preserves availability when the LLM is unavailable or
// returns malformed JSON. The raw prompt is a deterministic, bounded query.
func FallbackPlaylistIntent(rawPrompt string) PlaylistIntent {
	query := normalizeIntentText(rawPrompt, maxSemanticIntentTextLength)
	return PlaylistIntent{
		IntentSummary:      query,
		SemanticQuery:      query,
		DiscoveryBias:      0.5,
		FamiliarityBias:    0.5,
		YearConstraintHard: false,
	}
}

func normalizePlaylistIntent(intent PlaylistIntent, rawPrompt string) PlaylistIntent {
	fallback := FallbackPlaylistIntent(rawPrompt)
	intent.IntentSummary = normalizeIntentText(intent.IntentSummary, maxSemanticIntentTextLength)
	intent.SemanticQuery = normalizeIntentText(intent.SemanticQuery, maxSemanticIntentTextLength)
	intent.NegativeSemanticQuery = normalizeIntentText(intent.NegativeSemanticQuery, maxSemanticIntentTextLength)
	if intent.IntentSummary == "" {
		intent.IntentSummary = fallback.IntentSummary
	}
	if intent.SemanticQuery == "" {
		intent.SemanticQuery = fallback.SemanticQuery
	}
	intent.IncludeArtists = normalizeIntentValues(intent.IncludeArtists, maxSemanticIntentListLength)
	intent.ExcludeArtists = normalizeIntentValues(intent.ExcludeArtists, maxSemanticIntentListLength)
	intent.PreferredGenres = normalizeIntentValues(intent.PreferredGenres, maxSemanticIntentListLength)
	if intent.MinYear < 1000 || intent.MinYear > 2100 {
		intent.MinYear = 0
	}
	if intent.MaxYear < 1000 || intent.MaxYear > 2100 {
		intent.MaxYear = 0
	}
	if intent.MinYear > 0 && intent.MaxYear > 0 && intent.MinYear > intent.MaxYear {
		intent.MinYear, intent.MaxYear = intent.MaxYear, intent.MinYear
	}
	if intent.MinYear == 0 && intent.MaxYear == 0 {
		intent.YearConstraintHard = false
	}
	intent.DiscoveryBias = clampIntentBias(intent.DiscoveryBias)
	intent.FamiliarityBias = clampIntentBias(intent.FamiliarityBias)
	return intent
}

func extractJSONObject(response string) string {
	response = strings.TrimSpace(response)
	response = strings.TrimPrefix(response, "```json")
	response = strings.TrimPrefix(response, "```")
	response = strings.TrimSuffix(response, "```")
	start := strings.Index(response, "{")
	end := strings.LastIndex(response, "}")
	if start == -1 || end <= start {
		return ""
	}
	return response[start : end+1]
}

func normalizeIntentText(value string, maximum int) string {
	value = strings.TrimSpace(strings.Join(strings.Fields(value), " "))
	if maximum <= 0 {
		return ""
	}
	count := 0
	for index := range value {
		if count == maximum {
			return strings.TrimSpace(value[:index])
		}
		count++
	}
	return value
}

func normalizeIntentValues(values []string, maximum int) []string {
	result := make([]string, 0, min(maximum, len(values)))
	seen := make(map[string]struct{})
	for _, value := range values {
		value = normalizeIntentText(value, maxSemanticIntentTextLength)
		key := strings.ToLower(value)
		if value == "" {
			continue
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, value)
		if len(result) == maximum {
			break
		}
	}
	return result
}

func clampIntentBias(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}
