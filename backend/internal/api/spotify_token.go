package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/db"
)

var (
	spotifyTokenRefreshMu sync.Mutex
	spotifyTokenEndpoint  = "https://accounts.spotify.com/api/token"
)

func readSpotifyCredentials(database *db.DB) (SpotifyCredentials, error) {
	raw, err := database.GetSetting("spotify_credentials")
	if err != nil || raw == "" {
		return SpotifyCredentials{}, fmt.Errorf("spotify credentials not configured")
	}
	var credentials SpotifyCredentials
	if err := json.Unmarshal([]byte(raw), &credentials); err != nil {
		return SpotifyCredentials{}, fmt.Errorf("parse spotify credentials: %w", err)
	}
	if credentials.AccessToken == "" {
		return SpotifyCredentials{}, fmt.Errorf("spotify access token missing")
	}
	return credentials, nil
}

func refreshSpotifyCredentials(ctx context.Context, database *db.DB, force bool) (SpotifyCredentials, error) {
	spotifyTokenRefreshMu.Lock()
	defer spotifyTokenRefreshMu.Unlock()

	credentials, err := readSpotifyCredentials(database)
	if err != nil {
		return SpotifyCredentials{}, err
	}
	if !force && (credentials.Expiry == 0 || time.Now().Add(5*time.Minute).Before(time.UnixMilli(credentials.Expiry))) {
		return credentials, nil
	}
	if credentials.RefreshToken == "" || credentials.ClientId == "" {
		return SpotifyCredentials{}, fmt.Errorf("spotify re-authentication required")
	}

	values := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {credentials.RefreshToken},
		"client_id":     {credentials.ClientId},
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, spotifyTokenEndpoint, strings.NewReader(values.Encode()))
	if err != nil {
		return SpotifyCredentials{}, err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := (&http.Client{Timeout: 15 * time.Second}).Do(request)
	if err != nil {
		return SpotifyCredentials{}, fmt.Errorf("refresh spotify token: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return SpotifyCredentials{}, fmt.Errorf("spotify token refresh returned %d", response.StatusCode)
	}

	var token struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
	}
	if err := json.NewDecoder(response.Body).Decode(&token); err != nil {
		return SpotifyCredentials{}, fmt.Errorf("decode spotify token response: %w", err)
	}
	if token.AccessToken == "" || token.ExpiresIn <= 0 {
		return SpotifyCredentials{}, fmt.Errorf("spotify token refresh returned incomplete credentials")
	}

	credentials.AccessToken = token.AccessToken
	if token.RefreshToken != "" {
		credentials.RefreshToken = token.RefreshToken
	}
	credentials.Expiry = time.Now().Add(time.Duration(token.ExpiresIn) * time.Second).UnixMilli()
	encoded, err := json.Marshal(credentials)
	if err != nil {
		return SpotifyCredentials{}, err
	}
	if err := database.SetSetting("spotify_credentials", string(encoded)); err != nil {
		return SpotifyCredentials{}, fmt.Errorf("persist refreshed spotify token: %w", err)
	}
	return credentials, nil
}

func loadValidSpotifyCredentials(ctx context.Context, database *db.DB) (SpotifyCredentials, error) {
	return refreshSpotifyCredentials(ctx, database, false)
}

func (a *API) validSpotifyAccessToken(ctx context.Context) (string, error) {
	credentials, err := loadValidSpotifyCredentials(ctx, a.db)
	if err != nil {
		return "", err
	}
	return credentials.AccessToken, nil
}

// doSpotifyRequest performs an authenticated request and retries exactly once
// after a 401 with a forced PKCE refresh. Body bytes are replayable, which keeps
// POST/PUT proxy requests safe to retry without reusing a consumed stream.
func (a *API) doSpotifyRequest(ctx context.Context, method, target string, body []byte, contentType string) (*http.Response, error) {
	credentials, err := loadValidSpotifyCredentials(ctx, a.db)
	if err != nil {
		return nil, err
	}

	client := &http.Client{Timeout: 30 * time.Second}
	for attempt := 0; attempt < 2; attempt++ {
		request, err := http.NewRequestWithContext(ctx, method, target, bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		request.Header.Set("Authorization", "Bearer "+credentials.AccessToken)
		if contentType != "" {
			request.Header.Set("Content-Type", contentType)
		}

		response, err := client.Do(request)
		if err != nil {
			return nil, err
		}
		if response.StatusCode != http.StatusUnauthorized || attempt == 1 {
			return response, nil
		}
		response.Body.Close()

		credentials, err = refreshSpotifyCredentials(ctx, a.db, true)
		if err != nil {
			return nil, err
		}
	}
	return nil, fmt.Errorf("spotify request retry exhausted")
}
