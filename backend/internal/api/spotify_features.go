package api

import (
	"context"
	"github.com/ajbergh/viib-mediahub/internal/analysis"
	spotifyanalysis "github.com/ajbergh/viib-mediahub/internal/spotify/analysis"
	spotifyrefresh "github.com/ajbergh/viib-mediahub/internal/spotify/refresh"
	"time"
)

// spotifyTrackFeatures shares the session-bound cache, cooldown and request
// coalescing used by the reference editor. Lookup failure is a local fallback.
func (a *API) spotifyTrackFeatures(ctx context.Context, id string) *spotifyanalysis.Observation {
	a.spotifyAnalysisMu.RLock()
	service := a.spotifyAnalysis
	a.spotifyAnalysisMu.RUnlock()
	if service == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	result, err := service.Refresh(ctx, id, spotifyrefresh.Features)
	if err != nil || result.Failure != nil || result.Cache == nil || !time.Now().Before(result.Cache.ExpiresAt) {
		return nil
	}
	return &result.Cache.Observation
}

func (a *API) spotifyFeaturesForSource(ctx context.Context, source analysis.ResolvedSource) *spotifyanalysis.Observation {
	link, err := a.db.GetSpotifyRecording(source.SongID, source.Fingerprint)
	if err != nil {
		return nil
	}
	if link == nil {
		link = a.matchSpotifyRecording(ctx, source)
	}
	if link == nil {
		return nil
	}
	observation := a.spotifyTrackFeatures(ctx, link.ExternalID)
	if observation == nil || ctx.Err() != nil {
		return nil
	}
	current, err := a.resolveAnalysisSource(ctx, source.SongID)
	if err != nil || current.Fingerprint != source.Fingerprint {
		return nil
	}
	return observation
}
