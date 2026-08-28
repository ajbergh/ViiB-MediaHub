package api

import (
	"context"
	"fmt"

	"github.com/ajbergh/viib-mediahub/internal/db"
	"github.com/ajbergh/viib-mediahub/internal/dj"
	"github.com/ajbergh/viib-mediahub/internal/llm"
	"github.com/ajbergh/viib-mediahub/internal/logger"
	"github.com/ajbergh/viib-mediahub/internal/semantic"
)

const maximumPlaylistAuditCandidates = 100

type playlistValidationDiagnostics struct {
	RequiredStyles   []string `json:"requiredStyles,omitempty"`
	ExcludedTerms    []string `json:"excludedTerms,omitempty"`
	HardExcluded     int      `json:"hardExcluded"`
	StyleMismatches  int      `json:"styleMismatches"`
	NegativeRejected int      `json:"negativeRejected"`
	RankingFiltered  int      `json:"rankingFiltered"`
	AuditStatus      string   `json:"auditStatus"`
	AuditReviewed    int      `json:"auditReviewed"`
	AuditRejected    int      `json:"auditRejected"`
	Shortened        bool     `json:"shortened"`
}

func (a *API) auditPlaylistSongs(ctx context.Context, prompt string, intent llm.PlaylistIntent, songs []db.Song) ([]db.Song, playlistValidationDiagnostics) {
	diagnostics := playlistValidationDiagnostics{
		RequiredStyles: intent.RequiredStyles,
		ExcludedTerms:  intent.ExcludedTerms,
		AuditStatus:    "not_needed",
	}
	if len(songs) == 0 {
		return songs, diagnostics
	}
	provider, err := llm.GetConfiguredProvider(a.db)
	if err != nil {
		diagnostics.AuditStatus = "unavailable"
		logger.API("AI DJ Audit: unavailable; deterministic constraints remain active: %v", err)
		return songs, diagnostics
	}
	defer provider.Close()
	rejected := make(map[string]struct{})
	for start := 0; start < len(songs); start += maximumPlaylistAuditCandidates {
		end := min(len(songs), start+maximumPlaylistAuditCandidates)
		result, auditErr := provider.AuditPlaylistCandidates(ctx, prompt, intent, songs[start:end])
		if auditErr != nil {
			diagnostics.AuditStatus = "failed"
			logger.API("AI DJ Audit: failed; deterministic constraints remain active: %v", auditErr)
			return songs, diagnostics
		}
		diagnostics.AuditReviewed += end - start
		for _, rejection := range result.Rejected {
			rejected[rejection.ID] = struct{}{}
		}
	}
	diagnostics.AuditStatus = "passed"
	diagnostics.AuditRejected = len(rejected)
	accepted := make([]db.Song, 0, len(songs)-len(rejected))
	for _, song := range songs {
		if _, exists := rejected[song.ID]; !exists {
			accepted = append(accepted, song)
		}
	}
	return accepted, diagnostics
}

func (a *API) validateLegacyPlaylistCandidates(ctx context.Context, prompt string, intent llm.PlaylistIntent, values []any, target int) ([]any, playlistValidationDiagnostics, error) {
	songs := make([]db.Song, 0, len(values))
	for _, value := range values {
		switch song := value.(type) {
		case db.Song:
			songs = append(songs, song)
		case *db.Song:
			if song != nil {
				songs = append(songs, *song)
			}
		default:
			return nil, playlistValidationDiagnostics{}, fmt.Errorf("unexpected AI DJ song type %T", value)
		}
	}
	filtered, local := semantic.FilterSongsByConstraints(songs, semantic.SemanticRetrievalOptions{
		IncludeArtists:     intent.IncludeArtists,
		ExcludeArtists:     intent.ExcludeArtists,
		MinYear:            intent.MinYear,
		MaxYear:            intent.MaxYear,
		YearConstraintHard: intent.YearConstraintHard,
		InstrumentalOnly:   intent.InstrumentalOnly,
		RequiredStyles:     intent.RequiredStyles,
		ExcludedTerms:      intent.ExcludedTerms,
	})
	accepted, diagnostics := a.auditPlaylistSongs(ctx, prompt, intent, filtered)
	diagnostics.HardExcluded = local.HardExcluded
	diagnostics.StyleMismatches = local.StyleMismatches
	if target > 0 && len(accepted) > target {
		accepted = accepted[:target]
	}
	diagnostics.Shortened = target > 0 && len(accepted) < target
	result := make([]any, len(accepted))
	for index, song := range accepted {
		result[index] = song
	}
	return result, diagnostics, nil
}

func auditReserveLimit(target int) int {
	if target <= 0 {
		return 50
	}
	return min(maximumPlaylistAuditCandidates, max(target*2, target+12))
}

func reconcileDJPhaseResults(queue []db.Song, phases []dj.PhaseResult) []dj.PhaseResult {
	byID := make(map[string]db.Song, len(queue))
	for _, song := range queue {
		byID[song.ID] = song
	}
	result := make([]dj.PhaseResult, len(phases))
	for index, phase := range phases {
		result[index] = phase
		ids := make([]string, 0, len(phase.SongIDs))
		totalBPM, bpmCount, minimumBPM, maximumBPM := 0, 0, 0, 0
		for _, id := range phase.SongIDs {
			song, exists := byID[id]
			if !exists {
				continue
			}
			ids = append(ids, id)
			if song.BPM > 0 {
				totalBPM += song.BPM
				bpmCount++
				if minimumBPM == 0 || song.BPM < minimumBPM {
					minimumBPM = song.BPM
				}
				if song.BPM > maximumBPM {
					maximumBPM = song.BPM
				}
			}
		}
		result[index].SongIDs = ids
		result[index].SongCount = len(ids)
		result[index].MinBPM = minimumBPM
		result[index].MaxBPM = maximumBPM
		result[index].AvgBPM = 0
		if bpmCount > 0 {
			result[index].AvgBPM = totalBPM / bpmCount
		}
	}
	return result
}
