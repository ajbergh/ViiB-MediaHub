// Tests mapping Spotify download failures to sanitized HTTP responses for authentication, availability, and cooldown.
package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	spotifyauth "github.com/ajbergh/viib-mediahub/internal/spotify/auth"
)

func TestCookieGroupedDownloadSessionFailures(t *testing.T) {
	for _, kind := range []string{"album", "playlist"} {
		for _, viaURL := range []bool{false, true} {
			for _, status := range []int{401, 429} {
				name := kind + "/" + http.StatusText(status)
				if viaURL {
					name += "/url"
				}
				t.Run(name, func(t *testing.T) {
					a, _, _ := fixtureCookieRuntime(t)
					if err := a.spotifyAuth.connect(context.Background(), "fixture-cookie"); err != nil {
						t.Fatal(err)
					}
					dm := NewDownloadManager(a.db, t.TempDir(), a.spotifyAuth)
					defer dm.cancel()
					a.downloadManager = dm
					var reads atomic.Int32
					a.spotifyHTTPClient = &http.Client{Transport: sessionTransport(func(r *http.Request) (*http.Response, error) {
						code, body := status, "fixture-private-upstream-body"
						if r.URL.Host == "clienttoken.spotify.com" {
							code, body = 200, `{"granted_token":{"token":"client","expires_after_seconds":600}}`
						} else {
							reads.Add(1)
						}
						return &http.Response{StatusCode: code, Header: http.Header{"Retry-After": []string{"90"}}, Body: io.NopCloser(strings.NewReader(body))}, nil
					})}
					id := "4aawyAB9vmqN3uQ7FjRGTy"
					body := `{"spotifyId":"` + id + `"}`
					if viaURL {
						body = `{"url":"spotify:` + kind + ":" + id + `"}`
					}
					request := httptest.NewRequest("POST", "/spotify/download/"+kind, strings.NewReader(body))
					w := httptest.NewRecorder()
					if viaURL {
						a.downloadFromURL(w, request)
					} else if kind == "album" {
						a.downloadAlbum(w, request)
					} else {
						a.downloadPlaylist(w, request)
					}
					if strings.Contains(w.Body.String(), "fixture-private-upstream-body") {
						t.Fatal("upstream body leaked")
					}
					if w.Code != status {
						t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
					}
					if status == 429 && w.Header().Get("Retry-After") != "90" {
						t.Fatal("cooldown lost")
					}
					expectedReads := int32(1)
					if status == 401 {
						expectedReads = 2
					}
					if reads.Load() != expectedReads {
						t.Fatal("unexpected retries", reads.Load())
					}
					if a.spotifyAuth.status().Connected != (status != 401) {
						t.Fatal("wrong connection state")
					}
					queued, err := dm.GetAllDownloads(10, 0)
					if err != nil || len(queued) != 0 {
						t.Fatal("failed group queued rows")
					}
					// A later group shares the same auth rejection or cooldown without another read.
					next := httptest.NewRecorder()
					a.downloadAlbumByIDPaginated(next, context.Background(), id)
					if next.Code != status || reads.Load() != expectedReads {
						t.Fatal("failure not shared")
					}
				})
			}
		}
	}
}

func TestSpotifyDownloadResponseStatusesAndRedaction(t *testing.T) {
	for _, status := range []int{401, 403, 404, 429, 500} {
		response := &http.Response{StatusCode: status, Header: http.Header{"Retry-After": []string{"90"}}}
		w := httptest.NewRecorder()
		respondSpotifyDownloadError(w, spotifyDownloadResponseError(response), 502, "Cannot fetch metadata")
		expected := status
		if status == 500 {
			expected = 502
		}
		if w.Code != expected {
			t.Fatal("upstream status lost", w.Code)
		}
		if status == 429 && w.Header().Get("Retry-After") != "90" {
			t.Fatal("upstream retry delay lost")
		}
	}
	w := httptest.NewRecorder()
	respondSpotifyDownloadError(w, spotifyauth.ErrAuthenticationRequired, 500, "Cannot queue")
	if w.Code != 401 {
		t.Fatal("account retirement lost")
	}
}

