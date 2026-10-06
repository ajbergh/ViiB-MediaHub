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
	"sync/atomic"
	"testing"
	"time"
)

func TestSpotifyProfileBindsAuthenticatedOwner(t *testing.T) {
	a, _ := newBPMRouteTestAPI(t, false)
	credentials, _ := json.Marshal(SpotifyCredentials{ClientId: "client", AccessToken: "fixture", Expiry: time.Now().Add(time.Hour).UnixMilli()})
	if err := a.db.SetSetting("spotify_credentials", string(credentials)); err != nil {
		t.Fatal(err)
	}
	runtime := a.spotifyTokens()
	a.spotifyHTTPClient = &http.Client{Transport: sessionTransport(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"id":"stable-account","display_name":"Name","future":false}`))}, nil
	})}
	w := httptest.NewRecorder()
	a.spotifyGetUserProfile(w, httptest.NewRequest("GET", "/spotify/me", nil))
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"future":false`) {
		t.Fatal("profile response changed", w.Code, w.Body.String())
	}
	if err := a.db.BindSpotifyMetadataOwner(runtime.metadataEpoch, "oauth", "changed-account", runtime.metadataContext); err == nil {
		t.Fatal("live context rebound to different identity")
	}
	owner, err := a.db.ReserveSpotifyMetadataRuntime("next-runtime")
	if err != nil || owner == nil || owner.Provider != "oauth" || owner.AccountID != "stable-account" || owner.ContextKey != runtime.metadataContext {
		t.Fatal("authenticated owner missing", owner, err)
	}
	if err := a.db.BindSpotifyMetadataOwner(runtime.metadataEpoch, "oauth", "late-account", runtime.metadataContext); err == nil {
		t.Fatal("stale runtime changed identity")
	}
}

