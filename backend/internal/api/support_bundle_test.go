// Tests building support archives with bounded, allowlisted diagnostic records.
package api

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ajbergh/viib-mediahub/internal/db"
)

func TestSupportBundleContainsSanitizedDiagnosticsOnly(t *testing.T) {
	dataDir := t.TempDir()
	database, err := db.New(filepath.Join(dataDir, "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err := database.SetSetting("spotify_credentials", `{"accessToken":"stored-secret"}`); err != nil {
		t.Fatal(err)
	}
	if err := database.SetSetting(spotifyCookieSetting, `{"provider":"webplayer","spDC":"stored-cookie"}`); err != nil {
		t.Fatal(err)
	}
	privatePayload := supportFixtureLine("FE ERROR", `[DJEngine] Private fixture title | Data: {"id":"private-resource-id","error":{"message":"private-error-text","stack":"/arbitrary/private/tree"},"requestId":"private-host-request"}`)
	logText := privatePayload + supportFixtureLine("FE WARN", `[AudioRead] Audio read failed | Data: {"operation":"local_bands","category":"http","status":503}`) + `request failed api_key=secret-value path=/Users/alice/Music/private/song.mp3?access_token=url-secret` + "\n" + `spDC=log-cookie sp_dc=snake-cookie {"spDC":"json-cookie"} Cookie: sp_dc=header-cookie; sp_key=key-cookie ?sp_dc=query-cookie&ok=1` + "\n"
	if err := os.WriteFile(filepath.Join(dataDir, "viib.log"), []byte(logText), 0600); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(dataDir, "scan.log"), []byte("private-scan-decision"), 0600); err != nil {
		t.Fatal(err)
	}
	api := &API{db: database, dataDir: dataDir}
	response := httptest.NewRecorder()
	api.downloadSupportBundle(response, httptest.NewRequest(http.MethodGet, "/api/support-bundle", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", response.Code, response.Body.String())
	}
	if got := response.Header().Get("Content-Type"); got != "application/zip" {
		t.Fatalf("Content-Type = %q", got)
	}
	if got := response.Header().Get("Content-Disposition"); !strings.Contains(got, "viib-support-") {
		t.Fatalf("Content-Disposition = %q", got)
	}

	reader, err := zip.NewReader(bytes.NewReader(response.Body.Bytes()), int64(response.Body.Len()))
	if err != nil {
		t.Fatal(err)
	}
	entries := make(map[string]string, len(reader.File))
	for _, file := range reader.File {
		opened, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		content, readErr := io.ReadAll(opened)
		_ = opened.Close()
		if readErr != nil {
			t.Fatal(readErr)
		}
		entries[file.Name] = string(content)
	}

	for _, name := range []string{"manifest.json", "diagnostics.json", "logs/diagnostics.jsonl", "README.txt"} {
		if _, ok := entries[name]; !ok {
			t.Errorf("missing ZIP entry %q", name)
		}
	}
	for _, forbidden := range []string{"Private fixture title", "private-resource-id", "private-error-text", "/arbitrary/private/tree", "private-host-request", "private-scan-decision", "stored-cookie", "log-cookie", "snake-cookie", "json-cookie", "header-cookie", "key-cookie", "query-cookie", "stored-secret", "secret-value", "url-secret", "/Users/alice", "library.db"} {
		for name, content := range entries {
			if strings.Contains(content, forbidden) {
				t.Errorf("entry %s contains forbidden value %q", name, forbidden)
			}
		}
	}
	if len(entries) != 4 {
		t.Fatalf("unexpected archive entries: %v", entries)
	}
	if !strings.Contains(entries["logs/diagnostics.jsonl"], `"event":"audio_read_failed"`) {
		t.Fatal("safe diagnostic missing")
	}
	var manifest supportBundleManifest
	if err := json.Unmarshal([]byte(entries["manifest.json"]), &manifest); err != nil || manifest.SchemaVersion != 2 {
		t.Fatal("wrong bundle schema", err)
	}
	var diagnostics supportBundleDiagnostics
	if err := json.Unmarshal([]byte(entries["diagnostics.json"]), &diagnostics); err != nil || diagnostics.Version != 2 || diagnostics.Included != 1 || diagnostics.Dropped != 3 {
		t.Fatalf("projection metadata: %+v %v", diagnostics, err)
	}
	retained, err := os.ReadFile(filepath.Join(dataDir, "viib.log"))
	if err != nil || string(retained) != logText {
		t.Fatal("support export changed local logging", err)
	}
	if !strings.Contains(entries["diagnostics.json"], `"spotify": true`) {
		t.Fatalf("diagnostics did not retain configured status: %s", entries["diagnostics.json"])
	}
}
