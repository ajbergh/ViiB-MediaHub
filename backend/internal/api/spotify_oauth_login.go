// Handles desktop Spotify PKCE sign-in through the system browser.
package api

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
)

var spotifyPublicClientID = regexp.MustCompile(`^[a-fA-F0-9]{32}$`)

type spotifyOAuthAttempt struct {
	ctx      context.Context
	clientID string
	verifier string
	state    string
}

// WithSpotifyOAuthCallback registers the desktop redirect URI and handles its
// browser callback before passing other requests to the standard server.
func (a *API) WithSpotifyOAuthCallback(origin string, next http.Handler) http.Handler {
	a.spotifyOAuthCallback = origin + "/callback"
	host := strings.TrimPrefix(origin, "http://")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/callback" {
			next.ServeHTTP(w, r)
			return
		}
		if r.Host != host || r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
		if result := r.URL.Query().Get("spotify"); result != "" && r.URL.Query().Get("state") == "" {
			message := "Spotify sign-in did not complete. Return to ViiB to try again."
			if result == "connected" {
				message = "Spotify is connected. You can close this tab and return to ViiB."
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = fmt.Fprintf(w, "<!doctype html><html lang=en><meta charset=utf-8><title>Spotify sign-in</title><main><h1>Spotify sign-in</h1><p>%s</p></main></html>", message)
			return
		}
		result := "failed"
		if a.finishSpotifyOAuthLogin(r.URL.Query()) {
			result = "connected"
		}
		http.Redirect(w, r, "/callback?spotify="+result, http.StatusSeeOther)
	})
}

