package dj

import (
	"strings"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/db"
)

// ============================================================================
// Song Scoring for DJ Mode
// ============================================================================

// ScoreSongForPhase calculates how well a song fits a phase given the persona.
// Returns a score from 0.0 to 1.0 (higher is better) and optional breakdown.
func ScoreSongForPhase(song db.Song, phase DJPhase, ctx *ScoreContext, persona PersonaDefinition) (float64, *ScoreBreakdown) {
	weights := persona.Weights
	breakdown := &ScoreBreakdown{
		SongID: song.ID,
	}

	var totalScore float64

	// ========================================================================
	// 1. Phase Fit Score (base relevance to phase requirements)
	// ========================================================================

	// Energy match
	energyScore := scoreEnergyMatch(song.Energy, phase.TargetEnergy)
	breakdown.EnergyMatchScore = energyScore

	// Tempo match
	tempoScore := scoreTempoMatch(song.Tempo, phase.TargetTempo)
	breakdown.TempoMatchScore = tempoScore

	// Mood match
	moodScore := scoreMoodMatch(song.Mood, phase.TargetMoods)
	breakdown.MoodMatchScore = moodScore

	// BPM range match
	bpmScore := scoreBPMRange(getSongBPM(song), phase.MinBPM, phase.MaxBPM)
	breakdown.BPMMatchScore = bpmScore

	// Combine phase fit (weighted average)
	phaseFitScore := (energyScore*0.3 + tempoScore*0.25 + moodScore*0.25 + bpmScore*0.2)
	breakdown.PhaseFitScore = phaseFitScore
	totalScore += phaseFitScore * 0.4 // 40% of total score

	// ========================================================================
	// 2. BPM Continuity Score (smoothness from previous song)
	// ========================================================================

	if ctx.LastSongBPM > 0 {
		songBPM := getSongBPM(song)
		bpmDiff := songBPM - ctx.LastSongBPM
		if bpmDiff < 0 {
			bpmDiff = -bpmDiff
		}

		// Calculate continuity score (inverted penalty)
		bpmPenalty := weights.CalculateBPMPenalty(bpmDiff)
		bpmContinuityScore := 1.0 - bpmPenalty

		// Apply flow strictness modifier
		strictnessMultiplier := float64(ctx.FlowStrictness) / 100.0
		bpmContinuityScore = bpmContinuityScore * (0.5 + 0.5*strictnessMultiplier)

		breakdown.BPMContinuityScore = bpmContinuityScore
		totalScore += bpmContinuityScore * weights.BPMContinuityWeight * 0.2 // Up to 20%
	} else {
		breakdown.BPMContinuityScore = 1.0 // First song has no penalty
		totalScore += 0.2
	}

	// ========================================================================
	// 3. Persona Bonuses
	// ========================================================================

	var personaBonus float64

	// Favorites bonus (based on play count and completion rate)
	completionRate := calculateCompletionRate(song, ctx)
	favoritesBonus := weights.CalculateFavoritesBonus(completionRate, song.PlayCount)
	personaBonus += favoritesBonus * 0.3

	// Underplayed bonus
	underplayedBonus := weights.CalculateUnderplayedBonus(song.PlayCount)
	personaBonus += underplayedBonus * 0.2

	// Liked song bonus
	likedBonus := weights.CalculateLikedBonus(song.Liked)
	personaBonus += likedBonus * 0.2

	breakdown.PersonaBonus = personaBonus
	totalScore += personaBonus * 0.2 // 20% of total

	// ========================================================================
	// 4. Genre Affinity Bonus
	// ========================================================================

	affinityBonus := calculateAffinityBonus(song.Genre, ctx.GenreAffinity, weights.AffinityBoost)
	breakdown.AffinityBonus = affinityBonus
	totalScore += affinityBonus * 0.1 // 10% of total

	// ========================================================================
	// 5. Penalties
	// ========================================================================

	var totalPenalty float64

	// Recency penalty
	if ctx.RecentlyPlayedIDs != nil && ctx.RecentlyPlayedIDs[song.ID] {
		recencyPenalty := weights.RecencyPenalty * 0.5 // Halve score for recently played
		breakdown.RecencyPenalty = recencyPenalty
		totalPenalty += recencyPenalty
	}

	// Artist penalty (one-per-artist)
	if ctx.ArtistSeen != nil && ctx.ArtistSeen[strings.ToLower(song.Artist)] {
		artistPenalty := weights.CalculateArtistPenalty(true)
		breakdown.ArtistPenalty = artistPenalty
		totalPenalty += artistPenalty * 0.3
	}

	// Skip rate penalty
	if ctx.SongSkipRates != nil {
		if skipRate, ok := ctx.SongSkipRates[song.ID]; ok {
			skipPenalty := weights.CalculateSkipPenalty(skipRate)
			totalPenalty += skipPenalty * 0.2
		}
	}

	// Apply penalties (subtract from score)
	totalScore = totalScore * (1.0 - totalPenalty*0.5)

	// Clamp to valid range
	if totalScore < 0 {
		totalScore = 0
	}
	if totalScore > 1 {
		totalScore = 1
	}

	breakdown.TotalScore = totalScore
	return totalScore, breakdown
}

