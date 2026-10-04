// Tests translation of supported REST resources into account-scoped Web Player catalog responses.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	spotifyauth "github.com/ajbergh/viib-mediahub/internal/spotify/auth"
	"github.com/ajbergh/viib-mediahub/internal/spotify/catalog"
)

func TestCookieCatalogProfileRoutingCacheAndRetirement(t *testing.T) {
	a, _, _ := fixtureCookieRuntime(t)
	if err := a.spotifyAuth.connect(context.Background(), "fixture-cookie"); err != nil {
		t.Fatal(err)
	}
	var mints, profiles atomic.Int32
	a.spotifyHTTPClient = &http.Client{Transport: sessionTransport(func(r *http.Request) (*http.Response, error) {
		if r.Method != "POST" || r.Header.Get("Cookie") != "" {
			t.Fatal("unsafe catalog request")
		}
		body := ""
		switch r.URL.Host {
		case "clienttoken.spotify.com":
			if r.Header.Get("Authorization") != "" || r.Header.Get("Client-Token") != "" {
				t.Fatal("account credential escaped client-token origin")
			}
			var request map[string]any
			if json.NewDecoder(r.Body).Decode(&request) != nil {
				t.Fatal("bad mint")
			}
			data := request["client_data"].(map[string]any)
			if data["client_id"] != "fixture-public-client" {
				t.Fatal("developer context used")
			}
			body = fmt.Sprintf("{\"granted_token\":{\"token\":\"client-%d\",\"expires_after_seconds\":1209600,\"refresh_after_seconds\":1209600}}", mints.Add(1))
		case "api-partner.spotify.com":
			profiles.Add(1)
			if r.URL.Path != "/pathfinder/v2/query" || r.Header.Get("Client-Token") == "" || !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer fixture-bearer-") {
				t.Fatal("wrong catalog context")
			}
			body = "{\"data\":{\"me\":{\"profile\":{\"username\":\"fixture-user\",\"uri\":\"spotify:user:fixture-user\",\"name\":\"Fixture Account\",\"avatar\":null}}}}"
		default:
			t.Fatal("profile reached public API")
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
	})}
	for i := 0; i < 2; i++ {
		w := httptest.NewRecorder()
		a.spotifyGetUserProfile(w, httptest.NewRequest("GET", "/spotify/me", nil))
		if w.Code != 200 {
			t.Fatalf("profile: %d %s", w.Code, w.Body.String())
		}
		var profile catalog.Profile
		if json.Unmarshal(w.Body.Bytes(), &profile) != nil || profile.ID != "fixture-user" || profile.DisplayName != "Fixture Account" || profile.Images == nil {
			t.Fatal("profile conversion failed")
		}
		if profile.Product != nil || profile.Followers != nil || profile.Country != nil || profile.Email != nil {
			t.Fatal("fabricated missing profile fields")
		}
		if profile.ExternalURLs["spotify"] != "https://open.spotify.com/user/fixture-user" {
			t.Fatal("bad profile URL")
		}
		if strings.Contains(w.Body.String(), "fixture-bearer") || strings.Contains(w.Body.String(), "client-1") {
			t.Fatal("credential disclosed")
		}
	}
	if mints.Load() != 1 || profiles.Load() != 2 {
		t.Fatal("client token was not reused")
	}
	old := a.spotifyAuth.catalog
	token, err := a.spotifyAuth.Token(context.Background(), spotifyauth.WebAPI)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.spotifyAuth.connect(context.Background(), "fixture-cookie"); err != nil {
		t.Fatal(err)
	}
	if _, err := old.Profile(context.Background(), token); !errors.Is(err, context.Canceled) {
		t.Fatal("retired catalog client reused", err)
	}
	response, err := a.doSpotifyRequest(context.Background(), "GET", "https://api.spotify.com/v1/me", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if mints.Load() != 2 {
		t.Fatal("replacement account reused old client context")
	}
}
func TestCookieCatalogRateLimitAndRejection(t *testing.T) {
	for _, status := range []int{401, 403, 429} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			a, _, _ := fixtureCookieRuntime(t)
			if err := a.spotifyAuth.connect(context.Background(), "fixture-cookie"); err != nil {
				t.Fatal(err)
			}
			calls := 0
			a.spotifyHTTPClient = &http.Client{Transport: sessionTransport(func(r *http.Request) (*http.Response, error) {
				code := status
				body := "{}"
				if r.URL.Host == "clienttoken.spotify.com" {
					code = 200
					body = "{\"granted_token\":{\"token\":\"fixture-client-token\",\"expires_after_seconds\":600}}"
				} else {
					calls++
				}
				return &http.Response{StatusCode: code, Header: http.Header{"Retry-After": []string{"120"}}, Body: io.NopCloser(strings.NewReader(body))}, nil
			})}
			w := httptest.NewRecorder()
			a.spotifyGetUserProfile(w, httptest.NewRequest("GET", "/spotify/me", nil))
			if w.Code != status {
				t.Fatalf("%d %s", w.Code, w.Body.String())
			}
			if status == 401 {
				if calls != 2 || a.spotifyAuth.status().Connected {
					t.Fatal("rejection lifecycle failed")
				}
			} else if !a.spotifyAuth.status().Connected {
				t.Fatal("non-auth status disconnected account")
			}
			if status == 429 {
				if w.Header().Get("Retry-After") != "120" {
					t.Fatal("missing cooldown")
				}
				_, err := a.doSpotifyRequest(context.Background(), "GET", "https://api.spotify.com/v1/search?q=test", nil, "")
				var limited *spotifyauth.WebPlayerHTTPError
				if !errors.As(err, &limited) || limited.Status != 429 || calls != 1 {
					t.Fatal("cooldown did not suppress another route")
				}
			}
		})
	}
}
func TestCookieCatalogClientTokenExpiry(t *testing.T) {
	a, _, _ := fixtureCookieRuntime(t)
	if err := a.spotifyAuth.connect(context.Background(), "fixture-cookie"); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	mints := 0
	transport := &http.Client{Transport: sessionTransport(func(r *http.Request) (*http.Response, error) {
		body := "{\"data\":{\"me\":{\"profile\":{\"username\":\"user\",\"name\":\"User\",\"avatar\":null}}}}"
		if r.URL.Host == "clienttoken.spotify.com" {
			mints++
			body = "{\"granted_token\":{\"token\":\"client\",\"expires_after_seconds\":120}}"
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
	})}
	a.spotifyAuth.catalog = catalog.New(catalog.Options{Client: transport, Now: func() time.Time { return now }})
	token, err := a.spotifyAuth.Token(context.Background(), spotifyauth.WebAPI)
	if err != nil {
		t.Fatal(err)
	}
	for _, advance := range []time.Duration{0, 90 * time.Second, 20 * time.Second} {
		now = now.Add(advance)
		if _, err := a.spotifyAuth.catalog.Profile(context.Background(), token); err != nil {
			t.Fatal(err)
		}
	}
	if mints != 2 {
		t.Fatal("client-token expiry was not honored")
	}
}

func TestCookieCatalogSchemaFailureKeepsAccount(t *testing.T) {
	for _, body := range []string{
		"{\"errors\":[{\"message\":\"private-upstream-message\"}]}",
		"{\"data\":{\"me\":{\"profile\":{\"username\":\"user\",\"uri\":\"spotify:user:other-account\"}}}}",
		strings.Repeat("x", (1<<20)+1),
	} {
		a, _, _ := fixtureCookieRuntime(t)
		if err := a.spotifyAuth.connect(context.Background(), "fixture-cookie"); err != nil {
			t.Fatal(err)
		}
		a.spotifyHTTPClient = &http.Client{Transport: sessionTransport(func(r *http.Request) (*http.Response, error) {
			value := body
			if r.URL.Host == "clienttoken.spotify.com" {
				value = "{\"granted_token\":{\"token\":\"client\",\"expires_after_seconds\":120}}"
			}
			return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(value))}, nil
		})}
		w := httptest.NewRecorder()
		a.spotifyGetUserProfile(w, httptest.NewRequest("GET", "/spotify/me", nil))
		if w.Code != 503 || !a.spotifyAuth.status().Connected {
			t.Fatal("schema failure became authentication rejection")
		}
		if strings.Contains(w.Body.String(), "private-upstream-message") || strings.Contains(w.Body.String(), "other-account") {
			t.Fatal("raw upstream failure disclosed")
		}
	}
}
func TestCookieCatalogLogoutCancelsPendingProfile(t *testing.T) {
	a, _, _ := fixtureCookieRuntime(t)
	if err := a.spotifyAuth.connect(context.Background(), "fixture-cookie"); err != nil {
		t.Fatal(err)
	}
	entered := make(chan struct{})
	a.spotifyHTTPClient = &http.Client{Transport: sessionTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host == "clienttoken.spotify.com" {
			return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("{\"granted_token\":{\"token\":\"client\",\"expires_after_seconds\":120}}"))}, nil
		}
		close(entered)
		<-r.Context().Done()
		return nil, r.Context().Err()
	})}
	done := make(chan error, 1)
	go func() {
		_, err := a.doSpotifyRequest(context.Background(), "GET", "https://api.spotify.com/v1/me", nil, "")
		done <- err
	}()
	<-entered
	if err := a.spotifyAuth.disconnect(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal("pending profile survived logout", err)
		}
	case <-time.After(time.Second):
		t.Fatal("profile did not cancel")
	}
}

