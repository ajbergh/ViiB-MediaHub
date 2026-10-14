// Defines semantic playlist functionality for package api.

package api

import (
	"context"
	"errors"
	"fmt"

	"github.com/ajbergh/viib-mediahub/internal/db"
	"github.com/ajbergh/viib-mediahub/internal/llm"
	"github.com/ajbergh/viib-mediahub/internal/semantic"
)

const semanticPlaylistRankingLimit = 120

// semanticPlaylistRequest contains the existing Smart Playlist controls that
// shape retrieval and ranking. It is deliberately internal so the public HTTP
// request and response contracts remain backward compatible.
type semanticPlaylistRequest struct {
	Prompt            string
	TargetSongs       int
	DiscoverMode      string
	RecentlyPlayedIDs map[string]bool
	OnePerArtist      bool
	Source            string
	PromptMinYear     int
	PromptMaxYear     int
}

// semanticRetrievalDiagnostics is optional response metadata. It contains
// counts only: raw documents and embedding vectors never leave the backend.
type semanticRetrievalDiagnostics struct {
	Mode                 string                        `json:"mode"`
	CandidateCount       int                           `json:"candidateCount"`
	ReturnedCount        int                           `json:"returnedCount"`
	TrackMatches         int                           `json:"trackMatches"`
	AlbumMatches         int                           `json:"albumMatches"`
	ArtistMatches        int                           `json:"artistMatches"`
	SearchPasses         int                           `json:"searchPasses"`
	SearchLimitReached   bool                          `json:"searchLimitReached,omitempty"`
	NegativeQueryApplied bool                          `json:"negativeQueryApplied"`
	Validation           playlistValidationDiagnostics `json:"validation"`
}

type semanticPlaylistResult struct {
	Filter         llm.PlaylistFilter
	Songs          []db.Song
	SemanticScores map[string]float64
	Diagnostics    semanticRetrievalDiagnostics
}

// compileSemanticPlaylistIntent prefers the dedicated compiler prompt. If a
// configured LLM is unavailable, semantic retrieval can still use its bounded
// raw-prompt fallback rather than making the Smart Playlist endpoint fail.
func (a *API) compileSemanticPlaylistIntent(ctx context.Context, prompt string) llm.PlaylistIntent {
	fallback := llm.FallbackPlaylistIntent(prompt)
	provider, err := llm.GetConfiguredProvider(a.db)
	if err != nil {
		return fallback
	}
	defer provider.Close()
	intent, err := provider.ParsePlaylistIntent(ctx, prompt)
	if err != nil {
		if intent.SemanticQuery != "" {
			return intent
		}
		return fallback
	}
	return intent
}

// retrieveSemanticPlaylist runs the complete Phase 1 playlist path when an
// in-memory semantic service has a ready index. The boolean is false only when
// the caller should use the existing legacy matching path instead.
func (a *API) retrieveSemanticPlaylist(ctx context.Context, intent llm.PlaylistIntent, request semanticPlaylistRequest) (semanticPlaylistResult, bool, error) {
	return a.retrieveSemanticPlaylistWithAudit(ctx, intent, request, a.auditPlaylistSongs)
}

