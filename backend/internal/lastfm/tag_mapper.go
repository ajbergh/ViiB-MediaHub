// Package lastfm provides Last.FM API integration for ViiB MediaHub.
//
// tag_mapper.go - Maps Last.FM user tags to structured mood/energy/tempo values.
//
// Last.FM tags are free-form, user-generated strings. This mapper converts them
// to the structured values expected by ViiB's AI DJ and smart playlist features:
//
//   - Genres: Extracted from known genre tags (rock, pop, electronic, etc.)
//   - Mood: happy, sad, calm, energetic, dark, romantic, aggressive, dreamy, nostalgic
//   - Energy: high, medium, low
//   - Tempo: fast, medium, slow
//   - Instrumental: detected from tags like "instrumental", "no vocals"
//
// Tags are sorted by usage count (from Last.FM) and the first matching tag
// for each category is used. Minimum tag count threshold (default 30) filters
// out rare/unreliable tags.
//
// Created: 2025-12-31
// Last Modified: 2025-12-31
package lastfm

import (
	"sort"
	"strings"
)

// TagMapper maps Last.FM user tags to structured mood/energy/tempo values.
type TagMapper struct {
	// Genre tags that should be extracted as genres
	genreTags map[string]bool

	// Mood mappings: tag -> normalized mood
	moodMappings map[string]string

	// Energy mappings: tag -> normalized energy level
	energyMappings map[string]string

	// Tempo mappings: tag -> normalized tempo
	tempoMappings map[string]string

	// Tags indicating instrumental tracks
	instrumentalTags map[string]bool
}

// NewTagMapper creates a new TagMapper with default mappings.
func NewTagMapper() *TagMapper {
	return &TagMapper{
		genreTags:        defaultGenreTags(),
		moodMappings:     defaultMoodMappings(),
		energyMappings:   defaultEnergyMappings(),
		tempoMappings:    defaultTempoMappings(),
		instrumentalTags: defaultInstrumentalTags(),
	}
}

// MapTags converts Last.FM tags to our enrichment format.
// minCount filters out tags with fewer uses (default: 30).
func (m *TagMapper) MapTags(tags []TagWithCount, minCount int) *TagEnrichment {
	if minCount <= 0 {
		minCount = 30
	}

	enrichment := &TagEnrichment{
		Genres: make([]string, 0),
	}

	// Sort tags by count (highest first)
	sortedTags := make([]TagWithCount, len(tags))
	copy(sortedTags, tags)
	sort.Slice(sortedTags, func(i, j int) bool {
		return sortedTags[i].Count > sortedTags[j].Count
	})

	// Extract raw tags for reference
	for _, tag := range sortedTags[:min(len(sortedTags), 10)] {
		enrichment.RawTags = append(enrichment.RawTags, tag.Name)
	}

	// Process each tag
	for _, tag := range sortedTags {
		normalizedTag := strings.ToLower(strings.TrimSpace(tag.Name))

		// Skip low-count tags for genre extraction
		if tag.Count >= minCount {
			// Check if it's a genre tag
			if m.genreTags[normalizedTag] {
				// Capitalize properly
				enrichment.Genres = append(enrichment.Genres, properCase(tag.Name))
			}
		}

		// Mood detection (use first match with highest count)
		if enrichment.Mood == "" {
			if mood, ok := m.moodMappings[normalizedTag]; ok {
				enrichment.Mood = mood
			}
		}

		// Energy detection (use first match with highest count)
		if enrichment.Energy == "" {
			if energy, ok := m.energyMappings[normalizedTag]; ok {
				enrichment.Energy = energy
			}
		}

		// Tempo detection (use first match with highest count)
		if enrichment.Tempo == "" {
			if tempo, ok := m.tempoMappings[normalizedTag]; ok {
				enrichment.Tempo = tempo
			}
		}

		// Instrumental detection
		if m.instrumentalTags[normalizedTag] {
			enrichment.Instrumental = true
		}
	}

	// Limit genres to top 5
	if len(enrichment.Genres) > 5 {
		enrichment.Genres = enrichment.Genres[:5]
	}

	// Deduplicate genres
	enrichment.Genres = deduplicate(enrichment.Genres)

	return enrichment
}

