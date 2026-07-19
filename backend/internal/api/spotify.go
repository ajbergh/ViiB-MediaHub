// Package api provides REST API handlers for ViiB MediaHub.
// This file implements Spotify integration endpoints including credential management,
// Web API proxy, and download management.
package api

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/logger"
	"github.com/ajbergh/viib-mediahub/internal/spotify"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// SpotifyCredentials represents OAuth 2.0 credentials for Spotify Web API.
// These credentials are stored in the database and used for:
//   - Web API calls (search, metadata, user profile)
//   - librespot authentication for downloads and streaming
//
// The access token is refreshed by the frontend when it expires.
type SpotifyCredentials struct {
	ClientId     string `json:"clientId"`     // OAuth public client ID
	ClientSecret string `json:"clientSecret"` // Legacy field; never returned to the renderer
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
	Expiry       int64  `json:"expiry"`
	CodeVerifier string `json:"codeVerifier,omitempty"`
	OAuthState   string `json:"oauthState,omitempty"`
	RedirectURI  string `json:"redirectUri,omitempty"`
}

// DownloadResponse extends SpotifyDownload with extracted metadata fields.
// Used for API responses to include artwork URL without requiring clients
// to parse the metadata JSON themselves.
type DownloadResponse struct {
	ID           string `json:"id"`
	SpotifyID    string `json:"spotifyId"`
	Type         string `json:"type"`
	Title        string `json:"title"`
	Artist       string `json:"artist,omitempty"`
	Album        string `json:"album,omitempty"`
	Status       string `json:"status"`
	Progress     int    `json:"progress"`
	ErrorMessage string `json:"errorMessage,omitempty"`
	FilePath     string `json:"filePath,omitempty"`
	AddedAt      int64  `json:"addedAt"`
	StartedAt    int64  `json:"startedAt,omitempty"`
	CompletedAt  int64  `json:"completedAt,omitempty"`
	ArtworkUrl   string `json:"artworkUrl,omitempty"` // Extracted from metadata
}

// downloadMetadata is used to parse the Metadata JSON field
type downloadMetadata struct {
	ImageURL string `json:"imageUrl,omitempty"`
}

// saveSpotifyCredentials saves OAuth credentials to the database.
// Called by the frontend after successful OAuth authorization.
// Credentials are stored as JSON in the "spotify_credentials" setting.
//
// The access token is used for:
//   - Spotify Web API calls (proxied through this backend)
//   - librespot-go authentication for direct downloads and streaming
//
// POST /api/spotify/credentials
// Request body: SpotifyCredentials JSON
// Response: {"status": "ok"}
func (a *API) saveSpotifyCredentials(w http.ResponseWriter, r *http.Request) {
	var creds SpotifyCredentials
	if err := json.NewDecoder(r.Body).Decode(&creds); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	log.Printf("[Spotify] Saving credentials. ClientID: %s, HasVerifier: %v", creds.ClientId, creds.CodeVerifier != "")

	// Store credentials as JSON string for atomic read/write
	// This ensures all credential fields are updated together
	credsJSON, err := json.Marshal(creds)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to marshal credentials")
		return
	}

	if err := a.db.SetSetting("spotify_credentials", string(credsJSON)); err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to save credentials")
		return
	}

	respondJSON(w, map[string]string{"status": "ok"})
}

// getSpotifyCredentials retrieves stored OAuth credentials.
// Returns empty object if credentials are not configured.
// Used by frontend to check authentication status and for token refresh.
//
// GET /api/spotify/credentials
// Response: SpotifyCredentials JSON or {}
func (a *API) getSpotifyCredentials(w http.ResponseWriter, r *http.Request) {
	val, err := a.db.GetSetting("spotify_credentials")
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to retrieve credentials")
		return
	}

	if val == "" {
		respondJSON(w, map[string]interface{}{}) // Empty object if not set
		return
	}

	var creds SpotifyCredentials
	if err := json.Unmarshal([]byte(val), &creds); err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to parse credentials")
		return
	}

	creds.ClientSecret = ""
	respondJSON(w, creds)
}

// spotifySearch proxies search requests to Spotify Web API.
// This keeps the access token server-side for security.
//
// Query Parameters:
//   - q: Search query string (required)
//   - type: Comma-separated list of types (track, album, artist, playlist)
//   - limit: Number of results per type (default 20, max 50)
//   - offset: Offset for pagination (default 0)
//
// GET /api/spotify/search?q=query&type=track,album&limit=20
// Response: Spotify API search response (proxied)
func (a *API) spotifySearch(w http.ResponseWriter, r *http.Request) {
	accessToken, err := a.validSpotifyAccessToken(r.Context())
	if err != nil {
		respondError(w, http.StatusUnauthorized, err.Error())
		return
	}
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if query == "" {
		respondError(w, http.StatusBadRequest, "Missing query parameter")
		return
	}
	params := url.Values{"q": {query}}
	for _, key := range []string{"type", "limit", "offset", "market"} {
		if value := r.URL.Query().Get(key); value != "" {
			params.Set(key, value)
		}
	}
	request, err := http.NewRequestWithContext(r.Context(), http.MethodGet, "https://api.spotify.com/v1/search?"+params.Encode(), nil)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to create request")
		return
	}
	request.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(request)
	if err != nil {
		respondError(w, http.StatusBadGateway, "Failed to fetch from Spotify")
		return
	}
	defer resp.Body.Close()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

// spotifySearchPlaylists provides a fallback playlist search using web scraping.
// The Spotify Web API does not return "First Party" playlists (Made For You,
// Discover Weekly, etc.) in search results. This endpoint scrapes the actual
// Spotify search page to find all playlists including first-party ones.
//
// Query Parameters:
//   - q: Search query string (required)
//
// GET /api/spotify/search/playlists?q=this+is
// Response: { "playlists": { "items": [...] } }
func (a *API) spotifySearchPlaylists(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	if query == "" {
		respondError(w, http.StatusBadRequest, "Missing query parameter 'q'")
		return
	}

	log.Printf("[SpotifySearchPlaylists] Searching for: %s", query)

	result, err := spotify.SearchPlaylists(query)
	if err != nil {
		log.Printf("[SpotifySearchPlaylists] Error: %v", err)
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to search playlists: %v", err))
		return
	}

	log.Printf("[SpotifySearchPlaylists] Found %d playlists", len(result.Playlists))

	// Format response to match Spotify API structure
	// Convert SearchedPlaylist to the format frontend expects
	items := make([]map[string]interface{}, 0, len(result.Playlists))
	for _, p := range result.Playlists {
		item := map[string]interface{}{
			"id":   p.ID,
			"name": p.Name,
			"owner": map[string]interface{}{
				"display_name": p.Author,
			},
			"external_urls": map[string]string{
				"spotify": p.URL,
			},
		}
		// Add images if available
		if len(p.Images) > 0 {
			images := make([]map[string]interface{}, len(p.Images))
			for i, img := range p.Images {
				images[i] = map[string]interface{}{
					"url": img.URL,
				}
			}
			item["images"] = images
		}
		items = append(items, item)
	}

	response := map[string]interface{}{
		"playlists": map[string]interface{}{
			"items": items,
			"total": result.Total,
		},
	}

	respondJSON(w, response)
}

