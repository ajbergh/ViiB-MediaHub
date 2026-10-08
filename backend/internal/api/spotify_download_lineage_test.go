package api

import (
	"context"
	"encoding/json"
	"github.com/ajbergh/viib-mediahub/internal/db"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPlaylistDownloadRetainsExplicitOriginAndOriginalPositions(t *testing.T) {
	a, _, _ := fixtureCookieRuntime(t)
	if err := a.spotifyAuth.connect(context.Background(), "fixture-cookie"); err != nil {
		t.Fatal(err)
	}
	dm := NewDownloadManager(a.db, t.TempDir(), a.spotifyAuth)
	defer dm.cancel()
	a.downloadManager = dm
	playlistID := strings.Repeat("P", 22)
	a.spotifyHTTPClient = &http.Client{Transport: sessionTransport(func(r *http.Request) (*http.Response, error) {
		body := `{"granted_token":{"token":"client","expires_after_seconds":600}}`
		if r.URL.Host != "clienttoken.spotify.com" {
			var root map[string]any
			if err := json.Unmarshal([]byte(playlistCatalogFixture(playlistID, 0, 3, 3)), &root); err != nil {
				t.Fatal(err)
			}
			content := root["data"].(map[string]any)["playlistV2"].(map[string]any)["content"].(map[string]any)
			content["items"].([]any)[0] = map[string]any{"itemV2": map[string]any{"data": map[string]any{"__typename": "NotFound"}}}
			raw, _ := json.Marshal(root)
			body = string(raw)
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
	})}
	w := httptest.NewRecorder()
	a.downloadPlaylist(w, httptest.NewRequest("POST", "/spotify/download/playlist", strings.NewReader(`{"spotifyId":"`+playlistID+`","origins":[{"kind":"library","id":"saved_playlists","entityId":"`+playlistID+`","position":-1}]}`)))
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	captured, err := a.db.GetSpotifyEntitySnapshot(db.SpotifySnapshotKey{EntityType: "playlist", SpotifyID: playlistID, Resource: "fetchPlaylist:page:0:100", ContextKey: a.spotifyTokens().metadataContext})
	if err != nil || captured == nil || captured.CaptureRevision != "version1" {
		t.Fatal("Web Player page binding missing", captured, err)
	}
	queued, err := dm.GetAllDownloads(10, 0)
	if err != nil || len(queued) != 2 {
		t.Fatal(len(queued), err)
	}
	path := filepath.Join(t.TempDir(), "final.mp3")
	if err := os.WriteFile(path, []byte("final media"), 0600); err != nil {
		t.Fatal(err)
	}
	song := db.Song{ID: "song", FilePath: path, FileHash: "stable", AddedAt: 1}
	if _, err := a.db.SaveSongsWithResult([]db.Song{song}); err != nil {
		t.Fatal(err)
	}
	for _, row := range queued {
		if ok, err := a.db.MarkDownloadStarted(row.ID); err != nil || !ok {
			t.Fatal(ok, err)
		}
		if ok, err := a.db.MarkDownloadCompletedWithEvidence(t.Context(), row.ID, path); err != nil || !ok {
			t.Fatal(ok, err)
		}
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := a.db.GetDownloadedSpotifyCatalog(t.Context(), "song", db.LocalSourceFingerprint(song, info))
	if err != nil || catalog == nil || len(catalog.Origins) != 3 {
		t.Fatalf("catalog: %+v %v", catalog, err)
	}
	if catalog.CollectionStatus == nil || catalog.CollectionStatus.State != "incomplete" {
		t.Fatal("missing saved-library payload was not explicit", catalog.CollectionStatus)
	}
	retainedPlaylist := false
	for _, snapshot := range catalog.Snapshots {
		if snapshot.EntityType == "playlist" && snapshot.CapturedResource == "fetchPlaylist:page:0:100" && snapshot.CaptureRevision == "version1" {
			retainedPlaylist = true
		}
	}
	if !retainedPlaylist {
		t.Fatal("validated playlist payload not promoted")
	}
	positions := map[int]bool{}
	for _, origin := range catalog.Origins {
		if origin.Kind == "library" && origin.EntityID != playlistID {
			t.Fatal("collection identity lost", origin)
		}
		if origin.Kind == "playlist" {
			if origin.ID != playlistID || origin.Revision != "version1" {
				t.Fatal(origin)
			}
			positions[origin.Position] = true
		}
	}
	if !positions[1] || !positions[2] || positions[0] {
		t.Fatal("unavailable row compressed original positions", positions)
	}
}

func TestPlaylistOriginFallbackDoesNotInventPosition(t *testing.T) {
	origin := playlistDownloadOrigin(strings.Repeat("A", 22), PlaylistTrackInfo{})
	if origin.Position != -1 || origin.Revision != "" {
		t.Fatal(origin)
	}
}

func TestGroupedDownloadRejectsOriginOverflowBeforeProviderIO(t *testing.T) {
	a, _, _ := fixtureCookieRuntime(t)
	origins := make([]db.SpotifyDownloadOrigin, 32)
	for i := range origins {
		origins[i] = db.SpotifyDownloadOrigin{Kind: "playlist", ID: strings.Repeat("P", 22), Position: i}
	}
	body, _ := json.Marshal(map[string]any{"spotifyId": strings.Repeat("A", 22), "origins": origins})
	for _, kind := range []string{"album", "playlist"} {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/spotify/download/"+kind, strings.NewReader(string(body)))
		if kind == "album" {
			a.downloadAlbum(w, r)
		} else {
			a.downloadPlaylist(w, r)
		}
		if w.Code != 400 {
			t.Fatal(kind, w.Code, w.Body.String())
		}
	}
}