func TestCookieGroupedDownloadsWithNoAvailableTracks(t *testing.T) {
	for _, kind := range []string{"album", "playlist"} {
		for _, viaURL := range []bool{false, true} {
			name := kind
			if viaURL {
				name += "/url"
			}
			t.Run(name, func(t *testing.T) {
				a, _, _ := fixtureCookieRuntime(t)
				if err := a.spotifyAuth.connect(context.Background(), "fixture-cookie"); err != nil {
					t.Fatal(err)
				}
				dm := NewDownloadManager(a.db, t.TempDir(), a.spotifyAuth)
				defer dm.cancel()
				a.downloadManager = dm
				id := strings.Repeat("A", 22)
				a.spotifyHTTPClient = &http.Client{Transport: sessionTransport(func(r *http.Request) (*http.Response, error) {
					body := `{"granted_token":{"token":"client","expires_after_seconds":600}}`
					if r.URL.Host != "clienttoken.spotify.com" {
						fixture := libraryAlbumFixture(id)
						if kind == "playlist" {
							fixture = playlistCatalogFixture(id, 0, 1, 1)
						}
						var root map[string]any
						if err := json.Unmarshal([]byte(fixture), &root); err != nil {
							t.Fatal(err)
						}
						data := root["data"].(map[string]any)
						if kind == "album" {
							tracks := data["albumUnion"].(map[string]any)["tracksV2"].(map[string]any)
							tracks["totalCount"] = 1
							tracks["items"] = []any{map[string]any{"track": map[string]any{"__typename": "NotFound"}}}
						} else {
							content := data["playlistV2"].(map[string]any)["content"].(map[string]any)
							content["items"] = []any{map[string]any{"itemV2": map[string]any{"data": map[string]any{"__typename": "NotFound"}}}}
						}
						encoded, _ := json.Marshal(root)
						body = string(encoded)
					}
					return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
				})}
				body := `{"spotifyId":"` + id + `"}`
				if viaURL {
					body = `{"url":"spotify:` + kind + ":" + id + `"}`
				}
				r := httptest.NewRequest("POST", "/spotify/download/"+kind, strings.NewReader(body))
				w := httptest.NewRecorder()
				if viaURL {
					a.downloadFromURL(w, r)
				} else if kind == "album" {
					a.downloadAlbum(w, r)
				} else {
					a.downloadPlaylist(w, r)
				}
				if w.Code != 422 || !strings.Contains(w.Body.String(), "No downloadable Spotify tracks") {
					t.Fatal("empty group became internal error", w.Code, w.Body.String())
				}
				rows, err := dm.GetAllDownloads(10, 0)
				if err != nil || len(rows) != 0 {
					t.Fatal("empty group inserted rows")
				}
				if !a.spotifyAuth.status().Connected {
					t.Fatal("empty group disconnected account")
				}
			})
		}
	}
}

func TestPlaybackRecoveryDoesNotRequireLogin(t *testing.T) {
	for _, tc := range []struct {
		err    error
		status int
	}{
		{fmt.Errorf("failed to get session: session not initialized"), http.StatusServiceUnavailable},
		{fmt.Errorf("session reset while streaming"), http.StatusServiceUnavailable},
		{fmt.Errorf("auth: %w", spotifyauth.ErrAuthenticationRequired), http.StatusUnauthorized},
	} {
		response := httptest.NewRecorder()
		respondSpotifyDownloadError(response, tc.err, http.StatusServiceUnavailable, "Spotify playback is temporarily unavailable. Please retry.")
		if response.Code != tc.status {
			t.Fatalf("status=%d want=%d", response.Code, tc.status)
		}
	}
}
