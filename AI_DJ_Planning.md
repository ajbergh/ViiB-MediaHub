You are GitHub Copilot running in Agent Mode. You will implement a new “WOW” AI DJ experience for an existing local music player app. The backend is Golang, and the frontend is React/TypeScript. Follow the instructions precisely, update/create the files listed, and keep changes minimal and well-factored. Do not remove existing functionality—extend it.

================================================================================IMPLEMENTATION PHASES - EXECUTION ROADMAP
================================================================================

## Phase 1: Foundation & Data Structures (Priority: HIGH)
**Estimated Time: 2-3 hours**
**Dependencies: None**
**Status: ✅ COMPLETE**

### 1.1 Create Core DJ Package Structure
- [x] Create `backend/internal/dj/` directory
- [x] Create `backend/internal/dj/types.go` - All shared types (DJPhase, DJSetPlan, ScoreContext, etc.)
- [x] Create `backend/internal/dj/constants.go` - BPM defaults, phase weights, energy/tempo mappings

### 1.2 Define API Contract
- [x] Update `services/api.ts` - Add DJ mode request/response types
- [x] Update `slices/aiDjSlice.ts` - Add state for DJ mode, personas, plan results

**Deliverables:**
- ✅ Type definitions in Go and TypeScript
- ✅ No runtime changes yet - pure data structure setup

---

## Phase 2: Persona System (Priority: HIGH)
**Estimated Time: 1-2 hours**
**Dependencies: Phase 1**
**Status: ✅ COMPLETE**

### 2.1 Implement Persona Definitions
- [x] Create `backend/internal/dj/personas.go`
- [x] Define PersonaWeights struct with all scoring factors
- [x] Implement 6 personas: FlowMaster, CrowdPleaser, DeepCutDJ, Explorer, Curator, NightDrive
- [x] Implement `GetPersona()` and `ListPersonas()` functions
- [x] Implement weight calculation helpers (BPM penalty, artist penalty, recency, etc.)

### 2.2 Add Persona API Endpoint
- [x] Add `GET /api/dj/personas` endpoint to return available personas
- [x] Wire into existing API router
- [x] Add `api.getDJPersonas()` frontend method

**Deliverables:**
- ✅ Working persona retrieval
- ✅ Persona weight calculations ready for scoring engine

---

## Phase 3: DJ Set Planner - LLM Integration (Priority: HIGH)
**Estimated Time: 3-4 hours**
**Dependencies: Phase 1, Phase 2**
**Status: ✅ COMPLETE**

### 3.1 LLM Prompt Templates
- [x] Update `backend/internal/llm/prompts.go` - Add DJSetPlanSystemPrompt
- [x] Add DJSetPlanUserPromptTemplate for plan generation
- [x] Add DJNarrationSystemPrompt and DJNarrationUserPromptTemplate for talk mode

### 3.2 Planner Implementation
- [x] Create `backend/internal/dj/dj_set_planner.go`
- [x] Implement in-memory TTL cache (30 min, keyed by prompt+persona+settings hash)
- [x] Implement `BuildPlan()` with LLM call
- [x] Implement `buildDefaultPlan()` fallback for LLM failures
- [x] Add helper functions: NormalizePrompt, HashGenres, extractJSON, validatePhaseName

### 3.3 Plan Validation
- [x] Validate phase count (3-5 phases)
- [x] Validate BPM ranges (60-190, min < max)
- [x] Auto-correct song counts with adjustPhaseCounts()
- [x] Implement GenerateNarration() for talk mode

**Deliverables:**
- ✅ Working plan generation with cache
- ✅ Fallback plan for offline/error scenarios
- ✅ Narration generation for talk mode

---

## Phase 4: Scoring Engine (Priority: HIGH)
**Estimated Time: 2-3 hours**
**Dependencies: Phase 2, Phase 3**
**Status: ✅ COMPLETE**

### 4.1 Score Context Builder
- [x] Implement ScoreContext initialization from DB data
- [x] Load genre affinity scores (GenreAffinity map)
- [x] Track artistSeen set and recentlyPlayedIDs
- [x] Implement BuildScoreContext() helper

### 4.2 Song Scoring Function
- [x] Implement `ScoreSongForPhase()` in `backend/internal/dj/scoring.go`
- [x] Phase fit scoring (energy/tempo/mood match, BPM range)
- [x] Persona weight application (favorites, underplayed, liked bonuses)
- [x] Preference learning terms (genre affinity, skip rate penalty)
- [x] BPM continuity scoring based on last song
- [x] Score breakdown for debugging

