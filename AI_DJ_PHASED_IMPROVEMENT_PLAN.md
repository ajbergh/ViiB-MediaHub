# AI DJ – Phased Improvement Plan (Golang Backend)

> **Last Reviewed:** 2025-01-13
> 
> This plan explicitly **respects the current implementation**, builds in **phases**, and avoids "big bang" rewrites. Each phase delivers value on its own and reduces API cost progressively.

---

## Implementation Review Notes (2025-01-13)

Based on detailed analysis of the existing codebase, here are corrections, clarifications, and suggested improvements to this plan:

### Current State Corrections

1. **Tier Structure is 0-3, not 0-2**: The plan correctly identifies Tiers 0-3, but the "Tier 3 – Relaxed fallback queries" description should clarify this is `GetSongsBySmartFilter()` using Gemini's raw filter when smart indexed matching fails.

2. **Mood/Energy Already Stored**: The database schema ALREADY includes `mood`, `energy`, `tempo`, `bpm`, `instrumental`, `mood_analyzed_at` columns. The `AnalyzeSongMood()` function in Gemini package populates these. The gap is that `GetSongsBySmartFilter()` doesn't USE them.

3. **Filter Cache Exists**: The Gemini client already has a 15-minute, 100-entry LRU filter cache. This reduces API calls for repeated prompts.

4. **Genre Variations Map Exists**: There's already a 40+ entry `getGenreVariations()` map handling synonyms like "alt"→"alternative", "hiphop"→"hip hop".

5. **Decade Detection Exists**: `extractDecadeFromPrompt()` already handles "90s", "early 90s", "late 80s", "modern", "classic", "retro", "vintage", and explicit year ranges.

### Phase-Specific Suggestions

**Phase 0 – Stabilization:**
- ✅ **IMPLEMENTED (2025-01-13)**: True random shuffle using `math/rand` with time-based seeding. Replaced all pseudo-random shuffles `(i * n) % (i + 1)` with proper Fisher-Yates algorithm using `rand.Intn()`.
- ⚠️ Session-aware continuity would require new session tracking infrastructure—consider if the complexity is worth it.

**Phase 1 – Metadata-Driven Intelligence:**
- ✅ **IMPLEMENTED (2025-01-13)**: Extended `GetSongsBySmartFilter()` to accept and filter by mood, energy, and tempo parameters. The SQL query now includes WHERE clauses for these fields when provided by Gemini's filter parsing.
- ✅ **IMPLEMENTED (2025-01-13)**: Added `tryMoodBasedMatch()` function as Tier 1.5 in the matching hierarchy. This intercepts 85+ common mood/activity keywords (chill, workout, relax, focus, party, etc.) and queries by mood directly, bypassing Gemini entirely. Keywords are mapped to mood/energy/tempo combinations.
  ```go
  // Expanded Tier 1.5: Mood keyword detection
  moodKeywords := map[string]struct{mood, energy, tempo string}{
      "chill": {"calm", "low", "slow"},
      "workout": {"energetic", "high", "fast"},
      "focus": {"peaceful", "medium", "medium"},
      // ... 85+ mappings
  }
  ```

**Phase 2 – LLM Abstraction:**
- The current Gemini client is already abstracted into `internal/gemini/gemini.go`. Adding provider interface would require:
  - New `internal/llm/provider.go` with interface
  - Implementations: `gemini.go`, `openai.go`, `ollama.go`
  - Settings UI for provider selection
- Consider starting with Ollama for local-first privacy benefits.

**Phase 3 – QueryPlan:**
- Current implementation already has implicit intent types:
  - Artist-based: `tryArtistBasedMatch()`
  - Genre-based: `tryLocalGenreMatch()`
  - Complex: Gemini fallback
- Formalizing QueryPlan would add value but requires refactoring `handleGenerateSmartPlaylist()`. Consider whether this abstraction is needed before Phase 4.

**Phase 4 – Library-as-Knowledge (RAG):**
- SQLite FTS5 is a natural fit for existing architecture.
- Artist profiles could be generated during background enrichment.
- This phase would benefit from completing Phase 1 (mood/energy filtering) first.

