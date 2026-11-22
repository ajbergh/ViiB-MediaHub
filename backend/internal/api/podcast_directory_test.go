package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/ajbergh/viib-mediahub/internal/db"
	"github.com/ajbergh/viib-mediahub/internal/podcast"
	"github.com/go-chi/chi/v5"
)

func mountPodcastDirectoryTestRouter(a *API) http.Handler {
	r := chi.NewRouter()
	r.Get("/discover/status", a.getPodcastDiscoverStatus)
	r.Put("/discover/credentials", a.putPodcastDiscoverCredentials)
	r.Delete("/discover/credentials", a.deletePodcastDiscoverCredentials)
	r.Get("/discover/search", a.searchPodcastDirectory)
	r.Get("/discover/trending", a.trendingPodcastDirectory)
	return r
}

type httptestDirectoryClient struct {
	serverURL string
}

func (c *httptestDirectoryClient) Search(ctx context.Context, creds podcast.DirectoryCredentials, query string, maxResults int) ([]podcast.DirectoryShow, error) {
	u, _ := url.Parse(c.serverURL + "/search/byterm")
	q := u.Query()
	q.Set("q", query)
	q.Set("max", strconv.Itoa(maxResults))
	u.RawQuery = q.Encode()
	return c.do(ctx, creds, u.String())
}

func (c *httptestDirectoryClient) Trending(ctx context.Context, creds podcast.DirectoryCredentials, category, lang string, maxResults int) ([]podcast.DirectoryShow, error) {
	u, _ := url.Parse(c.serverURL + "/podcasts/trending")
	q := u.Query()
	q.Set("max", strconv.Itoa(maxResults))
	if category != "" {
		q.Set("cat", category)
	}
	if lang != "" {
		q.Set("lang", lang)
	}
	u.RawQuery = q.Encode()
	return c.do(ctx, creds, u.String())
}

