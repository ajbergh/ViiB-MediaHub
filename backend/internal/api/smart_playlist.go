// Package api provides HTTP handlers for the ViiB MediaHub API.
//
// smart_playlist.go implements the AI DJ feature, which generates smart playlists
// based on natural language prompts. It uses a four-tier matching system:
//
//  1. Tier 0 - Artist-Based Matching: Detects "more like [artist]" patterns and
//     returns songs from that artist plus similar artists (based on shared genres).
//
//  2. Tier 1 - Local Genre Matching: Direct match against indexed genre names
//     without calling external APIs. Handles exact and partial matches.
//
//  3. Tier 1.5 - Mood/Activity Keyword Matching: Intercepts 85+ common mood/vibe
//     keywords (chill, workout, focus, party, etc.) and queries by mood/energy/tempo
//     directly, bypassing the LLM API entirely.
//
//  4. Tier 2 - LLM Fallback: For complex prompts, uses the configured LLM provider
//     (Ollama, Gemini, OpenAI, Anthropic, or X.AI) to parse intent and smart-score
//     indexed genres. Provider is selected from user settings.
//
// The AI DJ always uses multi-genre blending to create cross-genre playlists based
// on the user's input query. The backend intelligently selects the best matching
// genres and blends them proportionally for a cohesive listening experience.
//
// Additional features include:
//   - Multi-genre blending with proportional song selection
//   - Play history integration (discover mode, avoid recently played)
//   - Time-of-day awareness for contextual recommendations
//   - Decade extraction from prompts (supports early/mid/late qualifiers)
//   - True random shuffling using Fisher-Yates algorithm (seeded at init)
//   - Mood/energy/tempo filtering in database queries
//
// Key functions:
//   - handleGenerateSmartPlaylist: Main HTTP handler for /api/smart-playlist
//   - tryArtistBasedMatch: Detects artist-based prompts (Tier 0)
//   - tryLocalGenreMatch: Matches against indexed genres locally (Tier 1)
//   - tryMoodBasedMatch: Intercepts mood/activity keywords (Tier 1.5)
//   - tryMatchMultipleGenres: Returns top matching genres for blending
//   - scoreGenreMatch: Calculates genre match scores
//   - applyPlayHistoryFilters: Filters based on play history preferences
//   - getTimeContext: Returns current time context for recommendations
package api

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/db"
	"github.com/ajbergh/viib-mediahub/internal/dj"
	"github.com/ajbergh/viib-mediahub/internal/llm"
	"github.com/ajbergh/viib-mediahub/internal/logger"
)

// LocalPlaylistFilter represents the filter criteria used to generate a playlist.
// It contains all the parameters extracted from the user's prompt or determined
// through local/AI matching, including genres, year range, mood, and blending info.
type LocalPlaylistFilter struct {
	Genres        []string       `json:"genres"`
	Artists       []string       `json:"artists"`
	MinYear       int            `json:"minYear"`
	MaxYear       int            `json:"maxYear"`
	Description   string         `json:"description"`
	Mood          string         `json:"mood,omitempty"`
	Energy        string         `json:"energy,omitempty"`
	Tempo         string         `json:"tempo,omitempty"`
	Occasion      string         `json:"occasion,omitempty"`
	FromCache     bool           `json:"fromCache,omitempty"`
	LocalMatch    bool           `json:"localMatch,omitempty"` // True if matched locally without AI
	BlendMode     string         `json:"blendMode,omitempty"`  // "single" or "mixed" genre mode
	MatchedGenres []MatchedGenre `json:"matchedGenres,omitempty"`
}

// MatchedGenre represents a genre that matched the AI DJ prompt with its score and proportion.
type MatchedGenre struct {
	Name       string  `json:"name"`
	Score      int     `json:"score"`
	SongCount  int     `json:"songCount"`
	Proportion float64 `json:"proportion"` // 0.0-1.0, percentage of playlist from this genre
}

// transformSongsForAPI converts raw database songs to have API URLs for FilePath and CoverPath.
// This is required because the frontend expects URLs like /api/audio/{id} and /api/cover/{id},
// but database songs store the actual filesystem paths.
// The function handles both []db.Song and []any (containing db.Song values).
func transformSongsForAPI(songs []any) []any {
	result := make([]any, 0, len(songs))
	for _, s := range songs {
		switch song := s.(type) {
		case db.Song:
			// Transform paths to API URLs
			transformLibrarySongForAPI(&song)
			result = append(result, song)
		case *db.Song:
			// Transform paths to API URLs (pointer case)
			transformLibrarySongForAPI(song)
			result = append(result, *song)
		default:
			// If not a recognized song type, include as-is
			result = append(result, s)
		}
	}
	return result
}

// getLLMSettings retrieves the LLM configuration from the database.
// Falls back to default settings (Ollama) if not configured.
func getLLMSettings(a *API) llm.Settings {
	settings := llm.DefaultSettings()

	if provider, err := a.db.GetSetting("llm_provider"); err == nil && provider != "" {
		settings.Provider = provider
	}
	if model, err := a.db.GetSetting("llm_model"); err == nil && model != "" {
		settings.Model = model
	}
	if apiKey, err := a.db.GetSetting("llm_api_key"); err == nil && apiKey != "" {
		settings.APIKey = apiKey
	}
	if baseURL, err := a.db.GetSetting("llm_base_url"); err == nil && baseURL != "" {
		settings.BaseURL = baseURL
	}

	return settings
}

// getSongsForGenre applies explicit era constraints to every local-genre path.
// An era in a user prompt is a hard constraint, never a ranking hint.
func (a *API) getSongsForGenre(genreName string, minYear, maxYear int) ([]db.Song, error) {
	if minYear > 0 || maxYear > 0 {
		return a.db.GetSongsByExactGenreWithYears(genreName, minYear, maxYear)
	}
	return a.db.GetSongsByExactGenre(genreName)
}

// tryLocalGenreMatch attempts to match the prompt against known local genres.
// Returns matched genre name, songs, and whether a match was found.
//
// IMPORTANT: This only matches if the genre has at least 20 songs to ensure
// a good playlist experience. For small genres, we fall back to LLM blending.
func (a *API) tryLocalGenreMatch(prompt string, minYear, maxYear int) (string, []any, bool) {
	const minSongsForLocalMatch = 20 // Require at least this many songs for direct match

	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return "", nil, false
	}

	// Get all known genre names from the database
	genreNames, err := a.db.GetGenreNames()
	if err != nil {
		logger.API("Failed to get genre names for local matching: %v", err)
		return "", nil, false
	}

	promptLower := strings.ToLower(prompt)

	// First pass: exact match (case-insensitive)
	for _, genreName := range genreNames {
		if strings.ToLower(genreName) == promptLower {
			songs, err := a.getSongsForGenre(genreName, minYear, maxYear)
			if err != nil {
				logger.API("Failed to get songs for genre %s: %v", genreName, err)
				return "", nil, false
			}
			// Only use direct match if we have enough songs
			if len(songs) >= minSongsForLocalMatch {
				// Convert to interface slice for JSON encoding
				result := make([]any, len(songs))
				for i, s := range songs {
					result[i] = s
				}
				logger.API("Local genre exact match: '%s' with %d songs (>= %d min)", genreName, len(songs), minSongsForLocalMatch)
				return genreName, result, true
			}
			logger.API("Local genre exact match '%s' has only %d songs (< %d min), falling back to LLM", genreName, len(songs), minSongsForLocalMatch)
		}
	}

	// Second pass: partial match - ONLY if the genre name is contained in the prompt
	// AND the genre name is significant (more than 50% of the prompt length)
	// This prevents "rock" from matching "90s alt rock" when "90s Alternative" exists
	for _, genreName := range genreNames {
		genreLower := strings.ToLower(genreName)
		// Check if the genre name is substantially contained in the prompt
		// The genre should be at least 60% of the prompt length to avoid false matches
		if strings.Contains(promptLower, genreLower) {
			// Only match if the genre name is a significant portion of the prompt
			if len(genreLower) >= len(promptLower)*60/100 {
				songs, err := a.getSongsForGenre(genreName, minYear, maxYear)
				if err != nil {
					logger.API("Failed to get songs for genre %s: %v", genreName, err)
					continue
				}
				// Only use direct match if we have enough songs
				if len(songs) >= minSongsForLocalMatch {
					result := make([]any, len(songs))
					for i, s := range songs {
						result[i] = s
					}
					logger.API("Local genre partial match: '%s' with %d songs (>= %d min)", genreName, len(songs), minSongsForLocalMatch)
					return genreName, result, true
				}
				logger.API("Local genre partial match '%s' has only %d songs (< %d min), continuing", genreName, len(songs), minSongsForLocalMatch)
			}
		}
		// Or if the prompt is contained in the genre name (e.g., "alternative" matches "90s Alternative")
		if strings.Contains(genreLower, promptLower) {
			songs, err := a.getSongsForGenre(genreName, minYear, maxYear)
			if err != nil {
				logger.API("Failed to get songs for genre %s: %v", genreName, err)
				continue
			}
			// Only use direct match if we have enough songs
			if len(songs) >= minSongsForLocalMatch {
				result := make([]any, len(songs))
				for i, s := range songs {
					result[i] = s
				}
				logger.API("Local genre inverse match: '%s' with %d songs (>= %d min)", genreName, len(songs), minSongsForLocalMatch)
				return genreName, result, true
			}
		}
	}

	return "", nil, false
}

