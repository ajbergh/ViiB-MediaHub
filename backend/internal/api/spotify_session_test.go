package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/ajbergh/viib-mediahub/internal/crypto"
	"github.com/ajbergh/viib-mediahub/internal/db"
	spotifyauth "github.com/ajbergh/viib-mediahub/internal/spotify/auth"
	"github.com/ajbergh/viib-mediahub/internal/validation"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type sessionTransport func(*http.Request) (*http.Response, error)

func (f sessionTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func fixtureCookieRuntime(t *testing.T) (*API, string, *atomic.Int32) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "session.db")
	database, err := db.New(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	var calls atomic.Int32
	opts := spotifyauth.WebPlayerOptions{Client: &http.Client{Transport: sessionTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "open.spotify.com" {
			t.Fatal("wrong auth origin")
		}
		body := fmt.Sprintf(`{"serverTime":%d}`, time.Now().Unix())
		if r.URL.Path == "/api/token" {
			cookie, err := r.Cookie("sp_dc")
			if err != nil || cookie.Value != "fixture-cookie" {
				t.Fatal("missing cookie")
			}
			body = fmt.Sprintf(`{"accessToken":"fixture-bearer-%d","isAnonymous":false,"clientId":"fixture-public-client","accessTokenExpirationTimestampMs":%d}`, calls.Add(1), time.Now().Add(time.Hour).UnixMilli())
		} else if r.Header.Get("Cookie") != "" {
			t.Fatal("cookie escaped token endpoint")
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
	})}}
	runtime := newSpotifyAuthRuntime(database, opts)
	t.Cleanup(runtime.close)
	return &API{db: database, spotifyAuth: runtime}, path, &calls
}

func TestCookieRuntimeStorageRestoreAndLogoutWithoutFallback(t *testing.T) {
	a, path, calls := fixtureCookieRuntime(t)
	oauth := SpotifyCredentials{ClientId: "legacy-client", AccessToken: "legacy-bearer", Expiry: time.Now().Add(time.Hour).UnixMilli()}
	raw, _ := json.Marshal(oauth)
	if err := a.db.SetSetting("spotify_credentials", string(raw)); err != nil {
		t.Fatal(err)
	}
	if err := a.spotifyAuth.connect(context.Background(), "fixture-cookie"); err != nil {
		t.Fatal(err)
	}
	for _, purpose := range []spotifyauth.Purpose{spotifyauth.WebAPI, spotifyauth.Playback, spotifyauth.InternalAnalysis} {
		token, err := a.spotifyAuth.Token(context.Background(), purpose)
		if err != nil || token.Kind != spotifyauth.WebPlayer || token.Bearer() != "fixture-bearer-1" {
			t.Fatalf("purpose %v: %v", purpose, err)
		}
	}
	if calls.Load() != 1 {
		t.Fatal("shared token was not reused")
	}
	sqlDB, err := sql.Open("viib_sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	var stored string
	if err := sqlDB.QueryRow("SELECT value FROM settings WHERE key=?", spotifyCookieSetting).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if !crypto.IsEncrypted(stored) || strings.Contains(stored, "fixture-cookie") || strings.Contains(stored, "fixture-bearer") {
		t.Fatal("session is not encrypted")
	}
	if validation.IsValidSettingKey(spotifyCookieSetting) {
		t.Fatal("session exposed through generic settings")
	}
	restored := newSpotifyAuthRuntime(a.db, a.spotifyAuth.options)
	defer restored.close()
	token, err := restored.Token(context.Background(), spotifyauth.Playback)
	if err != nil || token.Kind != spotifyauth.WebPlayer || calls.Load() != 2 {
		t.Fatalf("restore: %v", err)
	}
	w := httptest.NewRecorder()
	a.getSpotifyCredentials(w, httptest.NewRequest("GET", "/spotify/credentials", nil))
	if strings.Contains(w.Body.String(), "legacy") || strings.Contains(w.Body.String(), "fixture") {
		t.Fatal("cookie mode exposes credential payload")
	}
	if err := a.spotifyAuth.disconnect(); err != nil {
		t.Fatal(err)
	}
	for _, purpose := range []spotifyauth.Purpose{spotifyauth.WebAPI, spotifyauth.Playback, spotifyauth.InternalAnalysis} {
		if _, err := a.spotifyAuth.Token(context.Background(), purpose); !errors.Is(err, spotifyauth.ErrAuthenticationRequired) {
			t.Fatalf("logout fallback: %v", err)
		}
	}
	afterLogout := newSpotifyAuthRuntime(a.db, a.spotifyAuth.options)
	defer afterLogout.close()
	if _, err := afterLogout.Token(context.Background(), spotifyauth.WebAPI); !errors.Is(err, spotifyauth.ErrAuthenticationRequired) {
		t.Fatal("restart fell back to OAuth")
	}
	status, _ := json.Marshal(afterLogout.status())
	if strings.Contains(string(status), "fixture") || strings.Contains(string(status), "legacy") {
		t.Fatal("status leaked secret")
	}
}

func TestCookieRequestRefreshOnceOriginIsolationAndRateLimit(t *testing.T) {
	for _, status := range []int{200, 401, 429} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			a, _, tokenCalls := fixtureCookieRuntime(t)
			if err := a.spotifyAuth.connect(context.Background(), "fixture-cookie"); err != nil {
				t.Fatal(err)
			}
			requests := 0
			a.spotifyHTTPClient = &http.Client{Transport: sessionTransport(func(r *http.Request) (*http.Response, error) {
				requests++
				if r.URL.Scheme != "https" || r.URL.Host != "api.spotify.com" || r.Header.Get("Cookie") != "" {
					t.Fatal("bearer origin isolation failed")
				}
				if r.Header.Get("Authorization") != fmt.Sprintf("Bearer fixture-bearer-%d", tokenCalls.Load()) {
					t.Fatal("wrong bearer")
				}
				responseStatus := status
				if status == 200 && requests == 1 {
					responseStatus = 401
				}
				return &http.Response{StatusCode: responseStatus, Header: http.Header{"Retry-After": []string{"31"}}, Body: io.NopCloser(strings.NewReader("{}"))}, nil
			})}
			response, err := a.doSpotifyRequest(context.Background(), "GET", "https://api.spotify.com/v1/me/player/recently-played?limit=1", nil, "")
			if status == 401 {
				if !errors.Is(err, spotifyauth.ErrAuthenticationRequired) || requests != 2 {
					t.Fatalf("rejection: %v calls=%d", err, requests)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			response.Body.Close()
			expectedRequests := 2
			if status == 429 {
				expectedRequests = 1
			}
			if requests != expectedRequests || response.StatusCode != status {
				t.Fatalf("calls=%d status=%d", requests, response.StatusCode)
			}
			before := requests
			if _, err := a.doSpotifyRequest(context.Background(), "GET", "https://untrusted.invalid/v1/me", nil, ""); err == nil || requests != before {
				t.Fatal("cookie bearer accepted arbitrary origin")
			}
		})
	}
}

