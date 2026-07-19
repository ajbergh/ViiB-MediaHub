// Package gemini is deprecated and maintained for backward compatibility.
//
// Deprecated: Use internal/llm instead for all new code.
// The llm package provides a unified interface supporting multiple LLM providers:
//   - Google Gemini
//   - OpenAI (GPT-4o, GPT-4o-mini)
//   - Anthropic Claude
//   - Ollama (local models)
//   - X.AI (Grok)
//
// This package will be removed in a future version.
//
// Original functionality (now in internal/llm):
//
// Unified Metadata Enrichment (TOON Format):
//   - EnrichAllMetadata: High-efficiency batch enrichment for up to 200 songs
//   - Uses TOON (Token-Oriented Object Notation) for compact response format
//   - Combines genres, mood, energy, tempo, BPM, instrumental detection, and original year
//   - Single API call per batch for maximum efficiency
//
// Legacy Wrapper Methods (call EnrichAllMetadata internally):
//   - EnrichGenres: Returns genre classifications only
//   - AnalyzeSongMood: Returns mood, energy, tempo, BPM, and instrumental flag
//   - AnalyzeOriginalYear: Returns original release year detection
//
// Playlist Filter Generation:
//   - GeneratePlaylistFilter: Parses natural language prompts into structured filters
//   - Extracts genres, years, mood, energy, tempo from user requests
//   - Uses caching to avoid duplicate API calls for similar prompts
//
// The package includes retry logic with exponential backoff for API resilience,
// and caching to minimize API costs for repeated queries.
package gemini

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/db"
	"github.com/ajbergh/viib-mediahub/internal/logger"
)

const apiEndpoint = "https://generativelanguage.googleapis.com/v1beta/models/gemini-3-flash-preview:generateContent"

// Retry configuration for API calls
const (
	maxRetries     = 3                // Maximum number of retry attempts
	baseRetryDelay = 1 * time.Second  // Initial delay between retries
	maxRetryDelay  = 30 * time.Second // Maximum delay (cap for exponential backoff)
)

// Filter cache configuration to reduce API costs
const (
	filterCacheTTL     = 15 * time.Minute // How long cached filters remain valid
	filterCacheMaxSize = 100              // Maximum entries before eviction
)

// Client provides methods for interacting with the Gemini API.
// It includes built-in caching for playlist filters and retry logic.
type Client struct {
	apiKey      string
	filterCache *filterCache
}

// filterCache stores recent playlist filters to avoid duplicate API calls.
// Uses LRU-like eviction when the cache is full.
type filterCache struct {
	mu      sync.RWMutex
	entries map[string]*filterCacheEntry
}

type filterCacheEntry struct {
	filter    *PlaylistFilter
	createdAt time.Time
}

func newFilterCache() *filterCache {
	return &filterCache{
		entries: make(map[string]*filterCacheEntry),
	}
}

func (fc *filterCache) get(prompt string) (*PlaylistFilter, bool) {
	fc.mu.RLock()
	defer fc.mu.RUnlock()

	key := strings.ToLower(strings.TrimSpace(prompt))
	entry, ok := fc.entries[key]
	if !ok {
		return nil, false
	}

	// Check if entry is expired
	if time.Since(entry.createdAt) > filterCacheTTL {
		return nil, false
	}

	return entry.filter, true
}

func (fc *filterCache) set(prompt string, filter *PlaylistFilter) {
	fc.mu.Lock()
	defer fc.mu.Unlock()

	// Evict old entries if cache is too large
	if len(fc.entries) >= filterCacheMaxSize {
		fc.evictOldest()
	}

	key := strings.ToLower(strings.TrimSpace(prompt))
	fc.entries[key] = &filterCacheEntry{
		filter:    filter,
		createdAt: time.Now(),
	}
}

func (fc *filterCache) evictOldest() {
	var oldestKey string
	var oldestTime time.Time

	for key, entry := range fc.entries {
		if oldestKey == "" || entry.createdAt.Before(oldestTime) {
			oldestKey = key
			oldestTime = entry.createdAt
		}
	}

	if oldestKey != "" {
		delete(fc.entries, oldestKey)
	}
}

func NewClient(apiKey string) *Client {
	return &Client{
		apiKey:      apiKey,
		filterCache: newFilterCache(),
	}
}

type generateContentRequest struct {
	Contents []content `json:"contents"`
}

type content struct {
	Parts []part `json:"parts"`
}

type part struct {
	Text string `json:"text"`
}

type generateContentResponse struct {
	Candidates []candidate `json:"candidates"`
}

type candidate struct {
	Content content `json:"content"`
}