// moodKeywords maps common user keywords to database mood/energy/tempo values.
// This allows the AI DJ to match mood-based prompts without calling Gemini,
// reducing API costs and latency for common activity-based requests.
var moodKeywords = map[string]struct {
	mood   string
	energy string
	tempo  string
}{
	// Relaxation/Calm prompts
	"chill":      {mood: "calm", energy: "low", tempo: "slow"},
	"chillout":   {mood: "calm", energy: "low", tempo: "slow"},
	"relax":      {mood: "calm", energy: "low", tempo: "slow"},
	"relaxing":   {mood: "calm", energy: "low", tempo: "slow"},
	"relaxation": {mood: "calm", energy: "low", tempo: "slow"},
	"calm":       {mood: "calm", energy: "low", tempo: "slow"},
	"peaceful":   {mood: "peaceful", energy: "low", tempo: "slow"},
	"sleep":      {mood: "calm", energy: "low", tempo: "slow"},
	"sleeping":   {mood: "calm", energy: "low", tempo: "slow"},
	"bedtime":    {mood: "calm", energy: "low", tempo: "slow"},
	"meditation": {mood: "peaceful", energy: "low", tempo: "slow"},
	"zen":        {mood: "peaceful", energy: "low", tempo: "slow"},
	"ambient":    {mood: "calm", energy: "low", tempo: "slow"},
	"mellow":     {mood: "calm", energy: "low", tempo: "medium"},
	"soft":       {mood: "calm", energy: "low", tempo: "slow"},
	"gentle":     {mood: "calm", energy: "low", tempo: "slow"},

	// Focus/Study prompts
	"focus":         {mood: "peaceful", energy: "medium", tempo: "medium"},
	"study":         {mood: "calm", energy: "low", tempo: "medium"},
	"studying":      {mood: "calm", energy: "low", tempo: "medium"},
	"concentration": {mood: "peaceful", energy: "medium", tempo: "medium"},
	"work":          {mood: "peaceful", energy: "medium", tempo: "medium"},
	"coding":        {mood: "calm", energy: "medium", tempo: "medium"},

	// Energy/Workout prompts
	"workout":   {mood: "energetic", energy: "high", tempo: "fast"},
	"exercise":  {mood: "energetic", energy: "high", tempo: "fast"},
	"gym":       {mood: "energetic", energy: "high", tempo: "fast"},
	"running":   {mood: "energetic", energy: "high", tempo: "fast"},
	"run":       {mood: "energetic", energy: "high", tempo: "fast"},
	"pump":      {mood: "energetic", energy: "high", tempo: "fast"},
	"pump up":   {mood: "energetic", energy: "high", tempo: "fast"},
	"energetic": {mood: "energetic", energy: "high", tempo: "fast"},
	"energy":    {mood: "energetic", energy: "high", tempo: "fast"},
	"upbeat":    {mood: "happy", energy: "high", tempo: "fast"},
	"hype":      {mood: "energetic", energy: "high", tempo: "fast"},

	// Happy/Party prompts
	"happy":       {mood: "happy", energy: "high", tempo: "medium"},
	"party":       {mood: "happy", energy: "high", tempo: "fast"},
	"dance":       {mood: "happy", energy: "high", tempo: "fast"},
	"dancing":     {mood: "happy", energy: "high", tempo: "fast"},
	"fun":         {mood: "happy", energy: "high", tempo: "fast"},
	"celebration": {mood: "happy", energy: "high", tempo: "fast"},
	"celebrate":   {mood: "happy", energy: "high", tempo: "fast"},

	// Sad/Melancholic prompts
	"sad":         {mood: "sad", energy: "low", tempo: "slow"},
	"melancholy":  {mood: "melancholic", energy: "low", tempo: "slow"},
	"melancholic": {mood: "melancholic", energy: "low", tempo: "slow"},
	"emotional":   {mood: "sad", energy: "low", tempo: "slow"},
	"cry":         {mood: "sad", energy: "low", tempo: "slow"},
	"heartbreak":  {mood: "sad", energy: "low", tempo: "slow"},
	"breakup":     {mood: "sad", energy: "low", tempo: "slow"},

	// Romantic prompts
	"romantic":   {mood: "romantic", energy: "low", tempo: "slow"},
	"love":       {mood: "romantic", energy: "low", tempo: "slow"},
	"dinner":     {mood: "romantic", energy: "low", tempo: "slow"},
	"date":       {mood: "romantic", energy: "low", tempo: "slow"},
	"date night": {mood: "romantic", energy: "low", tempo: "slow"},

	// Aggressive/Intense prompts
	"aggressive": {mood: "aggressive", energy: "high", tempo: "fast"},
	"angry":      {mood: "aggressive", energy: "high", tempo: "fast"},
	"intense":    {mood: "intense", energy: "high", tempo: "fast"},
	"metal":      {mood: "aggressive", energy: "high", tempo: "fast"},

	// Driving prompts
	"driving":   {mood: "energetic", energy: "medium", tempo: "medium"},
	"road trip": {mood: "happy", energy: "medium", tempo: "medium"},
	"roadtrip":  {mood: "happy", energy: "medium", tempo: "medium"},

	// Nostalgic/Dreamy prompts
	"nostalgic": {mood: "nostalgic", energy: "low", tempo: "medium"},
	"nostalgia": {mood: "nostalgic", energy: "low", tempo: "medium"},
	"dreamy":    {mood: "dreamy", energy: "low", tempo: "slow"},

	// Morning/Evening prompts
	"morning": {mood: "uplifting", energy: "medium", tempo: "medium"},
	"wake up": {mood: "uplifting", energy: "medium", tempo: "medium"},
	"evening": {mood: "calm", energy: "low", tempo: "slow"},
}

// moodMatchInfo holds extracted mood/energy/tempo from a prompt without querying songs.
// Used by DJ mode to inform phase filtering.
type moodMatchInfo struct {
	mood   string
	energy string
	tempo  string
}

// tryMoodBasedMatchInfo extracts mood/energy/tempo hints from the prompt without querying songs.
// Returns nil if no mood keywords are found.
func (a *API) tryMoodBasedMatchInfo(prompt string) *moodMatchInfo {
	promptLower := strings.ToLower(strings.TrimSpace(prompt))
	if promptLower == "" {
		return nil
	}

	// Check for mood keywords in the prompt (allow longer prompts for DJ mode)
	for keyword, attrs := range moodKeywords {
		// Check if keyword appears as a word boundary in the prompt
		if strings.Contains(" "+promptLower+" ", " "+keyword+" ") ||
			strings.HasPrefix(promptLower, keyword+" ") ||
			strings.HasSuffix(promptLower, " "+keyword) ||
			promptLower == keyword {
			return &moodMatchInfo{
				mood:   attrs.mood,
				energy: attrs.energy,
				tempo:  attrs.tempo,
			}
		}
	}

	return nil
}

// tryMoodBasedMatch attempts to match the prompt against mood/activity keywords.
// This avoids Gemini API calls for common vibe-based requests like "chill music"
// or "workout playlist". Returns filter, songs, and whether a match was found.
//
// This is Tier 1.5 in the matching hierarchy:
//   - Tier 0: Artist-based matching
//   - Tier 1: Local genre matching
//   - Tier 1.5: Mood/activity keyword matching (this function)
//   - Tier 2: Gemini AI fallback
func (a *API) tryMoodBasedMatch(prompt string) (*LocalPlaylistFilter, []any, bool) {
	promptLower := strings.ToLower(strings.TrimSpace(prompt))
	if promptLower == "" {
		return nil, nil, false
	}

	// Count words in the prompt
	words := strings.Fields(promptLower)
	wordCount := len(words)

	// Only use mood-based matching for simple prompts (1-2 words)
	// This prevents "upbeat 90s hip hop" from matching just "upbeat"
	// Complex prompts should go to Gemini for full parsing
	if wordCount > 2 {
		return nil, nil, false
	}

	// Check for mood keywords in the prompt
	var matchedMood, matchedEnergy, matchedTempo string
	var matchedKeyword string

	for keyword, attrs := range moodKeywords {
		// Check if keyword appears as a word boundary in the prompt
		// This prevents "focus" matching "unfocused" etc.
		if strings.Contains(" "+promptLower+" ", " "+keyword+" ") ||
			strings.HasPrefix(promptLower, keyword+" ") ||
			strings.HasSuffix(promptLower, " "+keyword) ||
			promptLower == keyword {
			matchedMood = attrs.mood
			matchedEnergy = attrs.energy
			matchedTempo = attrs.tempo
			matchedKeyword = keyword
			break
		}
	}

	if matchedMood == "" {
		return nil, nil, false
	}

	logger.API("Mood keyword match: '%s' → mood=%s, energy=%s, tempo=%s",
		matchedKeyword, matchedMood, matchedEnergy, matchedTempo)

	// Query songs by mood (using empty strings for genres/artists lets the mood filter take over)
	songs, err := a.db.GetSongsBySmartFilter(nil, nil, 0, 0, matchedMood, matchedEnergy, matchedTempo)
	if err != nil {
		logger.API("Failed to get songs by mood filter: %v", err)
		return nil, nil, false
	}

	if len(songs) == 0 {
		// No songs with this exact mood - try with just mood, ignoring energy/tempo
		songs, err = a.db.GetSongsBySmartFilter(nil, nil, 0, 0, matchedMood, "", "")
		if err != nil || len(songs) == 0 {
			logger.API("No songs found for mood '%s'", matchedMood)
			return nil, nil, false
		}
	}

	logger.API("Mood-based match found: %d songs for '%s' vibe", len(songs), matchedKeyword)

	// Convert to interface slice
	result := make([]any, len(songs))
	for i, s := range songs {
		result[i] = s
	}

	filter := &LocalPlaylistFilter{
		Mood:        matchedMood,
		Energy:      matchedEnergy,
		Tempo:       matchedTempo,
		Description: fmt.Sprintf("%s music - %s vibes", matchedKeyword, matchedMood),
		LocalMatch:  true,
		BlendMode:   "single",
	}

	return filter, result, true
}

