// Tests proxy request bounds, response contracts, failures, and cancellation.
package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/db"
	"github.com/go-chi/chi/v5"
)

func connectedProxyFixture(t *testing.T) *API {
	t.Helper()
	a, _, _ := fixtureCookieRuntime(t)
	if err := a.spotifyAuth.connect(context.Background(), "fixture-cookie"); err != nil {
		t.Fatal(err)
	}
	return a
}

func TestSpotifyProxyResponseContract(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
		want   int
		exact  string
	}{
		{"valid", 200, `{"items":[]}`, 200, `{"items":[]}`},
		{"malformed", 200, `{"cookie":"fixture-cookie"`, 502, ""},
		{"html", 200, `<html>fixture-cookie</html>`, 502, ""},
		{"empty", 200, "", 502, ""},
		{"trailing", 200, `{} {}`, 502, ""},
		{"oversized", 200, `"` + strings.Repeat("x", spotifyProxyResponseLimit) + `"`, 502, ""},
		{"no-content", 204, "", 204, ""},
		{"forbidden", 403, `{"error":"fixture-cookie fixture-bearer-1"}`, 403, ""},
		{"missing", 404, `fixture-cookie`, 404, ""},
		{"rate-limit", 429, `fixture-bearer-1`, 429, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := connectedProxyFixture(t)
			calls := 0
			a.spotifyHTTPClient = &http.Client{Transport: sessionTransport(func(r *http.Request) (*http.Response, error) {
				calls++
				if r.Header.Get("Cookie") != "" || r.URL.Host != "api.spotify.com" {
					t.Fatal("credential origin failure")
				}
				return &http.Response{StatusCode: tc.status, Header: http.Header{"Retry-After": []string{"31"}}, Body: io.NopCloser(strings.NewReader(tc.body))}, nil
			})}
			w := httptest.NewRecorder()
			a.spotifyProxy(w, httptest.NewRequest("GET", "/proxy?path=me/player", nil))
			if w.Code != tc.want || calls != 1 {
				t.Fatalf("status %d want %d calls %d", w.Code, tc.want, calls)
			}
			if tc.exact != "" && w.Body.String() != tc.exact {
				t.Fatal("valid response changed")
			}
			if tc.want == 204 && w.Body.Len() != 0 {
				t.Fatal("204 has body")
			}
			for _, secret := range []string{"fixture-cookie", "fixture-bearer"} {
				if strings.Contains(w.Body.String(), secret) {
					t.Fatal("provider material exposed")
				}
			}
			if tc.status == 429 && w.Header().Get("Retry-After") != "31" {
				t.Fatal("lost retry delay")
			}
			if !a.spotifyAuth.status().Connected {
				t.Fatal("non-auth error disconnected session")
			}
		})
	}
}
func TestSpotifyProxyRejectsOversizedRequestBeforeDispatch(t *testing.T) {
	a := connectedProxyFixture(t)
	a.spotifyHTTPClient = &http.Client{Transport: sessionTransport(func(r *http.Request) (*http.Response, error) {
		t.Fatal("oversized request dispatched")
		return nil, fmt.Errorf("unexpected")
	})}
	w := httptest.NewRecorder()
	a.spotifyProxy(w, httptest.NewRequest("POST", "/proxy?path=me/player", strings.NewReader(strings.Repeat("x", spotifyProxyRequestLimit+1))))
	if w.Code != 413 {
		t.Fatalf("status %d", w.Code)
	}
}
func TestSpotifyProxyCancelsActiveRequest(t *testing.T) {
	a := connectedProxyFixture(t)
	entered := make(chan struct{})
	cancelled := make(chan struct{})
	a.spotifyHTTPClient = &http.Client{Transport: sessionTransport(func(r *http.Request) (*http.Response, error) {
		close(entered)
		<-r.Context().Done()
		close(cancelled)
		return nil, r.Context().Err()
	})}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w := httptest.NewRecorder()
	returned := make(chan struct{})
	go func() {
		a.spotifyProxy(w, httptest.NewRequest("GET", "/proxy?path=me/player", nil).WithContext(ctx))
		close(returned)
	}()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("request not entered")
	}
	cancel()
	select {
	case <-returned:
	case <-time.After(3 * time.Second):
		t.Fatal("proxy did not cancel")
	}
	<-cancelled
	if w.Code != 503 || strings.Contains(w.Body.String(), "fixture-") {
		t.Fatal("cancellation returned success or credentials")
	}
}

func TestSpotifyAccountReadUsesFixedResources(t *testing.T) {
	database, err := db.New(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	credentials, err := json.Marshal(SpotifyCredentials{
		ClientId: "fixture-client", AccessToken: "fixture-access", Expiry: time.Now().Add(time.Hour).UnixMilli(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.SetSetting("spotify_credentials", string(credentials)); err != nil {
		t.Fatal(err)
	}
	a := &API{db: database}
	calls := 0
	a.spotifyHTTPClient = &http.Client{Transport: sessionTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/v1/me" {
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"id":"fixture-owner"}`))}, nil
		}
		calls++
		if r.Method != http.MethodGet || r.URL.Host != "api.spotify.com" || r.URL.Path != "/v1/me/top/artists" ||
			r.URL.RawQuery != "limit=20&offset=20&time_range=short_term" {
			t.Fatalf("unexpected Spotify request: %s %s", r.Method, r.URL)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"items":[]}`))}, nil
	})}
	router := chi.NewRouter()
	router.Get("/account/{view}", a.spotifyAccountRead)
	router.Get("/account", a.spotifyAccountCapabilities)
	for _, tc := range []struct {
		url    string
		status int
	}{
		{"/account/top-artists?path=me/player&offset=20&time_range=short_term", 200},
		{"/account/top-tracks?time_range=invalid", 400},
		{"/account/liked?offset=-1", 400},
		{"/account/unknown", 404},
	} {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, tc.url, nil))
		if w.Code != tc.status {
			t.Fatalf("%s: got %d, want %d", tc.url, w.Code, tc.status)
		}
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/account", nil))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"oauthViews":true`) {
		t.Fatalf("unexpected OAuth capabilities: %s", w.Body.String())
	}
	if calls != 1 {
		t.Fatalf("got %d upstream requests, want one", calls)
	}
}

func TestWebPlayerAccountViewsNeverReachOAuthRoutes(t *testing.T) {
	a := connectedProxyFixture(t)
	a.spotifyHTTPClient = &http.Client{Transport: sessionTransport(func(r *http.Request) (*http.Response, error) {
		t.Fatalf("OAuth-only view reached upstream: %s", r.URL.Path)
		return nil, nil
	})}
	router := a.Routes()
	for view, path := range spotifyAccountPaths {
		if view == "liked" {
			continue
		}
		for _, endpoint := range []string{"/spotify/account/" + view, "/spotify/proxy?path=" + path} {
			w := httptest.NewRecorder()
			router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, endpoint, nil))
			if w.Code != http.StatusForbidden || !strings.Contains(w.Body.String(), "requires an OAuth connection") {
				t.Fatalf("%s: status %d: %s", endpoint, w.Code, w.Body.String())
			}
		}
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/spotify/account", nil))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"oauthViews":false`) {
		t.Fatalf("unexpected Web Player capabilities: %s", w.Body.String())
	}
	if !a.spotifyAuth.status().Connected || a.spotifyAuth.webAPICooldownRemaining() > 0 {
		t.Fatal("unsupported account views retired or rate-limited the active session")
	}
}
