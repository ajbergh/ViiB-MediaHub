// Package llm - Library enrichment using unified LLM provider.
//
// This file contains functions for enriching song metadata (genres, mood,
// energy, tempo, BPM, instrumental detection, original year) using any
// supported LLM provider via the omnillm SDK.
//
// Enrichment uses a validated JSON request/response contract so arbitrary song
// tags cannot corrupt a batch or silently update an unrelated record.
//
// Key functions:
//   - EnrichAllMetadata: Unified enrichment for all metadata fields
//   - EnrichGenres: Wrapper that returns only genre classifications
//   - AnalyzeSongMood: Wrapper that returns mood/energy/tempo/BPM
//   - AnalyzeOriginalYear: Wrapper that returns original release year
//
// This replaces the Gemini-specific implementation in internal/gemini
// while maintaining backward compatibility with existing data structures.
package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"time"

	"github.com/agentplexus/omnillm"
	"github.com/ajbergh/viib-mediahub/internal/db"
	"github.com/ajbergh/viib-mediahub/internal/logger"
)

// Retry configuration for enrichment API calls
const (
	enrichMaxRetries     = 3                // Maximum number of retry attempts
	enrichBaseRetryDelay = 1 * time.Second  // Initial delay between retries
	enrichMaxRetryDelay  = 30 * time.Second // Maximum delay (cap for exponential backoff)
)

// UnifiedMetadata contains all AI-enriched metadata for a song.
// This is compatible with gemini.UnifiedMetadata for migration.
type UnifiedMetadata struct {
	Genres       []string `json:"genres"`        // Genre classifications for the song
	Mood         string   `json:"mood"`          // Emotional quality: happy, sad, energetic, calm, melancholic, etc.
	Energy       string   `json:"energy"`        // Energy level: high, medium, low
	Tempo        string   `json:"tempo"`         // Perceived tempo: fast, medium, slow
	BPM          int      `json:"bpm"`           // Estimated beats per minute (0 if unknown)
	Instrumental bool     `json:"instrumental"`  // true if song has no vocals
	OriginalYear int      `json:"original_year"` // Original release year (not remaster date)
}

// MoodAnalysis represents the AI-detected mood, energy, tempo, and BPM for a song.
// Compatible with gemini.MoodAnalysis for migration.
type MoodAnalysis struct {
	Mood         string `json:"mood"`         // Emotional quality
	Energy       string `json:"energy"`       // Energy level: high, medium, low
	Tempo        string `json:"tempo"`        // Perceived tempo: fast, medium, slow
	BPM          int    `json:"bpm"`          // Estimated beats per minute (0 if unknown)
	Instrumental bool   `json:"instrumental"` // true if song has no vocals
}

// OriginalYearAnalysis represents the AI-determined original release year.
// Compatible with gemini.OriginalYearAnalysis for migration.
type OriginalYearAnalysis struct {
	OriginalYear int  `json:"original_year"` // The original release year (0 if unknown)
	Confident    bool `json:"confident"`     // true if AI is confident in the year
}

// GetOptimalBatchSize returns the recommended batch size for enrichment based on provider.
// Different providers have different context window sizes and token limits.
func (p *Provider) GetOptimalBatchSize() int {
	switch p.providerName {
	case ProviderGemini:
		return 100 // Structured JSON needs more output room than the former TOON format
	case ProviderOpenAI:
		return 75
	case ProviderAnthropic:
		return 75
	case ProviderOllama:
		// Batch size depends on model size - smaller models need smaller batches
		return p.getOllamaBatchSize()
	case ProviderXAI:
		return 75
	case ProviderOpenRouter:
		return 50 // Routed models vary substantially in context and output limits
	default:
		return 50 // Conservative default
	}
}

// GetOptimalConcurrency returns the recommended number of concurrent API calls.
// Cloud APIs can handle parallel requests, but local models should process sequentially
// to avoid GPU memory contention and request timeouts.
func (p *Provider) GetOptimalConcurrency() int {
	switch p.providerName {
	case ProviderGemini, ProviderOpenAI, ProviderAnthropic, ProviderXAI:
		return 3 // Cloud APIs handle parallel requests well
	case ProviderOllama:
		// Ollama typically runs on a single GPU and can only process one request at a time
		// Running multiple concurrent requests causes queueing and timeouts
		return 1
	default:
		return 1 // Conservative default for unknown providers
	}
}