// EnrichGenres takes a list of songs and returns a map of song ID to genres using Gemini.
// This is a legacy wrapper that calls EnrichAllMetadata and extracts only genres.
func (c *Client) EnrichGenres(songs []db.Song) (map[string][]string, error) {
	if len(songs) == 0 {
		return nil, nil
	}

	// Call unified enrichment
	unified, err := c.EnrichAllMetadata(songs)
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

// retriableError indicates an error that can be retried
type retriableError struct {
	err error
}

func (e *retriableError) Error() string {
	return e.err.Error()
}

func (e *retriableError) Unwrap() error {
	return e.err
}

// doWithRetry executes a function with exponential backoff retry
func (c *Client) doWithRetry(fn func() error) error {
	var lastErr error

	for attempt := 0; attempt < maxRetries; attempt++ {
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
		delay := time.Duration(1<<attempt) * baseRetryDelay
		if delay > maxRetryDelay {
			delay = maxRetryDelay
		}
		// Add jitter (0-25% of delay)
		jitter := time.Duration(rand.Int63n(int64(delay / 4)))
		delay += jitter

		time.Sleep(delay)
	}

	return fmt.Errorf("max retries exceeded: %w", lastErr)
}

// cleanAndRepairJSON attempts to fix common JSON errors from LLM responses
//
//lint:ignore U1000 Retained for compatibility with legacy Gemini response handling.
func cleanAndRepairJSON(input string) string {
	// 1. Extract JSON object (remove markdown and extra text)
	start := strings.Index(input, "{")
	end := strings.LastIndex(input, "}")
	if start != -1 && end != -1 && end > start {
		input = input[start : end+1]
	}

	// 2. Fix [ ... } pattern (common Gemini error where array ends with curly brace)
	// Matches [ followed by non-brackets, ending in }
	reArrayFix := regexp.MustCompile(`(?s)\[([^\[\]]*?)\}`)
	input = reArrayFix.ReplaceAllString(input, `[$1]`)

	// 3. Fix trailing commas before closing brackets/braces
	reTrailingComma := regexp.MustCompile(`,\s*([\]}])`)
	input = reTrailingComma.ReplaceAllString(input, `$1`)

	return input
}

// UnifiedMetadata contains all AI-enriched metadata for a song.
// This replaces MoodAnalysis and OriginalYearAnalysis for batch efficiency.
type UnifiedMetadata struct {
	Genres       []string `json:"genres"`        // Genre classifications for the song
	Mood         string   `json:"mood"`          // Emotional quality: happy, sad, energetic, calm, melancholic, etc.
	Energy       string   `json:"energy"`        // Energy level: high, medium, low
	Tempo        string   `json:"tempo"`         // Perceived tempo: fast, medium, slow
	BPM          int      `json:"bpm"`           // Estimated beats per minute (0 if unknown)
	Instrumental bool     `json:"instrumental"`  // true if song has no vocals
	OriginalYear int      `json:"original_year"` // Original release year (not remaster date)
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
		if bpm, err := strconv.Atoi(bpmStr); err == nil {
			metadata.BPM = bpm
		}
	}

	// Parse instrumental (boolean)
	instrStr := strings.ToLower(strings.TrimSpace(parts[6]))
	metadata.Instrumental = instrStr == "true" || instrStr == "1" || instrStr == "yes"

	// Parse original year (integer)
	yearStr := strings.TrimSpace(parts[7])
	if yearStr != "" && yearStr != "0" {
		if year, err := strconv.Atoi(yearStr); err == nil {
			metadata.OriginalYear = year
		}
	}

	return id, metadata, nil
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
func (c *Client) EnrichAllMetadata(songs []db.Song) (map[string]*UnifiedMetadata, error) {
	if len(songs) == 0 {
		return nil, nil
	}

	logger.Gemini("EnrichAllMetadata: Starting with %d songs", len(songs))

	var promptBuilder strings.Builder
	promptBuilder.WriteString(`You are a music expert with deep knowledge of artists, genres, and music history.

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

SONGS TO ANALYZE:
`)

	// Write input in TOON format
	for _, song := range songs {
		promptBuilder.WriteString(fmt.Sprintf("%s|%s|%s|%s|%d\n",
			song.ID, song.Artist, song.Title, song.Album, song.Year))
	}

	prompt := promptBuilder.String()
	logger.Gemini("EnrichAllMetadata: Prompt length: %d chars", len(prompt))

	reqBody := generateContentRequest{
		Contents: []content{
			{
				Parts: []part{
					{Text: prompt},
				},
			},
		},
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	logger.Gemini("EnrichAllMetadata: Request body size: %d bytes, calling Gemini API...", len(jsonData))

	result := make(map[string]*UnifiedMetadata)
	err = c.doWithRetry(func() error {
		url := fmt.Sprintf("%s?key=%s", apiEndpoint, c.apiKey)
		logger.Gemini("EnrichAllMetadata: Making HTTP POST to Gemini...")

		resp, err := http.Post(url, "application/json", bytes.NewBuffer(jsonData))
		if err != nil {
			logger.Gemini("EnrichAllMetadata: HTTP error: %v", err)
			return fmt.Errorf("failed to call Gemini API: %w", err)
		}
		defer resp.Body.Close()

		logger.Gemini("EnrichAllMetadata: Response status: %d", resp.StatusCode)

		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			body, _ := io.ReadAll(resp.Body)
			logger.Gemini("EnrichAllMetadata: Retriable error: %s", string(body))
			return &retriableError{fmt.Errorf("gemini API error (status %d): %s", resp.StatusCode, string(body))}
		}

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			logger.Gemini("EnrichAllMetadata: Non-OK status: %s", string(body))
			return fmt.Errorf("gemini API error (status %d): %s", resp.StatusCode, string(body))
		}

		var geminiResp generateContentResponse
		if err := json.NewDecoder(resp.Body).Decode(&geminiResp); err != nil {
			logger.Gemini("EnrichAllMetadata: JSON decode error: %v", err)
			return fmt.Errorf("failed to decode response: %w", err)
		}

		if len(geminiResp.Candidates) == 0 || len(geminiResp.Candidates[0].Content.Parts) == 0 {
			logger.Gemini("EnrichAllMetadata: No content in response")
			return fmt.Errorf("no content in response")
		}

		responseText := geminiResp.Candidates[0].Content.Parts[0].Text
		logger.Gemini("EnrichAllMetadata: Response length: %d chars", len(responseText))

		// Log first 500 chars of response for debugging
		preview := responseText
		if len(preview) > 500 {
			preview = preview[:500] + "..."
		}
		logger.Gemini("EnrichAllMetadata: Response preview: %s", preview)

		// Remove any markdown formatting
		responseText = strings.TrimPrefix(responseText, "```")
		responseText = strings.TrimSuffix(responseText, "```")
		responseText = strings.TrimSpace(responseText)

		// Parse TOON response line by line
		lines := strings.Split(responseText, "\n")
		logger.Gemini("EnrichAllMetadata: Parsing %d lines", len(lines))

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
					logger.Gemini("EnrichAllMetadata: Parse error on line: %s - %v", line, err)
				}
				continue
			}

			result[id] = metadata
		}

		logger.Gemini("EnrichAllMetadata: Parsed %d songs successfully, %d parse errors", len(result), parseErrors)

		// Verify we got results for at least some songs
		if len(result) == 0 && len(songs) > 0 {
			return fmt.Errorf("failed to parse TOON response: no valid entries. Response: %s", responseText)
		}

		return nil
	})

	if err != nil {
		logger.Gemini("EnrichAllMetadata: Final error: %v", err)
		return nil, err
	}

	logger.Gemini("EnrichAllMetadata: Complete - returned %d results", len(result))
	return result, nil
}

