package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	spotifyauth "github.com/ajbergh/viib-mediahub/internal/spotify/auth"
)

func latencyResult(t *testing.T, done <-chan error) error {
	t.Helper()
	select {
	case err := <-done:
		return err
	case <-time.After(2 * time.Second):
		t.Fatal("operation blocked")
		return nil
	}
}

func latencyResponse(body string) *http.Response {
	return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
}

func latencyRequest(a *API, target string) <-chan error {
	done := make(chan error, 1)
	go func() {
		response, err := a.doSpotifyRequest(context.Background(), http.MethodGet, "https://api.spotify.com/v1/"+target, nil, "")
		if response != nil {
			response.Body.Close()
		}
		done <- err
	}()
	return done
}

func TestCookieCatalogSearchBypassesSlowBackground(t *testing.T) {
	for _, background := range []string{"me", "me/albums?limit=1"} {
		t.Run(background, func(t *testing.T) {
			a, _, tokenCalls := fixtureCookieRuntime(t)
			if err := a.spotifyAuth.connect(context.Background(), "fixture-cookie"); err != nil {
				t.Fatal(err)
			}
			entered := make(chan struct{}, 3)
			release := make(chan struct{})
			defer close(release)
			var mints atomic.Int32
			a.spotifyHTTPClient = &http.Client{Transport: sessionTransport(func(r *http.Request) (*http.Response, error) {
				if r.URL.Host == "clienttoken.spotify.com" {
					mints.Add(1)
					return latencyResponse(`{"granted_token":{"token":"fixture-client","expires_after_seconds":600}}`), nil
				}
				var payload struct {
					Operation string `json:"operationName"`
				}
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					return nil, err
				}
				if payload.Operation == "searchDesktop" {
					return latencyResponse(`{"data":{"searchV2":{"artists":{"totalCount":0,"items":[]}}}}`), nil
				}
				if payload.Operation == "libraryV3" {
					// Exercise the N+1 hydration phase rather than only the list request.
					return latencyResponse(`{"data":{"me":{"libraryV3":{"totalCount":1,"items":[{"item":{"data":{"__typename":"Album","uri":"spotify:album:AAAAAAAAAAAAAAAAAAAAAA"}}}]}}}}`), nil
				}
				entered <- struct{}{}
				select {
				case <-release:
					return latencyResponse(`{}`), nil
				case <-r.Context().Done():
					return nil, r.Context().Err()
				}
			})}
			pending := make([]<-chan error, 3)
			for i := range pending {
				pending[i] = latencyRequest(a, background)
			}
			for range pending {
				select {
				case <-entered:
				case <-time.After(2 * time.Second):
					t.Fatal("background did not enter upstream")
				}
			}
			if err := latencyResult(t, latencyRequest(a, "search?q=fixture&type=artist")); err != nil {
				t.Fatal(err)
			}
			if mints.Load() != 1 || tokenCalls.Load() != 1 {
				t.Fatal("concurrent catalog requests did not coalesce tokens")
			}
			if err := a.spotifyAuth.disconnect(); err != nil {
				t.Fatal(err)
			}
			for _, done := range pending {
				if err := latencyResult(t, done); !errors.Is(err, context.Canceled) {
					t.Fatalf("background survived retirement: %v", err)
				}
			}
		})
	}
}

func TestSpotifyHookAccessDuringAccountTransition(t *testing.T) {
	a, _, _ := fixtureCookieRuntime(t)
	s := a.spotifyTokens() // One-time setup before requests/account changes.
	if err := s.connect(context.Background(), "fixture-cookie"); err != nil {
		t.Fatal(err)
	}
	entered, release := make(chan struct{}), make(chan struct{})
	defer close(release)
	s.setRetirementHooks(nil, func() error { close(entered); <-release; return nil })
	changed := make(chan error, 1)
	go func() { changed <- s.disconnect() }()
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("transition did not enter retirement hook")
	}
	access := make(chan error, 1)
	go func() {
		if a.spotifyTokens() != s {
			access <- errors.New("runtime changed")
			return
		}
		access <- nil
	}()
	if err := latencyResult(t, access); err != nil {
		t.Fatal(err)
	}
	// Cleanup must wait for the blocked hook before closing the runtime.
	t.Cleanup(func() { latencyResult(t, changed) })
}

