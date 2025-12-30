// Package llm - Library enrichment using unified LLM provider.
//
// This file contains functions for enriching song metadata (genres, mood,
// energy, tempo, BPM, instrumental detection, original year) using any
// supported LLM provider via the omnillm SDK.
//
// The enrichment uses TOON (Token-Oriented Object Notation) format for
// maximum token efficiency, allowing up to 200 songs per batch with
// Gemini and approximately 100 songs with other providers.
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

// enrichmentSystemPrompt is the system prompt for TOON-format metadata enrichment.
// This is optimized for token efficiency while maintaining output quality.
const enrichmentSystemPrompt = `You are a music expert with deep knowledge of artists, genres, and music history.

Analyze each song and return ALL metadata in TOON (Token-Oriented Object Notation) format.
TOON is a compact pipe-delimited format for maximum efficiency.

INPUT FORMAT (provided below):
ID|Artist|Title|Album|Year

OUTPUT FORMAT (one line per song, no headers):
ID|Genres|Mood|Energy|Tempo|BPM|Instrumental|OriginalYear

FIELD DEFINITIONS:
- ID: Return the exact ID from input
- Genres: Semicolon-separated list (e.g., "Rock;Alternative;90s Rock") - most specific first
- Mood: One of: happy, sad, energetic, calm, melancholic, uplifting, aggressive, romantic, chill, intense, dreamy, nostalgic
- Energy: One of: high, medium, low
- Tempo: One of: fast, medium, slow
- BPM: Estimated beats per minute (integer, 0 if unknown)
- Instrumental: true/false (true only if no vocals)
- OriginalYear: Original release year (NOT remaster date). Use your music history knowledge.

ANALYSIS RULES:
1. GENRES: Use real genres, from specific to broad. Include decade tags when appropriate (e.g., "80s Synthpop").
2. MOOD/ENERGY: Infer from artist's typical style, genre conventions, and title implications.
3. BPM: Estimate based on genre conventions (e.g., punk ~170, ballads ~70, dance ~128).
4. ORIGINAL YEAR: If album says "Remastered" or "Deluxe Edition", find the ORIGINAL release date.
5. INSTRUMENTAL: Most songs have vocals (false). Only true for classical, ambient, or explicitly instrumental.

CRITICAL: Return ONLY the TOON data. No headers, no explanations, no markdown. One song per line.

SONGS TO ANALYZE:`

