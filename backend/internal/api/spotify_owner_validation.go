package api

import (
	"context"
	"encoding/json"
	"errors"
	spotifyauth "github.com/ajbergh/viib-mediahub/internal/spotify/auth"
	"io"
	"math/rand/v2"
	"net/http"
	"time"
)

type spotifyOwnerFlight struct {
	lifetime context.Context
	done     chan struct{}
	cancel   context.CancelFunc
	waiters  int
	err      error
}

func (s *spotifyAuthRuntime) ensureMetadataOwner(ctx context.Context) error {
	s.mu.RLock()
	pending, verify := s.pendingOwner != nil, s.ownerVerifier
	retired := s.closed || s.lifetime.Err() != nil || s.invalid.Load() || ctx.Value(spotifyAccountContextKey{}) != s.lifetime
	s.mu.RUnlock()
	if retired {
		return spotifyauth.ErrAuthenticationRequired
	}
	if !pending {
		return ctx.Err()
	}
	if verify == nil {
		return spotifyauth.ErrDisabled
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	lifetime := ctx.Value(spotifyAccountContextKey{}).(context.Context)
	s.ownerMu.Lock()
	if s.ownerRetryLifetime == lifetime && s.ownerClock().Before(s.ownerRetryAt) {
		err := s.ownerRetryErr
		var limited *spotifyauth.WebPlayerHTTPError
		if errors.As(err, &limited) && limited.Status == 429 {
			err = &spotifyauth.WebPlayerHTTPError{Status: 429, RetryAfter: s.ownerRetryAt.Sub(s.ownerClock())}
		}
		s.ownerMu.Unlock()
		return err
	}
	f := s.ownerFlight
	if f == nil || f.lifetime != lifetime {
		root, stopAccount := s.requestContext(context.WithoutCancel(ctx))
		work, stopTimeout := context.WithTimeout(root, 25*time.Second)
		f = &spotifyOwnerFlight{lifetime: lifetime, done: make(chan struct{}), cancel: func() { stopTimeout(); stopAccount() }}
		s.ownerFlight = f
		go func() {
			err := verify(work)
			if work.Err() != nil {
				err = work.Err()
			}
			f.cancel()
			s.ownerMu.Lock()
			f.err = err
			if s.ownerFlight == f && !errors.Is(err, context.Canceled) {
				if err == nil {
					s.ownerFailures = 0
					s.ownerRetryAt = time.Time{}
					s.ownerRetryErr = nil
				} else {
					if s.ownerRetryLifetime != lifetime {
						s.ownerFailures = 0
					}
					s.ownerFailures++
					delay := time.Minute
					if s.ownerFailures == 2 {
						delay = 5 * time.Minute
					}
					if s.ownerFailures >= 3 {
						delay = 30 * time.Minute
					}
					var limited *spotifyauth.WebPlayerHTTPError
					if errors.As(err, &limited) && limited.Status == 429 && limited.RetryAfter > 0 {
						delay = limited.RetryAfter
					} else if s.ownerRetryDelay != nil {
						delay = s.ownerRetryDelay(delay)
					} else {
						delay = delay - delay/10 + time.Duration(rand.Int64N(int64(delay/5)+1))
					}
					s.ownerRetryLifetime = lifetime
					s.ownerRetryAt = s.ownerClock().Add(delay)
					s.ownerRetryErr = err
					if errors.Is(err, spotifyauth.ErrAuthenticationRequired) {
						s.invalid.Store(true)
					}
				}
			}
			if s.ownerFlight == f {
				s.ownerFlight = nil
			}
			close(f.done)
			s.ownerMu.Unlock()
		}()
	}
	f.waiters++
	s.ownerMu.Unlock()
	select {
	case <-f.done:
		if err := ctx.Err(); err != nil {
			return err
		}
		return f.err
	case <-ctx.Done():
		s.ownerMu.Lock()
		f.waiters--
		if f.waiters == 0 {
			f.cancel()
		}
		s.ownerMu.Unlock()
		return ctx.Err()
	}
}

func (a *API) revalidateSpotifyOwner(ctx context.Context, runtime *spotifyAuthRuntime) error {
	ctx = context.WithValue(ctx, spotifyOwnerBootstrapKey{}, true)
	response, err := a.doSpotifyRequest(ctx, http.MethodGet, "https://api.spotify.com/v1/me", nil, "")
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode == 401 {
		return spotifyauth.ErrAuthenticationRequired
	}
	if response.StatusCode != 200 {
		return &spotifyauth.WebPlayerHTTPError{Status: response.StatusCode, RetryAfter: spotifyauth.RetryAfter(response.Header.Get("Retry-After"), time.Now())}
	}
	var profile struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 64<<10)).Decode(&profile); err != nil {
		return err
	}
	if profile.ID == "" || len(profile.ID) > 256 {
		return errors.New("invalid Spotify owner profile")
	}
	if err := runtime.confirmProfileOwner(ctx, profile.ID); err != nil {
		return err
	}
	a.initSpotifyAnalysis()
	return nil
}

// Called under ownerMu; the injectable clock keeps retry tests deterministic.
func (s *spotifyAuthRuntime) ownerClock() time.Time {
	if s.ownerNow != nil {
		return s.ownerNow()
	}
	return time.Now()
}

func (s *spotifyAuthRuntime) ownerRateLimit(ctx context.Context) error {
	s.ownerMu.Lock()
	defer s.ownerMu.Unlock()
	var limited *spotifyauth.WebPlayerHTTPError
	if s.ownerRetryLifetime == ctx.Value(spotifyAccountContextKey{}) && s.ownerClock().Before(s.ownerRetryAt) && errors.As(s.ownerRetryErr, &limited) && limited.Status == 429 {
		return &spotifyauth.WebPlayerHTTPError{Status: 429, RetryAfter: s.ownerRetryAt.Sub(s.ownerClock())}
	}
	return nil
}

func (s *spotifyAuthRuntime) recordOwnerRateLimit(ctx context.Context, delay time.Duration) {
	s.mu.RLock()
	valid := ctx.Err() == nil && ctx.Value(spotifyAccountContextKey{}) == s.lifetime && !s.closed
	s.mu.RUnlock()
	if !valid {
		return
	}
	s.ownerMu.Lock()
	defer s.ownerMu.Unlock()
	lifetime := ctx.Value(spotifyAccountContextKey{}).(context.Context)
	until := s.ownerClock().Add(delay)
	if s.ownerRetryLifetime != lifetime || until.After(s.ownerRetryAt) {
		s.ownerRetryLifetime = lifetime
		s.ownerRetryAt = until
		s.ownerRetryErr = &spotifyauth.WebPlayerHTTPError{Status: 429, RetryAfter: delay}
	}
}