**Phase 5 – Preference Learning:**
- Database already tracks `play_count`, `skip_count`, `last_played`.
- The `discoverMode` option already uses play counts.
- Missing: Skip timing (< 10s vs < 30s), replay detection, explicit likes on AI DJ results.
- Preference table schema suggestion:
  ```sql
  CREATE TABLE user_preferences (
      genre TEXT PRIMARY KEY,
      affinity_score REAL DEFAULT 0.5,
      skip_rate REAL DEFAULT 0,
      completion_rate REAL DEFAULT 0,
      updated_at INTEGER
  );
  ```

**Phase 6 – Advanced RAG:**
- Consider deferring until Phases 1-5 prove value.
- "Playlist arc" (energy curves) is an interesting UX improvement.

### Suggested Priority Reordering

Based on effort vs. impact:

1. ~~**Phase 1.1** (1-2 hours): Enable mood/energy in SQL queries — immediate UX improvement~~ ✅ DONE (2025-01-13)
2. ~~**Phase 0** (2-4 hours): True random shuffle with seed — eliminates repetition complaints~~ ✅ DONE (2025-01-13)
3. ~~**Phase 1.2** (4-8 hours): Keyword-to-mood mapping before Gemini — reduces API cost~~ ✅ DONE (2025-01-13)
4. ~~**Phase 5.1** (4-8 hours): Preference tracking (skip timing) — enables future personalization~~ ✅ DONE (2025-01-13)
   - Added `listening_events` and `genre_preferences` tables to SQLite schema
   - Added `RecordListeningEvent()`, `UpdateGenrePreferences()`, `GetGenrePreferences()`, `GetAllGenrePreferences()` DB functions
   - Added `POST /api/songs/{id}/listen-event` endpoint with automatic event type detection
   - Event types: `play_complete` (≥90%), `skip_early` (<10s), `skip_mid` (10-30s), `skip_late` (>30s)
   - Genre affinity scoring: higher completion rate = higher score, early skips penalized more
   - ~~**Frontend integration needed**: Call listen-event API from useAudioPlayer on skip/complete~~ ✅ DONE
   - Frontend tracks play duration across pause/resume and records events on song change/complete
   - Added `recordListenEvent()` to types, librarySlice, and backendService
   - Added `PlaybackContext` type for future context tracking enhancement
5. **Phase 2** (1-2 days): LLM abstraction with Ollama — local-first option
6. **Phase 4** (2-3 days): FTS5 integration — better "more like" results
7. **Phase 3** (1-2 days): QueryPlan refactor — cleaner architecture

### Missing From Plan

1. **Time-Aware Context Improvements**: Current implementation has basic time-of-day awareness (`getTimeContext()`). Could enhance with:
   - User timezone detection
   - Activity detection (if device provides)
   - Calendar integration (optional)

2. **Multi-Genre Blend Improvements**: Current `tryMatchMultipleGenres()` uses score-weighted proportions. Could add:
   - Genre compatibility scoring (rock + metal = high, rock + classical = low)
   - Transition smoothing in playlist order

3. **Caching Strategy**: Filter cache exists but songs aren't cached. Consider:
   - Caching genre → songs mapping (invalidated on library change)
   - Reducing database queries for repeated genre lookups

---

## Purpose

This document outlines a phased improvement plan for the **AI DJ** feature in the local music media player application.  
The plan explicitly accounts for the **current Tier-based implementation**, preserves existing APIs and data models where possible, and incrementally introduces more advanced techniques (RAG, hybrid retrieval, preference learning, and multi-LLM support) while controlling complexity and API cost.

---

## Current State Summary (Baseline)

### Architecture
- Backend: **Golang**
- Endpoint: `/api/smart-playlist`
- Entry point: `handleGenerateSmartPlaylist()`
- Playlist generation follows a **Tiered model**:
  - **Tier 0** – Direct artist match
  - **Tier 1** – Genre-only filtering
  - **Tier 2** – Gemini LLM → `LocalPlaylistFilter`
  - **Tier 3** – Relaxed fallback queries

### Strengths
- Clear tier separation and fast-path logic
- Deterministic, structured filter output (`LocalPlaylistFilter`)
- Local DB-first design
- Time-of-day awareness and play-history post filtering

### Known Limitations
- Mood / energy / tempo metadata not used in queries
- Genre matching relies on string similarity only
- Deterministic shuffle yields repeated ordering
- No preference learning
- Gemini required for many prompts → unnecessary API spend
- No retrieval over the *library as knowledge*

---

## Phase 0 – Stabilization & Low-Risk Fixes (No Architecture Change)

**Goal:** Improve output quality and reduce repetition with minimal code changes.

