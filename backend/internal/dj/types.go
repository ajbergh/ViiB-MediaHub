// Package dj provides the DJ Set Planner functionality for ViiB MediaHub.
//
// This package implements:
//   - DJSetPlan: Structured energy/tempo/mood curves across phases
//   - Persona-based scoring biases for different DJ styles
//   - Intelligent song sequencing with BPM continuity
//   - Reactive adaptation based on listening events
//
// The DJ mode builds on top of the existing smart playlist system,
// adding structured phase planning and deliberate sequencing.
package dj

import (
	"strconv"
	"strings"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/db"
)

// ============================================================================
// DJ Phase & Plan Types
// ============================================================================

// DJPhase represents a single phase in a DJ set (e.g., Warm-up, Build, Peak).
// Each phase has target energy/tempo/mood parameters and BPM constraints.
type DJPhase struct {
	Name                  string   `json:"name"`         // "Warm-up", "Build", "Peak", "Cooldown", "Afterhours"
	TargetEnergy          string   `json:"targetEnergy"` // "low", "medium", "high"
	TargetTempo           string   `json:"targetTempo"`  // "slow", "medium", "fast"
	TargetMoods           []string `json:"targetMoods"`  // e.g., ["calm", "dreamy"]
	TargetCount           int      `json:"targetCount"`  // Number of songs for this phase
	MinBPM                int      `json:"minBPM"`       // Minimum BPM for this phase
	MaxBPM                int      `json:"maxBPM"`       // Maximum BPM for this phase
	Notes                 string   `json:"notes"`        // Short DJ note for this phase
	SemanticQuery         string   `json:"semanticQuery"`
	NegativeSemanticQuery string   `json:"negativeSemanticQuery,omitempty"`
	StyleHints            []string `json:"styleHints,omitempty"`
}

// DJSetPlan represents the complete structure of a DJ set.
// It includes intent summary, phases, and seed information.
type DJSetPlan struct {
	IntentSummary     string    `json:"intentSummary"`     // 1-sentence summary of vibe and intent
	TargetDurationMin int       `json:"targetDurationMin"` // Target set duration in minutes
	Persona           string    `json:"persona"`           // Active persona key
	FlowStrictness    int       `json:"flowStrictness"`    // 0-100, higher = stricter BPM continuity
	Phases            []DJPhase `json:"phases"`            // Ordered list of phases
	SeedGenres        []string  `json:"seedGenres"`        // Matched genres from filter
	SeedArtists       []string  `json:"seedArtists"`       // Matched artists from filter
	CreatedAtUnix     int64     `json:"createdAtUnix"`     // Unix timestamp of creation
	FromCache         bool      `json:"fromCache"`         // True if loaded from cache
}

// PhaseResult represents the actual songs selected for a phase after sequencing.
type PhaseResult struct {
	Name      string   `json:"name"`      // Phase name
	SongIDs   []string `json:"songIds"`   // Selected song IDs in order
	AvgBPM    int      `json:"avgBpm"`    // Average BPM of selected songs
	MinBPM    int      `json:"minBpm"`    // Actual min BPM in selection
	MaxBPM    int      `json:"maxBpm"`    // Actual max BPM in selection
	Notes     string   `json:"notes"`     // Computed notes for this phase
	SongCount int      `json:"songCount"` // Number of songs in phase
}

// PhaseCandidatePool is the bounded recall result for one DJ phase. Semantic
// score maps are phase-specific because the same song can have a different
// relevance to a warm-up than to a peak-time query.
type PhaseCandidatePool struct {
	Songs          []db.Song
	SemanticScores map[string]float64
}

// DJNarration contains optional DJ talk mode narration cues.
type DJNarration struct {
	Intro       string   `json:"intro"`       // Opening DJ line
	PhaseIntros []string `json:"phaseIntros"` // One line per phase transition
	Outro       string   `json:"outro"`       // Closing DJ line
}

// DJResponse is the full DJ mode response payload.
type DJResponse struct {
	Plan      *DJSetPlan    `json:"plan"`
	Phases    []PhaseResult `json:"phases"`
	Narration *DJNarration  `json:"narration,omitempty"` // Only if talkMode enabled
}

// ============================================================================
// Plan Options & Context
// ============================================================================

// PlanOptions configures how a DJ set plan should be generated.
type PlanOptions struct {
	Persona           string `json:"persona"`           // Persona key (e.g., "FlowMaster")
	TargetDurationMin int    `json:"targetDurationMin"` // Target duration in minutes
	FlowStrictness    int    `json:"flowStrictness"`    // 0-100 strictness
	UseTimeContext    bool   `json:"useTimeContext"`    // Whether to use time-of-day
	TalkMode          bool   `json:"talkMode"`          // Generate narration cues
}

// DefaultPlanOptions returns sensible default options for DJ set planning.
func DefaultPlanOptions() PlanOptions {
	return PlanOptions{
		Persona:           PersonaFlowMaster,
		TargetDurationMin: 45,
		FlowStrictness:    60,
		UseTimeContext:    true,
		TalkMode:          false,
	}
}

// ============================================================================
// Score Context
// ============================================================================

