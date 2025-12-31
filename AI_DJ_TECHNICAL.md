# AI DJ Technical Deep Dive

> **Purpose:** Comprehensive technical documentation of the AI DJ feature, including architecture, algorithms, LLM integration, and known shortcomings.
>
> **Last Updated:** 2025-12-30
>
> **Audience:** Developers working on or extending the AI DJ feature

---

## Table of Contents

1. [Architecture Overview](#architecture-overview)
2. [DJ Set Planning System (NEW)](#dj-set-planning-system)
3. [Four-Tier Matching System](#four-tier-matching-system)
4. [LLM Provider Integration](#llm-provider-integration)
5. [Genre Scoring Algorithm](#genre-scoring-algorithm)
6. [Multi-Genre Blending](#multi-genre-blending)
7. [Mood & Energy Analysis](#mood--energy-analysis)
8. [Play History Filters](#play-history-filters)
9. [Time-Aware Context](#time-aware-context)
10. [Caching Strategy](#caching-strategy)
11. [Known Shortcomings](#known-shortcomings)
12. [Preference Learning System](#preference-learning-system)
13. [Improvement Opportunities](#improvement-opportunities)

---

## DJ Set Planning System

> **Status:** ✅ Backend Complete (Phases 1-6) | ✅ Frontend Complete (Phases 7-8) | 🟡 Polish In Progress (Phase 9)

The DJ Set Planning system adds a structured layer on top of the existing smart playlist functionality, turning user prompts into curated DJ sets with energy arcs, phase structure, and deliberate sequencing.

### Key Files (DJ Mode)

| File | Purpose |
|------|---------|
| `backend/internal/dj/types.go` | Core types: DJSetPlan, DJPhase, ScoreContext, etc. |
| `backend/internal/dj/constants.go` | BPM defaults, phase weights, energy/tempo mappings |
| `backend/internal/dj/personas.go` | Persona definitions with scoring weights ✅ |
| `backend/internal/dj/dj_set_planner.go` | LLM-backed plan builder with cache ✅ |
| `backend/internal/dj/scoring.go` | Song scoring per phase with persona weights ✅ |
| `backend/internal/dj/sequencer.go` | Phase-based song sequencing ✅ |
| `backend/internal/api/smart_playlist.go` | API integration with handleDJMode() ✅ |
| `backend/internal/llm/provider.go` | Added Generate() method for DJ planner ✅ |
| `backend/internal/llm/prompts.go` | DJ set plan & narration prompts ✅ |
| `services/api.ts` | DJ mode TypeScript types ✅ |
| `slices/aiDjSlice.ts` | Frontend state for DJ mode ✅ |
| `slices/types.ts` | AIDJSlice interface with DJ mode properties ✅ |
| `pages/SmartPlaylists.tsx` | Complete DJ Console UI with loading/error states ✅ |

### DJ Mode Request Flow

```
┌──────────────────────────────────────────────────────────────────────┐
│  Frontend (SmartPlaylists.tsx - DJ Console)                          │
│  POST /api/smart-playlist                                            │
│  { prompt, mode: "dj", persona, targetDurationMinutes, ... }         │
└───────────────────────────────────┬──────────────────────────────────┘
                                    │
                                    ▼
┌──────────────────────────────────────────────────────────────────────┐
│  handleGenerateSmartPlaylist() [mode === "dj"]                       │
│  ├─ Existing matching logic (Tiers 0-4)                              │
│  ├─ Planner.BuildPlan() → LLM generates phase structure              │
│  ├─ Gather candidate songs from matched genres                       │
│  ├─ Sequencer.BuildQueue() → Score & sequence by phase               │
│  └─ Apply play history filters as final safety net                   │
└───────────────────────────────────┬──────────────────────────────────┘
                                    │
                                    ▼
┌──────────────────────────────────────────────────────────────────────┐
│  JSON Response: { filter, songs, dj: { plan, phases, narration } }   │
│  ├─ filter: LocalPlaylistFilter (same as playlist mode)              │
│  ├─ songs: Sequenced song list matching phase structure              │
│  └─ dj: DJResponse with plan, phase results, optional narration      │
└──────────────────────────────────────────────────────────────────────┘
```

### Data Structures (DJ Mode)

```go
// DJPhase - represents a single phase in a DJ set
type DJPhase struct {
    Name         string   // "Warm-up", "Build", "Peak", "Cooldown", "Afterhours"
    TargetEnergy string   // "low", "medium", "high"
    TargetTempo  string   // "slow", "medium", "fast"
    TargetMoods  []string // e.g., ["calm", "dreamy"]
    TargetCount  int      // Number of songs for this phase
    MinBPM       int      // Minimum BPM for this phase
    MaxBPM       int      // Maximum BPM for this phase
    Notes        string   // Short DJ note for this phase
}

// DJSetPlan - complete structure of a DJ set
type DJSetPlan struct {
    IntentSummary     string    // 1-sentence summary of vibe and intent
    TargetDurationMin int       // Target set duration in minutes
    Persona           string    // Active persona key
    FlowStrictness    int       // 0-100, higher = stricter BPM continuity
    Phases            []DJPhase // Ordered list of phases
    SeedGenres        []string  // Matched genres from filter
    SeedArtists       []string  // Matched artists from filter
    CreatedAtUnix     int64     // Unix timestamp of creation
    FromCache         bool      // True if loaded from cache
}

// ScoreContext - contextual information for scoring songs
type ScoreContext struct {
    GenreAffinity       map[string]float64 // Genre affinity scores
    ArtistSeen          map[string]bool    // Artists already used
    RecentlyPlayedIDs   map[string]bool    // Recently played songs
    LastSongBPM         int                // For continuity scoring
    LastSongMood        string
    LastSongEnergy      string
    DiscoverMode        string             // "balanced", "discover", "favorites"
    FlowStrictness      int                // 0-100
    SongSkipRates       map[string]float64 // Per-song skip rates
    GenreCompletionRate map[string]float64 // Per-genre completion rates
}
```

### Personas

| Persona | Description | Key Weights |
|---------|-------------|-------------|
| FlowMaster | Default: strong continuity, balanced novelty | High BPM continuity, medium favorites |
| CrowdPleaser | Favors high completion, favorites | High favorites boost, low novelty |
| DeepCutDJ | Heavy underplayed boost, novelty | High underplayed, high novelty |
| Explorer | Controlled novelty, medium continuity | Medium novelty, medium continuity |
| Curator | Strict genre purity, one-per-artist | High one-per-artist, strict genre |
| NightDrive | Smoother tempos, medium energy | Low energy variance, slow-medium tempo |

### Constants

```go
// BPM defaults for tempo categories
DefaultBPMSlow   = 90
DefaultBPMMedium = 120
DefaultBPMFast   = 150

// Valid BPM range
MinValidBPM = 60
MaxValidBPM = 190

// Phase distribution weights (typical)
PhaseWarmUp   = 25%
PhaseBuild    = 25%
PhasePeak     = 30%
PhaseCooldown = 20%

// Average song duration for calculations
DefaultAvgSongLengthSec = 210 (3.5 minutes)
```

---

## Architecture Overview

### Key Files

| File | Purpose |
|------|---------|
| `backend/internal/api/smart_playlist.go` | Main handler (~1500 lines), matching tiers, scoring |
| `backend/internal/llm/provider.go` | LLM provider abstraction (omnillm), filter generation |
| `backend/internal/llm/prompts.go` | System prompts for playlist filtering and enrichment |
| `backend/internal/llm/models.go` | Provider and model configuration |
| `backend/internal/db/db.go` | Database queries for songs, genres, play history, mood fields |
| `pages/SmartPlaylists.tsx` | Frontend UI for AI DJ prompt input and results |
| `services/api.ts` | TypeScript API client, interface definitions |

### Request Flow

```
┌──────────────────────────────────────────────────────────────────────┐
│  Frontend (SmartPlaylists.tsx)                                       │
│  POST /api/smart-playlist                                            │
│  { prompt, blendMode, targetSongs, discoverMode, ... }               │
└───────────────────────────────────┬──────────────────────────────────┘
                                    │
                                    ▼
┌──────────────────────────────────────────────────────────────────────┐
│  handleGenerateSmartPlaylist()                                       │
│  ├─ Tier 0: tryArtistBasedMatch()                                    │
│  ├─ Tier 1: tryLocalGenreMatch() [min 20 songs required]             │
│  ├─ Tier 2: tryMoodBasedMatch() [mood keywords direct match]         │
│  ├─ Tier 3: LLM + tryMatchIndexedGenre() [context-aware prompts]     │
│  └─ Tier 4: Direct database query with LLM filter                    │
└───────────────────────────────────┬──────────────────────────────────┘
                                    │
                                    ▼
┌──────────────────────────────────────────────────────────────────────┐
│  applyPlayHistoryFilters()                                           │
│  ├─ Filter out recently played songs                                 │
│  ├─ Apply one-per-artist constraint                                  │
│  └─ Sort by discover mode (balanced/discover/favorites)              │
└───────────────────────────────────┬──────────────────────────────────┘
                                    │
                                    ▼
┌──────────────────────────────────────────────────────────────────────┐
│  JSON Response: { filter, songs }                                    │
│  ├─ filter: LocalPlaylistFilter with matchedGenres, blendMode        │
│  └─ songs: []any - filtered, scored, limited song list               │
└──────────────────────────────────────────────────────────────────────┘
```

### Data Structures

```go
// LocalPlaylistFilter - returned to frontend
type LocalPlaylistFilter struct {
    Genres        []string       // Matched genre names
    Artists       []string       // Artist-based match targets
    MinYear       int            // Year range start (0 = any)
    MaxYear       int            // Year range end (0 = any)
    Mood          string         // e.g., "energetic", "chill"
    Energy        string         // "low", "medium", "high"
    Tempo         string         // "slow", "medium", "fast"
    Occasion      string         // "workout", "study", "party"
    Description   string         // Human-readable description
    LocalMatch    bool           // True if matched without AI
    BlendMode     string         // "single" or "mixed"
    MatchedGenres []MatchedGenre // Scored genres with proportions
}

// MatchedGenre - per-genre scoring info
type MatchedGenre struct {
    Name       string  // e.g., "90s Alternative"
    Score      int     // 0-100 matching score
    SongCount  int     // Songs available in this genre
    Proportion float64 // 0.0-1.0 weight in blended playlist
}
```

---

## Four-Tier Matching System

### Tier 0: Artist-Based Matching

**Trigger:** Prompts containing "more like", "similar to", "songs by", etc.

**Function:** `tryArtistBasedMatch(prompt string)`

**Algorithm:**
1. Regex patterns detect artist-related requests:
   - `"more like (?:the )?(.+)"`
   - `"similar to (?:the )?(.+)"`
   - `"songs by (?:the )?(.+)"`
   - `"artists? like (?:the )?(.+)"`
2. Extract artist name, normalize (lowercase, trim)
3. Query database for songs matching artist name
4. Find "similar artists" based on shared genres:
   - For each song by the target artist, collect their genres
   - Find other songs sharing those genres (different artist)
   - Include proportionally in results

**Advantages:**
- No API call needed
- Immediate response
- Leverages local genre tags for similarity

**Limitations:**
- Similarity is genre-based only (no audio analysis)
- Doesn't understand artist sound or era progression
- Small libraries may have insufficient cross-genre links

### Tier 1: Local Genre Matching

**Function:** `tryLocalGenreMatch(prompt string)`

**✅ Updated (2025-12-30):** Now requires a minimum of 20 songs for direct match.

**Algorithm:**
1. Normalize prompt: lowercase, trim whitespace
2. Check exact match against all indexed genre names
3. If no exact match, check partial matches:
   - Genre name must be ≥60% of prompt length (prevents "rock" matching everything)
   - Prompt must contain genre name as substring
4. **NEW:** Only return match if genre has ≥20 songs (otherwise fall back to LLM blending)

**Minimum Song Threshold:**
| Genre Songs | Behavior |
|-------------|----------|
| ≥20 songs | Direct match returned |
| <20 songs | Falls back to LLM for multi-genre blending |

This ensures users get diverse playlists even for sparse genres. For example, "Classic Rock" with only 11 songs will trigger LLM blending with "60s Rock", "70s Rock", etc.

**Examples:**
| Prompt | Match | Reason |
|--------|-------|--------|
| "90s Alternative" | Yes (exact) | Direct genre name match (if ≥20 songs) |
| "Give me some 90s Alternative" | Yes (partial) | Contains genre, genre is 60%+ of relevant part |
| "Rock" | Yes (exact) | If "Rock" is an indexed genre |
| "Something with drums" | No | No genre contains "drums" |

**Advantages:**
- Zero latency (no API call)
- 100% accurate when genre exists
- Handles user typos via case-insensitive matching

**Limitations:**
- Limited to exact or near-exact genre names
- Doesn't understand synonyms ("alt" → "alternative")
- Can't interpret mood/vibe descriptions

### Tier 1.5: Mood/Activity Keyword Matching ✨ NEW (2025-01-13)

**Function:** `tryMoodBasedMatch(prompt string)`

**Purpose:** Intercept common mood/activity prompts and query by mood fields directly, bypassing Gemini API entirely.

**Algorithm:**
1. Normalize prompt to lowercase
2. Check against 85+ mood keywords with word boundary detection
3. If match found, query songs by mood/energy/tempo
4. Falls back to mood-only query if exact match yields no results

**Keyword Categories:**
| Category | Keywords | Mood | Energy | Tempo |
|----------|----------|------|--------|-------|
| Relaxation | chill, relax, calm, sleep, meditation | calm | low | slow |
| Focus | study, focus, concentration, coding | peaceful/calm | medium/low | medium |
| Workout | workout, gym, running, pump, hype | energetic | high | fast |
| Happy | party, dance, celebration, fun | happy | high | fast |
| Sad | sad, melancholy, emotional, heartbreak | sad/melancholic | low | slow |
| Romantic | romantic, love, dinner, date | romantic | low | slow |
| Aggressive | aggressive, angry, intense, metal | aggressive | high | fast |
| Driving | driving, road trip | happy/energetic | medium | medium |
| Nostalgic | nostalgic, dreamy | nostalgic/dreamy | low | medium/slow |

**Examples:**
| Prompt | Matched Keyword | Query |
|--------|-----------------|-------|
| "chill vibes" | chill | mood=calm, energy=low, tempo=slow |
| "workout playlist" | workout | mood=energetic, energy=high, tempo=fast |
| "music for studying" | study | mood=calm, energy=low, tempo=medium |
| "party music" | party | mood=happy, energy=high, tempo=fast |

**Advantages:**
- Zero Gemini API calls for common vibe requests
- Sub-millisecond response for matched keywords
- Reduces API costs significantly
- Uses existing mood analysis data

**Limitations:**
- Requires songs to have been mood-analyzed by Gemini enrichment
- Limited to predefined keyword list (extensible)
- Doesn't combine mood + genre (e.g., "chill rock" not fully supported yet)

### Tier 2: LLM AI + Smart Indexed Matching

**Functions:** 
- `llm.NewProvider(settings)` → Creates LLM client for configured provider
- `provider.ParsePlaylistFilterWithContext(prompt, availableGenres)` → AI parses prompt with library context
- `tryMatchIndexedGenre(filter)` → Scores indexed genres against AI intent

**✅ Updated (2025-12-30):** Now uses multi-provider LLM abstraction (omnillm SDK) and context-aware prompts.

### Supported Providers

| Provider | Model Examples | API Key Required |
|----------|---------------|------------------|
| **Ollama** (local) | llama3.2:8b, qwen3:4b, mistral:7b | No |
| **Gemini** | gemini-2.0-flash, gemini-1.5-pro | Yes |
| **OpenAI** | gpt-4o-mini, gpt-4o | Yes |
| **Anthropic** | claude-sonnet-4-20250514, claude-3-5-haiku | Yes |
| **X.AI** | grok-4, grok-3 | Yes |

### Context-Aware Prompts ✨ NEW (2025-12-30)

The LLM now receives the user's available genres from their library, helping it select genres that actually exist:

```go
// Get available genres sorted by song count (most popular first)
availableGenres := a.getAvailableGenreNames()

// Create context-aware prompt with user's genres
llmFilter, err = provider.ParsePlaylistFilterWithContext(ctx, prompt, availableGenres)
```

**Before (generic prompt):**
```
You are a music expert. Convert the user's playlist request into JSON.
// LLM returns: {"genres": ["Alt-Rock", "90s Alternative Rock"]}
// Problem: Neither genre exists in user's library
```

**After (context-aware prompt):**
```
You are a music expert. The user's library contains these genres:
Hip-Hop, Rock, Pop, Alternative, Jazz, 90s Alternative, Classic Rock, ...

Convert the user's playlist request into JSON.
PREFER selecting from the user's library genres list.
// LLM returns: {"genres": ["90s Alternative", "Alternative"]}
// Success: Both genres exist in user's library
```

### LLM Prompt Template

```
You are a music expert assistant. Convert the user's natural language 
playlist request into a structured JSON filter.

IMPORTANT: The user's music library contains these genres. 
PREFER selecting from this list when matching their request:
[genre1, genre2, genre3, ...]

OUTPUT FORMAT (output ONLY this JSON, no other text):
{
    "genres": ["genre1", "genre2"],
    "artists": ["artist1"],
    "minYear": 0,
    "maxYear": 0,
    "description": "A short description of the playlist vibe",
    "mood": "",       // happy, sad, energetic, chill, etc.
    "energy": "",     // low, medium, high
    "tempo": "",      // slow, medium, fast
    "occasion": "",   // workout, study, party, etc.
    "instrumental": false
}

GENRE SELECTION RULES:
1. PREFER exact matches from the user's library genres list
2. If the request mentions "90s rock", look for "90s Rock", "90s Alternative"
3. If no exact match, return the closest matching genre from their library
4. Return 1-3 genres maximum, ordered by relevance
```

**Smart Indexed Matching Process:**
1. LLM returns structured filter (genres already aligned with user's library)
2. For each indexed genre, calculate matching score (0-100)
3. Two-pass selection:
   - Pass 1: Find highest scoring genre
   - Pass 2: Among genres within threshold of highest, prefer most songs
4. Accept if best score ≥ 55

### Tier 3: Direct Database Query

**When Used:** Smart indexed matching fails to find ≥55 score match

**Process:**
1. Use Gemini's filter directly
2. Query database with filter criteria:
   ```sql
   SELECT * FROM songs 
   WHERE genre LIKE ? AND year BETWEEN ? AND ?
   ```
3. This is the fallback for truly novel or complex prompts

---

## Genre Scoring Algorithm

**Function:** `scoreGenreMatch(genreName string, filter *gemini.PlaylistFilter, originalPrompt string) int`

The scoring algorithm determines how well an indexed genre matches Gemini's interpreted intent. 

**✅ Updated (2025-12-29):** Now accepts the original user prompt to boost exact subgenre matches.

### Scoring Components

| Component | Max Points | Description |
|-----------|------------|-------------|
| **Subgenre Phrase Match** | 60 | If indexed genre name appears in original prompt (e.g., "jazz trio" → "Jazz Trio") |
| **Near-Exact Phrase Match** | 20 (bonus) | Additional points if genre is ≥50% of prompt length |
| Decade Match | 40 | If genre contains decade (e.g., "90s") overlapping filter's year range |
| Decade Mismatch | -50 | Penalty if genre has decade outside filter's year range |
| Genre Name Match | 30 | If indexed genre contains any filter genre name |
| Word Match | 20 | If indexed genre contains any word from filter genres |
| Variation Match | 25 | If indexed genre contains synonym (e.g., "alt" → "alternative") |

### Subgenre Phrase Matching ✨ NEW (2025-12-29)

The algorithm now detects when the user's original prompt contains an indexed genre name, even if Gemini simplified it.

**Example:** Prompt "upbeat jazz trios"
- Gemini returns `genres: ["Jazz"]` (simplified)
- Indexed genre "Jazz Trio" scores: 60 (phrase match) + 30 (contains "Jazz") = **90**
- Indexed genre "50s Jazz" scores: 40 (decade bonus) + 30 (contains "Jazz") = **70**
- **Result:** "Jazz Trio" wins despite Gemini not extracting it

### Decade Detection

```go
// Detects decade patterns in genre names:
// "90s Alternative" → decadeStart=1990, decadeEnd=1999
// "2000s Hip Hop" → decadeStart=2000, decadeEnd=2009

decadePatterns := []string{
    `\b(19[5-9]\d)s?\b`,  // 1950s-1990s
    `\b(20[0-2]\d)s?\b`,  // 2000s-2020s
    `\b([5-9]0)s\b`,      // 50s-90s (short form)
    `\b(0\d)s\b`,         // 00s, 10s, 20s
}
```

### Genre Variations Map

```go
var variations = map[string][]string{
    "alternative":       {"alt", "alt rock", "alt-rock"},
    "rock":              {"rock", "rock and roll", "rock n roll"},
    "hip hop":           {"hiphop", "hip-hop", "rap"},
    "electronic":        {"electronica", "edm"},
    "indie":             {"indie", "independent"},
    "synthwave":         {"synth wave", "synth-wave", "synthpop"},
    "drum and bass":     {"dnb", "drum n bass", "d&b"},
    // ... 40+ mappings
}
```

### Example Scoring

**Prompt:** "90s alt rock vibes"

**Gemini Filter:**
```json
{
    "genres": ["Alternative Rock"],
    "minYear": 1990,
    "maxYear": 1999,
    "mood": "energetic"
}
```

**Scoring indexed genre "90s Alternative":**
| Check | Points | Reason |
|-------|--------|--------|
| Decade overlap (1990-1999) | +40 | Genre has "90s", filter has 1990-1999 |
| Contains "Alternative" | +30 | Direct match to filter genre |
| Variation "alt" | +25 | Indexed genre variation matches |
| **Total** | **95** | Strong match |

**Scoring indexed genre "80s Rock":**
| Check | Points | Reason |
|-------|--------|--------|
| Decade mismatch | -50 | Genre has "80s", filter wants 1990s |
| No "Alternative" match | 0 | Filter genre not in indexed name |
| Contains "Rock" (word) | +20 | Word match only |
| **Total** | **-30** | Poor match (rejected) |

---

## Multi-Genre Blending

**Function:** `tryMatchMultipleGenres(filter, originalPrompt, maxGenres, targetSongs)`

**When Used:** `blendMode: "mixed"` option selected by user

**✅ Updated (2025-12-30):** Now dynamically widens score threshold for sparse genres.

**Algorithm:**
1. Score all indexed genres against LLM filter **and original prompt**
2. Sort by score descending, then by song count
3. **Determine threshold dynamically:**
   - Default: Take genres within 30 points of highest
   - **If top genre has <30 songs:** Widen threshold to 60 points
   - **If still only 1 genre with insufficient songs:** Force-add next best genre
4. Calculate proportional song distribution based on scores:
   ```go
   proportion := float64(genreScore) / float64(totalScores)
   songsFromGenre := int(proportion * float64(targetSongs))
   ```
5. **Query songs with mood/energy/tempo filtering** (from LLM's parsed filter)
6. Fall back to genre-only query if mood filter returns too few songs
7. Shuffle songs from each genre (Fisher-Yates random)
8. Combine and shuffle final playlist

### Dynamic Threshold for Sparse Genres ✨ NEW (2025-12-30)

When the top-scoring genre has few songs, the algorithm automatically widens its selection:

| Top Genre Songs | Score Threshold | Behavior |
|-----------------|-----------------|----------|
| ≥30 songs | 30 points | Normal: only include closely-scored genres |
| <30 songs | 60 points | Widened: include more related genres |
| 1 genre, <target songs | N/A | Force-add next best genre regardless of score |

**Example: "upbeat classic rock"**

**Before (fixed 30-point threshold):**
- Classic Rock: 130 points, 11 songs → **ONLY SELECTION**
- Rock: 80 points (outside 30-point threshold)
- 60s Rock: 70 points (outside threshold)
- Result: Only 11 songs returned ❌

**After (dynamic threshold):**
- Classic Rock: 130 points, 11 songs (top, but <30 songs)
- Threshold widened to 60 points
- Rock: 80 points (within 60-point threshold) ✅
- 60s Rock: 70 points (within threshold) ✅
- 70s Rock: 70 points (within threshold) ✅
- Result: 3 genres blended, 50+ songs returned ✅

### Mood/Energy/Tempo Filtering (2025-12-29)

When the LLM extracts mood/energy/tempo from the prompt (e.g., "upbeat" → `energy: "high"`), these are now applied to the song query:

```go
// Uses new function that combines genre + mood filtering
songs, err = a.db.GetSongsByExactGenreWithMood(
    g.name,           // e.g., "Jazz Trio"
    filter.MinYear,   // e.g., 0
    filter.MaxYear,   // e.g., 0
    filter.Mood,      // e.g., "happy" (from "upbeat")
    filter.Energy,    // e.g., "high" (from "upbeat")
    filter.Tempo,     // e.g., "fast" (from "upbeat")
)
```

**Fallback Behavior:** If mood filtering is too restrictive (returns fewer songs than needed), the system falls back to genre-only queries to ensure playlist is filled.

**Example:**
- Target: 50 songs
- Matched genres: 90s Alternative (score 90), Grunge (score 75), Indie Rock (score 65)
- Total score: 230
- Distribution:
  - 90s Alternative: 90/230 × 50 ≈ 20 songs
  - Grunge: 75/230 × 50 ≈ 16 songs
  - Indie Rock: 65/230 × 50 ≈ 14 songs

---

## LLM Provider Integration

### Provider Architecture

The AI DJ uses the `omnillm` SDK to abstract across multiple LLM providers. This allows users to choose their preferred provider in Settings.

```go
// backend/internal/llm/provider.go
type Provider struct {
    client       *omnillm.Client
    model        string
    providerName string
}

// Create provider from user settings
provider, err := llm.NewProvider(llmSettings)
defer provider.Close()

// Parse playlist request with context
filter, err := provider.ParsePlaylistFilterWithContext(ctx, prompt, availableGenres)
```

### Configuration

Users configure their LLM provider in Settings:

| Setting | Description |
|---------|-------------|
| `llm_provider` | Provider name: "ollama", "gemini", "openai", "anthropic", "xai" |
| `llm_model` | Model identifier (e.g., "llama3.2:8b", "gpt-4o-mini") |
| `llm_api_key` | API key (not required for Ollama) |
| `ollama_endpoint` | Ollama server URL (default: http://localhost:11434) |

### Concurrency Management

Different providers have different concurrency limits:

| Provider | Optimal Concurrency | Reason |
|----------|---------------------|--------|
| Ollama | 1 | Single GPU, requests queue |
| Cloud APIs | 3 | Rate limits allow parallel |

```go
func (p *Provider) GetOptimalConcurrency() int {
    if p.providerName == ProviderOllama {
        return 1 // Ollama queues on single GPU
    }
    return 3 // Cloud APIs handle concurrency
}
```

### Prompts

All AI prompts are centralized in `backend/internal/llm/prompts.go`:

| Prompt | Purpose |
|--------|---------|
| `PlaylistFilterSystemPrompt` | Basic playlist parsing (no context) |
| `PlaylistFilterContextPromptTemplate` | Context-aware parsing with user's genres |
| `EnrichmentSystemPrompt` | TOON-format metadata enrichment |

---

## Mood & Energy Analysis

### Data Model

Songs table includes mood/energy fields:

```sql
CREATE TABLE songs (
    -- ... other fields ...
    mood TEXT,              -- "happy", "sad", "energetic", etc.
    energy TEXT,            -- "low", "medium", "high"
    tempo TEXT,             -- "slow", "medium", "fast"
    bpm INTEGER,            -- Estimated beats per minute
    instrumental INTEGER,   -- 0=vocals, 1=instrumental
    mood_analyzed_at INTEGER -- Unix timestamp of analysis
);
```

### Analysis Process

**Function:** `gemini.AnalyzeSongMood(songs []db.Song)`

**Gemini Prompt:**
```
You are a music expert. Analyze the mood, energy level, tempo, and 
vocal presence of each song based on the artist, title, album, and genre.

OUTPUT: JSON object mapping song ID to:
- "mood": One of: happy, sad, energetic, calm, melancholic, uplifting, 
          aggressive, romantic, chill, intense, dreamy, nostalgic
- "energy": One of: low, medium, high
- "tempo": One of: slow, medium, fast
- "bpm": Estimated BPM as integer
- "instrumental": true if no vocals, false otherwise

ANALYSIS GUIDELINES:
- Consider the artist's typical sound and style
- Consider the genre's typical characteristics
- Consider the song title's emotional implications
- For unknown songs, use genre conventions as guidance

Songs to analyze:
ID: abc123 | Artist: Radiohead | Title: Creep | Album: Pablo Honey | Genre: Alternative
```

### Batch Processing

The mood analysis runs as a background job:
1. `GetSongsWithoutMood(limit)` fetches unanalyzed songs
2. Songs are batched (typically 20 per API call)
3. Progress emitted via SSE for UI updates
4. Results stored with `UpdateSongMood()`

### Current Usage

~~**IMPORTANT:** Mood/energy fields are **stored but not yet used** for playlist filtering.~~

**✅ FIXED (2025-12-29):** Mood/energy/tempo fields are now used in playlist filtering:

1. **Tier 1.5 (tryMoodBasedMatch):** For simple prompts like "chill" or "workout", queries songs directly by mood fields
2. **Multi-Genre Blending (tryMatchMultipleGenres):** When Gemini extracts mood/energy/tempo from complex prompts, `GetSongsByExactGenreWithMood()` applies these filters within each matched genre
3. **Fallback Behavior:** If mood filtering is too restrictive, the system falls back to genre-only queries to ensure results

---

## Play History Filters

**Function:** `applyPlayHistoryFilters(songs, recentlyPlayedIDs, discoverMode, onePerArtist, limit)`

### Filter Options

| Option | Description |
|--------|-------------|
| `discoverMode: "balanced"` | Shuffle songs randomly |
| `discoverMode: "discover"` | Sort by play_count ascending (underplayed first) |
| `discoverMode: "favorites"` | Sort by play_count descending (frequently played first) |
| `avoidRecentlyHours: N` | Exclude songs played within last N hours |
| `onePerArtist: true` | Limit to one song per artist for variety |

### Implementation

```go
func applyPlayHistoryFilters(songs []any, recentlyPlayedIDs map[string]bool, 
                             discoverMode string, onePerArtist bool, limit int) []any {
    
    // 1. Filter out recently played
    if len(recentlyPlayedIDs) > 0 {
        filtered := []any{}
        for _, s := range songs {
            if id, ok := getSongID(s); ok && !recentlyPlayedIDs[id] {
                filtered = append(filtered, s)
            }
        }
        songs = filtered
    }

    // 2. One-per-artist filter
    if onePerArtist {
        seenArtists := make(map[string]bool)
        filtered := []any{}
        for _, s := range songs {
            if artist, ok := getArtistName(s); ok && !seenArtists[artist] {
                seenArtists[artist] = true
                filtered = append(filtered, s)
            }
        }
        songs = filtered
    }

    // 3. Sort by discover mode
    switch discoverMode {
    case "discover":
        sortSongsByPlayCount(songs, true)  // Ascending
    case "favorites":
        sortSongsByPlayCount(songs, false) // Descending
    default: // "balanced"
        shuffle(songs)
    }

    // 4. Limit to target
    if len(songs) > limit {
        songs = songs[:limit]
    }

    return songs
}
```

---

## Time-Aware Context

**Function:** `enhancePromptWithTimeContext(prompt string, useTimeContext bool)`

When `useTimeContext: true`, the system adds time-of-day hints to the LLM prompt.

### Time Periods

| Period | Hours | Suggested Mood | Suggested Energy |
|--------|-------|----------------|------------------|
| Morning | 5-11 | uplifting, peaceful | low to medium |
| Afternoon | 12-17 | energetic, focused | medium to high |
| Evening | 18-21 | relaxed, social | medium |
| Night | 22-4 | calm, introspective | low |

### Weekend Adjustment

On weekends (Saturday/Sunday), energy suggestions shift up one level.

### Prompt Enhancement

```go
func enhancePromptWithTimeContext(prompt string, useTimeContext bool) string {
    if !useTimeContext {
        return prompt
    }

    ctx := getTimeContext()
    return fmt.Sprintf("%s\n\n[Context: It's %s (%s). Suggested mood: %s, energy: %s]",
        prompt, ctx.TimeOfDay, ctx.DayType, ctx.SuggestedMood, ctx.SuggestedEnergy)
}
```

---

## Caching Strategy

### Filter Cache

**Location:** `gemini.Client.filterCache`

**Purpose:** Avoid duplicate API calls for similar prompts

**Configuration:**
- TTL: 15 minutes
- Max entries: 100
- Eviction: LRU-like (oldest entry removed)

**Cache Key:** Lowercase, trimmed prompt text

**Response Indicator:** `filter.FromCache: true` when cache hit

### Why Not Cache Songs?

Songs are NOT cached because:
1. Play history filters depend on real-time data
2. One-per-artist filter is stateful
3. Library may change between requests
4. Cache invalidation would be complex

---

## Known Shortcomings

### 1. ~~Mood/Energy Fields Not Used in Filtering~~ ✅ FIXED (2025-01-13)

**Issue:** ~~Gemini extracts mood, energy, tempo, occasion from prompts, and songs are analyzed for these fields, but the actual database query does NOT filter by them.~~

**Status:** **FIXED** - `GetSongsBySmartFilter()` now accepts and filters by mood, energy, and tempo parameters.

**Implementation:**
- Extended function signature: `GetSongsBySmartFilter(genres, artists, minYear, maxYear, mood, energy, tempo)`
- Added SQL WHERE clauses for exact matching when these filters are provided
- Updated `handleGenerateSmartPlaylist()` to pass Gemini's mood/energy/tempo to the query

**Note:** Prompts like "relaxing music for studying" now return results filtered by mood="calm" (if Gemini extracts it). Songs without mood analysis will be excluded when mood filters are active.

### 2. ~~Pseudo-Random Shuffling~~ ✅ FIXED (2025-01-13)

**Issue:** ~~Shuffle uses deterministic pseudo-random (`(i * 17) % (i + 1)`), not true randomness.~~

**Status:** **FIXED** - All shuffle operations now use `math/rand` with time-based seeding.

**Implementation:**
- Added `math/rand` import with `rand.Seed(time.Now().UnixNano())` in `init()`
- Replaced all pseudo-random shuffles with proper Fisher-Yates using `rand.Intn()`
- Affected locations: multi-genre song shuffle, combined playlist shuffle, balanced mode shuffle

### 3. No Audio Analysis

**Issue:** Mood/BPM estimation is metadata-based only. Gemini guesses from artist reputation and genre conventions.

**Impact:** Accuracy varies. A fast punk song might be tagged "high energy" correctly, but a slow ballad by the same artist might inherit assumptions.

**Workaround:** This is intentional—no DSP/FFT needed, but limits precision.

### 4. ~~Genre Scoring Edge Cases~~ Partially Fixed (2025-12-29)

**Issue:** ~~Scoring relies on string matching and decade detection. Creative genre names may not score well.~~

**Status:** **Partially Fixed** - Added subgenre phrase matching from original prompt.

**What's Fixed:**
- ✅ "upbeat jazz trios" now correctly matches "Jazz Trio" (phrase matching)
- ✅ Indexed subgenres appearing in user prompt get +60-80 bonus

**Remaining Issues:**
- "Shoegaze-inspired Indie" might not match "90s Shoegaze" well (variation map helps but imperfect)
- "Psychedelic" still scores poorly against "70s Psych Rock" if not in variations map

### 5. Artist Similarity is Genre-Only

**Issue:** "More like Radiohead" finds artists sharing genres, but doesn't understand sound/influence.

**Impact:** Results may include artists with same genre tags but vastly different sounds.

### 6. No Streaming/Preview Integration

**Issue:** AI DJ only works with local library. Cannot preview or suggest songs you don't own.

**Workaround:** Could integrate Spotify preview API for discovery, but currently out of scope.

### 7. API Rate Limits

**Issue:** Cloud LLM providers have rate limits. Heavy usage may hit 429 errors.

**Mitigation:** 
- Retry logic with exponential backoff
- 15-minute filter cache
- Use Ollama (local) for unlimited requests with no rate limiting

### 8. Single-Language Support

**Issue:** Prompts and genre matching assume English. Non-English prompts or genre names may fail.

### 9. Decade Extraction Limitations

**Issue:** Decade detection is regex-based. Complex phrasings may not be caught.

**Examples:**
- ✅ "90s rock" → 1990-1999
- ✅ "early 90s" → 1990-1993
- ❌ "music from the nineties" → not detected
- ❌ "post-millennium" → not detected

### 10. ~~No User Preference Learning~~ ✅ FIXED (2025-01-13)

**Issue:** System doesn't learn from user behavior. Each session starts fresh.

**Fix:** Implemented preference tracking infrastructure:
- `listening_events` table tracks play completions and skip timing
- `genre_preferences` table aggregates affinity scores per genre
- API endpoint `POST /api/songs/{id}/listen-event` records events
- Frontend integration complete (call API on skip/complete)

### 11. ~~SQL ESCAPE Clause Bug~~ ✅ FIXED (2025-01-13)

**Issue:** Genre queries were failing with "ESCAPE expression must be a single character" error.

**Root Cause:** In `GetSongsByExactGenre()` (genres.go), the SQL query used raw string literals (backticks) with `ESCAPE '\\'`. In Go raw strings, escape sequences are NOT processed, so `'\\'` produced two backslashes instead of one.

**Impact:** AI DJ playlists could not be created - all genre-based queries failed.

**Fix:** Changed `ESCAPE '\\'` to `ESCAPE '\'` in raw string literals in genres.go.

### 12. ~~DJ Mode Not Filtering By Prompt~~ ✅ FIXED (2025-12-31)

**Issue:** DJ mode was returning songs from the entire library regardless of prompt content.

**Root Cause:** The `handleDJMode()` function in `smart_playlist.go` was:
1. Passing ALL songs to the sequencer without filtering by prompt-derived genres
2. Setting `SeedGenres` to empty array `[]string{}`
3. Not using the existing matching tiers (local genre, LLM filter) to extract intent

**Impact:** "90s west coast hip-hop bangers" returned completely unrelated songs.

**Fix:** 
1. Added `tryLocalGenreMatch()` call to extract matching genres from prompt
2. Added `tryMoodBasedMatchInfo()` call to extract mood/energy/tempo hints
3. Added LLM filter parsing via `ParsePlaylistFilterWithContext()` to get genres/artists
4. Added filtering logic to match candidates by seed genres/artists before passing to sequencer
5. Fallback to all songs only if filtered candidates < 10
6. Added 15+ log statements for complete request traceability

**Files Modified:**
- `backend/internal/api/smart_playlist.go`: Added `moodMatchInfo` type, `tryMoodBasedMatchInfo()` function, comprehensive filtering in `handleDJMode()`

See [Preference Learning System](#preference-learning-system) for details.

---

## Preference Learning System

### Overview

The preference learning system tracks user listening behavior to personalize AI DJ recommendations over time.

### Database Schema

```sql
-- Detailed listening events
CREATE TABLE listening_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    song_id TEXT NOT NULL,
    event_type TEXT NOT NULL,  -- 'play_complete', 'skip_early', 'skip_mid', 'skip_late'
    play_duration REAL,        -- seconds played
    song_duration REAL,        -- total duration
    timestamp INTEGER NOT NULL,
    genre TEXT,
    mood TEXT,
    energy TEXT,
    context TEXT              -- 'ai_dj', 'album', 'playlist', 'queue', 'search'
);

-- Aggregated genre preferences
CREATE TABLE genre_preferences (
    genre TEXT PRIMARY KEY,
    play_count INTEGER DEFAULT 0,
    skip_count INTEGER DEFAULT 0,
    complete_rate REAL DEFAULT 0.5,    -- 0.0-1.0
    skip_early_rate REAL DEFAULT 0,     -- 0.0-1.0
    affinity_score REAL DEFAULT 0.5,    -- 0.0-1.0
    last_updated INTEGER
);
```

### Event Types

| Event Type | Condition | Signal |
|------------|-----------|--------|
| `play_complete` | playDuration ≥ 90% of songDuration | Strong positive |
| `skip_late` | playDuration > 30s but < 90% | Mild positive |
| `skip_mid` | playDuration 10-30s | Neutral/mild negative |
| `skip_early` | playDuration < 10s | Strong negative |

### Affinity Scoring Algorithm

```go
affinityScore = completeRate - (skipEarlyRate * 0.3)
// Range: 0.0 (avoided) to 1.0 (loved)
```

- Early skips penalized more heavily than late skips
- Genres with high completion rates get boosted
- Score recalculated after each listening event

### API Endpoint

**POST /api/songs/{id}/listen-event**

Request:
```json
{
  "playDuration": 15.5,
  "songDuration": 180.0,
  "context": "ai_dj"
}
```

Response:
```json
{
  "status": "ok",
  "eventType": "skip_mid"
}
```

### Future Integration

The affinity scores can be used to:
1. Boost/penalize genres in AI DJ ranking
2. Pre-filter low-affinity genres in certain contexts
3. Personalize mood detection thresholds
4. Provide "taste profile" insights in UI

### Frontend Integration (✅ DONE 2025-01-13)

The `useAudioPlayer.ts` hook now tracks listening events:
- Tracks accumulated play duration across pause/resume
- Records listen event when song changes (skip) or ends (complete)
- Calls backend API with play duration and song duration
- Backend auto-detects event type from ratio

**Files Modified:**
- `hooks/useAudioPlayer.ts` - Added `listenTrackingRef` for duration tracking
- `slices/types.ts` - Added `PlaybackContext` type
- `slices/librarySlice.ts` - Added `recordListenEvent()` function
- `services/api.ts` & `services/backendService.ts` - API wrapper

---

## Improvement Opportunities

### Short-Term

1. ~~**Enable Mood/Energy Filtering**~~ ✅ DONE (2025-01-13)
   - ~~Extend `GetSongsBySmartFilter()` to include mood/energy WHERE clauses~~
   - Add UI toggles for mood-based filtering (optional enhancement)

2. ~~**True Random Shuffling**~~ ✅ DONE (2025-01-13)
   - ~~Replace deterministic pseudo-random with `rand.Seed(time.Now().UnixNano())`~~
   - Using Fisher-Yates shuffle with `math/rand`

3. **Enhanced Decade Detection**
   - Add regex for "nineties", "two-thousands", "post-millennium"
   - Parse explicit year mentions: "2015 hits"

### Medium-Term

4. **Audio-Based BPM Detection**
   - Optional FFT analysis for actual BPM (requires audio file access)
   - Could run during background enrichment

5. ~~**User Preference Tracking**~~ ✅ DONE (2025-01-13)
   - ~~Track skip rates, completion percentages, explicit likes~~
   - Backend and frontend integration complete
   - See [Preference Learning System](#preference-learning-system)

6. **Cross-Artist Similarity via Embeddings**
   - Use artist embeddings (from Spotify or MusicBrainz) for better "more like" results

### Long-Term

7. ~~**Local LLM Option**~~ ✅ DONE (2025-12-30)
   - ~~Allow users to run local models (Ollama, LLaMA) instead of Gemini~~
   - Implemented via omnillm SDK with Ollama as default provider
   - Users can configure in Settings → AI Provider
   - See [LLM Provider Integration](#llm-provider-integration)

8. **Playlist "Evolution"**
   - Continuous DJ mode that adapts based on what's playing
   - Skip detection adjusts upcoming songs (infrastructure now in place)

---

## Testing Recommendations

### Unit Tests

| Test Case | Expected |
|-----------|----------|
| Prompt "90s Alternative" | Tier 1 local match, no API call |
| Prompt "more like Radiohead" | Tier 0 artist match |
| Prompt "upbeat workout music" | Tier 2 Gemini → indexed match |
| Prompt "xyz123nonsense" | Tier 3 fallback, likely empty |
| BlendMode "mixed" | Multiple genres in matchedGenres |
| DiscoverMode "discover" | Songs sorted by play_count ascending |
| AvoidRecentlyHours 24 | Recently played excluded |

### Integration Tests

1. **Cache Behavior:** Same prompt twice → second has `fromCache: true`
2. **Rate Limit Recovery:** Simulate 429 → retry succeeds
3. **Empty Library:** Graceful handling, error message

---

## Summary

The AI DJ is a sophisticated four-tier matching system that balances speed (local matching) with intelligence (multi-provider LLM support). Key features include:

- **Multi-provider LLM support** (2025-12-30): Ollama (local), Gemini, OpenAI, Anthropic, and X.AI all supported via omnillm SDK. Ollama is the default for privacy-focused local processing.
- **Context-aware prompts** (2025-12-30): LLM receives the user's available genres, helping it select genres that actually exist in their library.
- **Dynamic genre blending** (2025-12-30): For sparse genres (<30 songs), the system automatically widens its selection threshold to include more related genres.
- **Minimum song threshold** (2025-12-30): Local matching requires ≥20 songs to ensure diverse playlists.
- **Subgenre phrase matching** (2025-12-29): User prompt phrases like "jazz trios" correctly match indexed genres like "Jazz Trio" even when the LLM simplifies them.
- **Mood/energy/tempo filtering** (2025-12-29): Prompts like "upbeat" now filter songs by energy level, using the mood analysis data already stored in the database.
- **Preference learning** (2025-01-13): User listening behavior (skips, completions) is tracked to enable future personalization.
- **True random shuffling** (2025-01-13): Fisher-Yates algorithm with time-based seeding ensures varied results.

The architecture is extensible and most major gaps have been addressed. Remaining improvements focus on advanced features like audio-based BPM detection and cross-artist embeddings.
