package api

import (
	"context"
	"encoding/json"
	"errors"
	spotifyauth "github.com/ajbergh/viib-mediahub/internal/spotify/auth"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/db"
	"github.com/ajbergh/viib-mediahub/internal/spotify/catalog"
)

func TestCatalogSnapshotAccountFencingAndRetirement(t *testing.T) {
	a, _, _ := fixtureCookieRuntime(t)
	// Install the production retirement hooks before switching accounts.
	runtime := a.spotifyTokens()
	if err := runtime.connect(context.Background(), "fixture-cookie"); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := runtime.requestContext(context.Background())
	defer cancel()
	id := strings.Repeat("T", 22)
	entity := catalog.CapturedEntity{EntityType: "track", ID: id, Resource: "getTrack:page:0:0", Payload: []byte(`{"uri":"spotify:track:` + id + `","unknown":0,"token":"private"}`)}
	runtime.persistCatalogDomain(ctx, []catalog.CapturedEntity{entity})
	key := db.SpotifySnapshotKey{EntityType: "track", SpotifyID: id, Resource: entity.Resource, ContextKey: runtime.metadataContext}
	got, err := a.db.GetSpotifyEntitySnapshot(key)
	if err != nil || got == nil || strings.Contains(string(got.Payload), "private") {
		t.Fatalf("capture failed: %+v %v", got, err)
	}
	if err := runtime.connect(context.Background(), "fixture-cookie"); err != nil {
		t.Fatal(err)
	}
	if runtime.metadataContext == key.ContextKey {
		t.Fatal("context reused across replacement")
	}
	runtime.persistCatalogDomain(ctx, []catalog.CapturedEntity{entity})
	got, err = a.db.GetSpotifyEntitySnapshot(key)
	if err != nil || got != nil {
		t.Fatal("retired request published private snapshot")
	}
	fresh, freshCancel := runtime.requestContext(context.Background())
	defer freshCancel()
	runtime.persistCatalogDomain(fresh, []catalog.CapturedEntity{entity})
	key.ContextKey = runtime.metadataContext
	got, err = a.db.GetSpotifyEntitySnapshot(key)
	if err != nil || got == nil {
		t.Fatal("fresh account failed to capture")
	}
	if err := runtime.disconnect(); err != nil {
		t.Fatal(err)
	}
	got, err = a.db.GetSpotifyEntitySnapshot(key)
	if err != nil || got != nil {
		t.Fatal("logout retained private snapshot")
	}
}

