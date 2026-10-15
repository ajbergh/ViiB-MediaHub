package api

import (
	"context"

	"github.com/ajbergh/viib-mediahub/internal/db"
	"github.com/ajbergh/viib-mediahub/internal/llm"
)

// Keep cloud audit work finite even for requests that reject almost everything.
const maximumPlaylistRefillCandidates = 1000

type playlistAuditor func(context.Context, string, llm.PlaylistIntent, []db.Song) ([]db.Song, playlistValidationDiagnostics)

// Request-scoped decisions survive reranking and wider searches. Each catalog
// ID is offered to the auditor at most once, including IDs accepted on fallback.
type playlistAuditSession struct {
	decisions   map[string]bool
	rejected    map[string]bool
	attempted   int
	diagnostics playlistValidationDiagnostics
}

func newPlaylistAuditSession() *playlistAuditSession {
	return &playlistAuditSession{decisions: make(map[string]bool), rejected: make(map[string]bool), diagnostics: playlistValidationDiagnostics{AuditStatus: "not_needed"}}
}

func (session *playlistAuditSession) filter(ctx context.Context, prompt string, intent llm.PlaylistIntent, songs []db.Song, audit playlistAuditor) []db.Song {
	session.diagnostics.RequiredStyles = intent.RequiredStyles
	session.diagnostics.ExcludedTerms = intent.ExcludedTerms
	unknown := make([]db.Song, 0, len(songs))
	seen := make(map[string]bool)
	for _, song := range songs {
		if _, known := session.decisions[song.ID]; !known && !seen[song.ID] && len(unknown)+session.attempted < maximumPlaylistRefillCandidates {
			unknown = append(unknown, song)
			seen[song.ID] = true
		}
	}
	for start := 0; start < len(unknown); start += maximumPlaylistAuditCandidates {
		if ctx.Err() != nil {
			break
		}
		batch := unknown[start:min(start+maximumPlaylistAuditCandidates, len(unknown))]
		accepted, diagnostics := audit(ctx, prompt, intent, batch)
		allowed := make(map[string]bool, len(accepted))
		for _, song := range accepted {
			allowed[song.ID] = true
		}
		for _, song := range batch {
			session.decisions[song.ID] = allowed[song.ID]
			if !allowed[song.ID] {
				session.rejected[song.ID] = true
			}
		}
		session.attempted += len(batch)
		session.diagnostics.AuditReviewed += diagnostics.AuditReviewed
		session.diagnostics.AuditRejected = len(session.rejected)
		// Preserve a failure/unavailable status across later successful batches.
		if session.diagnostics.AuditStatus != "failed" && (session.diagnostics.AuditStatus != "unavailable" || diagnostics.AuditStatus == "failed") {
			session.diagnostics.AuditStatus = diagnostics.AuditStatus
		}
	}
	result := make([]db.Song, 0, len(songs))
	seen = make(map[string]bool)
	for _, song := range songs {
		if session.decisions[song.ID] && !seen[song.ID] {
			result = append(result, song)
			seen[song.ID] = true
		}
	}
	return result
}
