package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/db"
	spotifyauth "github.com/ajbergh/viib-mediahub/internal/spotify/auth"
	"github.com/ajbergh/viib-mediahub/internal/spotify/catalog"
)

const spotifyCookieSetting = "spotify_webplayer_session"

var errSpotifyAccountChanged = errors.New("Spotify account changed")

type spotifyTokenSource interface {
	Token(context.Context, spotifyauth.Purpose) (spotifyauth.Token, error)
	Refresh(context.Context, spotifyauth.Purpose, spotifyauth.Token) (spotifyauth.Token, error)
}
type spotifyCookieRecord struct {
	Provider string `json:"provider"`
	Cookie   string `json:"spDC,omitempty"`
}
type spotifySessionStatus struct {
	Provider        string `json:"provider"`
	Connected       bool   `json:"connected"`
	AuthRequired    bool   `json:"authRequired"`
	Message         string `json:"message"`
	TokenExpiresAt  int64  `json:"tokenExpiresAt,omitempty"`
	TokenIssueCount uint64 `json:"tokenIssueCount,omitempty"`
}
type spotifyAuthRuntime struct {
	storeSession          func(string) error
	login                 spotifyBrowserLoginOwner
	mu                    sync.RWMutex
	mediaMu               sync.RWMutex
	onCancel              func()
	onRetire              func() error
	onConnected           func()
	changeMu              sync.Mutex
	database              *db.DB
	manager               *spotifyauth.Manager
	provider              *spotifyauth.WebPlayerProvider
	catalog               *catalog.Client
	cookieMode            bool
	closed                bool
	invalid               atomic.Bool
	webAPIGate            chan struct{}
	catalogGate           chan struct{}
	catalogBackgroundGate chan struct{}
	webAPIMu              sync.Mutex
	webAPIUntil           time.Time
	webAPINow             func() time.Time
	options               spotifyauth.WebPlayerOptions
	lifetime              context.Context
	endLifetime           context.CancelFunc
}

func newSpotifyAuthRuntime(database *db.DB, options spotifyauth.WebPlayerOptions) *spotifyAuthRuntime {
	s := &spotifyAuthRuntime{database: database, options: options, manager: spotifyOAuthManager(database)}
	s.storeSession = func(raw string) error { return database.SetSetting(spotifyCookieSetting, raw) }
	s.webAPIGate = make(chan struct{}, 1)
	s.catalogGate = make(chan struct{}, 4)
	// Leave one transaction slot available for interactive search.
	s.catalogBackgroundGate = make(chan struct{}, 3)
	s.webAPINow = time.Now
	if database != nil {
		if raw, err := database.GetSetting(spotifyWebAPICooldownSetting); err == nil {
			if ms, err := strconv.ParseInt(raw, 10, 64); err == nil {
				s.webAPIUntil = time.UnixMilli(ms)
			}
		}
	}
	s.lifetime, s.endLifetime = context.WithCancel(context.Background())
	if database == nil {
		s.cookieMode = true
		s.manager = spotifyauth.NewWebPlayerManager(nil)
		return s
	}
	raw, err := database.GetSetting(spotifyCookieSetting)
	if err != nil { // An unreadable cookie record must never silently select OAuth.
		s.cookieMode = true
		s.manager = spotifyauth.NewWebPlayerManager(nil)
		return s
	}
	if raw != "" {
		s.cookieMode = true
		s.manager = spotifyauth.NewWebPlayerManager(nil)
		var record spotifyCookieRecord
		if json.Unmarshal([]byte(raw), &record) == nil && record.Provider == "webplayer" && record.Cookie != "" {
			if p, err := spotifyauth.NewWebPlayerProvider(record.Cookie, spotifyauth.PinnedWebPlayerContract(), options); err == nil {
				s.invalid.Store(false)
				s.provider = p
				s.manager = spotifyauth.NewWebPlayerManager(p)
			}
		}
	}
	return s
}

type spotifyAccountContextKey struct{}

