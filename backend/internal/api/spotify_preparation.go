package api

import (
	"context"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/analysis"
	"github.com/ajbergh/viib-mediahub/internal/db"
	"github.com/ajbergh/viib-mediahub/internal/logger"
	spotifyanalysis "github.com/ajbergh/viib-mediahub/internal/spotify/analysis"
	spotifyrefresh "github.com/ajbergh/viib-mediahub/internal/spotify/refresh"
)

// Preparation resolves identity once and enriches independent recording-scoped
// resources. Features have precedence in the compatibility scalar path;
// detailed alternatives and native waveform remain separate cached resources.
func (a *API) spotifyPreparationForSource(parent context.Context, source analysis.ResolvedSource) *spotifyanalysis.Observation {
	runtime := a.spotifyTokens()
	account, cancelAccount := runtime.requestContext(parent)
	defer cancelAccount()
	ctx, cancel := context.WithTimeout(spotifyrefresh.BackgroundContext(account), 45*time.Second)
	defer cancel()
	if err := runtime.ensureMetadataOwner(ctx); err != nil {
		return nil
	}
	a.initSpotifyAnalysis()
	a.spotifyAnalysisMu.RLock()
	service := a.spotifyAnalysis
	a.spotifyAnalysisMu.RUnlock()
	if service == nil {
		return nil
	}
	link, err := a.db.GetSpotifyRecording(source.SongID, source.Fingerprint)
	if err != nil {
		return nil
	}
	if link == nil {
		link = a.matchSpotifyRecording(ctx, source)
	}
	if link == nil || ctx.Err() != nil {
		return nil
	}
	var features, detailed *spotifyanalysis.Observation
	var workers sync.WaitGroup
	workers.Add(3)
	go func() {
		defer workers.Done()
		features, _, _ = a.spotifyTrackFeaturesResult(ctx, link.ExternalID)
	}()
	go func() {
		defer workers.Done()
		result, err := service.Refresh(ctx, link.ExternalID, spotifyrefresh.Detailed)
		if err == nil && result.Failure == nil && result.Cache != nil && time.Now().Before(result.Cache.ExpiresAt) {
			detailed = &result.Cache.Observation
		}
		if err != nil {
			logger.Scan("spotify_resource spotify_id=%q resource=audio_analysis status=unavailable", link.ExternalID)
		}
	}()
	go func() {
		defer workers.Done()
		_, _, err := a.spotifyWaveformForRecording(ctx, link.ExternalID)
		if err != nil {
			logger.Scan("spotify_resource spotify_id=%q resource=three_band_waveform status=unavailable", link.ExternalID)
		}
	}()
	workers.Wait()
	if ctx.Err() != nil {
		return nil
	}
	current, err := a.resolveAnalysisSource(ctx, source.SongID)
	if err != nil || current.Fingerprint != source.Fingerprint {
		return nil
	}
	currentLink, err := a.db.GetSpotifyRecording(source.SongID, source.Fingerprint)
	if err != nil || currentLink == nil || currentLink.ExternalID != link.ExternalID {
		return nil
	}
	return spotifyanalysis.PreparationScalars(features, detailed)
}

// spotifyDownloadFeatures warms independent audio resources before final-file
// promotion, returning only scalar features for existing file-tag compatibility.
func (a *API) spotifyDownloadFeatures(parent context.Context, id string) *spotifyanalysis.Observation {
	runtime := a.spotifyTokens()
	account, stop := runtime.requestContext(parent)
	defer stop()
	ctx, cancel := context.WithTimeout(spotifyrefresh.BackgroundContext(account), 45*time.Second)
	defer cancel()
	if err := runtime.ensureMetadataOwner(ctx); err != nil {
		return nil
	}
	a.initSpotifyAnalysis()
	a.spotifyAnalysisMu.RLock()
	service := a.spotifyAnalysis
	a.spotifyAnalysisMu.RUnlock()
	if service == nil {
		return nil
	}
	var features *spotifyanalysis.Observation
	var workers sync.WaitGroup
	workers.Add(4)
	go func() { defer workers.Done(); features, _, _ = a.spotifyTrackFeaturesResult(ctx, id) }()
	go func() { defer workers.Done(); _, _ = service.Refresh(ctx, id, spotifyrefresh.Detailed) }()
	go func() { defer workers.Done(); _, _, _ = a.spotifyWaveformForRecording(ctx, id) }()
	go func() {
		defer workers.Done()
		fresh := false
		_ = runtime.withMetadataRead(ctx, func(fence db.SpotifyMetadataReadFence) error {
			if fence.Pending {
				return nil
			}
			var err error
			fresh, err = a.db.HasFreshSpotifyTrackCatalogForRuntime(db.SpotifyMetadataFence{Epoch: fence.Epoch, ContextKey: fence.ContextKey}, id, time.Now())
			return err
		})
		if fresh {
			return
		}
		// Existing OAuth/Web Player adapters capture bounded sanitized full domain
		// snapshots before returning their presentation response.
		response, err := a.doSpotifyRequest(ctx, http.MethodGet, "https://api.spotify.com/v1/tracks/"+id, nil, "")
		if err != nil || response == nil {
			return
		}
		defer response.Body.Close()
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 2<<20))
	}()
	workers.Wait()
	if parent.Err() != nil {
		return nil
	}
	return features
}