### Scope
- No LLM or data model changes
- No new dependencies

### Improvements
1. **Non-deterministic shuffle**
   - Seed randomization with `(session_id + timestamp_bucket)`
   - Maintain reproducibility within a session

2. **Session-aware playlist continuity**
   - Persist session seed across multiple DJ requests
   - Avoid reordering tracks unnecessarily

3. **Minor ranking enhancements**
   - Penalize recently played tracks more aggressively
   - Strengthen one-per-artist enforcement

### Outcome
- Immediate UX improvement
- Zero API impact
- Zero data migration

---

## Phase 1 – Metadata-Driven Intelligence (Local-Only)

**Goal:** Eliminate unnecessary LLM calls by fully leveraging existing metadata.

### Scope
- SQL and ranking changes only
- No embeddings, no vector DB

### Improvements
1. **Activate existing metadata**
   - Use `mood`, `energy`, `tempo`, `bpm`, `instrumental` directly in SQL queries
   - Treat them as *soft constraints* when not explicitly specified

2. **Prompt-to-filter heuristics**
   - Simple keyword detection for:
     - workout / chill / focus / sleep
     - upbeat / calm / aggressive / mellow
   - Map keywords directly to metadata ranges

3. **Expanded Tier 1**
   - Many prompts currently falling into Tier 2 should now resolve locally

### Outcome
- Large reduction in Gemini usage
- Better playlists for “activity” and “vibe” prompts
- No schema changes

---

## Phase 2 – LLM Abstraction & Provider Choice

**Goal:** Decouple AI DJ logic from Gemini and enable user/provider choice.

### Scope
- Introduce LLM interface abstraction
- Maintain current `LocalPlaylistFilter` schema

### Design
```go
type LLMProvider interface {
    ParseIntent(ctx context.Context, prompt string) (QueryPlan, error)
}
````

### Supported Providers

* Gemini (existing)
* OpenAI
* Ollama (local-first)
* Future providers (Claude, etc.)

### Model Strategy

* **Small-model-first**

  * Use local or low-cost model for intent parsing
* **Escalation**

  * Only use higher-cost models if parsing confidence is low

### Libraries

* Preferred:

  * `langchaingo` for chains & retrievers (future-proof)
  * or `gollm` for lightweight provider abstraction

### Outcome

* User-visible model selection
* Reduced vendor lock-in
* Lower average cost per request

---

## Phase 3 – QueryPlan (Replacing “Filter-Only” Thinking)

**Goal:** Separate *intent understanding* from *data filtering*.

### New Concept: `QueryPlan`

```json
{
  "intent_type": "genre_vibe | artist_similar | activity",
  "hard_constraints": {
    "artists": [],
    "decades": []
  },
  "soft_preferences": {
    "mood": "calm",
    "energy": "low"
  },
  "retrieval_strategy": "lexical | hybrid",
  "budget_policy": "local_first"
}
```

### Changes

* LLM outputs a **QueryPlan**, not a DB filter
* QueryPlan drives:

  * Retrieval
  * Ranking
  * Escalation decisions

### Backward Compatibility

* QueryPlan is translated into `LocalPlaylistFilter`
* API response remains unchanged

### Outcome

* Cleaner orchestration logic
* Easier to extend to RAG
* More explainable AI behavior

---

## Phase 4 – Library-as-Knowledge (RAG Lite)

**Goal:** Treat the local music library as a searchable knowledge base.

### Scope

* Hybrid retrieval (no re-ranking LLM yet)
* Local-first design

### Retrieval Methods

1. **Lexical**

   * SQLite FTS5 over artist / title / album / genres / tags
2. **Semantic (optional initially)**

   * Track and artist embeddings
   * Stored locally

### Document Types

* Track documents
* Artist profiles (aggregated metadata)
* Genre/vibe clusters (derived)

### Flow

Prompt → QueryPlan → Retrieve Candidates → Rank → Playlist

### Outcome

* “More like X” works without LLM creativity
* Genre synonym issues reduced
* RAG without heavy agent complexity

---

## Phase 5 – Preference Learning & Feedback Loop

**Goal:** Make the AI DJ feel personal over time.

**Status:** ✅ Phase 5.1 (Backend) COMPLETE (2025-01-13)

### Implementation Notes (5.1)

**Database Tables Added:**
```sql
-- Detailed listening events for analysis
listening_events (
  id, song_id, event_type, play_duration, song_duration,
  timestamp, genre, mood, energy, context
)

