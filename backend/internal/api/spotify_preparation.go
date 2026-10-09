package api

import (
	"context"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/analysis"
	"github.com/ajbergh/viib-mediahub/internal/analysis/beatgrid"
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
	link, err := a.db.GetSpotifyRecording(source.SongID, source.Fingerprint)
	if err != nil {
		return nil
	}
	// Final-file-bound Spotify evidence remains usable independently of an
	// active account. Read it before requiring a private-cache owner.
	var importedObservation *spotifyanalysis.Observation
	var importedBeatGrid *beatgrid.Grid
	providerThreeBandAvailable := false
	if link != nil {
		fields, _, importedRecording, _, importErr := a.db.GetDownloadedSpotifyScalarCandidates(ctx, source.SongID, source.Fingerprint)
		if importErr == nil && importedRecording == link.ExternalID {
			importedObservation = downloadedPreparationObservation(fields, link.ExternalID)
		}
		if imported, importErr := a.db.GetDownloadedSpotifyAudioArtifact(ctx, source.SongID, source.Fingerprint, "three_band_waveform", "spotify_three_band"); importErr == nil && imported != nil && imported.TrackID == link.ExternalID {
			providerThreeBandAvailable = true
		}
		beats, beatsErr := a.db.GetDownloadedSpotifyAudioArtifact(ctx, source.SongID, source.Fingerprint, "audio_analysis", "beats")
		bars, barsErr := a.db.GetDownloadedSpotifyAudioArtifact(ctx, source.SongID, source.Fingerprint, "audio_analysis", "bars")
		if beatsErr == nil && barsErr == nil && freshSpotifyAudioArtifact(beats, link.ExternalID, time.Now()) && freshSpotifyAudioArtifact(bars, link.ExternalID, time.Now()) {
			importedBeatGrid = spotifyBeatGridFromArtifacts(beats.Payload, bars.Payload)
		}
		if importedObservation == nil && (providerThreeBandAvailable || importedBeatGrid != nil) {
			importedObservation = &spotifyanalysis.Observation{TrackID: link.ExternalID, Source: "spotify_download_import"}
		}
		if importedObservation != nil {
			importedObservation.ProviderBeatGrid = importedBeatGrid
		}
		if importedObservation != nil || providerThreeBandAvailable {
			current, sourceErr := a.resolveAnalysisSource(ctx, source.SongID)
			currentLink, linkErr := a.db.GetSpotifyRecording(source.SongID, source.Fingerprint)
			if sourceErr != nil || linkErr != nil || current.Fingerprint != source.Fingerprint || currentLink == nil || currentLink.ExternalID != link.ExternalID {
				return nil
			}
		}
	}
	if err := runtime.ensureMetadataOwner(ctx); err != nil {
		if importedObservation != nil {
			importedObservation.ProviderThreeBandAvailable = providerThreeBandAvailable
			return importedObservation
		}
		if providerThreeBandAvailable && link != nil {
			return &spotifyanalysis.Observation{TrackID: link.ExternalID, Source: "spotify_download_import", ProviderThreeBandAvailable: true}
		}
		return nil
	}
	a.initSpotifyAnalysis()
	a.spotifyAnalysisMu.RLock()
	service := a.spotifyAnalysis
	a.spotifyAnalysisMu.RUnlock()
	if service == nil {
		if importedObservation != nil {
			importedObservation.ProviderThreeBandAvailable = providerThreeBandAvailable
			return importedObservation
		}
		if providerThreeBandAvailable && link != nil {
			return &spotifyanalysis.Observation{TrackID: link.ExternalID, Source: "spotify_download_import", ProviderThreeBandAvailable: true}
		}
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
		if providerThreeBandAvailable {
			return
		}
		status, _, err := a.spotifyWaveformForRecording(ctx, link.ExternalID)
		if err != nil {
			logger.Scan("spotify_resource spotify_id=%q resource=three_band_waveform status=unavailable", link.ExternalID)
			return
		}
		state, _ := status["state"].(string)
		lastGood, _ := status["lastGoodAvailable"].(bool)
		// Expiry triggers a refresh attempt; it does not invalidate a decoded,
		// source-linked last-good waveform for the same Spotify recording. Keep
		// it as the single three-band representation when refresh is unavailable.
		providerThreeBandAvailable = state == "available" || lastGood
	}()
	workers.Wait()
	if ctx.Err() != nil {
		return nil
	}
	var privateBeatGrid *beatgrid.Grid
	_ = runtime.withMetadataRead(ctx, func(fence db.SpotifyMetadataReadFence) error {
		readArtifact := func(kind string) *db.SpotifyAudioArtifact {
			var artifact *db.SpotifyAudioArtifact
			var readErr error
			if fence.Pending {
				artifact, readErr = a.db.GetSpotifyAudioArtifactForRuntime(fence, link.ExternalID, "audio_analysis", kind)
			} else {
				artifact, readErr = a.db.GetSpotifyActiveAudioArtifactForRuntime(db.SpotifyMetadataFence{Epoch: fence.Epoch, ContextKey: fence.ContextKey}, link.ExternalID, "audio_analysis", kind)
			}
			if readErr != nil || !freshSpotifyAudioArtifact(artifact, link.ExternalID, time.Now()) {
				return nil
			}
			return artifact
		}
		beats, bars := readArtifact("beats"), readArtifact("bars")
		if beats != nil && bars != nil {
			privateBeatGrid = spotifyBeatGridFromArtifacts(beats.Payload, bars.Payload)
		}
		return nil
	})
	if privateBeatGrid == nil {
		privateBeatGrid = importedBeatGrid
	}
	current, err := a.resolveAnalysisSource(ctx, source.SongID)
	if err != nil || current.Fingerprint != source.Fingerprint {
		return nil
	}
	currentLink, err := a.db.GetSpotifyRecording(source.SongID, source.Fingerprint)
	if err != nil || currentLink == nil || currentLink.ExternalID != link.ExternalID {
		return nil
	}
	if detailed != nil {
		detailed.ProviderBeatGrid = privateBeatGrid
	}
	observation := mergePreparationObservations(spotifyanalysis.PreparationScalars(features, detailed), importedObservation)
	if observation == nil && !providerThreeBandAvailable && privateBeatGrid == nil {
		return nil
	}
	if observation == nil {
		observation = &spotifyanalysis.Observation{TrackID: link.ExternalID, Source: "spotify_preparation"}
	}
	observation.ProviderThreeBandAvailable = providerThreeBandAvailable
	if observation.ProviderBeatGrid == nil {
		observation.ProviderBeatGrid = privateBeatGrid
	}
	return observation
}