func TestWebPlayerProfileBindsAuthenticatedOwner(t *testing.T) {
	a, _, _ := fixtureCookieRuntime(t)
	if err := a.spotifyAuth.connect(t.Context(), "fixture-cookie"); err != nil {
		t.Fatal(err)
	}
	a.spotifyHTTPClient = &http.Client{Transport: sessionTransport(func(r *http.Request) (*http.Response, error) {
		body := `{"granted_token":{"token":"client","expires_after_seconds":600}}`
		if r.URL.Host != "clienttoken.spotify.com" {
			body = `{"data":{"me":{"profile":{"username":"stable-user","uri":"spotify:user:stable-user","name":"User"}}}}`
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
	})}
	w := httptest.NewRecorder()
	a.spotifyGetUserProfile(w, httptest.NewRequest("GET", "/spotify/me", nil))
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	owner, err := a.db.ReserveSpotifyMetadataRuntime("next-runtime")
	if err != nil || owner == nil || owner.Provider != "webplayer" || owner.AccountID != "stable-user" || owner.ContextKey != a.spotifyAuth.metadataContext {
		t.Fatal("Web Player owner missing", owner, err)
	}
	previous := a.spotifyAuth
	a.spotifyAuth = newSpotifyAuthRuntime(a.db, previous.options)
	current := a.spotifyTokens()
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	if _, err := current.Token(ctx, spotifyauth.WebAPI); err != nil {
		t.Fatal("automatic Web Player validation failed or deadlocked", err)
	}
	if current.pendingOwner != nil || current.metadataContext != owner.ContextKey {
		t.Fatal("Web Player owner not restored")
	}
}

func TestRestartOwnerRequiresProfileBeforeCheckpointReuse(t *testing.T) {
	a, _ := newBPMRouteTestAPI(t, false)
	credentials, _ := json.Marshal(SpotifyCredentials{ClientId: "client", AccessToken: "fixture", Expiry: time.Now().Add(time.Hour).UnixMilli()})
	if err := a.db.SetSetting("spotify_credentials", string(credentials)); err != nil {
		t.Fatal(err)
	}
	old := a.spotifyTokens()
	if err := a.db.BindSpotifyMetadataOwner(old.metadataEpoch, "oauth", "account", old.metadataContext); err != nil {
		t.Fatal(err)
	}
	id := strings.Repeat("P", 22)
	checkpoint := completedPlaylistCapture(id, "revision", []spotifyPlaylistItem{{}, {}})[0]
	now := time.Now()
	if err := a.db.PutSpotifyEntitySnapshot(db.SpotifyEntitySnapshot{SpotifySnapshotKey: db.SpotifySnapshotKey{EntityType: "playlist", SpotifyID: id, Resource: checkpoint.Resource, ContextKey: old.metadataContext}, SchemaVersion: 1, AdapterRevision: "catalog-domain-v1", RetrievedAt: now, ExpiresAt: now.Add(time.Hour), Payload: checkpoint.Payload}); err != nil {
		t.Fatal(err)
	}
	a.spotifyAuth = newSpotifyAuthRuntime(a.db, spotifyauth.WebPlayerOptions{})
	current := a.spotifyAuth
	ctx, cancel := current.requestContext(t.Context())
	defer cancel()
	if current.metadataContext != old.metadataContext || current.metadataEpoch == old.metadataEpoch {
		t.Fatal("restart ownership not retained/fenced")
	}
	if _, ok := a.completedPlaylistRows(ctx, id, "revision"); ok {
		t.Fatal("pending cache reused")
	}
	if _, err := current.Token(ctx, spotifyauth.WebAPI); err == nil {
		t.Fatal("pending provider work admitted")
	}
	status := 503
	a.spotifyHTTPClient = &http.Client{Transport: sessionTransport(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"id":"account"}`))}, nil
	})}
	w := httptest.NewRecorder()
	a.spotifyGetUserProfile(w, httptest.NewRequest("GET", "/spotify/me", nil))
	if w.Code != 503 || current.pendingOwner == nil {
		t.Fatal("transient profile failure activated cache", w.Code)
	}
	status = 200
	w = httptest.NewRecorder()
	a.spotifyGetUserProfile(w, httptest.NewRequest("GET", "/spotify/me", nil))
	if w.Code != 200 || current.pendingOwner != nil {
		t.Fatal("matching profile not confirmed", w.Code, w.Body.String())
	}
	if rows, ok := a.completedPlaylistRows(ctx, id, "revision"); !ok || len(rows) != 2 {
		t.Fatal("confirmed checkpoint not reused", rows, ok)
	}
}

func TestRestartOwnerMismatchCancelsProvisionalLifetime(t *testing.T) {
	a, _ := newBPMRouteTestAPI(t, false)
	credentials, _ := json.Marshal(SpotifyCredentials{ClientId: "client", AccessToken: "fixture", Expiry: time.Now().Add(time.Hour).UnixMilli()})
	if err := a.db.SetSetting("spotify_credentials", string(credentials)); err != nil {
		t.Fatal(err)
	}
	old := a.spotifyTokens()
	if err := a.db.BindSpotifyMetadataOwner(old.metadataEpoch, "oauth", "old-account", old.metadataContext); err != nil {
		t.Fatal(err)
	}
	a.spotifyAuth = newSpotifyAuthRuntime(a.db, spotifyauth.WebPlayerOptions{})
	current := a.spotifyAuth
	ctx, cancel := current.requestContext(t.Context())
	defer cancel()
	a.spotifyHTTPClient = &http.Client{Transport: sessionTransport(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"id":"new-account"}`))}, nil
	})}
	w := httptest.NewRecorder()
	a.spotifyGetUserProfile(w, httptest.NewRequest("GET", "/spotify/me", nil))
	if w.Code != 503 || current.pendingOwner != nil || current.metadataContext == old.metadataContext || ctx.Err() == nil {
		t.Fatal("mismatch did not replace/cancel provisional owner", w.Code, ctx.Err())
	}
	w = httptest.NewRecorder()
	a.spotifyGetUserProfile(w, httptest.NewRequest("GET", "/spotify/me", nil))
	if w.Code != 200 {
		t.Fatal("fresh account retry failed", w.Code, w.Body.String())
	}
}

