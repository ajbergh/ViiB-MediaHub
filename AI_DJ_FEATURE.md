# AI DJ Feature Documentation

> **Last Updated:** Phase 2 Complete (Play History, Artist Preferences, Time-of-Day Awareness)
> 
> **Implementation Status:** Phases 1 & 2 complete, Phase 3+ pending
> 
> **Key Files:**
> - `backend/internal/api/smart_playlist.go` - Main handler and matching logic
> - `backend/internal/db/db.go` - Play history and mood queries  
> - `backend/internal/gemini/gemini.go` - AI filter generation and mood analysis
> - `pages/SmartPlaylists.tsx` - Frontend UI
> - `services/api.ts` - API client

## Overview

The AI DJ is an intelligent playlist generation feature that creates custom playlists from your local music library based on natural language prompts. Users can describe a vibe, genre, era, or mood, and the AI DJ will build a playlist matching that description.

## How It Works

The AI DJ uses a **three-tier matching system** to process user prompts and find matching songs:

```
┌─────────────────────────────────────────────────────────────┐
│                      User Prompt                            │
│                  "90s alt rock vibes"                       │
└─────────────────────────────────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────┐
│              TIER 0: Artist-Based Matching                  │
│  • Detects "more like [artist]" patterns                    │
│  • Returns songs from artist + similar artists              │
│  • Similarity based on shared genres                        │
└─────────────────────────────────────────────────────────────┘
                           │
                    (if no match)
                           ▼
┌─────────────────────────────────────────────────────────────┐
│               TIER 1: Local Genre Matching                  │
│  • Exact match: "90s Alternative" → direct hit              │
│  • Smart partial match with 60%+ threshold                  │
│  • No AI needed - instant response                          │
└─────────────────────────────────────────────────────────────┘
                           │
                    (if no match)
                           ▼
┌─────────────────────────────────────────────────────────────┐
│            TIER 2: Gemini AI + Smart Matching               │
│  1. Gemini parses prompt → filter (genres, years, mood)     │
│  2. Smart matching scores indexed genres against intent     │
│  3. Two-pass selection prioritizes song count               │
└─────────────────────────────────────────────────────────────┘
                           │
               (if indexed genre found)
                           ▼
┌─────────────────────────────────────────────────────────────┐
│              TIER 3: Direct Database Query                  │
│  • Uses Gemini's filter for complex/novel prompts           │
│  • Falls back when no indexed genre matches                 │
└─────────────────────────────────────────────────────────────┘
```

---

## Tier 1: Local Genre Matching

**Purpose:** Handle exact genre names instantly without AI calls.

**Process:**
1. Normalize user prompt (lowercase, trim)
2. Check for exact match against indexed genre names
3. Check for significant partial matches (genre name must be 60%+ of prompt length)

**Examples:**
| Prompt | Match Type | Result |
|--------|------------|--------|
| "90s Alternative" | Exact | Returns 90s Alternative genre songs |
| "Jazz" | Exact | Returns Jazz genre songs |
| "Hip Hop" | Exact | Returns Hip Hop genre songs |

**Code Location:** `tryLocalGenreMatch()` in `smart_playlist.go`

---

## Tier 2: Gemini AI + Smart Indexed Matching

When local matching fails, we use Google's Gemini AI to understand the prompt's intent, then intelligently match against indexed genres.

### Step 1: Gemini Prompt Parsing

The prompt is sent to Gemini with a structured request to extract:

```json
{
  "genres": ["Alternative Rock"],
  "artists": null,
  "minYear": 1990,
  "maxYear": 1999,
  "mood": "energetic",
  "energy": "high",
  "tempo": "medium",
  "description": "A playlist of alternative rock from the 1990s"
}
```

**Gemini Prompt Template:**
```
Analyze this music request and extract structured filter criteria:
"90s alt rock vibes"

Return JSON with: genres (array), artists (array or null), 
minYear/maxYear (integers), mood, energy, tempo, description
```

### Step 2: Smart Indexed Genre Scoring

Each indexed genre is scored against Gemini's filter to find the best match.

#### Scoring Algorithm

```go
score = 0

// Decade Matching (max 40 points)
if genre has decade (e.g., "90s", "2000s") {
    if decade overlaps with filter's year range:
        score += 40  // Strong decade match
    else:
        score -= 50  // Wrong decade penalty
}

// Genre Name Matching (max 30 points)
for each filter.genre:
    if indexed_genre contains filter_genre:
        score += 30  // Direct name match

// Word Matching (max 20 points)
for each word in filter_genre:
    if indexed_genre contains word:
        score += 20  // Partial word match
        break

// Variation Matching (max 25 points)
// Maps: "alt" → "alternative", "hiphop" → "hip hop", etc.
for each variation:
    if indexed_genre contains variation:
        score += 25
        break

// Curated Genre Bonus (10 points)
if genre has decade AND score > 0:
    score += 10  // Prefer curated decade-specific genres
```

