package api

import (
	"context"
	"strings"

	"github.com/ajbergh/viib-mediahub/internal/db"
	"github.com/ajbergh/viib-mediahub/internal/dj"
	"github.com/ajbergh/viib-mediahub/internal/llm"
	"github.com/ajbergh/viib-mediahub/internal/semantic"
)

const semanticDJMinimumPhasePool = 24

type semanticDJRequest struct {
	Prompt            string
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
	Audit               playlistValidationDiagnostics
	SearchLimitReached  bool
	AuditLimitReached   bool
}

// Each phase uses the same adaptive recall/refill path as ordinary playlists.
// Audit decisions are shared across phases, but relevance/negative intent stay
// phase-specific. A genuine shortage stays semantic; it never broadens genres.
func (a *API) retrieveSemanticDJPhasePools(ctx context.Context, plan *dj.DJSetPlan, request semanticDJRequest) (semanticDJRetrievalResult, bool, error) {
	return a.retrieveSemanticDJPhasePoolsWithAudit(ctx, plan, request, a.auditPlaylistSongs)
}

func (a *API) retrieveSemanticDJPhasePoolsWithAudit(ctx context.Context, plan *dj.DJSetPlan, request semanticDJRequest, audit playlistAuditor) (semanticDJRetrievalResult, bool, error) {
	if a.currentSemanticService() == nil || plan == nil || len(plan.Phases) == 0 {
		return semanticDJRetrievalResult{}, false, nil
	}
	result := semanticDJRetrievalResult{Pools: make([]dj.PhaseCandidatePool, len(plan.Phases)), PhaseCandidateCount: make([]int, len(plan.Phases))}
	totalTarget := 0
	for _, phase := range plan.Phases {
		totalTarget += max(phase.TargetCount, 1)
	}
	session := newPlaylistAuditSession()
	sharedAudit := func(ctx context.Context, prompt string, _ llm.PlaylistIntent, songs []db.Song) ([]db.Song, playlistValidationDiagnostics) {
		beforeReviewed, beforeRejected := session.diagnostics.AuditReviewed, session.diagnostics.AuditRejected
		accepted := session.filter(ctx, prompt, request.Intent, songs, audit)
		diagnostics := session.diagnostics
		diagnostics.AuditReviewed -= beforeReviewed
		diagnostics.AuditRejected -= beforeRejected
		return accepted, diagnostics
	}
	for index, phase := range plan.Phases {
		intent := request.Intent
		intent.SemanticQuery = strings.TrimSpace(phase.SemanticQuery)
		if intent.SemanticQuery == "" {
			intent.SemanticQuery = strings.TrimSpace(plan.IntentSummary)
		}
		if intent.SemanticQuery == "" {
			return semanticDJRetrievalResult{}, false, nil
		}
		if strings.TrimSpace(phase.NegativeSemanticQuery) != "" {
			intent.NegativeSemanticQuery = phase.NegativeSemanticQuery
		}
		// Include reserves for overlap across the complete arc, so consuming
		// a song in an earlier phase does not exhaust later phase pools.
		reserve := min(maximumPlaylistRefillCandidates, max(semanticDJMinimumPhasePool, max(phase.TargetCount*5, totalTarget)))
		playlist, handled, err := a.retrieveSemanticPlaylistWithAudit(ctx, intent, semanticPlaylistRequest{
			Prompt: request.Prompt, TargetSongs: reserve, Source: request.Source,
			DiscoverMode: request.DiscoverMode, RecentlyPlayedIDs: request.RecentlyPlayedIDs,
		}, sharedAudit)
		if err != nil || !handled {
			return semanticDJRetrievalResult{}, false, err
		}
		result.Pools[index] = dj.PhaseCandidatePool{Songs: playlist.Songs, SemanticScores: playlist.SemanticScores}
		result.PhaseCandidateCount[index] = len(playlist.Songs)
		result.CandidateCount += playlist.Diagnostics.CandidateCount
		result.Diagnostics.HardExcluded += playlist.Diagnostics.Validation.HardExcluded
		result.Diagnostics.StyleMismatches += playlist.Diagnostics.Validation.StyleMismatches
		result.Diagnostics.NegativeRejected += playlist.Diagnostics.Validation.NegativeRejected
		result.RankingFiltered += playlist.Diagnostics.Validation.RankingFiltered
		result.SearchLimitReached = result.SearchLimitReached || playlist.Diagnostics.SearchLimitReached
	}
	result.Audit = session.diagnostics
	result.AuditLimitReached = session.attempted >= maximumPlaylistRefillCandidates
	return result, true, nil
}

// Empty constrained phases are valid shortages, not sequencer failures. Keep
// their positions in the response while sequencing only non-empty phase pools.
func sequenceAvailableSemanticPhases(pools []dj.PhaseCandidatePool, plan *dj.DJSetPlan, persona dj.PersonaDefinition, score *dj.ScoreContext) ([]db.Song, []dj.PhaseResult, error) {
	filteredPlan := *plan
	filteredPlan.Phases = nil
	var filteredPools []dj.PhaseCandidatePool
	var indices []int
	results := make([]dj.PhaseResult, len(plan.Phases))
	for i, phase := range plan.Phases {
		results[i] = dj.PhaseResult{Name: phase.Name, SongIDs: []string{}, Notes: "No eligible matching tracks available"}
		if len(pools[i].Songs) > 0 {
			filteredPlan.Phases = append(filteredPlan.Phases, phase)
			filteredPools = append(filteredPools, pools[i])
			indices = append(indices, i)
		}
	}
	if len(indices) == 0 {
		return []db.Song{}, results, nil
	}
	queue, phases, err := dj.NewSequencer().BuildQueueFromPhasePools(filteredPools, &filteredPlan, persona, score)
	if err != nil {
		return nil, nil, err
	}
	for i, result := range phases {
		results[indices[i]] = result
	}
	return queue, results, nil
}
