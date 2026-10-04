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
	"github.com/go-chi/chi/v5"
	"io"
	"math"
	"net/http"
	"strconv"
	"time"
)

type spotifyCacheResponse struct {
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
	cache, err := a.db.GetExternalAnalysis(id, endpoint)
	if err != nil {
		respondError(w, 500, "Cannot read Spotify reference cache")
		return
	}
	failure, err := a.db.GetExternalAnalysisStatus(id, endpoint)
	if err != nil {
		respondError(w, 500, "Cannot read Spotify reference status")
		return
	}
	result := spotifyCacheResponse{State: "not_cached", Endpoint: endpoint, Cache: cache, LastFailure: failure}
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
	respondJSON(w, result)
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
	client := spotifyanalysis.NewClient(runtime, spotifyanalysis.Options{Enabled: true, AppVersion: contract.AppVersion})
	service, err := spotifyrefresh.New(a.db, client, spotifyrefresh.Options{Enabled: true, AdapterRevision: contract.Revision})
	if err != nil {
		logger.API("Could not initialize Spotify reference service")
		return
	}
	a.spotifyAnalysis = service
}
