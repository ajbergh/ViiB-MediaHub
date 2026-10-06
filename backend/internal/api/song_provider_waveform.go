package api

import (
	"github.com/ajbergh/viib-mediahub/internal/analysis/waveformartifact"
	"github.com/ajbergh/viib-mediahub/internal/db"
	"github.com/ajbergh/viib-mediahub/internal/spotify/waveform"
	"github.com/go-chi/chi/v5"
	"math"
	"net/http"
	"time"
)

// This song view retains native signed samples while bounding display output.
// It never uses provider timing to change local playback duration.
func (a *API) getSongProviderWaveform(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "songID")
	current, err := a.currentAnalysisSourceFingerprints([]string{id})
	if err != nil || current[id] == "" {
		respondError(w, 404, "local source unavailable")
		return
	}
	fp := current[id]
	link, err := a.db.GetSpotifyRecording(id, fp)
	if err != nil || link == nil {
		respondError(w, 404, "current recording link unavailable")
		return
	}
	serve := func(artifact *db.SpotifyAudioArtifact, pending bool, provenance string) error {
		decoded, e := waveform.DecodeDomain(artifact.Payload)
		if e != nil {
			return e
		}
		localDuration := 0.0
		if local, e := a.db.GetTrackAnalysisArtifact(id, waveformartifact.Kind, waveformartifact.FormatVersion, waveformartifact.AlgorithmVersion); e == nil && local.SourceFingerprint == fp && local.Encoding == waveformartifact.Encoding && local.Provenance == "measured" {
			if o, e := waveformartifact.Decode(local.Data); e == nil {
				localDuration = o.Duration()
			}
		}
		final, e := a.currentAnalysisSourceFingerprints([]string{id})
		if e != nil {
			return e
		}
		finalLink, e := a.db.GetSpotifyRecording(id, fp)
		if e != nil {
			return e
		}
		if final[id] != fp || finalLink == nil || finalLink.ExternalID != link.ExternalID {
			respondError(w, 412, "source or recording changed")
			return nil
		}
		stride := max(1, (len(decoded.Lows)+599)/600)
		pool := func(band []int32) []int32 {
			out := []int32{}
			for start := 0; start < len(band); start += stride {
				v := band[start]
				for _, candidate := range band[start:min(start+stride, len(band))] {
					if math.Abs(float64(candidate)) > math.Abs(float64(v)) {
						v = candidate
					}
				}
				out = append(out, v)
			}
			return out
		}
		alignment := "local_duration_unavailable"
		if localDuration > 0 {
			alignment = "duration_mismatch"
			if decoded.Aligned(localDuration) {
				alignment = "duration_compatible"
			}
		}
		w.Header().Set("Cache-Control", "private, no-store")
		respondJSON(w, map[string]any{"readOnly": true, "unverified": pending, "provenance": provenance, "songId": id, "sourceFingerprint": fp, "recordingId": link.ExternalID, "representation": "spotify_three_band", "lows": pool(decoded.Lows), "mids": pool(decoded.Mids), "highs": pool(decoded.Highs), "totalSamples": len(decoded.Lows), "sampleRate": decoded.SampleRate, "windowMilliseconds": decoded.WindowMilliseconds, "displayStride": stride, "displayAggregation": "signed_absolute_max_v1", "durationSeconds": decoded.DurationSeconds(), "localDurationSeconds": localDuration, "alignment": alignment, "retrievedAt": artifact.RetrievedAt, "stale": !time.Now().Before(artifact.ExpiresAt)})
		return nil
	}

	imported, err := a.db.GetDownloadedSpotifyAudioArtifact(r.Context(), id, fp, "three_band_waveform", "spotify_three_band")
	if err != nil {
		respondError(w, 503, "Imported waveform unavailable")
		return
	}
	if imported != nil {
		if err = serve(imported, false, "spotify_durable_import"); err != nil {
			respondError(w, 503, "Imported waveform unavailable")
		}
		return
	}
	runtime := a.spotifyTokens()
	ctx, cancel := runtime.requestContext(r.Context())
	defer cancel()
	err = runtime.withMetadataRead(ctx, func(fence db.SpotifyMetadataReadFence) error {
		var artifact *db.SpotifyAudioArtifact
		var e error
		if fence.Pending {
			artifact, e = a.db.GetSpotifyAudioArtifactForRuntime(fence, link.ExternalID, "three_band_waveform", "spotify_three_band")
		} else {
			artifact, e = a.db.GetSpotifyActiveAudioArtifactForRuntime(db.SpotifyMetadataFence{Epoch: fence.Epoch, ContextKey: fence.ContextKey}, link.ExternalID, "three_band_waveform", "spotify_three_band")
		}
		if e != nil {
			return e
		}
		if artifact == nil {
			respondError(w, 404, "Spotify waveform not cached")
			return nil
		}
		return serve(artifact, fence.Pending, "spotify_private_cache")
	})
	if err != nil {
		respondError(w, 503, "Spotify waveform unavailable")
	}
}
