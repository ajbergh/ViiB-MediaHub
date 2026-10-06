// Serves explicit recording links and optional Spotify reference cache, status, and refresh operations.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/ajbergh/viib-mediahub/internal/db"
	"github.com/ajbergh/viib-mediahub/internal/logger"
	spotifyanalysis "github.com/ajbergh/viib-mediahub/internal/spotify/analysis"
	spotifyauth "github.com/ajbergh/viib-mediahub/internal/spotify/auth"
	spotifyrefresh "github.com/ajbergh/viib-mediahub/internal/spotify/refresh"
	"github.com/ajbergh/viib-mediahub/internal/spotify/waveform"
	"github.com/go-chi/chi/v5"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type spotifyCacheResponse struct {
	ReadOnly    bool                       `json:"readOnly"`
	Unverified  bool                       `json:"unverified"`
	Provenance  string                     `json:"provenance"`
	State       string                     `json:"state"`
	Endpoint    string                     `json:"endpoint"`
	Stale       bool                       `json:"stale"`
	AgeSeconds  *int64                     `json:"ageSeconds"`
	Cache       *db.ExternalAnalysisCache  `json:"cache"`
	LastFailure *db.ExternalAnalysisStatus `json:"lastFailure"`
}

func spotifyCacheEndpoint(r *http.Request) (string, bool) {
	endpoint := r.URL.Query().Get("endpoint")
	if endpoint == "" {
		endpoint = "audio_features"
	}
	return endpoint, endpoint == "audio_features" || endpoint == "audio_analysis"
}
func (a *API) getSpotifyAnalysisStatus(w http.ResponseWriter, r *http.Request) {
	a.spotifyAuthMu.Lock()
	runtime := a.spotifyAuth
	a.spotifyAuthMu.Unlock()
	if runtime != nil {
		ctx, cancel := runtime.requestContext(r.Context())
		defer cancel()
		pending := false
		err := runtime.withMetadataRead(ctx, func(fence db.SpotifyMetadataReadFence) error {
			if !fence.Pending {
				return nil
			}
			pending = true
			if err := a.db.ValidateSpotifyMetadataRead(fence); err != nil {
				return err
			}
			w.Header().Set("Cache-Control", "private, no-store")
			respondJSON(w, map[string]any{"state": "owner_confirmation_pending", "configured": true, "connected": false, "cacheOnly": true, "readOnly": true, "unverified": true, "provenance": "spotify_private_cache"})
			return nil
		})
		if pending {
			if err != nil {
				respondError(w, 503, "Spotify retained metadata unavailable")
			}
			return
		}
	}
	a.spotifyAnalysisMu.RLock()
	service := a.spotifyAnalysis
	a.spotifyAnalysisMu.RUnlock()
	if service != nil {
		respondJSON(w, service.Status())
		return
	}
	respondJSON(w, map[string]interface{}{"state": "disabled", "configured": false, "connected": false, "cacheOnly": true})
}
func (a *API) getSpotifyAnalysisCache(w http.ResponseWriter, r *http.Request) {
	endpoint, valid := spotifyCacheEndpoint(r)
	id := chi.URLParam(r, "trackID")
	if !valid || !db.ValidSpotifyRecordingID(id) {
		respondError(w, 400, "Invalid Spotify recording or endpoint")
		return
	}
	serve := func(fence *db.SpotifyMetadataReadFence) error {
		var cache *db.ExternalAnalysisCache
		var failure *db.ExternalAnalysisStatus
		var err error
		if fence != nil {
			cache, err = a.db.GetExternalAnalysisForRuntime(*fence, id, endpoint)
		} else {
			cache, err = a.db.GetExternalAnalysis(id, endpoint)
		}
		if err != nil {
			return err
		}
		if fence != nil {
			failure, err = a.db.GetExternalAnalysisStatusForRuntime(*fence, id, endpoint)
		} else {
			failure, err = a.db.GetExternalAnalysisStatus(id, endpoint)
		}
		if err != nil {
			return err
		}
		result := spotifyCacheResponse{ReadOnly: true, Provenance: "spotify_private_cache", State: "not_cached", Endpoint: endpoint, Cache: cache, LastFailure: failure}
		if fence != nil {
			result.Unverified = fence.Pending
		}
		now := time.Now()
		if cache != nil {
			result.State = "available"
			result.Stale = !now.Before(cache.ExpiresAt)
			age := int64(now.Sub(cache.Observation.RetrievedAt).Seconds())
			if age < 0 {
				age = 0
			}
			result.AgeSeconds = &age
			if failure != nil && !failure.CheckedAt.After(cache.Observation.RetrievedAt) {
				result.LastFailure = nil
			}
		}
		if cache == nil && failure != nil {
			result.State = string(failure.Code)
		}
		w.Header().Set("Cache-Control", "private, no-store")
		respondJSON(w, result)
		return nil
	}
	a.spotifyAuthMu.Lock()
	runtime := a.spotifyAuth
	a.spotifyAuthMu.Unlock()
	var err error
	if runtime == nil {
		err = serve(nil)
	} else {
		ctx, cancel := runtime.requestContext(r.Context())
		defer cancel()
		err = runtime.withMetadataRead(ctx, func(fence db.SpotifyMetadataReadFence) error { return serve(&fence) })
	}
	if err != nil {
		respondError(w, 503, "Spotify reference cache unavailable")
	}
}
func (a *API) getSpotifyRecordingLink(w http.ResponseWriter, r *http.Request) {
	songID := chi.URLParam(r, "songID")
	song, err := a.db.GetSongByID(songID)
	if err != nil {
		respondError(w, 500, "Cannot read song")
		return
	}
	if song == nil {
		respondError(w, 404, "Song not found")
		return
	}
	fingerprints, err := a.currentAnalysisSourceFingerprints([]string{songID})
	if err != nil {
		respondError(w, 500, "Cannot resolve song source")
		return
	}
	fingerprint := fingerprints[songID]
	link, err := a.db.GetSpotifyRecording(songID, fingerprint)
	if err != nil {
		respondError(w, 500, "Cannot read recording identity")
		return
	}
	respondJSON(w, map[string]interface{}{"songId": songID, "sourceFingerprint": fingerprint, "link": link})
}
func (a *API) putSpotifyRecordingLink(w http.ResponseWriter, r *http.Request) {
	var body struct {
		TrackID           string `json:"trackId"`
		SourceFingerprint string `json:"sourceFingerprint"`
		Confirmed         bool   `json:"confirmed"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&body); err != nil {
		respondError(w, 400, "Invalid recording confirmation")
		return
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF || !body.Confirmed ||
		!db.ValidSpotifyRecordingID(body.TrackID) || body.SourceFingerprint == "" {
		respondError(w, 400, "Explicit recording confirmation and source fingerprint required")
		return
	}
	songID := chi.URLParam(r, "songID")
	song, err := a.db.GetSongByID(songID)
	if err != nil {
		respondError(w, 500, "Cannot read song")
		return
	}
	if song == nil {
		respondError(w, 404, "Song not found")
		return
	}
	fingerprints, err := a.currentAnalysisSourceFingerprints([]string{songID})
	if err != nil {
		respondError(w, 500, "Cannot resolve song source")
		return
	}
	fingerprint := fingerprints[songID]
	if fingerprint == "" || fingerprint != body.SourceFingerprint {
		respondError(w, 409, "Source changed or unavailable; reconfirm recording")
		return
	}
	if err = a.db.RefreshTrackAnalysisSourceRevision(songID, fingerprint); err != nil {
		respondError(w, 500, "Cannot record source revision")
		return
	}
	applied, err := a.db.ConfirmSpotifyRecording(songID, body.TrackID, fingerprint, body.Confirmed)
	if err != nil {
		respondError(w, 500, "Cannot confirm recording")
		return
	}
	if !applied {
		respondError(w, 409, "Source changed; reconfirm recording")
		return
	}
	respondJSON(w, map[string]string{"status": "confirmed"})
}
func (a *API) deleteSpotifyRecordingLink(w http.ResponseWriter, r *http.Request) {
	if err := a.db.DeleteSpotifyRecording(chi.URLParam(r, "songID")); err != nil {
		respondError(w, 500, "Cannot remove recording identity")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// InstallSpotifyAnalysisService is an explicit composition seam. Normal New
// installs it only for a connected cookie lifetime. Installing a new session clears account-bound
// reference cache first; callers retain ownership if installation fails.
func (a *API) InstallSpotifyAnalysisService(service *spotifyrefresh.Service) error {
	if service == nil || !service.Status().Configured {
		return errors.New("configured Spotify reference service required")
	}
	a.spotifyAnalysisMu.Lock()
	defer a.spotifyAnalysisMu.Unlock()
	if a.spotifyAnalysisClosed || a.spotifyAnalysis != nil {
		return errors.New("spotify reference service already installed or API closed")
	}
	if err := a.db.PurgeExternalAnalysis(); err != nil {
		return err
	}
	a.spotifyAnalysis = service
	return nil
}
func (a *API) refreshSpotifyAnalysis(w http.ResponseWriter, r *http.Request) {
	endpoint, valid := spotifyCacheEndpoint(r)
	id := chi.URLParam(r, "trackID")
	if !valid || !db.ValidSpotifyRecordingID(id) {
		respondError(w, 400, "Invalid Spotify recording or endpoint")
		return
	}
	a.spotifyAnalysisMu.RLock()
	service := a.spotifyAnalysis
	a.spotifyAnalysisMu.RUnlock()
	if service == nil {
		respondJSONStatus(w, 503, map[string]string{"code": "disabled"})
		return
	}
	result, err := service.Refresh(r.Context(), id, endpoint)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			respondJSONStatus(w, 408, map[string]string{"code": "request_canceled"})
			return
		}
		var failure *spotifyanalysis.Error
		if errors.As(err, &failure) {
			if failure.RetryAfter > 0 {
				w.Header().Set("Retry-After", strconv.FormatInt(int64(math.Ceil(failure.RetryAfter.Seconds())), 10))
			}
			status := 503
			if failure.Code == spotifyanalysis.InvalidTrackID {
				status = 400
			}
			respondJSONStatus(w, status, map[string]string{"code": string(failure.Code)})
			return
		}
		respondError(w, 500, "Cannot refresh Spotify reference")
		return
	}
	status := http.StatusOK
	if result.Failure != nil {
		switch result.Failure.Code {
		case spotifyanalysis.NotFound:
			status = 404
		case spotifyanalysis.AnalysisUnavailable:
			status = 422
		case spotifyanalysis.AuthenticationRequired:
			status = 401
		case spotifyanalysis.AccessDenied:
			status = 403
		case spotifyanalysis.RateLimited:
			status = 429
		case spotifyanalysis.ProviderChanged:
			status = 502
		default:
			status = 503
		}
		delay := time.Until(result.Failure.RetryAt)
		if delay > 0 {
			w.Header().Set("Retry-After", strconv.FormatInt(int64(math.Ceil(delay.Seconds())), 10))
		}
	}
	respondJSONStatus(w, status, result)
}
func (a *API) disconnectSpotifyAnalysis(w http.ResponseWriter, r *http.Request) {
	a.spotifyAnalysisMu.Lock()
	defer a.spotifyAnalysisMu.Unlock()
	if a.spotifyAnalysis != nil {
		a.spotifyAnalysis.Disconnect()
	}
	if err := a.db.PurgeExternalAnalysis(); err != nil {
		respondError(w, 500, "Cannot purge Spotify reference cache")
		return
	}
	a.spotifyAnalysis = nil
	w.WriteHeader(http.StatusNoContent)
}
func respondJSONStatus(w http.ResponseWriter, status int, value interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(value)
}

// initSpotifyAnalysis composes an inert reference service for the selected cookie
// lifetime. Refresh remains explicit; construction does not contact Spotify.
func (a *API) initSpotifyAnalysis() {
	runtime := a.spotifyAuth
	if runtime == nil {
		return
	}
	status := runtime.status()
	if status.Provider != "webplayer" || !status.Connected {
		return
	}
	a.spotifyAnalysisMu.Lock()
	defer a.spotifyAnalysisMu.Unlock()
	if a.spotifyAnalysisClosed || a.spotifyAnalysis != nil {
		return
	}
	contract := spotifyauth.PinnedWebPlayerContract()
	runtime.mu.RLock()
	accountContext := runtime.metadataContext
	epoch := runtime.metadataEpoch
	pending := runtime.pendingOwner != nil
	runtime.mu.RUnlock()
	if pending {
		return
	}
	client := spotifyanalysis.NewClient(runtime, spotifyanalysis.Options{Enabled: true, AppVersion: contract.AppVersion, AccountContext: accountContext})
	store := spotifyRuntimeRefreshStore{DB: a.db, fence: db.SpotifyMetadataFence{Epoch: epoch, ContextKey: accountContext}}
	service, err := spotifyrefresh.New(store, client, spotifyrefresh.Options{Enabled: true, AdapterRevision: contract.Revision})
	if err != nil {
		logger.API("Could not initialize Spotify reference service")
		return
	}
	a.spotifyAnalysis = service
}

// getSpotifyAnalysisArtifact serves a bounded, explicitly requested representation.
// It performs no provider request and fences private data to the current account.
func (a *API) getSpotifyAnalysisArtifact(w http.ResponseWriter, r *http.Request) {
	endpoint, valid := spotifyCacheEndpoint(r)
	if r.URL.Query().Get("endpoint") == "three_band_waveform" {
		endpoint = "three_band_waveform"
		valid = true
	}
	id := chi.URLParam(r, "trackID")
	kind := r.URL.Query().Get("kind")
	if kind == "" {
		kind = "domain"
		if endpoint == "three_band_waveform" {
			kind = "spotify_three_band"
		}
	}
	if !valid || !db.ValidSpotifyRecordingID(id) {
		respondError(w, 400, "Invalid Spotify recording or endpoint")
		return
	}
	switch kind {
	case "domain", "bars", "beats", "tatums", "sections", "segments", "spotify_three_band":
	default:
		respondError(w, 400, "Invalid artifact kind")
		return
	}
	runtime := a.spotifyTokens()
	ctx, cancel := runtime.requestContext(r.Context())
	defer cancel()
	err := runtime.withMetadataRead(ctx, func(fence db.SpotifyMetadataReadFence) error {
		var artifact *db.SpotifyAudioArtifact
		var err error
		if fence.Pending {
			artifact, err = a.db.GetSpotifyAudioArtifactForRuntime(fence, id, endpoint, kind)
		} else {
			artifact, err = a.db.GetSpotifyActiveAudioArtifactForRuntime(db.SpotifyMetadataFence{Epoch: fence.Epoch, ContextKey: fence.ContextKey}, id, endpoint, kind)
		}
		if err != nil {
			return err
		}
		if artifact == nil {
			respondError(w, 404, "Spotify artifact not cached")
			return nil
		}
		if endpoint == "three_band_waveform" {
			decoded, err := waveform.DecodeDomain(artifact.Payload)
			if err != nil {
				return err
			}
			offset, limit := 0, 4096
			if value := r.URL.Query().Get("offset"); value != "" {
				offset, err = strconv.Atoi(value)
				if err != nil {
					respondError(w, 400, "Invalid waveform range")
					return nil
				}
			}
			if value := r.URL.Query().Get("limit"); value != "" {
				limit, err = strconv.Atoi(value)
				if err != nil {
					respondError(w, 400, "Invalid waveform range")
					return nil
				}
			}
			if offset < 0 || limit <= 0 || limit > 10000 {
				respondError(w, 400, "Invalid waveform range")
				return nil
			}
			if offset > len(decoded.Lows) {
				respondError(w, 416, "Waveform range unavailable")
				return nil
			}
			end := offset + limit
			if end > len(decoded.Lows) {
				end = len(decoded.Lows)
			}
			etag := `"` + artifact.PayloadHash + "-" + strconv.Itoa(offset) + "-" + strconv.Itoa(limit) + `"`
			if fence.Pending {
				etag = strings.TrimSuffix(etag, `"`) + `-unverified"`
			}
			w.Header().Set("ETag", etag)
			w.Header().Set("Cache-Control", "private, no-store")
			if r.Header.Get("If-None-Match") == etag {
				w.WriteHeader(http.StatusNotModified)
				return nil
			}
			respondJSON(w, map[string]any{"readOnly": true, "unverified": fence.Pending, "provenance": "spotify_private_cache", "trackId": id, "representation": "spotify_three_band", "sampleRate": decoded.SampleRate, "windowMilliseconds": decoded.WindowMilliseconds, "totalSamples": len(decoded.Lows), "offset": offset, "lows": decoded.Lows[offset:end], "mids": decoded.Mids[offset:end], "highs": decoded.Highs[offset:end], "durationSeconds": decoded.DurationSeconds(), "normalization": "provider_native_int32", "retrievedAt": artifact.RetrievedAt, "stale": !time.Now().Before(artifact.ExpiresAt)})
			return nil
		}
		etag := `"` + artifact.PayloadHash + `"`
		if fence.Pending {
			etag = strings.TrimSuffix(etag, `"`) + `-unverified"`
		}
		w.Header().Set("ETag", etag)
		w.Header().Set("Cache-Control", "private, no-store")
		if r.Header.Get("If-None-Match") == etag {
			w.WriteHeader(http.StatusNotModified)
			return nil
		}
		respondJSON(w, map[string]any{"readOnly": true, "unverified": fence.Pending, "provenance": "spotify_private_cache", "trackId": id, "resource": endpoint, "kind": kind, "schemaVersion": artifact.SchemaVersion, "retrievedAt": artifact.RetrievedAt, "expiresAt": artifact.ExpiresAt, "stale": !time.Now().Before(artifact.ExpiresAt), "payload": json.RawMessage(artifact.Payload)})
		return nil
	})
	if err != nil {
		respondError(w, 503, "Spotify artifact unavailable")
	}
}