**Deliverables:**
- ✅ Deterministic, explainable scoring with ScoreBreakdown
- ✅ All persona weight calculations integrated
- ✅ Batch scoring with ScoreAllCandidates()

---

## Phase 5: Sequencer Algorithm (Priority: HIGH)
**Estimated Time: 3-4 hours**
**Dependencies: Phase 3, Phase 4**
**Status: ✅ COMPLETE**

### 5.1 Core Sequencer
- [x] Create `backend/internal/dj/sequencer.go`
- [x] Implement candidate partitioning by energy/tempo/mood buckets
- [x] Implement per-phase selection with softmax sampling on top-50
- [x] Implement one-per-artist enforcement
- [x] Implement BPM continuity via sorting

### 5.2 Constraint Relaxation
- [x] Implement progressive relaxation when insufficient candidates
- [x] Handle edge cases gracefully (fall back to full pool)
- [x] Add micro-shuffle within phase windows for low flowStrictness

### 5.3 Queue Assembly
- [x] Concatenate phases into final queue
- [x] Generate PhaseResult metadata (avgBPM, bpmRange, notes)
- [x] Update context between phases (artist seen, last song)

**Deliverables:**
- ✅ Working BuildQueue() function
- ✅ Stochastic selection with softmax sampling
- ✅ BPM-based sorting within phases

---

## Phase 6: API Integration (Priority: HIGH)
**Estimated Time: 2-3 hours**
**Dependencies: Phase 3, Phase 4, Phase 5**
**Status: ✅ COMPLETE**

### 6.1 Extend Smart Playlist Endpoint
- [x] Update `backend/internal/api/smart_playlist.go`
- [x] Add request fields: mode, persona, targetDurationMinutes, talkMode, flowStrictness
- [x] Implement DJ mode flow:
  1. Parse filter (existing)
  2. Call Planner.BuildPlan()
  3. Gather candidate songs from DB
  4. Call Sequencer.BuildQueue()
  5. Apply final safety filters
  6. Return DJ payload
- [x] Add `dj` import to smart_playlist.go
- [x] Implement `handleDJMode()` helper function

### 6.2 Response Structure
- [x] Return songs + plan + phases + narration when mode == "dj"
- [x] Maintain backward compatibility for mode == "playlist"
- [x] Use existing logger.API() for debug logging

### 6.3 LLM Provider Enhancement
- [x] Add `Generate()` method to llm.Provider for generic LLM calls
- [x] Update dj.NewLLMPlanner() to handle llm.NewProvider() error

**Deliverables:**
- ✅ Working `/api/smart-playlist` with DJ mode
- ✅ Full backward compatibility with existing playlist mode

---

## Phase 7: Frontend - State & Types (Priority: MEDIUM)
**Estimated Time: 1-2 hours**
**Dependencies: Phase 6**
**Status: ✅ COMPLETE**

### 7.1 Update State Management
- [x] Extend `slices/aiDjSlice.ts` with:
  - aiDjMode: boolean
  - aiDjPersona: DJPersona
  - aiDjTargetDurationMinutes: number
  - aiDjFlowStrictness: number (0-100)
  - aiDjTalkMode: boolean
  - aiDjPlan: DJSetPlan | null
  - aiDjPhases: DJPhaseResult[]
  - aiDjNarration: DJNarration | null
- [x] Add all setter actions
- [x] Add bulk update action: setAIDJDJResult()

### 7.2 API Client Updates
- [x] Update `services/api.ts` with DJ request/response types:
  - SmartPlaylistMode
  - DJPersona, DJPersonaDefinition
  - DJPhase, DJSetPlan
  - DJPhaseResult, DJNarration, DJModeResponse
- [x] Add `dj?: DJModeResponse` to SmartPlaylistResponse
- [x] Add `getDJPersonas()` function

**Deliverables:**
- ✅ Complete frontend type definitions
- ✅ API client methods ready

---

## Phase 8: Frontend - DJ Console UI (Priority: MEDIUM)
**Estimated Time: 4-5 hours**
**Dependencies: Phase 7**
**Status: ✅ COMPLETE**

### 8.1 Mode Toggle
- [x] Add Playlist | DJ Mode tab/toggle at top of SmartPlaylists.tsx
- [x] Conditionally render controls based on mode