// GetOptimalBatchSize returns the recommended batch size for enrichment based on provider.
// Different providers have different context window sizes and token limits.
func (p *Provider) GetOptimalBatchSize() int {
	switch p.providerName {
	case ProviderGemini:
		return 200 // Gemini has large context window, handles TOON well
	case ProviderOpenAI:
		return 100 // GPT-4o handles 100 songs well
	case ProviderAnthropic:
		return 150 // Claude has large context window
	case ProviderOllama:
		// Batch size depends on model size - smaller models need smaller batches
		return p.getOllamaBatchSize()
	case ProviderXAI:
		return 100 // Similar to OpenAI
	default:
		return 50 // Conservative default
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

// UseTOONFormat returns whether to use TOON format for this provider.
// TOON is more token-efficient but requires good instruction-following.
func (p *Provider) UseTOONFormat() bool {
	// TOON works well with most modern LLMs
	switch p.providerName {
	case ProviderGemini, ProviderOpenAI, ProviderAnthropic, ProviderXAI:
		return true
	case ProviderOllama:
		// Check if using a capable model
		model := strings.ToLower(p.model)
		return strings.Contains(model, "llama3") ||
			strings.Contains(model, "mixtral") ||
			strings.Contains(model, "gemma") ||
			strings.Contains(model, "qwen")
	default:
		return false
	}
}

// EnrichAllMetadata performs unified enrichment of genres, mood, energy, tempo, BPM,
// instrumental detection, and original year analysis in a single API call.
//
// This method uses TOON (Token-Oriented Object Notation) format instead of JSON
// for maximum token efficiency, allowing up to 200 songs per batch.
//
// TOON format uses pipe-delimited values with semicolon-separated genres:
// Input:  ID|Artist|Title|Album|Year
// Output: ID|Genres|Mood|Energy|Tempo|BPM|Instrumental|OriginalYear
//
// Returns a map of song ID to UnifiedMetadata.
func (p *Provider) EnrichAllMetadata(ctx context.Context, songs []db.Song) (map[string]*UnifiedMetadata, error) {
	if len(songs) == 0 {
		return nil, nil
	}

	logger.API("LLM EnrichAllMetadata: Starting with %d songs using %s/%s", len(songs), p.providerName, p.model)

	// Build prompt with TOON-formatted song list
	var promptBuilder strings.Builder
	promptBuilder.WriteString(enrichmentSystemPrompt)
	promptBuilder.WriteString("\n")

	for _, song := range songs {
		promptBuilder.WriteString(fmt.Sprintf("%s|%s|%s|%s|%d\n",
			song.ID, song.Artist, song.Title, song.Album, song.Year))
	}

	prompt := promptBuilder.String()
	logger.API("LLM EnrichAllMetadata: Prompt length: %d chars", len(prompt))

	result := make(map[string]*UnifiedMetadata)

	err := p.doWithRetry(ctx, func() error {
		resp, err := p.client.CreateChatCompletion(ctx, &omnillm.ChatCompletionRequest{
			Model: p.model,
			Messages: []omnillm.Message{
				{Role: omnillm.RoleUser, Content: prompt},
			},
			Temperature: floatPtr(0.2), // Low temperature for consistent structured output
		})
		if err != nil {
			// Check if it's a rate limit error
			if isRateLimitError(err) {
				return &retriableError{err}
			}
			return fmt.Errorf("LLM request failed: %w", err)
		}

		if len(resp.Choices) == 0 {
			return &retriableError{fmt.Errorf("empty response from LLM")}
		}

		responseText := resp.Choices[0].Message.Content
		logger.API("LLM EnrichAllMetadata: Response length: %d chars", len(responseText))

		// Log preview for debugging
		preview := responseText
		if len(preview) > 500 {
			preview = preview[:500] + "..."
		}
		logger.API("LLM EnrichAllMetadata: Response preview: %s", preview)

		// Remove any markdown formatting
		responseText = strings.TrimPrefix(responseText, "```")
		responseText = strings.TrimSuffix(responseText, "```")
		responseText = strings.TrimSpace(responseText)

		// Parse TOON response line by line
		lines := strings.Split(responseText, "\n")
		logger.API("LLM EnrichAllMetadata: Parsing %d lines", len(lines))

		parseErrors := 0
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}

			id, metadata, err := parseTOONLine(line)
			if err != nil {
				parseErrors++
				if parseErrors <= 3 {
					logger.API("LLM EnrichAllMetadata: Parse error on line: %s - %v", line, err)
				}
				continue
			}

			result[id] = metadata
		}

		logger.API("LLM EnrichAllMetadata: Parsed %d songs successfully, %d parse errors", len(result), parseErrors)

		// Verify we got results for at least some songs
		if len(result) == 0 && len(songs) > 0 {
			return &retriableError{fmt.Errorf("failed to parse TOON response: no valid entries")}
		}

		return nil
	})

	if err != nil {
		logger.API("LLM EnrichAllMetadata: Final error: %v", err)
		return nil, err
	}

	logger.API("LLM EnrichAllMetadata: Complete - returned %d results", len(result))
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
		if meta != nil {
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

// isRateLimitError checks if an error is a rate limit error
func isRateLimitError(err error) bool {
	if err == nil {
		return false
	}
	errStr := strings.ToLower(err.Error())
	return strings.Contains(errStr, "429") ||
		strings.Contains(errStr, "rate") ||
		strings.Contains(errStr, "quota") ||
		strings.Contains(errStr, "too many requests")
}

// parseTOONLine parses a single line of TOON (Token-Oriented Object Notation) format.
// Format: ID|Genre1;Genre2;Genre3|Mood|Energy|Tempo|BPM|Instrumental|OriginalYear
// Example: abc123|Rock;Alternative;90s Rock|energetic|high|fast|140|false|1994
func parseTOONLine(line string) (id string, metadata *UnifiedMetadata, err error) {
	line = strings.TrimSpace(line)
	if line == "" {
		return "", nil, fmt.Errorf("empty line")
	}

	parts := strings.Split(line, "|")
	if len(parts) < 8 {
		return "", nil, fmt.Errorf("invalid TOON format: expected 8 fields, got %d", len(parts))
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

	// Parse tempo
	metadata.Tempo = strings.TrimSpace(parts[4])
	if metadata.Tempo == "" {
		metadata.Tempo = "medium"
	}

	// Parse BPM (integer)
	bpmStr := strings.TrimSpace(parts[5])
	if bpmStr != "" && bpmStr != "0" {
		if bpm, parseErr := strconv.Atoi(bpmStr); parseErr == nil {
			metadata.BPM = bpm
		}
	}

	// Parse instrumental (boolean)
	instrStr := strings.ToLower(strings.TrimSpace(parts[6]))
	metadata.Instrumental = instrStr == "true" || instrStr == "1" || instrStr == "yes"

	// Parse original year (integer)
	yearStr := strings.TrimSpace(parts[7])
	if yearStr != "" && yearStr != "0" {
		if year, parseErr := strconv.Atoi(yearStr); parseErr == nil {
			metadata.OriginalYear = year
		}
	}

	return id, metadata, nil
}
