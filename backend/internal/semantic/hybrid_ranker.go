package semantic

import (
	"sort"
	"strings"

	"github.com/ajbergh/viib-mediahub/internal/db"
)

const (
	semanticRelevanceWeight  = 0.70
	behaviorPreferenceWeight = 0.20
	metadataFitWeight        = 0.10

	artistRepetitionPenalty = 0.03
	preferencePlayCountCap  = 20.0
	semanticAbsoluteFloor   = 0.22
	semanticRelativeWindow  = 0.32
)

// HybridRankingOptions contains request-scoped policy that is deliberately
// kept out of semantic documents and embeddings.
type HybridRankingOptions struct {
	DiscoverMode      string
	RecentlyPlayedIDs map[string]bool
	OnePerArtist      bool
	Limit             int
	PreferredGenres   []string
	MinYear           int
	MaxYear           int
}

// RankedSemanticCandidate exposes ranking contributions to callers without
// exposing document text or embedding vectors.
type RankedSemanticCandidate struct {
	Candidate     SemanticCandidate
	Score         float64
	SemanticScore float64
	BehaviorScore float64
	MetadataScore float64
}

// RankSemanticCandidates applies the Phase 1 70/20/10 hybrid model to a
// bounded semantic candidate pool. It removes recent tracks, honours an
// optional one-per-artist rule, and applies a small dynamic artist repetition
// penalty while selecting the final order. The function is deterministic.
func RankSemanticCandidates(candidates []SemanticCandidate, options HybridRankingOptions) []RankedSemanticCandidate {
	eligible := make([]RankedSemanticCandidate, 0, len(candidates))
	bestSemanticScore := 0.0
	for _, candidate := range candidates {
		if candidate.Song.ID == "" || options.RecentlyPlayedIDs[candidate.Song.ID] {
			continue
		}
		semanticScore := clampSemanticScore(candidate.Evidence.Relevance())
		bestSemanticScore = max(bestSemanticScore, semanticScore)
		behaviorScore := semanticBehaviorScore(candidate.Song, options.DiscoverMode)
		metadataScore := semanticMetadataFit(candidate.Song, options)
		eligible = append(eligible, RankedSemanticCandidate{
			Candidate:     candidate,
			Score:         semanticRelevanceWeight*semanticScore + behaviorPreferenceWeight*behaviorScore + metadataFitWeight*metadataScore,
			SemanticScore: semanticScore,
			BehaviorScore: behaviorScore,
			MetadataScore: metadataScore,
		})
	}
	// Do not fill a requested playlist with the long tail of weak semantic
	// matches. Provider score scales vary, so combine an absolute guard with a
	// window relative to the best candidate in this request.
	qualityFloor := max(semanticAbsoluteFloor, bestSemanticScore-semanticRelativeWindow)
	qualityEligible := eligible[:0]
	for _, candidate := range eligible {
		if candidate.SemanticScore >= qualityFloor {
			qualityEligible = append(qualityEligible, candidate)
		}
	}
	eligible = qualityEligible

	selected := make([]RankedSemanticCandidate, 0, len(eligible))
	artistSelections := make(map[string]int)
	for len(eligible) > 0 && (options.Limit <= 0 || len(selected) < options.Limit) {
		best := -1
		bestScore := -1.0
		for index := range eligible {
			artist := normalizedArtist(eligible[index].Candidate.Song.Artist)
			if options.OnePerArtist && artist != "" && artistSelections[artist] > 0 {
				continue
			}
			adjusted := eligible[index].Score - artistRepetitionPenalty*float64(artistSelections[artist])
			if best == -1 || adjusted > bestScore || (adjusted == bestScore && eligible[index].Candidate.Song.ID < eligible[best].Candidate.Song.ID) {
				best = index
				bestScore = adjusted
			}
		}
		if best == -1 {
			break
		}
		choice := eligible[best]
		choice.Score = bestScore
		selected = append(selected, choice)
		artist := normalizedArtist(choice.Candidate.Song.Artist)
		if artist != "" {
			artistSelections[artist]++
		}
		eligible = append(eligible[:best], eligible[best+1:]...)
	}
	return selected
}

func semanticBehaviorScore(song db.Song, discoverMode string) float64 {
	playFamiliarity := clampSemanticScore(float64(song.PlayCount) / preferencePlayCountCap)
	completion := 0.5
	if song.PlayCount > 0 {
		completion = clampSemanticScore(1 - float64(song.SkipCount)/float64(song.PlayCount))
	}
	liked := 0.0
	if song.Liked {
		liked = 1
	}
	switch strings.ToLower(strings.TrimSpace(discoverMode)) {
	case "discover":
		return clampSemanticScore(0.60*(1-playFamiliarity) + 0.20*completion + 0.20*liked)
	case "favorites":
		return clampSemanticScore(0.50*liked + 0.35*playFamiliarity + 0.15*completion)
	default:
		return clampSemanticScore(0.45*liked + 0.30*playFamiliarity + 0.25*completion)
	}
}

func semanticMetadataFit(song db.Song, options HybridRankingOptions) float64 {
	signals := make([]float64, 0, 2)
	if len(options.PreferredGenres) > 0 {
		fit := 0.0
		for _, preferred := range options.PreferredGenres {
			for _, genre := range song.Genre {
				if strings.EqualFold(strings.TrimSpace(preferred), strings.TrimSpace(genre)) {
					fit = 1
					break
				}
			}
			if fit == 1 {
				break
			}
		}
		signals = append(signals, fit)
	}
	if options.MinYear > 0 || options.MaxYear > 0 {
		year := song.OriginalYear
		if year == 0 {
			year = song.Year
		}
		fit := 0.5
		if year != 0 {
			fit = 1
			if options.MinYear > 0 && year < options.MinYear {
				fit = 0
			}
			if options.MaxYear > 0 && year > options.MaxYear {
				fit = 0
			}
		}
		signals = append(signals, fit)
	}
	if len(signals) == 0 {
		return 0.5
	}
	total := 0.0
	for _, signal := range signals {
		total += signal
	}
	return total / float64(len(signals))
}

func clampSemanticScore(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}

func normalizedArtist(artist string) string {
	return strings.ToLower(strings.TrimSpace(artist))
}

// SortRankedSemanticCandidates preserves a deterministic score ordering for
// callers that combine independently ranked slices before the diversity pass.
func SortRankedSemanticCandidates(candidates []RankedSemanticCandidate) {
	sort.SliceStable(candidates, func(left, right int) bool {
		if candidates[left].Score == candidates[right].Score {
			return candidates[left].Candidate.Song.ID < candidates[right].Candidate.Song.ID
		}
		return candidates[left].Score > candidates[right].Score
	})
}