### 8.2 DJ Controls
- [x] Persona selection cards (name + 1-line description)
- [x] Set length slider (15-120 minutes) with target song count display
- [x] Flow strictness slider (Loose ↔ Strict)
- [x] DJ Talk toggle (off by default)
- [x] "Start Set" button with mode-aware label

### 8.3 Results Display
- [x] Phase timeline visualization (horizontal bars with energy colors)
- [x] DJ Set Queue list with clickable rows
- [x] Phase info: song count, BPM range, energy level
- [x] Intent summary line from plan
- [x] Narration display (if talkMode enabled) with intro/phase/outro cues

### 8.4 Queue Actions
- [x] Play generated set (mode-aware button label)
- [x] Save as playlist

**Deliverables:**
- ✅ Complete DJ Console UI
- ✅ Mode toggle, persona selection, duration/strictness sliders
- ✅ Phase visualization with energy-colored bars
- ✅ DJ narration cues when talk mode enabled

---

## Phase 9: Testing & Polish (Priority: MEDIUM)
**Estimated Time: 2-3 hours**
**Dependencies: All above**
**Status: 🟡 IN PROGRESS**

### 9.1 Unit Tests
- [ ] Plan JSON validation tests
- [ ] Phase count distribution tests
- [ ] Sequencer continuity tests
- [ ] Persona scoring difference tests

### 9.2 Integration Tests
- [ ] End-to-end DJ set generation
- [ ] Cache hit/miss verification
- [ ] Fallback plan triggering

### 9.3 UI Polish
- [x] Loading states during generation (animated spinner with context-aware messaging)
- [x] Error handling & user feedback (enhanced error messages for API/network/library issues)
- [x] Success toast on DJ set generation
- [x] TypeScript type sync (fixed AIDJSlice in types.ts to include DJ mode properties)
- [ ] Empty state when no songs match (basic empty state exists, could enhance)

### 9.4 Bug Fixes (2025-12-31)
- [x] **Critical: DJ mode not filtering by prompt** - handleDJMode was sending ALL songs to sequencer
  - Added LLM filter parsing to extract genres/artists from prompt
  - Added tryMoodBasedMatchInfo() to extract mood hints
  - Added filtering logic to match candidates by seed genres/artists
  - Added comprehensive logging for debugging
- [x] **Missing logs** - No logging in handleDJMode function
  - Added 15+ log statements for request, plan, phases, candidates, results

**Deliverables:**
- Test coverage for critical paths
- Polished user experience

---

## Phase 10: Documentation & Cleanup (Priority: LOW)
**Estimated Time: 1 hour**
**Dependencies: Phase 9**

- [ ] Update README with DJ feature documentation
- [ ] Add inline code comments
- [ ] Clean up any TODO markers
- [ ] Performance profiling for large libraries

---

## EXECUTION ORDER SUMMARY

| Order | Phase | Time Est. | Status |
|-------|-------|-----------|--------|
| 1     | Phase 1: Foundation | 2-3h | ✅ COMPLETE |
| 2     | Phase 2: Personas | 1-2h | ✅ COMPLETE |
| 3     | Phase 3: Planner | 3-4h | ✅ COMPLETE |
| 4     | Phase 4: Scoring | 2-3h | ✅ COMPLETE |
| 5     | Phase 5: Sequencer | 3-4h | ✅ COMPLETE |
| 6     | Phase 6: API | 2-3h | ✅ COMPLETE |
| 7     | Phase 7: FE State | 1-2h | ✅ COMPLETE |
| 8     | Phase 8: FE UI | 4-5h | ✅ COMPLETE |
| 9     | Phase 9: Testing | 2-3h | 🟡 IN PROGRESS |
| 10    | Phase 10: Docs | 1h | NOT STARTED |

**Total Estimated Time: 22-30 hours**

**Critical Path:** Phases 1→2→3→4→5→6 (backend must complete before frontend can fully test)

================================================================================GOAL
================================================================================
Introduce a DJ Set Planning layer (“DJSetPlanner”) that turns a user prompt into:
1) A structured DJ Set Plan (energy/tempo/mood curve across phases)
2) A deliberate sequencing strategy (flow, BPM continuity, phase grouping)
3) Reactive adaptation to listening events (skip/complete) without additional LLM calls
4) Persona-based scoring biases (Deep-Cut DJ, Crowd-Pleaser, Flow Master, etc.)
5) A UI redesign that makes this feel like a DJ experience rather than a filter form.

