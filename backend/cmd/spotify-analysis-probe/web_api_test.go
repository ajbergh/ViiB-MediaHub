//go:build spotify_research

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/spotify/auth"
)

const webTestID = "5r9W9MJLvHk83fcZSPQ8SE"

type webTestTransport func(*http.Request) (*http.Response, error)

func (f webTestTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func webTestToken() auth.Token {
	return auth.NewToken("fixture-secret-bearer", auth.WebPlayer, time.Now().Add(time.Hour), 1)
}
func fixtureCatalogTrack() map[string]any {
	return map[string]any{"id": webTestID, "name": "private track name", "duration_ms": 123000.0,
		"artists": []any{map[string]any{"id": webTestID, "name": "private artist"}},
		"album":   map[string]any{"id": webTestID, "name": "private album", "images": []any{}}}
}
func fixturePage(item any) map[string]any {
	return map[string]any{"items": []any{item}, "total": 1.0, "next": "https://untrusted.invalid/next"}
}
func fixtureProfile() map[string]any {
	return map[string]any{
		"id": "private-account-id", "display_name": "private display name", "email": "private@example.invalid",
		"product": "premium", "country": "US", "images": []any{}, "followers": map[string]any{"total": 2.0},
		"external_urls": map[string]any{"spotify": "https://open.spotify.com/user/private-account-id"}}
}
func fixtureWebAPI(path string) any {
	switch path {
	case "/v1/me":
		return fixtureProfile()
	case "/v1/tracks/" + webTestID:
		return fixtureCatalogTrack()
	case "/v1/search":
		named := map[string]any{"id": webTestID, "name": "private search result"}
		return map[string]any{"tracks": fixturePage(fixtureCatalogTrack()), "albums": fixturePage(named), "artists": fixturePage(named), "playlists": fixturePage(named)}
	case "/v1/me/albums":
		return fixturePage(map[string]any{"album": map[string]any{"id": webTestID, "name": "private saved album"}})
	case "/v1/me/playlists":
		return fixturePage(map[string]any{"id": webTestID, "name": "private saved playlist"})
	case "/v1/me/player/recently-played":
		return map[string]any{"items": []any{map[string]any{"track": fixtureCatalogTrack()}}}
	case "/v1/albums/" + webTestID:
		return map[string]any{"id": webTestID, "name": "private album", "images": []any{}, "release_date": "2020-01-01", "tracks": fixturePage(fixtureCatalogTrack())}
	case "/v1/artists/" + webTestID:
		return map[string]any{"id": webTestID, "name": "private artist", "images": []any{}}
	case "/v1/artists/" + webTestID + "/top-tracks":
		return map[string]any{"tracks": []any{fixtureCatalogTrack()}}
	case "/v1/playlists/" + webTestID:
		return map[string]any{"id": webTestID, "name": "private playlist", "owner": map[string]any{"id": "private-account-id"},
			"images": []any{}, "tracks": map[string]any{"total": 1.0}}
	case "/v1/playlists/" + webTestID + "/tracks":
		return fixturePage(map[string]any{"track": fixtureCatalogTrack()})
	default:
		return map[string]any{}
	}
}
func fixtureWebResponse(r *http.Request, status int, payload any) *http.Response {
	encoded, _ := json.Marshal(payload)
	return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(encoded)), Request: r}
}
func TestWebAPIFixedRoutesReadOnlyBoundedAndRedacted(t *testing.T) {
	var requests []*http.Request
	jar, _ := cookiejar.New(nil)
	origin, _ := url.Parse("https://api.spotify.com")
	jar.SetCookies(origin, []*http.Cookie{{Name: "sp_dc", Value: "fixture-cookie"}})
	client := &http.Client{Jar: jar, Transport: webTestTransport(func(r *http.Request) (*http.Response, error) {
		requests = append(requests, r)
		if r.Method != "GET" || r.URL.Scheme != "https" || r.URL.Host != "api.spotify.com" || r.Header.Get("Cookie") != "" ||
			r.Header.Get("Authorization") != "Bearer fixture-secret-bearer" {
			t.Fatalf("unsafe request %s", r.URL)
		}
		if r.URL.Query().Get("limit") != "" && r.URL.Query().Get("limit") != "1" {
			t.Fatal("page not bounded")
		}
		return fixtureWebResponse(r, 200, fixtureWebAPI(r.URL.Path)), nil
	})}
	var output bytes.Buffer
	if err := probeWebAPI(context.Background(), webTestToken(), webTestID, true, client, &output); err != nil {
		t.Fatal(err)
	}
	var report webAPIReport
	if err := json.Unmarshal(output.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if len(requests) != 11 || len(report.Results) != 11 || !report.RenewalVerified || report.StoppedEarly {
		t.Fatalf("matrix %+v calls=%d", report, len(requests))
	}
	for _, result := range report.Results {
		if result.Code != "compatible" || (result.ShapeCompatible == nil || !*result.ShapeCompatible) {
			t.Fatalf("result %+v", result)
		}
	}
	for _, secret := range []string{"private", "fixture-secret-bearer", "fixture-cookie", webTestID, "untrusted.invalid"} {
		if strings.Contains(output.String(), secret) {
			t.Fatalf("diagnostic leaked %s", secret)
		}
	}
	if requests[2].URL.Query().Get("q") != "music" {
		t.Fatal("search not fixed")
	}
}
func TestWebAPIStopsOnRateLimitAndTokenRejection(t *testing.T) {
	for _, status := range []int{401, 429} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			calls := 0
			client := &http.Client{Transport: webTestTransport(func(r *http.Request) (*http.Response, error) {
				calls++
				response := fixtureWebResponse(r, status, map[string]any{"error": "secret upstream body"})
				response.Header.Set("Retry-After", "123")
				return response, nil
			})}
			var output bytes.Buffer
			if err := probeWebAPI(context.Background(), webTestToken(), webTestID, false, client, &output); err != nil {
				t.Fatal(err)
			}
			var report webAPIReport
			json.Unmarshal(output.Bytes(), &report)
			if calls != 1 || !report.StoppedEarly || len(report.Results) != 1 {
				t.Fatal("did not stop")
			}
			if status == 429 && (report.CoverageState != "inconclusive_rate_limited" || report.Results[0].ShapeCompatible != nil) {
				t.Fatal("rate limiting misreported as incompatible")
			}
			if status == 429 && report.Results[0].RetryAfterSeconds != 123 {
				t.Fatal("cooldown lost")
			}
			if strings.Contains(output.String(), "secret") {
				t.Fatal("upstream body leaked")
			}
		})
	}
}
func TestWebAPIDenialSchemaAndMarketAreDistinct(t *testing.T) {
	for _, kind := range []string{"denied", "wrong_id", "market", "malformed", "too_large", "network"} {
		t.Run(kind, func(t *testing.T) {
			client := &http.Client{Transport: webTestTransport(func(r *http.Request) (*http.Response, error) {
				data := fixtureCatalogTrack()
				status := 200
				switch kind {
				case "denied":
					status = 403
				case "wrong_id":
					data["id"] = "11dFghVXANMlKmJXsNCbNl"
				case "market":
					data["is_playable"] = false
				case "malformed":
					return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("invalid secret")), Header: make(http.Header)}, nil
				case "too_large":
					return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(strings.Repeat("s", 1<<20+1))), Header: make(http.Header)}, nil
				case "network":
					return nil, errors.New("secret token url")
				}
				return fixtureWebResponse(r, status, data), nil
			})}
			result, _ := webAPIRequest(context.Background(), client, webTestToken(), "track", "tracks/"+webTestID, webTestID)
			expected := map[string]string{"denied": "access_or_scope_denied", "wrong_id": "schema_incompatible", "market": "market_unavailable", "malformed": "schema_incompatible", "too_large": "payload_too_large", "network": "temporarily_unavailable"}[kind]
			if result.Code != expected {
				t.Fatalf("%+v expected=%s", result, expected)
			}
		})
	}
}
func TestWebAPIEmptyLibraryAndMissingDependencies(t *testing.T) {
	calls := 0
	client := &http.Client{Transport: webTestTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		if strings.Contains(r.URL.Path, "/me/") {
			return fixtureWebResponse(r, 200, map[string]any{"items": []any{}, "total": 0.0}), nil
		}
		if r.URL.Path == "/v1/me" {
			return fixtureWebResponse(r, 200, fixtureProfile()), nil
		}
		return fixtureWebResponse(r, 404, nil), nil
	})}
	var output bytes.Buffer
	if err := probeWebAPI(context.Background(), webTestToken(), webTestID, true, client, &output); err != nil {
		t.Fatal(err)
	}
	var report webAPIReport
	json.Unmarshal(output.Bytes(), &report)
	if calls != 6 || len(report.Results) != 11 {
		t.Fatal("empty library enumerated dependencies")
	}
	if report.Results[3].Code != "compatible_empty" || (report.Results[3].ShapeCompatible == nil || !*report.Results[3].ShapeCompatible) {
		t.Fatal("empty album library incompatible")
	}
	for _, result := range report.Results[6:] {
		if result.Code != "skipped_dependency_unavailable" {
			t.Fatal(result)
		}
	}
}
func TestWebAPIBlocksRedirectsAndMixedModes(t *testing.T) {
	calls := 0
	client := &http.Client{Transport: webTestTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		response := fixtureWebResponse(r, 302, nil)
		response.Header.Set("Location", "https://untrusted.invalid/secret")
		return response, nil
	})}
	var output bytes.Buffer
	if err := probeWebAPI(context.Background(), webTestToken(), webTestID, false, client, &output); err != nil {
		t.Fatal(err)
	}
	if calls != 6 || strings.Contains(output.String(), "untrusted") {
		t.Fatal("redirect followed or exposed")
	}
	if validateWebAPIOptions(true, 4096, false, false) == nil || validateWebAPIOptions(true, 0, true, false) == nil ||
		validateWebAPIOptions(true, 0, false, true) == nil {
		t.Fatal("mixed modes accepted")
	}
	wrong := auth.NewToken("secret", auth.OAuth, time.Now().Add(time.Hour), 1)
	if err := probeWebAPI(context.Background(), wrong, webTestID, false, client, &output); err == nil {
		t.Fatal("OAuth used by WebPlayer probe")
	}
}