func freshSpotifyAudioArtifact(artifact *db.SpotifyAudioArtifact, recordingID string, now time.Time) bool {
	return artifact != nil && artifact.TrackID == recordingID && artifact.Resource == "audio_analysis" && artifact.RetrievedAt.IsZero() == false && artifact.ExpiresAt.After(artifact.RetrievedAt) && now.Before(artifact.ExpiresAt)
}

func spotifyBeatGridFromArtifacts(beatsPayload, barsPayload []byte) *beatgrid.Grid {
	type interval struct {
		Start *float64 `json:"start"`
	}
	var beats struct {
		Values []interval `json:"beats"`
	}
	var bars struct {
		Values []interval `json:"bars"`
	}
	if json.Unmarshal(beatsPayload, &beats) != nil || json.Unmarshal(barsPayload, &bars) != nil || len(beats.Values) < 2 || len(bars.Values) == 0 || len(beats.Values) > 200000 || len(bars.Values) > 200000 {
		return nil
	}
	grid := beatgrid.Grid{Beats: make([]float64, 0, len(beats.Values)), Provenance: beatgrid.ProvenanceSpotify}
	previous := -1.0
	for _, value := range beats.Values {
		if value.Start == nil || math.IsNaN(*value.Start) || math.IsInf(*value.Start, 0) || *value.Start < 0 || *value.Start <= previous {
			return nil
		}
		previous = *value.Start
		grid.Beats = append(grid.Beats, *value.Start)
	}
	previous = -1
	for _, bar := range bars.Values {
		if bar.Start == nil || math.IsNaN(*bar.Start) || math.IsInf(*bar.Start, 0) || *bar.Start < 0 || *bar.Start <= previous {
			return nil
		}
		previous = *bar.Start
		index := sort.SearchFloat64s(grid.Beats, *bar.Start)
		if index == len(grid.Beats) || math.Abs(grid.Beats[index]-*bar.Start) > 0.001 {
			if index == 0 || math.Abs(grid.Beats[index-1]-*bar.Start) > 0.001 {
				return nil
			}
			index--
		}
		if len(grid.DownbeatIndices) > 0 && index <= grid.DownbeatIndices[len(grid.DownbeatIndices)-1] {
			return nil
		}
		grid.DownbeatIndices = append(grid.DownbeatIndices, index)
	}
	if err := grid.Validate(); err != nil {
		return nil
	}
	if _, err := grid.Encode(); err != nil {
		return nil
	}
	return &grid
}