Existing system:
- Endpoint: POST /api/smart-playlist
- Tiers: artist match, local genre match, mood keyword match, LLM parsed filter, fallback DB filter
- DB schema includes: mood, energy, tempo, bpm, instrumental, play counts, listening_events, genre_preferences
- Provider abstraction: omnillm with Ollama/Gemini/OpenAI/Anthropic/XAI
- Frontend page: pages/SmartPlaylists.tsx

We will ADD a DJ mode that still uses the existing filter generation and multi-genre match,
but inserts a DJSetPlan + sequencing layer before returning songs.

================================================================================
IMPLEMENTATION OVERVIEW
================================================================================
Backend changes:
A) Add backend/internal/dj/dj_set_planner.go (new package `dj`)
B) Add backend/internal/dj/personas.go (persona definitions + scoring matrix)
C) Add backend/internal/dj/sequencer.go (phase-based sequencing + BPM smoothing)
D) Wire into backend/internal/api/smart_playlist.go:
   - Accept new request fields: mode, persona, targetDurationMinutes, talkMode, flowStrictness
   - If mode == "dj":
       1) Build/parse filter (existing logic)
       2) Create DJSetPlan (LLM call #1, cached)
       3) Candidate song pool (existing DB queries)
       4) Apply persona scoring + preferences
       5) Sequence via energy/tempo phases + BPM continuity
       6) Return: songs + plan + phase breakdown + narration cues

Frontend changes:
E) Redesign pages/SmartPlaylists.tsx into a “DJ Console”:
   - Prompt input (top)
   - Persona selection cards
   - “Set length” slider (minutes or song count)
   - “Flow” slider (loose -> strict)
   - Optional “DJ talk” toggle (subtle narration)
   - Results view:
     - Shows phases (Warm-up, Build, Peak, Cooldown)
     - Shows “Now Playing” style queue with phase separators
     - Shows “DJ Notes” summary (1-2 lines) generated from plan
   - Keep the existing mode available (playlist mode) as a toggle/tab.

================================================================================
API / DATA CONTRACT
================================================================================
1) Update request/response types (backend + frontend)

Request additions:
- mode: "playlist" | "dj" (default "playlist")
- persona: string (default "FlowMaster")
- targetDurationMinutes: int (default 45)
- talkMode: bool (default false)
- flowStrictness: int 0-100 (default 60) // higher = stricter BPM continuity

Response additions when mode == "dj":
{
  filter: LocalPlaylistFilter,
  songs: [...],
  dj: {
    plan: DJSetPlan,
    phases: [
      { name, energy, tempo, mood, count, bpmRange, notes }
    ],
    narration: {
      intro: string,
      phaseIntros: [string],
      outro: string
    }
  }
}

Ensure backward compatibility: if mode not provided, response remains as before.

================================================================================
A) DJSetPlanner.go SKETCH (CREATE THIS)
================================================================================
Create file: backend/internal/dj/dj_set_planner.go

Package: dj

Define:

1) DJSetPlan structs:

type DJPhase struct {
  Name           string   // "Warm-up", "Build", "Peak", "Cooldown"
  TargetEnergy   string   // low|medium|high
  TargetTempo    string   // slow|medium|fast
  TargetMoods    []string // e.g. ["calm","dreamy"]
  TargetCount    int
  MinBPM         int
  MaxBPM         int
  Notes          string
}

type DJSetPlan struct {
  IntentSummary       string
  TargetDurationMin   int
  Persona             string
  FlowStrictness      int
  Phases              []DJPhase
  SeedGenres          []string // from matchedGenres or filter.Genres
  SeedArtists         []string
  CreatedAtUnix       int64
  FromCache           bool
}

2) Planner interface:

type Planner interface {
  BuildPlan(ctx context.Context, prompt string, filter any, availableGenres []string, now time.Time, opts PlanOptions) (*DJSetPlan, error)
}

type PlanOptions struct {
  Persona            string
  TargetDurationMin  int
  FlowStrictness     int
  UseTimeContext     bool
  TalkMode           bool
}

3) Implementation: LLM-backed plan builder with cache:
- Use existing omnillm Provider from backend/internal/llm/provider.go
- Add a small in-memory TTL cache in dj package (15-60 minutes) keyed by:
  provider+model + normalized prompt + persona + targetDurationMin + flowStrictness + topGenresHash
- Return plan.FromCache = true on hit

