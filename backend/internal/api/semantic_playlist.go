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
	NegativeQueryApplied bool                          `json:"negativeQueryApplied"`
	Validation           playlistValidationDiagnostics `json:"validation"`
}

type semanticPlaylistResult struct {
	Filter      llm.PlaylistFilter
	Songs       []db.Song
	Diagnostics semanticRetrievalDiagnostics
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
	service := a.currentSemanticService()
	if service == nil {
		return semanticPlaylistResult{}, false, nil
	}
	if request.PromptMinYear > 0 || request.PromptMaxYear > 0 {
		intent.MinYear = request.PromptMinYear
		intent.MaxYear = request.PromptMaxYear
		intent.YearConstraintHard = true
	}
	retrieval, err := service.RetrieveSemanticCandidates(ctx, intent.SemanticQuery, semantic.SemanticRetrievalOptions{
		Source:                request.Source,
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
	ranked := semantic.RankSemanticCandidates(retrieval.Candidates, semantic.HybridRankingOptions{
		DiscoverMode:      request.DiscoverMode,
		RecentlyPlayedIDs: request.RecentlyPlayedIDs,
		OnePerArtist:      request.OnePerArtist,
		Limit:             semanticPlaylistRankingLimit,
		PreferredGenres:   intent.PreferredGenres,
		MinYear:           intent.MinYear,
		MaxYear:           intent.MaxYear,
	})
	validation := playlistValidationDiagnostics{
		RequiredStyles:   intent.RequiredStyles,
		ExcludedTerms:    intent.ExcludedTerms,
		HardExcluded:     retrieval.Diagnostics.HardExcluded,
		StyleMismatches:  retrieval.Diagnostics.StyleMismatches,
		NegativeRejected: retrieval.Diagnostics.NegativeRejected,
		RankingFiltered:  max(0, len(retrieval.Candidates)-len(ranked)),
	}
	selected, err := service.ApplyMMRDiversity(ctx, ranked, semantic.DiversityOptions{
		DiscoverMode: request.DiscoverMode,
		Limit:        auditReserveLimit(request.TargetSongs),
	})
	if err != nil {
		return semanticPlaylistResult{}, false, fmt.Errorf("semantic diversity ranking: %w", err)
	}
	songs := make([]db.Song, len(selected))
	negativeApplied := false
	for index, candidate := range selected {
		songs[index] = candidate.Candidate.Song
		negativeApplied = negativeApplied || candidate.Candidate.Evidence.NegativeApplied
	}
	auditedSongs, auditDiagnostics := a.auditPlaylistSongs(ctx, request.Prompt, intent, songs)
	validation.AuditStatus = auditDiagnostics.AuditStatus
	validation.AuditReviewed = auditDiagnostics.AuditReviewed
	validation.AuditRejected = auditDiagnostics.AuditRejected
	if request.TargetSongs > 0 && len(auditedSongs) > request.TargetSongs {
		auditedSongs = auditedSongs[:request.TargetSongs]
	}
	validation.Shortened = request.TargetSongs > 0 && len(auditedSongs) < request.TargetSongs
	songs = auditedSongs
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
		Songs: songs,
		Diagnostics: semanticRetrievalDiagnostics{
			Mode:                 "semantic",
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
