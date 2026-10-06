// Routes cookie catalog requests and legacy OAuth token refresh through shared request and cooldown handling.
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/db"
	spotifyauth "github.com/ajbergh/viib-mediahub/internal/spotify/auth"
	"github.com/ajbergh/viib-mediahub/internal/spotify/catalog"
)

type spotifyResponseBody struct {
	io.ReadCloser
	end context.CancelFunc
}

func (b *spotifyResponseBody) Close() error { defer b.end(); return b.ReadCloser.Close() }

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
	if !force && credentials.Expiry > 0 && time.Now().Add(5*time.Minute).Before(time.UnixMilli(credentials.Expiry)) {
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

// doSpotifyRequest performs an authenticated request and retries exactly once
// after a 401 through the selected provider. Body bytes are replayable, which keeps
// POST/PUT proxy requests safe to retry without reusing a consumed stream.
func (a *API) doSpotifyRequest(ctx context.Context, method, target string, body []byte, contentType string) (*http.Response, error) {
	manager := a.spotifyTokens()
	ctx, endRequest := manager.requestContext(ctx)
	if ctx.Value(spotifyOwnerBootstrapKey{}) != true {
		if err := manager.ensureMetadataOwner(ctx); err != nil {
			endRequest()
			return nil, err
		}
	}
	// The response body owns cancellation once a request succeeds.
	finished := false
	defer func() {
		if !finished {
			endRequest()
		}
	}()
	status := manager.status()
	if status.Provider == "webplayer" {
		u, err := url.Parse(target)
		if err != nil || u.Scheme != "https" || u.Host != "api.spotify.com" || u.User != nil {
			return nil, spotifyauth.ErrDisabled
		}
	}
	cookieMode := status.Provider == "webplayer"
	var catalogTarget *url.URL
	if cookieMode && method == http.MethodGet {
		parsed, _ := url.Parse(target)
		_, _, albumPath := catalog.AlbumPath(parsed.Path)
		_, _, artistPath := catalog.ArtistPath(parsed.Path)
		_, artistAlbumsPath := catalog.ArtistAlbumsPath(parsed.Path)
		_, _, playlistPath := catalog.PlaylistPath(parsed.Path)
		_, libraryPath := catalog.LibraryPath(parsed.Path)
		_, _, trackPath := catalog.TrackPath(parsed.Path)
		if parsed.Path == "/v1/me" || parsed.Path == "/v1/search" || albumPath || artistPath || artistAlbumsPath || playlistPath || libraryPath || trackPath {
			catalogTarget = parsed
		}
	}
	if cookieMode {
		if status.AuthRequired {
			return nil, spotifyauth.ErrAuthenticationRequired
		}
		var release func()
		var err error
		if catalogTarget != nil {
			release, err = manager.acquireCatalog(ctx, catalogTarget.Path == "/v1/search")
		} else {
			release, err = manager.acquireWebAPI(ctx)
		}
		if err != nil {
			return nil, err
		}
		defer release()
	}
	token, err := manager.Token(ctx, spotifyauth.WebAPI)
	if err != nil {
		return nil, err
	}

	if catalogTarget != nil {
		response, err := a.cookieSpotifyCatalog(ctx, manager, token, catalogTarget)
		if err != nil {
			return nil, err
		}
		response.Body = &spotifyResponseBody{ReadCloser: response.Body, end: endRequest}
		finished = true
		return response, nil
	}
	client := &http.Client{Timeout: 30 * time.Second}
	if a.spotifyHTTPClient != nil {
		copyClient := *a.spotifyHTTPClient
		client = &copyClient
	}
	client.Jar = nil
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	refreshed := false
	rateRetries := 0
	for {
		request, err := http.NewRequestWithContext(ctx, method, target, bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		request.Header.Set("Authorization", "Bearer "+token.Bearer())
		if contentType != "" {
			request.Header.Set("Content-Type", contentType)
		}

		if cookieMode {
			if err := manager.checkWebAPICooldown(ctx); err != nil {
				return nil, err
			}
		}
		response, err := client.Do(request)
		if err != nil {
			return nil, err
		}
		if response.StatusCode == http.StatusUnauthorized && !refreshed {
			response.Body.Close()
			token, err = manager.Refresh(ctx, spotifyauth.WebAPI, token)
			if err != nil {
				return nil, err
			}
			refreshed = true
			continue
		}
		if cookieMode && response.StatusCode == http.StatusUnauthorized {
			manager.rejectWebAPISession(ctx)
			response.Body.Close()
			return nil, spotifyauth.ErrAuthenticationRequired
		}
		if cookieMode && response.StatusCode == http.StatusTooManyRequests {
			if err := manager.recordWebAPICooldown(response.Header.Get("Retry-After")); err != nil {
				response.Body.Close()
				return nil, &spotifyauth.WebPlayerHTTPError{Status: 429, RetryAfter: manager.webAPICooldownRemaining()}
			}
			response.Header.Set("Retry-After", strconv.FormatInt(int64((manager.webAPICooldownRemaining()+time.Second-1)/time.Second), 10))
		}
		if response.StatusCode == http.StatusTooManyRequests && !cookieMode && rateRetries < 2 && ctx.Value(spotifyOwnerBootstrapKey{}) != true {
			delaySeconds, _ := strconv.Atoi(response.Header.Get("Retry-After"))
			if delaySeconds < 1 {
				delaySeconds = 1
			}
			if delaySeconds > 30 {
				delaySeconds = 30
			}
			response.Body.Close()
			timer := time.NewTimer(time.Duration(delaySeconds) * time.Second)
			select {
			case <-ctx.Done():
				timer.Stop()
				return nil, ctx.Err()
			case <-timer.C:
			}
			rateRetries++
			continue
		}
		if !cookieMode && method == http.MethodGet && response.StatusCode == http.StatusOK {
			parsed, parseErr := url.Parse(target)
			if parseErr == nil {
				a.captureOAuthCatalogResponse(ctx, manager, parsed, response)
			}
		}
		response.Body = &spotifyResponseBody{ReadCloser: response.Body, end: endRequest}
		finished = true
		return response, nil
	}
}

// spotifyOAuthManager preserves the existing OAuth owner and persistence.
// Internal analysis remains disabled in the application until explicitly composed.
func spotifyOAuthManager(database *db.DB) *spotifyauth.Manager {
	load := func(ctx context.Context, force bool) (spotifyauth.Token, error) {
		credentials, err := refreshSpotifyCredentials(ctx, database, force)
		if err != nil {
			return spotifyauth.Token{}, err
		}
		return spotifyauth.NewToken(credentials.AccessToken, spotifyauth.OAuth, time.UnixMilli(credentials.Expiry), 0), nil
	}
	return spotifyauth.NewManager(spotifyauth.ProviderFuncs{
		Load:  func(ctx context.Context) (spotifyauth.Token, error) { return load(ctx, false) },
		Renew: func(ctx context.Context, _ spotifyauth.Token) (spotifyauth.Token, error) { return load(ctx, true) },
	}, nil)
}