4) Plan generation prompt (see section “PROPOSED LLM PROMPTS” below)
- Output MUST be JSON only.
- Validate JSON: phase count 3-5, BPM ranges plausible, TargetCount sums to approx target song count
- Compute target song count:
  avgSongLenSec assume 210 sec unless you can compute from library stats
  targetSongs = (TargetDurationMin*60)/avgSongLenSec (round)
  distribute across phases with typical weights: 25/25/30/20 or similar

5) Provide helper functions:
- NormalizePrompt
- HashGenres
- DefaultPlan if LLM fails (fallback deterministic plan based on time-of-day + prompt mood keywords)

================================================================================
B) PERSONAS + SCORING MATRIX (CREATE THIS)
================================================================================
Create file: backend/internal/dj/personas.go

Design a persona scoring matrix as weights applied to song candidate scoring.
You already have (or can derive):
- play_count
- last_played
- genre affinity_score (genre_preferences)
- completion/skip rates (via listening_events aggregates)
- bpm / tempo / energy / mood
- one-per-artist preference

Define:

type PersonaWeights struct {
  UnderplayedBoost         float64 // favor low play_count
  FavoritesBoost           float64 // favor high play_count / high completion rate
  AffinityBoost            float64 // favor high genre affinity_score
  NoveltyBoost             float64 // favor genres with mid affinity but underexplored
  BPMContinuityWeight      float64 // penalize BPM jumps
  MoodContinuityWeight     float64
  EnergyContinuityWeight   float64
  OnePerArtistStrictness   float64
  InstrumentalBias         float64 // optional
}

type PersonaDefinition struct {
  Key         string
  Name        string
  Description string
  Weights     PersonaWeights
}

Implement:
- GetPersona(key string) PersonaDefinition (with default)
- ListPersonas() []PersonaDefinition

Personas to include:
1) FlowMaster (default): strong continuity, balanced novelty
2) CrowdPleaser: favors high completion, favorites, familiar
3) DeepCutDJ: heavy underplayed boost, novelty, low favorites bias
4) Explorer: controlled novelty, medium continuity
5) Curator: strict genre purity + one-per-artist
6) NightDrive: smoother tempos, medium energy, nostalgic bias if available

Scoring function signature:
func ScoreSongForPhase(song db.Song, phase DJPhase, ctx ScoreContext, persona PersonaDefinition) float64

ScoreContext includes:
- genreAffinity map[string]float64
- artistSeen set
- lastSongBPM int
- lastSongMood/energy/tempo
- recentlyPlayedIDs map
- discoverMode
- flowStrictness 0-100

Implementation guidance:
- Compute base fit to phase (energy/tempo/mood match + bpm range)
- Add persona terms
- Add preference learning terms:
   - boost if genre affinity high
   - penalize if genre affinity low AND skipEarlyRate high
- Keep score explainable: return also a breakdown optionally (for debugging)

================================================================================
C) SEQUENCER (CREATE THIS)
================================================================================
Create file: backend/internal/dj/sequencer.go

Purpose:
Given a candidate pool of songs (already filtered by genres/artists/years) and a DJSetPlan,
select and order songs to match each phase and maintain flow.

Define:
type Sequencer struct {}

func (s *Sequencer) BuildQueue(
  candidates []db.Song,
  plan *DJSetPlan,
  persona PersonaDefinition,
  ctx ScoreContext,
) (queue []db.Song, phases []PhaseResult, err error)

Where PhaseResult includes:
- phase name
- chosen song IDs
- avgBPM, bpmRange
- short notes (computed)

Algorithm (must be deterministic with light randomness):
1) Partition candidates by rough buckets (energy/tempo/mood).
2) For each phase:
   - score candidates with ScoreSongForPhase
   - pick top K with some stochasticity (e.g. softmax sampling on top 50)
   - enforce one-per-artist according to persona weight
   - maintain BPM continuity: penalize big jumps; stricter if flowStrictness high
   - if phase cannot fill count, relax constraints in this order:
        mood -> bpm range -> tempo -> energy
3) After building phase lists, concatenate phases.
4) Apply micro-shuffle within phase windows of 3-5 songs if flowStrictness low.

IMPORTANT:
- Do not require FFT or actual audio processing.
- Use stored bpm if present; if bpm missing, approximate from tempo:
  slow=90, medium=120, fast=150 (or derive from library stats).
- Keep runtime efficient for libraries up to ~100k tracks.