// tryArtistBasedMatch detects "more like [artist]" or similar patterns and returns songs
// from that artist plus similar artists.
func (a *API) tryArtistBasedMatch(prompt string) (string, []any, bool) {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return "", nil, false
	}

	// Detect patterns like "more like <artist>", "songs like <artist>", "similar to <artist>"
	artistPatterns := []string{
		`more like (.+)`,
		`songs? like (.+)`,
		`similar to (.+)`,
		`music like (.+)`,
		`artists? like (.+)`,
		`play (.+) style`,
		`(.+) style music`,
	}

	var artistName string
	for _, pattern := range artistPatterns {
		re := regexp.MustCompile(`(?i)` + pattern)
		if matches := re.FindStringSubmatch(prompt); len(matches) > 1 {
			artistName = strings.TrimSpace(matches[1])
			break
		}
	}

	if artistName == "" {
		return "", nil, false
	}

	logger.API("Detected artist-based prompt for: %s", artistName)

	// Get songs by this artist
	artistSongs, err := a.db.GetSongsByArtist(artistName)
	if err != nil {
		logger.API("Failed to get songs by artist %s: %v", artistName, err)
		return "", nil, false
	}

	if len(artistSongs) == 0 {
		logger.API("No songs found for artist: %s", artistName)
		return "", nil, false
	}

	// Get similar artists
	similarArtists, err := a.db.GetSimilarArtists(artistName, 10)
	if err != nil {
		logger.API("Failed to get similar artists: %v", err)
	}

	// Collect songs from similar artists
	allSongs := make([]any, 0, len(artistSongs)*3)

	// Add all songs from the target artist first
	for _, s := range artistSongs {
		allSongs = append(allSongs, s)
	}

	// Add songs from similar artists
	for _, simArtist := range similarArtists {
		simSongs, err := a.db.GetSongsByArtist(simArtist)
		if err != nil {
			continue
		}
		for _, s := range simSongs {
			allSongs = append(allSongs, s)
		}
	}

	logger.API("Artist-based match: %d songs from %s, %d similar artists", len(artistSongs), artistName, len(similarArtists))

	return artistName, allSongs, true
}

// getArtistAffinityBonus returns a score bonus for songs by frequently played artists.
//
//lint:ignore U1000 Retained for the next recommendation-scoring iteration.
func (a *API) getArtistAffinityBonus(artistName string, artistStats map[string]int) int {
	if artistStats == nil {
		return 0
	}
	totalPlays, ok := artistStats[artistName]
	if !ok {
		return 0
	}

	// Calculate bonus based on play count tier
	switch {
	case totalPlays >= 50:
		return 15 // Heavily played artist
	case totalPlays >= 20:
		return 10 // Frequently played
	case totalPlays >= 5:
		return 5 // Occasionally played
	default:
		return 0
	}
}

// TimeContext represents the current time context for playlist generation.
type TimeContext struct {
	Period          string // "morning", "afternoon", "evening", "night"
	IsWeekend       bool
	Hour            int
	Description     string
	SuggestedMood   string
	SuggestedEnergy string
}

// getTimeContext returns contextual information about the current time.
func getTimeContext() TimeContext {
	now := time.Now()
	hour := now.Hour()
	weekday := now.Weekday()
	isWeekend := weekday == time.Saturday || weekday == time.Sunday

	ctx := TimeContext{
		Hour:      hour,
		IsWeekend: isWeekend,
	}

	switch {
	case hour >= 5 && hour < 10:
		ctx.Period = "morning"
		ctx.Description = "early morning"
		ctx.SuggestedMood = "uplifting"
		ctx.SuggestedEnergy = "medium"
	case hour >= 10 && hour < 12:
		ctx.Period = "morning"
		ctx.Description = "late morning"
		ctx.SuggestedMood = "focused"
		ctx.SuggestedEnergy = "medium-high"
	case hour >= 12 && hour < 14:
		ctx.Period = "afternoon"
		ctx.Description = "midday"
		ctx.SuggestedMood = "energetic"
		ctx.SuggestedEnergy = "high"
	case hour >= 14 && hour < 17:
		ctx.Period = "afternoon"
		ctx.Description = "afternoon"
		ctx.SuggestedMood = "productive"
		ctx.SuggestedEnergy = "medium-high"
	case hour >= 17 && hour < 20:
		ctx.Period = "evening"
		ctx.Description = "evening"
		ctx.SuggestedMood = "relaxing"
		ctx.SuggestedEnergy = "medium"
	case hour >= 20 && hour < 23:
		ctx.Period = "evening"
		ctx.Description = "late evening"
		ctx.SuggestedMood = "mellow"
		ctx.SuggestedEnergy = "low"
	default:
		ctx.Period = "night"
		ctx.Description = "late night"
		ctx.SuggestedMood = "chill"
		ctx.SuggestedEnergy = "low"
	}

	// Weekend adjustments - more relaxed/party vibes
	if isWeekend {
		if ctx.Period == "evening" || ctx.Period == "night" {
			ctx.SuggestedMood = "party"
			ctx.SuggestedEnergy = "high"
		} else if ctx.Period == "morning" {
			ctx.SuggestedMood = "relaxed"
			ctx.SuggestedEnergy = "low-medium"
		}
	}

	return ctx
}

// enhancePromptWithTimeContext adds time-of-day context hints to the prompt if useTimeContext is true.
func enhancePromptWithTimeContext(prompt string, useTimeContext bool) string {
	if !useTimeContext {
		return prompt
	}

	// Check if prompt already mentions time context
	promptLower := strings.ToLower(prompt)
	timeKeywords := []string{"morning", "afternoon", "evening", "night", "late", "early", "weekend", "party", "workout", "sleep", "focus", "work"}
	for _, kw := range timeKeywords {
		if strings.Contains(promptLower, kw) {
			// User already specified time context, don't override
			return prompt
		}
	}

	ctx := getTimeContext()
	weekendStr := ""
	if ctx.IsWeekend {
		weekendStr = " (weekend)"
	}

	// Add subtle time context hint
	enhanced := fmt.Sprintf("%s [Context: %s%s, suggested mood: %s]", prompt, ctx.Description, weekendStr, ctx.SuggestedMood)
	logger.API("Enhanced prompt with time context: %s", enhanced)
	return enhanced
}

// scoreGenreMatch calculates how well an indexed genre matches the LLM filter intent.
// Returns a score from 0-100 where higher is better.
// The originalPrompt parameter allows boosting exact subgenre matches (e.g., "jazz trios" → "Jazz Trio").
func scoreGenreMatch(genreName string, filter *llm.PlaylistFilter, originalPrompt string) int {
	genreLower := strings.ToLower(genreName)
	promptLower := strings.ToLower(originalPrompt)
	score := 0

	// PRIORITY BOOST: Check if the indexed genre name appears as a phrase in the original prompt
	// This handles cases like "upbeat jazz trios" matching "Jazz Trio" directly
	// Apply a high bonus (+60) to prioritize subgenre matches over generic genre matches
	if strings.Contains(promptLower, genreLower) {
		score += 60
		// Even higher bonus for near-exact matches (genre is significant portion of prompt)
		if len(genreLower) >= len(promptLower)/2 {
			score += 20
		}
	}

	// Also check if prompt words form the genre name (e.g., "jazz trio" matches "Jazz Trio")
	// Normalize the genre name by removing hyphens and extra spaces
	genreNormalized := strings.ReplaceAll(genreLower, "-", " ")
	genreNormalized = strings.Join(strings.Fields(genreNormalized), " ")
	if strings.Contains(promptLower, genreNormalized) && genreNormalized != genreLower {
		score += 50
	}

	// Check for decade indicators in genre name
	decadePatterns := map[string][2]int{
		"50s": {1950, 1959}, "1950s": {1950, 1959},
		"60s": {1960, 1969}, "1960s": {1960, 1969},
		"70s": {1970, 1979}, "1970s": {1970, 1979},
		"80s": {1980, 1989}, "1980s": {1980, 1989},
		"90s": {1990, 1999}, "1990s": {1990, 1999},
		"00s": {2000, 2009}, "2000s": {2000, 2009},
		"10s": {2010, 2019}, "2010s": {2010, 2019},
		"20s": {2020, 2029}, "2020s": {2020, 2029},
	}

	// Check if genre name contains decade indicator that matches the filter years
	genreHasDecade := false
	for pattern, years := range decadePatterns {
		if strings.Contains(genreLower, pattern) {
			genreHasDecade = true
			// If filter has year constraints, check if they overlap with this decade
			if filter.MinYear > 0 || filter.MaxYear > 0 {
				minY := filter.MinYear
				maxY := filter.MaxYear
				if minY == 0 {
					minY = 1900
				}
				if maxY == 0 {
					maxY = 2100
				}
				// Check for overlap between filter years and genre decade
				if minY <= years[1] && maxY >= years[0] {
					score += 40 // Strong decade match
				} else {
					score -= 50 // Wrong decade - penalize heavily
				}
			}
			break
		}
	}

	// If filter specifies years but genre has no decade indicator, don't add decade points
	// but also don't penalize

	// Check for genre name matches
	for _, filterGenre := range filter.Genres {
		filterGenreLower := strings.ToLower(filterGenre)
		if strings.Contains(genreLower, filterGenreLower) {
			score += 30 // Genre name match
		}

		// Also check if each word in the filter genre matches the indexed genre
		// This helps "Alternative Rock" match "90s Alternative" since it contains "alternative"
		filterWords := strings.Fields(filterGenreLower)
		for _, word := range filterWords {
			if len(word) > 3 && strings.Contains(genreLower, word) {
				score += 20 // Partial word match
				break       // Only count once
			}
		}

		// Also check common variations/abbreviations
		variations := getGenreVariations(filterGenreLower)
		for _, v := range variations {
			if strings.Contains(genreLower, v) {
				score += 25 // Genre variation match
				break
			}
		}
	}

	// If genre has decade and matches filter, it's likely a curated decade-genre combo
	if genreHasDecade && score > 0 {
		score += 10 // Bonus for being a curated decade-specific genre
	}

	return score
}