func TestOAuthCredentialSaveRetiresSnapshotsAndSurvivesRestart(t *testing.T) {
	a, _ := newBPMRouteTestAPI(t, false)
	runtime := a.spotifyTokens()
	old, cancel := runtime.requestContext(t.Context())
	defer cancel()
	id := strings.Repeat("T", 22)
	entity := catalog.CapturedEntity{EntityType: "track", ID: id, Resource: "rest-detail", Payload: []byte(`{"id":"` + id + `","unknown":0}`)}
	runtime.persistCatalogDomain(old, []catalog.CapturedEntity{entity})
	key := db.SpotifySnapshotKey{EntityType: "track", SpotifyID: id, Resource: entity.Resource, ContextKey: runtime.metadataContext}
	raw, _ := json.Marshal(SpotifyCredentials{ClientId: "new-client", AccessToken: "new-token", RefreshToken: "new-refresh", Expiry: time.Now().Add(time.Hour).UnixMilli()})
	w := httptest.NewRecorder()
	a.saveSpotifyCredentials(w, httptest.NewRequest("POST", "/spotify/credentials", strings.NewReader(string(raw))))
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if runtime.metadataContext == key.ContextKey || runtime.cookieMode {
		t.Fatal("OAuth context not replaced")
	}
	select {
	case <-old.Done():
	case <-time.After(time.Second):
		t.Fatal("old request survived replacement")
	}
	runtime.persistCatalogDomain(old, []catalog.CapturedEntity{entity})
	got, err := a.db.GetSpotifyEntitySnapshot(key)
	if err != nil || got != nil {
		t.Fatal("old snapshot survived or late request wrote", err)
	}
	if runtime.pendingOwner == nil || runtime.pendingOwner.Provider != "oauth" {
		t.Fatal("replacement credentials were not left pending profile confirmation")
	}
	fresh, stop := runtime.requestContext(t.Context())
	runtime.persistCatalogDomain(fresh, []catalog.CapturedEntity{entity})
	key.ContextKey = runtime.metadataContext
	got, err = a.db.GetSpotifyEntitySnapshot(key)
	if err != nil || got != nil {
		t.Fatal("unconfirmed replacement published private metadata", err)
	}
	stop()
	verifyOwner := runtime.ownerVerifier
	runtime.ownerVerifier = func(context.Context) error { return errors.New("profile confirmation unavailable") }
	if _, err := runtime.Token(t.Context(), spotifyauth.WebAPI); err == nil || runtime.pendingOwner == nil {
		t.Fatal("provider token was admitted before owner confirmation", err)
	}
	runtime.ownerVerifier = verifyOwner
	a.spotifyHTTPClient = &http.Client{Transport: sessionTransport(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"id":"new-account"}`))}, nil
	})}
	profile := httptest.NewRecorder()
	a.spotifyGetUserProfile(profile, httptest.NewRequest("GET", "/spotify/me", nil))
	if profile.Code != http.StatusOK || runtime.pendingOwner != nil {
		t.Fatalf("profile did not confirm replacement owner: status=%d body=%s", profile.Code, profile.Body.String())
	}
	fresh, stop = runtime.requestContext(t.Context())
	defer stop()
	runtime.persistCatalogDomain(fresh, []catalog.CapturedEntity{entity})
	got, err = a.db.GetSpotifyEntitySnapshot(key)
	if err != nil || got == nil {
		t.Fatal("confirmed context could not persist", err)
	}
	restarted := newSpotifyAuthRuntime(a.db, spotifyauth.WebPlayerOptions{})
	a.spotifyAuth = restarted
	a.spotifyTokens()
	token, err := restarted.Token(t.Context(), spotifyauth.WebAPI)
	if err != nil || restarted.cookieMode || token.Bearer() != "new-token" {
		t.Fatal("restart did not select replacement OAuth", err)
	}
}

func TestSavedOAuthCredentialsWithoutOwnerRequireProfileBeforeToken(t *testing.T) {
	a, _ := newBPMRouteTestAPI(t, false)
	raw, _ := json.Marshal(SpotifyCredentials{ClientId: "client", AccessToken: "startup-token", Expiry: time.Now().Add(time.Hour).UnixMilli()})
	if err := a.db.SetSetting("spotify_credentials", string(raw)); err != nil {
		t.Fatal(err)
	}
	runtime := newSpotifyAuthRuntime(a.db, spotifyauth.WebPlayerOptions{})
	a.spotifyAuth = runtime
	t.Cleanup(runtime.close)
	if runtime.pendingOwner == nil || runtime.pendingOwner.Provider != "oauth" {
		t.Fatal("saved credentials without a verified owner were not fenced")
	}
	active, err := a.db.GetSetting("spotify_metadata_active_context")
	if err != nil || active != "" {
		t.Fatalf("unverified context was activated: %q %v", active, err)
	}
	var routes []string
	a.spotifyHTTPClient = &http.Client{Transport: sessionTransport(func(r *http.Request) (*http.Response, error) {
		routes = append(routes, r.URL.Path)
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"id":"startup-account"}`))}, nil
	})}
	runtime.ownerVerifier = func(ctx context.Context) error { return a.revalidateSpotifyOwner(ctx, runtime) }
	token, err := runtime.Token(t.Context(), spotifyauth.WebAPI)
	if err != nil || token.Bearer() != "startup-token" || runtime.pendingOwner != nil {
		t.Fatalf("confirmed startup credentials were not admitted: token=%q pending=%+v err=%v", token.Bearer(), runtime.pendingOwner, err)
	}
	if len(routes) != 1 || routes[0] != "/v1/me" {
		t.Fatalf("provider work escaped before profile confirmation: %v", routes)
	}
}
func TestOAuthCredentialReplacementProfileReturnsAfterConfirmation(t *testing.T) {
	a, _ := newBPMRouteTestAPI(t, false)
	runtime := a.spotifyTokens()
	oldContext := runtime.metadataContext
	confirmTestSpotifyMetadataOwner(t, runtime, "same-account")
	raw, _ := json.Marshal(SpotifyCredentials{ClientId: "new-client", AccessToken: "new-token", Expiry: time.Now().Add(time.Hour).UnixMilli()})
	if err := runtime.replaceOAuthCredentials(string(raw)); err != nil {
		t.Fatal(err)
	}
	provisionalContext := runtime.metadataContext
	if runtime.pendingOwner == nil || provisionalContext == oldContext {
		t.Fatal("replacement was not held in a provisional context")
	}
	a.spotifyHTTPClient = &http.Client{Transport: sessionTransport(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"id":"same-account"}`))}, nil
	})}
	w := httptest.NewRecorder()
	a.spotifyGetUserProfile(w, httptest.NewRequest("GET", "/spotify/me", nil))
	if w.Code != http.StatusOK || runtime.pendingOwner != nil || runtime.metadataContext != provisionalContext {
		t.Fatalf("profile confirmation should complete the response and activate the provisional context: status=%d context=%q body=%s", w.Code, runtime.metadataContext, w.Body.String())
	}
}
func TestOAuthReplacementRetirementFailureStaysFenced(t *testing.T) {
	a, _ := newBPMRouteTestAPI(t, false)
	runtime := a.spotifyTokens()
	old, cancel := runtime.requestContext(t.Context())
	defer cancel()
	lifetime := runtime.lifetime
	before, _ := a.db.GetSetting("spotify_credentials")
	runtime.setRetirementHooks(nil, func() error { return errors.New("fixture retirement storage failure") })
	if err := runtime.replaceOAuthCredentials(`{"accessToken":"replacement"}`); err == nil {
		t.Fatal("retirement error ignored")
	}
	if lifetime.Err() == nil || runtime.lifetime != lifetime {
		t.Fatal("failed replacement exposed a fresh lifetime")
	}
	after, _ := a.db.GetSetting("spotify_credentials")
	if after != before {
		t.Fatal("replacement credentials committed before retirement succeeded")
	}
	if _, err := runtime.Token(old, spotifyauth.WebAPI); err == nil {
		t.Fatal("retired request still supplied credentials")
	}
}

func TestOAuthLogoutClearsCredentialsAndRetiresContext(t *testing.T) {
	a, _ := newBPMRouteTestAPI(t, false)
	runtime := a.spotifyTokens()
	raw, _ := json.Marshal(SpotifyCredentials{ClientId: "client", AccessToken: "token", RefreshToken: "refresh", Expiry: time.Now().Add(time.Hour).UnixMilli()})
	if err := runtime.replaceOAuthCredentials(string(raw)); err != nil {
		t.Fatal(err)
	}
	old, stop := runtime.requestContext(t.Context())
	defer stop()
	id := strings.Repeat("T", 22)
	entity := catalog.CapturedEntity{EntityType: "track", ID: id, Resource: "logout-fixture", Payload: []byte(`{"id":"` + id + `"}`)}
	runtime.persistCatalogDomain(old, []catalog.CapturedEntity{entity})
	key := db.SpotifySnapshotKey{EntityType: "track", SpotifyID: id, Resource: entity.Resource, ContextKey: runtime.metadataContext}
	if err := runtime.disconnect(); err != nil {
		t.Fatal(err)
	}
	stored, err := a.db.GetSetting("spotify_credentials")
	if err != nil || stored != "" {
		t.Fatal("OAuth credentials survived logout", err)
	}
	got, err := a.db.GetSpotifyEntitySnapshot(key)
	if err != nil || got != nil {
		t.Fatal("private snapshot survived logout", err)
	}
	select {
	case <-old.Done():
	case <-time.After(time.Second):
		t.Fatal("old lifetime survived logout")
	}
	restarted := newSpotifyAuthRuntime(a.db, spotifyauth.WebPlayerOptions{})
	if _, err := restarted.Token(t.Context(), spotifyauth.WebAPI); err == nil {
		t.Fatal("logout recovered OAuth credentials after restart")
	}
}
