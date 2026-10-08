package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Rendered saved-library actions use real local download routes and temporary
// SQLite. Provider browsing/auth is synthetic; no downloader worker is started.
func TestSpotifySavedLibraryBrowserDownloadOrigins(t *testing.T) {
	preview := os.Getenv("VIIB_BROWSER_PREVIEW_URL")
	if preview == "" {
		t.Skip("set VIIB_BROWSER_PREVIEW_URL to run saved-library browser audit")
	}
	a, path, _ := fixtureCookieRuntime(t)
	if err := a.spotifyAuth.connect(context.Background(), "fixture-cookie"); err != nil {
		t.Fatal(err)
	}
	dm := NewDownloadManager(a.db, t.TempDir(), a.spotifyAuth)
	defer dm.cancel()
	a.downloadManager = dm
	albumID := strings.Repeat("B", 22)
	playlistID := strings.Repeat("P", 22)
	a.spotifyHTTPClient = &http.Client{Transport: sessionTransport(func(r *http.Request) (*http.Response, error) {
		body := `{"granted_token":{"token":"client","expires_after_seconds":600}}`
		if r.URL.Host != "clienttoken.spotify.com" {
			var query struct {
				OperationName string `json:"operationName"`
			}
			if err := json.NewDecoder(r.Body).Decode(&query); err != nil {
				return nil, err
			}
			if query.OperationName == "getAlbum" {
				track := map[string]any{"uri": "spotify:track:" + strings.Repeat("U", 22), "name": "Album Track", "trackNumber": 2, "discNumber": 1, "artists": map[string]any{"items": []any{map[string]any{"uri": "spotify:artist:" + strings.Repeat("A", 22), "profile": map[string]any{"name": "Artist"}}}}, "duration": map[string]any{"totalMilliseconds": 123456}}
				album := map[string]any{"__typename": "Album", "uri": "spotify:album:" + albumID, "name": "Saved Album", "artists": map[string]any{"items": []any{map[string]any{"uri": "spotify:artist:" + strings.Repeat("A", 22), "profile": map[string]any{"name": "Artist"}}}}, "tracksV2": map[string]any{"totalCount": 1, "items": []any{map[string]any{"track": track}}}}
				raw, _ := json.Marshal(map[string]any{"data": map[string]any{"albumUnion": album}})
				body = string(raw)
			} else if query.OperationName == "fetchPlaylist" {
				var root map[string]any
				if err := json.Unmarshal([]byte(playlistCatalogFixture(playlistID, 0, 3, 3)), &root); err != nil {
					return nil, err
				}
				content := root["data"].(map[string]any)["playlistV2"].(map[string]any)["content"].(map[string]any)
				content["items"].([]any)[0] = map[string]any{"itemV2": map[string]any{"data": map[string]any{"__typename": "NotFound"}}}
				raw, _ := json.Marshal(root)
				body = string(raw)
			} else {
				return nil, fmt.Errorf("unexpected fixture operation %q", query.OperationName)
			}
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
	})}
	server := httptest.NewServer(a.Routes())
	defer server.Close()
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "node", filepath.Join(root, "scripts", "spotify-library-download-audit.mjs"))
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "SPOTIFY_LIBRARY_AUDIT_URL="+preview, "SPOTIFY_LIBRARY_AUDIT_BACKEND="+server.URL)
	output, err := cmd.CombinedOutput()
	t.Log(string(output))
	if err != nil {
		t.Fatal(err)
	}
	inspect, err := sql.Open("viib_sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer inspect.Close()
	for _, want := range []struct {
		kind, id, entity string
		position         int
	}{{"library", "saved_albums", albumID, -1}, {"album", albumID, "", 1}, {"library", "saved_playlists", playlistID, -1}, {"playlist", playlistID, "", 1}, {"playlist", playlistID, "", 2}} {
		var n int
		revision := ""
		if want.kind == "playlist" {
			revision = "version1"
		}
		if err := inspect.QueryRow(`SELECT COUNT(*) FROM spotify_download_lineage_staging s JOIN spotify_downloads d ON d.id=s.download_id WHERE d.status='queued' AND s.origin_kind=? AND s.origin_id=? AND s.entity_id=? AND s.position=? AND s.context_key=? AND s.origin_revision=?`, want.kind, want.id, want.entity, want.position, a.spotifyTokens().metadataContext, revision).Scan(&n); err != nil || n < 1 {
			t.Fatal("persisted origin missing", want, n, err)
		}
	}
	var invalid int
	if err := inspect.QueryRow(`SELECT COUNT(*) FROM spotify_download_lineage_staging WHERE origin_kind='playlist' AND position=0`).Scan(&invalid); err != nil || invalid != 0 {
		t.Fatal("unavailable row position invented", invalid, err)
	}
}