// getOllamaBatchSize returns the batch size for Ollama based on model parameters.
// Smaller models (like 4B) need much smaller batches to complete within timeout.
func (p *Provider) getOllamaBatchSize() int {
	model := strings.ToLower(p.model)

	// Check for small models (1-7B parameters) - need very small batches
	if strings.Contains(model, ":1b") || strings.Contains(model, ":2b") ||
		strings.Contains(model, ":3b") || strings.Contains(model, ":4b") ||
		strings.Contains(model, ":7b") {
		return 10 // Very small batch for smaller models
	}

	// Medium models (8-13B) - moderate batches
	if strings.Contains(model, ":8b") || strings.Contains(model, ":13b") ||
		strings.Contains(model, "3.2") { // Llama 3.2 variants are typically 8b
		return 20
	}

	// Larger models (30B+) - can handle more
	if strings.Contains(model, ":30b") || strings.Contains(model, ":32b") ||
		strings.Contains(model, ":70b") {
		return 40
	}

	// Default for unknown local models - conservative
	return 15
}

// UseTOONFormat is retained for compatibility. Enrichment now always uses JSON
// because it safely represents titles and artists containing delimiters.
func (p *Provider) UseTOONFormat() bool {
	return false
}

// EnrichAllMetadata performs unified enrichment in one LLM request. It uses a
// structured JSON contract so track metadata cannot corrupt the batch format.
func (p *Provider) EnrichAllMetadata(ctx context.Context, songs []db.Song) (map[string]*UnifiedMetadata, error) {
	if len(songs) == 0 {
		return nil, nil
	}

	logger.API("LLM EnrichAllMetadata: Starting with %d songs using %s/%s", len(songs), p.providerName, p.model)

	type inputSong struct {
		ID     string `json:"id"`
		Artist string `json:"artist"`
		Title  string `json:"title"`
		Album  string `json:"album"`
		Year   int    `json:"year"`
	}
	input := make([]inputSong, 0, len(songs))
	allowedIDs := make(map[string]struct{}, len(songs))
	for _, song := range songs {
		input = append(input, inputSong{song.ID, song.Artist, song.Title, song.Album, song.Year})
		allowedIDs[song.ID] = struct{}{}
	}
	prompt, err := json.Marshal(input)
	if err != nil {
		return nil, fmt.Errorf("marshal enrichment input: %w", err)
	}
	logger.API("LLM EnrichAllMetadata: Input length: %d chars", len(prompt))

	var result map[string]*UnifiedMetadata

	err = p.doWithRetry(ctx, func() error {
		resp, err := p.client.CreateChatCompletion(ctx, &omnillm.ChatCompletionRequest{
			Model: p.model,
			Messages: []omnillm.Message{
				{Role: omnillm.RoleSystem, Content: EnrichmentSystemPrompt},
				{Role: omnillm.RoleUser, Content: string(prompt)},
			},
			Temperature: p.temperature(0.2), // Low temperature for consistent structured output when supported
		})
		if err != nil {
			if isTransientError(err) {
				return &retriableError{err}
			}
			return fmt.Errorf("LLM request failed: %w", err)
		}

		if len(resp.Choices) == 0 {
			return &retriableError{fmt.Errorf("empty response from LLM")}
		}

		responseText := resp.Choices[0].Message.Content
		logger.API("LLM EnrichAllMetadata: Response length: %d chars", len(responseText))
		parsed, err := parseEnrichmentResponse(responseText, allowedIDs)
		if err != nil {
			return &retriableError{fmt.Errorf("invalid enrichment response: %w", err)}
		}
		result = parsed

		return nil
	})

	if err != nil {
		logger.API("LLM EnrichAllMetadata: Final error: %v", err)
		return nil, err
	}

	logger.API("LLM EnrichAllMetadata: Complete - returned %d validated results", len(result))
	return result, nil
}

// EnrichGenres enriches songs with genre classifications only.
// This is a convenience wrapper around EnrichAllMetadata that extracts only genres.
// Compatible with gemini.Client.EnrichGenres for migration.
func (p *Provider) EnrichGenres(ctx context.Context, songs []db.Song) (map[string][]string, error) {
	if len(songs) == 0 {
		return nil, nil
	}

	// Call unified enrichment
	unified, err := p.EnrichAllMetadata(ctx, songs)
	if err != nil {
		return nil, err
	}

	// Extract only genres from unified result
	result := make(map[string][]string)
	for id, meta := range unified {
		if meta != nil && len(meta.Genres) > 0 {
			result[id] = meta.Genres
		}
	}

	return result, nil
}