func downloadedPreparationObservation(fields []db.SpotifyScalarField, recordingID string) *spotifyanalysis.Observation {
	if recordingID == "" {
		return nil
	}
	result := &spotifyanalysis.Observation{TrackID: recordingID, Source: "spotify_download_import", DurableImport: true}
	for _, field := range db.SelectProviderScalarFields(fields) {
		if !field.DurableImport || field.Stale {
			continue
		}
		switch field.Key {
		case "tempo_bpm":
			var value float64
			if json.Unmarshal(field.Value, &value) == nil && value > 0 {
				result.BPM = &value
				result.BPMConfidence = field.Confidence
				origin := &spotifyanalysis.Observation{TrackID: recordingID, Source: "spotify_download_import", SourceEndpoint: field.Endpoint, RetrievedAt: field.RetrievedAt, DurableImport: true}
				result.BPMOrigin = origin
			}
		case "key_mode":
			var value struct {
				Tonic          int      `json:"tonic"`
				Mode           int      `json:"mode"`
				ModeConfidence *float64 `json:"modeConfidence"`
			}
			if json.Unmarshal(field.Value, &value) == nil && value.Tonic >= 0 && value.Tonic <= 11 && (value.Mode == 0 || value.Mode == 1) {
				result.Key, result.Mode = &value.Tonic, &value.Mode
				result.KeyConfidence, result.ModeConfidence = field.Confidence, value.ModeConfidence
				origin := &spotifyanalysis.Observation{TrackID: recordingID, Source: "spotify_download_import", SourceEndpoint: field.Endpoint, RetrievedAt: field.RetrievedAt, DurableImport: true}
				result.KeyOrigin = origin
			}
		}
	}
	if result.BPM == nil && (result.Key == nil || result.Mode == nil) {
		return nil
	}
	return result
}

func mergePreparationObservations(primary, fallback *spotifyanalysis.Observation) *spotifyanalysis.Observation {
	if primary == nil {
		return fallback
	}
	if fallback == nil || primary.TrackID != fallback.TrackID {
		return primary
	}
	if primary.BPM == nil && fallback.BPM != nil {
		primary.BPM, primary.BPMConfidence, primary.BPMOrigin = fallback.BPM, fallback.BPMConfidence, fallback.BPMOrigin
		primary.BPMRetained = fallback.BPMRetained
	}
	if (primary.Key == nil || primary.Mode == nil) && fallback.Key != nil && fallback.Mode != nil {
		primary.Key, primary.Mode, primary.Camelot = fallback.Key, fallback.Mode, fallback.Camelot
		primary.KeyConfidence, primary.ModeConfidence, primary.KeyOrigin = fallback.KeyConfidence, fallback.ModeConfidence, fallback.KeyOrigin
		primary.KeyRetained = fallback.KeyRetained
	}
	primary.ProviderThreeBandAvailable = fallback.ProviderThreeBandAvailable
	if primary.ProviderBeatGrid == nil {
		primary.ProviderBeatGrid = fallback.ProviderBeatGrid
	}
	return primary
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