func (c *httptestDirectoryClient) do(ctx context.Context, creds podcast.DirectoryCredentials, target string) ([]podcast.DirectoryShow, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, podcast.ErrDirectoryUnavailable
	}
	req.Header.Set("X-Auth-Key", creds.APIKey)
	req.Header.Set("X-Test-Secret", creds.APISecret)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, podcast.ErrDirectoryUnavailable
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusUnauthorized, http.StatusForbidden:
		return nil, podcast.ErrDirectoryUnauthorized
	case http.StatusTooManyRequests:
		return nil, podcast.ErrDirectoryRateLimited
	default:
		return nil, podcast.ErrDirectoryUnavailable
	}
	var payload struct {
		Shows []podcast.DirectoryShow `json:"shows"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, podcast.ErrDirectoryUnavailable
	}
	return payload.Shows, nil
}

func TestPodcastDirectoryHandlersLifecycleSearchTrendingAndErrors(t *testing.T) {
	database, err := db.New(filepath.Join(t.TempDir(), "podcast-dir.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	var upstreamStatus = http.StatusOK
	var lastAuthKey string
	var lastAuthSecret string
	var lastQuery string
	var lastCat string
	var lastLang string
	var lastMax string

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lastAuthKey = r.Header.Get("X-Auth-Key")
		lastAuthSecret = r.Header.Get("X-Test-Secret")
		lastMax = r.URL.Query().Get("max")
		if upstreamStatus != http.StatusOK {
			w.WriteHeader(upstreamStatus)
			return
		}
		switch r.URL.Path {
		case "/search/byterm":
			lastQuery = r.URL.Query().Get("q")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"shows": []podcast.DirectoryShow{
					{
						ID:         1,
						Title:      "Go Time",
						Author:     "Changelog",
						FeedURL:    "https://changelog.com/gotime/feed",
						ArtworkURL: "https://cdn.changelog.com/gotime.png",
					},
				},
			})
		case "/podcasts/trending":
			lastCat = r.URL.Query().Get("cat")
			lastLang = r.URL.Query().Get("lang")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"shows": []podcast.DirectoryShow{
					{
						ID:      2,
						Title:   "Trending Tech",
						FeedURL: "https://example.org/trending.xml",
					},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	a := &API{db: database}
	a.setPodcastDirectoryClientForTest(&httptestDirectoryClient{serverURL: upstream.URL})
	defer a.setPodcastDirectoryClientForTest(nil)
	router := mountPodcastDirectoryTestRouter(a)

	req := func(method, path, body string) *httptest.ResponseRecorder {
		t.Helper()
		rec := httptest.NewRecorder()
		r := httptest.NewRequest(method, path, bytes.NewBufferString(body))
		if body != "" {
			r.Header.Set("Content-Type", "application/json")
		}
		router.ServeHTTP(rec, r)
		return rec
	}

	// Initial unconfigured state
	statusRec := req(http.MethodGet, "/discover/status", "")
	if statusRec.Code != http.StatusOK ||
		!strings.Contains(statusRec.Body.String(), `"configured":false`) ||
		!strings.Contains(statusRec.Body.String(), `"provider":"none"`) {
		t.Fatalf("unexpected initial status: %d %s", statusRec.Code, statusRec.Body.String())
	}

	// Unconfigured search & trending return 412 provider_not_configured
	unconfSearch := req(http.MethodGet, "/discover/search?q=golang", "")
	if unconfSearch.Code != http.StatusPreconditionFailed ||
		!strings.Contains(unconfSearch.Body.String(), `"provider_not_configured"`) {
		t.Fatalf("expected 412 provider_not_configured on search, got %d %s", unconfSearch.Code, unconfSearch.Body.String())
	}
	unconfTrending := req(http.MethodGet, "/discover/trending", "")
	if unconfTrending.Code != http.StatusPreconditionFailed ||
		!strings.Contains(unconfTrending.Body.String(), `"provider_not_configured"`) {
		t.Fatalf("expected 412 provider_not_configured on trending, got %d %s", unconfTrending.Code, unconfTrending.Body.String())
	}

	// Strict validation on PUT /discover/credentials
	for _, bad := range []string{
		`{"apiKey":"","apiSecret":"secret"}`,
		`{"apiKey":"key","apiSecret":""}`,
		`{"apiKey":"key","apiSecret":"secret","extra":true}`,
		`{"apiKey":"key","apiSecret":"secret"}{}`,
		`{"apiKey":"` + strings.Repeat("a", 257) + `","apiSecret":"secret"}`,
		`{"apiKey":"key\n","apiSecret":"secret"}`,
		strings.Repeat(" ", 1025),
	} {
		res := req(http.MethodPut, "/discover/credentials", bad)
		if res.Code != http.StatusBadRequest {
			t.Fatalf("accepted invalid credentials %q: %d %s", bad, res.Code, res.Body.String())
		}
	}

	// Missing Content-Type rejected
	noCTRec := httptest.NewRecorder()
	router.ServeHTTP(noCTRec, httptest.NewRequest(http.MethodPut, "/discover/credentials", strings.NewReader(`{"apiKey":"k","apiSecret":"s"}`)))
	if noCTRec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 when Content-Type missing, got %d", noCTRec.Code)
	}

	// Save valid credentials; response must mask key and never contain secret or full key
	const apiKey = "pi-live-key-9876"
	const apiSecret = "pi-live-secret-do-not-leak"
	putRec := req(http.MethodPut, "/discover/credentials", `{"apiKey":"`+apiKey+`","apiSecret":"`+apiSecret+`"}`)
	if putRec.Code != http.StatusOK {
		t.Fatalf("save credentials failed: %d %s", putRec.Code, putRec.Body.String())
	}
	putBody := putRec.Body.String()
	if !strings.Contains(putBody, `"configured":true`) ||
		!strings.Contains(putBody, `"provider":"podcast_index"`) ||
		!strings.Contains(putBody, `"keyHint":"••••9876"`) {
		t.Fatalf("unexpected save status body: %s", putBody)
	}
	if strings.Contains(putBody, apiKey) || strings.Contains(putBody, apiSecret) {
		t.Fatalf("credentials leaked in PUT response: %s", putBody)
	}

	// Query validation on search & trending
	if badQ := req(http.MethodGet, "/discover/search?q=", ""); badQ.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for empty q, got %d", badQ.Code)
	}
	if longQ := req(http.MethodGet, "/discover/search?q="+strings.Repeat("a", 201), ""); longQ.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for >200 char q, got %d", longQ.Code)
	}
	if badMax := req(http.MethodGet, "/discover/search?q=go&max=41", ""); badMax.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for max=41, got %d", badMax.Code)
	}
	if badMax0 := req(http.MethodGet, "/discover/trending?max=0", ""); badMax0.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for max=0, got %d", badMax0.Code)
	}

	// Successful search
	searchRec := req(http.MethodGet, "/discover/search?q=go+time&max=10", "")
	if searchRec.Code != http.StatusOK || !strings.Contains(searchRec.Body.String(), `"Go Time"`) {
		t.Fatalf("search failed: %d %s", searchRec.Code, searchRec.Body.String())
	}
	if lastAuthKey != apiKey || lastAuthSecret != apiSecret || lastQuery != "go time" || lastMax != "10" {
		t.Fatalf("unexpected upstream search call: key=%q secret=%q q=%q max=%q", lastAuthKey, lastAuthSecret, lastQuery, lastMax)
	}

	// Successful trending
	trendRec := req(http.MethodGet, "/discover/trending?category=Technology&lang=en&max=15", "")
	if trendRec.Code != http.StatusOK || !strings.Contains(trendRec.Body.String(), `"Trending Tech"`) {
		t.Fatalf("trending failed: %d %s", trendRec.Code, trendRec.Body.String())
	}
	if lastCat != "Technology" || lastLang != "en" || lastMax != "15" {
		t.Fatalf("unexpected upstream trending params: cat=%q lang=%q max=%q", lastCat, lastLang, lastMax)
	}

	// Upstream 401 -> 401 directory_unauthorized without leaking credentials
	upstreamStatus = http.StatusUnauthorized
	unauthRec := req(http.MethodGet, "/discover/search?q=go", "")
	if unauthRec.Code != http.StatusUnauthorized || !strings.Contains(unauthRec.Body.String(), `"directory_unauthorized"`) {
		t.Fatalf("expected 401 directory_unauthorized, got %d %s", unauthRec.Code, unauthRec.Body.String())
	}
	if strings.Contains(unauthRec.Body.String(), apiKey) || strings.Contains(unauthRec.Body.String(), apiSecret) {
		t.Fatal("credentials leaked in 401 error response")
	}

	// Upstream 429 -> 429 directory_rate_limited
	upstreamStatus = http.StatusTooManyRequests
	rateRec := req(http.MethodGet, "/discover/trending", "")
	if rateRec.Code != http.StatusTooManyRequests || !strings.Contains(rateRec.Body.String(), `"directory_rate_limited"`) {
		t.Fatalf("expected 429 directory_rate_limited, got %d %s", rateRec.Code, rateRec.Body.String())
	}

	// Upstream 500 -> 502 directory_unavailable
	upstreamStatus = http.StatusInternalServerError
	errRec := req(http.MethodGet, "/discover/search?q=go", "")
	if errRec.Code != http.StatusBadGateway || !strings.Contains(errRec.Body.String(), `"directory_unavailable"`) {
		t.Fatalf("expected 502 directory_unavailable, got %d %s", errRec.Code, errRec.Body.String())
	}

	// Delete credentials -> back to unconfigured
	delRec := req(http.MethodDelete, "/discover/credentials", "")
	if delRec.Code != http.StatusOK ||
		!strings.Contains(delRec.Body.String(), `"configured":false`) ||
		!strings.Contains(delRec.Body.String(), `"provider":"none"`) {
		t.Fatalf("unexpected delete response: %d %s", delRec.Code, delRec.Body.String())
	}
	afterDelSearch := req(http.MethodGet, "/discover/search?q=go", "")
	if afterDelSearch.Code != http.StatusPreconditionFailed {
		t.Fatalf("expected 412 after credential delete, got %d", afterDelSearch.Code)
	}
}
