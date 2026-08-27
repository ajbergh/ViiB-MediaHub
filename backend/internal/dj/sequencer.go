package dj

import (
	"fmt"
	"math"
	"math/rand"
	"sort"
	"strings"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/db"
)

// ============================================================================
// Sequencer
// ============================================================================

// Sequencer builds the final song queue from candidates and a DJ set plan.
type Sequencer struct {
	rng *rand.Rand
}

// NewSequencer creates a new Sequencer with a random seed.
func NewSequencer() *Sequencer {
	return &Sequencer{
		rng: rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// BuildQueue selects and orders songs to match each phase in the plan.
// Returns the final queue, phase results, and any error.
func (s *Sequencer) BuildQueue(
	candidates []db.Song,
	plan *DJSetPlan,
	persona PersonaDefinition,
	ctx *ScoreContext,
) ([]db.Song, []PhaseResult, error) {
	if plan == nil || len(plan.Phases) == 0 {
		return nil, nil, fmt.Errorf("plan is required with at least one phase")
	}
	if len(candidates) == 0 {
		return nil, nil, fmt.Errorf("no candidates provided")
	}
	pools := make([]PhaseCandidatePool, len(plan.Phases))
	for index := range pools {
		pools[index].Songs = candidates
	}
	return s.BuildQueueFromPhasePools(pools, plan, persona, ctx)
}

// BuildQueueFromPhasePools sequences independently retrieved phase pools. It
// retains the existing metadata/BPM scoring and stochastic selection, but the
// semantic retriever is now the primary recall mechanism rather than a
// full-library energy or tempo bucket scan.
func (s *Sequencer) BuildQueueFromPhasePools(
	pools []PhaseCandidatePool,
	plan *DJSetPlan,
	persona PersonaDefinition,
	ctx *ScoreContext,
) ([]db.Song, []PhaseResult, error) {
	if plan == nil || len(plan.Phases) == 0 {
		return nil, nil, fmt.Errorf("plan is required with at least one phase")
	}
	if len(pools) < len(plan.Phases) {
		return nil, nil, fmt.Errorf("phase candidate pools do not cover the plan")
	}

	var queue []db.Song
	var phaseResults []PhaseResult

	// Process each phase
	for i, phase := range plan.Phases {
		pool := pools[i]
		if len(pool.Songs) == 0 {
			return nil, nil, fmt.Errorf("phase %q has no candidates", phase.Name)
		}
		// Partition only this phase's bounded recall set. The existing energy,
		// tempo, BPM, persona, and stochastic logic remains selection logic.
		buckets := s.partitionCandidates(pool.Songs)
		previousScores := ctx.SemanticScores
		if pool.SemanticScores != nil {
			ctx.SemanticScores = pool.SemanticScores
		}
		phaseResult, phaseSongs := s.selectForPhase(phase, buckets, persona, ctx, pool.Songs)
		ctx.SemanticScores = previousScores

		// Apply micro-shuffle within phase if low flow strictness
		if ctx.FlowStrictness < FlowStrictnessNormal && len(phaseSongs) > 3 {
			phaseSongs = s.microShuffle(phaseSongs, 3)
		}

		// Add to queue
		queue = append(queue, phaseSongs...)
		phaseResults = append(phaseResults, phaseResult)

		// Update context for next phase
		for _, song := range phaseSongs {
			ctx.MarkArtistSeen(song.Artist)
			ctx.UpdateLastSong(song)
			if ctx.UsedSongIDs == nil {
				ctx.UsedSongIDs = make(map[string]bool)
			}
			ctx.UsedSongIDs[song.ID] = true
		}

		// Log progress
		_ = i // Avoid unused variable warning
	}

	return queue, phaseResults, nil
}

// ============================================================================
// Candidate Partitioning
// ============================================================================

// songBucket groups songs by energy/tempo for faster phase matching.
type songBucket struct {
	energy string
	tempo  string
	songs  []db.Song
}

// partitionCandidates divides candidates into buckets by energy and tempo.
func (s *Sequencer) partitionCandidates(candidates []db.Song) map[string]*songBucket {
	buckets := make(map[string]*songBucket)

	energies := []string{EnergyLow, EnergyMedium, EnergyHigh, ""}
	tempos := []string{TempoSlow, TempoMedium, TempoFast, ""}

	// Initialize all bucket combinations
	for _, e := range energies {
		for _, t := range tempos {
			key := e + "|" + t
			buckets[key] = &songBucket{
				energy: e,
				tempo:  t,
				songs:  []db.Song{},
			}
		}
	}

	// Assign songs to buckets
	for _, song := range candidates {
		energy := strings.ToLower(song.Energy)
		tempo := strings.ToLower(song.Tempo)

		// Normalize unknown values
		if energy != EnergyLow && energy != EnergyMedium && energy != EnergyHigh {
			energy = ""
		}
		if tempo != TempoSlow && tempo != TempoMedium && tempo != TempoFast {
			tempo = ""
		}

		key := energy + "|" + tempo
		if bucket, ok := buckets[key]; ok {
			bucket.songs = append(bucket.songs, song)
		}
	}

	return buckets
}

// ============================================================================
// Phase Selection
// ============================================================================

// selectForPhase selects songs for a single phase.
func (s *Sequencer) selectForPhase(
	phase DJPhase,
	buckets map[string]*songBucket,
	persona PersonaDefinition,
	ctx *ScoreContext,
	allCandidates []db.Song, // Fallback pool
) (PhaseResult, []db.Song) {
	targetCount := phase.TargetCount
	if targetCount < 1 {
		targetCount = 1
	}

	// Get candidates from matching buckets
	candidates := s.getCandidatesForPhase(phase, buckets)

	// If not enough candidates, try relaxing constraints
	if len(candidates) < targetCount {
		candidates = s.relaxAndExpand(phase, buckets, candidates, targetCount)
	}

	// If still not enough, use all candidates as fallback
	if len(candidates) < targetCount {
		candidates = allCandidates
	}

	// Score all candidates
	scored := ScoreAllCandidates(candidates, phase, ctx, persona)

	// Sort by score descending
	sort.Slice(scored, func(i, j int) bool {
		return scored[i].Score > scored[j].Score
	})

	// Select top candidates with stochastic sampling
	selected := s.selectWithStochasticity(scored, targetCount, persona.Weights.OnePerArtistStrictness, ctx)

	// Sort selected by BPM for smooth transitions within phase
	s.sortByBPM(selected)

	// Build phase result
	result := s.buildPhaseResult(phase, selected)

	return result, selected
}

// getCandidatesForPhase gets candidates from buckets matching the phase.
func (s *Sequencer) getCandidatesForPhase(phase DJPhase, buckets map[string]*songBucket) []db.Song {
	var candidates []db.Song

	// Primary bucket: exact match
	primaryKey := strings.ToLower(phase.TargetEnergy) + "|" + strings.ToLower(phase.TargetTempo)
	if bucket, ok := buckets[primaryKey]; ok {
		candidates = append(candidates, bucket.songs...)
	}

	// Adjacent buckets for more options
	adjacentEnergies := getAdjacentEnergies(phase.TargetEnergy)
	adjacentTempos := getAdjacentTempos(phase.TargetTempo)

	for _, e := range adjacentEnergies {
		key := strings.ToLower(e) + "|" + strings.ToLower(phase.TargetTempo)
		if bucket, ok := buckets[key]; ok {
			candidates = append(candidates, bucket.songs...)
		}
	}

	for _, t := range adjacentTempos {
		key := strings.ToLower(phase.TargetEnergy) + "|" + strings.ToLower(t)
		if bucket, ok := buckets[key]; ok {
			candidates = append(candidates, bucket.songs...)
		}
	}

	// Unknown energy/tempo buckets as fallback
	for _, e := range []string{phase.TargetEnergy, ""} {
		for _, t := range []string{phase.TargetTempo, ""} {
			key := strings.ToLower(e) + "|" + strings.ToLower(t)
			if bucket, ok := buckets[key]; ok && len(bucket.songs) > 0 {
				// Add songs not already included
				for _, song := range bucket.songs {
					found := false
					for _, c := range candidates {
						if c.ID == song.ID {
							found = true
							break
						}
					}
					if !found {
						candidates = append(candidates, song)
					}
				}
			}
		}
	}

	return candidates
}

// relaxAndExpand expands the candidate pool by relaxing constraints.
func (s *Sequencer) relaxAndExpand(phase DJPhase, buckets map[string]*songBucket, current []db.Song, targetCount int) []db.Song {
	candidates := current
	existingIDs := make(map[string]bool)
	for _, c := range candidates {
		existingIDs[c.ID] = true
	}

	// Relaxation order: mood -> bpm range -> tempo -> energy
	// Add songs from all buckets that we haven't considered

	allBuckets := []string{}
	for key := range buckets {
		allBuckets = append(allBuckets, key)
	}

	for _, key := range allBuckets {
		bucket := buckets[key]
		for _, song := range bucket.songs {
			if !existingIDs[song.ID] {
				candidates = append(candidates, song)
				existingIDs[song.ID] = true
			}
			if len(candidates) >= targetCount*3 {
				break
			}
		}
	}

	return candidates
}

// selectWithStochasticity selects songs using weighted random sampling.
func (s *Sequencer) selectWithStochasticity(
	scored []ScoredSong,
	targetCount int,
	onePerArtistStrictness float64,
	ctx *ScoreContext,
) []db.Song {
	if len(scored) == 0 {
		return nil
	}

	selected := make([]db.Song, 0, targetCount)
	artistsInPhase := make(map[string]bool)
	usedIDs := make(map[string]bool)

	// Take from top 50 candidates with softmax-style selection
	poolSize := 50
	if poolSize > len(scored) {
		poolSize = len(scored)
	}
	pool := scored[:poolSize]

	for len(selected) < targetCount && len(pool) > 0 {
		// Calculate softmax weights
		weights := make([]float64, len(pool))
		maxScore := pool[0].Score
		for i, s := range pool {
			// Temperature scaling (lower = more deterministic)
			temperature := 0.3
			weights[i] = math.Exp((s.Score - maxScore) / temperature)

			// Apply one-per-artist penalty
			artistLower := strings.ToLower(s.Song.Artist)
			if artistsInPhase[artistLower] && onePerArtistStrictness > 0.5 {
				weights[i] *= 0.1 // Heavy penalty
			} else if artistsInPhase[artistLower] {
				weights[i] *= 0.5 // Moderate penalty
			}

			// Skip already used
			if usedIDs[s.Song.ID] || (ctx.UsedSongIDs != nil && ctx.UsedSongIDs[s.Song.ID]) {
				weights[i] = 0
			}
		}

		// Normalize weights
		totalWeight := 0.0
		for _, w := range weights {
			totalWeight += w
		}

		if totalWeight <= 0 {
			// All remaining songs have been used or blocked
			break
		}

		// Weighted random selection
		r := s.rng.Float64() * totalWeight
		cumulative := 0.0
		selectedIdx := 0
		for i, w := range weights {
			cumulative += w
			if r <= cumulative {
				selectedIdx = i
				break
			}
		}

		// Add selected song
		song := pool[selectedIdx].Song
		selected = append(selected, song)
		usedIDs[song.ID] = true
		artistsInPhase[strings.ToLower(song.Artist)] = true

		// Remove from pool
		pool = append(pool[:selectedIdx], pool[selectedIdx+1:]...)
	}

	return selected
}

// sortByBPM sorts songs by BPM for smooth transitions.
func (s *Sequencer) sortByBPM(songs []db.Song) {
	sort.Slice(songs, func(i, j int) bool {
		bpmI := getSongBPM(songs[i])
		bpmJ := getSongBPM(songs[j])
		return bpmI < bpmJ
	})
}

// microShuffle performs a light shuffle within windows.
func (s *Sequencer) microShuffle(songs []db.Song, windowSize int) []db.Song {
	if len(songs) <= windowSize {
		return songs
	}

	result := make([]db.Song, len(songs))
	copy(result, songs)

	// Shuffle within each window
	for start := 0; start < len(result); start += windowSize {
		end := start + windowSize
		if end > len(result) {
			end = len(result)
		}

		// Fisher-Yates shuffle on the window
		for i := end - 1; i > start; i-- {
			j := start + s.rng.Intn(i-start+1)
			result[i], result[j] = result[j], result[i]
		}
	}

	return result
}

// buildPhaseResult creates a PhaseResult from selected songs.
func (s *Sequencer) buildPhaseResult(phase DJPhase, songs []db.Song) PhaseResult {
	result := PhaseResult{
		Name:      phase.Name,
		SongIDs:   make([]string, 0, len(songs)),
		Notes:     phase.Notes,
		SongCount: len(songs),
	}

	if len(songs) == 0 {
		return result
	}

	// Calculate BPM stats
	var totalBPM, minBPM, maxBPM int
	minBPM = 999
	maxBPM = 0

	for _, song := range songs {
		result.SongIDs = append(result.SongIDs, song.ID)

		bpm := getSongBPM(song)
		if bpm > 0 {
			totalBPM += bpm
			if bpm < minBPM {
				minBPM = bpm
			}
			if bpm > maxBPM {
				maxBPM = bpm
			}
		}
	}

	if len(songs) > 0 && totalBPM > 0 {
		result.AvgBPM = totalBPM / len(songs)
		result.MinBPM = minBPM
		result.MaxBPM = maxBPM
	}

	return result
}

// ============================================================================
// Helper Functions
// ============================================================================

// getAdjacentEnergies returns energy levels adjacent to the target.
func getAdjacentEnergies(energy string) []string {
	switch strings.ToLower(energy) {
	case EnergyLow:
		return []string{EnergyMedium}
	case EnergyHigh:
		return []string{EnergyMedium}
	case EnergyMedium:
		return []string{EnergyLow, EnergyHigh}
	default:
		return []string{EnergyMedium}
	}
}

// getAdjacentTempos returns tempo levels adjacent to the target.
func getAdjacentTempos(tempo string) []string {
	switch strings.ToLower(tempo) {
	case TempoSlow:
		return []string{TempoMedium}
	case TempoFast:
		return []string{TempoMedium}
	case TempoMedium:
		return []string{TempoSlow, TempoFast}
	default:
		return []string{TempoMedium}
	}
}