// Each group can run independently of profile; no group follows pagination.
func TestWebAPIIndependentSurfaces(t *testing.T) {
	for _, test := range []struct {
		surface string
		calls   int
	}{{"profile", 1}, {"track", 1}, {"search", 1}, {"library", 5}, {"catalog", 3}, {"recent", 1}, {"all", 11}} {
		t.Run(test.surface, func(t *testing.T) {
			calls := 0
			client := &http.Client{Transport: webTestTransport(func(r *http.Request) (*http.Response, error) {
				calls++
				if test.surface != "all" && test.surface != "profile" && r.URL.Path == "/v1/me" {
					t.Fatal("group depends on profile")
				}
				if r.URL.Host != "api.spotify.com" || r.Method != "GET" || r.Header.Get("Cookie") != "" {
					t.Fatal("unsafe request")
				}
				return fixtureWebResponse(r, 200, fixtureWebAPI(r.URL.Path)), nil
			})}
			var output bytes.Buffer
			if err := probeWebAPISurface(context.Background(), webTestToken(), webTestID, true, test.surface, false, client, &output); err != nil {
				t.Fatal(err)
			}
			var report webAPIReport
			if err := json.Unmarshal(output.Bytes(), &report); err != nil {
				t.Fatal(err)
			}
			if calls != test.calls || report.RequestCount != calls || report.Surface != test.surface || report.CoverageState != "compatible_sampled_routes" {
				t.Fatalf("report %+v calls=%d", report, calls)
			}
		})
	}
	for _, surface := range []string{"profile", "track", "search", "library", "catalog", "recent", "all"} {
		for _, status := range []int{401, 429} {
			calls := 0
			client := &http.Client{Transport: webTestTransport(func(r *http.Request) (*http.Response, error) { calls++; return fixtureWebResponse(r, status, nil), nil })}
			var output bytes.Buffer
			if err := probeWebAPISurface(context.Background(), webTestToken(), webTestID, false, surface, false, client, &output); err != nil {
				t.Fatal(err)
			}
			if calls != 1 {
				t.Fatalf("%s status %d continued after rejection", surface, status)
			}
		}
	}
	if err := probeWebAPISurface(context.Background(), webTestToken(), webTestID, false, "https://untrusted.invalid", false, nil, io.Discard); err == nil {
		t.Fatal("arbitrary group accepted")
	}
}