// spotifyGetPlaylistByScraping retrieves playlist details using web scraping.
// This is a fallback for first-party playlists that return 404/403 from the Web API.
//
// URL Parameters:
//   - id: Spotify playlist ID (required)
//
// GET /api/spotify/playlists/{id}/scrape
// Response: Spotify playlist format with tracks array
func (a *API) spotifyGetPlaylistByScraping(w http.ResponseWriter, r *http.Request) {
	playlistID := chi.URLParam(r, "id")
	if playlistID == "" {
		respondError(w, http.StatusBadRequest, "Missing playlist ID")
		return
	}

	log.Printf("[SpotifyGetPlaylistByScraping] Scraping playlist: %s", playlistID)

	result, err := spotify.ScrapePlaylistDetail(playlistID)
	if err != nil {
		log.Printf("[SpotifyGetPlaylistByScraping] Error: %v", err)
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to scrape playlist: %v", err))
		return
	}

	log.Printf("[SpotifyGetPlaylistByScraping] Found %d tracks for playlist: %s", len(result.Tracks), result.Name)

	// Convert ScrapedPlaylistDetail to Spotify API format
	tracks := make([]map[string]interface{}, 0, len(result.Tracks))
	for _, t := range result.Tracks {
		artists := make([]map[string]interface{}, 0, len(t.Artists))
		for _, a := range t.Artists {
			artists = append(artists, map[string]interface{}{
				"id":   a.ID,
				"name": a.Name,
			})
		}

		track := map[string]interface{}{
			"id":          t.ID,
			"name":        t.Name,
			"duration_ms": t.Duration,
			"artists":     artists,
		}

		// Always include album with images array (even if empty)
		albumImages := make([]map[string]interface{}, 0)
		albumName := ""
		albumID := ""

		if t.Album != nil {
			albumName = t.Album.Name
			albumID = t.Album.ID
			for _, img := range t.Album.Images {
				albumImages = append(albumImages, map[string]interface{}{
					"url": img.URL,
				})
			}
		}

		// If no album images, use the playlist images as fallback
		if len(albumImages) == 0 && len(result.Images) > 0 {
			for _, img := range result.Images {
				albumImages = append(albumImages, map[string]interface{}{
					"url": img.URL,
				})
			}
		}

		track["album"] = map[string]interface{}{
			"id":     albumID,
			"name":   albumName,
			"images": albumImages,
		}

		tracks = append(tracks, map[string]interface{}{
			"track": track,
		})
	}

	// Build images array
	images := make([]map[string]interface{}, 0, len(result.Images))
	for _, img := range result.Images {
		images = append(images, map[string]interface{}{
			"url": img.URL,
		})
	}

	response := map[string]interface{}{
		"id":          result.ID,
		"name":        result.Name,
		"description": result.Description,
		"images":      images,
		"owner": map[string]interface{}{
			"id":           result.Owner.ID,
			"display_name": result.Owner.Name,
		},
		"tracks": map[string]interface{}{
			"total": len(tracks),
			"items": tracks,
		},
		"scraped": true, // Flag to indicate this was scraped
	}

	respondJSON(w, response)
}

// spotifyGetUserProfile proxies user profile requests to Spotify Web API.
// Returns information about the authenticated user including display name,
// follower count, and profile images.
//
// GET /api/spotify/me
// Response: Spotify user profile JSON (proxied from /v1/me)
func (a *API) spotifyGetUserProfile(w http.ResponseWriter, r *http.Request) {
	accessToken, err := a.validSpotifyAccessToken(r.Context())
	if err != nil {
		respondError(w, http.StatusUnauthorized, err.Error())
		return
	}
	req, err := http.NewRequestWithContext(r.Context(), "GET", "https://api.spotify.com/v1/me", nil)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to create request")
		return
	}

	req.Header.Set("Authorization", "Bearer "+accessToken)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to fetch from Spotify")
		return
	}
	defer resp.Body.Close()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	var result json.RawMessage
	json.NewDecoder(resp.Body).Decode(&result)
	json.NewEncoder(w).Encode(result)
}

// Generic proxy endpoint for Spotify API
func (a *API) spotifyProxy(w http.ResponseWriter, r *http.Request) {
	accessToken, err := a.validSpotifyAccessToken(r.Context())
	if err != nil {
		respondError(w, http.StatusUnauthorized, err.Error())
		return
	}

	// Get the path after /api/spotify/proxy/
	path := r.URL.Query().Get("path")
	if path == "" {
		respondError(w, http.StatusBadRequest, "Missing path parameter")
		return
	}

	path = strings.TrimLeft(path, "/")
	if strings.Contains(path, "..") || strings.Contains(path, "\\") || strings.Contains(path, "://") {
		respondError(w, http.StatusBadRequest, "Invalid Spotify API path")
		return
	}
	spotifyURL := "https://api.spotify.com/v1/" + path

	// Copy query parameters except 'path'
	params := r.URL.Query()
	params.Del("path")
	if len(params) > 0 {
		spotifyURL += "?" + params.Encode()
	}

	req, err := http.NewRequest(r.Method, spotifyURL, r.Body)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to create request")
		return
	}

	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to fetch from Spotify")
		return
	}
	defer resp.Body.Close()

	// Forward response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	var result json.RawMessage
	json.NewDecoder(resp.Body).Decode(&result)
	json.NewEncoder(w).Encode(result)
}

// Download endpoints

