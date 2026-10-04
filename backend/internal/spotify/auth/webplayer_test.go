package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

var fixtureNow = time.Unix(1777993436, 0)

func fixtureContract() WebPlayerContract {
	return WebPlayerContract{Version: "61", Secret: []byte("synthetic-contract-key"), AppVersion: "fixture-version", Revision: "fixture"}
}
func fixtureClient(server *httptest.Server) *http.Client {
	target, _ := url.Parse(server.URL)
	return &http.Client{Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		clone := r.Clone(r.Context())
		u := *r.URL
		u.Scheme, u.Host = target.Scheme, target.Host
		clone.URL = &u
		return server.Client().Transport.RoundTrip(clone)
	})}
}
func providerFor(t *testing.T, server *httptest.Server) *WebPlayerProvider {
	t.Helper()
	provider, err := NewWebPlayerProvider("fixture-session", fixtureContract(), WebPlayerOptions{Client: fixtureClient(server), Now: func() time.Time { return fixtureNow }})
	if err != nil {
		t.Fatal(err)
	}
	return provider
}
func tokenReply(w http.ResponseWriter, bearer string) {
	_ = json.NewEncoder(w).Encode(map[string]any{"accessToken": bearer, "isAnonymous": false, "accessTokenExpirationTimestampMs": fixtureNow.Add(time.Hour).UnixMilli()})
}
func TestWebPlayerRefreshAndCookieBoundary(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/server-time" {
			if r.Header.Get("Cookie") != "" {
				t.Error("cookie sent to server-time")
			}
			fmt.Fprint(w, `{"serverTime":1777993496}`)
			return
		}
		if r.URL.Path != "/api/token" {
			t.Error("unexpected route")
		}
		cookie, err := r.Cookie("sp_dc")
		if err != nil || cookie.Value != "fixture-session" {
			t.Error("missing session cookie")
		}
		clientCode, _ := TOTP(fixtureContract().Secret, fixtureNow)
		serverCode, _ := TOTP(fixtureContract().Secret, fixtureNow.Add(time.Minute))
		if r.URL.Query().Get("totp") != clientCode || r.URL.Query().Get("totpServer") != serverCode ||
			r.URL.Query().Get("totpVer") != "61" || r.URL.Query().Get("productType") != "web-player" {
			t.Error("invalid token request contract")
		}
		tokenReply(w, fmt.Sprintf("fixture-token-%d", calls.Add(1)))
	}))
	defer server.Close()
	p := providerFor(t, server)
	old, err := p.Token(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	next, err := p.Refresh(context.Background(), old)
	if err != nil || next.Bearer() == old.Bearer() {
		t.Fatalf("refresh: %v", err)
	}
	again, err := p.Refresh(context.Background(), old)
	if err != nil || again.Bearer() != next.Bearer() || calls.Load() != 2 {
		t.Fatal("stale rejection refreshed replacement again")
	}
}
func TestWebPlayerCoalescesConcurrentLoads(t *testing.T) {
	var calls atomic.Int32
	entered, release := make(chan struct{}), make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/server-time" {
			fmt.Fprint(w, `{"serverTime":1777993436}`)
			return
		}
		if calls.Add(1) == 1 {
			close(entered)
		}
		<-release
		tokenReply(w, "shared")
	}))
	defer server.Close()
	p := providerFor(t, server)
	var wg sync.WaitGroup
	for range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			token, err := p.Token(context.Background())
			if err != nil || token.Bearer() != "shared" {
				t.Errorf("load: %v", err)
			}
		}()
	}
	<-entered
	close(release)
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatalf("token requests: %d", calls.Load())
	}
}
func TestWebPlayerDisconnectRejectsLateToken(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/server-time" {
			fmt.Fprint(w, `{"serverTime":1777993436}`)
			return
		}
		close(entered)
		<-release
		tokenReply(w, "late-token")
	}))
	defer server.Close()
	p := providerFor(t, server)
	done := make(chan error, 1)
	go func() { _, err := p.Token(context.Background()); done <- err }()
	<-entered
	p.Disconnect()
	close(release)
	if err := <-done; !errors.Is(err, ErrAuthenticationRequired) {
		t.Fatalf("late response: %v", err)
	}
	if _, err := p.Token(context.Background()); !errors.Is(err, ErrAuthenticationRequired) {
		t.Fatalf("disconnected: %v", err)
	}
}
func TestWebPlayerFailuresAreRedacted(t *testing.T) {
	for _, test := range []struct {
		name, body string
		status     int
		want       error
	}{
		{"anonymous", `{"accessToken":"body-secret","isAnonymous":true,"accessTokenExpirationTimestampMs":9999999999999}`, 200, ErrAuthenticationRequired},
		{"missing-auth-state", `{"accessToken":"body-secret","accessTokenExpirationTimestampMs":9999999999999}`, 200, ErrProviderChanged},
		{"bad-expiry", `{"accessToken":"body-secret","isAnonymous":false,"accessTokenExpirationTimestampMs":1}`, 200, ErrProviderChanged},
		{"malformed", "<html>body-secret</html>", 200, ErrProviderChanged},
		{"unauthorized", "body-secret", 401, ErrAuthenticationRequired},
		{"forbidden", "body-secret", 403, nil},
		{"rate-limit", "body-secret", 429, nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/server-time" {
					fmt.Fprint(w, `{"serverTime":1777993436}`)
					return
				}
				w.Header().Set("Retry-After", "120")
				w.WriteHeader(test.status)
				fmt.Fprint(w, test.body)
			}))
			defer server.Close()
			_, err := providerFor(t, server).Token(context.Background())
			if err == nil || strings.Contains(err.Error(), "body-secret") || strings.Contains(err.Error(), "fixture-session") {
				t.Fatal("missing or unredacted failure")
			}
			if test.want != nil && !errors.Is(err, test.want) {
				t.Fatalf("wrong failure: %v", err)
			}
			if test.status == 429 {
				var denied *WebPlayerHTTPError
				if !errors.As(err, &denied) || denied.RetryAfter != 120*time.Second {
					t.Fatal("lost cooldown")
				}
			}
		})
	}
}
func TestWebPlayerRejectsCredentialRedirect(t *testing.T) {
	var destinationCalls atomic.Int32
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { destinationCalls.Add(1) }))
	defer destination.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/server-time" {
			fmt.Fprint(w, `{"serverTime":1777993436}`)
			return
		}
		http.Redirect(w, r, destination.URL, http.StatusTemporaryRedirect)
	}))
	defer server.Close()
	_, err := providerFor(t, server).Token(context.Background())
	if err == nil || destinationCalls.Load() != 0 {
		t.Fatal("followed credential redirect")
	}
}
func TestRetryAfterBothForms(t *testing.T) {
	if RetryAfter("90", fixtureNow) != 90*time.Second || RetryAfter(fixtureNow.Add(time.Minute).UTC().Format(http.TimeFormat), fixtureNow) != time.Minute || RetryAfter("-1", fixtureNow) != 0 {
		t.Fatal("invalid retry delay")
	}
}

