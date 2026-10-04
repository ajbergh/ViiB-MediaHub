// Manages cancellable Spotify browser sign-in operations and guarded session installation.
package api

import (
	"context"
	"errors"
	"log"
	"mime"
	"net/http"
	"net/url"
	"sync"
	"time"

	spotifyauth "github.com/ajbergh/viib-mediahub/internal/spotify/auth"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type spotifyBrowserLoginStatus struct {
	ID      string `json:"id"`
	State   string `json:"state"`
	Message string `json:"message,omitempty"`
}
type spotifyBrowserLogin struct {
	status  spotifyBrowserLoginStatus
	cancel  context.CancelFunc
	account context.Context
}

// Independent from account locks so capture/provider I/O can be canceled even
// while an account transition waits. Never retain cookie material here.
type spotifyBrowserLoginOwner struct {
	mu        sync.Mutex
	wg        sync.WaitGroup
	operation *spotifyBrowserLogin
	closed    bool
	capture   func(context.Context) (string, error)
}

func (s *spotifyAuthRuntime) cancelBrowserLogin(id string, closing bool) {
	s.login.mu.Lock()
	defer s.login.mu.Unlock()
	if closing {
		s.login.closed = true
	}
	op := s.login.operation
	if op != nil && (id == "" || op.status.ID == id) && op.status.State == "pending" {
		op.cancel()
		op.status.State = "canceled"
	}
}
func (s *spotifyAuthRuntime) startBrowserLogin() (spotifyBrowserLoginStatus, error) {
	s.login.mu.Lock()
	defer s.login.mu.Unlock()
	if s.login.closed {
		return spotifyBrowserLoginStatus{}, spotifyauth.ErrAuthenticationRequired
	}
	if old := s.login.operation; old != nil && old.status.State == "pending" {
		old.cancel()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	op := &spotifyBrowserLogin{status: spotifyBrowserLoginStatus{ID: uuid.NewString(), State: "pending"}, cancel: cancel}
	s.mu.RLock()
	op.account = s.lifetime
	s.mu.RUnlock()
	s.login.operation = op
	capture := s.login.capture
	if capture == nil {
		capture = spotifyauth.CaptureBrowserSession
	}
	s.login.wg.Add(1)
	go s.runBrowserLogin(ctx, op, capture)
	return op.status, nil
}
func (s *spotifyAuthRuntime) runBrowserLogin(ctx context.Context, op *spotifyBrowserLogin, capture func(context.Context) (string, error)) {
	defer s.login.wg.Done()
	defer op.cancel()
	stage := "capture"
	cookie, err := capture(ctx)
	if err == nil {
		stage = "validation_or_commit"
		err = s.connectGuarded(ctx, cookie, func() (func(bool), error) {
			s.login.mu.Lock()
			s.mu.RLock()
			currentAccount := s.lifetime == op.account && !s.closed
			s.mu.RUnlock()
			if !currentAccount || s.login.closed || s.login.operation != op || op.status.State != "pending" || ctx.Err() != nil {
				s.login.mu.Unlock()
				return nil, context.Canceled
			}
			return func(committed bool) {
				if committed {
					op.status.State = "connected"
				}
				s.login.mu.Unlock()
			}, nil
		})
	}
	cookie = ""
	s.login.mu.Lock()
	defer s.login.mu.Unlock()
	if s.login.operation != op || op.status.State != "pending" {
		return
	}
	op.status.State = "failed"
	category := "internal"
	var denied *spotifyauth.WebPlayerHTTPError
	switch {
	case errors.As(err, &denied):
		category = "provider_denied"
		log.Printf("[SpotifyLogin] stage=%s category=%s status=%d", stage, category, denied.Status)
		if denied.Status == http.StatusTooManyRequests {
			op.status.Message = "Spotify is rate limiting sign-in. Try again later."
		} else {
			op.status.Message = "Spotify declined this sign-in. Try again later."
		}
		return
	case errors.Is(err, spotifyauth.ErrAuthenticationRequired):
		category = "authentication_required"
	case errors.Is(err, spotifyauth.ErrLoginProfileCleanup):
		category = "profile_cleanup"
	case errors.Is(err, spotifyauth.ErrProviderChanged):
		category = "provider_changed"
	case errors.Is(err, spotifyauth.ErrTemporarilyUnavailable):
		category = "temporarily_unavailable"
	case errors.Is(err, context.DeadlineExceeded):
		category = "timeout"
	case errors.Is(err, context.Canceled):
		category = "canceled"
	case errors.Is(err, spotifyauth.ErrLoginBrowserClosed):
		category = "browser_closed"
	case errors.Is(err, spotifyauth.ErrLoginBrowserUnavailable):
		category = "browser_unavailable"
	}
	log.Printf("[SpotifyLogin] stage=%s category=%s", stage, category)
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		op.status.Message = "Sign-in timed out. Try again."
	case errors.Is(err, context.Canceled):
		op.status.State = "canceled"
	case errors.Is(err, spotifyauth.ErrLoginBrowserUnavailable):
		op.status.Message = "Install Chrome, Chromium or Microsoft Edge to sign in."
	case errors.Is(err, spotifyauth.ErrLoginProfileCleanup):
		op.status.Message = "Could not remove the temporary sign-in profile. Try again."
	case errors.Is(err, spotifyauth.ErrAuthenticationRequired):
		op.status.Message = "Spotify did not accept the signed-in session. Please sign in again."
	case errors.Is(err, spotifyauth.ErrProviderChanged):
		op.status.Message = "Spotify sign-in has changed. Please update ViiB or try again later."
	case errors.Is(err, spotifyauth.ErrTemporarilyUnavailable):
		op.status.Message = "Spotify is temporarily unavailable. Try again later."
	case errors.Is(err, spotifyauth.ErrLoginBrowserClosed):
		op.status.Message = "The sign-in window closed. Try again."
	default:
		op.status.Message = "Could not connect to Spotify. Try again after any rate limit clears."
	}
}
func (s *spotifyAuthRuntime) browserLoginStatus(id string) (spotifyBrowserLoginStatus, bool) {
	s.login.mu.Lock()
	defer s.login.mu.Unlock()
	if s.login.operation == nil || s.login.operation.status.ID != id {
		return spotifyBrowserLoginStatus{}, false
	}
	return s.login.operation.status, true
}
func (a *API) startSpotifyBrowserLogin(w http.ResponseWriter, r *http.Request) {
	if !spotifyLoginRequestAllowed(r) {
		respondError(w, http.StatusForbidden, "Sign-in request unavailable")
		return
	}
	status, err := a.spotifyTokens().startBrowserLogin()
	if err != nil {
		respondSpotifySessionError(w, err)
		return
	}
	respondJSON(w, status)
}
func (a *API) getSpotifyBrowserLogin(w http.ResponseWriter, r *http.Request) {
	status, ok := a.spotifyTokens().browserLoginStatus(chi.URLParam(r, "id"))
	if !ok {
		respondError(w, http.StatusNotFound, "Sign-in session unavailable")
		return
	}
	respondJSON(w, status)
}
func (a *API) cancelSpotifyBrowserLogin(w http.ResponseWriter, r *http.Request) {
	if !spotifyLoginRequestAllowed(r) {
		respondError(w, http.StatusForbidden, "Sign-in request unavailable")
		return
	}
	id := chi.URLParam(r, "id")
	a.spotifyTokens().cancelBrowserLogin(id, false)
	a.getSpotifyBrowserLogin(w, r)
}

// A JSON request cannot be submitted by a cross-origin HTML form. Validate
// browser origins too; CORS alone does not prevent side effects on loopback.
func spotifyLoginRequestAllowed(r *http.Request) bool {
	kind, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || kind != "application/json" {
		return false
	}
	origin := r.Header.Get("Origin")
	if origin == "" {
		return r.Header.Get("Sec-Fetch-Site") != "cross-site"
	}
	parsed, err := url.Parse(origin)
	if err != nil || parsed.User != nil || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return false
	}
	if parsed.Scheme == "wails" {
		return parsed.Host == "wails"
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return false
	}
	switch parsed.Hostname() {
	case "wails.localhost":
		return true
	case "localhost", "127.0.0.1":
		return parsed.Host == r.Host || (parsed.Scheme == "http" && (parsed.Port() == "5173" || parsed.Port() == "3000"))
	}
	return false
}
