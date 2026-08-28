package api

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/ajbergh/viib-mediahub/internal/db"
	"github.com/ajbergh/viib-mediahub/internal/dj"
	"github.com/ajbergh/viib-mediahub/internal/llm"
	"github.com/ajbergh/viib-mediahub/internal/semantic"
)

const semanticDJMinimumPhasePool = 24

type semanticDJRequest struct {
	Source            string
	DiscoverMode      string
	RecentlyPlayedIDs map[string]bool
	Intent            llm.PlaylistIntent
}

type semanticDJRetrievalResult struct {
	Pools               []dj.PhaseCandidatePool
	CandidateCount      int
	PhaseCandidateCount []int
	Diagnostics         semantic.SemanticFilterDiagnostics
	RankingFiltered     int
}

// retrieveSemanticDJPhasePools resolves each planned semantic query into a
// bounded pool for the sequencer. A false result means the caller should use
// the established full-catalog fallback because semantic retrieval is absent
// or cannot produce enough songs for a complete phase.
func (a *API) retrieveSemanticDJPhasePools(ctx context.Context, plan *dj.DJSetPlan, request semanticDJRequest) (semanticDJRetrievalResult, bool, error) {
	service := a.currentSemanticService()
	if service == nil || plan == nil || len(plan.Phases) == 0 {
		return semanticDJRetrievalResult{}, false, nil
	}
	result := semanticDJRetrievalResult{
		Pools:               make([]dj.PhaseCandidatePool, len(plan.Phases)),
		PhaseCandidateCount: make([]int, len(plan.Phases)),
	}
	distinctSongIDs := make(map[string]struct{})
	targetSongCount := 0
	for index, phase := range plan.Phases {
		query := strings.TrimSpace(phase.SemanticQuery)
		if query == "" {
			query = strings.TrimSpace(plan.IntentSummary)
		}
		if query == "" {
			return semanticDJRetrievalResult{}, false, nil
		}
		negativeQuery := phase.NegativeSemanticQuery
		if strings.TrimSpace(negativeQuery) == "" {
			negativeQuery = request.Intent.NegativeSemanticQuery
		}
		retrieval, err := service.RetrieveSemanticCandidates(ctx, query, semantic.SemanticRetrievalOptions{
			Source:                request.Source,
			IncludeArtists:        request.Intent.IncludeArtists,
			ExcludeArtists:        request.Intent.ExcludeArtists,
			MinYear:               request.Intent.MinYear,
			MaxYear:               request.Intent.MaxYear,
			YearConstraintHard:    request.Intent.YearConstraintHard,
			InstrumentalOnly:      request.Intent.InstrumentalOnly,
			NegativeSemanticQuery: negativeQuery,
			RequiredStyles:        request.Intent.RequiredStyles,
			ExcludedTerms:         request.Intent.ExcludedTerms,
		})
		if err != nil {
			if errors.Is(err, semantic.ErrNoSearchableSemanticIndex) {
				return semanticDJRetrievalResult{}, false, nil
			}
			return semanticDJRetrievalResult{}, false, fmt.Errorf("phase %q semantic retrieval: %w", phase.Name, err)
		}
		result.Diagnostics.HardExcluded += retrieval.Diagnostics.HardExcluded
		result.Diagnostics.StyleMismatches += retrieval.Diagnostics.StyleMismatches
		result.Diagnostics.NegativeRejected += retrieval.Diagnostics.NegativeRejected
		targetCount := max(phase.TargetCount, 1)
		targetSongCount += targetCount
		if len(retrieval.Candidates) < targetCount {
			return semanticDJRetrievalResult{}, false, nil
		}
		ranked := semantic.RankSemanticCandidates(retrieval.Candidates, semantic.HybridRankingOptions{
			DiscoverMode:      request.DiscoverMode,
			RecentlyPlayedIDs: request.RecentlyPlayedIDs,
			Limit:             semanticPlaylistRankingLimit,
			PreferredGenres:   request.Intent.PreferredGenres,
			MinYear:           request.Intent.MinYear,
			MaxYear:           request.Intent.MaxYear,
		})
		result.RankingFiltered += max(0, len(retrieval.Candidates)-len(ranked))
		poolLimit := min(semanticPlaylistRankingLimit, max(semanticDJMinimumPhasePool, targetCount*5))
		selected, err := service.ApplyMMRDiversity(ctx, ranked, semantic.DiversityOptions{
			DiscoverMode: request.DiscoverMode,
			Limit:        poolLimit,
		})
		if err != nil {
			return semanticDJRetrievalResult{}, false, fmt.Errorf("phase %q diversity ranking: %w", phase.Name, err)
		}
		if len(selected) < targetCount {
			return semanticDJRetrievalResult{}, false, nil
		}
		pool := dj.PhaseCandidatePool{
			Songs:          make([]db.Song, len(selected)),
			SemanticScores: make(map[string]float64, len(selected)),
		}
		for candidateIndex, candidate := range selected {
			pool.Songs[candidateIndex] = candidate.Candidate.Song
			pool.SemanticScores[candidate.Candidate.Song.ID] = candidate.Candidate.Evidence.Relevance()
			distinctSongIDs[candidate.Candidate.Song.ID] = struct{}{}
		}
		result.Pools[index] = pool
		result.PhaseCandidateCount[index] = len(pool.Songs)
		result.CandidateCount += len(retrieval.Candidates)
	}
	if len(distinctSongIDs) < targetSongCount {
		return semanticDJRetrievalResult{}, false, nil
	}
	return result, true, nil
}