// defaultGenreTags returns the default set of genre tags.
func defaultGenreTags() map[string]bool {
	genres := []string{
		// Main genres
		"rock", "pop", "electronic", "jazz", "hip-hop", "hip hop", "classical",
		"r&b", "rnb", "metal", "folk", "country", "blues", "punk", "soul", "funk",
		"reggae", "disco", "techno", "house", "trance", "dubstep", "drum and bass",
		"ambient", "indie", "alternative", "grunge", "new wave", "synth-pop", "synthpop",

		// Sub-genres rock
		"hard rock", "soft rock", "progressive rock", "prog rock", "classic rock",
		"indie rock", "alternative rock", "psychedelic rock", "garage rock",
		"post-rock", "post rock", "shoegaze", "britpop", "glam rock",

		// Sub-genres electronic
		"edm", "electro", "electronica", "idm", "downtempo", "trip-hop", "trip hop",
		"chillout", "lounge", "deep house", "progressive house", "tech house",
		"minimal", "breakbeat", "industrial", "ebm", "synthwave", "retrowave",
		"vaporwave", "lo-fi", "lofi",

		// Sub-genres metal
		"heavy metal", "death metal", "black metal", "thrash metal", "power metal",
		"doom metal", "progressive metal", "metalcore", "nu metal", "nu-metal",

		// Sub-genres hip-hop
		"rap", "trap", "boom bap", "old school hip hop", "conscious hip hop",
		"gangsta rap", "southern hip hop", "west coast hip hop", "east coast hip hop",

		// Sub-genres jazz
		"smooth jazz", "bebop", "swing", "big band", "fusion", "jazz fusion",
		"acid jazz", "nu jazz", "cool jazz", "free jazz",

		// World music
		"latin", "latin pop", "reggaeton", "salsa", "bossa nova", "samba",
		"afrobeat", "world music", "celtic", "flamenco", "k-pop", "kpop",
		"j-pop", "jpop", "j-rock", "jrock", "bollywood",

		// Other
		"soundtrack", "score", "ost", "musical", "opera", "choral",
		"new age", "easy listening", "lounge", "exotica",
		"gospel", "christian", "worship",
		"ska", "dub", "dancehall",
		"emo", "screamo", "post-hardcore", "hardcore",
		"acoustic", "singer-songwriter", "singer songwriter",
	}

	m := make(map[string]bool)
	for _, g := range genres {
		m[g] = true
	}
	return m
}

// defaultMoodMappings returns mappings from tags to mood values.
func defaultMoodMappings() map[string]string {
	return map[string]string{
		// Happy/Uplifting
		"happy":     "happy",
		"uplifting": "happy",
		"feel good": "happy",
		"feel-good": "happy",
		"positive":  "happy",
		"joyful":    "happy",
		"cheerful":  "happy",
		"fun":       "happy",
		"party":     "happy",
		"upbeat":    "happy",

		// Sad/Melancholic
		"sad":         "sad",
		"melancholic": "sad",
		"melancholy":  "sad",
		"depressing":  "sad",
		"emotional":   "sad",
		"tearjerker":  "sad",
		"heartbreak":  "sad",
		"somber":      "sad",
		"bittersweet": "sad",

		// Calm/Relaxing
		"chill":      "calm",
		"chillout":   "calm",
		"relaxing":   "calm",
		"relaxed":    "calm",
		"calm":       "calm",
		"mellow":     "calm",
		"peaceful":   "calm",
		"soothing":   "calm",
		"zen":        "calm",
		"meditative": "calm",

		// Energetic
		"energetic":   "energetic",
		"anthemic":    "energetic",
		"powerful":    "energetic",
		"driving":     "energetic",
		"pumping":     "energetic",
		"adrenaline":  "energetic",
		"high energy": "energetic",

		// Dark/Moody
		"dark":        "dark",
		"brooding":    "dark",
		"atmospheric": "dark",
		"moody":       "dark",
		"gothic":      "dark",
		"haunting":    "dark",
		"eerie":       "dark",
		"mysterious":  "dark",

		// Romantic
		"romantic": "romantic",
		"love":     "romantic",
		"sensual":  "romantic",
		"intimate": "romantic",
		"sexy":     "romantic",
		"passion":  "romantic",

		// Aggressive
		"aggressive": "aggressive",
		"angry":      "aggressive",
		"intense":    "aggressive",
		"brutal":     "aggressive",
		"heavy":      "aggressive",
		"fierce":     "aggressive",

		// Dreamy
		"dreamy":   "dreamy",
		"ethereal": "dreamy",
		"ambient":  "dreamy",
		"spacey":   "dreamy",
		"hypnotic": "dreamy",
		"trippy":   "dreamy",

		// Nostalgic
		"nostalgic": "nostalgic",
		"retro":     "nostalgic",
		"80s":       "nostalgic",
		"90s":       "nostalgic",
		"throwback": "nostalgic",
	}
}

