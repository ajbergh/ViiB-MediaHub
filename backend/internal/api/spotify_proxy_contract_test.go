// Tests proxy request bounds, response contracts, failures, and cancellation.
package api

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
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
			a.spotifyProxy(w, httptest.NewRequest("GET", "/proxy?path=me/player/recently-played", nil))
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
		a.spotifyProxy(w, httptest.NewRequest("GET", "/proxy?path=me/player/recently-played", nil).WithContext(ctx))
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
