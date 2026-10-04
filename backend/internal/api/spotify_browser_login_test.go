// Tests managing cancellable Spotify browser sign-in operations and guarded session installation.
package api

import (
	"context"
	"encoding/json"
	"errors"
	spotifyauth "github.com/ajbergh/viib-mediahub/internal/spotify/auth"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func waitBrowserLogin(t *testing.T, s *spotifyAuthRuntime, id, state string) spotifyBrowserLoginStatus {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		status, ok := s.browserLoginStatus(id)
		if ok && status.State == state {
			return status
		}
		time.Sleep(time.Millisecond)
	}
	status, _ := s.browserLoginStatus(id)
	t.Fatalf("wanted %s, got %+v", state, status)
	return status
}
func TestBrowserLoginConnectsThroughEncryptedSessionBoundary(t *testing.T) {
	a, _, calls := fixtureCookieRuntime(t)
	s := a.spotifyAuth
	s.login.capture = func(context.Context) (string, error) { return "fixture-cookie", nil }
	op, err := s.startBrowserLogin()
	if err != nil {
		t.Fatal(err)
	}
	result := waitBrowserLogin(t, s, op.ID, "connected")
	if !s.status().Connected || calls.Load() != 1 {
		t.Fatal("capture did not validate/install session")
	}
	raw, _ := json.Marshal(result)
	if strings.Contains(string(raw), "fixture") {
		t.Fatal("status leaked credential")
	}
	restored := newSpotifyAuthRuntime(a.db, s.options)
	defer restored.close()
	if !restored.status().Connected {
		t.Fatal("session did not restore")
	}
}
func TestBrowserLoginCanceledCaptureCannotCommit(t *testing.T) {
	for _, action := range []string{"cancel", "disconnect", "replacement", "close"} {
		t.Run(action, func(t *testing.T) {
			a, _, calls := fixtureCookieRuntime(t)
			s := a.spotifyAuth
			entered, release, finished := make(chan struct{}), make(chan struct{}), make(chan struct{})
			s.login.capture = func(context.Context) (string, error) { close(entered); <-release; return "fixture-cookie", nil }
			op, err := s.startBrowserLogin()
			if err != nil {
				t.Fatal(err)
			}
			<-entered
			switch action {
			case "cancel":
				s.cancelBrowserLogin(op.ID, false)
			case "disconnect":
				if err := s.disconnect(); err != nil {
					t.Fatal(err)
				}
			case "replacement":
				if err := s.connect(context.Background(), "fixture-cookie"); err != nil {
					t.Fatal(err)
				}
			case "close":
				go func() { s.close(); close(finished) }()
				waitBrowserLogin(t, s, op.ID, "canceled")
			}
			close(release)
			s.login.wg.Wait()
			if action == "close" {
				<-finished
				if _, err := s.startBrowserLogin(); err == nil {
					t.Fatal("closed runtime accepted login")
				}
			}
			if result, _ := s.browserLoginStatus(op.ID); result.State != "canceled" {
				t.Fatalf("late capture changed state: %+v", result)
			}
			expected := int32(0)
			if action == "replacement" {
				expected = 1
			}
			if calls.Load() != expected {
				t.Fatal("late capture minted token")
			}
			if s.status().Connected != (action == "replacement") {
				t.Fatal("late capture changed account")
			}
		})
	}
}
func TestBrowserLoginReplacementAndSafeFailures(t *testing.T) {
	a, _, _ := fixtureCookieRuntime(t)
	s := a.spotifyAuth
	entered, canceled := make(chan struct{}), make(chan struct{})
	s.login.capture = func(ctx context.Context) (string, error) {
		close(entered)
		<-ctx.Done()
		close(canceled)
		return "", ctx.Err()
	}
	old, _ := s.startBrowserLogin()
	<-entered
	s.login.mu.Lock()
	s.login.capture = func(context.Context) (string, error) { return "", errors.New("sensitive-cookie-or-browser-path") }
	s.login.mu.Unlock()
	next, _ := s.startBrowserLogin()
	<-canceled
	if old.ID == next.ID {
		t.Fatal("operation ID reused")
	}
	result := waitBrowserLogin(t, s, next.ID, "failed")
	raw, _ := json.Marshal(result)
	if strings.Contains(string(raw), "sensitive") {
		t.Fatal("raw error escaped")
	}
	if _, ok := s.browserLoginStatus(old.ID); ok {
		t.Fatal("old operation still selected")
	}
}
func TestBrowserLoginSafeErrorMessages(t *testing.T) {
	for _, failure := range []error{context.DeadlineExceeded, spotifyauth.ErrLoginBrowserUnavailable, spotifyauth.ErrLoginBrowserClosed, spotifyauth.ErrLoginProfileCleanup} {
		t.Run(failure.Error(), func(t *testing.T) {
			a, _, _ := fixtureCookieRuntime(t)
			s := a.spotifyAuth
			s.login.capture = func(context.Context) (string, error) { return "", failure }
			op, _ := s.startBrowserLogin()
			result := waitBrowserLogin(t, s, op.ID, "failed")
			if result.Message == "" || s.status().Connected {
				t.Fatal("failed capture connected or has no useful error")
			}
		})
	}
}

func TestBrowserLoginStorageFailurePreservesConnectedAccount(t *testing.T) {
	a, _, _ := fixtureCookieRuntime(t)
	s := a.spotifyAuth
	if err := s.connect(context.Background(), "fixture-cookie"); err != nil {
		t.Fatal(err)
	}
	lifetime := s.lifetime
	s.storeSession = func(string) error { return errors.New("storage unavailable") }
	s.login.capture = func(context.Context) (string, error) { return "fixture-cookie", nil }
	op, _ := s.startBrowserLogin()
	waitBrowserLogin(t, s, op.ID, "failed")
	if !s.status().Connected || s.lifetime != lifetime || lifetime.Err() != nil {
		t.Fatal("storage failure retired prior account")
	}
	if _, err := s.Token(context.Background(), spotifyauth.Playback); err != nil {
		t.Fatal(err)
	}
}

func TestBrowserLoginLaunchRejectsUntrustedBrowserRequests(t *testing.T) {
	for _, tc := range []struct {
		origin, kind, site string
		allowed            bool
	}{
		{"http://127.0.0.1:34219", "application/json", "same-origin", true},
		{"http://localhost:3000", "application/json", "same-site", true},
		{"http://wails.localhost", "application/json", "", true},
		{"wails://wails", "application/json", "", true},
		{"wails://evil.test", "application/json", "", false},
		{"https://wails.localhost:1234", "application/json", "", true},
		{"", "application/json", "", true},
		{"https://evil.test", "application/json", "cross-site", false},
		{"null", "application/json", "", false},
		{"http://127.0.0.1:34219", "text/plain", "", false},
		{"", "application/x-www-form-urlencoded", "", false},
		{"", "application/json", "cross-site", false},
		{"http://wails.localhost.evil.test", "application/json", "", false},
	} {
		r := httptest.NewRequest("POST", "http://127.0.0.1:34219/api/spotify/auth/browser-login", nil)
		r.Header.Set("Origin", tc.origin)
		r.Header.Set("Content-Type", tc.kind)
		r.Header.Set("Sec-Fetch-Site", tc.site)
		if spotifyLoginRequestAllowed(r) != tc.allowed {
			t.Fatalf("wrong origin decision: %+v", tc)
		}
	}
}