// AnalyzeSongMood analyzes songs for mood, energy, tempo, and BPM.
// This is a convenience wrapper around EnrichAllMetadata that extracts mood-related fields.
// Compatible with gemini.Client.AnalyzeSongMood for migration.
func (p *Provider) AnalyzeSongMood(ctx context.Context, songs []db.Song) (map[string]*MoodAnalysis, error) {
	if len(songs) == 0 {
		return nil, nil
	}

	// Call unified enrichment
	unified, err := p.EnrichAllMetadata(ctx, songs)
	if err != nil {
		return nil, err
	}

	// Extract mood analysis from unified result
	result := make(map[string]*MoodAnalysis)
	for id, meta := range unified {
		if meta != nil && meta.Mood != "" {
			result[id] = &MoodAnalysis{
				Mood:         meta.Mood,
				Energy:       meta.Energy,
				Tempo:        meta.Tempo,
				BPM:          meta.BPM,
				Instrumental: meta.Instrumental,
			}
		}
	}

	return result, nil
}

// AnalyzeOriginalYear determines the original release year of songs.
// This is a convenience wrapper around EnrichAllMetadata that extracts original year.
// Compatible with gemini.Client.AnalyzeOriginalYear for migration.
func (p *Provider) AnalyzeOriginalYear(ctx context.Context, songs []db.Song) (map[string]*OriginalYearAnalysis, error) {
	if len(songs) == 0 {
		return nil, nil
	}

	// Call unified enrichment
	unified, err := p.EnrichAllMetadata(ctx, songs)
	if err != nil {
		return nil, err
	}

	// Extract original year from unified result
	result := make(map[string]*OriginalYearAnalysis)
	for id, meta := range unified {
		if meta != nil {
			// Mark as confident if we got a valid year
			confident := meta.OriginalYear > 0
			result[id] = &OriginalYearAnalysis{
				OriginalYear: meta.OriginalYear,
				Confident:    confident,
			}
		}
	}

	return result, nil
}

// retriableError indicates an error that can be retried with backoff
type retriableError struct {
	err error
}

func (e *retriableError) Error() string {
	return e.err.Error()
}

func (e *retriableError) Unwrap() error {
	return e.err
}

// doWithRetry executes a function with exponential backoff retry.
// It handles transient errors (rate limits, server errors) gracefully.
func (p *Provider) doWithRetry(ctx context.Context, fn func() error) error {
	var lastErr error

	for attempt := 0; attempt < enrichMaxRetries; attempt++ {
		// Check context before each attempt
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		err := fn()
		if err == nil {
			return nil
		}

		lastErr = err

		// Check if error is retriable
		if _, ok := err.(*retriableError); !ok {
			return err // Non-retriable error, return immediately
		}

		// Calculate backoff with jitter
		delay := time.Duration(1<<attempt) * enrichBaseRetryDelay
		if delay > enrichMaxRetryDelay {
			delay = enrichMaxRetryDelay
		}
		// Add jitter (0-25% of delay)
		jitter := time.Duration(rand.Int63n(int64(delay / 4)))
		delay += jitter

		logger.API("LLM doWithRetry: Attempt %d failed, retrying in %v: %v", attempt+1, delay, err)

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
	}

	return fmt.Errorf("max retries exceeded: %w", lastErr)
}

// isTransientError identifies provider failures where retrying is useful.
func isTransientError(err error) bool {
	if err == nil {
		return false
	}
	errStr := strings.ToLower(err.Error())
	return strings.Contains(errStr, "429") ||
		strings.Contains(errStr, "rate") ||
		strings.Contains(errStr, "quota") ||
		strings.Contains(errStr, "too many requests") ||
		strings.Contains(errStr, "timeout") ||
		strings.Contains(errStr, "temporar") ||
		strings.Contains(errStr, "connection reset") ||
		strings.Contains(errStr, " 5") ||
		strings.Contains(errStr, "status 5")
}

