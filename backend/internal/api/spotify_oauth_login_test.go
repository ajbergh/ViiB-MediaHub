package api

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
)

func TestDesktopSpotifyOAuthCallback(t *testing.T) {
	a, _, _ := fixtureCookieRuntime(t)
	const origin = "http://127.0.0.1:34115"
	const clientID = "0123456789abcdef0123456789abcdef"
	handler := a.WithSpotifyOAuthCallback(origin, http.NotFoundHandler())
	var exchangeCount atomic.Int32
	var expectedChallenge atomic.Value
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		exchangeCount.Add(1)
		if err := r.ParseForm(); err != nil {
			t.Error(err)
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}
		verifierHash := sha256.Sum256([]byte(r.Form.Get("code_verifier")))
		if r.Form.Get("grant_type") != "authorization_code" || r.Form.Get("code") != "one-time-code" ||
			r.Form.Get("client_id") != clientID || r.Form.Get("redirect_uri") != origin+"/callback" ||
			base64.RawURLEncoding.EncodeToString(verifierHash[:]) != expectedChallenge.Load().(string) {
			t.Error("token exchange did not use the original PKCE challenge and redirect")
			http.Error(w, "bad code exchange", http.StatusBadRequest)
			return
		}
		_, _ = io.WriteString(w, `{"access_token":"test-access","refresh_token":"test-refresh","expires_in":3600}`)
	}))
	defer tokenServer.Close()
	oldEndpoint := spotifyTokenEndpoint
	spotifyTokenEndpoint = tokenServer.URL
	t.Cleanup(func() { spotifyTokenEndpoint = oldEndpoint })

	start := func() (string, string) {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, origin+"/api/spotify/auth/oauth-login", strings.NewReader(`{"clientId":"`+clientID+`"}`))
		req.Header.Set("Content-Type", "application/json")
		res := httptest.NewRecorder()
		a.startSpotifyOAuthLogin(res, req)
		var login struct {
			ID      string `json:"id"`
			AuthURL string `json:"authUrl"`
		}
		if err := json.Unmarshal(res.Body.Bytes(), &login); err != nil || res.Code != http.StatusOK || login.ID == "" {
			t.Fatalf("start login: HTTP %d, %s", res.Code, res.Body)
		}
		authURL, err := url.Parse(login.AuthURL)
		if err != nil || authURL.Host != "accounts.spotify.com" {
			t.Fatalf("unexpected authorization URL: %q", login.AuthURL)
		}
		query := authURL.Query()
		if query.Get("code_challenge_method") != "S256" || query.Get("redirect_uri") != origin+"/callback" ||
			query.Get("state") == "" || query.Get("state") == login.ID {
			t.Fatal("authorization URL is missing the callback, independent state, or PKCE method")
		}
		expectedChallenge.Store(query.Get("code_challenge"))
		return login.ID, query.Get("state")
	}
	callback := func(state string) string {
		t.Helper()
		res := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, origin+"/callback?code=one-time-code&state="+url.QueryEscape(state), nil)
		req.Header.Set("Sec-Fetch-Site", "cross-site")
		handler.ServeHTTP(res, req)
		if res.Code != http.StatusSeeOther || strings.Contains(res.Header().Get("Location"), "one-time-code") {
			t.Fatalf("callback returned HTTP %d, location %q", res.Code, res.Header().Get("Location"))
		}
		return res.Header().Get("Location")
	}

	canceled, staleState := start()
	a.spotifyAuth.cancelBrowserLogin(canceled, false)
	if location := callback(staleState); location != "/callback?spotify=failed" {
		t.Fatalf("canceled login returned %q", location)
	}
	id, state := start()
	if location := callback(id); location != "/callback?spotify=failed" || exchangeCount.Load() != 0 {
		t.Fatal("polling ID was accepted as OAuth state")
	}
	if location := callback(state); location != "/callback?spotify=connected" {
		t.Fatalf("valid callback returned %q", location)
	}
	if location := callback(state); location != "/callback?spotify=failed" || exchangeCount.Load() != 1 {
		t.Fatal("authorization code could be replayed")
	}
	if status, ok := a.spotifyAuth.browserLoginStatus(id); !ok || status.State != "connected" {
		t.Fatalf("desktop poll returned %+v", status)
	}
	if creds, err := readSpotifyCredentials(a.db); err != nil || creds.RefreshToken != "test-refresh" || !creds.DesktopPKCE {
		t.Fatal("desktop credentials were not saved")
	}
	res := httptest.NewRecorder()
	a.getSpotifyAuthStatus(res, httptest.NewRequest(http.MethodGet, origin+"/api/spotify/auth/status", nil))
	var capability struct {
		spotifySessionStatus
		CanStream   bool `json:"canStream"`
		CanDownload bool `json:"canDownload"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &capability); err != nil || !capability.Connected ||
		!capability.WebAPIOnly || capability.CanStream || capability.CanDownload {
		t.Fatalf("unexpected desktop OAuth capabilities: %s", res.Body)
	}
	res = httptest.NewRecorder()
	if a.requireSpotifyMedia(res, httptest.NewRequest(http.MethodPost, origin+"/api/spotify/download/track", nil)) ||
		res.Code != http.StatusForbidden {
		t.Fatal("Web API sign-in was admitted to the Web Player download path")
	}
	res = httptest.NewRecorder()
	a.getSpotifyCredentials(res, httptest.NewRequest(http.MethodGet, origin+"/api/spotify/credentials", nil))
	if strings.Contains(res.Body.String(), "test-refresh") || strings.Contains(res.Body.String(), "test-access") {
		t.Fatal("desktop credentials were exposed through the compatibility route")
	}
}