// defaultEnergyMappings returns mappings from tags to energy levels.
func defaultEnergyMappings() map[string]string {
	return map[string]string{
		// High energy
		"high energy": "high",
		"energetic":   "high",
		"driving":     "high",
		"anthemic":    "high",
		"party":       "high",
		"dance":       "high",
		"upbeat":      "high",
		"fast":        "high",
		"intense":     "high",
		"powerful":    "high",
		"aggressive":  "high",
		"heavy":       "high",
		"loud":        "high",
		"hard":        "high",
		"pumping":     "high",

		// Medium energy
		"groovy":    "medium",
		"uptempo":   "medium",
		"moderate":  "medium",
		"mid-tempo": "medium",
		"midtempo":  "medium",
		"funky":     "medium",
		"bouncy":    "medium",
		"catchy":    "medium",

		// Low energy
		"low energy": "low",
		"chill":      "low",
		"chillout":   "low",
		"calm":       "low",
		"relaxing":   "low",
		"ambient":    "low",
		"mellow":     "low",
		"downtempo":  "low",
		"slow":       "low",
		"peaceful":   "low",
		"soft":       "low",
		"quiet":      "low",
		"gentle":     "low",
		"sleepy":     "low",
	}
}

// defaultTempoMappings returns mappings from tags to tempo values.
func defaultTempoMappings() map[string]string {
	return map[string]string{
		// Fast tempo
		"fast":       "fast",
		"upbeat":     "fast",
		"uptempo":    "fast",
		"high tempo": "fast",
		"energetic":  "fast",
		"driving":    "fast",
		"racing":     "fast",

		// Medium tempo
		"mid-tempo": "medium",
		"midtempo":  "medium",
		"moderate":  "medium",
		"groovy":    "medium",
		"steady":    "medium",

		// Slow tempo
		"slow":       "slow",
		"downtempo":  "slow",
		"slow tempo": "slow",
		"ballad":     "slow",
		"slow jam":   "slow",
		"laid back":  "slow",
		"laid-back":  "slow",
	}
}

// defaultInstrumentalTags returns tags that indicate instrumental tracks.
func defaultInstrumentalTags() map[string]bool {
	tags := []string{
		"instrumental",
		"no vocals",
		"no singing",
		"wordless",
		"post-rock", // Often instrumental
		"post rock",
		"neoclassical",
		"orchestral",
		"soundtrack",
		"score",
		"ambient", // Usually instrumental
		"classical",
	}

	m := make(map[string]bool)
	for _, t := range tags {
		m[t] = true
	}
	return m
}

// properCase capitalizes the first letter of each word.
func properCase(s string) string {
	words := strings.Fields(s)
	for i, word := range words {
		if len(word) > 0 {
			words[i] = strings.ToUpper(word[:1]) + strings.ToLower(word[1:])
		}
	}
	return strings.Join(words, " ")
}

// deduplicate removes duplicate strings from a slice (case-insensitive).
func deduplicate(s []string) []string {
	seen := make(map[string]bool)
	result := make([]string, 0, len(s))
	for _, v := range s {
		key := strings.ToLower(v)
		if !seen[key] {
			seen[key] = true
			result = append(result, v)
		}
	}
	return result
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