func TestCookieCatalogSearchRoutingAndPaging(t *testing.T) {
	a, _, _ := fixtureCookieRuntime(t)
	if err := a.spotifyAuth.connect(context.Background(), "fixture-cookie"); err != nil {
		t.Fatal(err)
	}
	artistURI := "spotify:artist:" + strings.Repeat("A", 22)
	mints, queries := 0, 0
	a.spotifyHTTPClient = &http.Client{Transport: sessionTransport(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("Cookie") != "" {
			t.Fatal("cookie sent to catalog")
		}
		body := ""
		if r.URL.Host == "clienttoken.spotify.com" {
			mints++
			body = "{\"granted_token\":{\"token\":\"client\",\"expires_after_seconds\":600}}"
		} else {
			if r.URL.Host != "api-partner.spotify.com" {
				t.Fatal("search reached public API")
			}
			queries++
			var request struct {
				Operation string `json:"operationName"`
				Variables struct {
					Term   string `json:"searchTerm"`
					Limit  int    `json:"limit"`
					Offset int    `json:"offset"`
				} `json:"variables"`
			}
			if json.NewDecoder(r.Body).Decode(&request) != nil || request.Operation != "searchDesktop" || request.Variables.Term != "Artist & Friends" || request.Variables.Limit != 1 {
				t.Fatal("wrong search request")
			}
			if request.Variables.Offset != queries-1 {
				t.Fatal("wrong offset")
			}
			body = fmt.Sprintf("{\"data\":{\"searchV2\":{\"artists\":{\"totalCount\":3,\"items\":[{\"data\":{\"__typename\":\"Artist\",\"uri\":%q,\"profile\":{\"name\":\"Artist\"},\"visuals\":{\"avatarImage\":{\"sources\":[]}}}}]}}}}", artistURI)
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
	})}
	for _, target := range []string{"/spotify/search?q=Artist+%26+Friends&type=artist&limit=1&offset=0", "/spotify/proxy?path=search&q=Artist+%26+Friends&type=artist&limit=1&offset=1"} {
		w := httptest.NewRecorder()
		request := httptest.NewRequest("GET", target, nil)
		if strings.Contains(target, "proxy") {
			a.spotifyProxy(w, request)
		} else {
			a.spotifySearch(w, request)
		}
		if w.Code != 200 {
			t.Fatalf("%d %s", w.Code, w.Body.String())
		}
		var result catalog.SearchResult
		if json.Unmarshal(w.Body.Bytes(), &result) != nil || result.Artists == nil || result.Tracks != nil || len(result.Artists.Items) != 1 || result.Artists.Items[0].ExternalURLs["spotify"] == "" || result.Artists.Next == nil {
			t.Fatal("search response contract lost")
		}
	}
	if mints != 1 || queries != 2 {
		t.Fatal("catalog context was not reused")
	}
	for _, target := range []string{"/spotify/search?q=music&type=user", "/spotify/search?q=music&limit=0", "/spotify/search?q=music&offset=-1"} {
		w := httptest.NewRecorder()
		a.spotifySearch(w, httptest.NewRequest("GET", target, nil))
		if w.Code != 400 {
			t.Fatal("invalid search not rejected")
		}
	}
	if queries != 2 {
		t.Fatal("invalid query sent upstream")
	}
}
func TestCookieCatalogSearchRenewalAndCooldown(t *testing.T) {
	for _, status := range []int{401, 429} {
		a, _, authCalls := fixtureCookieRuntime(t)
		if err := a.spotifyAuth.connect(context.Background(), "fixture-cookie"); err != nil {
			t.Fatal(err)
		}
		requests := 0
		a.spotifyHTTPClient = &http.Client{Transport: sessionTransport(func(r *http.Request) (*http.Response, error) {
			if r.URL.Host == "clienttoken.spotify.com" {
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("{\"granted_token\":{\"token\":\"client\",\"expires_after_seconds\":600}}"))}, nil
			}
			requests++
			return &http.Response{StatusCode: status, Header: http.Header{"Retry-After": []string{"90"}}, Body: io.NopCloser(strings.NewReader("{}"))}, nil
		})}
		w := httptest.NewRecorder()
		a.spotifySearch(w, httptest.NewRequest("GET", "/spotify/search?q=music&type=artist", nil))
		if w.Code != status {
			t.Fatalf("status %d", w.Code)
		}
		if status == 401 {
			if requests != 2 || authCalls.Load() != 2 || a.spotifyAuth.status().Connected {
				t.Fatal("search did not renew once before rejection")
			}
		} else {
			if requests != 1 || !a.spotifyAuth.status().Connected || w.Header().Get("Retry-After") != "90" {
				t.Fatal("search cooldown failed")
			}
			w = httptest.NewRecorder()
			a.spotifyGetUserProfile(w, httptest.NewRequest("GET", "/spotify/me", nil))
			if w.Code != 429 || requests != 1 {
				t.Fatal("profile escaped search cooldown")
			}
		}
	}
}

