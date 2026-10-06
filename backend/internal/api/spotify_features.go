// Defines spotify features functionality for package api.

package api

import (
	"context"
	"errors"
	"github.com/ajbergh/viib-mediahub/internal/analysis"
	"github.com/ajbergh/viib-mediahub/internal/logger"
	spotifyanalysis "github.com/ajbergh/viib-mediahub/internal/spotify/analysis"
	spotifyrefresh "github.com/ajbergh/viib-mediahub/internal/spotify/refresh"
	"time"
)

// spotifyTrackFeatures shares the session-bound cache, cooldown and request
// coalescing used by the reference editor. Lookup failure is a local fallback.
func (a *API) spotifyTrackFeatures(ctx context.Context, id string) *spotifyanalysis.Observation {
	observation, _, _ := a.spotifyTrackFeaturesResult(ctx, id)
	return observation
}

func (a *API) spotifyTrackFeaturesResult(ctx context.Context, id string) (*spotifyanalysis.Observation, string, bool) {
	a.spotifyAnalysisMu.RLock()
	service := a.spotifyAnalysis
	a.spotifyAnalysisMu.RUnlock()
	if service == nil {
		runtime := a.spotifyTokens()
		account, stop := runtime.requestContext(ctx)
		defer stop()
		budget, cancel := context.WithTimeout(account, 25*time.Second)
		defer cancel()
		if err := runtime.ensureMetadataOwner(budget); err != nil {
			return nil, "spotify_owner_unavailable", false
		}
		a.initSpotifyAnalysis()
		a.spotifyAnalysisMu.RLock()
		service = a.spotifyAnalysis
		a.spotifyAnalysisMu.RUnlock()
		if service == nil {
			return nil, "spotify_disabled", false
		}
		ctx = budget
	}
	ctx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	result, err := service.Refresh(ctx, id, spotifyrefresh.Features)
	if err != nil {
		reason := "spotify_features_error"
		var providerError *spotifyanalysis.Error
		if errors.As(err, &providerError) {
			reason = "spotify_" + string(providerError.Code)
		}
		if ctx.Err() != nil {
			reason = "spotify_lookup_canceled_or_timed_out"
		}
		return nil, reason, false
	}
	if result.Failure != nil {
		return nil, "spotify_" + string(result.Failure.Code), result.FromCache
	}
	if result.Cache == nil {
		return nil, "spotify_features_unavailable", result.FromCache
	}
	if !time.Now().Before(result.Cache.ExpiresAt) {
		return nil, "spotify_features_expired", result.FromCache
	}
	return &result.Cache.Observation, "spotify_available", result.FromCache
}

func (a *API) spotifyFeaturesForSource(ctx context.Context, source analysis.ResolvedSource) *spotifyanalysis.Observation {
	link, err := a.db.GetSpotifyRecording(source.SongID, source.Fingerprint)
	if err != nil {
		logger.Scan("spotify_lookup song_id=%q reason=recording_identity_error", source.SongID)
		return nil
	}
	if link == nil {
		link = a.matchSpotifyRecording(ctx, source)
	}
	if link == nil {
		return nil
	}
	observation, reason, cached := a.spotifyTrackFeaturesResult(ctx, link.ExternalID)
	logger.Scan("spotify_lookup song_id=%q spotify_id=%q link_origin=%q reason=%q from_cache=%t", source.SongID, link.ExternalID, link.LinkOrigin, reason, cached)
	if observation == nil || ctx.Err() != nil {
		return nil
	}
	current, err := a.resolveAnalysisSource(ctx, source.SongID)
	if err != nil || current.Fingerprint != source.Fingerprint {
		logger.Scan("spotify_lookup song_id=%q reason=source_changed_or_unavailable", source.SongID)
		return nil
	}
	return observation
}