type enrichmentResponse struct {
	ID           string   `json:"id"`
	Genres       []string `json:"genres"`
	Mood         string   `json:"mood"`
	Energy       string   `json:"energy"`
	Tempo        string   `json:"tempo"`
	BPM          int      `json:"bpm"`
	Instrumental bool     `json:"instrumental"`
	OriginalYear int      `json:"original_year"`
}

func parseEnrichmentResponse(responseText string, allowedIDs map[string]struct{}) (map[string]*UnifiedMetadata, error) {
	responseText = strings.TrimSpace(responseText)
	if strings.HasPrefix(responseText, "```") {
		responseText = strings.TrimPrefix(responseText, "```")
		if newline := strings.IndexByte(responseText, '\n'); newline >= 0 {
			responseText = responseText[newline+1:]
		}
		responseText = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(responseText), "```"))
	}

	var records []enrichmentResponse
	if err := json.Unmarshal([]byte(responseText), &records); err != nil {
		return nil, fmt.Errorf("expected JSON array: %w", err)
	}
	if len(records) != len(allowedIDs) {
		return nil, fmt.Errorf("expected %d records, got %d", len(allowedIDs), len(records))
	}

	result := make(map[string]*UnifiedMetadata, len(records))
	for _, record := range records {
		if _, ok := allowedIDs[record.ID]; !ok {
			return nil, fmt.Errorf("response contains an unknown id")
		}
		if _, duplicate := result[record.ID]; duplicate {
			return nil, fmt.Errorf("response contains duplicate id %q", record.ID)
		}
		metadata, err := validateEnrichmentMetadata(record)
		if err != nil {
			return nil, fmt.Errorf("id %q: %w", record.ID, err)
		}
		result[record.ID] = metadata
	}
	if len(result) != len(allowedIDs) {
		return nil, fmt.Errorf("response omitted one or more requested ids")
	}
	return result, nil
}

func validateEnrichmentMetadata(record enrichmentResponse) (*UnifiedMetadata, error) {
	genres := db.NormalizeGenres(record.Genres)
	if len(genres) > 5 {
		genres = genres[:5]
	}
	mood, err := normalizeChoice(record.Mood, map[string]string{
		"happy": "happy", "sad": "sad", "energetic": "energetic", "chill": "chill",
		"romantic": "romantic", "melancholic": "melancholic", "aggressive": "aggressive",
		"peaceful": "peaceful", "nostalgic": "nostalgic", "uplifting": "uplifting",
		"calm": "peaceful", "intense": "energetic", "dreamy": "chill",
	})
	if err != nil {
		return nil, fmt.Errorf("invalid mood: %w", err)
	}
	energy, err := normalizeChoice(record.Energy, map[string]string{"low": "low", "medium": "medium", "high": "high"})
	if err != nil {
		return nil, fmt.Errorf("invalid energy: %w", err)
	}
	tempo, err := normalizeChoice(record.Tempo, map[string]string{"slow": "slow", "medium": "medium", "fast": "fast"})
	if err != nil {
		return nil, fmt.Errorf("invalid tempo: %w", err)
	}
	if record.BPM != 0 && (record.BPM < 20 || record.BPM > 300) {
		return nil, fmt.Errorf("BPM must be 0 or between 20 and 300")
	}
	if record.OriginalYear != 0 && (record.OriginalYear < 1900 || record.OriginalYear > time.Now().Year()+1) {
		return nil, fmt.Errorf("original_year is outside the accepted range")
	}
	return &UnifiedMetadata{Genres: genres, Mood: mood, Energy: energy, Tempo: tempo, BPM: record.BPM, Instrumental: record.Instrumental, OriginalYear: record.OriginalYear}, nil
}

func normalizeChoice(value string, allowed map[string]string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" || value == "unknown" {
		return "", nil
	}
	if normalized, ok := allowed[value]; ok {
		return normalized, nil
	}
	return "", fmt.Errorf("%q is not allowed", value)
}