func TestCookieCatalogAlbumPagesAndValidation(t *testing.T) {
	a, _, _ := fixtureCookieRuntime(t)
	if err := a.spotifyAuth.connect(context.Background(), "fixture-cookie"); err != nil {
		t.Fatal(err)
	}
	id := strings.Repeat("B", 22)
	artistURI := "spotify:artist:" + strings.Repeat("A", 22)
	trackURI := "spotify:track:" + strings.Repeat("T", 22)
	mints, queries := 0, 0
	a.spotifyHTTPClient = &http.Client{Transport: sessionTransport(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("Cookie") != "" {
			t.Fatal("cookie escaped")
		}
		body := ""
		if r.URL.Host == "clienttoken.spotify.com" {
			mints++
			body = "{\"granted_token\":{\"token\":\"client\",\"expires_after_seconds\":600}}"
		} else {
			if r.URL.Host != "api-partner.spotify.com" || r.Header.Get("Client-Token") == "" || r.Header.Get("Authorization") == "" {
				t.Fatal("unsafe album route")
			}
			queries++
			var request struct {
				Operation string `json:"operationName"`
				Variables struct {
					URI    string `json:"uri"`
					Limit  int    `json:"limit"`
					Offset int    `json:"offset"`
				} `json:"variables"`
			}
			if json.NewDecoder(r.Body).Decode(&request) != nil || request.Operation != "getAlbum" || request.Variables.URI != "spotify:album:"+id || request.Variables.Limit != 1 || request.Variables.Offset != queries-1 {
				t.Fatal("wrong album request")
			}
			artist := map[string]any{"uri": artistURI, "profile": map[string]any{"name": "Artist"}}
			track := map[string]any{"uri": trackURI, "name": "Track", "trackNumber": queries, "discNumber": 2, "artists": map[string]any{"items": []any{artist}}, "duration": map[string]any{"totalMilliseconds": 123456}, "contentRating": map[string]any{"label": "NONE"}, "playability": map[string]any{"playable": true}}
			album := map[string]any{"__typename": "Album", "uri": "spotify:album:" + id, "name": "Album", "type": "ALBUM", "artists": map[string]any{"items": []any{artist}}, "coverArt": map[string]any{"sources": []any{}}, "date": map[string]any{"isoString": "2020-03-04T00:00:00Z"}, "label": "Label", "copyright": map[string]any{"items": []any{map[string]any{"text": "Copyright", "type": "C"}}}, "tracksV2": map[string]any{"totalCount": 2, "items": []any{map[string]any{"track": track}}}}
			raw, _ := json.Marshal(map[string]any{"data": map[string]any{"albumUnion": album}})
			body = string(raw)
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
	})}
	w := httptest.NewRecorder()
	a.spotifyProxy(w, httptest.NewRequest("GET", "/spotify/proxy?path=albums/"+id+"&limit=1", nil))
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var album catalog.Album
	if json.Unmarshal(w.Body.Bytes(), &album) != nil || album.TotalTracks != 2 || len(album.Tracks.Items) != 1 || album.Tracks.Next == nil || album.ReleaseDate == nil || *album.ReleaseDate != "2020-03-04" || album.Label == nil || album.Copyrights[0].Text != "Copyright" {
		t.Fatal("album metadata mismatch")
	}
	track := album.Tracks.Items[0]
	if track.TrackNumber != 1 || track.DiscNumber != 2 || track.DurationMS != 123456 || track.Explicit == nil || *track.Explicit || track.Album.ID != id {
		t.Fatal("playback fields missing")
	}
	w = httptest.NewRecorder()
	a.spotifyProxy(w, httptest.NewRequest("GET", "/spotify/proxy?path=albums/"+id+"/tracks&limit=1&offset=1", nil))
	var page catalog.Page[catalog.AlbumTrack]
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &page) != nil || page.Next != nil || page.Previous == nil || page.Items[0].TrackNumber != 2 {
		t.Fatal("terminal album page mismatch", w.Body.String())
	}
	if mints != 1 || queries != 2 {
		t.Fatal("client token not reused")
	}
	for _, path := range []string{"albums/bad", "albums/" + id + "/tracks&limit=0", "albums/" + id + "/tracks&offset=-1"} {
		w := httptest.NewRecorder()
		a.spotifyProxy(w, httptest.NewRequest("GET", "/spotify/proxy?path="+path, nil))
		if w.Code != 400 {
			t.Fatal("invalid album query accepted", w.Code)
		}
	}
	if queries != 2 {
		t.Fatal("invalid query reached upstream")
	}
}
func TestCookieCatalogAlbumRenewalCooldownAndSchema(t *testing.T) {
	for _, status := range []int{401, 403, 404, 429, 200} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			a, _, _ := fixtureCookieRuntime(t)
			if err := a.spotifyAuth.connect(context.Background(), "fixture-cookie"); err != nil {
				t.Fatal(err)
			}
			calls := 0
			a.spotifyHTTPClient = &http.Client{Transport: sessionTransport(func(r *http.Request) (*http.Response, error) {
				code := status
				body := "{\"data\":{\"albumUnion\":{}}}"
				if r.URL.Host == "clienttoken.spotify.com" {
					code = 200
					body = "{\"granted_token\":{\"token\":\"client\",\"expires_after_seconds\":600}}"
				} else {
					calls++
				}
				return &http.Response{StatusCode: code, Header: http.Header{"Retry-After": []string{"90"}}, Body: io.NopCloser(strings.NewReader(body))}, nil
			})}
			w := httptest.NewRecorder()
			a.spotifyProxy(w, httptest.NewRequest("GET", "/spotify/proxy?path=albums/"+strings.Repeat("B", 22), nil))
			expected := status
			if status == 200 {
				expected = 503
			}
			if w.Code != expected {
				t.Fatal(w.Code, w.Body.String())
			}
			if status == 401 {
				if calls != 2 || a.spotifyAuth.status().Connected {
					t.Fatal("album rejection did not retire")
				}
			} else if !a.spotifyAuth.status().Connected {
				t.Fatal("non-auth error retired account")
			}
			if status == 429 {
				w2 := httptest.NewRecorder()
				a.spotifyGetUserProfile(w2, httptest.NewRequest("GET", "/spotify/me", nil))
				if w2.Code != 429 || calls != 1 || w2.Header().Get("Retry-After") != "90" {
					t.Fatal("album cooldown not shared")
				}
			}
		})
	}
}