#### Scoring Examples

| Query | Gemini Returns | Indexed Genre | Score Breakdown | Total |
|-------|----------------|---------------|-----------------|-------|
| "90s alt rock" | genres=["Alternative Rock"], years=1990-1999 | 90s Alternative | 40 (decade) + 20 (word "alternative") + 10 (bonus) | **70** |
| "90s alt rock" | genres=["Alternative Rock"], years=1990-1999 | 90s Alternative Rock | 40 (decade) + 30 (name match) + 20 (word) + 10 (bonus) | **100** |
| "90s alt rock" | genres=["Alternative Rock"], years=1990-1999 | Alternative Rock | 30 (name match) | **30** |
| "90s alt rock" | genres=["Alternative Rock"], years=1990-1999 | Rock | 0 (no matching criteria) | **0** |

### Step 3: Two-Pass Selection

To avoid selecting low-content genres, we use a two-pass algorithm:

**Pass 1:** Find the highest score among all indexed genres.

**Pass 2:** Among all genres within 30 points of the highest score, select the one with the most songs.

```
Example:
- 90s Alternative Rock: 100 points, 3 songs
- 90s Alternative: 70 points, 145 songs
- Grunge: 75 points, 83 songs

Highest score: 100
Within 30 points (70+): All three qualify
Selected: 90s Alternative (most songs: 145)
```

**Code Location:** `tryMatchIndexedGenre()` and `scoreGenreMatch()` in `smart_playlist.go`

---

## Tier 3: Direct Database Query

When no indexed genre matches well (score < 55), we fall back to querying the database directly using Gemini's filter criteria.

**Query Construction:**
```sql
SELECT * FROM songs 
WHERE genre LIKE '%Alternative%'  -- From filter.genres
  AND year >= 1990               -- From filter.minYear
  AND year <= 1999               -- From filter.maxYear
LIMIT 50
```

---

## Genre Variations Mapping

The system understands common abbreviations and synonyms:

| Input | Matches |
|-------|---------|
| alt | alternative, indie |
| hiphop, hip-hop | hip hop, rap |
| rnb | r&b, rhythm and blues |
| prog | progressive |
| psych | psychedelic |
| grunge | seattle |
| synthwave | synth wave, synthpop |
| dnb | drum and bass |
| britpop | brit pop |

---

## Current Limitations

1. **Single Genre Focus:** Currently returns songs from one best-matching genre, not a blend
2. **No Artist Weighting:** Doesn't factor in artist preferences from listening history
3. **Limited Mood Understanding:** Mood/energy/tempo from Gemini aren't fully utilized
4. **No Cross-Genre Blending:** Can't create playlists spanning multiple genres
5. **Static Scoring:** Weights are hardcoded, not personalized
6. **No Learning:** System doesn't learn from user feedback

---

## API Endpoint

```
POST /api/smart-playlist

Request:
{
  "prompt": "90s alternative rock vibes"
}

Response:
{
  "filter": {
    "genres": ["90s Alternative"],
    "description": "Songs from the 90s Alternative genre",
    "localMatch": true
  },
  "songs": [
    { "id": "...", "title": "...", "artist": "...", ... }
  ]
}
```

---

# Improvement Plan

## Phase 1: Enhanced Matching (Week 1-2)

### 1.1 Multi-Genre Blending ✅ COMPLETED
**Goal:** Allow playlists to span multiple matching genres.

**Tasks:**
- [x] Modify scoring to return top 3 matching genres instead of just 1
- [x] Blend songs from multiple genres proportionally to their scores
- [x] Add `blendMode` option: "single" (current) or "mixed"
- [x] Update response to include `matchedGenres` array
- [x] Add UI toggle for Single Genre vs Multi-Genre Mix
- [x] Display genre proportions in UI when blended

**Implementation:**
- Added `tryMatchMultipleGenres()` function that returns top 3 genres within 30 points of highest score
- Proportional song selection based on genre scores
- Shuffle algorithm for natural mixing of genres
- New `MatchedGenre` struct with name, score, songCount, proportion
- API now accepts `blendMode` and `targetSongs` parameters
- Frontend UI with toggle buttons for blend mode selection

**Example:**
```
"90s rock vibes" → Returns songs from:
- 90s Alternative (40%)
- 90s Alternative Rock (30%)
- Grunge (30%)
```

### 1.2 Mood/Energy Integration ✅ COMPLETE
**Goal:** Use Gemini's mood and energy analysis to filter within genres.

