// Package gemini provides integration with Google's Gemini AI for ViiB MediaHub.
//
// This package handles all AI-powered features including:
//
// Playlist Filter Generation:
//   - GeneratePlaylistFilter: Parses natural language prompts into structured filters
//   - Extracts genres, years, mood, energy, tempo from user requests
//   - Uses caching to avoid duplicate API calls for similar prompts
//
// Genre Enrichment:
//   - EnrichGenres: Analyzes songs to suggest detailed genre classifications
//   - Uses song metadata (artist, title, album) to infer genres
//   - Supports batch processing for efficiency
//
// Mood/Energy Analysis:
//   - AnalyzeSongMood: Detects mood, energy, tempo, and estimated BPM
//   - Based on metadata analysis (no audio processing required)
//   - Leverages Gemini's knowledge of artist styles and genre conventions
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
	"strings"
	"sync"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/db"
)

const apiEndpoint = "https://generativelanguage.googleapis.com/v1beta/models/gemini-2.5-flash-preview-09-2025:generateContent"

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
func (c *Client) EnrichGenres(songs []db.Song) (map[string][]string, error) {
	if len(songs) == 0 {
		return nil, nil
	}

	var promptBuilder strings.Builder
	promptBuilder.WriteString(`You are a music expert with deep knowledge of artists, genres, and subgenres.

Your task is to identify the most accurate genres for each song, taking into account:

1. Song-specific characteristics (sound, instrumentation, production era, stylistic elements).
2. The band's or artist's overall established genres, which must be reflected in the song's genre list unless clearly contradicted by the actual sound.

OUTPUT REQUIREMENTS:
- Return ONLY a valid JSON object.
- Keys: Song IDs exactly as provided.
- Values: Arrays of genre strings.
- Genres should be ordered from most specific (e.g., "Dream Pop") to broader parent genres (e.g., "Indie Rock").
- Do not include any explanations, comments, markdown, or code blocks — JSON only.

GENRE SELECTION RULES:
- Use real, widely recognized genres.
- Prefer specific subgenres when strongly supported.
- Include broader genres only as parents or fallback categories.
- If uncertain, choose the most widely accepted critical consensus for the song or artist.

DECADE LABELING RULES:
- Include an additional decade-based genre tag when clearly supported by the song’s release era and stylistic traits (e.g., "90s Alternative", "80s Pop", "2000s Hip Hop").
- Decade tags should appear after the specific subgenres but before broad parent genres.
- Only apply decade labels when they meaningfully describe the song's stylistic or cultural placement—not solely its release date.
- Use the format: "[Decade] [Genre]" (e.g., "80s Synthpop", "90s Grunge")`)

	for _, song := range songs {
		promptBuilder.WriteString(fmt.Sprintf("ID: %s | Artist: %s | Title: %s | Album: %s\n", song.ID, song.Artist, song.Title, song.Album))
	}

	reqBody := generateContentRequest{
		Contents: []content{
			{
				Parts: []part{
					{Text: promptBuilder.String()},
				},
			},
		},
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Use retry logic for API call
	var result map[string][]string
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

		// Clean up and repair JSON response
		responseText = cleanAndRepairJSON(responseText)

		if err := json.Unmarshal([]byte(responseText), &result); err != nil {
			return fmt.Errorf("failed to parse JSON response from Gemini: %w. Response was: %s", err, responseText)
		}

		return nil
	})

	if err != nil {
		return nil, err
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
// Unlike audio-based analysis, this uses metadata (artist, title, album, genre) to infer
// musical characteristics based on Gemini's knowledge of artists, genres, and conventions.
//
// This approach was chosen over local audio analysis because:
//   - Gemini understands genre conventions (e.g., death metal = high energy, ~180 BPM)
//   - Artist reputation provides context (e.g., Radiohead = often melancholic)
//   - No complex DSP/FFT processing required
//   - Works without actual audio file access
//
// Returns a map of song ID to MoodAnalysis, allowing batch updates to the database.
func (c *Client) AnalyzeSongMood(songs []db.Song) (map[string]*MoodAnalysis, error) {
	if len(songs) == 0 {
		return nil, nil
	}

	var promptBuilder strings.Builder
	promptBuilder.WriteString(`You are a music expert. Analyze the mood, energy level, tempo, and vocal presence of each song based on the artist, title, album, and genre.

OUTPUT REQUIREMENTS:
- Return ONLY a valid JSON object.
- Keys: Song IDs exactly as provided.
- Values: Objects with these fields:
  - "mood": One of: "happy", "sad", "energetic", "calm", "melancholic", "uplifting", "aggressive", "romantic", "chill", "intense", "dreamy", "nostalgic"
  - "energy": One of: "high", "medium", "low"
  - "tempo": One of: "fast", "medium", "slow"
  - "bpm": Estimated BPM as integer (use 0 if truly unknown, but make your best estimate based on genre/style)
  - "instrumental": true if song has no vocals (instrumental only), false if it has vocals

ANALYSIS GUIDELINES:
- Consider the artist's typical sound and style
- Consider the genre's typical characteristics
- Consider the song title's emotional implications
- For unknown songs, use genre conventions as guidance
- Most songs have vocals (instrumental: false) unless explicitly instrumental, classical, or electronic/ambient

Example output:
{
  "song-id-1": {"mood": "energetic", "energy": "high", "tempo": "fast", "bpm": 140, "instrumental": false},
  "song-id-2": {"mood": "melancholic", "energy": "low", "tempo": "slow", "bpm": 70, "instrumental": true}
}

Songs to analyze:
`)

	for _, song := range songs {
		genres := strings.Join(song.Genre, ", ")
		if genres == "" {
			genres = "Unknown"
		}
		promptBuilder.WriteString(fmt.Sprintf("ID: %s | Artist: %s | Title: %s | Album: %s | Genre: %s\n",
			song.ID, song.Artist, song.Title, song.Album, genres))
	}

	reqBody := generateContentRequest{
		Contents: []content{
			{
				Parts: []part{
					{Text: promptBuilder.String()},
				},
			},
		},
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	var result map[string]*MoodAnalysis
	err = c.doWithRetry(func() error {
		url := fmt.Sprintf("%s?key=%s", apiEndpoint, c.apiKey)
		resp, err := http.Post(url, "application/json", bytes.NewBuffer(jsonData))
		if err != nil {
			return fmt.Errorf("failed to call Gemini API: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			body, _ := io.ReadAll(resp.Body)
			return &retriableError{fmt.Errorf("gemini API error (status %d): %s", resp.StatusCode, string(body))}
		}

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("gemini API error: %s", string(body))
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

		if err := json.Unmarshal([]byte(responseText), &result); err != nil {
			return fmt.Errorf("failed to parse JSON response: %w", err)
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return result, nil
}