type PlaylistFilter struct {
	Genres      []string `json:"genres"`
	Artists     []string `json:"artists"`
	MinYear     int      `json:"minYear"`
	MaxYear     int      `json:"maxYear"`
	Description string   `json:"description"`
	// Enhanced mood/energy fields
	Mood         string `json:"mood,omitempty"`         // e.g., "happy", "sad", "energetic", "chill", "romantic"
	Energy       string `json:"energy,omitempty"`       // "low", "medium", "high"
	Tempo        string `json:"tempo,omitempty"`        // "slow", "medium", "fast"
	Occasion     string `json:"occasion,omitempty"`     // "workout", "study", "party", "relaxation", "driving"
	Instrumental bool   `json:"instrumental,omitempty"` // true if user wants instrumental only
	FromCache    bool   `json:"fromCache,omitempty"`    // indicates if result was from cache
}

// GeneratePlaylistFilter converts a natural language prompt into a structured filter.
// Uses caching to avoid duplicate API calls for similar prompts.
func (c *Client) GeneratePlaylistFilter(prompt string) (*PlaylistFilter, error) {
	// Check cache first
	if cached, ok := c.filterCache.get(prompt); ok {
		cachedCopy := *cached
		cachedCopy.FromCache = true
		return &cachedCopy, nil
	}

	systemPrompt := `You are a music expert. Convert the user's natural language playlist request into a structured JSON filter.
	
	OUTPUT FORMAT:
	{
		"genres": ["genre1", "genre2"], // List of relevant genres (e.g., "Pop", "Rock", "Synthwave")
		"artists": ["artist1"], // Specific artists if mentioned, otherwise empty
		"minYear": 1980, // Start year if a decade/era is mentioned (e.g., 1980 for "80s")
		"maxYear": 1989, // End year if a decade/era is mentioned
		"description": "A short description of the playlist vibe",
		"mood": "energetic", // One of: "happy", "sad", "energetic", "chill", "romantic", "melancholic", "aggressive", "peaceful"
		"energy": "high", // One of: "low", "medium", "high"
		"tempo": "fast", // One of: "slow", "medium", "fast"
		"occasion": "workout", // One of: "workout", "study", "party", "relaxation", "driving", "sleep", "focus", "dinner"
		"instrumental": false // true if user explicitly wants instrumental/no vocals
	}

	RULES:
	- If no specific era is mentioned, set minYear and maxYear to 0.
	- Infer genres AND mood/energy from descriptions (e.g., "Upbeat" -> energy: "high", mood: "happy").
	- Infer occasion from context (e.g., "for running" -> occasion: "workout").
	- Map tempo from energy hints (e.g., "pump up" -> tempo: "fast").
	- Return ONLY valid JSON. No markdown.`

	reqBody := generateContentRequest{
		Contents: []content{
			{
				Parts: []part{
					{Text: systemPrompt + "\n\nUser Request: " + prompt},
				},
			},
		},
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	var filter PlaylistFilter
	err = c.doWithRetry(func() error {
		url := fmt.Sprintf("%s?key=%s", apiEndpoint, c.apiKey)
		resp, err := http.Post(url, "application/json", bytes.NewBuffer(jsonData))
		if err != nil {
			return fmt.Errorf("failed to call Gemini API: %w", err)
		}
		defer resp.Body.Close()

		// Check for rate limiting or server errors (retriable)
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			body, _ := io.ReadAll(resp.Body)
			return &retriableError{fmt.Errorf("gemini API error (status %d): %s", resp.StatusCode, string(body))}
		}

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("gemini API error (status %d): %s", resp.StatusCode, string(body))
		}

		var geminiResp generateContentResponse
		if err := json.NewDecoder(resp.Body).Decode(&geminiResp); err != nil {
			return fmt.Errorf("failed to decode response: %w", err)
		}

		if len(geminiResp.Candidates) == 0 || len(geminiResp.Candidates[0].Content.Parts) == 0 {
			return fmt.Errorf("no content in response")
		}

		responseText := geminiResp.Candidates[0].Content.Parts[0].Text
		responseText = strings.TrimPrefix(responseText, "```json")
		responseText = strings.TrimPrefix(responseText, "```")
		responseText = strings.TrimSuffix(responseText, "```")
		responseText = strings.TrimSpace(responseText)

		if err := json.Unmarshal([]byte(responseText), &filter); err != nil {
			return fmt.Errorf("failed to parse JSON response: %w", err)
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	// Cache the result
	c.filterCache.set(prompt, &filter)

	return &filter, nil
}

// MoodAnalysis represents the AI-detected mood, energy, tempo, and BPM for a song.
// These values are inferred from song metadata (artist, title, album, genre) using
// Gemini's knowledge of music styles and conventions.
type MoodAnalysis struct {
	Mood         string `json:"mood"`         // Emotional quality: happy, sad, energetic, calm, melancholic, uplifting, aggressive, romantic, chill, intense, dreamy, nostalgic
	Energy       string `json:"energy"`       // Energy level: high, medium, low
	Tempo        string `json:"tempo"`        // Perceived tempo: fast, medium, slow
	BPM          int    `json:"bpm"`          // Estimated beats per minute (0 if unknown)
	Instrumental bool   `json:"instrumental"` // true if song has no vocals (instrumental only)
}

// AnalyzeSongMood uses Gemini AI to analyze the mood, energy, tempo, and BPM of songs.
// This is a legacy wrapper that calls EnrichAllMetadata and extracts mood-related fields.
func (c *Client) AnalyzeSongMood(songs []db.Song) (map[string]*MoodAnalysis, error) {
	if len(songs) == 0 {
		return nil, nil
	}

	// Call unified enrichment
	unified, err := c.EnrichAllMetadata(songs)
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

// OriginalYearAnalysis represents the AI-determined original release year for a song.
type OriginalYearAnalysis struct {
	OriginalYear int  `json:"original_year"` // The original release year (0 if unknown)
	Confident    bool `json:"confident"`     // true if AI is confident in the year
}

// AnalyzeOriginalYear uses Gemini AI to determine the original release year of songs.
// This is a legacy wrapper that calls EnrichAllMetadata and extracts original year.
func (c *Client) AnalyzeOriginalYear(songs []db.Song) (map[string]*OriginalYearAnalysis, error) {
	if len(songs) == 0 {
		return nil, nil
	}

	// Call unified enrichment
	unified, err := c.EnrichAllMetadata(songs)
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
