package dj

import (
	"strings"
)

// ============================================================================
// Persona Weights
// ============================================================================

// PersonaWeights defines the scoring biases for a DJ persona.
// Each weight influences how songs are scored during sequencing.
type PersonaWeights struct {
	// UnderplayedBoost favors songs with low play counts (0.0-1.0)
	UnderplayedBoost float64 `json:"underplayedBoost"`

	// FavoritesBoost favors songs with high play counts and completion rates (0.0-1.0)
	FavoritesBoost float64 `json:"favoritesBoost"`

	// AffinityBoost favors songs in genres with high user affinity (0.0-1.0)
	AffinityBoost float64 `json:"affinityBoost"`

	// NoveltyBoost favors genres with mid affinity but underexplored (0.0-1.0)
	NoveltyBoost float64 `json:"noveltyBoost"`

	// BPMContinuityWeight penalizes large BPM jumps between songs (0.0-1.0)
	BPMContinuityWeight float64 `json:"bpmContinuityWeight"`

	// MoodContinuityWeight penalizes mood changes between songs (0.0-1.0)
	MoodContinuityWeight float64 `json:"moodContinuityWeight"`

	// EnergyContinuityWeight penalizes energy level changes (0.0-1.0)
	EnergyContinuityWeight float64 `json:"energyContinuityWeight"`

	// OnePerArtistStrictness enforces artist variety (0.0=none, 1.0=strict)
	OnePerArtistStrictness float64 `json:"onePerArtistStrictness"`

	// InstrumentalBias favors/disfavors instrumental tracks (-1.0 to 1.0)
	InstrumentalBias float64 `json:"instrumentalBias"`

	// RecencyPenalty reduces score for recently played songs (0.0-1.0)
	RecencyPenalty float64 `json:"recencyPenalty"`

	// SkipPenalty reduces score based on historical skip rate (0.0-1.0)
	SkipPenalty float64 `json:"skipPenalty"`

	// LikedBoost favors user-liked songs (0.0-1.0)
	LikedBoost float64 `json:"likedBoost"`

	// TempoVarianceAllowed controls how much tempo can vary within a phase (0.0-1.0)
	TempoVarianceAllowed float64 `json:"tempoVarianceAllowed"`

	// EnergyVarianceAllowed controls how much energy can vary within a phase (0.0-1.0)
	EnergyVarianceAllowed float64 `json:"energyVarianceAllowed"`
}

// ============================================================================
// Persona Definition
// ============================================================================