**Tasks:**
- [x] Add mood/energy/tempo fields to song database (mood, energy, tempo, bpm, mood_analyzed_at)
- [x] Create enrichment job to analyze songs with Gemini (`AnalyzeSongMood()`)
- [x] Add API endpoints for mood enrichment (`/library/enrich-mood/stream`)
- [x] Add database query function (`GetSongsWithoutMood()`)
- [x] Add frontend API method (`enrichMoodStream()`)
- [x] Add Settings UI button to trigger mood analysis with progress bar

**Implementation Details:**
- **Approach:** Uses Gemini AI to analyze mood/energy/tempo/BPM based on song metadata (artist, title, album, genre)
- **Rationale:** Gemini understands genre conventions and artist styles, providing accurate estimates without needing actual audio analysis
- **Database:** Added `mood`, `energy`, `tempo`, `bpm`, `mood_analyzed_at` columns to songs table
- Gemini: Added `AnalyzeSongMood()` function that returns mood analysis per song
- API: `/library/enrich-mood/stream` SSE endpoint for batch mood analysis
- Frontend: `api.enrichMoodStream()` method for triggering mood analysis

### 1.3 Improved Decade Extraction ✅ COMPLETED
**Goal:** Better decade parsing from natural language.

**Tasks:**
- [x] Handle "early 90s" (1990-1993), "late 90s" (1997-1999), "mid 90s" (1994-1996)
- [x] Support "turn of the century" (1998-2002)
- [x] Handle "modern" (last 5 years), "classic" (>20 years old)
- [x] Support year ranges like "2015-2020"

**Implementation:**
- Updated `extractDecadeFromPrompt()` to detect "early", "mid", "late" modifiers
- Added special phrase support: "turn of the century", "y2k", "modern", "recent", "classic", "retro", "vintage"
- Added regex for explicit year ranges like "2015-2020" or "2015 to 2020"
- Decade modifiers: early = base to base+3, mid = base+3 to base+6, late = base+6 to base+9

---

## Phase 2: Context & Personalization (Week 3-4)

### 2.1 Play History Integration ✅ COMPLETE
**Goal:** Weight songs based on user listening history.

**Tasks:**
- [x] Track song play counts and last played date (`plays` table already exists)
- [x] Create `GetRecentlyPlayedSongIDs()` and `GetFrequentlyPlayed()` functions in `db.go`
- [x] Add weighting options: "discover new" vs "familiar favorites" (discoverMode: balanced/discover/favorites)
- [x] Implement "avoid recently played" filter (avoidRecentlyHours option in UI)
- [x] Add "one per artist" option to increase variety
- [x] UI controls added to SmartPlaylists.tsx: Discovery Mode buttons, Avoid Recently dropdown, One Per Artist checkbox

**Implementation Details:**
- Backend: `applyPlayHistoryFilters()` in `smart_playlist.go`
- Database: `GetRecentlyPlayedSongIDs(hours)`, `GetFrequentlyPlayed(limit)`, `GetUnderplayedSongs(maxPlays, limit)`
- API options: `discoverMode`, `avoidRecentlyHours`, `onePerArtist`

### 2.2 Artist Preferences ✅ COMPLETE
**Goal:** Incorporate artist preferences into scoring.

**Tasks:**
- [x] Track favorite artists based on play count (`GetFavoriteArtists()`, `GetArtistPlayStats()`)
- [x] Add artist affinity scoring to genre matching (`getArtistAffinityBonus()`)
- [x] Support "more like [artist]" prompts (`tryArtistBasedMatch()`)
- [x] Implement artist variety controls (one song per artist option) - done in Phase 2.1

**Implementation Details:**
- Prompt patterns detected: "more like X", "songs like X", "similar to X", "music like X", "X style music"
- Database functions: `GetSongsByArtist()`, `GetSimilarArtists()` (finds artists with shared genres)
- Processing order: Artist match → Local genre match → Gemini AI fallback

### 2.3 Time-of-Day Awareness ✅ COMPLETE
**Goal:** Adjust recommendations based on context.

**Tasks:**
- [x] Morning: energetic, uplifting (5am-12pm)
- [x] Afternoon: varied, productive (12pm-5pm)
- [x] Evening: relaxing, mellow (5pm-11pm)
- [x] Night: chill, low energy (11pm-5am)
- [x] Weekend vs weekday patterns (weekends: more relaxed mornings, party vibes evening)
- [x] UI toggle to enable/disable time-aware mode

**Implementation Details:**
- Backend: `getTimeContext()` returns period, mood, energy suggestions
- Backend: `enhancePromptWithTimeContext()` adds subtle hints to Gemini prompts
- Only applied when `useTimeContext` is enabled AND user hasn't specified time context
- Detects existing time keywords (morning, party, workout, etc.) to avoid overriding user intent
- UI: Sun icon toggle with amber highlight when active