func TestCookieCatalogGroupedAlbumMetadataPages(t *testing.T) {
	a, _, _ := fixtureCookieRuntime(t)
	if err := a.spotifyAuth.connect(context.Background(), "fixture-cookie"); err != nil {
		t.Fatal(err)
	}
	id := strings.Repeat("B", 22)
	queries := 0
	a.spotifyHTTPClient = &http.Client{Transport: sessionTransport(func(r *http.Request) (*http.Response, error) {
		body := ""
		if r.URL.Host == "clienttoken.spotify.com" {
			body = "{\"granted_token\":{\"token\":\"client\",\"expires_after_seconds\":600}}"
		} else {
			if r.URL.Host != "api-partner.spotify.com" {
				t.Fatal("grouped album used public API")
			}
			var request struct {
				Variables struct {
					Offset int `json:"offset"`
					Limit  int `json:"limit"`
				} `json:"variables"`
			}
			if json.NewDecoder(r.Body).Decode(&request) != nil || request.Variables.Limit != 50 || request.Variables.Offset != queries*50 {
				t.Fatal("wrong grouped page")
			}
			queries++
			items := []any{}
			for i := request.Variables.Offset; i < 51 && i < request.Variables.Offset+50; i++ {
				track := map[string]any{"uri": "spotify:track:" + strings.Repeat("T", 22), "name": "Track", "trackNumber": i + 1, "discNumber": 1, "artists": map[string]any{"items": []any{map[string]any{"uri": "spotify:artist:" + strings.Repeat("A", 22), "profile": map[string]any{"name": "Artist"}}}}, "duration": map[string]any{"totalMilliseconds": 123456}}
				if i == 0 {
					track = map[string]any{"playability": map[string]any{"playable": false}}
				}
				items = append(items, map[string]any{"track": track})
			}
			album := map[string]any{"__typename": "Album", "uri": "spotify:album:" + id, "name": "Album", "artists": map[string]any{"items": []any{map[string]any{"uri": "spotify:artist:" + strings.Repeat("A", 22), "profile": map[string]any{"name": "Artist"}}}}, "date": map[string]any{"isoString": "2020-03-04"}, "tracksV2": map[string]any{"totalCount": 51, "items": items}}
			raw, _ := json.Marshal(map[string]any{"data": map[string]any{"albumUnion": album}})
			body = string(raw)
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
	})}
	tracks, _, err := a.fetchAlbumTracks(context.Background(), id)
	if err != nil || queries != 2 || len(tracks) != 50 {
		t.Fatal("grouped album incomplete", len(tracks), queries, err)
	}
	if tracks[0].TrackNumber != 2 || tracks[49].TrackNumber != 51 || tracks[49].DiscNumber != 1 || tracks[49].ReleaseDate != "2020-03-04" || tracks[49].AlbumArtist != "Artist" {
		t.Fatal("grouped metadata lost")
	}
}

func TestCookieCatalogAlbumCommitRejectsReplacedAccount(t *testing.T) {
	a, _, _ := fixtureCookieRuntime(t)
	if err := a.spotifyAuth.connect(context.Background(), "fixture-cookie"); err != nil {
		t.Fatal(err)
	}
	old, cancel := a.spotifyAuth.requestContext(context.Background())
	defer cancel()
	if err := a.spotifyAuth.connect(context.Background(), "fixture-cookie"); err != nil {
		t.Fatal(err)
	}
	committed := false
	if err := a.spotifyAuth.withAccount(old, func() error { committed = true; return nil }); err == nil || committed {
		t.Fatal("old album committed under replacement account")
	}
	current, end := a.spotifyAuth.requestContext(context.Background())
	defer end()
	if err := a.spotifyAuth.withAccount(current, func() error { committed = true; return nil }); err != nil || !committed {
		t.Fatal("current album commit rejected", err)
	}
}

func TestCookieCatalogArtistRoutingAndAlbumResolution(t *testing.T) {
	a, _, _ := fixtureCookieRuntime(t)
	if err := a.spotifyAuth.connect(context.Background(), "fixture-cookie"); err != nil {
		t.Fatal(err)
	}
	artistID := strings.Repeat("A", 22)
	albumID := strings.Repeat("B", 22)
	trackID := strings.Repeat("T", 22)
	mints, artists, albums := 0, 0, 0
	a.spotifyHTTPClient = &http.Client{Transport: sessionTransport(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("Cookie") != "" {
			t.Fatal("cookie escaped")
		}
		body := ""
		if r.URL.Host == "clienttoken.spotify.com" {
			mints++
			body = "{\"granted_token\":{\"token\":\"client\",\"expires_after_seconds\":600}}"
		} else {
			if r.URL.Host != "api-partner.spotify.com" || r.Header.Get("Authorization") == "" || r.Header.Get("Client-Token") == "" {
				t.Fatal("unsafe artist request")
			}
			var request struct {
				Operation string `json:"operationName"`
				Variables struct {
					URI   string `json:"uri"`
					Limit int    `json:"limit"`
				} `json:"variables"`
			}
			if json.NewDecoder(r.Body).Decode(&request) != nil {
				t.Fatal("bad query")
			}
			simpleArtist := map[string]any{"uri": "spotify:artist:" + artistID, "profile": map[string]any{"name": "Artist"}}
			sources := []any{map[string]any{"url": "https://images.example/cover", "height": 640, "width": 640}}
			track := map[string]any{"uri": "spotify:track:" + trackID, "name": "Track", "duration": map[string]any{"totalMilliseconds": 123456}, "artists": map[string]any{"items": []any{simpleArtist}}, "contentRating": map[string]any{"label": "EXPLICIT"}, "albumOfTrack": map[string]any{"uri": "spotify:album:" + albumID, "coverArt": map[string]any{"sources": sources}}}
			var data map[string]any
			switch request.Operation {
			case "queryArtistOverview":
				artists++
				if request.Variables.URI != "spotify:artist:"+artistID {
					t.Fatal("wrong artist")
				}
				data = map[string]any{"artistUnion": map[string]any{"__typename": "Artist", "id": artistID, "profile": map[string]any{"name": "Artist"}, "visuals": map[string]any{"avatarImage": map[string]any{"sources": sources}}, "stats": map[string]any{"followers": 123}, "discography": map[string]any{"topTracks": map[string]any{"items": []any{map[string]any{"track": track}, map[string]any{"track": track}, map[string]any{"track": map[string]any{"playability": map[string]any{"playable": false}}}}}}}}
			case "getAlbum":
				albums++
				if request.Variables.URI != "spotify:album:"+albumID || request.Variables.Limit != 1 {
					t.Fatal("wrong resolution")
				}
				track["trackNumber"] = 1
				track["discNumber"] = 1
				data = map[string]any{"albumUnion": map[string]any{"__typename": "Album", "uri": "spotify:album:" + albumID, "name": "Resolved Album", "type": "ALBUM", "artists": map[string]any{"items": []any{simpleArtist}}, "coverArt": map[string]any{"sources": sources}, "tracksV2": map[string]any{"totalCount": 1, "items": []any{map[string]any{"track": track}}}}}
			default:
				t.Fatal("unexpected operation")
			}
			raw, _ := json.Marshal(map[string]any{"data": data})
			body = string(raw)
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
	})}
	w := httptest.NewRecorder()
	a.spotifyProxy(w, httptest.NewRequest("GET", "/spotify/proxy?path=artists/"+artistID, nil))
	var profile catalog.Artist
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &profile) != nil || profile.ID != artistID || profile.Followers == nil || profile.Followers.Total != 123 || len(profile.Images) != 1 || profile.ExternalURLs["spotify"] == "" {
		t.Fatal("artist profile mismatch", w.Code, w.Body.String())
	}
	if albums != 0 {
		t.Fatal("profile resolved unrequested top tracks")
	}
	w = httptest.NewRecorder()
	a.spotifyProxy(w, httptest.NewRequest("GET", "/spotify/proxy?path=artists/"+artistID+"/top-tracks&market=US", nil))
	var top catalog.ArtistTopTracks
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &top) != nil || len(top.Tracks) != 2 {
		t.Fatal("top tracks mismatch", w.Code, w.Body.String())
	}
	if top.Tracks[0].ID != trackID || top.Tracks[0].DurationMS != 123456 || top.Tracks[0].Album.Name != "Resolved Album" || len(top.Tracks[0].Album.Images) != 1 || top.Tracks[0].Artists[0].Name != "Artist" || top.Tracks[0].Explicit == nil || !*top.Tracks[0].Explicit {
		t.Fatal("playback metadata missing")
	}
	if artists != 2 || albums != 1 || mints != 1 {
		t.Fatal("unnecessary metadata/client-token requests", artists, albums, mints)
	}
	for _, path := range []string{"artists/bad", "artists/" + artistID + "/top-tracks&market=invalid"} {
		w := httptest.NewRecorder()
		a.spotifyProxy(w, httptest.NewRequest("GET", "/spotify/proxy?path="+path, nil))
		if w.Code != 400 {
			t.Fatal("invalid artist query accepted")
		}
	}
	if artists != 2 {
		t.Fatal("invalid request reached upstream")
	}
}
func TestCookieCatalogArtistErrorsAndCooldown(t *testing.T) {
	for _, status := range []int{401, 403, 404, 429, 200} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			a, _, _ := fixtureCookieRuntime(t)
			if err := a.spotifyAuth.connect(context.Background(), "fixture-cookie"); err != nil {
				t.Fatal(err)
			}
			calls := 0
			a.spotifyHTTPClient = &http.Client{Transport: sessionTransport(func(r *http.Request) (*http.Response, error) {
				code := status
				body := "{\"data\":{\"artistUnion\":{}}}"
				if r.URL.Host == "clienttoken.spotify.com" {
					code = 200
					body = "{\"granted_token\":{\"token\":\"client\",\"expires_after_seconds\":600}}"
				} else {
					calls++
				}
				return &http.Response{StatusCode: code, Header: http.Header{"Retry-After": []string{"90"}}, Body: io.NopCloser(strings.NewReader(body))}, nil
			})}
			w := httptest.NewRecorder()
			a.spotifyProxy(w, httptest.NewRequest("GET", "/spotify/proxy?path=artists/"+strings.Repeat("A", 22)+"/top-tracks", nil))
			expected := status
			if status == 200 {
				expected = 503
			}
			if w.Code != expected {
				t.Fatal(w.Code, w.Body.String())
			}
			if status == 401 {
				if calls != 2 || a.spotifyAuth.status().Connected {
					t.Fatal("artist rejection failed")
				}
			} else if !a.spotifyAuth.status().Connected {
				t.Fatal("non-auth error disconnected")
			}
			if status == 429 {
				w2 := httptest.NewRecorder()
				a.spotifyGetUserProfile(w2, httptest.NewRequest("GET", "/spotify/me", nil))
				if w2.Code != 429 || calls != 1 || w2.Header().Get("Retry-After") != "90" {
					t.Fatal("artist cooldown not shared")
				}
			}
		})
	}
}