func TestCookieRuntimeRetirementCancelsTokenRefresh(t *testing.T) {
	a, _, _ := fixtureCookieRuntime(t)
	base := a.spotifyAuth.options.Client.Transport
	var block atomic.Bool
	entered := make(chan struct{})
	a.spotifyAuth.options.Client = &http.Client{Transport: sessionTransport(func(r *http.Request) (*http.Response, error) {
		if block.Load() && r.URL.Path == "/api/token" {
			close(entered)
			<-r.Context().Done()
			return nil, r.Context().Err()
		}
		return base.RoundTrip(r)
	})}
	if err := a.spotifyAuth.connect(context.Background(), "fixture-cookie"); err != nil {
		t.Fatal(err)
	}
	token, err := a.spotifyAuth.Token(context.Background(), spotifyauth.WebAPI)
	if err != nil {
		t.Fatal(err)
	}
	block.Store(true)
	refreshed := make(chan error, 1)
	go func() {
		_, err := a.spotifyAuth.Refresh(context.Background(), spotifyauth.WebAPI, token)
		refreshed <- err
	}()
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("refresh did not enter upstream")
	}
	changed := make(chan error, 1)
	go func() { changed <- a.spotifyAuth.disconnect() }()
	if err := latencyResult(t, changed); err != nil {
		t.Fatal(err)
	}
	if err := latencyResult(t, refreshed); !errors.Is(err, spotifyauth.ErrAuthenticationRequired) {
		t.Fatalf("retired refresh returned a token: %v", err)
	}
}

func TestCookieRuntimeRetirementCancelsColdToken(t *testing.T) {
	a, _, _ := fixtureCookieRuntime(t)
	if err := a.spotifyAuth.connect(context.Background(), "fixture-cookie"); err != nil {
		t.Fatal(err)
	}
	options := a.spotifyAuth.options
	base := options.Client.Transport
	entered := make(chan struct{})
	options.Client = &http.Client{Transport: sessionTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/api/token" {
			close(entered)
			<-r.Context().Done()
			return nil, r.Context().Err()
		}
		return base.RoundTrip(r)
	})}
	s := newSpotifyAuthRuntime(a.db, options)
	defer s.close()
	done := make(chan error, 1)
	go func() { _, err := s.Token(context.Background(), spotifyauth.WebAPI); done <- err }()
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("cold token did not enter upstream")
	}
	changed := make(chan error, 1)
	go func() { changed <- s.disconnect() }()
	if err := latencyResult(t, changed); err != nil {
		t.Fatal(err)
	}
	if err := latencyResult(t, done); !errors.Is(err, spotifyauth.ErrAuthenticationRequired) {
		t.Fatalf("retired cold token succeeded: %v", err)
	}
}

func TestCookieRuntimeFencesLateTokenResults(t *testing.T) {
	for _, rejection := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "rejection"}[rejection], func(t *testing.T) {
			a, _, _ := fixtureCookieRuntime(t)
			if err := a.spotifyAuth.connect(context.Background(), "fixture-cookie"); err != nil {
				t.Fatal(err)
			}
			entered, release := make(chan struct{}), make(chan struct{})
			defer close(release)
			a.spotifyAuth.manager = spotifyauth.NewManager(spotifyauth.ProviderFuncs{Load: func(context.Context) (spotifyauth.Token, error) {
				close(entered)
				<-release // Deliberately ignore cancellation to exercise the fence.
				if rejection {
					return spotifyauth.Token{}, spotifyauth.ErrAuthenticationRequired
				}
				return spotifyauth.NewToken("fixture-stale", spotifyauth.WebPlayer, time.Now().Add(time.Hour), 1), nil
			}}, nil)
			done := make(chan error, 1)
			go func() { _, err := a.spotifyAuth.Token(context.Background(), spotifyauth.WebAPI); done <- err }()
			select {
			case <-entered:
			case <-time.After(2 * time.Second):
				t.Fatal("token call did not enter")
			}
			changed := make(chan error, 1)
			go func() { changed <- a.spotifyAuth.connect(context.Background(), "fixture-cookie") }()
			if err := latencyResult(t, changed); err != nil {
				t.Fatal(err)
			}
			// The deferred release runs before cleanup waits for the late result.
			t.Cleanup(func() {
				if err := latencyResult(t, done); !errors.Is(err, spotifyauth.ErrAuthenticationRequired) {
					t.Errorf("late token: %v", err)
				}
				if !a.spotifyAuth.status().Connected {
					t.Error("late result invalidated replacement")
				}
			})
		})
	}
}