// PersonaDefinition fully describes a DJ persona with its key, name,
// description, and scoring weights.
type PersonaDefinition struct {
	Key         string         `json:"key"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Weights     PersonaWeights `json:"weights"`
}

// ============================================================================
// Persona Registry
// ============================================================================

// allPersonas is the internal registry of all available personas.
var allPersonas = map[string]PersonaDefinition{
	PersonaFlowMaster:   flowMasterPersona,
	PersonaCrowdPleaser: crowdPleaserPersona,
	PersonaDeepCutDJ:    deepCutDJPersona,
	PersonaExplorer:     explorerPersona,
	PersonaCurator:      curatorPersona,
	PersonaNightDrive:   nightDrivePersona,
}

// ============================================================================
// Persona Definitions
// ============================================================================

// flowMasterPersona is the default persona with strong continuity and balanced novelty.
var flowMasterPersona = PersonaDefinition{
	Key:         PersonaFlowMaster,
	Name:        "Flow Master",
	Description: "Smooth transitions with balanced discovery. The default choice for seamless DJ sets.",
	Weights: PersonaWeights{
		UnderplayedBoost:       0.3,
		FavoritesBoost:         0.4,
		AffinityBoost:          0.5,
		NoveltyBoost:           0.3,
		BPMContinuityWeight:    0.8, // Strong BPM continuity
		MoodContinuityWeight:   0.6,
		EnergyContinuityWeight: 0.7,
		OnePerArtistStrictness: 0.6,
		InstrumentalBias:       0.0, // Neutral
		RecencyPenalty:         0.5,
		SkipPenalty:            0.4,
		LikedBoost:             0.3,
		TempoVarianceAllowed:   0.4,
		EnergyVarianceAllowed:  0.4,
	},
}

// crowdPleaserPersona favors high completion rates, favorites, and familiar songs.
var crowdPleaserPersona = PersonaDefinition{
	Key:         PersonaCrowdPleaser,
	Name:        "Crowd Pleaser",
	Description: "Plays the hits and favorites. High energy, crowd-tested tracks.",
	Weights: PersonaWeights{
		UnderplayedBoost:       0.1, // Low - prefers popular tracks
		FavoritesBoost:         0.9, // Very high - plays what works
		AffinityBoost:          0.7,
		NoveltyBoost:           0.1, // Low - sticks to familiar
		BPMContinuityWeight:    0.5, // Moderate continuity
		MoodContinuityWeight:   0.4,
		EnergyContinuityWeight: 0.5,
		OnePerArtistStrictness: 0.4, // Some repeats OK
		InstrumentalBias:       0.0,
		RecencyPenalty:         0.3, // Less penalty - OK to repeat hits
		SkipPenalty:            0.8, // Heavy skip penalty
		LikedBoost:             0.8, // Strongly favors liked songs
		TempoVarianceAllowed:   0.5,
		EnergyVarianceAllowed:  0.5,
	},
}

// deepCutDJPersona has heavy underplayed boost and high novelty.
var deepCutDJPersona = PersonaDefinition{
	Key:         PersonaDeepCutDJ,
	Name:        "Deep Cut DJ",
	Description: "Unearths hidden gems and rarely played tracks. For adventurous listeners.",
	Weights: PersonaWeights{
		UnderplayedBoost:       0.9, // Very high - loves obscure tracks
		FavoritesBoost:         0.1, // Low - avoids popular
		AffinityBoost:          0.3,
		NoveltyBoost:           0.8, // High novelty
		BPMContinuityWeight:    0.5, // Moderate
		MoodContinuityWeight:   0.4,
		EnergyContinuityWeight: 0.4,
		OnePerArtistStrictness: 0.7, // Variety is key
		InstrumentalBias:       0.1, // Slight instrumental preference
		RecencyPenalty:         0.7, // Strong recency penalty
		SkipPenalty:            0.2, // Less concerned about skips
		LikedBoost:             0.2,
		TempoVarianceAllowed:   0.6,
		EnergyVarianceAllowed:  0.6,
	},
}

// explorerPersona offers controlled novelty with medium continuity.
var explorerPersona = PersonaDefinition{
	Key:         PersonaExplorer,
	Name:        "Explorer",
	Description: "Balances familiar favorites with new discoveries. Expands your horizons.",
	Weights: PersonaWeights{
		UnderplayedBoost:       0.5, // Moderate
		FavoritesBoost:         0.4,
		AffinityBoost:          0.4,
		NoveltyBoost:           0.6, // Good novelty
		BPMContinuityWeight:    0.5,
		MoodContinuityWeight:   0.5,
		EnergyContinuityWeight: 0.5,
		OnePerArtistStrictness: 0.6,
		InstrumentalBias:       0.0,
		RecencyPenalty:         0.5,
		SkipPenalty:            0.4,
		LikedBoost:             0.4,
		TempoVarianceAllowed:   0.5,
		EnergyVarianceAllowed:  0.5,
	},
}

// curatorPersona has strict genre purity and one-per-artist enforcement.
var curatorPersona = PersonaDefinition{
	Key:         PersonaCurator,
	Name:        "Curator",
	Description: "Carefully selected tracks with strict variety. One artist, one chance.",
	Weights: PersonaWeights{
		UnderplayedBoost:       0.4,
		FavoritesBoost:         0.5,
		AffinityBoost:          0.8, // Strong affinity focus
		NoveltyBoost:           0.3,
		BPMContinuityWeight:    0.7,
		MoodContinuityWeight:   0.7,
		EnergyContinuityWeight: 0.6,
		OnePerArtistStrictness: 1.0, // Strict one-per-artist
		InstrumentalBias:       0.0,
		RecencyPenalty:         0.6,
		SkipPenalty:            0.5,
		LikedBoost:             0.5,
		TempoVarianceAllowed:   0.3, // Tight variance
		EnergyVarianceAllowed:  0.3,
	},
}

// nightDrivePersona favors smoother tempos and medium energy for late night vibes.
var nightDrivePersona = PersonaDefinition{
	Key:         PersonaNightDrive,
	Name:        "Night Drive",
	Description: "Smooth, atmospheric tracks for late night sessions. Medium energy, nostalgic vibes.",
	Weights: PersonaWeights{
		UnderplayedBoost:       0.4,
		FavoritesBoost:         0.5,
		AffinityBoost:          0.5,
		NoveltyBoost:           0.3,
		BPMContinuityWeight:    0.9, // Very smooth transitions
		MoodContinuityWeight:   0.8, // Strong mood consistency
		EnergyContinuityWeight: 0.8, // Keep energy steady
		OnePerArtistStrictness: 0.5,
		InstrumentalBias:       0.2, // Slight instrumental preference
		RecencyPenalty:         0.4,
		SkipPenalty:            0.4,
		LikedBoost:             0.5,
		TempoVarianceAllowed:   0.2, // Very tight tempo
		EnergyVarianceAllowed:  0.2, // Very tight energy
	},
}

// ============================================================================
// Persona Access Functions
// ============================================================================

// GetPersona returns the persona definition for the given key.
// Returns FlowMaster as default if key is not found.
func GetPersona(key string) PersonaDefinition {
	// Normalize key for case-insensitive lookup
	normalizedKey := normalizePersonaKey(key)

	if persona, ok := allPersonas[normalizedKey]; ok {
		return persona
	}

	// Return default persona
	return flowMasterPersona
}

// ListPersonas returns all available persona definitions.
func ListPersonas() []PersonaDefinition {
	personas := make([]PersonaDefinition, 0, len(allPersonas))

	// Return in a consistent order
	order := []string{
		PersonaFlowMaster,
		PersonaCrowdPleaser,
		PersonaDeepCutDJ,
		PersonaExplorer,
		PersonaCurator,
		PersonaNightDrive,
	}

	for _, key := range order {
		if persona, ok := allPersonas[key]; ok {
			personas = append(personas, persona)
		}
	}

	return personas
}

// PersonaKeys returns all valid persona keys.
func PersonaKeys() []string {
	return []string{
		PersonaFlowMaster,
		PersonaCrowdPleaser,
		PersonaDeepCutDJ,
		PersonaExplorer,
		PersonaCurator,
		PersonaNightDrive,
	}
}

// IsValidPersona checks if a persona key is valid.
func IsValidPersona(key string) bool {
	normalizedKey := normalizePersonaKey(key)
	_, ok := allPersonas[normalizedKey]
	return ok
}

// ============================================================================
// Helper Functions
// ============================================================================

// normalizePersonaKey normalizes a persona key for case-insensitive lookup.
func normalizePersonaKey(key string) string {
	key = strings.TrimSpace(key)

	// Direct match first
	if _, ok := allPersonas[key]; ok {
		return key
	}

	// Case-insensitive match
	lowerKey := strings.ToLower(key)
	for k := range allPersonas {
		if strings.ToLower(k) == lowerKey {
			return k
		}
	}

	return key
}

// DefaultPersonaWeights returns the weights for the default persona (FlowMaster).
func DefaultPersonaWeights() PersonaWeights {
	return flowMasterPersona.Weights
}

// ============================================================================
// Weight Application Helpers
// ============================================================================

// CalculateBPMPenalty calculates the penalty for a BPM jump based on persona weights.
// Returns a value between 0.0 (no penalty) and 1.0 (maximum penalty).
func (w *PersonaWeights) CalculateBPMPenalty(bpmDiff int) float64 {
	if bpmDiff < 0 {
		bpmDiff = -bpmDiff
	}

	// No continuity weight = no penalty
	if w.BPMContinuityWeight <= 0 {
		return 0.0
	}

	// Calculate base penalty based on BPM difference
	var basePenalty float64
	switch {
	case bpmDiff <= BPMJumpSmall:
		basePenalty = 0.0
	case bpmDiff <= BPMJumpMedium:
		basePenalty = 0.3
	case bpmDiff <= BPMJumpLarge:
		basePenalty = 0.6
	default:
		basePenalty = 1.0
	}

	// Apply weight
	return basePenalty * w.BPMContinuityWeight
}

// CalculateArtistPenalty calculates the penalty for repeating an artist.
// Returns 1.0 if artist should be blocked, 0.0 if no penalty.
func (w *PersonaWeights) CalculateArtistPenalty(artistSeen bool) float64 {
	if !artistSeen {
		return 0.0
	}
	return w.OnePerArtistStrictness
}

// CalculateRecencyPenalty calculates the penalty for a recently played song.
// hoursAgo is how many hours since the song was last played.
func (w *PersonaWeights) CalculateRecencyPenalty(hoursAgo float64) float64 {
	if w.RecencyPenalty <= 0 {
		return 0.0
	}

	// Decay over 72 hours (3 days)
	const decayHours = 72.0

	if hoursAgo >= decayHours {
		return 0.0
	}

	// Linear decay
	remainingPenalty := 1.0 - (hoursAgo / decayHours)
	return remainingPenalty * w.RecencyPenalty
}

// CalculateSkipPenalty calculates the penalty based on historical skip rate.
// skipRate is 0.0 to 1.0 (percentage of plays that were skipped).
func (w *PersonaWeights) CalculateSkipPenalty(skipRate float64) float64 {
	if w.SkipPenalty <= 0 || skipRate <= 0 {
		return 0.0
	}
	return skipRate * w.SkipPenalty
}

// CalculateFavoritesBonus calculates the bonus for a frequently played song.
// completionRate is the percentage of full plays (0.0 to 1.0).
// playCount is the raw play count (normalized internally).
func (w *PersonaWeights) CalculateFavoritesBonus(completionRate float64, playCount int) float64 {
	if w.FavoritesBoost <= 0 {
		return 0.0
	}

	// Normalize play count (diminishing returns after 50 plays)
	normalizedPlayCount := float64(playCount) / 50.0
	if normalizedPlayCount > 1.0 {
		normalizedPlayCount = 1.0
	}

	// Combine completion rate and play count
	combinedScore := (completionRate*0.6 + normalizedPlayCount*0.4)
	return combinedScore * w.FavoritesBoost
}

// CalculateUnderplayedBonus calculates the bonus for rarely played songs.
// playCount is the raw play count.
func (w *PersonaWeights) CalculateUnderplayedBonus(playCount int) float64 {
	if w.UnderplayedBoost <= 0 {
		return 0.0
	}

	// Inverse relationship - lower play count = higher bonus
	// Max bonus at 0 plays, diminishes to 0 at 20+ plays
	if playCount >= 20 {
		return 0.0
	}

	underplayedScore := 1.0 - (float64(playCount) / 20.0)
	return underplayedScore * w.UnderplayedBoost
}

// CalculateLikedBonus returns the bonus for a liked song.
func (w *PersonaWeights) CalculateLikedBonus(isLiked bool) float64 {
	if !isLiked || w.LikedBoost <= 0 {
		return 0.0
	}
	return w.LikedBoost
}