func TestCookieCatalogArtistIdentityAndUnavailable(t *testing.T) {
	id := strings.Repeat("A", 22)
	for _, test := range []struct {
		body   string
		status int
	}{{"{\"__typename\":\"NotFound\"}", 404}, {"{\"__typename\":\"Artist\",\"id\":\"" + strings.Repeat("B", 22) + "\",\"profile\":{\"name\":\"Wrong Artist\"}}", 503}, {"{\"__typename\":\"Artist\",\"id\":\"" + id + "\",\"uri\":\"spotify:artist:" + strings.Repeat("B", 22) + "\",\"profile\":{\"name\":\"Wrong Artist\"}}", 503}} {
		a, _, _ := fixtureCookieRuntime(t)
		if err := a.spotifyAuth.connect(context.Background(), "fixture-cookie"); err != nil {
			t.Fatal(err)
		}
		a.spotifyHTTPClient = &http.Client{Transport: sessionTransport(func(r *http.Request) (*http.Response, error) {
			body := "{\"data\":{\"artistUnion\":" + test.body + "}}"
			if r.URL.Host == "clienttoken.spotify.com" {
				body = "{\"granted_token\":{\"token\":\"client\",\"expires_after_seconds\":600}}"
			}
			return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
		})}
		w := httptest.NewRecorder()
		a.spotifyProxy(w, httptest.NewRequest("GET", "/spotify/proxy?path=artists/"+id, nil))
		if w.Code != test.status || !a.spotifyAuth.status().Connected {
			t.Fatal("identity/unavailable classification failed", w.Code, w.Body.String())
		}
	}
}

func playlistCatalogFixture(id string, offset, limit, total int) string {
	items := []any{}
	for i := offset; i < total && i < offset+limit; i++ {
		artist := map[string]any{"uri": "spotify:artist:" + strings.Repeat("A", 22), "profile": map[string]any{"name": "Artist"}}
		track := map[string]any{"__typename": "Track", "uri": "spotify:track:" + strings.Repeat("T", 22), "name": "Track", "trackDuration": map[string]any{"totalMilliseconds": 123456}, "artists": map[string]any{"items": []any{artist}}, "contentRating": map[string]any{"label": "EXPLICIT"}, "albumOfTrack": map[string]any{"uri": "spotify:album:" + strings.Repeat("B", 22), "name": "Album", "date": map[string]any{"year": 2020}, "coverArt": map[string]any{"sources": []any{map[string]any{"url": "https://images.example/album"}}}}}
		if i == 100 {
			track = map[string]any{"__typename": "NotFound"}
		}
		items = append(items, map[string]any{"addedAt": map[string]any{"isoString": "2020-03-04T00:00:00Z"}, "itemV2": map[string]any{"data": track}})
	}
	playlist := map[string]any{"__typename": "Playlist", "uri": "spotify:playlist:" + id, "name": "Playlist", "description": "Description", "followers": 123, "revisionId": "version1", "ownerV2": map[string]any{"data": map[string]any{"uri": "spotify:user:owner", "name": "Owner"}}, "images": map[string]any{"items": []any{map[string]any{"sources": []any{map[string]any{"url": "https://images.example/playlist"}}}}}, "content": map[string]any{"totalCount": total, "items": items}}
	raw, _ := json.Marshal(map[string]any{"data": map[string]any{"playlistV2": playlist}})
	return string(raw)
}
func TestCookieCatalogPlaylistRoutingAndGroupedMetadata(t *testing.T) {
	a, _, _ := fixtureCookieRuntime(t)
	if err := a.spotifyAuth.connect(context.Background(), "fixture-cookie"); err != nil {
		t.Fatal(err)
	}
	id := strings.Repeat("P", 22)
	mints, queries := 0, 0
	a.spotifyHTTPClient = &http.Client{Transport: sessionTransport(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("Cookie") != "" {
			t.Fatal("cookie escaped")
		}
		body := ""
		if r.URL.Host == "clienttoken.spotify.com" {
			mints++
			body = "{\"granted_token\":{\"token\":\"client\",\"expires_after_seconds\":600}}"
		} else {
			if r.URL.Host != "api-partner.spotify.com" || r.Header.Get("Client-Token") == "" || r.Header.Get("Authorization") == "" {
				t.Fatal("unsafe playlist route")
			}
			queries++
			var request struct {
				Operation string `json:"operationName"`
				Variables struct {
					URI    string `json:"uri"`
					Limit  int    `json:"limit"`
					Offset int    `json:"offset"`
					Watch  bool   `json:"enableWatchFeedEntrypoint"`
				} `json:"variables"`
			}
			if json.NewDecoder(r.Body).Decode(&request) != nil || request.Operation != "fetchPlaylist" || request.Variables.URI != "spotify:playlist:"+id || request.Variables.Watch {
				t.Fatal("wrong playlist operation")
			}
			body = playlistCatalogFixture(id, request.Variables.Offset, request.Variables.Limit, 102)
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
	})}
	w := httptest.NewRecorder()
	a.spotifyProxy(w, httptest.NewRequest("GET", "/spotify/proxy?path=playlists/"+id+"&limit=1", nil))
	var playlist catalog.Playlist
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &playlist) != nil || playlist.Name != "Playlist" || playlist.Followers == nil || playlist.Followers.Total != 123 || playlist.Owner.DisplayName != "Owner" || playlist.Tracks.Next == nil || playlist.SnapshotID == nil || *playlist.SnapshotID != "version1" || playlist.Public != nil {
		t.Fatal("playlist metadata mismatch", w.Code, w.Body.String())
	}
	track := playlist.Tracks.Items[0].Track
	if track == nil || track.DurationMS != 123456 || track.Album.Name != "Album" || len(track.Album.Images) != 1 || track.Explicit == nil || !*track.Explicit || playlist.Tracks.Items[0].AddedAt == nil {
		t.Fatal("playlist playback fields lost")
	}
	w = httptest.NewRecorder()
	a.spotifyProxy(w, httptest.NewRequest("GET", "/spotify/proxy?path=playlists/"+id+"/tracks&limit=1&offset=101", nil))
	var page catalog.Page[catalog.PlaylistItem]
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &page) != nil || page.Next != nil || page.Previous == nil || page.SnapshotID == nil || page.Items[0].Track == nil {
		t.Fatal("playlist terminal page mismatch", w.Code, w.Body.String())
	}
	tracks, name, image, err := a.fetchPlaylistTracks(context.Background(), id, nil)
	if err != nil || len(tracks) != 101 || name != "Playlist" || image != "https://images.example/playlist" || queries != 4 || mints != 1 {
		t.Fatal("grouped playlist incomplete", len(tracks), queries, err)
	}
	if tracks[0].Album != "Album" || tracks[0].Artist != "Artist" || tracks[0].ReleaseDate != "2020" {
		t.Fatal("grouped playlist metadata lost")
	}
	for _, path := range []string{"playlists/bad", "playlists/" + id + "/tracks&limit=101", "playlists/" + id + "/tracks&offset=-1"} {
		w := httptest.NewRecorder()
		a.spotifyProxy(w, httptest.NewRequest("GET", "/spotify/proxy?path="+path, nil))
		if w.Code != 400 {
			t.Fatal("invalid playlist query accepted")
		}
	}
	if queries != 4 {
		t.Fatal("invalid playlist query reached upstream")
	}
}
func TestCookieCatalogPlaylistErrorsAndCooldown(t *testing.T) {
	for _, status := range []int{401, 403, 404, 429, 200} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			a, _, _ := fixtureCookieRuntime(t)
			if err := a.spotifyAuth.connect(context.Background(), "fixture-cookie"); err != nil {
				t.Fatal(err)
			}
			calls := 0
			a.spotifyHTTPClient = &http.Client{Transport: sessionTransport(func(r *http.Request) (*http.Response, error) {
				code := status
				body := "{\"data\":{\"playlistV2\":{}}}"
				if r.URL.Host == "clienttoken.spotify.com" {
					code = 200
					body = "{\"granted_token\":{\"token\":\"client\",\"expires_after_seconds\":600}}"
				} else {
					calls++
				}
				return &http.Response{StatusCode: code, Header: http.Header{"Retry-After": []string{"90"}}, Body: io.NopCloser(strings.NewReader(body))}, nil
			})}
			w := httptest.NewRecorder()
			a.spotifyProxy(w, httptest.NewRequest("GET", "/spotify/proxy?path=playlists/"+strings.Repeat("P", 22)+"/tracks", nil))
			expected := status
			if status == 200 {
				expected = 503
			}
			if w.Code != expected {
				t.Fatal(w.Code, w.Body.String())
			}
			if status == 401 {
				if calls != 2 || a.spotifyAuth.status().Connected {
					t.Fatal("playlist rejection failed")
				}
			} else if !a.spotifyAuth.status().Connected {
				t.Fatal("non-auth error disconnected")
			}
			if status == 429 {
				w2 := httptest.NewRecorder()
				a.spotifyGetUserProfile(w2, httptest.NewRequest("GET", "/spotify/me", nil))
				if w2.Code != 429 || calls != 1 || w2.Header().Get("Retry-After") != "90" {
					t.Fatal("playlist cooldown not shared")
				}
			}
		})
	}
}

