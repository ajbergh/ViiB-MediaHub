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
	fresh, stop := runtime.requestContext(t.Context())
	defer stop()
	runtime.persistCatalogDomain(fresh, []catalog.CapturedEntity{entity})
	key.ContextKey = runtime.metadataContext
	got, err = a.db.GetSpotifyEntitySnapshot(key)
	if err != nil || got == nil {
		t.Fatal("new context could not persist", err)
	}
	restarted := newSpotifyAuthRuntime(a.db, spotifyauth.WebPlayerOptions{})
	token, err := restarted.Token(t.Context(), spotifyauth.WebAPI)
	if err != nil || restarted.cookieMode || token.Bearer() != "new-token" {
		t.Fatal("restart did not select replacement OAuth", err)
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