func TestCookieCatalogConcurrentCooldownStopsHydration(t *testing.T) {
	a, _, _ := fixtureCookieRuntime(t)
	if err := a.spotifyAuth.connect(context.Background(), "fixture-cookie"); err != nil {
		t.Fatal(err)
	}
	entered, release := make(chan struct{}), make(chan struct{})
	defer close(release)
	var calls atomic.Int32
	a.spotifyHTTPClient = &http.Client{Transport: sessionTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host == "clienttoken.spotify.com" {
			return latencyResponse(`{"granted_token":{"token":"fixture-client","expires_after_seconds":600}}`), nil
		}
		calls.Add(1)
		var payload struct {
			Operation string `json:"operationName"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			return nil, err
		}
		if payload.Operation == "libraryV3" {
			close(entered)
			select {
			case <-release:
			case <-r.Context().Done():
				return nil, r.Context().Err()
			}
			return latencyResponse(`{"data":{"me":{"libraryV3":{"totalCount":1,"items":[{"item":{"data":{"__typename":"Album","uri":"spotify:album:AAAAAAAAAAAAAAAAAAAAAA"}}}]}}}}`), nil
		}
		if payload.Operation != "searchDesktop" {
			return nil, errors.New("hydration escaped cooldown")
		}
		response := latencyResponse(`{}`)
		response.StatusCode = 429
		response.Header.Set("Retry-After", "120")
		return response, nil
	})}
	background := latencyRequest(a, "me/albums?limit=1")
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("library did not enter upstream")
	}
	assertLimited := func(err error) {
		t.Helper()
		var limited *spotifyauth.WebPlayerHTTPError
		if !errors.As(err, &limited) || limited.Status != 429 || limited.RetryAfter <= 0 {
			t.Fatalf("missing shared cooldown: %v", err)
		}
	}
	assertLimited(latencyResult(t, latencyRequest(a, "search?q=fixture&type=artist")))
	assertLimited(latencyResult(t, latencyRequest(a, "me")))
	assertLimited(latencyResult(t, latencyRequest(a, "me/player/recently-played?limit=1")))
	t.Cleanup(func() {
		assertLimited(latencyResult(t, background))
		if calls.Load() != 2 {
			t.Errorf("cooldown allowed additional dispatches: %d", calls.Load())
		}
	})
}

func TestCookieCatalogQueuedSlotRechecksCooldown(t *testing.T) {
	s := newSpotifyAuthRuntime(nil, spotifyauth.WebPlayerOptions{})
	defer s.close()
	releases := make([]func(), cap(s.catalogGate))
	for i := range releases {
		var err error
		releases[i], err = s.acquireCatalog(context.Background(), true)
		if err != nil {
			t.Fatal(err)
		}
	}
	done := make(chan error, 1)
	go func() {
		release, err := s.acquireCatalog(context.Background(), true)
		if release != nil {
			release()
		}
		done <- err
	}()
	if err := s.recordWebAPICooldown("120"); err != nil {
		t.Fatal(err)
	}
	for _, release := range releases {
		release()
	}
	var limited *spotifyauth.WebPlayerHTTPError
	if err := latencyResult(t, done); !errors.As(err, &limited) || limited.Status != 429 {
		t.Fatalf("queued slot ignored cooldown: %v", err)
	}
}
