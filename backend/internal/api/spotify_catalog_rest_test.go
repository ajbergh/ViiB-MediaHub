package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/ajbergh/viib-mediahub/internal/spotify/metadata"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/db"
)

func TestOAuthCatalogResponseCapturedAndReplayed(t *testing.T) {
	database, err := db.New(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	credentials, _ := json.Marshal(SpotifyCredentials{ClientId: "client", AccessToken: "fixture", RefreshToken: "refresh", Expiry: time.Now().Add(time.Hour).UnixMilli()})
	if err := database.SetSetting("spotify_credentials", string(credentials)); err != nil {
		t.Fatal(err)
	}
	id := strings.Repeat("T", 22)
	albumID := strings.Repeat("A", 22)
	artistID := strings.Repeat("B", 22)
	album := map[string]any{"id": albumID, "type": "album", "uri": "spotify:album:" + albumID, "name": "Album"}
	artist := map[string]any{"id": artistID, "type": "artist", "uri": "spotify:artist:" + artistID, "name": "Artist"}
	rawBytes, _ := json.Marshal(map[string]any{"id": id, "type": "track", "uri": "spotify:track:" + id, "name": "Song", "duration_ms": 180000, "future_domain": 0, "token": "secret", "album": album, "artists": []any{artist}})
	raw := string(rawBytes)
	a := &API{db: database, spotifyHTTPClient: &http.Client{Transport: sessionTransport(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(raw))}, nil
	})}}
	response, err := a.doSpotifyRequest(context.Background(), "GET", "https://api.spotify.com/v1/tracks/"+id, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil || string(body) != raw {
		t.Fatal("consumer response changed", err)
	}
	runtime := a.spotifyTokens()
	snapshot, err := database.GetSpotifyEntitySnapshot(db.SpotifySnapshotKey{EntityType: "track", SpotifyID: id, Resource: "rest:/v1/tracks/" + id + ":page::", ContextKey: runtime.metadataContext})
	if err != nil || snapshot == nil || !strings.Contains(string(snapshot.Payload), "future_domain") || strings.Contains(string(snapshot.Payload), "secret") {
		t.Fatalf("snapshot: %+v %v", snapshot, err)
	}
	foundAlbumEdge := false
	for _, relation := range snapshot.Relations {
		if relation.Kind == "album" && relation.ChildType == "album" && relation.ChildID == albumID && !relation.Unavailable {
			foundAlbumEdge = true
		}
	}
	if !foundAlbumEdge {
		t.Fatalf("OAuth REST track snapshot lost its album edge: %+v", snapshot.Relations)
	}
	albumSnapshot, err := database.GetSpotifyEntitySnapshot(db.SpotifySnapshotKey{EntityType: "album", SpotifyID: albumID, Resource: "rest:/v1/tracks/" + id + ":page:::related", ContextKey: runtime.metadataContext})
	if err != nil || albumSnapshot == nil {
		t.Fatalf("OAuth REST response did not persist its nested album: %+v %v", albumSnapshot, err)
	}
}

type captureTestBody struct {
	io.Reader
	closed bool
}

func (r *captureTestBody) Close() error { r.closed = true; return nil }

type captureFailureReader struct {
	sent bool
	err  error
}

func (r *captureFailureReader) Read(p []byte) (int, error) {
	if r.sent {
		return 0, io.EOF
	}
	r.sent = true
	return copy(p, "partial"), r.err
}