func (s *spotifyAuthRuntime) requestContext(parent context.Context) (context.Context, context.CancelFunc) {
	s.mu.RLock()
	lifetime := s.lifetime
	if original, ok := parent.Value(spotifyAccountContextKey{}).(context.Context); ok {
		lifetime = original
	}
	s.mu.RUnlock()
	ctx, cancel := context.WithCancelCause(parent)
	ctx = context.WithValue(ctx, spotifyAccountContextKey{}, lifetime)
	stop := context.AfterFunc(lifetime, func() { cancel(errSpotifyAccountChanged) })
	if lifetime.Err() != nil {
		cancel(errSpotifyAccountChanged)
	}
	return ctx, func() {
		stop()
		if lifetime.Err() != nil {
			cancel(errSpotifyAccountChanged)
		} else {
			cancel(context.Canceled)
		}
	}
}

func (s *spotifyAuthRuntime) Token(ctx context.Context, p spotifyauth.Purpose) (spotifyauth.Token, error) {
	return s.accountToken(ctx, p, nil)
}
func (s *spotifyAuthRuntime) Refresh(ctx context.Context, p spotifyauth.Purpose, rejected spotifyauth.Token) (spotifyauth.Token, error) {
	return s.accountToken(ctx, p, &rejected)
}

// Never hold the runtime lock over provider I/O: retirement must be able to
// cancel the lifetime that the provider request is waiting on.
func (s *spotifyAuthRuntime) accountToken(ctx context.Context, p spotifyauth.Purpose, rejected *spotifyauth.Token) (spotifyauth.Token, error) {
	ctx, cancel := s.requestContext(ctx)
	defer cancel()
	s.mu.RLock()
	lifetime := s.lifetime
	manager := s.manager
	if ctx.Err() != nil || ctx.Value(spotifyAccountContextKey{}) != lifetime || s.closed || lifetime.Err() != nil || (s.cookieMode && (s.provider == nil || s.invalid.Load())) {
		s.mu.RUnlock()
		return spotifyauth.Token{}, spotifyauth.ErrAuthenticationRequired
	}
	s.mu.RUnlock()
	var token spotifyauth.Token
	var err error
	if rejected == nil {
		token, err = manager.Token(ctx, p)
	} else {
		token, err = manager.Refresh(ctx, p, *rejected)
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	// A late success or rejection belongs only to the captured account.
	if s.lifetime != lifetime || lifetime.Err() != nil || s.closed || ctx.Err() != nil {
		return spotifyauth.Token{}, spotifyauth.ErrAuthenticationRequired
	}
	if s.cookieMode && errors.Is(err, spotifyauth.ErrAuthenticationRequired) {
		s.invalidateLocked()
	}
	if s.cookieMode && s.invalid.Load() {
		return spotifyauth.Token{}, spotifyauth.ErrAuthenticationRequired
	}
	return token, err
}
func (s *spotifyAuthRuntime) status() spotifySessionStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.cookieMode {
		connected := s.provider != nil && !s.invalid.Load() && !s.closed && s.lifetime.Err() == nil
		status := spotifySessionStatus{Provider: "webplayer", Connected: connected, AuthRequired: !connected}
		if connected {
			expiry, issues := s.provider.CachedLifetime()
			if !expiry.IsZero() {
				status.TokenExpiresAt = expiry.UnixMilli()
				status.TokenIssueCount = issues
			}
		}
		return status
	}
	connected := false
	if !s.closed {
		if creds, err := readSpotifyCredentials(s.database); err == nil {
			connected = creds.AccessToken != ""
		}
	}
	return spotifySessionStatus{Provider: "oauth", Connected: connected, AuthRequired: !connected}
}

