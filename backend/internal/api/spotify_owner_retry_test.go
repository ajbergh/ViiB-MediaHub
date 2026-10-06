package api

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/ajbergh/viib-mediahub/internal/db"
	spotifyauth "github.com/ajbergh/viib-mediahub/internal/spotify/auth"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestOwnerValidationBackoffAndRateLimit(t *testing.T) {
	for _, limited := range []bool{false, true} {
		t.Run(map[bool]string{false: "temporary", true: "rate_limit"}[limited], func(t *testing.T) {
			a, _ := newBPMRouteTestAPI(t, false)
			s := a.spotifyTokens()
			s.pendingOwner = &db.SpotifyMetadataOwner{ContextKey: s.metadataContext}
			now := time.Unix(1700000000, 0)
			s.ownerNow = func() time.Time { return now }
			s.ownerRetryDelay = func(delay time.Duration) time.Duration { return delay }
			calls := 0
			s.ownerVerifier = func(context.Context) error {
				calls++
				if limited {
					return &spotifyauth.WebPlayerHTTPError{Status: 429, RetryAfter: 90 * time.Second}
				}
				return errors.New("offline")
			}
			ctx, cancel := s.requestContext(t.Context())
			defer cancel()
			for i, delay := range []time.Duration{time.Minute, 5 * time.Minute, 30 * time.Minute} {
				if limited {
					delay = 90 * time.Second
				}
				if err := s.ensureMetadataOwner(ctx); err == nil {
					t.Fatal("failure admitted")
				}
				if calls != i+1 || !s.ownerRetryAt.Equal(now.Add(delay)) {
					t.Fatal("incorrect retry schedule", calls, s.ownerRetryAt, delay)
				}
				if limited {
					w := httptest.NewRecorder()
					a.spotifyGetUserProfile(w, httptest.NewRequest("GET", "/spotify/me", nil))
					if w.Code != 429 || w.Header().Get("Retry-After") != "90" || calls != i+1 {
						t.Fatal("manual profile bypassed live cooldown", w.Code, w.Header(), calls)
					}
				}
				for j := 0; j < 3; j++ {
					if err := s.ensureMetadataOwner(ctx); err == nil {
						t.Fatal("retry delay ignored")
					}
				}
				if calls != i+1 {
					t.Fatal("profile request storm", calls)
				}
				s.ownerMu.Lock()
				now = now.Add(delay / 2)
				s.ownerMu.Unlock()
				if err := s.ensureMetadataOwner(ctx); limited {
					var e *spotifyauth.WebPlayerHTTPError
					if !errors.As(err, &e) || e.Status != 429 || e.RetryAfter != delay/2 {
						t.Fatal("remaining cooldown lost", err)
					}
				}
				s.ownerMu.Lock()
				now = now.Add(delay / 2)
				s.ownerMu.Unlock()
			}
		})
	}
}

func TestOwnerValidationAuthenticationRejectionStopsRetries(t *testing.T) {
	a, _ := newBPMRouteTestAPI(t, false)
	s := a.spotifyTokens()
	s.pendingOwner = &db.SpotifyMetadataOwner{ContextKey: s.metadataContext}
	calls := 0
	s.ownerVerifier = func(context.Context) error { calls++; return spotifyauth.ErrAuthenticationRequired }
	ctx, cancel := s.requestContext(t.Context())
	defer cancel()
	for i := 0; i < 3; i++ {
		if err := s.ensureMetadataOwner(ctx); !errors.Is(err, spotifyauth.ErrAuthenticationRequired) {
			t.Fatal(err)
		}
	}
	if calls != 1 || !s.invalid.Load() || s.status().Connected {
		t.Fatal("rejected owner kept retrying", calls)
	}
}

func TestOAuthOwnerValidationPropagatesRateLimitWithoutInlineRetry(t *testing.T) {
	a, _ := newBPMRouteTestAPI(t, false)
	credentials, _ := json.Marshal(SpotifyCredentials{ClientId: "client", AccessToken: "fixture", Expiry: time.Now().Add(time.Hour).UnixMilli()})
	if err := a.db.SetSetting("spotify_credentials", string(credentials)); err != nil {
		t.Fatal(err)
	}
	old := a.spotifyTokens()
	if err := a.db.BindSpotifyMetadataOwner(old.metadataEpoch, "oauth", "account", old.metadataContext); err != nil {
		t.Fatal(err)
	}
	a.spotifyAuth = newSpotifyAuthRuntime(a.db, spotifyauth.WebPlayerOptions{})
	s := a.spotifyTokens()
	now := time.Now()
	s.ownerNow = func() time.Time { return now }
	calls := 0
	a.spotifyHTTPClient = &http.Client{Transport: sessionTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 429, Header: http.Header{"Retry-After": []string{"90"}}, Body: io.NopCloser(strings.NewReader(`{}`))}, nil
	})}
	for i := 0; i < 2; i++ {
		_, err := s.Token(t.Context(), spotifyauth.WebAPI)
		var limited *spotifyauth.WebPlayerHTTPError
		if !errors.As(err, &limited) || limited.Status != 429 || limited.RetryAfter != 90*time.Second {
			t.Fatal("rate limit not propagated", err)
		}
	}
	if calls != 1 {
		t.Fatal("inline retry ignored cooldown", calls)
	}
}

func TestProfileMalformedRateLimitStillStopsManualRetry(t *testing.T) {
	a, _ := newBPMRouteTestAPI(t, false)
	credentials, _ := json.Marshal(SpotifyCredentials{ClientId: "client", AccessToken: "fixture", Expiry: time.Now().Add(time.Hour).UnixMilli()})
	if err := a.db.SetSetting("spotify_credentials", string(credentials)); err != nil {
		t.Fatal(err)
	}
	calls := 0
	a.spotifyHTTPClient = &http.Client{Transport: sessionTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 429, Header: http.Header{"Retry-After": []string{"90"}}, Body: io.NopCloser(strings.NewReader("unparseable"))}, nil
	})}
	for i := 0; i < 2; i++ {
		w := httptest.NewRecorder()
		a.spotifyGetUserProfile(w, httptest.NewRequest("GET", "/spotify/me", nil))
		if w.Code != 429 || w.Header().Get("Retry-After") == "" {
			t.Fatal("malformed rate limit lost status/header", w.Code, w.Header())
		}
	}
	if calls != 1 {
		t.Fatal("manual retry ignored malformed rate limit", calls)
	}
}