func TestOAuthCatalogCaptureBoundsRetirementAndBodyOwnership(t *testing.T) {
	for _, kind := range []string{"oversized", "retired", "profile", "read_error"} {
		t.Run(kind, func(t *testing.T) {
			a, _ := newBPMRouteTestAPI(t, false)
			runtime := a.spotifyTokens()
			ctx, cancel := runtime.requestContext(t.Context())
			defer cancel()
			id := strings.Repeat("T", 22)
			target, _ := url.Parse("https://api.spotify.com/v1/tracks/" + id)
			raw := `{"type":"track","id":"` + id + `","name":"Song"}`
			if kind == "oversized" {
				raw += strings.Repeat(" ", metadata.CatalogLimit)
			}
			if kind == "retired" {
				runtime.beginRetirement()
			}
			if kind == "profile" {
				target.Path = "/v1/me"
			}
			expectedErr := errors.New("source read failed")
			original := &captureTestBody{Reader: strings.NewReader(raw)}
			if kind == "read_error" {
				original.Reader = &captureFailureReader{err: expectedErr}
				raw = "partial"
			}
			response := &http.Response{Body: original}
			a.captureOAuthCatalogResponse(ctx, runtime, target, response)
			if original.closed {
				t.Fatal("capture closed consumer-owned body")
			}
			got, err := io.ReadAll(response.Body)
			if string(got) != raw {
				t.Fatal("response replay changed")
			}
			if kind == "read_error" && !errors.Is(err, expectedErr) {
				t.Fatalf("read error lost: %v", err)
			}
			if kind != "read_error" && err != nil {
				t.Fatal(err)
			}
			response.Body.Close()
			if !original.closed {
				t.Fatal("original body was not closed")
			}
			snapshot, err := a.db.GetSpotifyEntitySnapshot(db.SpotifySnapshotKey{EntityType: "track", SpotifyID: id, Resource: "rest:/v1/tracks/" + id + ":page::", ContextKey: runtime.metadataContext})
			if err != nil || snapshot != nil {
				t.Fatalf("ineligible snapshot persisted: %+v %v", snapshot, err)
			}
		})
	}
}

func TestOAuthSavedLibraryPageCapturedAndReplayed(t *testing.T) {
	database, err := db.New(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	credentials, _ := json.Marshal(SpotifyCredentials{ClientId: "client", AccessToken: "fixture", RefreshToken: "refresh", Expiry: time.Now().Add(time.Hour).UnixMilli()})
	if err := database.SetSetting("spotify_credentials", string(credentials)); err != nil {
		t.Fatal(err)
	}
	id := strings.Repeat("T", 22)
	raw := `{"offset":4,"total":5,"items":[{"track":{"type":"track","id":"` + id + `","future_domain":0,"token":"secret"}}]}`
	a := &API{db: database, spotifyHTTPClient: &http.Client{Transport: sessionTransport(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(raw))}, nil
	})}}
	response, err := a.doSpotifyRequest(context.Background(), "GET", "https://api.spotify.com/v1/me/tracks?offset=4&limit=1", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil || string(body) != raw {
		t.Fatal("consumer response changed", err)
	}
	runtime := a.spotifyTokens()
	snapshot, err := database.GetSpotifyEntitySnapshot(db.SpotifySnapshotKey{EntityType: "library", SpotifyID: "rest_saved_tracks", Resource: "rest:/v1/me/tracks:page:4:1", ContextKey: runtime.metadataContext})
	if err != nil || snapshot == nil || !strings.Contains(string(snapshot.Payload), "future_domain") || strings.Contains(string(snapshot.Payload), "secret") {
		t.Fatalf("snapshot: %+v %v", snapshot, err)
	}

}

