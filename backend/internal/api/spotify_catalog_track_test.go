// Tests individual and batched track catalog response contracts.
package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/spotify/catalog"
)

func trackCatalogFixture(id, albumID string) string {
	raw, _ := json.Marshal(map[string]any{"data": map[string]any{"trackUnion": map[string]any{"__typename": "Track", "id": id, "name": "Track", "duration": map[string]any{"totalMilliseconds": 123456}, "firstArtist": map[string]any{"items": []any{map[string]any{"uri": "spotify:artist:" + strings.Repeat("A", 22), "profile": map[string]any{"name": "Artist"}}}}, "otherArtists": map[string]any{"items": []any{}}, "albumOfTrack": map[string]any{"id": albumID}, "contentRating": map[string]any{"label": "EXPLICIT"}}}})
	return string(raw)
}
func TestCookieCatalogTrackSingleBatchAndDownloadURL(t *testing.T) {
	a, _, _ := fixtureCookieRuntime(t)
	if err := a.spotifyAuth.connect(context.Background(), "fixture-cookie"); err != nil {
		t.Fatal(err)
	}
	const id = "5r9W9MJLvHk83fcZSPQ8SE"
	second, missing := strings.Repeat("S", 22), strings.Repeat("M", 22)
	mints, tracks, albums := 0, 0, 0
	a.spotifyHTTPClient = &http.Client{Transport: sessionTransport(func(r *http.Request) (*http.Response, error) {
		if r.Method != "POST" || r.Header.Get("Cookie") != "" {
			t.Fatal("unsafe track request")
		}
		status := 200
		body := ""
		if r.URL.Host == "clienttoken.spotify.com" {
			if r.Header.Get("Authorization") != "" || r.Header.Get("Client-Token") != "" {
				t.Fatal("account token escaped mint")
			}
			mints++
			body = "{\"granted_token\":{\"token\":\"client\",\"expires_after_seconds\":600}}"
		} else {
			if r.URL.Host != "api-partner.spotify.com" || r.Header.Get("Authorization") == "" || r.Header.Get("Client-Token") != "client" {
				t.Fatal("public API or unsafe track route")
			}
			var request struct {
				Operation  string         `json:"operationName"`
				Variables  map[string]any `json:"variables"`
				Extensions map[string]any `json:"extensions"`
			}
			if json.NewDecoder(r.Body).Decode(&request) != nil {
				t.Fatal("bad request")
			}
			switch request.Operation {
			case "getTrack":
				tracks++
				if request.Extensions["persistedQuery"].(map[string]any)["sha256Hash"] != "612585ae06ba435ad26369870deaae23b5c8800a256cd8a57e08eddc25a37294" {
					t.Fatal("wrong fixed hash")
				}
				trackID := strings.TrimPrefix(request.Variables["uri"].(string), "spotify:track:")
				if trackID == missing {
					status = 404
				} else {
					body = trackCatalogFixture(trackID, id)
				}
			case "getAlbum":
				albums++
				body = libraryAlbumFixture(id)
			default:
				t.Fatal("unknown operation")
			}
		}
		return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
	})}
	w := httptest.NewRecorder()
	a.spotifyProxy(w, httptest.NewRequest("GET", "/spotify/proxy?path=tracks/"+id, nil))
	var track catalog.Track
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &track) != nil || track.ID != id || track.URI != "spotify:track:"+id || track.DurationMS != 123456 || len(track.Artists) != 1 || track.Album.ReleaseDate == nil || *track.Album.ReleaseDate != "2020-03-04" || len(track.Album.Images) != 1 || track.Explicit == nil || !*track.Explicit {
		t.Fatal("track metadata mismatch", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	a.spotifyProxy(w, httptest.NewRequest("GET", "/spotify/proxy?path=tracks&ids="+id+","+missing+","+second+","+id, nil))
	var batch catalog.TracksResult
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &batch) != nil || len(batch.Tracks) != 4 || batch.Tracks[0].ID != id || batch.Tracks[1] != nil || batch.Tracks[2].ID != second || batch.Tracks[3].ID != id || tracks != 4 || albums != 2 || mints != 1 {
		t.Fatal("batch order, nulls or metadata reuse failed", w.Code, tracks, albums, mints)
	}
	for _, query := range []string{"tracks/bad", "tracks&ids=", "tracks&ids=" + id + ",", "tracks&ids=" + strings.Repeat(id+",", 50) + id, "tracks/" + id + "&market=bad"} {
		invalid := httptest.NewRecorder()
		a.spotifyProxy(invalid, httptest.NewRequest("GET", "/spotify/proxy?path="+query, nil))
		if invalid.Code != 400 {
			t.Fatal("bad track query accepted", query, invalid.Code)
		}
	}
	if tracks != 4 {
		t.Fatal("invalid query reached upstream")
	}
	dm := NewDownloadManager(a.db, t.TempDir(), a.spotifyAuth)
	defer dm.cancel()
	a.downloadManager = dm
	w = httptest.NewRecorder()
	a.downloadFromURL(w, httptest.NewRequest("POST", "/spotify/download/url", strings.NewReader(fmt.Sprintf("{\"url\":\"https://open.spotify.com/track/%s\"}", id))))
	queued, err := dm.GetAllDownloads(10, 0)
	if w.Code != 200 || err != nil || len(queued) != 1 || queued[0].SpotifyID != id || queued[0].Title != "Track" || queued[0].Artist != "Artist" || queued[0].Album != "Saved Album" || !strings.Contains(queued[0].Metadata, "https://images.example/album") {
		t.Fatal("track URL queue metadata failed", w.Code, err, queued)
	}
}
func TestCookieCatalogTrackErrorsAndCooldown(t *testing.T) {
	for _, status := range []int{401, 403, 429, 200} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			a, _, _ := fixtureCookieRuntime(t)
			if err := a.spotifyAuth.connect(context.Background(), "fixture-cookie"); err != nil {
				t.Fatal(err)
			}
			calls := 0
			a.spotifyHTTPClient = &http.Client{Transport: sessionTransport(func(r *http.Request) (*http.Response, error) {
				code := status
				body := "{\"data\":{\"trackUnion\":{\"__typename\":\"UnknownTrack\"}}}"
				if r.URL.Host == "clienttoken.spotify.com" {
					code = 200
					body = "{\"granted_token\":{\"token\":\"client\",\"expires_after_seconds\":600}}"
				} else {
					calls++
				}
				return &http.Response{StatusCode: code, Header: http.Header{"Retry-After": []string{"90"}}, Body: io.NopCloser(strings.NewReader(body))}, nil
			})}
			w := httptest.NewRecorder()
			a.spotifyProxy(w, httptest.NewRequest("GET", "/spotify/proxy?path=tracks/5r9W9MJLvHk83fcZSPQ8SE", nil))
			expected := status
			if status == 200 {
				expected = 503
			}
			if w.Code != expected {
				t.Fatal("track status mismatch", w.Code, w.Body.String())
			}
			if status == 401 {
				if calls != 2 || a.spotifyAuth.status().Connected {
					t.Fatal("repeated track rejection failed")
				}
			} else if !a.spotifyAuth.status().Connected {
				t.Fatal("non-auth error disconnected")
			}
			if status == 401 || status == 429 {
				urlResult := httptest.NewRecorder()
				a.downloadTrackByID(urlResult, context.Background(), "5r9W9MJLvHk83fcZSPQ8SE")
				if urlResult.Code != status {
					t.Fatal("URL download hid session status", urlResult.Code)
				}
				if status == 429 && urlResult.Header().Get("Retry-After") != "90" {
					t.Fatal("URL download lost cooldown")
				}
			}
			if status == 429 {
				next := httptest.NewRecorder()
				a.spotifyGetUserProfile(next, httptest.NewRequest("GET", "/spotify/me", nil))
				if next.Code != 429 || calls != 1 || next.Header().Get("Retry-After") != "90" {
					t.Fatal("track cooldown not shared")
				}
			}
		})
	}
}
func TestCookieCatalogTrackDownloadCancelledOnAccountChange(t *testing.T) {
	a, _, _ := fixtureCookieRuntime(t)
	if err := a.spotifyAuth.connect(context.Background(), "fixture-cookie"); err != nil {
		t.Fatal(err)
	}
	dm := NewDownloadManager(a.db, t.TempDir(), a.spotifyAuth)
	defer dm.cancel()
	a.downloadManager = dm
	entered := make(chan struct{})
	a.spotifyHTTPClient = &http.Client{Transport: sessionTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host == "clienttoken.spotify.com" {
			return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("{\"granted_token\":{\"token\":\"client\",\"expires_after_seconds\":600}}"))}, nil
		}
		close(entered)
		<-r.Context().Done()
		return nil, r.Context().Err()
	})}
	done := make(chan int, 1)
	go func() {
		w := httptest.NewRecorder()
		a.downloadTrackByID(w, context.Background(), "5r9W9MJLvHk83fcZSPQ8SE")
		done <- w.Code
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("track request did not enter")
	}
	if err := a.spotifyAuth.disconnect(); err != nil {
		t.Fatal(err)
	}
	select {
	case code := <-done:
		if code == 200 {
			t.Fatal("canceled download succeeded")
		}
	case <-time.After(time.Second):
		t.Fatal("canceled download survived")
	}
	queued, err := dm.GetAllDownloads(10, 0)
	if err != nil || len(queued) != 0 {
		t.Fatal("old account queued a track")
	}
}