func (a *API) downloadTrack(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SpotifyID string `json:"spotifyId"`
		Title     string `json:"title"`
		Artist    string `json:"artist"`
		Album     string `json:"album"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("Error decoding download track request: %v", err)
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	log.Printf("Received download track request: %+v", req)

	if req.SpotifyID == "" {
		respondError(w, http.StatusBadRequest, "Missing spotifyId")
		return
	}

	// Queue the download (nil metadata for single track downloads - uses fallback path)
	downloadID, err := a.downloadManager.QueueDownload(
		req.SpotifyID,
		fmt.Sprintf("spotify:track:%s", req.SpotifyID),
		"track",
		req.Title,
		req.Artist,
		req.Album,
		nil, // No metadata for single track downloads
	)

	if err != nil {
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to queue download: %v", err))
		return
	}

	respondJSON(w, map[string]interface{}{
		"id":      downloadID,
		"status":  "queued",
		"message": "Track download queued successfully",
	})
}

func (a *API) downloadAlbum(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SpotifyID string `json:"spotifyId"`
		Title     string `json:"title"`
		Artist    string `json:"artist"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("Error decoding download album request: %v", err)
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	log.Printf("Received download album request: %+v", req)

	if req.SpotifyID == "" {
		respondError(w, http.StatusBadRequest, "Missing spotifyId")
		return
	}

	// Fetch tracks for the album
	tracks, imageURL, err := a.fetchAlbumTracks(req.SpotifyID)
	if err != nil {
		log.Printf("Error fetching album tracks: %v", err)
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to fetch album tracks: %v", err))
		return
	}

	log.Printf("Found %d tracks for album %s (image: %s)", len(tracks), req.SpotifyID, imageURL)

	// Queue each track with metadata for proper file organization
	queuedCount := 0
	for _, track := range tracks {
		metadata := &DownloadMetadata{
			TrackNumber: track.TrackNumber,
			DiscNumber:  track.DiscNumber,
			AlbumArtist: track.AlbumArtist,
			ReleaseDate: track.ReleaseDate,
			ImageURL:    imageURL, // Album artwork URL
		}

		_, err := a.downloadManager.QueueDownload(
			track.ID,
			fmt.Sprintf("spotify:track:%s", track.ID),
			"track",
			track.Name,
			track.Artist,
			track.Album,
			metadata,
		)
		if err != nil {
			log.Printf("Failed to queue track %s: %v", track.ID, err)
			continue
		}
		queuedCount++
	}

	respondJSON(w, map[string]interface{}{
		"status":  "queued",
		"message": fmt.Sprintf("Queued %d tracks from album", queuedCount),
		"count":   queuedCount,
	})
}

func (a *API) downloadPlaylist(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SpotifyID string `json:"spotifyId"`
		Name      string `json:"name"`
		Owner     string `json:"owner"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("Error decoding download playlist request: %v", err)
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	log.Printf("Received download playlist request: %+v", req)

	if req.SpotifyID == "" {
		respondError(w, http.StatusBadRequest, "Missing spotifyId")
		return
	}

	// Fetch tracks for the playlist (also returns the playlist name and image from Spotify)
	tracks, playlistName, imageURL, err := a.fetchPlaylistTracks(req.SpotifyID, nil)
	if err != nil {
		log.Printf("Error fetching playlist tracks: %v", err)
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to fetch playlist tracks: %v", err))
		return
	}

	// Use the playlist name from request if provided, otherwise from Spotify
	if req.Name != "" {
		playlistName = req.Name
	}

	log.Printf("Found %d tracks for playlist %s (image: %s)", len(tracks), playlistName, imageURL)

	// Queue each track with playlist metadata for proper file organization
	queuedCount := 0
	for i, track := range tracks {
		metadata := &DownloadMetadata{
			PlaylistName:  playlistName,
			PlaylistOrder: i + 1, // 1-based position in playlist
			ReleaseDate:   track.ReleaseDate,
			ImageURL:      imageURL, // Playlist artwork URL
		}

		_, err := a.downloadManager.QueueDownload(
			track.ID,
			fmt.Sprintf("spotify:track:%s", track.ID),
			"track",
			track.Name,
			track.Artist,
			track.Album,
			metadata,
		)
		if err != nil {
			log.Printf("Failed to queue track %s: %v", track.ID, err)
			continue
		}
		queuedCount++
	}

	respondJSON(w, map[string]interface{}{
		"status":  "queued",
		"message": fmt.Sprintf("Queued %d tracks from playlist", queuedCount),
		"count":   queuedCount,
	})
}

// downloadFromURL handles direct Spotify URL/URI downloads.
// Accepts URLs like:
//   - https://open.spotify.com/track/4iV5W9uYEdYUVa79Axb7Rh
//   - https://open.spotify.com/album/1DFixLWuPkv3KT3TnV35m3
//   - https://open.spotify.com/playlist/37i9dQZF1DXcBWIGoYBM5M
//   - spotify:track:4iV5W9uYEdYUVa79Axb7Rh
//   - spotify:album:1DFixLWuPkv3KT3TnV35m3
//   - spotify:playlist:37i9dQZF1DXcBWIGoYBM5M
//
// POST /api/spotify/download/url
func (a *API) downloadFromURL(w http.ResponseWriter, r *http.Request) {
	var req struct {
		URL string `json:"url"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.URL == "" {
		respondError(w, http.StatusBadRequest, "URL is required")
		return
	}

	// Parse the URL/URI to extract type and ID
	contentType, spotifyID := parseSpotifyURL(req.URL)
	if contentType == "" || spotifyID == "" {
		respondError(w, http.StatusBadRequest, "Invalid Spotify URL or URI. Supported formats: https://open.spotify.com/{track|album|playlist}/ID or spotify:{track|album|playlist}:ID")
		return
	}

	// Get access token for Spotify API calls
	credsJSON, err := a.db.GetSetting("spotify_credentials")
	if err != nil || credsJSON == "" {
		respondError(w, http.StatusUnauthorized, "Spotify not configured. Please log in to Spotify first.")
		return
	}

	var creds SpotifyCredentials
	if err := json.Unmarshal([]byte(credsJSON), &creds); err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to parse Spotify credentials")
		return
	}

	if creds.AccessToken == "" {
		respondError(w, http.StatusUnauthorized, "Not logged in to Spotify")
		return
	}

	// Handle based on content type
	switch contentType {
	case "track":
		a.downloadTrackByID(w, creds.AccessToken, spotifyID)
	case "album":
		a.downloadAlbumByID(w, creds.AccessToken, spotifyID)
	case "playlist":
		a.downloadPlaylistByID(w, creds.AccessToken, spotifyID)
	default:
		respondError(w, http.StatusBadRequest, "Unsupported content type: "+contentType)
	}
}