func TestCookieCatalogNestedAccountContextKeepsOriginal(t *testing.T) {
	a, _, _ := fixtureCookieRuntime(t)
	if err := a.spotifyAuth.connect(context.Background(), "fixture-cookie"); err != nil {
		t.Fatal(err)
	}
	old, cancel := a.spotifyAuth.requestContext(context.Background())
	defer cancel()
	original := old.Value(spotifyAccountContextKey{})
	if err := a.spotifyAuth.connect(context.Background(), "fixture-cookie"); err != nil {
		t.Fatal(err)
	}
	nested, end := a.spotifyAuth.requestContext(old)
	defer end()
	if nested.Err() == nil || nested.Value(spotifyAccountContextKey{}) != original {
		t.Fatal("nested request adopted replacement account")
	}
}
func TestCookieCatalogGroupedPlaylistRejectsChangedRevision(t *testing.T) {
	a, _, _ := fixtureCookieRuntime(t)
	if err := a.spotifyAuth.connect(context.Background(), "fixture-cookie"); err != nil {
		t.Fatal(err)
	}
	id := strings.Repeat("P", 22)
	queries := 0
	a.spotifyHTTPClient = &http.Client{Transport: sessionTransport(func(r *http.Request) (*http.Response, error) {
		body := ""
		if r.URL.Host == "clienttoken.spotify.com" {
			body = "{\"granted_token\":{\"token\":\"client\",\"expires_after_seconds\":600}}"
		} else {
			var request struct {
				Variables struct {
					Offset int `json:"offset"`
					Limit  int `json:"limit"`
				} `json:"variables"`
			}
			if json.NewDecoder(r.Body).Decode(&request) != nil {
				t.Fatal("bad query")
			}
			queries++
			body = playlistCatalogFixture(id, request.Variables.Offset, request.Variables.Limit, 102)
			if queries == 2 {
				body = strings.ReplaceAll(body, "version1", "version2")
			}
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
	})}
	tracks, _, _, err := a.fetchPlaylistTracks(context.Background(), id, nil)
	if err == nil || tracks != nil || queries != 2 {
		t.Fatal("changed playlist produced grouped metadata", len(tracks), err)
	}
}

func TestCookieCatalogPlaylistMissingRowsFail(t *testing.T) {
	a, _, _ := fixtureCookieRuntime(t)
	if err := a.spotifyAuth.connect(context.Background(), "fixture-cookie"); err != nil {
		t.Fatal(err)
	}
	id := strings.Repeat("P", 22)
	a.spotifyHTTPClient = &http.Client{Transport: sessionTransport(func(r *http.Request) (*http.Response, error) {
		body := playlistCatalogFixture(id, 0, 0, 2)
		if r.URL.Host == "clienttoken.spotify.com" {
			body = "{\"granted_token\":{\"token\":\"client\",\"expires_after_seconds\":600}}"
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
	})}
	w := httptest.NewRecorder()
	a.spotifyProxy(w, httptest.NewRequest("GET", "/spotify/proxy?path=playlists/"+id, nil))
	if w.Code != 503 || !a.spotifyAuth.status().Connected {
		t.Fatal("incomplete playlist accepted", w.Code)
	}
}

func libraryAlbumFixture(id string) string {
	artist := map[string]any{"uri": "spotify:artist:" + strings.Repeat("A", 22), "profile": map[string]any{"name": "Artist"}}
	track := map[string]any{"uri": "spotify:track:" + strings.Repeat("T", 22), "name": "Track", "trackNumber": 1, "discNumber": 1, "duration": map[string]any{"totalMilliseconds": 123456}, "artists": map[string]any{"items": []any{artist}}}
	album := map[string]any{"__typename": "Album", "uri": "spotify:album:" + id, "name": "Saved Album", "artists": map[string]any{"items": []any{artist}}, "date": map[string]any{"isoString": "2020-03-04"}, "coverArt": map[string]any{"sources": []any{map[string]any{"url": "https://images.example/album"}}}, "tracksV2": map[string]any{"totalCount": 2, "items": []any{map[string]any{"track": track}}}}
	raw, _ := json.Marshal(map[string]any{"data": map[string]any{"albumUnion": album}})
	return string(raw)
}
func TestCookieCatalogLibraryRoutingAndPaging(t *testing.T) {
	a, _, _ := fixtureCookieRuntime(t)
	if err := a.spotifyAuth.connect(context.Background(), "fixture-cookie"); err != nil {
		t.Fatal(err)
	}
	albumID := strings.Repeat("B", 22)
	playlistID := strings.Repeat("P", 22)
	mints, libraries, metadata := 0, 0, 0
	a.spotifyHTTPClient = &http.Client{Transport: sessionTransport(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("Cookie") != "" {
			t.Fatal("cookie escaped")
		}
		body := ""
		if r.URL.Host == "clienttoken.spotify.com" {
			mints++
			body = "{\"granted_token\":{\"token\":\"client\",\"expires_after_seconds\":600}}"
		} else {
			if r.URL.Host != "api-partner.spotify.com" || r.Header.Get("Client-Token") == "" || r.Header.Get("Authorization") == "" {
				t.Fatal("unsafe library route")
			}
			var request struct {
				Operation string `json:"operationName"`
				Variables struct {
					Filters  []string `json:"filters"`
					Features []string `json:"features"`
					Flatten  bool     `json:"flatten"`
					Folders  bool     `json:"includeFoldersWhenFlattening"`
					Limit    int      `json:"limit"`
					Offset   int      `json:"offset"`
				} `json:"variables"`
			}
			if json.NewDecoder(r.Body).Decode(&request) != nil {
				t.Fatal("bad request")
			}
			switch request.Operation {
			case "libraryV3":
				libraries++
				if len(request.Variables.Filters) != 1 || len(request.Variables.Features) != 0 || !request.Variables.Flatten || request.Variables.Folders || request.Variables.Limit != 1 {
					t.Fatal("wrong library variables")
				}
				kind, id := "Album", albumID
				if request.Variables.Filters[0] == "Playlists" {
					kind, id = "Playlist", playlistID
				} else if request.Variables.Filters[0] != "Albums" {
					t.Fatal("wrong filter")
				}
				items := []any{map[string]any{"addedAt": map[string]any{"isoString": "2020-03-04T00:00:00Z"}, "item": map[string]any{"_uri": "spotify:" + strings.ToLower(kind) + ":" + id, "data": map[string]any{"__typename": kind}}}}
				raw, _ := json.Marshal(map[string]any{"data": map[string]any{"me": map[string]any{"libraryV3": map[string]any{"totalCount": 2, "items": items}}}})
				body = string(raw)
			case "getAlbum":
				metadata++
				body = libraryAlbumFixture(albumID)
			case "fetchPlaylist":
				metadata++
				body = playlistCatalogFixture(playlistID, 0, 1, 3)
			default:
				t.Fatal("unexpected operation")
			}
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
	})}
	for _, kind := range []string{"albums", "playlists"} {
		for _, offset := range []int{0, 1} {
			w := httptest.NewRecorder()
			a.spotifyProxy(w, httptest.NewRequest("GET", fmt.Sprintf("/spotify/proxy?path=me/%s&limit=1&offset=%d", kind, offset), nil))
			if w.Code != 200 {
				t.Fatal(w.Code, w.Body.String())
			}
			if kind == "albums" {
				var page catalog.Page[catalog.SavedAlbum]
				if json.Unmarshal(w.Body.Bytes(), &page) != nil || len(page.Items) != 1 || page.Items[0].Album.ID != albumID || page.Items[0].Album.TotalTracks != 2 || page.Items[0].Album.ReleaseDate == nil || *page.Items[0].Album.ReleaseDate != "2020-03-04" || page.Items[0].AddedAt == nil {
					t.Fatal("saved album metadata mismatch")
				}
				if (page.Next != nil) != (offset == 0) {
					t.Fatal("album pagination mismatch")
				}
			} else {
				var page catalog.Page[catalog.Playlist]
				if json.Unmarshal(w.Body.Bytes(), &page) != nil || len(page.Items) != 1 || page.Items[0].ID != playlistID || page.Items[0].Owner.DisplayName != "Owner" || page.Items[0].Tracks.Total != 3 || len(page.Items[0].Images) != 1 {
					t.Fatal("saved playlist metadata mismatch")
				}
				if (page.Next != nil) != (offset == 0) {
					t.Fatal("playlist pagination mismatch")
				}
			}
		}
	}
	if mints != 1 || libraries != 4 || metadata != 4 {
		t.Fatal("library client-token reuse mismatch", mints, libraries, metadata)
	}
	for _, path := range []string{"me/albums&limit=0", "me/playlists&limit=51", "me/albums&offset=-1"} {
		w := httptest.NewRecorder()
		a.spotifyProxy(w, httptest.NewRequest("GET", "/spotify/proxy?path="+path, nil))
		if w.Code != 400 {
			t.Fatal("invalid library query accepted")
		}
	}
	if libraries != 4 {
		t.Fatal("invalid query reached library")
	}
}
func TestCookieCatalogLibraryErrorsAndCooldown(t *testing.T) {
	for _, status := range []int{401, 403, 404, 429, 200} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			a, _, _ := fixtureCookieRuntime(t)
			if err := a.spotifyAuth.connect(context.Background(), "fixture-cookie"); err != nil {
				t.Fatal(err)
			}
			calls := 0
			a.spotifyHTTPClient = &http.Client{Transport: sessionTransport(func(r *http.Request) (*http.Response, error) {
				code := status
				body := "{\"data\":{\"me\":{\"libraryV3\":{}}}}"
				if r.URL.Host == "clienttoken.spotify.com" {
					code = 200
					body = "{\"granted_token\":{\"token\":\"client\",\"expires_after_seconds\":600}}"
				} else {
					calls++
				}
				return &http.Response{StatusCode: code, Header: http.Header{"Retry-After": []string{"90"}}, Body: io.NopCloser(strings.NewReader(body))}, nil
			})}
			w := httptest.NewRecorder()
			a.spotifyProxy(w, httptest.NewRequest("GET", "/spotify/proxy?path=me/albums", nil))
			expected := status
			if status == 200 {
				expected = 503
			}
			if w.Code != expected {
				t.Fatal(w.Code, w.Body.String())
			}
			if status == 401 {
				if calls != 2 || a.spotifyAuth.status().Connected {
					t.Fatal("library rejection failed")
				}
			} else if !a.spotifyAuth.status().Connected {
				t.Fatal("non-auth library error disconnected")
			}
			if status == 429 {
				w2 := httptest.NewRecorder()
				a.spotifyGetUserProfile(w2, httptest.NewRequest("GET", "/spotify/me", nil))
				if w2.Code != 429 || calls != 1 || w2.Header().Get("Retry-After") != "90" {
					t.Fatal("library cooldown not shared")
				}
			}
		})
	}
}

