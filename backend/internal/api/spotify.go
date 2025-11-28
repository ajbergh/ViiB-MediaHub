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

	"github.com/go-chi/chi/v5"
)

// SpotifyCredentials represents OAuth 2.0 credentials for Spotify Web API.
// These credentials are stored in the database and used for:
//   - Web API calls (search, metadata, user profile)
//   - librespot authentication for downloads
//
// The access token is refreshed by the frontend when it expires.
type SpotifyCredentials struct {
	ClientId     string `json:"clientId"`     // OAuth client ID from Spotify Developer Dashboard
	ClientSecret string `json:"clientSecret"` // OAuth client secret (stored server-side for security)
	AccessToken  string `json:"accessToken"`  // OAuth access token (expires in 1 hour)
	RefreshToken string `json:"refreshToken"` // OAuth refresh token (used to get new access tokens)
	Expiry       int64  `json:"expiry"`       // Unix timestamp when access token expires
}

// saveSpotifyCredentials saves OAuth credentials to the database.
// Called by the frontend after successful OAuth authorization.
// Credentials are stored as JSON in the "spotify_credentials" setting.
//
// The access token is used for:
//   - Spotify Web API calls (proxied through this backend)
//   - librespot-go authentication for direct downloads
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
	// Get stored credentials for authorization
	val, err := a.db.GetSetting("spotify_credentials")
	if err != nil || val == "" {
		respondError(w, http.StatusUnauthorized, "Spotify credentials not configured")
		return
	}

	var creds SpotifyCredentials
	if err := json.Unmarshal([]byte(val), &creds); err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to parse credentials")
		return
	}

	// Get query parameters
	query := r.URL.Query().Get("q")
	types := r.URL.Query().Get("type")
	limit := r.URL.Query().Get("limit")
	offset := r.URL.Query().Get("offset")

	if query == "" {
		respondError(w, http.StatusBadRequest, "Missing query parameter")
		return
	}

	// Build Spotify API URL
	spotifyURL := "https://api.spotify.com/v1/search?q=" + query
	if types != "" {
		spotifyURL += "&type=" + types
	}
	if limit != "" {
		spotifyURL += "&limit=" + limit
	}
	if offset != "" {
		spotifyURL += "&offset=" + offset
	}

	// Make request to Spotify API
	req, err := http.NewRequest("GET", spotifyURL, nil)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to create request")
		return
	}

	req.Header.Set("Authorization", "Bearer "+creds.AccessToken)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to fetch from Spotify")
		return
	}
	defer resp.Body.Close()

	// Forward the response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	json.NewDecoder(resp.Body).Decode(&json.RawMessage{})
	// Better: copy the body directly
	var result json.RawMessage
	json.NewDecoder(resp.Body).Decode(&result)
	json.NewEncoder(w).Encode(result)
}

// spotifyGetUserProfile proxies user profile requests to Spotify Web API.
// Returns information about the authenticated user including display name,
// follower count, and profile images.
//
// GET /api/spotify/me
// Response: Spotify user profile JSON (proxied from /v1/me)
func (a *API) spotifyGetUserProfile(w http.ResponseWriter, r *http.Request) {
	val, err := a.db.GetSetting("spotify_credentials")
	if err != nil || val == "" {
		respondError(w, http.StatusUnauthorized, "Spotify credentials not configured")
		return
	}

	var creds SpotifyCredentials
	if err := json.Unmarshal([]byte(val), &creds); err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to parse credentials")
		return
	}

	req, err := http.NewRequest("GET", "https://api.spotify.com/v1/me", nil)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to create request")
		return
	}

	req.Header.Set("Authorization", "Bearer "+creds.AccessToken)

	client := &http.Client{}
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
	val, err := a.db.GetSetting("spotify_credentials")
	if err != nil || val == "" {
		respondError(w, http.StatusUnauthorized, "Spotify credentials not configured")
		return
	}

	var creds SpotifyCredentials
	if err := json.Unmarshal([]byte(val), &creds); err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to parse credentials")
		return
	}

	// Get the path after /api/spotify/proxy/
	path := r.URL.Query().Get("path")
	if path == "" {
		respondError(w, http.StatusBadRequest, "Missing path parameter")
		return
	}

	// Build full URL
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

	req.Header.Set("Authorization", "Bearer "+creds.AccessToken)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
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

func (a *API) getDownloads(w http.ResponseWriter, r *http.Request) {
	log.Printf("getDownloads called")
	downloads, err := a.downloadManager.GetAllDownloads()
	if err != nil {
		log.Printf("getDownloads error: %v", err)
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to fetch downloads: %v", err))
		return
	}
	log.Printf("getDownloads returning %d downloads", len(downloads))

	respondJSON(w, downloads)
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

func (a *API) clearCompletedDownloads(w http.ResponseWriter, r *http.Request) {
	count, err := a.downloadManager.ClearCompletedDownloads()
	if err != nil {
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to clear completed downloads: %v", err))
		return
	}

	respondJSON(w, map[string]interface{}{"status": "cleared", "count": count})
}

func (a *API) downloadProgressSSE(w http.ResponseWriter, r *http.Request) {
	log.Printf("downloadProgressSSE: Connection started")
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	flusher, ok := w.(http.Flusher)
	if !ok {
		log.Printf("downloadProgressSSE: Streaming not supported")
		respondError(w, http.StatusInternalServerError, "Streaming not supported")
		return
	}

	log.Printf("downloadProgressSSE: Getting progress channel")
	progressChan := a.downloadManager.GetProgressChan()
	if progressChan == nil {
		log.Printf("downloadProgressSSE: Progress channel is nil!")
		respondError(w, http.StatusInternalServerError, "Progress channel not available")
		return
	}

	log.Printf("downloadProgressSSE: Entering event loop")
	for {
		select {
		case <-r.Context().Done():
			log.Printf("downloadProgressSSE: Client disconnected")
			return
		case progress := <-progressChan:
			log.Printf("downloadProgressSSE: Sending progress update for %s", progress.DownloadID)
			data, _ := json.Marshal(progress)
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}
	}
}

// Helper functions to fetch Spotify metadata

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

	client := &http.Client{}
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

	client := &http.Client{}
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

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", "", err
	}
	defer resp.Body.Close()

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