// getGenreVariations returns common variations/abbreviations of genre names
func getGenreVariations(genre string) []string {
	variations := map[string][]string{
		"alternative":       {"alt", "indie"},
		"alt":               {"alternative", "indie"},
		"rock":              {"rock"},
		"hip hop":           {"hiphop", "hip-hop", "rap"},
		"hip-hop":           {"hiphop", "hip hop", "rap"},
		"hiphop":            {"hip hop", "hip-hop", "rap"},
		"rap":               {"hip hop", "hip-hop", "hiphop"},
		"electronic":        {"electro", "edm"},
		"r&b":               {"rnb", "r and b", "rhythm and blues"},
		"rnb":               {"r&b", "r and b"},
		"country":           {"country"},
		"metal":             {"metal", "heavy metal"},
		"heavy metal":       {"metal"},
		"pop":               {"pop"},
		"jazz":              {"jazz"},
		"blues":             {"blues"},
		"classical":         {"classical"},
		"punk":              {"punk"},
		"folk":              {"folk"},
		"soul":              {"soul", "motown"},
		"reggae":            {"reggae"},
		"grunge":            {"grunge", "seattle"},
		"psychedelic":       {"psych", "psychedelic"},
		"progressive":       {"prog"},
		"prog":              {"progressive"},
		"indie":             {"indie", "independent"},
		"new wave":          {"newwave", "new-wave"},
		"post-punk":         {"post punk", "postpunk"},
		"shoegaze":          {"shoegaze"},
		"dream pop":         {"dreampop", "dream-pop"},
		"synthwave":         {"synth wave", "synth-wave", "synthpop"},
		"synthpop":          {"synth pop", "synth-pop", "synthwave"},
		"disco":             {"disco"},
		"funk":              {"funk"},
		"ambient":           {"ambient"},
		"hardcore":          {"hardcore"},
		"thrash":            {"thrash"},
		"death metal":       {"deathmetal", "death-metal"},
		"black metal":       {"blackmetal", "black-metal"},
		"doom":              {"doom", "doom metal"},
		"stoner":            {"stoner", "stoner rock"},
		"garage":            {"garage"},
		"brit pop":          {"britpop", "brit-pop"},
		"britpop":           {"brit pop", "brit-pop"},
		"trip hop":          {"triphop", "trip-hop"},
		"triphop":           {"trip hop", "trip-hop"},
		"drum and bass":     {"dnb", "drum n bass", "d&b"},
		"dnb":               {"drum and bass", "drum n bass"},
		"dubstep":           {"dubstep"},
		"house":             {"house"},
		"techno":            {"techno"},
		"trance":            {"trance"},
		"acoustic":          {"acoustic", "unplugged"},
		"singer-songwriter": {"singer songwriter", "singersongwriter"},
	}

	if v, ok := variations[genre]; ok {
		return v
	}
	return []string{}
}

// getAvailableGenreNames returns a list of genre names from the user's library,
// sorted by popularity (most songs first). This is used to help the LLM select
// genres that actually exist in the user's collection.
func (a *API) getAvailableGenreNames() []string {
	stats, err := a.db.GetAllGenreStats()
	if err != nil {
		logger.API("Failed to get genre stats for context: %v", err)
		return nil
	}

	names := make([]string, len(stats))
	for i, stat := range stats {
		names[i] = stat.Name
	}
	return names
}

// truncateSlice returns the first n elements of a slice for logging purposes.
func truncateSlice(slice []string, n int) []string {
	if len(slice) <= n {
		return slice
	}
	return slice[:n]
}

// tryMatchIndexedGenre attempts to find the best indexed genre that matches the LLM filter intent.
// This bridges the gap between the LLM's generic understanding and the user's actual indexed genres.
// The originalPrompt is used to boost exact subgenre matches (e.g., "jazz trios" → "Jazz Trio").
func (a *API) tryMatchIndexedGenre(filter *llm.PlaylistFilter, originalPrompt string) (string, []any, bool) {
	genreNames, err := a.db.GetGenreNames()
	if err != nil {
		logger.API("Failed to get genre names for indexed matching: %v", err)
		return "", nil, false
	}

	// Get genre stats to factor in song counts
	genreStats, err := a.db.GetAllGenreStats()
	if err != nil {
		logger.API("Failed to get genre stats for indexed matching: %v", err)
		// Continue without stats - we'll just use scoring without count bonus
		genreStats = nil
	}

	// Build a map of genre name to song count
	genreCounts := make(map[string]int)
	for _, stat := range genreStats {
		genreCounts[stat.Name] = stat.Count
	}

	type scoredGenre struct {
		name  string
		score int
		count int
	}
	var candidates []scoredGenre

	// Log scoring for debugging
	logger.API("Scoring indexed genres against filter: genres=%v, years=%d-%d, prompt='%s'",
		filter.Genres, filter.MinYear, filter.MaxYear, originalPrompt)

	for _, genreName := range genreNames {
		score := scoreGenreMatch(genreName, filter, originalPrompt)
		if score > 50 { // Only consider genres with meaningful scores
			count := genreCounts[genreName]
			logger.API("  Genre '%s' scored %d (%d songs)", genreName, score, count)
			candidates = append(candidates, scoredGenre{genreName, score, count})
		}
	}

	// Two-pass selection algorithm:
	// 1. Find the highest scoring genre
	// 2. Among genres within 30 points of the highest, prefer the one with the most songs

	// Pass 1: Find highest score
	highestScore := 0
	for _, c := range candidates {
		if c.score > highestScore {
			highestScore = c.score
		}
	}

	// Pass 2: Among genres within 30 points of highest, pick the one with most songs
	bestGenre := ""
	bestScore := 0
	bestCount := 0

	for _, c := range candidates {
		// Only consider genres within 30 points of the highest score
		if c.score >= highestScore-30 {
			// Among competitive candidates, prefer more songs
			if c.count > bestCount {
				bestScore = c.score
				bestGenre = c.name
				bestCount = c.count
			}
		}
	}

	logger.API("Best match: '%s' with score %d and %d songs (highest was %d, threshold: 55)", bestGenre, bestScore, bestCount, highestScore)

	// Only accept matches with a reasonable score threshold
	// A score of 55+ means we matched both decade and genre concept
	if bestScore >= 55 {
		// Use year-filtered query if year range is specified (e.g., "90s hip hop")
		var songs []db.Song
		var err error
		if filter.MinYear > 0 || filter.MaxYear > 0 {
			songs, err = a.db.GetSongsByExactGenreWithYears(bestGenre, filter.MinYear, filter.MaxYear)
		} else {
			songs, err = a.db.GetSongsByExactGenre(bestGenre)
		}
		if err != nil {
			logger.API("Failed to get songs for matched genre %s: %v", bestGenre, err)
			return "", nil, false
		}
		if len(songs) > 0 {
			result := make([]any, len(songs))
			for i, s := range songs {
				result[i] = s
			}
			logger.API("Smart indexed genre match: '%s' with score %d (%d songs, years: %d-%d)", bestGenre, bestScore, len(songs), filter.MinYear, filter.MaxYear)
			return bestGenre, result, true
		}
	}

	return "", nil, false
}