// startSpotifyOAuthLogin creates a timed PKCE attempt and returns its authorization URL.
// The verifier stays in memory in the backend.
func (a *API) startSpotifyOAuthLogin(w http.ResponseWriter, r *http.Request) {
	if a.spotifyOAuthCallback == "" || !spotifyLoginRequestAllowed(r) {
		respondError(w, http.StatusForbidden, "System browser sign-in is available in the ViiB desktop app.")
		return
	}
	var request struct {
		ClientID string `json:"clientId"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&request); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid Spotify sign-in request.")
		return
	}
	clientID := strings.TrimSpace(request.ClientID)
	if !spotifyPublicClientID.MatchString(clientID) {
		respondError(w, http.StatusBadRequest, "Enter the Client ID from your Spotify Developer Dashboard.")
		return
	}
	var entropy [64]byte
	if _, err := rand.Read(entropy[:]); err != nil {
		respondError(w, http.StatusInternalServerError, "Could not generate Spotify PKCE data")
		return
	}
	verifier := base64.RawURLEncoding.EncodeToString(entropy[:32])
	state := base64.RawURLEncoding.EncodeToString(entropy[32:])
	challenge := sha256.Sum256([]byte(verifier))
	s := a.spotifyTokens()
	s.login.mu.Lock()
	if s.login.closed {
		s.login.mu.Unlock()
		respondError(w, http.StatusServiceUnavailable, "Spotify sign-in is shutting down.")
		return
	}
	if old := s.login.operation; old != nil && old.status.State == "pending" {
		old.cancel()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	status := spotifyBrowserLoginStatus{ID: uuid.NewString(), State: "pending"}
	s.mu.RLock()
	op := &spotifyBrowserLogin{status: status, cancel: cancel, account: s.lifetime,
		oauth: &spotifyOAuthAttempt{ctx: ctx, clientID: clientID, verifier: verifier, state: state}}
	s.mu.RUnlock()
	s.login.operation = op
	s.login.mu.Unlock()

	context.AfterFunc(ctx, func() {
		s.login.mu.Lock()
		defer s.login.mu.Unlock()
		op.oauth = nil
		if s.login.operation == op && op.status.State == "pending" {
			op.status.State = "failed"
			op.status.Message = "Sign-in timed out. Try again."
		}
	})
	params := url.Values{
		"client_id": {clientID}, "response_type": {"code"}, "redirect_uri": {a.spotifyOAuthCallback},
		"code_challenge_method": {"S256"}, "code_challenge": {base64.RawURLEncoding.EncodeToString(challenge[:])},
		"state": {state}, "scope": {"user-read-private user-library-read user-top-read user-read-playback-state user-modify-playback-state playlist-read-private playlist-read-collaborative user-read-recently-played"},
	}
	w.Header().Set("Cache-Control", "no-store")
	respondJSON(w, struct {
		spotifyBrowserLoginStatus
		AuthURL string `json:"authUrl"`
	}{status, "https://accounts.spotify.com/authorize?" + params.Encode()})
}

// finishSpotifyOAuthLogin validates and consumes the callback state, then installs
// exchanged credentials only while the attempt still owns the connection.
func (a *API) finishSpotifyOAuthLogin(query url.Values) bool {
	s := a.spotifyTokens()
	s.login.mu.Lock()
	op := s.login.operation
	if s.login.closed || op == nil || op.oauth == nil || op.status.State != "pending" ||
		op.oauth.ctx.Err() != nil || subtle.ConstantTimeCompare([]byte(query.Get("state")), []byte(op.oauth.state)) != 1 {
		s.login.mu.Unlock()
		return false
	}
	attempt := op.oauth
	op.oauth = nil // Consume the callback before contacting Spotify.
	s.login.wg.Add(1)
	s.login.mu.Unlock()
	defer s.login.wg.Done()
	defer op.cancel()

	message := "Spotify authorization was canceled or declined. Try again from ViiB."
	if code := query.Get("code"); code != "" && query.Get("error") == "" {
		message = "Spotify could not complete sign-in. Check the app's Client ID and account access, then try again."
		raw, err := exchangeSpotifyOAuthCode(attempt, code, a.spotifyOAuthCallback)
		if err == nil {
			err = s.replaceOAuthCredentialsGuarded(raw, s.browserLoginCommit(attempt.ctx, op))
			if err == nil {
				return true
			}
			message = "Could not save the Spotify connection. Return to ViiB and try again."
		}
		if errors.Is(err, errSpotifyOAuthRateLimited) {
			message = "Spotify is rate limiting sign-in. Try again later."
		}
	}
	s.login.mu.Lock()
	defer s.login.mu.Unlock()
	if s.login.operation == op && op.status.State == "pending" {
		op.status.State = "failed"
		op.status.Message = message
	}
	return false
}

var errSpotifyOAuthRateLimited = errors.New("rate limited during Spotify sign-in")

// exchangeSpotifyOAuthCode exchanges a single-use code and serializes the resulting
// Web API credentials for encrypted persistence.
func exchangeSpotifyOAuthCode(attempt *spotifyOAuthAttempt, code, redirectURI string) (string, error) {
	params := url.Values{
		"grant_type": {"authorization_code"}, "code": {code}, "redirect_uri": {redirectURI},
		"client_id": {attempt.clientID}, "code_verifier": {attempt.verifier},
	}
	req, err := http.NewRequestWithContext(attempt.ctx, http.MethodPost, spotifyTokenEndpoint, strings.NewReader(params.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	client := http.Client{Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	res, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusTooManyRequests {
		return "", errSpotifyOAuthRateLimited
	}
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status %d from Spotify token exchange", res.StatusCode)
	}
	var token struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 64<<10)).Decode(&token); err != nil {
		return "", err
	}
	if token.AccessToken == "" || token.RefreshToken == "" || token.ExpiresIn <= 0 {
		return "", errors.New("incomplete credentials from Spotify token exchange")
	}
	raw, err := json.Marshal(SpotifyCredentials{
		ClientId: attempt.clientID, AccessToken: token.AccessToken, RefreshToken: token.RefreshToken,
		Expiry: time.Now().Add(time.Duration(token.ExpiresIn) * time.Second).UnixMilli(), RedirectURI: redirectURI, DesktopPKCE: true,
	})
	return string(raw), err
}
