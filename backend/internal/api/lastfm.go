// Package api provides HTTP handlers for ViiB MediaHub backend.
//
// lastfm.go - HTTP handlers for Last.FM API integration.
//
// This file provides REST endpoints for:
//   - Settings management (GET/POST /api/lastfm/settings)
//   - Connection testing (POST /api/lastfm/test)
//   - Mobile authentication for scrobbling (POST /api/lastfm/authenticate)
//   - Integration status (GET /api/lastfm/status)
//   - Song enrichment triggers (POST /api/lastfm/enrich/songs)
//   - Artist enrichment triggers (POST /api/lastfm/enrich/artists)
//   - Track info lookup (GET /api/lastfm/track)
//   - Similar tracks lookup (GET /api/lastfm/similar)
//
// Sensitive settings (API key, shared secret, session key) are stored encrypted
// in the database via the crypto.IsSensitiveKey() mechanism.
//
// The Last.FM client is initialized on startup if enabled in settings, and can
// be reinitialized when settings are saved via initLastFMClient().
//
// Created: 2025-12-31
// Last Modified: 2025-12-31
package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/lastfm"
	"github.com/ajbergh/viib-mediahub/internal/logger"
)

// EnrichmentSource defines the metadata enrichment source type.
// Valid values: "ai" (use LLM), "lastfm" (use Last.FM), "hybrid" (Last.FM first, AI fallback).
type EnrichmentSource string

const (
	EnrichmentSourceAI     EnrichmentSource = "ai"
	EnrichmentSourceLastFM EnrichmentSource = "lastfm"
	EnrichmentSourceHybrid EnrichmentSource = "hybrid"
)

// LastFMSettings contains the user's Last.FM API configuration.
type LastFMSettings struct {
	APIKey           string           `json:"apiKey"`
	SharedSecret     string           `json:"sharedSecret"`
	SessionKey       string           `json:"sessionKey,omitempty"`
	Username         string           `json:"username,omitempty"`
	Enabled          bool             `json:"enabled"`
	EnrichmentSource EnrichmentSource `json:"enrichmentSource,omitempty"` // "ai", "lastfm", or "hybrid"
}

// LastFMStatus contains the current status of Last.FM integration.
type LastFMStatus struct {
	Configured   bool                   `json:"configured"`
	Connected    bool                   `json:"connected"`
	CanScrobble  bool                   `json:"canScrobble"`
	Username     string                 `json:"username,omitempty"`
	Stats        map[string]interface{} `json:"stats,omitempty"`
	LastError    string                 `json:"lastError,omitempty"`
	LastSyncTime int64                  `json:"lastSyncTime,omitempty"`
}

// handleGetLastFMSettings returns the current Last.FM settings (excluding secrets).
func (a *API) handleGetLastFMSettings(w http.ResponseWriter, r *http.Request) {
	apiKey, _ := a.db.GetSetting("lastfm_api_key")
	enabled, _ := a.db.GetSetting("lastfm_enabled")
	username, _ := a.db.GetSetting("lastfm_username")
	enrichmentSource, _ := a.db.GetSetting("enrichment_source")

	// Default to "ai" if not set
	if enrichmentSource == "" {
		enrichmentSource = "ai"
	}

	settings := LastFMSettings{
		APIKey:           apiKey, // Only return that it exists, not the actual key
		Enabled:          enabled == "true",
		Username:         username,
		EnrichmentSource: EnrichmentSource(enrichmentSource),
	}

	// Mask the API key if present
	if apiKey != "" {
		settings.APIKey = apiKey[:4] + "..." + apiKey[len(apiKey)-4:]
	}

	respondJSON(w, settings)
}

// handleSaveLastFMSettings saves the Last.FM API configuration.
func (a *API) handleSaveLastFMSettings(w http.ResponseWriter, r *http.Request) {
	var settings LastFMSettings
	if err := json.NewDecoder(r.Body).Decode(&settings); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Save settings (use encrypted storage for secrets)
	if settings.APIKey != "" && !isKeyMasked(settings.APIKey) {
		if err := a.db.SetSetting("lastfm_api_key", settings.APIKey); err != nil {
			respondError(w, http.StatusInternalServerError, "Failed to save API key")
			return
		}
	}

	if settings.SharedSecret != "" {
		if err := a.db.SetSetting("lastfm_shared_secret", settings.SharedSecret); err != nil {
			respondError(w, http.StatusInternalServerError, "Failed to save shared secret")
			return
		}
	}

	if err := a.db.SetSetting("lastfm_enabled", strconv.FormatBool(settings.Enabled)); err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to save enabled setting")
		return
	}

	if settings.Username != "" {
		if err := a.db.SetSetting("lastfm_username", settings.Username); err != nil {
			respondError(w, http.StatusInternalServerError, "Failed to save username")
			return
		}
	}

	// Save enrichment source preference
	if settings.EnrichmentSource != "" {
		// Validate enrichment source value
		switch settings.EnrichmentSource {
		case EnrichmentSourceAI, EnrichmentSourceLastFM, EnrichmentSourceHybrid:
			if err := a.db.SetSetting("enrichment_source", string(settings.EnrichmentSource)); err != nil {
				respondError(w, http.StatusInternalServerError, "Failed to save enrichment source")
				return
			}
		default:
			respondError(w, http.StatusBadRequest, "Invalid enrichment source. Must be 'ai', 'lastfm', or 'hybrid'")
			return
		}
	}

	// Reinitialize the Last.FM client if enabled
	if settings.Enabled {
		a.initLastFMClient()
	}

	logger.Log("LastFM", "Settings saved, enabled=%v, enrichmentSource=%s", settings.Enabled, settings.EnrichmentSource)
	respondJSON(w, map[string]bool{"success": true})
}