// tryMatchMultipleGenres finds the top matching genres and blends songs proportionally.
// The originalPrompt is used to boost exact subgenre matches (e.g., "jazz trios" → "Jazz Trio").
// When the LLM provides mood/energy/tempo values, they are used to filter songs within each genre.
// Returns matched genres, blended songs, and whether any matches were found.
func (a *API) tryMatchMultipleGenres(filter *llm.PlaylistFilter, originalPrompt string, maxGenres int, targetSongs int) ([]MatchedGenre, []any, bool) {
	genreNames, err := a.db.GetGenreNames()
	if err != nil {
		logger.API("Failed to get genre names for multi-genre matching: %v", err)
		return nil, nil, false
	}

	// Get genre stats to factor in song counts
	genreStats, err := a.db.GetAllGenreStats()
	if err != nil {
		logger.API("Failed to get genre stats for multi-genre matching: %v", err)
		genreStats = nil
	}

	// Build a map of genre name to song count
	genreCounts := make(map[string]int)
	for _, stat := range genreStats {
		genreCounts[stat.Name] = stat.Count
	}

	type scoredGenre struct {
		name  string
		score int
		count int
	}
	var candidates []scoredGenre

	logger.API("Multi-genre scoring against filter: genres=%v, years=%d-%d, mood=%s, energy=%s, tempo=%s, prompt='%s'",
		filter.Genres, filter.MinYear, filter.MaxYear, filter.Mood, filter.Energy, filter.Tempo, originalPrompt)

	for _, genreName := range genreNames {
		score := scoreGenreMatch(genreName, filter, originalPrompt)
		if score > 50 {
			count := genreCounts[genreName]
			logger.API("  Genre '%s' scored %d (%d songs)", genreName, score, count)
			candidates = append(candidates, scoredGenre{genreName, score, count})
		}
	}

	if len(candidates) == 0 {
		return nil, nil, false
	}

	// Sort candidates by score descending, then by song count descending
	// Use simple bubble sort since we have few candidates
	for i := 0; i < len(candidates)-1; i++ {
		for j := i + 1; j < len(candidates); j++ {
			if candidates[j].score > candidates[i].score ||
				(candidates[j].score == candidates[i].score && candidates[j].count > candidates[i].count) {
				candidates[i], candidates[j] = candidates[j], candidates[i]
			}
		}
	}

	// Take top N genres (within 30 points of highest)
	highestScore := candidates[0].score
	var topGenres []scoredGenre

	// Determine the score threshold for including genres
	// Default is within 30 points of the highest score
	scoreThreshold := 30

	// If the top genre has few songs, widen the threshold to include more genres
	// This helps blend "Classic Rock" (11 songs) with "60s Rock" (100 songs) etc.
	if candidates[0].count < 30 {
		scoreThreshold = 60 // Widen threshold to include more related genres
		logger.API("Multi-genre: Top genre '%s' has only %d songs, widening threshold to %d points",
			candidates[0].name, candidates[0].count, scoreThreshold)
	}

	for _, c := range candidates {
		if c.score >= highestScore-scoreThreshold && len(topGenres) < maxGenres {
			topGenres = append(topGenres, c)
		}
	}

	// If we still have only 1 genre with few songs, try to add the next best genre
	// even if it's further below the threshold
	if len(topGenres) == 1 && topGenres[0].count < targetSongs && len(candidates) > 1 {
		// Add the next best genre regardless of score threshold
		topGenres = append(topGenres, candidates[1])
		logger.API("Multi-genre: Forced additional genre '%s' (%d songs) to supplement '%s' (%d songs)",
			candidates[1].name, candidates[1].count, topGenres[0].name, topGenres[0].count)
	}

	// Only proceed if we have good matches (score >= 55)
	if topGenres[0].score < 55 {
		return nil, nil, false
	}

	// Calculate total score for proportional distribution
	totalScore := 0
	for _, g := range topGenres {
		totalScore += g.score
	}

	// Build matched genres with proportions and collect songs
	matchedGenres := make([]MatchedGenre, len(topGenres))
	var allSongs []any

	for i, g := range topGenres {
		proportion := float64(g.score) / float64(totalScore)
		matchedGenres[i] = MatchedGenre{
			Name:       g.name,
			Score:      g.score,
			SongCount:  g.count,
			Proportion: proportion,
		}

		// Calculate how many songs to take from this genre
		songsFromGenre := int(proportion * float64(targetSongs))
		if songsFromGenre < 1 {
			songsFromGenre = 1 // At least 1 song per genre
		}

		// Use GetSongsByExactGenreWithMood to filter by genre AND mood/energy/tempo
		// This enables prompts like "upbeat jazz trios" to filter by both genre and energy
		var songs []db.Song
		var err error

		// Determine mood/energy/tempo to use for filtering
		// Map Gemini's mood values to database mood values if needed
		mood := filter.Mood
		energy := filter.Energy
		tempo := filter.Tempo

		// Use the new function that combines exact genre matching with mood filtering
		songs, err = a.db.GetSongsByExactGenreWithMood(g.name, filter.MinYear, filter.MaxYear, mood, energy, tempo)
		if err != nil {
			logger.API("Failed to get songs for genre %s with mood filter: %v", g.name, err)
			continue
		}

		// If mood filtering returned too few songs, fall back to genre-only query
		if len(songs) < songsFromGenre && (mood != "" || energy != "" || tempo != "") {
			logger.API("Mood filter too restrictive for genre %s (%d songs), falling back to genre-only", g.name, len(songs))
			if filter.MinYear > 0 || filter.MaxYear > 0 {
				songs, err = a.db.GetSongsByExactGenreWithYears(g.name, filter.MinYear, filter.MaxYear)
			} else {
				songs, err = a.db.GetSongsByExactGenre(g.name)
			}
			if err != nil {
				logger.API("Failed to get songs for genre %s: %v", g.name, err)
				continue
			}
		}

		// Shuffle songs for variety using true random (Fisher-Yates algorithm)
		for j := len(songs) - 1; j > 0; j-- {
			k := rand.Intn(j + 1)
			songs[j], songs[k] = songs[k], songs[j]
		}

		// Take proportional amount
		if songsFromGenre > len(songs) {
			songsFromGenre = len(songs)
		}

		for _, s := range songs[:songsFromGenre] {
			allSongs = append(allSongs, s)
		}

		logger.API("  Added %d songs from '%s' (proportion: %.0f%%, years: %d-%d)", songsFromGenre, g.name, proportion*100, filter.MinYear, filter.MaxYear)
	}

	// Shuffle the combined playlist for natural mixing using true random
	for i := len(allSongs) - 1; i > 0; i-- {
		j := rand.Intn(i + 1)
		allSongs[i], allSongs[j] = allSongs[j], allSongs[i]
	}

	logger.API("Multi-genre blend: %d genres, %d total songs", len(matchedGenres), len(allSongs))

	return matchedGenres, allSongs, len(allSongs) > 0
}

// extractDecadeFromPrompt tries to find decade references in the prompt for smarter matching.
// Supports: "90s", "early 90s", "late 80s", "mid 2000s", "turn of the century", "modern", "classic"
func extractDecadeFromPrompt(prompt string) (minYear, maxYear int) {
	promptLower := strings.ToLower(prompt)

	// Check for "early", "mid", "late" modifiers
	hasEarly := strings.Contains(promptLower, "early")
	hasMid := strings.Contains(promptLower, "mid")
	hasLate := strings.Contains(promptLower, "late")

	// Check for special phrases
	if strings.Contains(promptLower, "turn of the century") || strings.Contains(promptLower, "y2k") {
		return 1998, 2002
	}
	if strings.Contains(promptLower, "modern") || strings.Contains(promptLower, "recent") || strings.Contains(promptLower, "new") {
		currentYear := 2025 // Could be dynamic
		return currentYear - 5, currentYear
	}
	if strings.Contains(promptLower, "classic") || strings.Contains(promptLower, "old school") || strings.Contains(promptLower, "oldschool") {
		return 1960, 1989
	}
	if strings.Contains(promptLower, "retro") {
		return 1970, 1989
	}
	if strings.Contains(promptLower, "vintage") {
		return 1950, 1979
	}

	// Check for explicit year ranges like "2015-2020" or "2015 to 2020"
	yearRangeRegex := regexp.MustCompile(`(19|20)(\d{2})\s*[-–to]+\s*(19|20)?(\d{2})`)
	if matches := yearRangeRegex.FindStringSubmatch(promptLower); len(matches) >= 5 {
		startDecade := matches[1]
		startYear := matches[2]
		endYear := matches[4]
		endDecade := matches[3]
		if endDecade == "" {
			endDecade = startDecade // Assume same century
		}
		minY := 0
		maxY := 0
		fmt.Sscanf(startDecade+startYear, "%d", &minY)
		fmt.Sscanf(endDecade+endYear, "%d", &maxY)
		if minY > 0 && maxY > 0 {
			return minY, maxY
		}
	}

	// Check for decade patterns
	decadeRegex := regexp.MustCompile(`\b(19|20)?([0-9])0s?\b`)
	matches := decadeRegex.FindStringSubmatch(promptLower)
	if len(matches) > 0 {
		fullMatch := matches[0]
		var baseYear int

		// Handle cases like "90s", "1990s", "90"
		if strings.Contains(fullMatch, "90") || fullMatch == "90s" || fullMatch == "1990s" {
			baseYear = 1990
		} else if strings.Contains(fullMatch, "80") || fullMatch == "80s" || fullMatch == "1980s" {
			baseYear = 1980
		} else if strings.Contains(fullMatch, "70") || fullMatch == "70s" || fullMatch == "1970s" {
			baseYear = 1970
		} else if strings.Contains(fullMatch, "60") || fullMatch == "60s" || fullMatch == "1960s" {
			baseYear = 1960
		} else if strings.Contains(fullMatch, "50") || fullMatch == "50s" || fullMatch == "1950s" {
			baseYear = 1950
		} else if strings.Contains(fullMatch, "00") || fullMatch == "00s" || fullMatch == "2000s" {
			baseYear = 2000
		} else if strings.Contains(fullMatch, "10") || fullMatch == "10s" || fullMatch == "2010s" {
			baseYear = 2010
		} else if strings.Contains(fullMatch, "20") || fullMatch == "20s" || fullMatch == "2020s" {
			baseYear = 2020
		}

		if baseYear > 0 {
			// Apply early/mid/late modifiers
			if hasEarly {
				return baseYear, baseYear + 3 // e.g., early 90s = 1990-1993
			} else if hasMid {
				return baseYear + 3, baseYear + 6 // e.g., mid 90s = 1993-1996
			} else if hasLate {
				return baseYear + 6, baseYear + 9 // e.g., late 90s = 1996-1999
			}
			return baseYear, baseYear + 9 // Full decade
		}
	}

	return 0, 0
}