func TestWebAPIReconnectAcquiresFreshProviderAndReportsSeparately(t *testing.T) {
	for _, status := range []int{200, 429} {
		tokenCalls := 0
		opts := auth.WebPlayerOptions{Client: &http.Client{Transport: webTestTransport(func(r *http.Request) (*http.Response, error) {
			if r.URL.Host != "open.spotify.com" {
				t.Fatal("wrong auth origin")
			}
			if r.URL.Path == "/api/server-time" {
				return fixtureWebResponse(r, 200, map[string]any{"serverTime": time.Now().Unix()}), nil
			}
			tokenCalls++
			return fixtureWebResponse(r, 200, map[string]any{"accessToken": "fixture-secret-bearer", "isAnonymous": false, "accessTokenExpirationTimestampMs": time.Now().Add(time.Hour).UnixMilli()}), nil
		})}}
		apiCalls := 0
		client := &http.Client{Transport: webTestTransport(func(r *http.Request) (*http.Response, error) {
			apiCalls++
			return fixtureWebResponse(r, status, fixtureProfile()), nil
		})}
		var output bytes.Buffer
		if err := runWebAPIResearch(context.Background(), "fixture-cookie", researchContract(), webTestID, "profile", true, true, opts, client, &output); err != nil {
			t.Fatal(err)
		}
		var report webAPIReport
		if err := json.Unmarshal(output.Bytes(), &report); err != nil {
			t.Fatal(err)
		}
		if tokenCalls != 3 || apiCalls != 1 || !report.ReconnectVerified || !report.RenewalVerified {
			t.Fatalf("lifecycle report %+v tokenCalls=%d", report, tokenCalls)
		}
		if status == 429 && report.CoverageState != "inconclusive_rate_limited" {
			t.Fatal("reconnect incorrectly established API access")
		}
		if strings.Contains(output.String(), "fixture-secret") || strings.Contains(output.String(), "fixture-cookie") {
			t.Fatal("secret leaked")
		}
	}
}