---

## Phase 3: Advanced AI Features (Week 5-6)

### 3.1 Conversational Refinement
**Goal:** Allow users to refine playlists through follow-up prompts.

**Tasks:**
- [ ] Maintain conversation context across requests
- [ ] Support "more upbeat", "less heavy", "add some jazz"
- [ ] Track rejected songs and avoid them
- [ ] Implement "more like this" for individual songs

### 3.2 Audio Feature Analysis
**Goal:** Analyze actual audio for better matching.

**Tasks:**
- [ ] Extract BPM from audio files
- [ ] Detect key/scale for harmonic mixing
- [ ] Analyze energy levels from waveform
- [ ] Store audio features in database

### 3.3 Collaborative Filtering
**Goal:** Learn from aggregate user behavior.

**Tasks:**
- [ ] (Privacy-preserving) Track successful playlist patterns
- [ ] Identify genre co-occurrence patterns
- [ ] Weight scoring based on common transitions
- [ ] Suggest "users who liked X also liked Y"

---

## Phase 4: UI/UX Enhancements (Week 7-8)

### 4.1 Real-Time Preview
**Goal:** Stream preview while building playlist.

**Tasks:**
- [ ] Show songs appearing as they're selected
- [ ] Play 15-second previews automatically
- [ ] Allow skip/keep feedback during generation
- [ ] Save feedback for future refinement

### 4.2 Visual Playlist Builder
**Goal:** Interactive visualization of playlist creation.

**Tasks:**
- [ ] Show genre distribution pie chart
- [ ] Display decade timeline
- [ ] Visualize energy/tempo curve
- [ ] Drag-to-reorder with smart suggestions

### 4.3 Natural Language Feedback
**Goal:** Accept natural language feedback on results.

**Tasks:**
- [ ] "Too much Nirvana" → reduce artist weight
- [ ] "Missing the vibe" → adjust genre scoring
- [ ] "Perfect!" → save preferences
- [ ] Learn from skip patterns

---

## Phase 5: Advanced Playlist Types (Week 9-10)

### 5.1 Workout/Activity Playlists
**Goal:** Create playlists optimized for activities.

**Tasks:**
- [ ] Define BPM targets for activities (running: 160-180, yoga: 60-80)
- [ ] Build energy curves (warm up → peak → cool down)
- [ ] Duration targeting (30 min, 60 min playlists)
- [ ] Crossfade suggestions for seamless transitions

### 5.2 Mood Journey Playlists
**Goal:** Create emotional arc playlists.

**Tasks:**
- [ ] Define mood progressions (calm → energetic → calm)
- [ ] Map songs to mood coordinates
- [ ] Optimize song ordering for emotional flow
- [ ] Support "take me from sad to happy"

### 5.3 Discovery Mode
**Goal:** Surface underplayed music from library.

**Tasks:**
- [ ] Identify "hidden gems" (good match, low play count)
- [ ] Create "rediscover your library" playlists
- [ ] Highlight songs not played in 6+ months
- [ ] Mix familiar and undiscovered tracks

---

## Implementation Progress

| Phase | Feature | Status |
|-------|---------|--------|
| 1.1 | Multi-Genre Blending | ✅ Complete |
| 1.2 | Mood/Energy Integration | ✅ Complete |
| 1.3 | Improved Decade Extraction | ✅ Complete |
| 2.1 | Play History Integration | ✅ Complete |
| 2.2 | Artist Preferences | ✅ Complete |
| 2.3 | Time-of-Day Awareness | ✅ Complete |
| 3.x | Advanced AI Features | ⏳ Not Started |
| 4.x | UI/UX Enhancements | ⏳ Not Started |
| 5.x | Advanced Playlist Types | ⏳ Not Started |

---

## Success Metrics

| Metric | Current | Phase 1 Target | Phase 5 Target |
|--------|---------|----------------|----------------|
| Prompt → Playlist success rate | 75% | 90% | 98% |
| Average songs returned | 50 | 50 | 25-100 (configurable) |
| User skip rate | Unknown | <20% | <10% |
| Genre accuracy | 80% | 95% | 99% |
| Response time | 2-5s | 1-2s | <500ms (cached) |

---

## Technical Debt

- [ ] Move `PlaylistFilter` to shared types package
- [ ] Add comprehensive unit tests for scoring algorithm
- [ ] Cache Gemini responses for common prompts
- [ ] Add request rate limiting for Gemini API
- [ ] Implement proper error handling and user feedback
- [ ] Add telemetry for debugging matching issues

---

*Last Updated: December 2025*
*Phase 1 Progress: 90% (2.8/3 tasks complete)*