// applyPlayHistoryFilters filters songs based on play history preferences.
// discoverMode: "balanced", "discover" (prefer underplayed), "favorites" (prefer frequently played)
func (a *API) applyPlayHistoryFilters(songs []any, recentlyPlayedIDs map[string]bool, discoverMode string, onePerArtist bool, limit int) []any {
	if len(songs) == 0 {
		return songs
	}

	// Filter out recently played songs if exclusion is enabled
	if len(recentlyPlayedIDs) > 0 {
		filtered := make([]any, 0, len(songs))
		for _, s := range songs {
			if id, ok := songID(s); !ok || !recentlyPlayedIDs[id] {
				filtered = append(filtered, s)
			}
		}
		songs = filtered
		logger.API("After filtering recent: %d songs", len(songs))
	}

	// Apply one-per-artist filter
	if onePerArtist {
		seenArtists := make(map[string]bool)
		filtered := make([]any, 0, len(songs))
		for _, s := range songs {
			if artist, ok := songArtist(s); !ok || !seenArtists[strings.ToLower(artist)] {
				if ok {
					seenArtists[strings.ToLower(artist)] = true
				}
				filtered = append(filtered, s)
			}
		}
		songs = filtered
		logger.API("After one-per-artist: %d songs", len(songs))
	}

	// Sort based on discover mode
	switch discoverMode {
	case "discover":
		// Sort by play count ascending (underplayed first)
		sortSongsByPlayCount(songs, true)
	case "favorites":
		// Sort by play count descending (favorites first)
		sortSongsByPlayCount(songs, false)
	default:
		// "balanced" - shuffle for variety using true random (Fisher-Yates)
		for i := len(songs) - 1; i > 0; i-- {
			j := rand.Intn(i + 1)
			songs[i], songs[j] = songs[j], songs[i]
		}
	}

	// Limit to target
	if len(songs) > limit {
		songs = songs[:limit]
	}

	return songs
}

// sortSongsByPlayCount sorts songs by play_count.
// ascending=true: underplayed first, ascending=false: favorites first
func sortSongsByPlayCount(songs []any, ascending bool) {
	// Simple bubble sort since we have limited songs
	for i := 0; i < len(songs)-1; i++ {
		for j := i + 1; j < len(songs); j++ {
			countI := getPlayCount(songs[i])
			countJ := getPlayCount(songs[j])

			shouldSwap := false
			if ascending && countI > countJ {
				shouldSwap = true
			} else if !ascending && countI < countJ {
				shouldSwap = true
			}

			if shouldSwap {
				songs[i], songs[j] = songs[j], songs[i]
			}
		}
	}
}

func getPlayCount(s any) int {
	switch song := s.(type) {
	case db.Song:
		return song.PlayCount
	case *db.Song:
		if song != nil {
			return song.PlayCount
		}
	case map[string]any:
		if count, ok := song["playCount"].(float64); ok {
			return int(count)
		}
		if count, ok := song["play_count"].(float64); ok {
			return int(count)
		}
	}
	return 0
}

func songID(s any) (string, bool) {
	switch song := s.(type) {
	case db.Song:
		return song.ID, song.ID != ""
	case *db.Song:
		if song != nil {
			return song.ID, song.ID != ""
		}
	case map[string]any:
		id, ok := song["id"].(string)
		return id, ok && id != ""
	}
	return "", false
}

func songArtist(s any) (string, bool) {
	switch song := s.(type) {
	case db.Song:
		return song.Artist, song.Artist != ""
	case *db.Song:
		if song != nil {
			return song.Artist, song.Artist != ""
		}
	case map[string]any:
		artist, ok := song["artist"].(string)
		return artist, ok && artist != ""
	}
	return "", false
}

// songMatchesYear implements the same original-release-year preference as the
// database queries. A missing year cannot satisfy an explicit era constraint.
func songMatchesYear(song db.Song, minYear, maxYear int) bool {
	if minYear == 0 && maxYear == 0 {
		return true
	}
	year := song.OriginalYear
	if year == 0 {
		year = song.Year
	}
	if year == 0 {
		return false
	}
	return (minYear == 0 || year >= minYear) && (maxYear == 0 || year <= maxYear)
}

