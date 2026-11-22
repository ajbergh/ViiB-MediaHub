// Verifies the live Web App Recents contract and browser-session routing.
package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ajbergh/viib-mediahub/internal/spotify/catalog"
)

func recentFixture() string {
	track := `{"addedAt":{"day":15,"month":11,"year":2026},"entity":{"_uri":"spotify:track:TTTTTTTTTTTTTTTTTTTTTT","data":{"__typename":"Entity","uri":"spotify:track:TTTTTTTTTTTTTTTTTTTTTT","identityTrait":{"name":"Fixture track","type":"Song","contentHierarchyParent":{"uri":"spotify:album:AAAAAAAAAAAAAAAAAAAAAA","identityTrait":{"name":"Album"}},"contributors":{"items":[{"name":"Artist","uri":"spotify:artist:BBBBBBBBBBBBBBBBBBBBBB"}]}},"consumptionExperienceTrait":{"duration":{"seconds":123,"nanoSeconds":456000000},"contentRatings":["CONTENT_RATING_EXPLICIT"]},"visualIdentityTrait":{"squareCoverImage":{"image":{"data":{"sources":[{"url":"https://images.example/cover","maxWidth":640,"maxHeight":640}]}}}}}},"formatListAttributes":[{"key":"group_id_5","value":""},{"key":"recent_type_played","value":""}]}`
	parent := `{"addedAt":{"day":15,"month":11,"year":2026},"entity":null,"formatListAttributes":[{"key":"group_id_0","value":""},{"key":"children_group_id","value":"5"},{"key":"group_metadata","value":"CAI="}]}`
	return `{"data":{"lists":[{"uri":"spotify:list:recents:page","items":{"items":[` + parent + `,` + track + `,` + track + `],"pagingInfo":{"offset":0,"limit":50,"nextOffset":3},"totalCount":5}}]}}`
}

func TestCookieRecentsRoutingAndFailures(t *testing.T) {
	for _, status := range []int{200, 401, 403, 429, 501} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			a := connectedProxyFixture(t)
			calls := 0
			a.spotifyHTTPClient = &http.Client{Transport: sessionTransport(func(r *http.Request) (*http.Response, error) {
				if r.Method != "POST" || r.Header.Get("Cookie") != "" {
					t.Fatal("unsafe recents request")
				}
				body := `{"granted_token":{"token":"fixture-client","expires_after_seconds":600}}`
				code := 200
				if r.URL.Host != "clienttoken.spotify.com" {
					calls++
					if r.URL.Host != "api-partner.spotify.com" || r.URL.Path != "/pathfinder/v2/query" || r.Header.Get("Client-Token") != "fixture-client" || !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer fixture-bearer-") {
						t.Fatal("wrong recents transport")
					}
					var payload struct {
						Operation string `json:"operationName"`
						Variables struct {
							URIs          []string `json:"uris"`
							Limit, Offset int
						} `json:"variables"`
						Extensions struct {
							Query struct {
								Hash string `json:"sha256Hash"`
							} `json:"persistedQuery"`
						} `json:"extensions"`
					}
					if json.NewDecoder(r.Body).Decode(&payload) != nil || payload.Operation != "recents" || len(payload.Variables.URIs) != 1 || payload.Variables.URIs[0] != "spotify:list:recents:page" || payload.Variables.Limit != 50 || payload.Variables.Offset != 0 || payload.Extensions.Query.Hash != "94be830be40f0f3759ef83a4ea6e4dd1bef1f6108f7502c11ee1fa7fb1bb1658" {
						t.Fatal("recents contract changed")
					}
					code = status
					body = recentFixture()
					if status == 501 {
						code = 200
						body = strings.Replace(body, `"nextOffset":3`, `"nextOffset":null`, 1)
					}
				} else if r.Header.Get("Authorization") != "" {
					t.Fatal("bearer leaked to mint")
				}
				return &http.Response{StatusCode: code, Header: http.Header{"Retry-After": []string{"90"}}, Body: io.NopCloser(strings.NewReader(body))}, nil
			})}
			w := httptest.NewRecorder()
			a.spotifyProxy(w, httptest.NewRequest("GET", "/proxy?path=me/player/recently-played&limit=50", nil))
			if w.Code != status {
				t.Fatalf("status %d: %s", w.Code, w.Body.String())
			}
			if status == 501 && !strings.Contains(w.Body.String(), "spotify_recents_unavailable") {
				t.Fatal("missing typed recents failure")
			}
			if status == 200 {
				var page catalog.RecentsResult
				if json.Unmarshal(w.Body.Bytes(), &page) != nil || page.Source != "webplayer_recents" || len(page.Items) != 3 || page.Total != 5 || page.NextOffset == nil || *page.NextOffset != 3 {
					t.Fatal("recents page lost")
				}
				track := page.Items[1].Track
				if track == nil || track.DurationMS != 123456 || track.Album.ID != "AAAAAAAAAAAAAAAAAAAAAA" || track.Artists[0].Name != "Artist" || track.Explicit == nil || !*track.Explicit || len(track.Album.Images) != 1 || page.Items[0].GroupCount != 2 || page.Items[1].Date != "2026-11-15" || page.Items[2].Track.ID != track.ID {
					t.Fatal("metadata, date or repeated plays lost")
				}
				if strings.Contains(w.Body.String(), "played_at") || strings.Contains(w.Body.String(), "fixture-bearer") {
					t.Fatal("fabricated timestamp or credential exposed")
				}
			}
			if status == 401 && (calls != 2 || a.spotifyAuth.status().Connected) {
				t.Fatal("history rejection not fenced")
			}
			if status == 429 {
				if w.Header().Get("Retry-After") != "90" {
					t.Fatal("missing history cooldown")
				}
				next := httptest.NewRecorder()
				a.spotifyProxy(next, httptest.NewRequest("GET", "/proxy?path=me/player/recently-played&limit=50", nil))
				if next.Code != 429 || calls != 1 {
					t.Fatal("history cooldown sent another request")
				}
			}
		})
	}
}

func TestCookieSpotifyLikedTracksUsesCatalog(t *testing.T) {
	a := connectedProxyFixture(t)
	calls := 0
	a.spotifyHTTPClient = &http.Client{Transport: sessionTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		body := `{"granted_token":{"token":"fixture-client","expires_after_seconds":600}}`
		if r.URL.Host != "clienttoken.spotify.com" {
			var payload map[string]any
			if r.URL.Host != "api-partner.spotify.com" || r.Method != http.MethodPost ||
				json.NewDecoder(r.Body).Decode(&payload) != nil || payload["operationName"] != "fetchLibraryTracks" {
				t.Fatal("unexpected Liked Songs transport")
			}
			body = `{"data":{"me":{"library":{"tracks":{"totalCount":2,"items":[{"track":{"_uri":"spotify:track:TTTTTTTTTTTTTTTTTTTTTT","data":{"name":"Fixture song"}}},{"track":null}]}}}}}`
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body))}, nil
	})}
	w := httptest.NewRecorder()
	a.spotifyProxy(w, httptest.NewRequest(http.MethodGet, "/proxy?path=me/tracks", nil))
	if w.Code != http.StatusOK || calls != 2 || !strings.Contains(w.Body.String(), "Fixture song") ||
		!strings.Contains(w.Body.String(), `{"track":null}`) {
		t.Fatalf("expected liked tracks and their positions, got %d: %s", w.Code, w.Body.String())
	}
}