// handleTestLastFMConnection tests the Last.FM API connection.
func (a *API) handleTestLastFMConnection(w http.ResponseWriter, r *http.Request) {
	if a.lastfmClient == nil {
		respondError(w, http.StatusBadRequest, "Last.FM not configured")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	if err := a.lastfmClient.TestConnection(ctx); err != nil {
		respondError(w, http.StatusServiceUnavailable, err.Error())
		return
	}

	respondJSON(w, map[string]interface{}{
		"success": true,
		"message": "Connection to Last.FM successful",
	})
}

// handleLastFMAuthenticate performs Last.FM authentication for scrobbling.
func (a *API) handleLastFMAuthenticate(w http.ResponseWriter, r *http.Request) {
	if a.lastfmClient == nil {
		respondError(w, http.StatusBadRequest, "Last.FM not configured")
		return
	}

	var creds struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&creds); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if creds.Username == "" || creds.Password == "" {
		respondError(w, http.StatusBadRequest, "Username and password required")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	sessionKey, err := a.lastfmClient.Authenticate(ctx, creds.Username, creds.Password)
	if err != nil {
		respondError(w, http.StatusUnauthorized, err.Error())
		return
	}

	// Save session key and username
	if err := a.db.SetSetting("lastfm_session_key", sessionKey); err != nil {
		logger.Log("LastFM", "Failed to save session key: %v", err)
	}
	if err := a.db.SetSetting("lastfm_username", creds.Username); err != nil {
		logger.Log("LastFM", "Failed to save username: %v", err)
	}

	respondJSON(w, map[string]interface{}{
		"success":  true,
		"username": creds.Username,
	})
}

// handleLastFMStatus returns the current Last.FM integration status.
func (a *API) handleLastFMStatus(w http.ResponseWriter, r *http.Request) {
	status := LastFMStatus{
		Configured:  a.lastfmClient != nil && a.lastfmClient.IsConfigured(),
		Connected:   false,
		CanScrobble: a.lastfmClient != nil && a.lastfmClient.CanScrobble(),
	}

	if username, _ := a.db.GetSetting("lastfm_username"); username != "" {
		status.Username = username
	}

	// Test connection if configured
	if status.Configured {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		if err := a.lastfmClient.TestConnection(ctx); err != nil {
			status.LastError = err.Error()
		} else {
			status.Connected = true
		}
	}

	// Get enrichment stats
	if stats, err := a.db.GetLastFMEnrichmentStats(); err == nil {
		status.Stats = stats
	}

	respondJSON(w, status)
}

// handleLastFMEnrichSongs starts enrichment of songs with Last.FM data.
func (a *API) handleLastFMEnrichSongs(w http.ResponseWriter, r *http.Request) {
	if a.lastfmClient == nil || !a.lastfmClient.IsConfigured() {
		respondError(w, http.StatusBadRequest, "Last.FM not configured")
		return
	}

	var opts struct {
		Limit        int  `json:"limit"`
		Force        bool `json:"force"`
		FetchSimilar bool `json:"fetchSimilar"`
	}

	if r.Body != nil && r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&opts); err != nil {
			respondError(w, http.StatusBadRequest, "Invalid request body")
			return
		}
	}

	if opts.Limit <= 0 {
		opts.Limit = 100
	}
	if opts.Limit > 1000 {
		opts.Limit = 1000
	}

	// Create enricher and get songs to enrich
	enricher := lastfm.NewEnricher(a.lastfmClient, a.db)

	songs, err := enricher.GetSongsNeedingEnrichment(opts.Limit)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to get songs: "+err.Error())
		return
	}

	if len(songs) == 0 {
		respondJSON(w, map[string]interface{}{
			"message":   "No songs need enrichment",
			"processed": 0,
			"enriched":  0,
		})
		return
	}

	// Start enrichment in background
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()

		result, err := enricher.EnrichSongs(ctx, songs, lastfm.EnrichOptions{
			Force:          opts.Force,
			FetchSimilar:   opts.FetchSimilar,
			MinTagCount:    30,
			MaxConcurrency: 2,
		})

		if err != nil {
			logger.Log("LastFM", "Enrichment error: %v", err)
		} else {
			logger.Log("LastFM", "Enrichment complete: processed=%d, enriched=%d, errors=%d",
				result.Processed, result.Enriched, result.Errors)
		}

		// Store last sync time
		a.db.SetSetting("lastfm_last_sync", strconv.FormatInt(time.Now().Unix(), 10))
	}()

	respondJSON(w, map[string]interface{}{
		"message":  "Enrichment started",
		"queued":   len(songs),
		"inFlight": true,
	})
}