func (a *API) handleGenerateSmartPlaylist(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Prompt             string `json:"prompt"`
		BlendMode          string `json:"blendMode"`          // "single" (default) or "mixed"
		TargetSongs        int    `json:"targetSongs"`        // Number of songs to return (default 50)
		DiscoverMode       string `json:"discoverMode"`       // "balanced" (default), "discover", "favorites"
		AvoidRecentlyHours int    `json:"avoidRecentlyHours"` // Avoid songs played in last N hours (0 = disabled)
		OnePerArtist       bool   `json:"onePerArtist"`       // Limit to one song per artist
		UseTimeContext     bool   `json:"useTimeContext"`     // Add time-of-day context to prompt
		// DJ Mode fields
		Mode                  string `json:"mode"`                  // "playlist" (default) or "dj"
		Persona               string `json:"persona"`               // DJ persona key (default "FlowMaster")
		TargetDurationMinutes int    `json:"targetDurationMinutes"` // DJ set duration in minutes (default 45)
		TalkMode              bool   `json:"talkMode"`              // Enable DJ narration cues
		FlowStrictness        int    `json:"flowStrictness"`        // BPM continuity strictness 0-100 (default 60)
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Set defaults
	if req.BlendMode == "" {
		req.BlendMode = "mixed"
	}
	if req.TargetSongs <= 0 || req.TargetSongs > 100 {
		req.TargetSongs = 50
	}
	if req.DiscoverMode == "" {
		req.DiscoverMode = "balanced"
	}
	// DJ Mode defaults
	if req.Mode == "" {
		req.Mode = dj.ModePlaylist
	}
	if req.Persona == "" {
		req.Persona = dj.PersonaFlowMaster
	}
	if req.TargetDurationMinutes <= 0 {
		req.TargetDurationMinutes = 45
	}
	if req.FlowStrictness <= 0 || req.FlowStrictness > 100 {
		req.FlowStrictness = 60
	}

	// Extract explicit date constraints before any fast-path matching. This keeps
	// requests such as "90s west coast hip-hop" from being reduced to a genre-only
	// query and gives deterministic constraints precedence over LLM output.
	promptMinYear, promptMaxYear := extractDecadeFromPrompt(req.Prompt)

	// Handle DJ Mode separately
	if req.Mode == dj.ModeDJ {
		a.handleDJMode(w, r, req.Prompt, req.Persona, req.TargetDurationMinutes,
			req.FlowStrictness, req.TalkMode, req.UseTimeContext,
			req.DiscoverMode, req.AvoidRecentlyHours)
		return
	}

	// Get recently played song IDs to exclude
	var recentlyPlayedIDs map[string]bool
	if req.AvoidRecentlyHours > 0 {
		ids, err := a.db.GetRecentlyPlayedSongIDs(req.AvoidRecentlyHours)
		if err == nil && len(ids) > 0 {
			recentlyPlayedIDs = make(map[string]bool)
			for _, id := range ids {
				recentlyPlayedIDs[id] = true
			}
			logger.API("Excluding %d recently played songs", len(recentlyPlayedIDs))
		}
	}

	// First, try artist-based matching for "more like [artist]" prompts
	if artistName, songs, matched := a.tryArtistBasedMatch(req.Prompt); matched {
		logger.API("Artist-based match found: %s with %d songs", artistName, len(songs))

		// Apply filters
		songs = a.applyPlayHistoryFilters(songs, recentlyPlayedIDs, req.DiscoverMode, req.OnePerArtist, req.TargetSongs)

		// Transform paths to API URLs for frontend consumption
		songs = transformSongsForAPI(songs)

		filter := LocalPlaylistFilter{
			Artists:     []string{artistName},
			Description: fmt.Sprintf("Songs similar to %s", artistName),
			LocalMatch:  true,
			BlendMode:   "single",
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"filter": filter,
			"songs":  songs,
		})
		return
	}

	// Second, try local genre matching before calling Gemini
	// This handles basic cases like exact genre names efficiently without API calls
	if matchedGenre, songs, matched := a.tryLocalGenreMatch(req.Prompt, promptMinYear, promptMaxYear); matched {
		logger.API("Local genre match found: %s with %d songs", matchedGenre, len(songs))

		// Apply filters
		songs = a.applyPlayHistoryFilters(songs, recentlyPlayedIDs, req.DiscoverMode, req.OnePerArtist, req.TargetSongs)

		// Transform paths to API URLs for frontend consumption
		songs = transformSongsForAPI(songs)

		filter := LocalPlaylistFilter{
			Genres:      []string{matchedGenre},
			MinYear:     promptMinYear,
			MaxYear:     promptMaxYear,
			Description: fmt.Sprintf("Songs from the %s genre", matchedGenre),
			LocalMatch:  true,
			BlendMode:   "single",
			MatchedGenres: []MatchedGenre{{
				Name:       matchedGenre,
				Score:      100,
				SongCount:  len(songs),
				Proportion: 1.0,
			}},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"filter": filter,
			"songs":  songs,
		})
		return
	}

	// Third, try mood/activity keyword matching (Tier 1.5)
	// This intercepts common vibe prompts like "chill", "workout", "relaxing"
	// to avoid Gemini API calls for simple mood-based requests
	if moodFilter, songs, matched := a.tryMoodBasedMatch(req.Prompt); matched {
		logger.API("Mood-based match found: mood=%s, energy=%s with %d songs",
			moodFilter.Mood, moodFilter.Energy, len(songs))

		// Apply filters
		songs = a.applyPlayHistoryFilters(songs, recentlyPlayedIDs, req.DiscoverMode, req.OnePerArtist, req.TargetSongs)

		// Transform paths to API URLs for frontend consumption
		songs = transformSongsForAPI(songs)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"filter": moodFilter,
			"songs":  songs,
		})
		return
	}

	// No local match found - fall back to LLM AI for complex prompts
	// Get LLM settings from database (falls back to Gemini for backward compatibility)
	llmSettings := getLLMSettings(a)

	// Validate we have the required API key (Ollama doesn't need one)
	if llmSettings.Provider != llm.ProviderOllama && llmSettings.APIKey == "" {
		// Fall back to Gemini API key if LLM not configured
		geminiKey, err := a.db.GetSetting("gemini_api_key")
		if err != nil || geminiKey == "" {
			http.Error(w, "AI provider not configured. Set up Ollama locally or add an API key in Settings.", http.StatusServiceUnavailable)
			return
		}
		llmSettings.Provider = llm.ProviderGemini
		llmSettings.APIKey = geminiKey
		llmSettings.Model = llm.DefaultGeminiModel
	}

	// Enhance prompt with time context if enabled
	promptToUse := enhancePromptWithTimeContext(req.Prompt, req.UseTimeContext)

	// Create LLM provider and parse the playlist filter
	provider, err := llm.NewProvider(llmSettings)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to create AI provider: %v", err), http.StatusInternalServerError)
		return
	}
	defer provider.Close()

	// Get available genres from user's library to help LLM select relevant genres
	availableGenres := a.getAvailableGenreNames()
	logger.API("AI DJ: Available genres in library: %d total, top 10: %v", len(availableGenres), truncateSlice(availableGenres, 10))

	// Use context-aware parsing if we have local genres, otherwise fall back to generic
	var llmFilter *llm.PlaylistFilter
	if len(availableGenres) > 0 {
		llmFilter, err = provider.ParsePlaylistFilterWithContext(r.Context(), promptToUse, availableGenres)
	} else {
		llmFilter, err = provider.ParsePlaylistFilter(r.Context(), promptToUse)
	}
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to generate filter: %v", err), http.StatusInternalServerError)
		return
	}

	// Use the LLM filter directly (no conversion needed)
	filter := llmFilter

	// Log what the LLM returned with detailed info
	logger.API("AI DJ Result: prompt='%s' → genres=%v mood=%s energy=%s years=%d-%d blendMode=%s",
		req.Prompt, filter.Genres, filter.Mood, filter.Energy, filter.MinYear, filter.MaxYear, req.BlendMode)

	// Explicit dates in the prompt are authoritative. The LLM can fill an absent
	// date range, but it must not broaden or replace one the user supplied.
	if promptMinYear > 0 || promptMaxYear > 0 {
		filter.MinYear = promptMinYear
		filter.MaxYear = promptMaxYear
		logger.API("AI DJ: Applied explicit prompt years: %d-%d", promptMinYear, promptMaxYear)
	}

	// Multi-genre blending mode
	if req.BlendMode == "mixed" {
		matchedGenres, songs, matched := a.tryMatchMultipleGenres(filter, req.Prompt, 3, req.TargetSongs)
		if matched {
			logger.API("Multi-genre blend: %d genres, %d songs", len(matchedGenres), len(songs))

			// Apply play history filters
			songs = a.applyPlayHistoryFilters(songs, recentlyPlayedIDs, req.DiscoverMode, req.OnePerArtist, req.TargetSongs)

			// Transform paths to API URLs for frontend consumption
			songs = transformSongsForAPI(songs)

			genreNames := make([]string, len(matchedGenres))
			for i, g := range matchedGenres {
				genreNames[i] = g.Name
			}

			smartFilter := LocalPlaylistFilter{
				Genres:        genreNames,
				MinYear:       filter.MinYear,
				MaxYear:       filter.MaxYear,
				Mood:          filter.Mood,
				Energy:        filter.Energy,
				Tempo:         filter.Tempo,
				Description:   fmt.Sprintf("Blended playlist from %d genres (matched from: %s)", len(matchedGenres), req.Prompt),
				LocalMatch:    true,
				BlendMode:     "mixed",
				MatchedGenres: matchedGenres,
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"filter": smartFilter,
				"songs":  songs,
			})
			return
		}
	}

	// SMART MATCHING: Try to match Gemini's intent against indexed genres (single mode)
	// This bridges the gap between Gemini's generic understanding (e.g., "Alternative" + 1990-1999)
	// and the user's actual indexed genres (e.g., "90s Alternative")
	if matchedGenre, songs, matched := a.tryMatchIndexedGenre(filter, req.Prompt); matched {
		logger.API("Smart indexed genre match after Gemini: '%s' with %d songs", matchedGenre, len(songs))

		// Apply play history filters
		songs = a.applyPlayHistoryFilters(songs, recentlyPlayedIDs, req.DiscoverMode, req.OnePerArtist, req.TargetSongs)

		// Transform paths to API URLs for frontend consumption
		songs = transformSongsForAPI(songs)

		smartFilter := LocalPlaylistFilter{
			Genres:      []string{matchedGenre},
			MinYear:     filter.MinYear,
			MaxYear:     filter.MaxYear,
			Mood:        filter.Mood,
			Energy:      filter.Energy,
			Tempo:       filter.Tempo,
			Description: fmt.Sprintf("Songs from the %s genre (matched from: %s)", matchedGenre, req.Prompt),
			LocalMatch:  true,
			BlendMode:   "single",
			MatchedGenres: []MatchedGenre{{
				Name:       matchedGenre,
				Score:      100,
				SongCount:  len(songs),
				Proportion: 1.0,
			}},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"filter": smartFilter,
			"songs":  songs,
		})
		return
	}

	// No indexed genre match - use Gemini's filter directly
	// This is the Tier 3 fallback when smart indexed matching fails.
	// Now includes mood/energy/tempo filters when Gemini extracts them from the prompt.
	songs, err := a.db.GetSongsBySmartFilter(
		filter.Genres,
		filter.Artists,
		filter.MinYear,
		filter.MaxYear,
		filter.Mood,
		filter.Energy,
		filter.Tempo,
	)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to query songs: %v", err), http.StatusInternalServerError)
		return
	}

	// Convert to []any for filtering
	songsAny := make([]any, len(songs))
	for i, s := range songs {
		songsAny[i] = s
	}

	// Apply play history filters
	songsAny = a.applyPlayHistoryFilters(songsAny, recentlyPlayedIDs, req.DiscoverMode, req.OnePerArtist, req.TargetSongs)

	// Transform paths to API URLs for frontend consumption
	songsAny = transformSongsForAPI(songsAny)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"filter": filter,
		"songs":  songsAny,
	})
}