func TestOAuthPlaylistFinalRevisionFence(t *testing.T) {
	for _, revision := range []string{"version1", "version2", ""} {
		t.Run("final_"+revision, func(t *testing.T) {
			a, _ := newBPMRouteTestAPI(t, false)
			credentials, _ := json.Marshal(SpotifyCredentials{ClientId: "client", AccessToken: "fixture", Expiry: time.Now().Add(time.Hour).UnixMilli()})
			if err := a.db.SetSetting("spotify_credentials", string(credentials)); err != nil {
				t.Fatal(err)
			}
			id := strings.Repeat("P", 22)
			rootCalls := 0
			a.spotifyHTTPClient = &http.Client{Transport: sessionTransport(func(r *http.Request) (*http.Response, error) {
				raw := `{"offset":1,"limit":1,"items":[null],"next":null}`
				if !strings.HasSuffix(r.URL.Path, "/tracks") {
					rootCalls++
					if rootCalls == 1 {
						raw = `{"type":"playlist","id":"` + id + `","name":"Playlist","snapshot_id":"version1","tracks":{"offset":0,"items":[null],"next":"https://api.spotify.com/v1/playlists/` + id + `/tracks?offset=1&limit=1"}}`
					} else {
						raw = `{"snapshot_id":"` + revision + `"}`
					}
				}
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(raw))}, nil
			})}
			_, _, _, err := a.fetchPlaylistTracks(t.Context(), id, nil)
			if rootCalls != 2 {
				t.Fatal("root was not revalidated")
			}
			if (err == nil) != (revision == "version1") {
				t.Fatalf("revision fence: %v", err)
			}
			runtime := a.spotifyTokens()
			snapshot, readErr := a.db.GetSpotifyEntitySnapshot(db.SpotifySnapshotKey{EntityType: "playlist", SpotifyID: id, Resource: "rest:/v1/playlists/" + id + ":page::", ContextKey: runtime.metadataContext})
			if readErr != nil || (snapshot != nil) != (revision == "version1") {
				t.Fatalf("traversal publication: %+v %v", snapshot, readErr)
			}
			if snapshot != nil && snapshot.CaptureRevision != "version1" {
				t.Fatal("root revision not persisted", snapshot)
			}
		})
	}
}

func TestOAuthPlaylistRejectsOverlappingOrSkippedPage(t *testing.T) {
	for _, revision := range []string{"0", "2"} {
		t.Run("offset_"+revision, func(t *testing.T) {
			a, _ := newBPMRouteTestAPI(t, false)
			credentials, _ := json.Marshal(SpotifyCredentials{ClientId: "client", AccessToken: "fixture", Expiry: time.Now().Add(time.Hour).UnixMilli()})
			if err := a.db.SetSetting("spotify_credentials", string(credentials)); err != nil {
				t.Fatal(err)
			}
			id := strings.Repeat("P", 22)
			rootCalls := 0
			a.spotifyHTTPClient = &http.Client{Transport: sessionTransport(func(r *http.Request) (*http.Response, error) {
				raw := `{"offset":` + revision + `,"limit":1,"items":[null],"next":null}`
				if !strings.HasSuffix(r.URL.Path, "/tracks") {
					rootCalls++
					if rootCalls == 1 {
						raw = `{"type":"playlist","id":"` + id + `","name":"Playlist","snapshot_id":"version1","tracks":{"offset":0,"items":[null],"next":"https://api.spotify.com/v1/playlists/` + id + `/tracks?offset=1&limit=1"}}`
					} else {
						raw = `{"snapshot_id":"` + revision + `"}`
					}
				}
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(raw))}, nil
			})}
			_, _, _, err := a.fetchPlaylistTracks(t.Context(), id, nil)
			if rootCalls != 1 {
				t.Fatal("overlap proceeded to final revalidation")
			}
			if err == nil {
				t.Fatalf("revision fence: %v", err)
			}
			runtime := a.spotifyTokens()
			snapshot, readErr := a.db.GetSpotifyEntitySnapshot(db.SpotifySnapshotKey{EntityType: "playlist", SpotifyID: id, Resource: "rest:/v1/playlists/" + id + ":page::", ContextKey: runtime.metadataContext})
			if readErr != nil || snapshot != nil {
				t.Fatalf("traversal publication: %+v %v", snapshot, readErr)
			}
		})
	}
}