// ============================================================================
// Score Component Functions
// ============================================================================

// scoreEnergyMatch scores how well the song energy matches the phase target.
func scoreEnergyMatch(songEnergy, targetEnergy string) float64 {
	if songEnergy == "" {
		return 0.5 // Unknown = neutral
	}

	songEnergy = strings.ToLower(songEnergy)
	targetEnergy = strings.ToLower(targetEnergy)

	if songEnergy == targetEnergy {
		return 1.0
	}

	// Adjacent levels get partial credit
	energyOrder := map[string]int{"low": 0, "medium": 1, "high": 2}
	songLevel, ok1 := energyOrder[songEnergy]
	targetLevel, ok2 := energyOrder[targetEnergy]

	if !ok1 || !ok2 {
		return 0.5
	}

	diff := songLevel - targetLevel
	if diff < 0 {
		diff = -diff
	}

	switch diff {
	case 0:
		return 1.0
	case 1:
		return 0.6
	default:
		return 0.2
	}
}

// scoreTempoMatch scores how well the song tempo matches the phase target.
func scoreTempoMatch(songTempo, targetTempo string) float64 {
	if songTempo == "" {
		return 0.5 // Unknown = neutral
	}

	songTempo = strings.ToLower(songTempo)
	targetTempo = strings.ToLower(targetTempo)

	if songTempo == targetTempo {
		return 1.0
	}

	// Adjacent tempos get partial credit
	tempoOrder := map[string]int{"slow": 0, "medium": 1, "fast": 2}
	songLevel, ok1 := tempoOrder[songTempo]
	targetLevel, ok2 := tempoOrder[targetTempo]

	if !ok1 || !ok2 {
		return 0.5
	}

	diff := songLevel - targetLevel
	if diff < 0 {
		diff = -diff
	}

	switch diff {
	case 0:
		return 1.0
	case 1:
		return 0.6
	default:
		return 0.2
	}
}

// scoreMoodMatch scores how well the song mood matches the phase target moods.
func scoreMoodMatch(songMood string, targetMoods []string) float64 {
	if songMood == "" || len(targetMoods) == 0 {
		return 0.5 // Unknown or no targets = neutral
	}

	songMood = strings.ToLower(songMood)

	// Check for direct match
	for _, target := range targetMoods {
		if strings.ToLower(target) == songMood {
			return 1.0
		}
	}

	// Check for related moods (partial match)
	relatedMoods := map[string][]string{
		"happy":      {"uplifting", "joyful", "fun", "cheerful"},
		"sad":        {"melancholic", "emotional", "heartbreak"},
		"energetic":  {"intense", "hype", "powerful"},
		"calm":       {"peaceful", "serene", "relaxed", "chill"},
		"romantic":   {"love", "sensual", "intimate"},
		"aggressive": {"angry", "intense", "powerful"},
		"nostalgic":  {"retro", "vintage", "throwback"},
		"uplifting":  {"happy", "inspiring", "hopeful"},
		"chill":      {"calm", "relaxed", "laid-back"},
		"dreamy":     {"ethereal", "atmospheric", "ambient"},
	}

	for _, target := range targetMoods {
		targetLower := strings.ToLower(target)

		// Check if song mood is related to target
		if related, ok := relatedMoods[targetLower]; ok {
			for _, r := range related {
				if r == songMood {
					return 0.7 // Related mood
				}
			}
		}

		// Check if target is related to song mood
		if related, ok := relatedMoods[songMood]; ok {
			for _, r := range related {
				if r == targetLower {
					return 0.7
				}
			}
		}
	}

	return 0.3 // No match
}