func TestCookieSessionHandlersBoundedAndRedacted(t *testing.T) {
	a, _, _ := fixtureCookieRuntime(t)
	w := httptest.NewRecorder()
	a.connectSpotifySession(w, httptest.NewRequest("POST", "/spotify/auth/session", strings.NewReader(`{"spDC":"fixture-cookie"}`)))
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"provider":"webplayer"`) || strings.Contains(w.Body.String(), "fixture") {
		t.Fatalf("connect: %d %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	a.disconnectSpotifySession(w, httptest.NewRequest("DELETE", "/spotify/auth/session", nil))
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"connected":false`) {
		t.Fatalf("disconnect: %s", w.Body.String())
	}
	for _, body := range []string{`{"spDC":"bad cookie"}`, `{"spDC":"fixture-cookie","unexpected":true}`, strings.Repeat("x", 4097)} {
		w = httptest.NewRecorder()
		a.connectSpotifySession(w, httptest.NewRequest("POST", "/spotify/auth/session", strings.NewReader(body)))
		if w.Code == 200 || strings.Contains(w.Body.String(), "fixture-cookie") {
			t.Fatal("invalid request accepted or leaked")
		}
	}
	a.Close()
	if _, err := a.spotifyAuth.Token(context.Background(), spotifyauth.Playback); !errors.Is(err, spotifyauth.ErrAuthenticationRequired) {
		t.Fatal("closed runtime accepted token")
	}
}