func (a *API) retrieveSemanticPlaylistWithAudit(ctx context.Context, intent llm.PlaylistIntent, request semanticPlaylistRequest, audit playlistAuditor) (semanticPlaylistResult, bool, error) {
	service := a.currentSemanticService()
	if service == nil {
		return semanticPlaylistResult{}, false, nil
	}
	if request.PromptMinYear > 0 || request.PromptMaxYear > 0 {
		intent.MinYear = request.PromptMinYear
		intent.MaxYear = request.PromptMaxYear
		intent.YearConstraintHard = true
	}
	target := request.TargetSongs
	if target <= 0 {
		target = 50
	}
	target = min(target, maximumPlaylistRefillCandidates)
	session := newPlaylistAuditSession()
	var retrieval semantic.SemanticRetrievalResult
	var songs []db.Song
	scores := make(map[string]float64)
	var validation playlistValidationDiagnostics
	negativeApplied := false
	searchPasses := 0
	searchLimitReached := false
	for breadth := 1; breadth <= semantic.MaxSemanticSearchBreadth; breadth *= 4 {
		if err := ctx.Err(); err != nil {
			return semanticPlaylistResult{}, false, err
		}
		var err error
		retrieval, err = service.RetrieveSemanticCandidates(ctx, intent.SemanticQuery, semantic.SemanticRetrievalOptions{
			Source:                request.Source,
			SearchBreadth:         breadth,
			IncludeArtists:        intent.IncludeArtists,
			ExcludeArtists:        intent.ExcludeArtists,
			MinYear:               intent.MinYear,
			MaxYear:               intent.MaxYear,
			YearConstraintHard:    intent.YearConstraintHard,
			InstrumentalOnly:      intent.InstrumentalOnly,
			NegativeSemanticQuery: intent.NegativeSemanticQuery,
			RequiredStyles:        intent.RequiredStyles,
			ExcludedTerms:         intent.ExcludedTerms,
		})
		if err != nil {
			if errors.Is(err, semantic.ErrNoSearchableSemanticIndex) {
				return semanticPlaylistResult{}, false, nil
			}
			return semanticPlaylistResult{}, false, fmt.Errorf("semantic retrieval: %w", err)
		}
		searchPasses++
		for {
			if err := ctx.Err(); err != nil {
				return semanticPlaylistResult{}, false, err
			}
			ranked := semantic.RankSemanticCandidates(retrieval.Candidates, semantic.HybridRankingOptions{
				DiscoverMode: request.DiscoverMode, RecentlyPlayedIDs: request.RecentlyPlayedIDs,
				RejectedSongIDs: session.rejected, OnePerArtist: request.OnePerArtist,
				Limit: max(semanticPlaylistRankingLimit, target*2), PreferredGenres: intent.PreferredGenres,
				MinYear: intent.MinYear, MaxYear: intent.MaxYear,
			})
			validation = playlistValidationDiagnostics{
				RequiredStyles: intent.RequiredStyles, ExcludedTerms: intent.ExcludedTerms,
				HardExcluded: retrieval.Diagnostics.HardExcluded, StyleMismatches: retrieval.Diagnostics.StyleMismatches,
				NegativeRejected: retrieval.Diagnostics.NegativeRejected,
				RankingFiltered:  max(0, len(retrieval.Candidates)-len(ranked)),
			}
			selected, err := service.ApplyMMRDiversity(ctx, ranked, semantic.DiversityOptions{DiscoverMode: request.DiscoverMode, Limit: max(auditReserveLimit(target), target)})
			if err != nil {
				return semanticPlaylistResult{}, false, fmt.Errorf("semantic diversity ranking: %w", err)
			}
			offered := make([]db.Song, len(selected))
			negativeApplied = false
			for i, candidate := range selected {
				offered[i] = candidate.Candidate.Song
				scores[offered[i].ID] = candidate.Candidate.Evidence.Relevance()
				negativeApplied = negativeApplied || candidate.Candidate.Evidence.NegativeApplied
			}
			before := session.attempted
			songs = session.filter(ctx, request.Prompt, intent, offered, audit)
			if err := ctx.Err(); err != nil {
				return semanticPlaylistResult{}, false, err
			}
			if len(songs) >= target || session.attempted == before || session.attempted >= maximumPlaylistRefillCandidates {
				break
			}
			// Rerank without rejected tracks, exposing replacements beyond the
			// previous MMR window and alternate songs by a rejected artist.
		}
		if len(songs) >= target || session.attempted >= maximumPlaylistRefillCandidates || !retrieval.MoreAvailable {
			break
		}
		if breadth == semantic.MaxSemanticSearchBreadth {
			searchLimitReached = true
		}
	}
	validation.AuditStatus = session.diagnostics.AuditStatus
	validation.AuditReviewed = session.diagnostics.AuditReviewed
	validation.AuditRejected = session.diagnostics.AuditRejected
	if len(songs) > target {
		songs = songs[:target]
	}
	validation.Shortened = len(songs) < target
	if validation.Shortened {
		switch {
		case session.attempted >= maximumPlaylistRefillCandidates:
			validation.ShortfallReason = "audit_limit_reached"
		case searchLimitReached:
			validation.ShortfallReason = "search_limit_reached"
		default:
			validation.ShortfallReason = "matching_tracks_exhausted"
		}
	}
	return semanticPlaylistResult{
		Filter: llm.PlaylistFilter{
			Genres:       intent.PreferredGenres,
			Artists:      intent.IncludeArtists,
			MinYear:      intent.MinYear,
			MaxYear:      intent.MaxYear,
			Description:  intent.IntentSummary,
			Instrumental: intent.InstrumentalOnly,
			Source:       request.Source,
		},
		Songs: songs, SemanticScores: scores,
		Diagnostics: semanticRetrievalDiagnostics{
			Mode:         "semantic",
			SearchPasses: searchPasses, SearchLimitReached: searchLimitReached,
			CandidateCount:       len(retrieval.Candidates),
			ReturnedCount:        len(songs),
			TrackMatches:         len(retrieval.Search.Tracks),
			AlbumMatches:         len(retrieval.Search.Albums),
			ArtistMatches:        len(retrieval.Search.Artists),
			NegativeQueryApplied: negativeApplied,
			Validation:           validation,
		},
	}, true, nil
}