// parseSpotifyURL extracts the content type and ID from a Spotify URL or URI.
// Returns empty strings if the format is invalid.
func parseSpotifyURL(input string) (contentType, spotifyID string) {
	input = strings.TrimSpace(input)

	// Handle Spotify URIs (spotify:track:ID, spotify:album:ID, spotify:playlist:ID)
	if strings.HasPrefix(input, "spotify:") {
		parts := strings.Split(input, ":")
		if len(parts) == 3 {
			contentType = parts[1]
			spotifyID = parts[2]
			// Validate content type
			if contentType == "track" || contentType == "album" || contentType == "playlist" {
				return contentType, spotifyID
			}
		}
		return "", ""
	}

	// Handle Spotify URLs (https://open.spotify.com/track/ID?query)
	// Also handle with /intl-xx/ path segment
	if strings.Contains(input, "open.spotify.com") {
		// Remove query string
		if idx := strings.Index(input, "?"); idx != -1 {
			input = input[:idx]
		}

		// Split path and find content type + ID
		parts := strings.Split(input, "/")
		for i, part := range parts {
			if (part == "track" || part == "album" || part == "playlist") && i+1 < len(parts) {
				contentType = part
				spotifyID = parts[i+1]
				return contentType, spotifyID
			}
		}
	}

	return "", ""
}

// downloadTrackByID fetches track metadata from Spotify and queues the download
func (a *API) downloadTrackByID(w http.ResponseWriter, accessToken, spotifyID string) {
	// Fetch track metadata from Spotify
	req, _ := http.NewRequest("GET", fmt.Sprintf("https://api.spotify.com/v1/tracks/%s", spotifyID), nil)
	req.Header.Set("Authorization", "Bearer "+accessToken)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to fetch track metadata")
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respondError(w, resp.StatusCode, "Failed to fetch track from Spotify")
		return
	}

	var track struct {
		ID      string `json:"id"`
		Name    string `json:"name"`
		Artists []struct {
			Name string `json:"name"`
		} `json:"artists"`
		Album struct {
			Name   string `json:"name"`
			Images []struct {
				URL string `json:"url"`
			} `json:"images"`
		} `json:"album"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&track); err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to parse track metadata")
		return
	}

	artist := ""
	if len(track.Artists) > 0 {
		artist = track.Artists[0].Name
	}

	// Prepare metadata with artwork
	var metadata *DownloadMetadata
	if len(track.Album.Images) > 0 {
		metadata = &DownloadMetadata{ImageURL: track.Album.Images[0].URL}
	}

	// Queue the download
	downloadID, err := a.downloadManager.QueueDownload(
		track.ID,
		fmt.Sprintf("spotify:track:%s", track.ID),
		"track",
		track.Name,
		artist,
		track.Album.Name,
		metadata,
	)

	if err != nil {
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to queue download: %v", err))
		return
	}

	respondJSON(w, map[string]interface{}{
		"id":      downloadID,
		"status":  "queued",
		"type":    "track",
		"title":   track.Name,
		"artist":  artist,
		"album":   track.Album.Name,
		"message": "Track download queued successfully",
	})
}

// downloadAlbumByID fetches album metadata and all tracks, then queues downloads
func (a *API) downloadAlbumByID(w http.ResponseWriter, accessToken, spotifyID string) {
	// Fetch album metadata from Spotify
	req, _ := http.NewRequest("GET", fmt.Sprintf("https://api.spotify.com/v1/albums/%s", spotifyID), nil)
	req.Header.Set("Authorization", "Bearer "+accessToken)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to fetch album metadata")
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respondError(w, resp.StatusCode, "Failed to fetch album from Spotify")
		return
	}

	var album struct {
		Name    string `json:"name"`
		Artists []struct {
			Name string `json:"name"`
		} `json:"artists"`
		Images []struct {
			URL string `json:"url"`
		} `json:"images"`
		Tracks struct {
			Items []struct {
				ID      string `json:"id"`
				Name    string `json:"name"`
				Artists []struct {
					Name string `json:"name"`
				} `json:"artists"`
				TrackNumber int `json:"track_number"`
				DiscNumber  int `json:"disc_number"`
			} `json:"items"`
		} `json:"tracks"`
		TotalTracks int `json:"total_tracks"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&album); err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to parse album metadata")
		return
	}

	albumArtist := ""
	if len(album.Artists) > 0 {
		albumArtist = album.Artists[0].Name
	}

	// Get artwork URL
	imageURL := ""
	if len(album.Images) > 0 {
		imageURL = album.Images[0].URL
	}

	// Queue each track
	queuedCount := 0
	for _, track := range album.Tracks.Items {
		trackArtist := albumArtist
		if len(track.Artists) > 0 {
			trackArtist = track.Artists[0].Name
		}

		metadata := &DownloadMetadata{
			ImageURL:    imageURL,
			TrackNumber: track.TrackNumber,
			DiscNumber:  track.DiscNumber,
			AlbumArtist: albumArtist,
		}

		_, err := a.downloadManager.QueueDownload(
			track.ID,
			fmt.Sprintf("spotify:track:%s", track.ID),
			"track",
			track.Name,
			trackArtist,
			album.Name,
			metadata,
		)
		if err != nil {
			log.Printf("Failed to queue track %s: %v", track.Name, err)
			continue
		}
		queuedCount++
	}

	respondJSON(w, map[string]interface{}{
		"status":  "queued",
		"type":    "album",
		"title":   album.Name,
		"artist":  albumArtist,
		"message": fmt.Sprintf("Queued %d tracks from album", queuedCount),
		"count":   queuedCount,
	})
}

