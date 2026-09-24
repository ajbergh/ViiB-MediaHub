package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ajbergh/viib-mediahub/internal/db"
	"github.com/ajbergh/viib-mediahub/internal/scanner"
	"github.com/ajbergh/viib-mediahub/internal/validation"
	"github.com/go-chi/chi/v5"
)

func TestSetDownloadRescanThresholdPersistsAndUpdatesScanner(t *testing.T) {
	tempDir := t.TempDir()
	database, err := db.New(filepath.Join(tempDir, "test.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	sc := scanner.New(database, tempDir)
	t.Cleanup(sc.Close)
	sc.SetSpotifyDownloadDir(filepath.Join(tempDir, "downloads"))
	a := &API{db: database, dataDir: tempDir, scanner: sc}

	response := setSettingForTest(a, "spotify_download_rescan_threshold", "3")
	if response.Code != http.StatusOK {
		t.Fatalf("set threshold returned %d: %s", response.Code, response.Body.String())
	}
	value, err := database.GetSetting("spotify_download_rescan_threshold")
	if err != nil {
		t.Fatalf("get persisted threshold: %v", err)
	}
	if value != "3" {
		t.Fatalf("persisted threshold = %q, want 3", value)
	}
	if sc.NotifyDownloadComplete() {
		t.Fatal("runtime scanner reached persisted threshold on first download")
	}
	if sc.NotifyDownloadComplete() {
		t.Fatal("runtime scanner reached persisted threshold on second download")
	}
	if !sc.NotifyDownloadComplete() {
		t.Fatal("runtime scanner did not reach persisted threshold on third download")
	}

	response = setSettingForTest(a, "spotify_download_rescan_threshold", "-1")
	if response.Code != http.StatusBadRequest {
		t.Fatalf("negative threshold returned %d, want 400", response.Code)
	}
	value, err = database.GetSetting("spotify_download_rescan_threshold")
	if err != nil {
		t.Fatalf("get threshold after rejected update: %v", err)
	}
	if value != "3" {
		t.Fatalf("rejected update changed persisted threshold to %q", value)
	}
}

// The analysis toggle is a regular persisted setting. Keep this at the HTTP
// boundary because a missing validation allowlist entry otherwise makes the UI
// checkbox appear to change briefly while the POST is rejected with a 400.
func TestSetAutoAnalyzeNewTracksPersists(t *testing.T) {
	tempDir := t.TempDir()
	database, err := db.New(filepath.Join(tempDir, "test.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	a := &API{db: database, dataDir: tempDir}

	response := setSettingForTest(a, SettingAutoAnalyzeNewTracks, "true")
	if response.Code != http.StatusOK {
		t.Fatalf("set auto-analyze returned %d: %s", response.Code, response.Body.String())
	}
	value, err := database.GetSetting(SettingAutoAnalyzeNewTracks)
	if err != nil {
		t.Fatalf("get persisted auto-analyze setting: %v", err)
	}
	if value != "true" {
		t.Fatalf("persisted auto-analyze setting = %q, want true", value)
	}
}

func TestAutomaticCuePointsSettingDefaultsAndValidatesServerSide(t *testing.T) {
	tempDir := t.TempDir()
	database, err := db.New(filepath.Join(tempDir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	a := &API{db: database, dataDir: tempDir}
	if !validation.IsValidSettingKey(SettingAutoCueMode) {
		t.Fatal("automatic cue mode key is not allowlisted")
	}

	get := func() *httptest.ResponseRecorder {
		routeContext := chi.NewRouteContext()
		routeContext.URLParams.Add("key", SettingAutoCueMode)
		req := httptest.NewRequest(http.MethodGet, "/settings/"+SettingAutoCueMode, nil)
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, routeContext))
		response := httptest.NewRecorder()
		a.getSetting(response, req)
		return response
	}
	if response := get(); response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"value":"fill-empty"`) {
		t.Fatalf("unset automatic cue setting = %d %s, want fill-empty", response.Code, response.Body.String())
	}
	for _, mode := range []string{"off", "suggest", "fill-empty", "replace-generated"} {
		response := setSettingForTest(a, SettingAutoCueMode, mode)
		if response.Code != http.StatusOK {
			t.Fatalf("set mode %q = %d %s", mode, response.Code, response.Body.String())
		}
		stored, err := database.GetSetting(SettingAutoCueMode)
		if err != nil || stored != mode {
			t.Fatalf("stored mode = %q, %v; want %q", stored, err, mode)
		}
	}
	if response := setSettingForTest(a, SettingAutoCueMode, "replace-all"); response.Code != http.StatusBadRequest {
		t.Fatalf("invalid mode write = %d, want 400: %s", response.Code, response.Body.String())
	}
	if err := database.SetSetting(SettingAutoCueMode, "corrupt-legacy-value"); err != nil {
		t.Fatal(err)
	}
	if response := get(); response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"value":"fill-empty"`) {
		t.Fatalf("invalid stored value = %d %s, want fill-empty fallback", response.Code, response.Body.String())
	}
}

func setSettingForTest(a *API, key, value string) *httptest.ResponseRecorder {
	routeContext := chi.NewRouteContext()
	routeContext.URLParams.Add("key", key)
	req := httptest.NewRequest(http.MethodPost, "/settings/"+key, strings.NewReader(`{"value":"`+value+`"}`))
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, routeContext))
	response := httptest.NewRecorder()
	a.setSetting(response, req)
	return response
}