func TestCookieCatalogLibraryPositionsAndMetadataReuse(t *testing.T) {
	const id = "5r9W9MJLvHk83fcZSPQ8SE"
	for _, scenario := range []string{"empty", "beyond-end", "unavailable", "duplicates", "conflict", "wrong-kind", "truncated"} {
		t.Run(scenario, func(t *testing.T) {
			a, _, _ := fixtureCookieRuntime(t)
			if err := a.spotifyAuth.connect(context.Background(), "fixture-cookie"); err != nil {
				t.Fatal(err)
			}
			lookups := 0
			a.spotifyHTTPClient = &http.Client{Transport: sessionTransport(func(r *http.Request) (*http.Response, error) {
				body := ""
				if r.URL.Host == "clienttoken.spotify.com" {
					body = "{\"granted_token\":{\"token\":\"client\",\"expires_after_seconds\":600}}"
				} else {
					var request map[string]any
					if json.NewDecoder(r.Body).Decode(&request) != nil {
						t.Fatal("bad request")
					}
					if request["operationName"] == "getAlbum" {
						lookups++
						body = libraryAlbumFixture(id)
					} else {
						total := 0
						items := []any{}
						album := func() any {
							return map[string]any{"item": map[string]any{"_uri": "spotify:album:" + id, "data": map[string]any{"__typename": "Album"}}}
						}
						switch scenario {
						case "beyond-end":
							total = 1
						case "unavailable":
							total = 1
							items = append(items, map[string]any{"item": map[string]any{"data": map[string]any{"__typename": "NotFound"}}})
						case "duplicates":
							total = 2
							items = append(items, album(), album())
						case "conflict":
							total = 1
							items = append(items, map[string]any{"item": map[string]any{"_uri": "spotify:album:" + id, "data": map[string]any{"__typename": "Album", "uri": "spotify:album:11dFghVXANMlKmJXsNCbNl"}}})
						case "wrong-kind":
							total = 1
							items = append(items, map[string]any{"item": map[string]any{"data": map[string]any{"__typename": "Playlist", "uri": "spotify:playlist:" + id}}})
						case "truncated":
							total = 3
							items = append(items, album())
						}
						encoded, _ := json.Marshal(map[string]any{"data": map[string]any{"me": map[string]any{"libraryV3": map[string]any{"totalCount": total, "items": items}}}})
						body = string(encoded)
					}
				}
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
			})}
			target := "/spotify/proxy?path=me/albums&limit=2"
			if scenario == "beyond-end" {
				target += "&offset=2"
			}
			w := httptest.NewRecorder()
			a.spotifyProxy(w, httptest.NewRequest("GET", target, nil))
			invalid := scenario == "conflict" || scenario == "wrong-kind" || scenario == "truncated"
			if invalid {
				if w.Code != 503 || lookups != 0 {
					t.Fatal("invalid library accepted or looked up", w.Code, lookups)
				}
				return
			}
			var page catalog.Page[catalog.SavedAlbum]
			if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &page) != nil || page.Items == nil {
				t.Fatal("invalid page", w.Code)
			}
			if scenario == "duplicates" {
				if len(page.Items) != 2 || lookups != 1 || page.Items[0].Album.ID != id || page.Items[1].Album.ID != id || page.Items[0].AddedAt != nil {
					t.Fatal("duplicate occurrence or metadata reuse lost")
				}
			} else if scenario == "unavailable" {
				if len(page.Items) != 1 || page.Items[0] != nil || lookups != 0 {
					t.Fatal("unavailable position changed")
				}
			} else if len(page.Items) != 0 || lookups != 0 || page.Next != nil {
				t.Fatal("empty/end page mismatch")
			}
		})
	}
}