func TestCookieAccountChangeCancelsInFlightRequests(t *testing.T) {
	a, _, _ := fixtureCookieRuntime(t)
	if err := a.spotifyAuth.connect(context.Background(), "fixture-cookie"); err != nil {
		t.Fatal(err)
	}
	entered := make(chan struct{})
	a.spotifyHTTPClient = &http.Client{Transport: sessionTransport(func(r *http.Request) (*http.Response, error) {
		close(entered)
		<-r.Context().Done()
		return nil, r.Context().Err()
	})}
	finished := make(chan error, 1)
	go func() {
		_, err := a.doSpotifyRequest(context.Background(), "GET", "https://api.spotify.com/v1/me/player/recently-played?limit=1", nil, "")
		finished <- err
	}()
	<-entered
	if err := a.spotifyAuth.disconnect(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-finished:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("request was not canceled: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("old-account request survived logout")
	}
}

func TestCookieRetirementWaitsForMediaPreparation(t *testing.T) {
	for _, replace := range []bool{false, true} {
		t.Run(fmt.Sprint(replace), func(t *testing.T) {
			a, _, _ := fixtureCookieRuntime(t)
			s := a.spotifyAuth
			if err := s.connect(context.Background(), "fixture-cookie"); err != nil {
				t.Fatal(err)
			}
			entered, canceled, release, retired := make(chan struct{}), make(chan struct{}), make(chan struct{}), make(chan struct{})
			s.setRetirementHooks(nil, func() error { close(retired); return nil })
			preparation := make(chan error, 1)
			go func() {
				preparation <- s.withPlayback(context.Background(), func(ctx context.Context, token spotifyauth.Token) error {
					if token.Bearer() != "fixture-bearer-1" {
						return errors.New("wrong initial token")
					}
					close(entered)
					<-ctx.Done()
					close(canceled)
					<-release
					return nil
				})
			}()
			<-entered
			changed := make(chan error, 1)
			go func() {
				if replace {
					changed <- s.connect(context.Background(), "fixture-cookie")
				} else {
					changed <- s.disconnect()
				}
			}()
			select {
			case <-canceled:
			case <-time.After(time.Second):
				close(release)
				t.Fatal("preparation was not canceled")
			}
			select {
			case <-retired:
				close(release)
				t.Fatal("retirement overtook old media setup")
			default:
			}
			close(release)
			if err := <-preparation; !errors.Is(err, context.Canceled) {
				t.Fatalf("stale setup succeeded: %v", err)
			}
			if err := <-changed; err != nil {
				t.Fatal(err)
			}
			select {
			case <-retired:
			default:
				t.Fatal("media was not retired")
			}
			token, err := s.Token(context.Background(), spotifyauth.Playback)
			if replace {
				if err != nil || token.Bearer() != "fixture-bearer-2" {
					t.Fatalf("new account token: %v", err)
				}
			} else if !errors.Is(err, spotifyauth.ErrAuthenticationRequired) {
				t.Fatal("logout retained media auth")
			}
		})
	}
}

func TestCookieRetirementCancelsDispatchedQueueAndRequeuesLateCleanup(t *testing.T) {
	a, _, _ := fixtureCookieRuntime(t)
	dm := NewDownloadManager(a.db, t.TempDir(), a.spotifyAuth)
	defer dm.cancel()
	a.downloadManager = dm
	source := a.spotifyTokens()
	if err := source.connect(context.Background(), "fixture-cookie"); err != nil {
		t.Fatal(err)
	}
	dm.ClearAuthRequired()
	id, err := dm.QueueDownload("5r9W9MJLvHk83fcZSPQ8SE", "spotify:track:5r9W9MJLvHk83fcZSPQ8SE", "track", "fixture", "fixture", "fixture", nil)
	if err != nil {
		t.Fatal(err)
	}
	dm.dispatchDownloads()
	var old downloadJob
	select {
	case old = <-dm.workChan:
	default:
		t.Fatal("job was not dispatched")
	}
	if _, err := a.db.MarkDownloadStarted(id); err != nil {
		t.Fatal(err)
	}
	if err := source.connect(context.Background(), "fixture-cookie"); err != nil {
		t.Fatal(err)
	}
	dm.ClearAuthRequired() // The old worker can finish after new login is visible.
	if !errors.Is(context.Cause(old.ctx), errSpotifyAccountChanged) {
		t.Fatal("dispatched job lost account identity")
	}
	dm.processDownload(0, old)
	queued, err := a.db.GetQueuedDownloads(10)
	if err != nil || len(queued) != 1 || queued[0].ID != id {
		t.Fatalf("unfinished queue lost on replacement: %v count=%d", err, len(queued))
	}
	dm.dispatchDownloads()
	select {
	case next := <-dm.workChan:
		if next.ctx.Err() != nil {
			t.Fatal("new dispatch reused canceled account context")
		}
		token, err := source.Token(next.ctx, spotifyauth.Playback)
		if err != nil || token.Bearer() != "fixture-bearer-2" {
			t.Fatal("new dispatch used old credentials")
		}
	default:
		t.Fatal("queue did not resume after replacement")
	}
}

func TestCookieStatusReportsCachedLifetimeWithoutProviderIO(t *testing.T) {
	a, _, calls := fixtureCookieRuntime(t)
	if err := a.spotifyAuth.connect(context.Background(), "fixture-cookie"); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		status := a.spotifyAuth.status()
		if status.TokenExpiresAt <= time.Now().UnixMilli() || status.TokenIssueCount != 1 {
			t.Fatalf("missing cached lifetime: %#v", status)
		}
		raw, err := json.Marshal(status)
		if err != nil {
			t.Fatal(err)
		}
		for _, secret := range []string{"fixture-cookie", "fixture-bearer", "fixture-public-client"} {
			if strings.Contains(string(raw), secret) {
				t.Fatal("credential escaped lifetime status")
			}
		}
	}
	if calls.Load() != 1 {
		t.Fatal("status read minted a token")
	}
	if err := a.spotifyAuth.disconnect(); err != nil {
		t.Fatal(err)
	}
	status := a.spotifyAuth.status()
	if status.TokenExpiresAt != 0 || status.TokenIssueCount != 0 {
		t.Fatal("disconnected lifetime remained visible")
	}
}