// scoreBPMRange scores how well the song BPM fits within the phase BPM range.
func scoreBPMRange(songBPM, minBPM, maxBPM int) float64 {
	if songBPM == 0 || minBPM == 0 || maxBPM == 0 {
		return 0.5 // Unknown = neutral
	}

	// Perfect fit: within range
	if songBPM >= minBPM && songBPM <= maxBPM {
		return 1.0
	}

	// Calculate distance from range
	var distance int
	if songBPM < minBPM {
		distance = minBPM - songBPM
	} else {
		distance = songBPM - maxBPM
	}

	// Penalize based on distance
	// Up to 10 BPM outside: 0.7
	// Up to 20 BPM outside: 0.4
	// More than 20 BPM: 0.1
	switch {
	case distance <= 10:
		return 0.7
	case distance <= 20:
		return 0.4
	default:
		return 0.1
	}
}

// ============================================================================
// Helper Functions
// ============================================================================

// getSongBPM returns the song's BPM, estimating from tempo if unknown.
func getSongBPM(song db.Song) int {
	if song.BPM > 0 {
		return song.BPM
	}
	return TempoToBPM(song.Tempo)
}

// calculateCompletionRate estimates the completion rate for a song.
// Uses skip count and play count to estimate.
func calculateCompletionRate(song db.Song, ctx *ScoreContext) float64 {
	// Check if we have pre-calculated rates
	if ctx.SongSkipRates != nil {
		if skipRate, ok := ctx.SongSkipRates[song.ID]; ok {
			return 1.0 - skipRate
		}
	}

	// Estimate from play count and skip count
	totalPlays := song.PlayCount + song.SkipCount
	if totalPlays == 0 {
		return 0.5 // Unknown = neutral
	}

	return float64(song.PlayCount) / float64(totalPlays)
}

// calculateAffinityBonus calculates the bonus based on genre affinity.
func calculateAffinityBonus(songGenres []string, affinityMap map[string]float64, weight float64) float64 {
	if len(songGenres) == 0 || len(affinityMap) == 0 || weight <= 0 {
		return 0
	}

	// Find the highest affinity for any of the song's genres
	maxAffinity := 0.0
	for _, genre := range songGenres {
		genreLower := strings.ToLower(genre)
		if affinity, ok := affinityMap[genreLower]; ok && affinity > maxAffinity {
			maxAffinity = affinity
		}
	}

	return maxAffinity * weight
}

// ============================================================================
// Score Context Builder
// ============================================================================

// BuildScoreContext creates a ScoreContext from database data.
func BuildScoreContext(
	genrePrefs map[string]float64,
	recentSongIDs []string,
	skipRates map[string]float64,
	discoverMode string,
	flowStrictness int,
) *ScoreContext {
	ctx := NewScoreContext()

	// Copy genre affinities
	for genre, score := range genrePrefs {
		ctx.GenreAffinity[strings.ToLower(genre)] = score
	}

	// Build recently played set
	for _, id := range recentSongIDs {
		ctx.RecentlyPlayedIDs[id] = true
	}

	// Copy skip rates
	for id, rate := range skipRates {
		ctx.SongSkipRates[id] = rate
	}

	ctx.DiscoverMode = discoverMode
	ctx.FlowStrictness = flowStrictness
	ctx.CurrentTime = time.Now()

	return ctx
}

// MarkArtistSeen marks an artist as seen in the context.
func (ctx *ScoreContext) MarkArtistSeen(artist string) {
	if ctx.ArtistSeen == nil {
		ctx.ArtistSeen = make(map[string]bool)
	}
	ctx.ArtistSeen[strings.ToLower(artist)] = true
}

// UpdateLastSong updates the context with the last selected song's properties.
func (ctx *ScoreContext) UpdateLastSong(song db.Song) {
	ctx.LastSongBPM = getSongBPM(song)
	ctx.LastSongMood = song.Mood
	ctx.LastSongEnergy = song.Energy
	ctx.LastSongTempo = song.Tempo
}

// ============================================================================
// Batch Scoring
// ============================================================================

// ScoreCandidates scores all candidates for a phase and returns sorted by score.
type ScoredSong struct {
	Song      db.Song
	Score     float64
	Breakdown *ScoreBreakdown
}

// ScoreAllCandidates scores all songs for a phase.
func ScoreAllCandidates(candidates []db.Song, phase DJPhase, ctx *ScoreContext, persona PersonaDefinition) []ScoredSong {
	scored := make([]ScoredSong, 0, len(candidates))

	for _, song := range candidates {
		score, breakdown := ScoreSongForPhase(song, phase, ctx, persona)
		scored = append(scored, ScoredSong{
			Song:      song,
			Score:     score,
			Breakdown: breakdown,
		})
	}

	return scored
}