func TestCookieCatalogLibraryChildAuthenticationAndCooldown(t *testing.T) {
	const id = "5r9W9MJLvHk83fcZSPQ8SE"
	for _, scenario := range []string{"renewed", "rejected", "throttled"} {
		t.Run(scenario, func(t *testing.T) {
			a, _, _ := fixtureCookieRuntime(t)
			if err := a.spotifyAuth.connect(context.Background(), "fixture-cookie"); err != nil {
				t.Fatal(err)
			}
			libraryCalls, childCalls := 0, 0
			a.spotifyHTTPClient = &http.Client{Transport: sessionTransport(func(r *http.Request) (*http.Response, error) {
				status := 200
				body := ""
				if r.URL.Host == "clienttoken.spotify.com" {
					body = "{\"granted_token\":{\"token\":\"client\",\"expires_after_seconds\":600}}"
				} else {
					var request map[string]any
					if json.NewDecoder(r.Body).Decode(&request) != nil {
						t.Fatal("bad request")
					}
					switch request["operationName"] {
					case "libraryV3":
						libraryCalls++
						encoded, _ := json.Marshal(map[string]any{"data": map[string]any{"me": map[string]any{"libraryV3": map[string]any{"totalCount": 1, "items": []any{map[string]any{"item": map[string]any{"data": map[string]any{"__typename": "Album", "uri": "spotify:album:" + id}}}}}}}})
						body = string(encoded)
					case "getAlbum":
						childCalls++
						status = 401
						if scenario == "throttled" {
							status = 429
						}
						if scenario == "renewed" && childCalls == 2 {
							status = 200
							body = libraryAlbumFixture(id)
						}
					default:
						t.Fatal("unexpected operation after child failure")
					}
				}
				return &http.Response{StatusCode: status, Header: http.Header{"Retry-After": []string{"90"}}, Body: io.NopCloser(strings.NewReader(body))}, nil
			})}
			w := httptest.NewRecorder()
			a.spotifyProxy(w, httptest.NewRequest("GET", "/spotify/proxy?path=me/albums", nil))
			switch scenario {
			case "renewed":
				if w.Code != 200 || libraryCalls != 2 || childCalls != 2 || !a.spotifyAuth.status().Connected {
					t.Fatal("child renewal failed", w.Code, libraryCalls, childCalls)
				}
			case "rejected":
				if w.Code != 401 || libraryCalls != 2 || childCalls != 2 || a.spotifyAuth.status().Connected {
					t.Fatal("repeated child rejection failed", w.Code, libraryCalls, childCalls)
				}
			case "throttled":
				if w.Code != 429 || libraryCalls != 1 || childCalls != 1 || !a.spotifyAuth.status().Connected {
					t.Fatal("child throttle failed", w.Code, libraryCalls, childCalls)
				}
				next := httptest.NewRecorder()
				a.spotifyGetUserProfile(next, httptest.NewRequest("GET", "/spotify/me", nil))
				if next.Code != 429 || next.Header().Get("Retry-After") != "90" || libraryCalls != 1 || childCalls != 1 {
					t.Fatal("child cooldown not shared")
				}
			}
		})
	}
}

// Exercise the real proxy dispatch so artist albums cannot silently fall through to Web API search.
func TestCookieCatalogArtistAlbumsRouting(t *testing.T) {
	a, _, _ := fixtureCookieRuntime(t)
	if err := a.spotifyAuth.connect(context.Background(), "fixture-cookie"); err != nil {
		t.Fatal(err)
	}
	id := strings.Repeat("A", 22)
	albumID := strings.Repeat("B", 22)
	requests := 0
	a.spotifyHTTPClient = &http.Client{Transport: sessionTransport(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("Cookie") != "" {
			t.Fatal("cookie escaped")
		}
		body := `{"granted_token":{"token":"client","expires_after_seconds":600}}`
		if r.URL.Host != "clienttoken.spotify.com" {
			if r.URL.Host != "api-partner.spotify.com" || r.Header.Get("Authorization") == "" || r.Header.Get("Client-Token") == "" {
				t.Fatal("unsafe discography request")
			}
			var request struct {
				Operation string `json:"operationName"`
				Variables struct {
					URI           string `json:"uri"`
					Limit, Offset int
				}
				Extensions struct {
					PersistedQuery struct {
						Hash string `json:"sha256Hash"`
					} `json:"persistedQuery"`
				}
			}
			if json.NewDecoder(r.Body).Decode(&request) != nil || request.Operation != "queryArtistDiscographyAll" || request.Variables.URI != "spotify:artist:"+id || request.Variables.Limit != 100 || request.Variables.Offset != 0 || request.Extensions.PersistedQuery.Hash != "9380995a9d4663cbcb5113fef3c6aabf70ae6d407ba61793fd01e2a1dd6929b0" {
				t.Fatal("wrong discography contract")
			}
			requests++
			body = `{"data":{"artistUnion":{"id":"` + id + `","discography":{"all":{"items":[{"releases":{"items":[{"uri":"spotify:album:` + albumID + `","name":"Artist album","type":"ALBUM","date":{"isoString":"2020-01-01"},"tracks":{"totalCount":10}}]}}]}}}}}`
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
	})}
	w := httptest.NewRecorder()
	a.spotifyProxy(w, httptest.NewRequest("GET", "/spotify/proxy?path=artists/"+id+"/albums&limit=100", nil))
	var result catalog.ArtistAlbums
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &result) != nil || len(result.Items) != 1 || result.Items[0].ID != albumID || result.Items[0].Name != "Artist album" {
		t.Fatal("discography proxy failed", w.Code, w.Body.String())
	}
	for _, path := range []string{"artists/bad/albums", "artists/" + id + "/albums&offset=1", "artists/" + id + "/albums&limit=101"} {
		w = httptest.NewRecorder()
		a.spotifyProxy(w, httptest.NewRequest("GET", "/spotify/proxy?path="+path, nil))
		if w.Code != 400 {
			t.Fatal("invalid discography input accepted", w.Code)
		}
	}
	if requests != 1 {
		t.Fatal("invalid requests reached upstream", requests)
	}
}
