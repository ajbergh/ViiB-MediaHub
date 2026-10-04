// Tests routing of cookie catalog requests and legacy OAuth token refresh through shared request and cooldown handling.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/db"
	spotifyauth "github.com/ajbergh/viib-mediahub/internal/spotify/auth"
)

func TestPKCERefreshDoesNotRequireClientSecret(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		if r.Form.Get("client_id") != "client-id" || r.Form.Get("client_secret") != "" {
			t.Fatalf("unexpected refresh form: %v", r.Form)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "new-access-token",
			"expires_in":   3600,
		})
	}))
	defer server.Close()

	previousEndpoint := spotifyTokenEndpoint
	spotifyTokenEndpoint = server.URL
	defer func() { spotifyTokenEndpoint = previousEndpoint }()

	database, err := db.New(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	credentials := SpotifyCredentials{
		ClientId:     "client-id",
		AccessToken:  "expired",
		RefreshToken: "refresh",
		Expiry:       time.Now().Add(-time.Minute).UnixMilli(),
	}
	raw, _ := json.Marshal(credentials)
	if err := database.SetSetting("spotify_credentials", string(raw)); err != nil {
		t.Fatal(err)
	}

	refreshed, err := loadValidSpotifyCredentials(context.Background(), database)
	if err != nil {
		t.Fatal(err)
	}
	if refreshed.AccessToken != "new-access-token" {
		t.Fatalf("unexpected token %q", refreshed.AccessToken)
	}
}

func TestSpotifyRequestRetriesOnceAfterUnauthorized(t *testing.T) {
	var resourceCalls int
	resource := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resourceCalls++
		if r.Header.Get("Authorization") == "Bearer old-access-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.Header.Get("Authorization") != "Bearer refreshed-access-token" {
			t.Fatalf("unexpected authorization header %q", r.Header.Get("Authorization"))
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer resource.Close()

	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "refreshed-access-token",
			"expires_in":   3600,
		})
	}))
	defer tokenServer.Close()

	previousEndpoint := spotifyTokenEndpoint
	spotifyTokenEndpoint = tokenServer.URL
	defer func() { spotifyTokenEndpoint = previousEndpoint }()

	database, err := db.New(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	credentials := SpotifyCredentials{
		ClientId:     "client-id",
		AccessToken:  "old-access-token",
		RefreshToken: "refresh-token",
		Expiry:       time.Now().Add(time.Hour).UnixMilli(),
	}
	raw, _ := json.Marshal(credentials)
	if err := database.SetSetting("spotify_credentials", string(raw)); err != nil {
		t.Fatal(err)
	}

	api := &API{db: database}
	response, err := api.doSpotifyRequest(context.Background(), http.MethodGet, resource.URL, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK || resourceCalls != 2 {
		t.Fatalf("expected one retry and 200, got calls=%d status=%d", resourceCalls, response.StatusCode)
	}
}

func TestSpotifyOAuthPurposeAdapterPreservesCredentials(t *testing.T) {
	database, err := db.New(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	original := SpotifyCredentials{ClientId: "client", AccessToken: "existing", RefreshToken: "refresh", Expiry: time.Now().Add(time.Hour).UnixMilli()}
	raw, _ := json.Marshal(original)
	if err := database.SetSetting("spotify_credentials", string(raw)); err != nil {
		t.Fatal(err)
	}
	manager := spotifyOAuthManager(database)
	for _, purpose := range []spotifyauth.Purpose{spotifyauth.WebAPI, spotifyauth.Playback} {
		token, err := manager.Token(context.Background(), purpose)
		if err != nil || token.Kind != spotifyauth.OAuth || token.Bearer() != original.AccessToken || token.ExpiresAt.UnixMilli() != original.Expiry {
			t.Fatalf("purpose adapter: %v", err)
		}
	}
	if _, err := manager.Token(context.Background(), spotifyauth.InternalAnalysis); !errors.Is(err, spotifyauth.ErrDisabled) {
		t.Fatalf("internal analysis enabled: %v", err)
	}
	stored, err := readSpotifyCredentials(database)
	if err != nil || stored != original {
		t.Fatalf("credential shape changed: %v", err)
	}
}