// ScoreContext provides contextual information for scoring songs.
// It tracks the current state of sequencing and user preferences.
type ScoreContext struct {
	// Genre affinity scores from user's listening history
	GenreAffinity map[string]float64

	// Set of artist names already used in this set (for one-per-artist)
	ArtistSeen map[string]bool

	// Recently played song IDs to avoid (keyed by ID)
	RecentlyPlayedIDs map[string]bool

	// Song IDs already used in the set. This prevents semantic phase pools from
	// selecting the same catalog identity twice across an arc.
	UsedSongIDs map[string]bool

	// Last song context for continuity scoring
	LastSongBPM    int
	LastSongMood   string
	LastSongEnergy string
	LastSongTempo  string

	// User preference settings
	DiscoverMode   string // "balanced", "discover", "favorites"
	FlowStrictness int    // 0-100

	// Aggregated listening stats for preference learning
	SongSkipRates       map[string]float64 // songID -> skip rate (0-1)
	GenreCompletionRate map[string]float64 // genre -> avg completion rate (0-1)

	// Current timestamp for time-aware recommendations
	CurrentTime time.Time

	// SemanticScores holds retrieval relevance keyed by ViiB song ID.
	SemanticScores map[string]float64
}

// NewScoreContext creates a new empty ScoreContext with initialized maps.
func NewScoreContext() *ScoreContext {
	return &ScoreContext{
		GenreAffinity:       make(map[string]float64),
		ArtistSeen:          make(map[string]bool),
		RecentlyPlayedIDs:   make(map[string]bool),
		UsedSongIDs:         make(map[string]bool),
		SongSkipRates:       make(map[string]float64),
		GenreCompletionRate: make(map[string]float64),
		SemanticScores:      make(map[string]float64),
		CurrentTime:         time.Now(),
		DiscoverMode:        "balanced",
		FlowStrictness:      60,
	}
}

// ============================================================================
// Score Breakdown (for debugging/transparency)
// ============================================================================

// ScoreBreakdown provides detailed information about how a song was scored.
// Useful for debugging and understanding DJ decisions.
type ScoreBreakdown struct {
	SongID             string  `json:"songId"`
	TotalScore         float64 `json:"totalScore"`
	SemanticScore      float64 `json:"semanticScore,omitempty"`
	PhaseFitScore      float64 `json:"phaseFitScore"`      // How well song fits phase requirements
	BPMMatchScore      float64 `json:"bpmMatchScore"`      // BPM within phase range
	BPMContinuityScore float64 `json:"bpmContinuityScore"` // BPM proximity to last song
	MoodMatchScore     float64 `json:"moodMatchScore"`     // Mood alignment with phase
	EnergyMatchScore   float64 `json:"energyMatchScore"`   // Energy level match
	TempoMatchScore    float64 `json:"tempoMatchScore"`    // Tempo category match
	PersonaBonus       float64 `json:"personaBonus"`       // Persona-specific bonuses
	AffinityBonus      float64 `json:"affinityBonus"`      // Genre affinity bonus
	RecencyPenalty     float64 `json:"recencyPenalty"`     // Penalty for recently played
	ArtistPenalty      float64 `json:"artistPenalty"`      // Penalty if artist already used
	Notes              string  `json:"notes,omitempty"`    // Human-readable notes
}

// ============================================================================
// Request/Response Extensions for API
// ============================================================================

// DJModeRequest extends the smart playlist request with DJ-specific fields.
type DJModeRequest struct {
	// Mode: "playlist" (default) or "dj"
	Mode string `json:"mode"`

	// Persona key (default "FlowMaster")
	Persona string `json:"persona"`

	// Target set duration in minutes (default 45)
	TargetDurationMinutes int `json:"targetDurationMinutes"`

	// Enable DJ talk mode narration (default false)
	TalkMode bool `json:"talkMode"`

	// Flow strictness 0-100 (default 60)
	// Higher = stricter BPM continuity, less flexibility
	FlowStrictness int `json:"flowStrictness"`
}

// DefaultDJModeRequest returns a request with default DJ mode settings.
func DefaultDJModeRequest() DJModeRequest {
	return DJModeRequest{
		Mode:                  ModePlaylist,
		Persona:               PersonaFlowMaster,
		TargetDurationMinutes: 45,
		TalkMode:              false,
		FlowStrictness:        60,
	}
}

// ============================================================================
// Plan Cache Key
// ============================================================================

// PlanCacheKey generates a unique cache key for a DJ set plan.
// Used for the in-memory TTL cache to avoid redundant LLM calls.
type PlanCacheKey struct {
	Provider          string
	Model             string
	NormalizedPrompt  string
	Persona           string
	TargetDurationMin int
	FlowStrictness    int
	UseTimeContext    bool
	TimeBucket        string
}

// String returns a string representation of the cache key for use as a map key.
func (k PlanCacheKey) String() string {
	parts := []string{
		k.Provider,
		k.Model,
		k.NormalizedPrompt,
		k.Persona,
		strconv.Itoa(k.TargetDurationMin),
		strconv.Itoa(k.FlowStrictness),
		strconv.FormatBool(k.UseTimeContext),
	}
	if k.UseTimeContext {
		parts = append(parts, k.TimeBucket)
	}
	return strings.Join(parts, "|")
}
