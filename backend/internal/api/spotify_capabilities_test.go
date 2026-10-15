package api

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSpotifyProductClassification(t *testing.T) {
	for _, tc := range []struct{ catalogue, product, want string }{
		{"free", "free", "free"}, {"open", "free", "free"},
		{"premium", "premium", "premium"}, {"premium", "", "premium"},
		{"", "free", "free"}, {"", "premium", "premium"},
		{"", "", "unknown"}, {"other", "premium", "unknown"},
		{"premium", "free", "unknown"}, {"free", "premium", "unknown"},
	} {
		if got := spotifyProduct(tc.catalogue, tc.product); got != tc.want {
			t.Errorf("%q/%q: %s", tc.catalogue, tc.product, got)
		}
	}
}

func TestSpotifyProductFreeKeepsMetadataAndRejectsMedia(t *testing.T) {
	a, _, _ := fixtureCookieRuntime(t)
	if err := a.spotifyAuth.connect(context.Background(), "fixture-cookie"); err != nil {
		t.Fatal(err)
	}
	confirmTestSpotifyMetadataOwner(t, a.spotifyAuth, "free-owner")
	calls := 0
	upstreamProduct := "free"
	a.spotifyHTTPClient = &http.Client{Transport: sessionTransport(func(r *http.Request) (*http.Response, error) {
		body := `{"granted_token":{"token":"client","expires_after_seconds":600}}`
		if r.URL.Host == "spclient.wg.spotify.com" {
			calls++
			if r.URL.Path != "/melody/v1/product_state" || r.Method != "GET" || r.Header.Get("Authorization") == "" || r.Header.Get("Cookie") != "" {
				t.Fatal("invalid product request")
			}
			body = `{"catalogue":"` + upstreamProduct + `","product":"` + upstreamProduct + `"}`
		} else if r.URL.Host == "api-partner.spotify.com" {
			body = `{"data":{"me":{"profile":{"username":"free-owner","name":"Free account","uri":"spotify:user:free-owner"}}}}`
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
	})}
	w := httptest.NewRecorder()
	a.getSpotifyAuthStatus(w, httptest.NewRequest("GET", "/spotify/auth/status", nil))
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"product":"free"`) || !strings.Contains(w.Body.String(), `"canStream":false`) || !strings.Contains(w.Body.String(), `"connected":true`) {
		t.Fatal(w.Body.String())
	}
	// No download manager is installed: reaching queue/session work would panic.
	for _, handler := range []http.HandlerFunc{a.downloadTrack, a.downloadAlbum, a.downloadPlaylist, a.downloadFromURL, a.retryDownload, a.forceRestartDownload, a.streamSpotifyTrack} {
		w := httptest.NewRecorder()
		a.spotifyMediaOnly(handler)(w, httptest.NewRequest("POST", "/spotify/media", strings.NewReader(`{"spotifyId":"fixture"}`)))
		if w.Code != http.StatusForbidden {
			t.Fatalf("media returned %d: %s", w.Code, w.Body.String())
		}
	}
	resp, err := a.doSpotifyRequest(context.Background(), "GET", "https://api.spotify.com/v1/me", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatal("metadata blocked")
	}
	if calls != 1 {
		t.Fatalf("product was not cached: %d requests", calls)
	}
	if err := a.spotifyAuth.disconnect(); err != nil {
		t.Fatal(err)
	}
	if got := a.spotifyAccountProduct(context.Background()); got != "unknown" {
		t.Fatalf("logout retained %q", got)
	}
	if err := a.spotifyAuth.connect(context.Background(), "fixture-cookie"); err != nil {
		t.Fatal(err)
	}
	confirmTestSpotifyMetadataOwner(t, a.spotifyAuth, "free-owner")
	if got := a.spotifyAccountProduct(context.Background()); got != "free" || calls != 2 {
		t.Fatal("account replacement reused old capability")
	}
	upstreamProduct = "premium"
	if got := a.spotifyAccountProduct(context.Background(), true); got != "premium" || calls != 3 {
		t.Fatal("explicit recheck did not detect plan upgrade")
	}
	admitted := false
	a.spotifyMediaOnly(func(http.ResponseWriter, *http.Request) { admitted = true })(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil))
	if !admitted {
		t.Fatal("verified Premium media was blocked")
	}
}

func TestSpotifyProductProbeFailureDoesNotDisconnect(t *testing.T) {
	for _, body := range []string{`{}`, `{"catalogue":true}`, `not json`, `{"catalogue":"premium","product":"free"}`} {
		t.Run(body, func(t *testing.T) {
			a, _, _ := fixtureCookieRuntime(t)
			if err := a.spotifyAuth.connect(context.Background(), "fixture-cookie"); err != nil {
				t.Fatal(err)
			}
			confirmTestSpotifyMetadataOwner(t, a.spotifyAuth, "owner")
			a.spotifyHTTPClient = &http.Client{Transport: sessionTransport(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
			})}
			w := httptest.NewRecorder()
			if a.requireSpotifyMedia(w, httptest.NewRequest("GET", "/", nil)) || w.Code != 503 {
				t.Fatal("unverified media admitted")
			}
			if !a.spotifyAuth.status().Connected {
				t.Fatal("probe failure disconnected metadata session")
			}
		})
	}
}