================================================================================
D) WIRING INTO EXISTING ENDPOINT
================================================================================
Update backend/internal/api/smart_playlist.go:
- Extend request struct to accept DJ mode fields.
- Add new flow:
  if req.Mode == "dj":
    - existing matching logic to get filter + matched genres
    - gather candidate songs using existing DB methods (genre + mood filters)
    - call dj.Planner.BuildPlan(...)
    - call dj.Sequencer.BuildQueue(...)
    - apply play history filters only as a final safety net (avoidRecentlyHours)
    - return response with dj payload

Also:
- Add minimal logging + debug mode flag (optional) to inspect chosen phases.

================================================================================
PROPOSED LLM PROMPTS (IMPLEMENT IN backend/internal/llm/prompts.go)
================================================================================
Add a new prompt template: DJSetPlanContextPromptTemplate

SYSTEM:
You are an expert DJ and music curator. You design DJ sets with a clear energy arc,
smooth transitions, and phase structure. You must output STRICT JSON only.

USER:
- User prompt: {{PROMPT}}
- Persona: {{PERSONA}}
- Target duration minutes: {{DURATION}}
- Flow strictness (0-100): {{FLOW}}
- Time context: {{TIME_CONTEXT}}   // if enabled
- Available genres in library (prefer these): [{{GENRES}}]
- Seed genres already matched: [{{SEED_GENRES}}]
- Seed artists matched: [{{SEED_ARTISTS}}]

OUTPUT JSON ONLY:
{
  "intentSummary": "1 sentence summary of vibe and intent",
  "phases": [
    {
      "name": "Warm-up|Build|Peak|Cooldown|Afterhours",
      "targetEnergy": "low|medium|high",
      "targetTempo": "slow|medium|fast",
      "targetMoods": ["..."],
      "targetCount": 0,
      "minBPM": 0,
      "maxBPM": 0,
      "notes": "short DJ note for this phase"
    }
  ]
}

RULES:
- 3 to 5 phases
- targetCount values must sum to an appropriate number of songs for the duration
- BPM ranges must be plausible (minBPM < maxBPM, between 60 and 190)
- Prefer genre/mood terms that are likely present in a local library
- Keep notes concise, no emojis

Also add an optional prompt for narration if talkMode==true:
DJNarrationPromptTemplate
- Input: plan JSON
- Output: { "intro": "...", "phaseIntros": ["..."], "outro": "..." }
Keep it subtle and short.

================================================================================
E) FRONTEND UI REDESIGN (pages/SmartPlaylists.tsx)
================================================================================
Redesign into a DJ Console. Keep existing functionality but add:
- Mode toggle: Playlist | DJ Mode
- Persona cards: show name + 1-line description
- Set length: slider (15-120 minutes), plus “Target songs” display
- Flow strictness: slider (Loose -> Strict)
- Optional toggle: “DJ Talk” (off by default)
- Generate button: “Start DJ Set”
- Results:
  - Phase timeline (horizontal or stacked): Warm-up/Build/Peak/Cooldown with counts + BPM ranges
  - Queue list with phase separators and “DJ notes” per phase
  - A small “Intent Summary” line from plan
  - Keep ability to save the queue as a playlist (if you already have a save mechanism; otherwise add a TODO)

Frontend data types:
- Extend API client types in services/api.ts to include dj payload
- Render gracefully if dj payload absent (playlist mode)

Design style:
- Minimal, modern, dark-friendly
- Clear hierarchy: Prompt -> Persona -> Controls -> Output
- Avoid clutter; use cards + subtle labels

================================================================================
TESTING
================================================================================
Add unit tests (or at minimum a small test harness) for:
- Plan JSON validation
- Phase count distribution
- Sequencer continuity behavior with flowStrictness extremes
- Persona scoring differences (DeepCut vs CrowdPleaser)

================================================================================
DELIVERABLES CHECKLIST
================================================================================
1) backend/internal/dj/dj_set_planner.go (new)
2) backend/internal/dj/personas.go (new)
3) backend/internal/dj/sequencer.go (new)
4) backend/internal/api/smart_playlist.go (updated)
5) backend/internal/llm/prompts.go (updated with new templates)
6) services/api.ts (updated types)
7) pages/SmartPlaylists.tsx (UI redesigned into DJ Console)
8) Basic tests or a debug endpoint for development validation

Proceed to implement. Use clear, maintainable code. Keep changes incremental and safe.
