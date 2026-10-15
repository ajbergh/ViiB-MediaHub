package playlist

import (
	"math"
	"strings"
)

type Track struct {
	ID, Artist, Album, Mood string
	Features                AudioFeatures
	Relevance               float64
}

// TransitionScorer returns false when current artifacts are unavailable.
type TransitionScorer func(fromID, toID string, intent AudioIntent) (float64, bool)

// Order chooses within a bounded relevance window and looks one transition
// ahead. It never adds/removes tracks or turns missing evidence into a veto.
func Order(tracks []Track, intent AudioIntent, transition TransitionScorer) []Track {
	return OrderAfter(tracks, intent, transition, nil)
}

func OrderAfter(tracks []Track, intent AudioIntent, transition TransitionScorer, predecessor *Track) []Track {
	remaining := append([]Track(nil), tracks...)
	ordered := make([]Track, 0, len(tracks))
	minimum, maximum := 10.0, 1.0
	for _, track := range tracks {
		if usable(track.Features.EnergyLevel) {
			minimum = math.Min(minimum, track.Features.EnergyLevel.Value)
			maximum = math.Max(maximum, track.Features.EnergyLevel.Value)
		}
	}
	for len(remaining) > 0 {
		best, bestScore := 0, math.Inf(-1)
		window := min(64, len(remaining))
		for i, candidate := range remaining[:window] {
			// Preserve relevance and stable initial ranking when no audio evidence exists.
			score := .15*candidate.Relevance - .00001*float64(i)
			if intent.EnergyDirection != "" && usable(candidate.Features.EnergyLevel) && maximum >= minimum {
				progress := float64(len(ordered)) / float64(max(1, len(tracks)-1))
				target := minimum + (maximum-minimum)*progress
				switch intent.EnergyDirection {
				case "cooldown":
					target = maximum - (maximum-minimum)*progress
				case "arc":
					target = minimum + (maximum-minimum)*math.Sin(math.Pi*progress)
				case "steady":
					target = (minimum + maximum) / 2
				}
				score += .4 * (confidenceScore(math.Max(0, 1-math.Abs(candidate.Features.EnergyLevel.Value-target)/4), candidate.Features.EnergyLevel.Confidence) - .5)
			}
			if len(ordered) > 0 || predecessor != nil {
				previous := Track{}
				if len(ordered) > 0 {
					previous = ordered[len(ordered)-1]
				} else {
					previous = *predecessor
				}
				pair, known := PairScore(previous.Features, candidate.Features, intent)
				if transition != nil {
					if native, ok := transition(previous.ID, candidate.ID, intent); ok {
						if known {
							pair = (pair + native) / 2
						} else {
							pair = native
						}
						known = true
					}
				}
				if known {
					score += .5 * (pair - .5)
				}
				if previous.Mood != "" && candidate.Mood != "" && strings.EqualFold(previous.Mood, candidate.Mood) {
					score += .03
				}
				if candidate.Artist != "" && strings.EqualFold(previous.Artist, candidate.Artist) {
					score -= .08
				}
				if candidate.Album != "" && strings.EqualFold(previous.Album, candidate.Album) && strings.EqualFold(previous.Artist, candidate.Artist) {
					score -= .03
				}
				// A small lookahead avoids spending the only easy transition too early.
				if len(remaining) > 2 {
					nextBest := 0.0
					knownNext := false
					for j, next := range remaining[:min(12, len(remaining))] {
						if j == i {
							continue
						}
						if pair, ok := PairScore(candidate.Features, next.Features, intent); ok {
							nextBest = math.Max(nextBest, pair)
							knownNext = true
						}
					}
					if knownNext {
						score += .05 * (nextBest - .5)
					}
				}
			}
			if score > bestScore {
				best, bestScore = i, score
			}
		}
		ordered = append(ordered, remaining[best])
		remaining = append(remaining[:best], remaining[best+1:]...)
	}
	return ordered
}

func Duration(tracks []Track) (seconds float64, estimated bool) {
	for _, track := range tracks {
		duration := track.Features.DurationSeconds
		if !finite(duration) || duration <= 0 {
			duration = 180
			estimated = true
		}
		seconds += duration
	}
	return
}

// ClosestDurationPrefix keeps a coherent ranked selection and chooses the
// nearest whole-track duration. Duration is an approximate preference.
func ClosestDurationPrefix(tracks []Track, targetSeconds float64) []Track {
	if targetSeconds <= 0 || len(tracks) == 0 {
		return tracks
	}
	best, total, distance := 1, 0.0, math.Inf(1)
	for i, track := range tracks {
		duration := track.Features.DurationSeconds
		if !finite(duration) || duration <= 0 {
			duration = 180
		}
		total += duration
		if delta := math.Abs(total - targetSeconds); delta < distance {
			best, distance = i+1, delta
		}
		if total >= targetSeconds {
			break
		}
	}
	return tracks[:best]
}