// downloadPlaylistByID fetches playlist metadata and all tracks, then queues downloads.
// For first-party playlists (e.g., "Made For You", "Daily Mix") that return 404/403 from
// the Web API, this function falls back to scraping the public embed page.
func (a *API) downloadPlaylistByID(w http.ResponseWriter, accessToken, spotifyID string) {
	// Fetch playlist metadata from Spotify
	req, _ := http.NewRequest("GET", fmt.Sprintf("https://api.spotify.com/v1/playlists/%s", spotifyID), nil)
	req.Header.Set("Authorization", "Bearer "+accessToken)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to fetch playlist metadata")
		return
	}
	defer resp.Body.Close()

	// Check if this is a first-party playlist that requires scraping fallback
	if spotify.IsFirstPartyPlaylistError(resp.StatusCode) {
		log.Printf("Playlist %s returned %d - attempting embed page scrape fallback", spotifyID, resp.StatusCode)
		a.downloadPlaylistByScraping(w, accessToken, spotifyID)
		return
	}

	if resp.StatusCode != http.StatusOK {
		respondError(w, resp.StatusCode, "Failed to fetch playlist from Spotify")
		return
	}

	var playlist struct {
		Name  string `json:"name"`
		Owner struct {
			DisplayName string `json:"display_name"`
		} `json:"owner"`
		Images []struct {
			URL    string `json:"url"`
			Height int    `json:"height"`
			Width  int    `json:"width"`
		} `json:"images"`
		Tracks struct {
			Items []struct {
				Track struct {
					ID      string `json:"id"`
					Name    string `json:"name"`
					Artists []struct {
						Name string `json:"name"`
					} `json:"artists"`
					Album struct {
						Name   string `json:"name"`
						Images []struct {
							URL string `json:"url"`
						} `json:"images"`
					} `json:"album"`
				} `json:"track"`
			} `json:"items"`
		} `json:"tracks"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&playlist); err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to parse playlist metadata")
		return
	}

	// Get playlist artwork URL (use playlist image, not individual track album images)
	var playlistImageURL string
	if len(playlist.Images) > 0 {
		playlistImageURL = playlist.Images[0].URL
	}

	// Queue each track with playlist metadata for proper file organization
	queuedCount := 0
	for i, item := range playlist.Tracks.Items {
		track := item.Track
		if track.ID == "" {
			continue // Skip local tracks
		}

		artist := ""
		if len(track.Artists) > 0 {
			artist = track.Artists[0].Name
		}

		metadata := &DownloadMetadata{
			PlaylistName:  playlist.Name,
			PlaylistOrder: i + 1,
			ImageURL:      playlistImageURL, // Use playlist artwork, not track album artwork
		}

		_, err := a.downloadManager.QueueDownload(
			track.ID,
			fmt.Sprintf("spotify:track:%s", track.ID),
			"track",
			track.Name,
			artist,
			track.Album.Name,
			metadata,
		)
		if err != nil {
			log.Printf("Failed to queue track %s: %v", track.Name, err)
			continue
		}
		queuedCount++
	}

	respondJSON(w, map[string]interface{}{
		"status":  "queued",
		"type":    "playlist",
		"title":   playlist.Name,
		"owner":   playlist.Owner.DisplayName,
		"message": fmt.Sprintf("Queued %d tracks from playlist", queuedCount),
		"count":   queuedCount,
	})
}

// downloadPlaylistByScraping uses web scraping to download first-party Spotify playlists
// that are not accessible via the Web API. It scrapes the public embed page to get
// track IDs, then fetches metadata for each track individually via the API.
func (a *API) downloadPlaylistByScraping(w http.ResponseWriter, accessToken, spotifyID string) {
	// Scrape playlist data from embed page
	scraped, err := spotify.ScrapePlaylist(spotifyID)
	if err != nil {
		log.Printf("Failed to scrape playlist %s: %v", spotifyID, err)
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to fetch playlist: %v. This playlist may not be publicly accessible.", err))
		return
	}

	log.Printf("Scraped playlist '%s' with %d tracks (artwork: %s)", scraped.Name, len(scraped.Tracks), scraped.Artwork)

	// Queue each track - we need to fetch individual track metadata via API
	queuedCount := 0
	client := &http.Client{Timeout: 30 * time.Second}

	for i, trackID := range scraped.Tracks {
		// Fetch track metadata from Spotify API
		req, _ := http.NewRequest("GET", fmt.Sprintf("https://api.spotify.com/v1/tracks/%s", trackID), nil)
		req.Header.Set("Authorization", "Bearer "+accessToken)

		resp, err := client.Do(req)
		if err != nil {
			log.Printf("Failed to fetch track %s metadata: %v", trackID, err)
			continue
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			log.Printf("Track %s returned status %d", trackID, resp.StatusCode)
			continue
		}

		var track struct {
			ID      string `json:"id"`
			Name    string `json:"name"`
			Artists []struct {
				Name string `json:"name"`
			} `json:"artists"`
			Album struct {
				Name   string `json:"name"`
				Images []struct {
					URL string `json:"url"`
				} `json:"images"`
			} `json:"album"`
		}

		if err := json.NewDecoder(resp.Body).Decode(&track); err != nil {
			resp.Body.Close()
			log.Printf("Failed to parse track %s metadata: %v", trackID, err)
			continue
		}
		resp.Body.Close()

		artist := ""
		if len(track.Artists) > 0 {
			artist = track.Artists[0].Name
		}

		// Use scraped playlist artwork for cover.jpg (not individual track album artwork)
		metadata := &DownloadMetadata{
			PlaylistName:  scraped.Name,
			PlaylistOrder: i + 1,
			ImageURL:      scraped.Artwork, // Use playlist artwork, not track album artwork
		}

		_, err = a.downloadManager.QueueDownload(
			track.ID,
			fmt.Sprintf("spotify:track:%s", track.ID),
			"track",
			track.Name,
			artist,
			track.Album.Name,
			metadata,
		)
		if err != nil {
			log.Printf("Failed to queue track %s: %v", track.Name, err)
			continue
		}
		queuedCount++
	}

	if queuedCount == 0 {
		respondError(w, http.StatusInternalServerError, "Failed to queue any tracks from playlist")
		return
	}

	respondJSON(w, map[string]interface{}{
		"status":       "queued",
		"type":         "playlist",
		"title":        scraped.Name,
		"owner":        "Spotify", // First-party playlists are from Spotify
		"message":      fmt.Sprintf("Queued %d tracks from playlist (via fallback)", queuedCount),
		"count":        queuedCount,
		"usedFallback": true,
	})
}

func (a *API) getDownloads(w http.ResponseWriter, r *http.Request) {
	log.Printf("getDownloads called")
	downloads, err := a.downloadManager.GetAllDownloads()
	if err != nil {
		log.Printf("getDownloads error: %v", err)
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to fetch downloads: %v", err))
		return
	}
	log.Printf("getDownloads returning %d downloads", len(downloads))

	// Convert to response format with extracted artwork URL
	responses := make([]DownloadResponse, len(downloads))
	for i, dl := range downloads {
		responses[i] = DownloadResponse{
			ID:           dl.ID,
			SpotifyID:    dl.SpotifyID,
			Type:         dl.Type,
			Title:        dl.Title,
			Artist:       dl.Artist,
			Album:        dl.Album,
			Status:       dl.Status,
			Progress:     dl.Progress,
			ErrorMessage: dl.Error,
			FilePath:     dl.FilePath,
			AddedAt:      dl.AddedAt,
			StartedAt:    dl.StartedAt,
			CompletedAt:  dl.CompletedAt,
		}

		// Extract artwork URL from metadata if present
		if dl.Metadata != "" {
			var meta downloadMetadata
			if err := json.Unmarshal([]byte(dl.Metadata), &meta); err == nil {
				responses[i].ArtworkUrl = meta.ImageURL
			}
		}
	}

	respondJSON(w, responses)
}

func (a *API) getDownloadStatus(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		respondError(w, http.StatusBadRequest, "Missing download ID")
		return
	}

	download, err := a.downloadManager.GetDownload(id)
	if err != nil {
		respondError(w, http.StatusNotFound, "Download not found")
		return
	}

	respondJSON(w, download)
}

func (a *API) deleteDownload(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		respondError(w, http.StatusBadRequest, "Missing download ID")
		return
	}

	if err := a.downloadManager.DeleteDownload(id); err != nil {
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to delete download: %v", err))
		return
	}

	respondJSON(w, map[string]string{"status": "deleted"})
}

func (a *API) retryDownload(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		respondError(w, http.StatusBadRequest, "Missing download ID")
		return
	}

	if err := a.downloadManager.RetryDownload(id); err != nil {
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to retry download: %v", err))
		return
	}

	respondJSON(w, map[string]string{"status": "queued"})
}

func (a *API) forceRestartDownload(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		respondError(w, http.StatusBadRequest, "Missing download ID")
		return
	}

	if err := a.downloadManager.ForceRestartDownload(id); err != nil {
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to force restart download: %v", err))
		return
	}

	respondJSON(w, map[string]string{"status": "queued"})
}

// getSpotifyAuthStatus returns the current Spotify authentication status.
// This is used to check if re-authentication is required (e.g., token expired/revoked).
//
// GET /api/spotify/auth/status
// Response: {"authRequired": true/false, "message": "..."}
func (a *API) getSpotifyAuthStatus(w http.ResponseWriter, r *http.Request) {
	authRequired := a.downloadManager.IsAuthRequired()
	respondJSON(w, map[string]interface{}{
		"authRequired": authRequired,
		"message":      "",
	})
}

// refreshSpotifyAuth is called after the user re-authenticates with Spotify.
// It clears the authRequired flag and resets the session to use new credentials.
//
// POST /api/spotify/auth/refresh
// Response: {"status": "ok"}
func (a *API) refreshSpotifyAuth(w http.ResponseWriter, r *http.Request) {
	a.downloadManager.ClearAuthRequired()
	respondJSON(w, map[string]string{"status": "ok"})
}

func (a *API) clearCompletedDownloads(w http.ResponseWriter, r *http.Request) {
	count, err := a.downloadManager.ClearCompletedDownloads()
	if err != nil {
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to clear completed downloads: %v", err))
		return
	}

	respondJSON(w, map[string]interface{}{"status": "cleared", "count": count})
}

func (a *API) downloadProgressSSE(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher, ok := w.(http.Flusher)
	if !ok {
		respondError(w, http.StatusInternalServerError, "Streaming not supported")
		return
	}
	progressChan := a.downloadManager.SubscribeProgress()
	defer a.downloadManager.UnsubscribeProgress(progressChan)
	keepalive := time.NewTicker(20 * time.Second)
	defer keepalive.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-keepalive.C:
			fmt.Fprint(w, ": keepalive\n\n")
			flusher.Flush()
		case progress, ok := <-progressChan:
			if !ok {
				return
			}
			data, _ := json.Marshal(progress)
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}
	}
}

// Helper functions to fetch Spotify metadata

//lint:ignore U1000 Retained for the planned backend metadata proxy.
func (a *API) fetchSpotifyTrack(trackID string) (map[string]interface{}, error) {
	val, err := a.db.GetSetting("spotify_credentials")
	if err != nil || val == "" {
		return nil, fmt.Errorf("spotify credentials not configured")
	}

	var creds SpotifyCredentials
	if err := json.Unmarshal([]byte(val), &creds); err != nil {
		return nil, err
	}

	req, err := http.NewRequest("GET", fmt.Sprintf("https://api.spotify.com/v1/tracks/%s", trackID), nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+creds.AccessToken)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var track map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&track); err != nil {
		return nil, err
	}

	// Extract relevant info
	result := map[string]interface{}{
		"name":   track["name"],
		"artist": track["artists"].([]interface{})[0].(map[string]interface{})["name"],
		"album":  track["album"].(map[string]interface{})["name"],
	}

	return result, nil
}

// AlbumTrackInfo contains metadata for a track within an album download
type AlbumTrackInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Artist      string `json:"artist"`
	Album       string `json:"album"`
	AlbumArtist string `json:"albumArtist"`
	TrackNumber int    `json:"trackNumber"`
	DiscNumber  int    `json:"discNumber"`
	ReleaseDate string `json:"releaseDate"`
}

func (a *API) fetchAlbumTracks(albumID string) ([]AlbumTrackInfo, string, error) {
	val, err := a.db.GetSetting("spotify_credentials")
	if err != nil || val == "" {
		return nil, "", fmt.Errorf("spotify credentials not configured")
	}

	var creds SpotifyCredentials
	if err := json.Unmarshal([]byte(val), &creds); err != nil {
		return nil, "", err
	}

	// Fetch album tracks (paginated, but we'll just get the first 50 for now)
	req, err := http.NewRequest("GET", fmt.Sprintf("https://api.spotify.com/v1/albums/%s?market=US", albumID), nil)
	if err != nil {
		return nil, "", err
	}

	req.Header.Set("Authorization", "Bearer "+creds.AccessToken)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, "", fmt.Errorf("spotify api error: %s - %s", resp.Status, string(body))
	}

	var album struct {
		Name        string `json:"name"`
		ReleaseDate string `json:"release_date"`
		Images      []struct {
			URL    string `json:"url"`
			Height int    `json:"height"`
			Width  int    `json:"width"`
		} `json:"images"`
		Artists []struct {
			Name string `json:"name"`
		} `json:"artists"`
		Tracks struct {
			Items []struct {
				ID          string `json:"id"`
				Name        string `json:"name"`
				TrackNumber int    `json:"track_number"`
				DiscNumber  int    `json:"disc_number"`
				Artists     []struct {
					Name string `json:"name"`
				} `json:"artists"`
			} `json:"items"`
		} `json:"tracks"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&album); err != nil {
		return nil, "", err
	}

	// Get album artist
	albumArtist := "Unknown Artist"
	if len(album.Artists) > 0 {
		albumArtist = album.Artists[0].Name
	}

	// Get the largest album image URL (first one is usually the largest)
	var imageURL string
	if len(album.Images) > 0 {
		imageURL = album.Images[0].URL
	}

	var tracks []AlbumTrackInfo
	for _, item := range album.Tracks.Items {
		// Track artist (may differ from album artist on compilations)
		trackArtist := albumArtist
		if len(item.Artists) > 0 {
			trackArtist = item.Artists[0].Name
		}

		tracks = append(tracks, AlbumTrackInfo{
			ID:          item.ID,
			Name:        item.Name,
			Artist:      trackArtist,
			Album:       album.Name,
			AlbumArtist: albumArtist,
			TrackNumber: item.TrackNumber,
			DiscNumber:  item.DiscNumber,
			ReleaseDate: album.ReleaseDate,
		})
	}

	return tracks, imageURL, nil
}