// handleDJMode processes DJ mode requests using the intelligent sequencer.
// This is called when the request has mode="dj" instead of "playlist".
func (a *API) handleDJMode(w http.ResponseWriter, r *http.Request,
	prompt string, persona string, durationMinutes int,
	flowStrictness int, talkMode bool, useTimeContext bool,
	discoverMode string, avoidRecentlyHours int) {

	ctx := r.Context()

	logger.API("DJ Mode request: prompt=%q persona=%s duration=%dmin flow=%d talkMode=%v",
		prompt, persona, durationMinutes, flowStrictness, talkMode)

	// Get the persona definition (returns default if not found)
	personaDef := dj.GetPersona(persona)
	logger.API("DJ Mode using persona: %s (%s)", personaDef.Name, personaDef.Description)

	// Get all songs from the database for analysis
	allSongs, err := a.db.GetAllSongs()
	if err != nil {
		logger.API("DJ Mode failed to get songs: %v", err)
		http.Error(w, fmt.Sprintf("Failed to get songs: %v", err), http.StatusInternalServerError)
		return
	}

	if len(allSongs) == 0 {
		logger.API("DJ Mode failed: library is empty")
		http.Error(w, "Library is empty", http.StatusNotFound)
		return
	}

	logger.API("DJ Mode: %d songs in library", len(allSongs))

	// First, try to match the prompt against genres using existing logic
	// This extracts seed genres from the user's prompt
	seedGenres := []string{}
	seedArtists := []string{}

	// Try local genre matching first
	promptMinYear, promptMaxYear := extractDecadeFromPrompt(prompt)
	if genreName, _, matched := a.tryLocalGenreMatch(prompt, promptMinYear, promptMaxYear); matched {
		seedGenres = append(seedGenres, genreName)
		logger.API("DJ Mode: Local genre match found: %s", genreName)
	}

	// Try mood-based matching to extract mood/energy/tempo hints
	moodMatch := a.tryMoodBasedMatchInfo(prompt)
	if moodMatch != nil {
		logger.API("DJ Mode: Mood match found: mood=%s energy=%s tempo=%s",
			moodMatch.mood, moodMatch.energy, moodMatch.tempo)
	}

	// If no local match and we have LLM, try to get genres from LLM
	if len(seedGenres) == 0 {
		llmSettings := getLLMSettings(a)
		if llmSettings.Provider == llm.ProviderOllama || llmSettings.APIKey != "" {
			provider, err := llm.NewProvider(llmSettings)
			if err == nil {
				defer provider.Close()
				availableGenres := a.getAvailableGenreNames()
				llmFilter, err := provider.ParsePlaylistFilterWithContext(ctx, prompt, availableGenres)
				if err == nil && llmFilter != nil {
					seedGenres = llmFilter.Genres
					seedArtists = llmFilter.Artists
					// Deterministic prompt years are already populated above. Only use
					// LLM years when the prompt did not contain an explicit era.
					if promptMinYear == 0 && promptMaxYear == 0 {
						promptMinYear = llmFilter.MinYear
						promptMaxYear = llmFilter.MaxYear
					}
					logger.API("DJ Mode: LLM extracted genres=%v artists=%v", seedGenres, seedArtists)

					// Apply mood match info from LLM filter if not already matched
					if moodMatch == nil && (llmFilter.Mood != "" || llmFilter.Energy != "" || llmFilter.Tempo != "") {
						moodMatch = &moodMatchInfo{
							mood:   llmFilter.Mood,
							energy: llmFilter.Energy,
							tempo:  llmFilter.Tempo,
						}
					}
				} else {
					logger.API("DJ Mode: LLM filter failed: %v", err)
				}
			}
		}
	}

	// Collect genre stats and calculate avg song length
	genreCounts := make(map[string]int)
	var totalDuration float64
	for _, song := range allSongs {
		for _, g := range song.Genre {
			genreCounts[g]++
		}
		totalDuration += song.Duration
	}
	var genres []string
	for genre := range genreCounts {
		genres = append(genres, genre)
	}
	avgSongLengthSec := 210 // Default 3.5 min
	if len(allSongs) > 0 {
		avgSongLengthSec = int(totalDuration / float64(len(allSongs)))
	}

	// Get recently played song IDs if avoidance is enabled
	recentlyPlayedIDs := make(map[string]bool)
	if avoidRecentlyHours > 0 {
		recentPlays, err := a.db.GetRecentlyPlayedSongIDs(avoidRecentlyHours)
		if err == nil {
			for _, songID := range recentPlays {
				recentlyPlayedIDs[songID] = true
			}
		}
	}

	// Build library context for planner
	libContext := dj.LibraryContext{
		AvailableGenres:  genres,
		SeedGenres:       seedGenres,
		SeedArtists:      seedArtists,
		AvgSongLengthSec: avgSongLengthSec,
		TotalSongs:       len(allSongs),
	}

	// Build plan options
	planOpts := dj.PlanOptions{
		Persona:           persona,
		TargetDurationMin: durationMinutes,
		FlowStrictness:    flowStrictness,
		UseTimeContext:    useTimeContext,
		TalkMode:          talkMode,
	}

	// Get LLM settings for the planner
	llmSettings := getLLMSettings(a)

	// Validate we have the required API key (Ollama doesn't need one)
	if llmSettings.Provider != llm.ProviderOllama && llmSettings.APIKey == "" {
		// Fall back to Gemini API key if LLM not configured
		geminiKey, err := a.db.GetSetting("gemini_api_key")
		if err != nil || geminiKey == "" {
			http.Error(w, "AI provider not configured. Set up Ollama locally or add an API key in Settings.", http.StatusServiceUnavailable)
			return
		}
		llmSettings.Provider = llm.ProviderGemini
		llmSettings.APIKey = geminiKey
		llmSettings.Model = llm.DefaultGeminiModel
	}

	// Create the LLM planner
	planner := dj.NewLLMPlanner(llmSettings)

	// Build the DJ set plan
	plan, err := planner.BuildPlan(ctx, prompt, planOpts, libContext)
	if err != nil {
		logger.API("DJ planner failed: %v, using default plan", err)
	}
	if plan == nil {
		logger.API("DJ Mode: Plan is nil, returning error")
		http.Error(w, "Failed to create DJ set plan", http.StatusInternalServerError)
		return
	}

	logger.API("DJ Mode plan created: %d phases, intent: %s", len(plan.Phases), plan.IntentSummary)
	for i, phase := range plan.Phases {
		logger.API("  Phase %d: %s - %s energy, %s tempo, %d songs, BPM %d-%d",
			i+1, phase.Name, phase.TargetEnergy, phase.TargetTempo, phase.TargetCount, phase.MinBPM, phase.MaxBPM)
	}

	// Filter candidates based on:
	// 1. Recently played (if avoidance enabled)
	// 2. Matching genres (if seed genres were extracted)
	// 3. Matching mood/energy/tempo (if mood match was found)
	candidates := make([]db.Song, 0, len(allSongs))
	for _, song := range allSongs {
		// Skip recently played
		if recentlyPlayedIDs[song.ID] {
			continue
		}

		// Era constraints are hard eligibility requirements. Sequencing can relax
		// BPM or mood fit, but it must never introduce a track outside this range.
		if !songMatchesYear(song, promptMinYear, promptMaxYear) {
			continue
		}

		// If we have seed genres, filter to matching songs
		if len(seedGenres) > 0 {
			matchesGenre := false
			for _, sg := range seedGenres {
				for _, songGenre := range song.Genre {
					if strings.EqualFold(songGenre, sg) || strings.Contains(strings.ToLower(songGenre), strings.ToLower(sg)) {
						matchesGenre = true
						break
					}
				}
				if matchesGenre {
					break
				}
			}
			if !matchesGenre {
				continue
			}
		}

		// If we have seed artists, include their songs
		if len(seedArtists) > 0 {
			matchesArtist := false
			for _, sa := range seedArtists {
				if strings.EqualFold(song.Artist, sa) || strings.Contains(strings.ToLower(song.Artist), strings.ToLower(sa)) {
					matchesArtist = true
					break
				}
			}
			// Note: We include if artist matches OR genre matches, not AND
			if matchesArtist {
				candidates = append(candidates, song)
				continue
			}
		}

		candidates = append(candidates, song)
	}

	logger.API("DJ Mode: %d candidate songs after filtering (from %d total)", len(candidates), len(allSongs))

	// Only broaden an unconstrained discovery request. Falling back to all songs
	// for an explicit genre, artist, or era violates the user's request.
	hasHardConstraints := promptMinYear > 0 || promptMaxYear > 0 || len(seedGenres) > 0 || len(seedArtists) > 0
	if len(candidates) < 10 && len(allSongs) > 10 && !hasHardConstraints {
		logger.API("DJ Mode: Too few candidates (%d), falling back to all songs", len(candidates))
		candidates = make([]db.Song, 0, len(allSongs))
		for _, song := range allSongs {
			if !recentlyPlayedIDs[song.ID] {
				candidates = append(candidates, song)
			}
		}
	}

	// Create score context
	scoreCtx := dj.NewScoreContext()
	scoreCtx.DiscoverMode = discoverMode
	scoreCtx.FlowStrictness = flowStrictness
	scoreCtx.RecentlyPlayedIDs = recentlyPlayedIDs

	// Create the sequencer
	sequencer := dj.NewSequencer()

	// Build the queue using the sequencer
	queue, phaseResults, err := sequencer.BuildQueue(candidates, plan, personaDef, scoreCtx)
	if err != nil {
		logger.API("DJ Mode: Failed to build queue: %v", err)
		http.Error(w, fmt.Sprintf("Failed to build DJ queue: %v", err), http.StatusInternalServerError)
		return
	}

	logger.API("DJ Mode: Built queue with %d songs across %d phases", len(queue), len(phaseResults))

	// Generate narration if talk mode is enabled
	var narration *dj.DJNarration
	if talkMode && len(queue) > 0 {
		narration, err = planner.GenerateNarration(ctx, plan)
		if err != nil {
			logger.API("Failed to generate DJ narration: %v", err)
		}
	}

	// Transform paths to API URLs
	songsAny := make([]any, len(queue))
	for i, s := range queue {
		songsAny[i] = s
	}
	songsAny = transformSongsForAPI(songsAny)

	// Build the DJ response
	djResponse := dj.DJResponse{
		Plan:      plan,
		Phases:    phaseResults,
		Narration: narration,
	}

	logger.API("DJ Mode: Returning %d songs for prompt %q", len(queue), prompt)

	// Return combined response
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"filter": map[string]interface{}{
			"mode":    "dj",
			"persona": persona,
			"prompt":  prompt,
			"genres":  seedGenres,
			"artists": seedArtists,
			"minYear": promptMinYear,
			"maxYear": promptMaxYear,
		},
		"songs": songsAny,
		"dj":    djResponse,
	})
}
