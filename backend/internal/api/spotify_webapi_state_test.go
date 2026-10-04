// Tests coordination of Spotify request admission, cooldown state, and session rejection handling.
package api

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	spotifyauth "github.com/ajbergh/viib-mediahub/internal/spotify/auth"
)

func TestCookieWebAPICooldownSurvivesAccountAndRestart(t *testing.T) {
	a, _, _ := fixtureCookieRuntime(t)
	if err := a.spotifyAuth.connect(context.Background(), "fixture-cookie"); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	a.spotifyAuth.webAPINow = func() time.Time { return now }
	var calls atomic.Int32
	a.spotifyHTTPClient = &http.Client{Transport: sessionTransport(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		h := make(http.Header)
		h.Set("Retry-After", "120")
		return &http.Response{StatusCode: 429, Header: h, Body: io.NopCloser(strings.NewReader("{}"))}, nil
	})}
	response, err := a.doSpotifyRequest(context.Background(), "GET", "https://api.spotify.com/v1/me/player/recently-played?limit=1", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	for _, path := range []string{"search?q=test", "me/tracks"} {
		_, err := a.doSpotifyRequest(context.Background(), "GET", "https://api.spotify.com/v1/"+path, nil, "")
		var limited *spotifyauth.WebPlayerHTTPError
		if !errors.As(err, &limited) || limited.Status != 429 || limited.RetryAfter != 120*time.Second {
			t.Fatalf("cooldown: %v", err)
		}
	}
	if calls.Load() != 1 {
		t.Fatal("cooldown sent another request")
	}
	if _, err := a.spotifyAuth.Token(context.Background(), spotifyauth.Playback); err != nil {
		t.Fatal("playback blocked", err)
	}
	if err := a.spotifyAuth.withPlayback(context.Background(), func(context.Context, spotifyauth.Token) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if !a.spotifyAuth.status().Connected {
		t.Fatal("rate limit disconnected account")
	}
	if err := a.spotifyAuth.disconnect(); err != nil {
		t.Fatal(err)
	}
	if err := a.spotifyAuth.connect(context.Background(), "fixture-cookie"); err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	a.spotifySearch(w, httptest.NewRequest("GET", "/spotify/search?q=test", nil))
	if w.Code != 429 || w.Header().Get("Retry-After") != "120" {
		t.Fatalf("handler: %d %s", w.Code, w.Header())
	}
	restored := newSpotifyAuthRuntime(a.db, a.spotifyAuth.options)
	defer restored.close()
	restored.webAPINow = func() time.Time { return now }
	release, err := restored.acquireWebAPI(context.Background())
	if err == nil {
		release()
		t.Fatal("restart lost cooldown")
	}
	now = now.Add(121 * time.Second)
	release, err = restored.acquireWebAPI(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	release()
}

func TestCookieWebAPIStatusDistinguishesRejection(t *testing.T) {
	for _, status := range []int{401, 403, 404, 429} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			a, _, _ := fixtureCookieRuntime(t)
			if err := a.spotifyAuth.connect(context.Background(), "fixture-cookie"); err != nil {
				t.Fatal(err)
			}
			var calls atomic.Int32
			a.spotifyHTTPClient = &http.Client{Transport: sessionTransport(func(r *http.Request) (*http.Response, error) {
				calls.Add(1)
				return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("{}"))}, nil
			})}
			resp, err := a.doSpotifyRequest(context.Background(), "GET", "https://api.spotify.com/v1/me/player/recently-played?limit=1", nil, "")
			if resp != nil {
				resp.Body.Close()
			}
			if status == 401 {
				if !errors.Is(err, spotifyauth.ErrAuthenticationRequired) || calls.Load() != 2 {
					t.Fatalf("rejection: %v %d", err, calls.Load())
				}
				if a.spotifyAuth.status().Connected || !a.spotifyAuth.status().AuthRequired {
					t.Fatal("rejected session appears connected")
				}
				restored := newSpotifyAuthRuntime(a.db, a.spotifyAuth.options)
				defer restored.close()
				if restored.status().Connected {
					t.Fatal("restart restored rejected cookie")
				}
				if err := a.spotifyAuth.connect(context.Background(), "fixture-cookie"); err != nil {
					t.Fatal(err)
				}
				if !a.spotifyAuth.status().Connected {
					t.Fatal("explicit reconnect failed")
				}
			} else if err != nil || !a.spotifyAuth.status().Connected {
				t.Fatalf("non-auth failure invalidated session: %v", err)
			}
		})
	}
}

func TestCookieWebAPIQueuedRequestHonorsNewCooldown(t *testing.T) {
	a, _, _ := fixtureCookieRuntime(t)
	if err := a.spotifyAuth.connect(context.Background(), "fixture-cookie"); err != nil {
		t.Fatal(err)
	}
	entered := make(chan struct{})
	finish := make(chan struct{})
	var calls atomic.Int32
	a.spotifyHTTPClient = &http.Client{Transport: sessionTransport(func(r *http.Request) (*http.Response, error) {
		if calls.Add(1) != 1 {
			t.Error("queued request escaped cooldown")
		}
		close(entered)
		<-finish
		h := make(http.Header)
		h.Set("Retry-After", time.Now().Add(2*time.Minute).UTC().Format(http.TimeFormat))
		return &http.Response{StatusCode: 429, Header: h, Body: io.NopCloser(strings.NewReader("{}"))}, nil
	})}
	first := make(chan error, 1)
	go func() {
		resp, err := a.doSpotifyRequest(context.Background(), "GET", "https://api.spotify.com/v1/me/player/recently-played?limit=1", nil, "")
		if resp != nil {
			resp.Body.Close()
		}
		first <- err
	}()
	<-entered
	second := make(chan error, 1)
	go func() {
		_, err := a.doSpotifyRequest(context.Background(), "GET", "https://api.spotify.com/v1/me/player/recently-played?limit=1", nil, "")
		second <- err
	}()
	close(finish)
	if err := <-first; err != nil {
		t.Fatal(err)
	}
	var limited *spotifyauth.WebPlayerHTTPError
	if err := <-second; !errors.As(err, &limited) || limited.Status != 429 {
		t.Fatalf("queued: %v", err)
	}
	if calls.Load() != 1 {
		t.Fatal("multiple upstream requests")
	}
}

func TestCookieTokenEndpointRejectionRequiresReconnect(t *testing.T) {
	a, _, _ := fixtureCookieRuntime(t)
	original := a.spotifyAuth.options.Client.Transport
	var revoked atomic.Bool
	a.spotifyAuth.options.Client = &http.Client{Transport: sessionTransport(func(r *http.Request) (*http.Response, error) {
		if revoked.Load() && r.URL.Path == "/api/token" {
			return &http.Response{StatusCode: 401, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("{}"))}, nil
		}
		return original.RoundTrip(r)
	})}
	if err := a.spotifyAuth.connect(context.Background(), "fixture-cookie"); err != nil {
		t.Fatal(err)
	}
	token, err := a.spotifyAuth.Token(context.Background(), spotifyauth.Playback)
	if err != nil {
		t.Fatal(err)
	}
	revoked.Store(true)
	_, err = a.spotifyAuth.Refresh(context.Background(), spotifyauth.Playback, token)
	if !errors.Is(err, spotifyauth.ErrAuthenticationRequired) {
		t.Fatalf("revoked cookie: %v", err)
	}
	if a.spotifyAuth.status().Connected {
		t.Fatal("revoked cookie appears connected")
	}
}