func TestWebPlayerInvalidInputAndFormat(t *testing.T) {
	for _, cookie := range []string{"", "value;other=secret", "value\r\nInjected: true", "value with space"} {
		if _, err := NewWebPlayerProvider(cookie, fixtureContract(), WebPlayerOptions{}); !errors.Is(err, ErrAuthenticationRequired) {
			t.Fatal("unsafe cookie accepted")
		}
	}
	p, err := NewWebPlayerProvider("fixture-session", fixtureContract(), WebPlayerOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, formatted := range []string{fmt.Sprintf("%v", p), fmt.Sprintf("%+v", p), fmt.Sprintf("%#v", p)} {
		if strings.Contains(formatted, "fixture-session") {
			t.Fatal("provider disclosure")
		}
	}
	if _, err := NewWebPlayerProvider("fixture-session", WebPlayerContract{}, WebPlayerOptions{}); !errors.Is(err, ErrDisabled) {
		t.Fatal("missing contract accepted")
	}
}

func TestWebPlayerRefreshesBeforeExpiry(t *testing.T) {
	var clock atomic.Int64
	clock.Store(fixtureNow.Unix())
	var calls atomic.Int32
	now := func() time.Time { return time.Unix(clock.Load(), 0) }
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/server-time" {
			_ = json.NewEncoder(w).Encode(map[string]any{"serverTime": clock.Load()})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"accessToken": fmt.Sprintf("token-%d", calls.Add(1)), "isAnonymous": false, "accessTokenExpirationTimestampMs": now().Add(time.Hour).UnixMilli()})
	}))
	defer server.Close()
	p, err := NewWebPlayerProvider("fixture-session", fixtureContract(), WebPlayerOptions{Client: fixtureClient(server), Now: now})
	if err != nil {
		t.Fatal(err)
	}
	first, err := p.Token(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	clock.Add(59 * 60)
	second, err := p.Token(context.Background())
	if err != nil || first.Bearer() == second.Bearer() || calls.Load() != 2 {
		t.Fatalf("expiry refresh: %v", err)
	}
}

func TestWebPlayerAutomaticExpiryAndReconnect(t *testing.T) {
	for _, advance := range []time.Duration{59 * time.Minute, time.Hour + time.Second} {
		t.Run(advance.String(), func(t *testing.T) {
			var clock atomic.Int64
			clock.Store(fixtureNow.UnixNano())
			now := func() time.Time { return time.Unix(0, clock.Load()) }
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/server-time" {
					_ = json.NewEncoder(w).Encode(map[string]any{"serverTime": now().Unix()})
					return
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"accessToken": fmt.Sprintf("fixture-%d", calls.Add(1)), "isAnonymous": false, "accessTokenExpirationTimestampMs": now().Add(time.Hour).UnixMilli()})
			}))
			defer server.Close()
			opts := WebPlayerOptions{Client: fixtureClient(server), Now: now}
			p, err := NewWebPlayerProvider("fixture-session", fixtureContract(), opts)
			if err != nil {
				t.Fatal(err)
			}
			defer p.Disconnect()
			initial, err := p.Token(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			cached, err := p.Token(context.Background())
			if err != nil || cached.Bearer() != initial.Bearer() || calls.Load() != 1 {
				t.Fatal("fresh token was not reused")
			}
			clock.Store(fixtureNow.Add(advance).UnixNano())
			replacement, err := p.Token(context.Background())
			if err != nil || replacement.Bearer() == initial.Bearer() || calls.Load() != 2 {
				t.Fatalf("automatic renewal failed: %v calls=%d", err, calls.Load())
			}
			p.Disconnect()
			if _, err := p.Token(context.Background()); !errors.Is(err, ErrAuthenticationRequired) {
				t.Fatal("disconnected provider remained usable")
			}
			next, err := NewWebPlayerProvider("fixture-session", fixtureContract(), opts)
			if err != nil {
				t.Fatal(err)
			}
			defer next.Disconnect()
			reconnected, err := next.Token(context.Background())
			if err != nil || reconnected.Bearer() == replacement.Bearer() || calls.Load() != 3 {
				t.Fatalf("reconnect failed: %v", err)
			}
		})
	}
}