func TestOAuthPlaylistPartialResumeRevisionFence(t *testing.T) {
	for _, changed := range []bool{false, true} {
		t.Run(fmt.Sprint(changed), func(t *testing.T) {
			a, _ := newBPMRouteTestAPI(t, false)
			credentials, _ := json.Marshal(SpotifyCredentials{ClientId: "client", AccessToken: "fixture", Expiry: time.Now().Add(time.Hour).UnixMilli()})
			if err := a.db.SetSetting("spotify_credentials", string(credentials)); err != nil {
				t.Fatal(err)
			}
			id := strings.Repeat("P", 22)
			revision := "version1"
			calls := map[string]int{}
			a.spotifyHTTPClient = &http.Client{Transport: sessionTransport(func(r *http.Request) (*http.Response, error) {
				status := 200
				offset := r.URL.Query().Get("offset")
				calls[offset]++
				raw := `{"snapshot_id":"` + revision + `"}`
				if r.URL.Query().Get("fields") == "" {
					switch offset {
					case "":
						raw = `{"type":"playlist","id":"` + id + `","snapshot_id":"` + revision + `","tracks":{"offset":0,"items":[null],"next":"https://api.spotify.com/v1/playlists/` + id + `/tracks?offset=1&limit=1"}}`
					case "1":
						raw = `{"offset":1,"items":[null],"next":"https://api.spotify.com/v1/playlists/` + id + `/tracks?offset=2&limit=1"}`
					case "2":
						raw = `{"offset":2,"items":[null],"next":null}`
						if calls[offset] == 1 {
							status = 503
						}
					}
				}
				return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(raw))}, nil
			})}
			if _, _, _, err := a.fetchPlaylistTracks(t.Context(), id, nil); err == nil {
				t.Fatal("interrupted traversal succeeded")
			}
			ctx, cancel := a.spotifyTokens().requestContext(t.Context())
			defer cancel()
			if partial, ok := a.loadPlaylistPartial(ctx, id, revision); !ok || partial.NextOffset != 2 {
				t.Fatal("validated prefix missing")
			}
			if changed {
				revision = "version2"
			}
			if _, _, _, err := a.fetchPlaylistTracks(t.Context(), id, nil); err != nil {
				t.Fatal(err)
			}
			want := 1
			if changed {
				want = 2
			}
			if calls["1"] != want || calls["2"] != 2 {
				t.Fatal("incorrect prefix reuse", calls)
			}
			if _, ok := a.loadPlaylistPartial(ctx, id, revision); ok {
				t.Fatal("completed traversal retained partial")
			}
			if rows, ok := a.completedPlaylistRows(ctx, id, revision); !ok || len(rows) != 3 {
				t.Fatal("completion lost unavailable positions")
			}
			page, err := a.db.GetSpotifyEntitySnapshot(db.SpotifySnapshotKey{EntityType: "playlist", SpotifyID: id, Resource: "rest:/v1/playlists/" + id + "/tracks:page:1:1", ContextKey: a.spotifyTokens().metadataContext})
			if err != nil || page == nil || page.CaptureRevision != revision {
				t.Fatal("resumed page revision not persisted", page, err)
			}
		})
	}
}

func TestOAuthPlaylistSupersededTraversalDiscardsResult(t *testing.T) {
	a, _ := newBPMRouteTestAPI(t, false)
	credentials, _ := json.Marshal(SpotifyCredentials{ClientId: "client", AccessToken: "fixture", Expiry: time.Now().Add(time.Hour).UnixMilli()})
	if err := a.db.SetSetting("spotify_credentials", string(credentials)); err != nil {
		t.Fatal(err)
	}
	id := strings.Repeat("P", 22)
	runtime := a.spotifyTokens()
	a.spotifyHTTPClient = &http.Client{Transport: sessionTransport(func(r *http.Request) (*http.Response, error) {
		// Simulate another owner starting while this root request is in flight.
		if _, err := a.db.BeginSpotifyPlaylistTraversal(runtime.metadataContext, id); err != nil {
			t.Fatal(err)
		}
		raw := `{"type":"playlist","id":"` + id + `","snapshot_id":"revision","tracks":{"offset":0,"items":[null],"next":null}}`
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(raw))}, nil
	})}
	rows, _, _, err := a.fetchPlaylistTracks(t.Context(), id, nil)
	if !errors.Is(err, db.ErrSpotifyTraversalSuperseded) || rows != nil {
		t.Fatal("stale application result returned", rows, err)
	}
	got, err := a.db.GetSpotifyEntitySnapshot(db.SpotifySnapshotKey{EntityType: "playlist", SpotifyID: id, Resource: playlistCompletedResource, ContextKey: runtime.metadataContext})
	if err != nil || got != nil {
		t.Fatal("superseded completion published", got, err)
	}
}