// withPlayback serializes token-to-session preparation with account retirement.
// Cancellation occurs before retirement takes the write gate, so held media
// leases can drain while preparation waits for the SessionManager.
func (s *spotifyAuthRuntime) withPlayback(ctx context.Context, prepare func(context.Context, spotifyauth.Token) error) error {
	ctx, cancel := s.requestContext(ctx)
	defer cancel()
	token, err := s.Token(ctx, spotifyauth.Playback)
	if err != nil {
		return err
	}
	s.mediaMu.RLock()
	defer s.mediaMu.RUnlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := prepare(ctx, token); err != nil {
		return err
	}
	return ctx.Err()
}
func (s *spotifyAuthRuntime) setRetirementHooks(cancel func(), retire func() error, connected ...func()) {
	s.changeMu.Lock()
	defer s.changeMu.Unlock()
	s.onCancel = cancel
	s.onRetire = retire
	if len(connected) > 0 {
		s.onConnected = connected[0]
	}
}
func (s *spotifyAuthRuntime) beginRetirement() {
	s.mu.Lock()
	s.endLifetime()
	if s.catalog != nil {
		s.catalog.Close()
		s.catalog = nil
	}
	s.mu.Unlock()
	if s.onCancel != nil {
		s.onCancel()
	}
}
func (s *spotifyAuthRuntime) connect(ctx context.Context, cookie string) error {
	s.cancelBrowserLogin("", false)
	return s.connectGuarded(ctx, cookie, nil)
}
func (s *spotifyAuthRuntime) connectGuarded(ctx context.Context, cookie string, guard func() (func(bool), error)) error {
	s.changeMu.Lock()
	defer s.changeMu.Unlock()
	p, err := spotifyauth.NewWebPlayerProvider(cookie, spotifyauth.PinnedWebPlayerContract(), s.options)
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			p.Disconnect()
		}
	}()
	if _, err = p.Token(ctx); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if guard != nil {
		finish, err := guard()
		if err != nil {
			return err
		}
		defer func() { finish(committed) }()
	}
	raw, err := json.Marshal(spotifyCookieRecord{Provider: "webplayer", Cookie: cookie})
	cookie = ""
	if err != nil {
		return err
	}
	defer clear(raw)
	// Storage failure must leave the current account and media usable.
	previous, err := s.database.GetSetting(spotifyCookieSetting)
	if err != nil {
		return errors.New("could not read Spotify session")
	}
	if err := s.storeSession(string(raw)); err != nil {
		return errors.New("could not store Spotify session")
	}
	defer func() {
		if !committed {
			_ = s.storeSession(previous)
			s.mu.Lock()
			if !s.closed {
				s.lifetime, s.endLifetime = context.WithCancel(context.Background())
			}
			s.mu.Unlock()
		}
	}()
	s.beginRetirement()
	s.mediaMu.Lock()
	defer s.mediaMu.Unlock()
	if s.onRetire != nil {
		if err := s.onRetire(); err != nil {
			return err
		}
	}
	s.mu.Lock()
	defer func() {
		s.mu.Unlock()
		if committed && s.onConnected != nil {
			s.onConnected()
		}
	}()
	if s.closed || ctx.Err() != nil {
		return spotifyauth.ErrAuthenticationRequired
	}
	if s.provider != nil {
		s.provider.Disconnect()
	}
	s.lifetime, s.endLifetime = context.WithCancel(context.Background())
	s.invalid.Store(false)
	s.provider = p
	s.manager = spotifyauth.NewWebPlayerManager(p)
	s.cookieMode = true
	committed = true
	return nil
}
func (s *spotifyAuthRuntime) disconnect() error {
	s.cancelBrowserLogin("", false)
	s.changeMu.Lock()
	defer s.changeMu.Unlock()
	s.beginRetirement()
	s.mediaMu.Lock()
	defer s.mediaMu.Unlock()
	if s.onRetire != nil {
		if err := s.onRetire(); err != nil {
			return err
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	raw, _ := json.Marshal(spotifyCookieRecord{Provider: "webplayer"})
	if err := s.database.SetSetting(spotifyCookieSetting, string(raw)); err != nil {
		return errors.New("could not remove Spotify session")
	}
	if s.provider != nil {
		s.provider.Disconnect()
	}
	s.lifetime, s.endLifetime = context.WithCancel(context.Background())
	s.provider = nil
	s.cookieMode = true
	s.manager = spotifyauth.NewWebPlayerManager(nil)
	return nil
}
func (s *spotifyAuthRuntime) close() {
	s.cancelBrowserLogin("", true)
	defer s.login.wg.Wait()
	s.changeMu.Lock()
	defer s.changeMu.Unlock()
	s.beginRetirement()
	s.mediaMu.Lock()
	defer s.mediaMu.Unlock()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	if s.provider != nil {
		s.provider.Disconnect()
	}
	s.provider = nil
}

func (a *API) spotifyTokens() *spotifyAuthRuntime {
	a.spotifyAuthMu.Lock()
	defer a.spotifyAuthMu.Unlock()
	if a.spotifyAuth == nil {
		a.spotifyAuth = newSpotifyAuthRuntime(a.db, spotifyauth.WebPlayerOptions{})
	}
	a.spotifyHooksOnce.Do(func() {
		a.spotifyAuth.setRetirementHooks(a.cancelSpotifyMedia, a.retireSpotifyAccount, func() {
			a.initSpotifyAnalysis()
			if a.downloadManager != nil {
				a.downloadManager.ClearAuthRequired()
			}
		})
	})
	return a.spotifyAuth
}
func (a *API) connectSpotifySession(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	var body struct {
		Cookie string `json:"spDC"`
	}
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if decoder.Decode(&body) != nil {
		respondError(w, 400, "Invalid session request")
		return
	}
	err := a.spotifyTokens().connect(r.Context(), body.Cookie)
	body.Cookie = ""
	if err != nil {
		respondSpotifySessionError(w, err)
		return
	}
	if a.downloadManager != nil {
		a.downloadManager.ClearAuthRequired()
	}
	respondJSON(w, a.spotifyTokens().status())
}
func (a *API) disconnectSpotifySession(w http.ResponseWriter, r *http.Request) {
	if err := a.spotifyTokens().disconnect(); err != nil {
		respondError(w, 500, "Could not remove Spotify session")
		return
	}
	respondJSON(w, a.spotifyTokens().status())
}

func (a *API) cancelSpotifyMedia() {
	a.spotifyStreamerMu.Lock()
	if a.spotifyStreamer != nil {
		a.spotifyStreamer.CloseAllStreams()
		// A retired streamer cannot be reopened, even when the session manager is reused.
		a.spotifyStreamer = nil
		a.spotifyStreamSession = nil
	}
	a.spotifyStreamerMu.Unlock()
	if a.downloadManager != nil {
		a.downloadManager.setAuthRequired(true, "Reconnect to Spotify")
		a.downloadManager.mu.Lock()
		for _, cancel := range a.downloadManager.activeDownloads {
			cancel()
		}
		a.downloadManager.mu.Unlock()
	}
}

func (a *API) retireSpotifyAccount() error {
	if a.downloadManager != nil && a.downloadManager.sessionManager != nil {
		a.downloadManager.sessionManager.ClearCredentials()
	}
	// Retire account-scoped analysis independently from local recording identities.
	a.spotifyAnalysisMu.Lock()
	service := a.spotifyAnalysis
	a.spotifyAnalysis = nil
	a.spotifyAnalysisMu.Unlock()
	if service != nil {
		service.Close()
	}
	return a.db.PurgeExternalAnalysis()
}
func respondSpotifySessionError(w http.ResponseWriter, err error) {
	status := http.StatusServiceUnavailable
	if errors.Is(err, catalog.ErrInvalidQuery) {
		status = http.StatusBadRequest
	}
	if errors.Is(err, spotifyauth.ErrAuthenticationRequired) || errors.Is(err, spotifyauth.ErrDisabled) {
		status = http.StatusUnauthorized
	}
	var upstream *spotifyauth.WebPlayerHTTPError
	if errors.As(err, &upstream) && upstream.Status == 429 {
		status = 429
		w.Header().Set("Retry-After", strconv.FormatInt(int64((upstream.RetryAfter+time.Second-1)/time.Second), 10))
	}
	respondError(w, status, "Spotify session unavailable")
}

// Commit account-derived work before account replacement can retire its context.
func (s *spotifyAuthRuntime) withAccount(ctx context.Context, commit func() error) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if ctx.Err() != nil || s.closed || s.lifetime.Err() != nil || ctx.Value(spotifyAccountContextKey{}) != s.lifetime || s.invalid.Load() {
		return spotifyauth.ErrAuthenticationRequired
	}
	return commit()
}