func TestAutomaticOwnerValidationSharesWorkAndIndependentCancellation(t *testing.T) {
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
	current := a.spotifyTokens()
	var calls atomic.Int32
	started, release := make(chan struct{}), make(chan struct{})
	a.spotifyHTTPClient = &http.Client{Transport: sessionTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/v1/me" {
			t.Error("unexpected validation route")
		}
		if calls.Add(1) == 1 {
			close(started)
		}
		select {
		case <-release:
		case <-r.Context().Done():
			return nil, r.Context().Err()
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"id":"account"}`))}, nil
	})}
	results := make(chan error, 4)
	canceled, stop := context.WithCancel(t.Context())
	defer stop()
	for i := 0; i < 4; i++ {
		parent := t.Context()
		if i == 0 {
			parent = canceled
		}
		go func() { _, err := current.Token(parent, spotifyauth.WebAPI); results <- err }()
	}
	<-started
	deadline := time.Now().Add(time.Second)
	for {
		current.ownerMu.Lock()
		waiters := 0
		if current.ownerFlight != nil {
			waiters = current.ownerFlight.waiters
		}
		current.ownerMu.Unlock()
		if waiters == 4 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("callers did not join")
		}
		time.Sleep(time.Millisecond)
	}
	stop()
	if err := <-results; !errors.Is(err, context.Canceled) {
		t.Fatal("canceled waiter did not leave", err)
	}
	close(release)
	for i := 0; i < 3; i++ {
		if err := <-results; err != nil {
			t.Fatal("remaining caller failed", err)
		}
	}
	if calls.Load() != 1 || current.pendingOwner != nil {
		t.Fatal("validation was duplicated or not committed", calls.Load())
	}
}

func TestAutomaticOwnerValidationCancellationStopsProfile(t *testing.T) {
	for _, kind := range []string{"last_waiter", "retirement"} {
		t.Run(kind, func(t *testing.T) {
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
			current := a.spotifyTokens()
			started, stopped := make(chan struct{}), make(chan struct{})
			a.spotifyHTTPClient = &http.Client{Transport: sessionTransport(func(r *http.Request) (*http.Response, error) {
				close(started)
				<-r.Context().Done()
				close(stopped)
				return nil, r.Context().Err()
			})}
			parent, cancel := context.WithCancel(t.Context())
			defer cancel()
			result := make(chan error, 1)
			go func() { _, err := current.Token(parent, spotifyauth.WebAPI); result <- err }()
			<-started
			current.ownerMu.Lock()
			flight := current.ownerFlight
			current.ownerMu.Unlock()
			if kind == "last_waiter" {
				cancel()
			} else {
				current.beginRetirement()
			}
			if err := <-result; err == nil {
				t.Fatal("canceled validation admitted token")
			}
			select {
			case <-stopped:
			case <-time.After(time.Second):
				t.Fatal("profile request not canceled")
			}
			<-flight.done
			if !current.ownerRetryAt.IsZero() || current.ownerFailures != 0 {
				t.Fatal("cancellation settled retry failure")
			}
		})
	}
}

func TestAutomaticOwnerValidationPrecedesPlaylistGeneration(t *testing.T) {
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
	id := strings.Repeat("P", 22)
	profiles := 0
	a.spotifyHTTPClient = &http.Client{Transport: sessionTransport(func(r *http.Request) (*http.Response, error) {
		body := `{"id":"account"}`
		if r.URL.Path == "/v1/me" {
			profiles++
		} else {
			body = `{"type":"playlist","id":"` + id + `","snapshot_id":"revision","tracks":{"offset":0,"items":[null],"next":null}}`
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
	})}
	if _, _, _, err := a.fetchPlaylistTracks(t.Context(), id, nil); err != nil {
		t.Fatal("first playlist request failed", err)
	}
	ctx, cancel := a.spotifyAuth.requestContext(t.Context())
	defer cancel()
	if rows, ok := a.completedPlaylistRows(ctx, id, "revision"); !ok || len(rows) != 1 || profiles != 1 {
		t.Fatal("first traversal did not publish after one validation", rows, ok, profiles)
	}
}
