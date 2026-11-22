package podcast

import (
	"context"
	"crypto/sha1"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"reflect"
	"testing"
	"time"
)

func TestPodcastIndexAuthHeaderSHA1(t *testing.T) {
	fixed := time.Unix(1700000123, 0).UTC()
	date, token := podcastIndexAuthHeaders("my-key", "my-secret", fixed)
	if date != "1700000123" {
		t.Fatalf("unexpected auth date %q", date)
	}
	expected := fmt.Sprintf("%x", sha1.Sum([]byte("my-keymy-secret1700000123")))
	if token != expected {
		t.Fatalf("auth token mismatch: got %q want %q", token, expected)
	}
}

func TestDirectorySearchAndSanitization(t *testing.T) {
	var capturedPath string
	var capturedQuery string
	var capturedMax string
	var capturedAuthDate string
	var capturedAuthKey string
	var capturedAuth string
	var capturedUA string

	fixedTime := time.Unix(1730000000, 0).UTC()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		capturedQuery = r.URL.Query().Get("q")
		capturedMax = r.URL.Query().Get("max")
		capturedAuthDate = r.Header.Get("X-Auth-Date")
		capturedAuthKey = r.Header.Get("X-Auth-Key")
		capturedAuth = r.Header.Get("Authorization")
		capturedUA = r.Header.Get("User-Agent")

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"status": "true",
			"feeds": [
				{
					"id": 101,
					"title": "<b>Tech</b> Talk",
					"url": "https://feeds.example.org/tech.xml?format=rss",
					"link": "https://tech.example.org/show?utm=1",
					"description": "<p>Hello &amp; welcome<script>alert(1)</script></p>",
					"author": "",
					"ownerName": "Example Network",
					"image": "https://cdn.example.org/fallback.jpg",
					"artwork": "https://cdn.example.org/art.png?w=600",
					"language": "en-us",
					"categories": {"55": "Technology", "10": "News"},
					"episodeCount": 42
				},
				{
					"id": 102,
					"title": "Local SSRF Feed",
					"url": "https://127.0.0.1/private.xml",
					"artwork": "https://cdn.example.org/art.png"
				},
				{
					"id": 103,
					"title": "Plain HTTP Feed",
					"url": "http://insecure.example.org/feed.xml"
				},
				{
					"id": 104,
					"title": "Tokenized Feed",
					"url": "https://private.example.org/feed.xml?token=secret123"
				},
				{
					"id": 105,
					"title": "Unsafe Artwork & Link",
					"url": "https://clean.example.org/podcast.xml",
					"link": "javascript:alert(1)",
					"artwork": "http://192.168.1.5/art.png",
					"image": "https://cdn.example.org/cover.jpg?token=bad",
					"categories": []
				}
			]
		}`))
	}))
	defer server.Close()

	client := &DirectoryClient{
		fetcher: testFetcher(t, server),
		baseURL: "http://public.example/api/1.0",
		now:     func() time.Time { return fixedTime },
	}

	shows, err := client.Search(context.Background(), DirectoryCredentials{
		APIKey:    "test-api-key",
		APISecret: "test-api-secret",
	}, "tech talk", 15)
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}

	if capturedPath != "/api/1.0/search/byterm" || capturedQuery != "tech talk" || capturedMax != "15" {
		t.Fatalf("unexpected request: path=%q q=%q max=%q", capturedPath, capturedQuery, capturedMax)
	}
	if capturedUA != directoryUserAgent {
		t.Fatalf("unexpected User-Agent: %q", capturedUA)
	}
	expectedDate, expectedToken := podcastIndexAuthHeaders("test-api-key", "test-api-secret", fixedTime)
	if capturedAuthDate != expectedDate || capturedAuthKey != "test-api-key" || capturedAuth != expectedToken {
		t.Fatalf("unexpected auth headers: date=%q key=%q auth=%q", capturedAuthDate, capturedAuthKey, capturedAuth)
	}

	if len(shows) != 2 {
		t.Fatalf("expected 2 safe shows after filtering, got %d: %#v", len(shows), shows)
	}

	first := shows[0]
	if first.ID != 101 || first.Title != "Tech Talk" || first.Author != "Example Network" ||
		first.Description != "Hello & welcome" ||
		first.FeedURL != "https://feeds.example.org/tech.xml?format=rss" ||
		first.WebsiteURL != "https://tech.example.org/show?utm=1" ||
		first.ArtworkURL != "https://cdn.example.org/art.png?w=600" ||
		first.Language != "en-us" || first.EpisodeCount != 42 ||
		!reflect.DeepEqual(first.Categories, []string{"News", "Technology"}) {
		t.Fatalf("unexpected normalized first show: %#v", first)
	}

	second := shows[1]
	if second.ID != 105 || second.FeedURL != "https://clean.example.org/podcast.xml" ||
		second.WebsiteURL != "" || second.ArtworkURL != "" || len(second.Categories) != 0 {
		t.Fatalf("unsafe artwork/link leaked: %#v", second)
	}
}

func TestDirectoryTrendingAndParameters(t *testing.T) {
	var capturedPath string
	var capturedCat string
	var capturedLang string
	var capturedMax string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		capturedCat = r.URL.Query().Get("cat")
		capturedLang = r.URL.Query().Get("lang")
		capturedMax = r.URL.Query().Get("max")
		_, _ = w.Write([]byte(`{
			"feeds": [
				{
					"id": 77,
					"title": "Trending Show",
					"url": "https://example.org/trending.xml",
					"author": "Host",
					"image": "https://example.org/cover.webp",
					"categories": ["Science", "Tech"]
				}
			]
		}`))
	}))
	defer server.Close()

	client := &DirectoryClient{
		fetcher: testFetcher(t, server),
		baseURL: "http://public.example/api/1.0",
	}

	shows, err := client.Trending(context.Background(), DirectoryCredentials{
		APIKey:    "k",
		APISecret: "s",
	}, "Science", "en", 99)
	if err != nil {
		t.Fatalf("trending failed: %v", err)
	}
	if capturedPath != "/api/1.0/podcasts/trending" || capturedCat != "Science" || capturedLang != "en" || capturedMax != "40" {
		t.Fatalf("unexpected trending request: path=%q cat=%q lang=%q max=%q", capturedPath, capturedCat, capturedLang, capturedMax)
	}
	if len(shows) != 1 || shows[0].ID != 77 || shows[0].ArtworkURL != "https://example.org/cover.webp" {
		t.Fatalf("unexpected trending shows: %#v", shows)
	}
}

func TestDirectoryErrorMappingAndSSRF(t *testing.T) {
	statusCode := http.StatusUnauthorized
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/1.0/redirect" {
			http.Redirect(w, r, "http://127.0.0.1/secret", http.StatusFound)
			return
		}
		w.WriteHeader(statusCode)
		_, _ = w.Write([]byte(`{"status":"false"}`))
	}))
	defer server.Close()

	client := &DirectoryClient{
		fetcher: testFetcher(t, server),
		baseURL: "http://public.example/api/1.0",
	}
	creds := DirectoryCredentials{APIKey: "k", APISecret: "s"}

	if _, err := client.Search(context.Background(), DirectoryCredentials{}, "test", 10); !errors.Is(err, ErrDirectoryNotConfigured) {
		t.Fatalf("expected ErrDirectoryNotConfigured, got %v", err)
	}

	statusCode = http.StatusUnauthorized
	if _, err := client.Search(context.Background(), creds, "test", 10); !errors.Is(err, ErrDirectoryUnauthorized) {
		t.Fatalf("expected ErrDirectoryUnauthorized for 401, got %v", err)
	}

	statusCode = http.StatusForbidden
	if _, err := client.Trending(context.Background(), creds, "", "", 10); !errors.Is(err, ErrDirectoryUnauthorized) {
		t.Fatalf("expected ErrDirectoryUnauthorized for 403, got %v", err)
	}

	statusCode = http.StatusTooManyRequests
	if _, err := client.Search(context.Background(), creds, "test", 10); !errors.Is(err, ErrDirectoryRateLimited) {
		t.Fatalf("expected ErrDirectoryRateLimited for 429, got %v", err)
	}

	statusCode = http.StatusInternalServerError
	if _, err := client.Search(context.Background(), creds, "test", 10); !errors.Is(err, ErrDirectoryUnavailable) {
		t.Fatalf("expected ErrDirectoryUnavailable for 500, got %v", err)
	}

	// DNS rebinding / private IP must be rejected at dial time.
	client.fetcher.lookup = func(_ context.Context, _ string) ([]netip.Addr, error) {
		return []netip.Addr{netip.MustParseAddr("127.0.0.1")}, nil
	}
	if _, err := client.Search(context.Background(), creds, "test", 10); !errors.Is(err, ErrDirectoryUnavailable) {
		t.Fatalf("expected ErrDirectoryUnavailable on private DNS, got %v", err)
	}
}

func TestEmbeddedDirectoryCredentialsResolution(t *testing.T) {
	t.Setenv("VIIB_PODCAST_INDEX_API_KEY", "")
	t.Setenv("VIIB_PODCAST_INDEX_API_SECRET", "")
	t.Setenv("PODCAST_INDEX_API_KEY", "")
	t.Setenv("PODCAST_INDEX_API_SECRET", "")

	prevKey, prevSecret := embeddedDirectoryAPIKey, embeddedDirectoryAPISecret
	t.Cleanup(func() {
		embeddedDirectoryAPIKey = prevKey
		embeddedDirectoryAPISecret = prevSecret
	})
	embeddedDirectoryAPIKey, embeddedDirectoryAPISecret = "", ""

	if HasEmbeddedDirectoryCredentials() {
		t.Fatal("expected no embedded credentials when vars and env are empty")
	}

	// Env var fallback works for local development.
	t.Setenv("PODCAST_INDEX_API_KEY", " env-key ")
	t.Setenv("PODCAST_INDEX_API_SECRET", " env-secret ")
	creds, ok := EmbeddedDirectoryCredentials()
	if !ok || creds.APIKey != "env-key" || creds.APISecret != "env-secret" {
		t.Fatalf("unexpected env fallback creds: %+v ok=%v", creds, ok)
	}

	// Build-time ldflags vars take precedence over env vars.
	embeddedDirectoryAPIKey = "ld-key"
	embeddedDirectoryAPISecret = "ld-secret"
	creds, ok = EmbeddedDirectoryCredentials()
	if !ok || creds.APIKey != "ld-key" || creds.APISecret != "ld-secret" {
		t.Fatalf("expected build-time embedded creds precedence, got %+v ok=%v", creds, ok)
	}

	// Control characters are rejected cleanly.
	embeddedDirectoryAPIKey = "bad\nkey"
	embeddedDirectoryAPISecret = "secret"
	t.Setenv("PODCAST_INDEX_API_KEY", "")
	t.Setenv("PODCAST_INDEX_API_SECRET", "")
	if _, ok := EmbeddedDirectoryCredentials(); ok {
		t.Fatal("expected control-character credential to be rejected")
	}
}