// handleLastFMEnrichArtists starts enrichment of artists with Last.FM data.
func (a *API) handleLastFMEnrichArtists(w http.ResponseWriter, r *http.Request) {
	if a.lastfmClient == nil || !a.lastfmClient.IsConfigured() {
		respondError(w, http.StatusBadRequest, "Last.FM not configured")
		return
	}

	var opts struct {
		Limit        int  `json:"limit"`
		FetchSimilar bool `json:"fetchSimilar"`
	}

	if r.Body != nil && r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&opts); err != nil {
			respondError(w, http.StatusBadRequest, "Invalid request body")
			return
		}
	}

	if opts.Limit <= 0 {
		opts.Limit = 50
	}
	if opts.Limit > 500 {
		opts.Limit = 500
	}

	// Get artists to enrich
	artists, err := a.db.GetArtistsWithoutLastFM(opts.Limit)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to get artists: "+err.Error())
		return
	}

	if len(artists) == 0 {
		respondJSON(w, map[string]interface{}{
			"message":   "No artists need enrichment",
			"processed": 0,
			"enriched":  0,
		})
		return
	}

	// Start enrichment in background
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
		defer cancel()

		enricher := lastfm.NewEnricher(a.lastfmClient, a.db)
		result, err := enricher.EnrichArtists(ctx, artists, opts.FetchSimilar)

		if err != nil {
			logger.Log("LastFM", "Artist enrichment error: %v", err)
		} else {
			logger.Log("LastFM", "Artist enrichment complete: processed=%d, enriched=%d, errors=%d",
				result.Processed, result.Enriched, result.Errors)
		}
	}()

	respondJSON(w, map[string]interface{}{
		"message":  "Artist enrichment started",
		"queued":   len(artists),
		"inFlight": true,
	})
}

// handleGetTrackLastFM fetches Last.FM data for a specific track.
func (a *API) handleGetTrackLastFM(w http.ResponseWriter, r *http.Request) {
	if a.lastfmClient == nil || !a.lastfmClient.IsConfigured() {
		respondError(w, http.StatusBadRequest, "Last.FM not configured")
		return
	}

	artist := r.URL.Query().Get("artist")
	track := r.URL.Query().Get("track")

	if artist == "" || track == "" {
		respondError(w, http.StatusBadRequest, "artist and track parameters required")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	info, err := a.lastfmClient.GetTrackInfo(ctx, artist, track)
	if err != nil {
		respondError(w, http.StatusNotFound, err.Error())
		return
	}

	respondJSON(w, info)
}

// handleGetSimilarTracks fetches similar tracks from Last.FM.
func (a *API) handleGetSimilarTracks(w http.ResponseWriter, r *http.Request) {
	if a.lastfmClient == nil || !a.lastfmClient.IsConfigured() {
		respondError(w, http.StatusBadRequest, "Last.FM not configured")
		return
	}

	artist := r.URL.Query().Get("artist")
	track := r.URL.Query().Get("track")
	limitStr := r.URL.Query().Get("limit")

	if artist == "" || track == "" {
		respondError(w, http.StatusBadRequest, "artist and track parameters required")
		return
	}

	limit := 20
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	similar, err := a.lastfmClient.GetSimilarTracks(ctx, artist, track, limit)
	if err != nil {
		respondError(w, http.StatusNotFound, err.Error())
		return
	}

	respondJSON(w, similar)
}

// initLastFMClient initializes the Last.FM client from saved settings.
func (a *API) initLastFMClient() {
	apiKey, _ := a.db.GetSetting("lastfm_api_key")
	sharedSecret, _ := a.db.GetSetting("lastfm_shared_secret")
	enabled, _ := a.db.GetSetting("lastfm_enabled")

	if enabled != "true" || apiKey == "" {
		a.lastfmClient = nil
		return
	}

	a.lastfmClient = lastfm.NewClient(apiKey, sharedSecret)

	// Restore session key if available
	if sessionKey, _ := a.db.GetSetting("lastfm_session_key"); sessionKey != "" {
		a.lastfmClient.SetSessionKey(sessionKey)
	}

	logger.Log("LastFM", "Client initialized (scrobbling=%v)", a.lastfmClient.CanScrobble())
}

// isKeyMasked checks if an API key is masked (e.g., "xxxx...yyyy")
func isKeyMasked(key string) bool {
	return len(key) > 8 && key[4:7] == "..."
}
