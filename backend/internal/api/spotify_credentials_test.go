package api

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/ajbergh/viib-mediahub/internal/db"
)

func TestGetSpotifyCredentialsTreatsMissingOrMalformedValuesAsUnconfigured(t *testing.T) {
	database, err := db.New(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	api := &API{db: database}

	for _, value := range []string{"", "legacy non-json value"} {
		t.Run(value, func(t *testing.T) {
			if err := database.SetSetting("spotify_credentials", value); err != nil {
				t.Fatal(err)
			}
			response := httptest.NewRecorder()
			api.getSpotifyCredentials(response, httptest.NewRequest(http.MethodGet, "/api/spotify/credentials", nil))
			if response.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200: %s", response.Code, response.Body.String())
			}
			if response.Body.String() != "{}\n" {
				t.Fatalf("response = %q, want empty credentials object", response.Body.String())
			}
		})
	}
}

func TestGetSpotifyCredentialsTreatsUnavailableStoreAsUnconfigured(t *testing.T) {
	database, err := db.New(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	api := &API{db: database}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}

	response := httptest.NewRecorder()
	api.getSpotifyCredentials(response, httptest.NewRequest(http.MethodGet, "/api/spotify/credentials", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", response.Code, response.Body.String())
	}
	if response.Body.String() != "{}\n" {
		t.Fatalf("response = %q, want empty credentials object", response.Body.String())
	}
}
