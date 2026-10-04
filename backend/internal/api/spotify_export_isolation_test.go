package api

import (
	"archive/zip"
	"bytes"
	"database/sql"
	"encoding/json"
	"github.com/go-chi/chi/v5"
	"io"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSpotifySessionExcludedFromGenericSettings(t *testing.T) {
	a, _, _ := fixtureCookieRuntime(t)
	original := `{"provider":"webplayer","spDC":"fixture-cookie"}`
	if err := a.db.SetSetting(spotifyCookieSetting, original); err != nil {
		t.Fatal(err)
	}
	router := chi.NewRouter()
	router.Get("/settings/{key}", a.getSetting)
	router.Post("/settings/{key}", a.setSetting)
	for _, method := range []string{"GET", "POST"} {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(method, "/settings/"+spotifyCookieSetting, strings.NewReader(`{"value":"replacement"}`)))
		if w.Code != 400 || strings.Contains(w.Body.String(), "fixture-cookie") {
			t.Fatal("generic session access allowed")
		}
	}
	stored, err := a.db.GetSetting(spotifyCookieSetting)
	if err != nil || stored != original {
		t.Fatal("session changed")
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("POST", "/settings/theme", strings.NewReader(`{"value":"dark"}`)))
	if w.Code != 200 {
		t.Fatal("normal settings write rejected")
	}
	w = httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("GET", "/settings/theme", nil))
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"value":"dark"`) {
		t.Fatal("normal settings read rejected")
	}
}
func TestSpotifyCredentialsExcludedFromBackupArchive(t *testing.T) {
	a, path, _ := fixtureCookieRuntime(t)
	a.dataDir = t.TempDir()
	for key, value := range map[string]string{spotifyCookieSetting: `{"provider":"webplayer","spDC":"archive-cookie"}`, "spotify_credentials": `{"accessToken":"archive-token"}`, "theme": "dark"} {
		if err := a.db.SetSetting(key, value); err != nil {
			t.Fatal(err)
		}
	}
	live, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer live.Close()
	var encryptedCookie string
	if err := live.QueryRow("SELECT value FROM settings WHERE key=?", spotifyCookieSetting).Scan(&encryptedCookie); err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	a.createBackupV2(w, httptest.NewRequest("POST", "/backup", strings.NewReader(`{"name":"isolation"}`)))
	if w.Code != 201 {
		t.Fatalf("backup failed %d %s", w.Code, w.Body.String())
	}
	var info backupInfo
	if err := json.Unmarshal(w.Body.Bytes(), &info); err != nil {
		t.Fatal(err)
	}
	archive, err := zip.OpenReader(filepath.Join(a.backupsDir(), info.Name))
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Close()
	var data []byte
	for _, entry := range archive.File {
		if entry.Name == "library.db" {
			r, err := entry.Open()
			if err != nil {
				t.Fatal(err)
			}
			data, err = io.ReadAll(r)
			r.Close()
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	if len(data) == 0 {
		t.Fatal("no database in backup")
	}
	for _, secret := range []string{encryptedCookie, "archive-cookie", "archive-token"} {
		if bytes.Contains(data, []byte(secret)) {
			t.Fatal("credential bytes remain in exported database")
		}
	}
	copyPath := filepath.Join(t.TempDir(), "backup.db")
	if err := os.WriteFile(copyPath, data, 0600); err != nil {
		t.Fatal(err)
	}
	copied, err := sql.Open("sqlite", copyPath)
	if err != nil {
		t.Fatal(err)
	}
	defer copied.Close()
	var count int
	if err := copied.QueryRow("SELECT count(*) FROM settings WHERE key IN ('spotify_webplayer_session','spotify_credentials')").Scan(&count); err != nil || count != 0 {
		t.Fatal("Spotify credential row in archive")
	}
	var theme string
	if err := copied.QueryRow("SELECT value FROM settings WHERE key='theme'").Scan(&theme); err != nil || theme != "dark" {
		t.Fatal("ordinary settings lost")
	}
	var liveCount int
	if err := live.QueryRow("SELECT count(*) FROM settings WHERE key IN ('spotify_webplayer_session','spotify_credentials')").Scan(&liveCount); err != nil || liveCount != 2 {
		t.Fatal("live credentials changed")
	}
}
