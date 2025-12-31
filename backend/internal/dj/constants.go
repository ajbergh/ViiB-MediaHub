package dj

// ============================================================================
// Mode Constants
// ============================================================================

// Mode constants for the smart playlist API.
const (
	ModePlaylist = "playlist" // Standard playlist mode
	ModeDJ       = "dj"       // DJ set planning mode
)

// ============================================================================
// Persona Keys
// ============================================================================

// Persona key constants for DJ set planning.
// Each persona applies different scoring weights.
const (
	PersonaFlowMaster   = "FlowMaster"   // Default: strong continuity, balanced novelty
	PersonaCrowdPleaser = "CrowdPleaser" // Favors high completion, favorites, familiar songs
	PersonaDeepCutDJ    = "DeepCutDJ"    // Heavy underplayed boost, novelty, low favorites bias
	PersonaExplorer     = "Explorer"     // Controlled novelty, medium continuity
	PersonaCurator      = "Curator"      // Strict genre purity, one-per-artist
	PersonaNightDrive   = "NightDrive"   // Smoother tempos, medium energy, nostalgic
)

// ============================================================================
// Phase Names
// ============================================================================

// Phase name constants for DJ set structure.
const (
	PhaseWarmUp     = "Warm-up"    // Opening phase, building mood
	PhaseBuild      = "Build"      // Energy building phase
	PhasePeak       = "Peak"       // High energy climax
	PhaseCooldown   = "Cooldown"   // Winding down
	PhaseAfterHours = "Afterhours" // Late night chill phase
)

// ValidPhaseNames returns all valid phase names.
func ValidPhaseNames() []string {
	return []string{PhaseWarmUp, PhaseBuild, PhasePeak, PhaseCooldown, PhaseAfterHours}
}

// ============================================================================
// Energy/Tempo/Mood Constants
// ============================================================================

// Energy level constants.
const (
	EnergyLow    = "low"
	EnergyMedium = "medium"
	EnergyHigh   = "high"
)

// Tempo category constants.
const (
	TempoSlow   = "slow"
	TempoMedium = "medium"
	TempoFast   = "fast"
)

// ============================================================================
// BPM Defaults
// ============================================================================

// Default BPM values when song BPM is unknown.
// These are used as approximations based on tempo category.
const (
	DefaultBPMSlow   = 90  // Slow tempo default BPM
	DefaultBPMMedium = 120 // Medium tempo default BPM
	DefaultBPMFast   = 150 // Fast tempo default BPM
)

// BPM range constraints.
const (
	MinValidBPM = 60  // Minimum valid BPM
	MaxValidBPM = 190 // Maximum valid BPM
)

// ============================================================================
// Phase Distribution Weights
// ============================================================================

// Default phase distribution weights for song count allocation.
// These represent the typical percentage of songs in each phase.
var DefaultPhaseWeights = map[string]float64{
	PhaseWarmUp:     0.25, // 25% of songs
	PhaseBuild:      0.25, // 25% of songs
	PhasePeak:       0.30, // 30% of songs
	PhaseCooldown:   0.20, // 20% of songs
	PhaseAfterHours: 0.20, // 20% (alternative to cooldown)
}

// ============================================================================
// Average Song Duration
// ============================================================================

// Default average song length in seconds for duration calculations.
const DefaultAvgSongLengthSec = 210 // 3.5 minutes

// ============================================================================
// Cache Configuration
// ============================================================================

// Plan cache TTL in minutes.
const (
	PlanCacheTTLMinutes = 30 // How long to cache LLM-generated plans
)

// ============================================================================
// Scoring Constants
// ============================================================================

// BPM continuity thresholds for scoring.
const (
	BPMJumpSmall  = 5  // Small BPM difference (minimal penalty)
	BPMJumpMedium = 15 // Medium BPM difference (moderate penalty)
	BPMJumpLarge  = 30 // Large BPM difference (significant penalty)
)

// Flow strictness thresholds.
const (
	FlowStrictnessLoose  = 30 // Below this, very loose constraints
	FlowStrictnessNormal = 60 // Standard flow enforcement
	FlowStrictnessStrict = 80 // Above this, very strict BPM continuity
)

// One-per-artist constraints.
const (
	OnePerArtistStrict = 1.0 // Never repeat artists
	OnePerArtistLoose  = 0.5 // Allow some repeats
	OnePerArtistNone   = 0.0 // No constraint
)

// ============================================================================
// Validation Constants
// ============================================================================

// Plan validation constraints.
const (
	MinPhaseCount     = 3   // Minimum phases in a plan
	MaxPhaseCount     = 5   // Maximum phases in a plan
	MinTargetDuration = 15  // Minimum duration in minutes
	MaxTargetDuration = 180 // Maximum duration in minutes (3 hours)
)

// ============================================================================
// Helper Functions
// ============================================================================

// TempoToBPM converts a tempo category to a default BPM value.
func TempoToBPM(tempo string) int {
	switch tempo {
	case TempoSlow:
		return DefaultBPMSlow
	case TempoMedium:
		return DefaultBPMMedium
	case TempoFast:
		return DefaultBPMFast
	default:
		return DefaultBPMMedium
	}
}

// BPMToTempo converts a BPM value to a tempo category.
func BPMToTempo(bpm int) string {
	if bpm <= 100 {
		return TempoSlow
	} else if bpm <= 135 {
		return TempoMedium
	}
	return TempoFast
}

// EnergyToNumeric converts energy level to a numeric value for calculations.
func EnergyToNumeric(energy string) float64 {
	switch energy {
	case EnergyLow:
		return 0.25
	case EnergyMedium:
		return 0.5
	case EnergyHigh:
		return 0.75
	default:
		return 0.5
	}
}

// NumericToEnergy converts a numeric value back to energy level.
func NumericToEnergy(val float64) string {
	if val < 0.35 {
		return EnergyLow
	} else if val < 0.65 {
		return EnergyMedium
	}
	return EnergyHigh
}

// CalculateTargetSongCount estimates the number of songs needed for a duration.
func CalculateTargetSongCount(durationMinutes int, avgSongLengthSec int) int {
	if avgSongLengthSec <= 0 {
		avgSongLengthSec = DefaultAvgSongLengthSec
	}
	totalSeconds := durationMinutes * 60
	count := totalSeconds / avgSongLengthSec
	if count < 1 {
		count = 1
	}
	return count
}

// ValidateBPMRange checks if a BPM range is valid.
func ValidateBPMRange(minBPM, maxBPM int) bool {
	return minBPM >= MinValidBPM && maxBPM <= MaxValidBPM && minBPM < maxBPM
}

// ClampBPM ensures a BPM value is within valid bounds.
func ClampBPM(bpm int) int {
	if bpm < MinValidBPM {
		return MinValidBPM
	}
	if bpm > MaxValidBPM {
		return MaxValidBPM
	}
	return bpm
}