// PlaylistTrackInfo contains metadata for a track within a playlist download
type PlaylistTrackInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Artist      string `json:"artist"`
	Album       string `json:"album"`
	ReleaseDate string `json:"releaseDate"`
}

func (a *API) fetchPlaylistTracks(playlistID string, playlistName *string) ([]PlaylistTrackInfo, string, string, error) {
	val, err := a.db.GetSetting("spotify_credentials")
	if err != nil || val == "" {
		return nil, "", "", fmt.Errorf("spotify credentials not configured")
	}

	var creds SpotifyCredentials
	if err := json.Unmarshal([]byte(val), &creds); err != nil {
		return nil, "", "", err
	}

	// Fetch playlist tracks (paginated, but we'll just get the first 100 for now)
	req, err := http.NewRequest("GET", fmt.Sprintf("https://api.spotify.com/v1/playlists/%s", playlistID), nil)
	if err != nil {
		return nil, "", "", err
	}

	req.Header.Set("Authorization", "Bearer "+creds.AccessToken)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", "", err
	}
	defer resp.Body.Close()

	// Check if this is a first-party playlist that requires scraping fallback
	if spotify.IsFirstPartyPlaylistError(resp.StatusCode) {
		log.Printf("Playlist %s returned %d - attempting embed page scrape fallback", playlistID, resp.StatusCode)
		return a.fetchPlaylistTracksByScraping(playlistID, creds.AccessToken)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, "", "", fmt.Errorf("spotify api error: %s", resp.Status)
	}

	var playlist struct {
		Name   string `json:"name"`
		Images []struct {
			URL    string `json:"url"`
			Height int    `json:"height"`
			Width  int    `json:"width"`
		} `json:"images"`
		Tracks struct {
			Items []struct {
				Track struct {
					ID      string `json:"id"`
					Name    string `json:"name"`
					Artists []struct {
						Name string `json:"name"`
					} `json:"artists"`
					Album struct {
						Name        string `json:"name"`
						ReleaseDate string `json:"release_date"`
					} `json:"album"`
				} `json:"track"`
			} `json:"items"`
		} `json:"tracks"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&playlist); err != nil {
		return nil, "", "", err
	}

	// Get the largest playlist image URL (first one is usually the largest)
	var imageURL string
	if len(playlist.Images) > 0 {
		imageURL = playlist.Images[0].URL
	}

	var tracks []PlaylistTrackInfo
	for _, item := range playlist.Tracks.Items {
		if item.Track.ID == "" {
			continue // Skip local files or invalid tracks
		}

		artist := "Unknown Artist"
		if len(item.Track.Artists) > 0 {
			artist = item.Track.Artists[0].Name
		}

		tracks = append(tracks, PlaylistTrackInfo{
			ID:          item.Track.ID,
			Name:        item.Track.Name,
			Artist:      artist,
			Album:       item.Track.Album.Name,
			ReleaseDate: item.Track.Album.ReleaseDate,
		})
	}

	return tracks, playlist.Name, imageURL, nil
}

// fetchPlaylistTracksByScraping uses web scraping to fetch first-party playlist tracks
// that are not accessible via the Web API, then fetches individual track metadata.
func (a *API) fetchPlaylistTracksByScraping(playlistID string, accessToken string) ([]PlaylistTrackInfo, string, string, error) {
	// Scrape playlist data from embed page
	scraped, err := spotify.ScrapePlaylist(playlistID)
	if err != nil {
		return nil, "", "", fmt.Errorf("failed to scrape playlist: %w", err)
	}

	log.Printf("Scraped playlist '%s' with %d tracks", scraped.Name, len(scraped.Tracks))

	// Fetch individual track metadata via API
	client := &http.Client{Timeout: 30 * time.Second}
	var tracks []PlaylistTrackInfo

	for _, trackID := range scraped.Tracks {
		req, _ := http.NewRequest("GET", fmt.Sprintf("https://api.spotify.com/v1/tracks/%s", trackID), nil)
		req.Header.Set("Authorization", "Bearer "+accessToken)

		resp, err := client.Do(req)
		if err != nil {
			log.Printf("Failed to fetch track %s metadata: %v", trackID, err)
			continue
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			continue
		}

		var track struct {
			ID      string `json:"id"`
			Name    string `json:"name"`
			Artists []struct {
				Name string `json:"name"`
			} `json:"artists"`
			Album struct {
				Name        string `json:"name"`
				ReleaseDate string `json:"release_date"`
			} `json:"album"`
		}

		if err := json.NewDecoder(resp.Body).Decode(&track); err != nil {
			resp.Body.Close()
			continue
		}
		resp.Body.Close()

		artist := "Unknown Artist"
		if len(track.Artists) > 0 {
			artist = track.Artists[0].Name
		}

		tracks = append(tracks, PlaylistTrackInfo{
			ID:          track.ID,
			Name:        track.Name,
			Artist:      artist,
			Album:       track.Album.Name,
			ReleaseDate: track.Album.ReleaseDate,
		})
	}

	return tracks, scraped.Name, scraped.Artwork, nil
}

// streamSpotifyTrack streams audio directly from Spotify without saving to disk.
// This enables real-time playback of Spotify tracks within the player.
//
// The endpoint supports HTTP Range requests for seeking within the stream.
// Audio is streamed as OGG Vorbis format (320kbps when available).
//
// GET /api/spotify/stream/{id}
//
// Response Headers:
//   - Content-Type: audio/ogg
//   - Accept-Ranges: bytes
//   - Content-Range: bytes start-end/total (for 206 responses)
//
// Response Codes:
//   - 200 OK: Full content
//   - 206 Partial Content: Range request
//   - 401 Unauthorized: No valid Spotify session
//   - 403 Forbidden: Premium required
//   - 404 Not Found: Track not available
func (a *API) streamSpotifyTrack(w http.ResponseWriter, r *http.Request) {
	spotifyID := chi.URLParam(r, "id")
	if spotifyID == "" {
		respondError(w, http.StatusBadRequest, "Missing track ID")
		return
	}

	// Get quality parameter (default to high)
	quality := r.URL.Query().Get("quality")
	if quality == "" {
		quality = "high"
	}
	// Validate quality parameter
	if quality != "high" && quality != "medium" && quality != "low" {
		quality = "high"
	}

	logger.SpotifyStreamer("Stream request for track: %s (quality: %s)", spotifyID, quality)

	// Get access token and initialize session
	if err := a.downloadManager.EnsureSession(); err != nil {
		logger.SpotifyStreamer("Session error: %v", err)
		// Check if this looks like an expired/invalid token error
		if strings.Contains(err.Error(), "expired") || strings.Contains(err.Error(), "refresh") {
			respondError(w, http.StatusUnauthorized, "Spotify session expired. Token refresh required.")
		} else {
			respondError(w, http.StatusUnauthorized, "Spotify session not available. Please log in.")
		}
		return
	}

	// Get the session manager from download manager
	sessionManager := a.downloadManager.GetSessionManager()
	if sessionManager == nil {
		logger.SpotifyStreamer("No session manager available")
		respondError(w, http.StatusInternalServerError, "Streaming not initialized")
		return
	}

	// Create a streamer for this request
	streamer := spotify.NewStreamer(sessionManager)

	// Generate unique request ID for tracking
	requestID := uuid.New().String()

	// Stream the track with specified quality
	stream, err := streamer.StreamTrackWithQuality(r.Context(), spotifyID, requestID, quality)
	if err != nil {
		logger.SpotifyStreamer("Failed to start stream: %v", err)
		// Check for specific errors
		if strings.Contains(err.Error(), "session") || strings.Contains(err.Error(), "token") {
			respondError(w, http.StatusUnauthorized, "Spotify session expired. Please log in again.")
		} else if strings.Contains(err.Error(), "premium") {
			respondError(w, http.StatusForbidden, "Spotify Premium required for streaming")
		} else {
			respondError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to stream track: %v", err))
		}
		return
	}
	defer stream.Close()

	// Get stream size for Range request support
	// Seek to end to get total size
	totalSize, err := stream.Seek(0, io.SeekEnd)
	if err != nil {
		logger.SpotifyStreamer("Failed to get stream size: %v", err)
		totalSize = 0 // Unknown size
	}

	// Seek back to beginning
	if _, err := stream.Seek(0, io.SeekStart); err != nil {
		logger.SpotifyStreamer("Failed to seek to start: %v", err)
	}

	// Set common response headers
	w.Header().Set("Content-Type", stream.Info().ContentType)
	w.Header().Set("Accept-Ranges", "bytes")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	// Handle Range requests for seeking
	rangeHeader := r.Header.Get("Range")
	if rangeHeader != "" && totalSize > 0 {
		// Parse range header (e.g., "bytes=0-1048575" or "bytes=1000-")
		var start, end int64
		if _, err := fmt.Sscanf(rangeHeader, "bytes=%d-%d", &start, &end); err != nil {
			// Try parsing without end (e.g., "bytes=1000-")
			if _, err := fmt.Sscanf(rangeHeader, "bytes=%d-", &start); err != nil {
				logger.SpotifyStreamer("Invalid Range header: %s", rangeHeader)
				// Fall through to normal response
			} else {
				end = totalSize - 1
			}
		}

		// Validate range
		if start >= 0 && start < totalSize {
			if end <= 0 || end >= totalSize {
				end = totalSize - 1
			}
			if end >= start {
				// Seek to start position
				if _, err := stream.Seek(start, io.SeekStart); err != nil {
					logger.SpotifyStreamer("Failed to seek to %d: %v", start, err)
				} else {
					contentLength := end - start + 1
					w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, totalSize))
					w.Header().Set("Content-Length", fmt.Sprintf("%d", contentLength))
					w.WriteHeader(http.StatusPartialContent)

					logger.SpotifyStreamer("Range request: bytes=%d-%d/%d", start, end, totalSize)

					// Stream the requested range
					written, err := io.CopyN(w, stream, contentLength)
					if err != nil && err != io.EOF {
						logger.SpotifyStreamer("Range stream ended: %v (wrote %d/%d bytes)", err, written, contentLength)
					} else {
						logger.SpotifyStreamer("Range stream completed: %d bytes", written)
					}
					return
				}
			}
		}
	}

	// Full content response
	if totalSize > 0 {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", totalSize))
	}

	logger.SpotifyStreamer("Starting full audio stream for track: %s", spotifyID)

	// Stream the audio data to the response
	written, err := io.Copy(w, stream)
	if err != nil {
		// Client may have disconnected - this is normal
		logger.SpotifyStreamer("Stream ended: %v (wrote %d bytes)", err, written)
	} else {
		logger.SpotifyStreamer("Stream completed: %d bytes", written)
	}
}
