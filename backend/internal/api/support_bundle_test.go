package api

import (
	"archive/zip"
	"bytes"
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
	logText := `request failed api_key=secret-value path=/Users/alice/Music/private/song.mp3?access_token=url-secret` + "\n"
	if err := os.WriteFile(filepath.Join(dataDir, "viib.log"), []byte(logText), 0600); err != nil {
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

	for _, name := range []string{"manifest.json", "diagnostics.json", "logs/viib.log", "README.txt"} {
		if _, ok := entries[name]; !ok {
			t.Errorf("missing ZIP entry %q", name)
		}
	}
	for _, forbidden := range []string{"stored-secret", "secret-value", "url-secret", "/Users/alice", "library.db"} {
		for name, content := range entries {
			if strings.Contains(content, forbidden) {
				t.Errorf("entry %s contains forbidden value %q", name, forbidden)
			}
		}
	}
	if !strings.Contains(entries["logs/viib.log"], "<redacted>") || !strings.Contains(entries["logs/viib.log"], "<local-path>") {
		t.Fatalf("sanitized log did not contain redaction markers: %q", entries["logs/viib.log"])
	}
	if !strings.Contains(entries["diagnostics.json"], `"spotify": true`) {
		t.Fatalf("diagnostics did not retain configured status: %s", entries["diagnostics.json"])
	}
}