// parseTOONLine parses a single line of TOON (Token-Oriented Object Notation) format.
// Format: ID|Genre1;Genre2;Genre3|Mood|Energy|Tempo|BPM|Instrumental|OriginalYear (8 fields)
// Also handles 7-field format when LLM skips Tempo: ID|Genres|Mood|Energy|BPM|Instrumental|OriginalYear
// Example: abc123|Rock;Alternative;90s Rock|energetic|high|fast|140|false|1994
func parseTOONLine(line string) (id string, metadata *UnifiedMetadata, err error) {
	line = strings.TrimSpace(line)
	if line == "" {
		return "", nil, fmt.Errorf("empty line")
	}

	parts := strings.Split(line, "|")

	// Handle both 7-field (LLM skipped Tempo) and 8-field formats
	if len(parts) < 7 {
		return "", nil, fmt.Errorf("invalid TOON format: expected at least 7 fields, got %d", len(parts))
	}

	id = strings.TrimSpace(parts[0])
	if id == "" {
		return "", nil, fmt.Errorf("empty ID")
	}

	metadata = &UnifiedMetadata{}

	// Parse genres (semicolon-separated)
	genreStr := strings.TrimSpace(parts[1])
	if genreStr != "" && genreStr != "unknown" {
		for _, g := range strings.Split(genreStr, ";") {
			g = strings.TrimSpace(g)
			if g != "" {
				metadata.Genres = append(metadata.Genres, g)
			}
		}
	}

	// Parse mood
	metadata.Mood = strings.TrimSpace(parts[2])
	if metadata.Mood == "" {
		metadata.Mood = "unknown"
	}

	// Parse energy
	metadata.Energy = strings.TrimSpace(parts[3])
	if metadata.Energy == "" {
		metadata.Energy = "medium"
	}

	// Determine if this is 7-field or 8-field format by checking if parts[4] is a tempo word or a number
	is8FieldFormat := len(parts) >= 8
	tempoIdx, bpmIdx, instrIdx, yearIdx := 4, 5, 6, 7

	if !is8FieldFormat {
		// 7-field format: LLM skipped Tempo, parts[4] is BPM
		// Try to detect by checking if parts[4] looks like a number (BPM) or tempo word
		potentialTempo := strings.ToLower(strings.TrimSpace(parts[4]))
		if potentialTempo != "slow" && potentialTempo != "medium" && potentialTempo != "fast" {
			// It's a number (BPM), so LLM skipped Tempo
			bpmIdx = 4
			instrIdx = 5
			yearIdx = 6
			metadata.Tempo = "medium" // Default tempo when skipped
		}
	}

	// Parse tempo (only if 8-field format or it was actually a tempo word)
	if len(parts) >= 8 || (len(parts) == 7 && bpmIdx == 5) {
		metadata.Tempo = strings.TrimSpace(parts[tempoIdx])
		if metadata.Tempo == "" {
			metadata.Tempo = "medium"
		}
	}

	// Parse BPM (integer, valid range 20-300)
	if bpmIdx < len(parts) {
		bpmStr := strings.TrimSpace(parts[bpmIdx])
		if bpmStr != "" && bpmStr != "0" {
			if bpm, parseErr := strconv.Atoi(bpmStr); parseErr == nil && bpm >= 20 && bpm <= 300 {
				metadata.BPM = bpm
				// Infer tempo from BPM if we defaulted earlier
				if metadata.Tempo == "medium" && bpmIdx == 4 {
					if bpm >= 140 {
						metadata.Tempo = "fast"
					} else if bpm <= 80 {
						metadata.Tempo = "slow"
					}
				}
			}
		}
	}

	// Parse instrumental (boolean)
	if instrIdx < len(parts) {
		instrStr := strings.ToLower(strings.TrimSpace(parts[instrIdx]))
		metadata.Instrumental = instrStr == "true" || instrStr == "1" || instrStr == "yes"
	}

	// Parse original year (integer, valid range 1900-current+1)
	currentYear := time.Now().Year()
	if yearIdx < len(parts) {
		yearStr := strings.TrimSpace(parts[yearIdx])
		if yearStr != "" && yearStr != "0" {
			if year, parseErr := strconv.Atoi(yearStr); parseErr == nil && year >= 1900 && year <= currentYear+1 {
				metadata.OriginalYear = year
			}
		}
	}

	validated, validationErr := validateEnrichmentMetadata(enrichmentResponse{
		ID:           id,
		Genres:       metadata.Genres,
		Mood:         metadata.Mood,
		Energy:       metadata.Energy,
		Tempo:        metadata.Tempo,
		BPM:          metadata.BPM,
		Instrumental: metadata.Instrumental,
		OriginalYear: metadata.OriginalYear,
	})
	if validationErr != nil {
		return "", nil, validationErr
	}
	return id, validated, nil
}
