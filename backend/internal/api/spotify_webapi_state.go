// Coordinates Spotify request admission, cooldown state, and session rejection handling.
package api

import (
	"context"
	"strconv"
	"time"

	spotifyauth "github.com/ajbergh/viib-mediahub/internal/spotify/auth"
)

const spotifyWebAPICooldownSetting = "spotify_webapi_cooldown_until"

// Queued callers recheck the deadline after the preceding response headers.
// This gate does not block playback or account retirement.
func (s *spotifyAuthRuntime) acquireWebAPI(ctx context.Context) (func(), error) {
	select {
	case s.webAPIGate <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	release := func() { <-s.webAPIGate }
	if err := ctx.Err(); err != nil {
		release()
		return nil, err
	}
	if err := s.checkWebAPICooldown(ctx); err != nil {
		release()
		return nil, err
	}
	return release, nil
}

// Catalog transactions are bounded independently of the serialized public API
// path. Background hydration cannot occupy the reserved search slot.
func (s *spotifyAuthRuntime) acquireCatalog(ctx context.Context, search bool) (func(), error) {
	if !search {
		select {
		case s.catalogBackgroundGate <- struct{}{}:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	releaseBackground := func() {
		if !search {
			<-s.catalogBackgroundGate
		}
	}
	select {
	case s.catalogGate <- struct{}{}:
	case <-ctx.Done():
		releaseBackground()
		return nil, ctx.Err()
	}
	release := func() { <-s.catalogGate; releaseBackground() }
	if err := s.checkWebAPICooldown(ctx); err != nil {
		release()
		return nil, err
	}
	return release, nil
}

func (s *spotifyAuthRuntime) webAPICooldownRemaining() time.Duration {
	s.webAPIMu.Lock()
	defer s.webAPIMu.Unlock()
	return s.webAPIUntil.Sub(s.webAPINow())
}

func (s *spotifyAuthRuntime) checkWebAPICooldown(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if remaining := s.webAPICooldownRemaining(); remaining > 0 {
		return &spotifyauth.WebPlayerHTTPError{Status: 429, RetryAfter: remaining}
	}
	return nil
}

// Concurrent catalog responses share this lock; never shorten a deadline.
func (s *spotifyAuthRuntime) recordWebAPICooldown(value string) error {
	s.webAPIMu.Lock()
	defer s.webAPIMu.Unlock()
	now := s.webAPINow()
	delay := spotifyauth.RetryAfter(value, now)
	if delay <= 0 {
		delay = time.Minute
	}
	until := now.Add(delay)
	if until.After(s.webAPIUntil) {
		s.webAPIUntil = until
	}
	if s.database != nil {
		return s.database.SetSetting(spotifyWebAPICooldownSetting, strconv.FormatInt(s.webAPIUntil.UnixMilli(), 10))
	}
	return nil
}

// The caller holds mu, fencing stale responses against account replacement.
// Removing the rejected cookie also requires reconnect after a restart.
func (s *spotifyAuthRuntime) invalidateLocked() {
	if !s.cookieMode || s.closed || s.invalid.Swap(true) {
		return
	}
	s.endLifetime()
	if s.catalog != nil {
		s.catalog.Close()
	}
	if s.database != nil {
		// Rejection remains effective in memory if persistence fails.
		_ = s.database.SetSetting(spotifyCookieSetting, "{\"provider\":\"webplayer\"}")
	}
}
func (s *spotifyAuthRuntime) rejectWebAPISession(ctx context.Context) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if ctx.Err() == nil && s.lifetime.Err() == nil && ctx.Value(spotifyAccountContextKey{}) == s.lifetime {
		s.invalidateLocked()
	}
}
