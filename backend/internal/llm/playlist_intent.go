package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
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
	RequiredStyles        []string `json:"requiredStyles,omitempty"`
	ExcludedTerms         []string `json:"excludedTerms,omitempty"`
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
	excludedTerms, query := extractDeterministicExclusions(rawPrompt)
	requiredStyles := extractRequiredStyles(query)
	query = sanitizePositiveSemanticQuery(query, excludedTerms)
	if query == "" {
		query = "music matching the requested positive style and mood"
	}
	return PlaylistIntent{
		IntentSummary:         query,
		SemanticQuery:         query,
		RequiredStyles:        requiredStyles,
		ExcludedTerms:         excludedTerms,
		NegativeSemanticQuery: strings.Join(excludedTerms, " "),
		DiscoveryBias:         0.5,
		FamiliarityBias:       0.5,
		YearConstraintHard:    false,
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
	intent.RequiredStyles = mergeIntentValues(intent.RequiredStyles, fallback.RequiredStyles, maxSemanticIntentListLength)
	intent.ExcludedTerms = mergeIntentValues(intent.ExcludedTerms, fallback.ExcludedTerms, maxSemanticIntentListLength)
	intent.SemanticQuery = sanitizePositiveSemanticQuery(intent.SemanticQuery, intent.ExcludedTerms)
	if intent.SemanticQuery == "" {
		intent.SemanticQuery = fallback.SemanticQuery
	}
	if intent.NegativeSemanticQuery == "" && len(intent.ExcludedTerms) > 0 {
		intent.NegativeSemanticQuery = strings.Join(intent.ExcludedTerms, " ")
	}
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

var (
	exclusionPattern     = regexp.MustCompile(`(?i)(?:^|[\s,;:\-])(?:no|without|exclude|excluding|avoid|avoiding)\s+([a-z0-9][a-z0-9&' /\-]{0,80})`)
	exclusionStopPattern = regexp.MustCompile(`(?i)\s+(?:but|while|with|favor|favour|and)\s+`)
	nonIntentWordPattern = regexp.MustCompile(`[^a-z0-9&+]+`)
)

var requiredStyleFamilies = []struct {
	canonical string
	aliases   []string
}{
	{"jazz", []string{"jazz", "nu jazz", "acid jazz", "jazz fusion", "electro jazz", "contemporary jazz"}},
	{"rock", []string{"rock"}},
	{"hip hop", []string{"hip hop", "hip-hop", "rap"}},
	{"electronic", []string{"electronic", "electronica", "edm", "techno", "house"}},
	{"classical", []string{"classical"}},
	{"country", []string{"country"}},
	{"metal", []string{"metal"}},
	{"blues", []string{"blues"}},
	{"reggae", []string{"reggae"}},
	{"folk", []string{"folk"}},
	{"soul", []string{"soul"}},
	{"funk", []string{"funk"}},
	{"r&b", []string{"r&b", "rnb", "rhythm and blues"}},
	{"latin", []string{"latin"}},
	{"pop", []string{"pop"}},
}

func extractDeterministicExclusions(rawPrompt string) ([]string, string) {
	positive := normalizeIntentText(rawPrompt, maxSemanticIntentTextLength)
	exclusions := make([]string, 0, 3)
	// A greedy natural-language clause may contain another exclusion (for
	// example, "without Christmas and no pop"). Re-process the retained text
	// until every explicit marker has been consumed, with a fixed safety bound.
	for iteration := 0; iteration < maxSemanticIntentListLength; iteration++ {
		matches := exclusionPattern.FindAllStringSubmatchIndex(positive, -1)
		if len(matches) == 0 {
			break
		}
		positiveParts := make([]string, 0, len(matches)+1)
		last := 0
		for _, match := range matches {
			if len(match) < 4 {
				continue
			}
			positiveParts = append(positiveParts, positive[last:match[0]])
			phrase := positive[match[2]:match[3]]
			if stop := exclusionStopPattern.FindStringIndex(phrase); stop != nil {
				positiveParts = append(positiveParts, phrase[stop[1]:])
				phrase = phrase[:stop[0]]
			}
			phrase = trimExclusionNouns(phrase)
			if phrase != "" {
				exclusions = append(exclusions, phrase)
			}
			last = match[1]
		}
		positiveParts = append(positiveParts, positive[last:])
		next := normalizeIntentText(strings.Trim(strings.Join(positiveParts, " "), " \t\r\n,;:-"), maxSemanticIntentTextLength)
		if next == positive {
			break
		}
		positive = next
	}
	return normalizeIntentValues(exclusions, maxSemanticIntentListLength), positive
}

func trimExclusionNouns(value string) string {
	value = strings.TrimSpace(value)
	for _, suffix := range []string{" songs", " song", " music", " tracks", " track"} {
		if strings.HasSuffix(strings.ToLower(value), suffix) {
			value = strings.TrimSpace(value[:len(value)-len(suffix)])
			break
		}
	}
	return value
}

func extractRequiredStyles(positivePrompt string) []string {
	normalized := " " + nonIntentWordPattern.ReplaceAllString(strings.ToLower(positivePrompt), " ") + " "
	styles := make([]string, 0, 3)
	for _, family := range requiredStyleFamilies {
		for _, alias := range family.aliases {
			alias = nonIntentWordPattern.ReplaceAllString(strings.ToLower(alias), " ")
			if strings.Contains(normalized, " "+strings.TrimSpace(alias)+" ") {
				styles = append(styles, family.canonical)
				break
			}
		}
	}
	return normalizeIntentValues(styles, maxSemanticIntentListLength)
}

func sanitizePositiveSemanticQuery(query string, exclusions []string) string {
	query = normalizeIntentText(query, maxSemanticIntentTextLength)
	for _, exclusion := range exclusions {
		if exclusion == "" {
			continue
		}
		pattern := regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(exclusion) + `\b`)
		query = pattern.ReplaceAllString(query, " ")
	}
	query = regexp.MustCompile(`(?i)\b(?:no|without|exclude|excluding|avoid|avoiding)\b`).ReplaceAllString(query, " ")
	return normalizeIntentText(strings.Trim(query, " ,;:-"), maxSemanticIntentTextLength)
}

func mergeIntentValues(primary, required []string, maximum int) []string {
	return normalizeIntentValues(append(append([]string(nil), primary...), required...), maximum)
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