-- Aggregated genre preferences
genre_preferences (
  genre PRIMARY KEY, play_count, skip_count,
  complete_rate, skip_early_rate, affinity_score, last_updated
)
```

**API Endpoint:**
- `POST /api/songs/{id}/listen-event` with body `{playDuration, songDuration, context}`
- Returns `{status, eventType}` where eventType is auto-detected

**DB Functions:**
- `RecordListeningEvent()` - Insert event
- `UpdateSkipCount()` - Increment song skip counter
- `UpdateGenrePreferences()` - Recalculate affinity from events
- `GetGenrePreferences()` / `GetAllGenrePreferences()` - Query prefs

**Affinity Scoring:**
- `affinityScore = completeRate - (skipEarlyRate * 0.3)`
- Range: 0.0 (avoided) to 1.0 (loved)
- Early skips hurt more than late skips

**Frontend Integration (✅ DONE):**
- `useAudioPlayer.ts` tracks play duration via `listenTrackingRef`
- Handles pause/resume by accumulating play time
- Records listen event on song change (skip) or song end (complete)
- `recordListenEvent()` added to librarySlice and backendService
- `PlaybackContext` type added for future enhancement

**Files Modified:**
- `hooks/useAudioPlayer.ts` - Listen tracking with pause/resume support
- `slices/types.ts` - Added `PlaybackContext` type, `recordListenEvent` signature
- `slices/librarySlice.ts` - Implemented `recordListenEvent`
- `services/api.ts` - Added `recordListenEvent` API call
- `services/backendService.ts` - Added `recordListenEvent` wrapper

### Signals

* Skip <10s
* Skip <30s
* Completed play
* Replay
* Like / Dislike

### Usage

* Penalize skipped tracks
* Boost completed or replayed tracks
* Learn time-of-day energy tolerance
* Adapt ranking, not filtering

### Storage

* Lightweight per-user preference table
* No ML model required initially

### Outcome

* Increasingly accurate playlists
* High stickiness
* Zero LLM cost increase

---

## Phase 6 – Optional Advanced RAG & Re-Ranking

**Goal:** Enable premium DJ behaviors without making them mandatory.

### Enhancements

* LLM-based re-ranking of top N candidates
* Playlist “arc” generation (energy curves)
* Natural-language explanations (“why these songs”)

### Cost Control

* Only applied when:

  * User opts into “Best Quality”
  * Confidence score is low
  * Playlist size is large

### Outcome

* Premium-feeling AI DJ
* Predictable cost envelope

---

## Success Metrics

| Metric                       | Baseline | Target     | How to Measure |
| ---------------------------- | -------- | ---------- | -------------- |
| Avg LLM calls / playlist     | ~50-70%  | ↓ 50–80%   | Log Tier 2+ hits vs total requests |
| Local-only resolution rate   | ~30%     | >70%       | Log Tier 0+1 hits vs total |
| Repeated ordering complaints | Present  | Eliminated | User feedback, shuffle seed logs |
| Playlist satisfaction        | Unknown  | Trackable  | Skip rate within first 10s |
| Mood/vibe prompt success     | 0%       | >80%       | Queries using mood/energy filters |

### Suggested Implementation for Metrics

Add request logging in `handleGenerateSmartPlaylist()`:

```go
type PlaylistMetrics struct {
    TierUsed       int       // 0, 1, 2, or 3
    GeminiCalled   bool
    CacheHit       bool
    MoodFilterUsed bool
    ResultCount    int
    ResponseTimeMs int64
}

// Log at end of handler
logger.API("DJ Metrics: tier=%d gemini=%v cache=%v mood=%v results=%d time=%dms",
    metrics.TierUsed, metrics.GeminiCalled, metrics.CacheHit, 
    metrics.MoodFilterUsed, metrics.ResultCount, metrics.ResponseTimeMs)
```

---

## Guiding Principles

* **Local-first, cloud-optional**
* **Structured outputs over free-form text**
* **Retrieval before reasoning**
* **Escalate intelligence only when necessary**
* **Backward compatibility at every phase**

---

## Final Note

This plan is intentionally incremental.
Each phase is independently shippable, reversible, and measurable.

AI DJ should feel *smarter* not because it talks more—but because it listens better, remembers, and uses what it already knows.


