package api

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/ajbergh/viib-mediahub/internal/db"
	spotifyanalysis "github.com/ajbergh/viib-mediahub/internal/spotify/analysis"
	spotifyauth "github.com/ajbergh/viib-mediahub/internal/spotify/auth"
	spotifyrefresh "github.com/ajbergh/viib-mediahub/internal/spotify/refresh"
	"github.com/go-chi/chi/v5"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const cachedSpotifyID = "5r9W9MJLvHk83fcZSPQ8SE"

func TestSpotifyCacheRoutesReadStoredDataOnly(t *testing.T) {
	a, _ := newBPMRouteTestAPI(t, true)
	// No token manager or HTTP client exists on this API fixture.
	router := chi.NewRouter()
	router.Get("/spotify/analysis/status", a.getSpotifyAnalysisStatus)
	router.Get("/spotify/analysis/{trackID}", a.getSpotifyAnalysisCache)
	request := func(path string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		return w
	}
	if w := request("/spotify/analysis/status"); w.Code != 200 || !strings.Contains(w.Body.String(), `"state":"disabled"`) {
		t.Fatalf("status: %s", w.Body.String())
	}
	if w := request("/spotify/analysis/" + cachedSpotifyID); w.Code != 200 || !strings.Contains(w.Body.String(), `"state":"not_cached"`) {
		t.Fatal(w.Body.String())
	}
	now := time.Now().UTC()
	bpm := 108.022
	o := spotifyanalysis.Observation{TrackID: cachedSpotifyID, Source: "spotify_internal", SourceEndpoint: "audio_features", RetrievedAt: now.Add(-2 * time.Hour), BPM: &bpm}
	if err := a.db.PutExternalAnalysis(o, "fixture", now.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	failure := db.ExternalAnalysisStatus{Code: spotifyanalysis.NotFound, CheckedAt: now, RetryAt: now.Add(time.Hour)}
	if err := a.db.PutExternalAnalysisStatus(cachedSpotifyID, "audio_features", failure); err != nil {
		t.Fatal(err)
	}
	w := request("/spotify/analysis/" + cachedSpotifyID)
	var result spotifyCacheResponse
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if w.Code != 200 || !result.Stale || result.Cache == nil || result.LastFailure == nil ||
		result.State != "available" || result.AgeSeconds == nil || *result.AgeSeconds < 7200 {
		t.Fatalf("cache: %s", w.Body.String())
	}
	w = request("/spotify/analysis/" + cachedSpotifyID + "?endpoint=audio_analysis")
	if !strings.Contains(w.Body.String(), `"state":"not_cached"`) {
		t.Fatal("implicit endpoint fallback")
	}
	for _, path := range []string{"/spotify/analysis/invalid", "/spotify/analysis/" + cachedSpotifyID + "?endpoint=proxy"} {
		if w = request(path); w.Code != 400 {
			t.Fatalf("invalid path: %d", w.Code)
		}
	}
}

func TestSpotifyRecordingRoutesRequireCurrentExplicitConfirmation(t *testing.T) {
	a, mediaPath := newBPMRouteTestAPI(t, true)
	router := a.V2Routes()
	fingerprint := getBPMSourceFingerprint(t, router)
	call := func(method, body string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(method, "/analysis/song/external/spotify", strings.NewReader(body)))
		return w
	}
	body := func(confirmed bool, fp string) string {
		encoded, _ := json.Marshal(map[string]interface{}{"trackId": cachedSpotifyID, "confirmed": confirmed, "sourceFingerprint": fp})
		return string(encoded)
	}
	if w := call(http.MethodPut, body(false, fingerprint)); w.Code != 400 {
		t.Fatalf("unconfirmed=%d", w.Code)
	}
	if w := call(http.MethodPut, body(true, "stale")); w.Code != 409 {
		t.Fatalf("stale=%d", w.Code)
	}
	if w := call(http.MethodPut, body(true, fingerprint)); w.Code != 200 {
		t.Fatalf("confirmed=%d %s", w.Code, w.Body.String())
	}
	if w := call(http.MethodGet, ""); !strings.Contains(w.Body.String(), cachedSpotifyID) {
		t.Fatal("confirmed link missing")
	}
	if err := os.WriteFile(mediaPath, []byte("changed recording with different length"), 0600); err != nil {
		t.Fatal(err)
	}
	if w := call(http.MethodGet, ""); !strings.Contains(w.Body.String(), `"link":null`) {
		t.Fatalf("stale link served: %s", w.Body.String())
	}
	if w := call(http.MethodPut, body(true, fingerprint)); w.Code != 409 {
		t.Fatal("changed audio accepted old confirmation")
	}
	if w := call(http.MethodDelete, ""); w.Code != 204 {
		t.Fatalf("delete=%d", w.Code)
	}
}

type spotifyRefreshFixtureProvider struct {
	calls atomic.Int32
	fn    func(context.Context, string, string) (spotifyanalysis.Observation, error)
}

func (p *spotifyRefreshFixtureProvider) Fetch(ctx context.Context, id string) (spotifyanalysis.Observation, error) {
	p.calls.Add(1)
	return p.fn(ctx, id, "audio_analysis")
}
func (p *spotifyRefreshFixtureProvider) FetchFeatures(ctx context.Context, id string) (spotifyanalysis.Observation, error) {
	p.calls.Add(1)
	return p.fn(ctx, id, "audio_features")
}

func TestSpotifyRefreshAPIIsExplicitAndDisconnectPurgesCache(t *testing.T) {
	a, _ := newBPMRouteTestAPI(t, true)
	router := a.Routes()
	call := func(method, path string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(method, path, nil))
		return w
	}
	path := "/spotify/analysis/" + cachedSpotifyID + "/refresh"
	if w := call(http.MethodPost, path); w.Code != 503 || !strings.Contains(w.Body.String(), "disabled") {
		t.Fatalf("default refresh: %d %s", w.Code, w.Body.String())
	}
	var disconnects atomic.Int32
	provider := &spotifyRefreshFixtureProvider{fn: func(ctx context.Context, id, endpoint string) (spotifyanalysis.Observation, error) {
		bpm := 108.022
		return spotifyanalysis.Observation{TrackID: id, SourceEndpoint: endpoint, Source: "spotify_internal", RetrievedAt: time.Now().UTC(), BPM: &bpm}, nil
	}}
	service, err := spotifyrefresh.New(a.db, provider, spotifyrefresh.Options{Enabled: true, AdapterRevision: "fixture-api", Disconnect: func() { disconnects.Add(1) }})
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	// Installation must discard a previous account's successful/failure cache.
	old := spotifyanalysis.Observation{TrackID: cachedSpotifyID, SourceEndpoint: "audio_analysis", Source: "spotify_internal", RetrievedAt: time.Now().Add(-time.Hour)}
	bpm := 99.0
	old.BPM = &bpm
	if err = a.db.PutExternalAnalysis(old, "prior-account", time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err = a.InstallSpotifyAnalysisService(service); err != nil {
		t.Fatal(err)
	}
	if oldCache, err := a.db.GetExternalAnalysis(cachedSpotifyID, "audio_analysis"); err != nil || oldCache != nil {
		t.Fatal("old account cache retained")
	}
	if provider.calls.Load() != 0 {
		t.Fatal("installation fetched data")
	}
	if w := call(http.MethodPost, path); w.Code != 200 || !strings.Contains(w.Body.String(), "audio_features") {
		t.Fatalf("refresh: %d %s", w.Code, w.Body.String())
	}
	if w := call(http.MethodPost, path); w.Code != 200 || provider.calls.Load() != 1 {
		t.Fatal("fresh cache repeated provider request")
	}
	if w := call(http.MethodGet, "/spotify/analysis/status"); w.Code != 200 || !strings.Contains(w.Body.String(), `"connected":true`) {
		t.Fatal(w.Body.String())
	}
	if w := call(http.MethodDelete, "/spotify/analysis/session"); w.Code != 204 {
		t.Fatalf("disconnect: %d %s", w.Code, w.Body.String())
	}
	if disconnects.Load() != 1 {
		t.Fatal("provider session not cleared")
	}
	if cache, err := a.db.GetExternalAnalysis(cachedSpotifyID, "audio_features"); err != nil || cache != nil {
		t.Fatal("disconnect retained account cache")
	}
	if w := call(http.MethodPost, path); w.Code != 503 || provider.calls.Load() != 1 {
		t.Fatal("disconnect allowed retrieval")
	}
}

func TestSpotifyRefreshAPIRateLimitAndSafeErrors(t *testing.T) {
	for _, kind := range []string{"rate", "arbitrary"} {
		t.Run(kind, func(t *testing.T) {
			a, _ := newBPMRouteTestAPI(t, true)
			provider := &spotifyRefreshFixtureProvider{fn: func(ctx context.Context, id, endpoint string) (spotifyanalysis.Observation, error) {
				if kind == "rate" {
					return spotifyanalysis.Observation{}, &spotifyanalysis.Error{Code: spotifyanalysis.RateLimited, RetryAfter: 2 * time.Hour}
				}
				return spotifyanalysis.Observation{}, errors.New("upstream cookie token secret")
			}}
			service, err := spotifyrefresh.New(a.db, provider, spotifyrefresh.Options{Enabled: true, AdapterRevision: "fixture"})
			if err != nil {
				t.Fatal(err)
			}
			defer service.Close()
			if err = a.InstallSpotifyAnalysisService(service); err != nil {
				t.Fatal(err)
			}
			router := a.Routes()
			w := httptest.NewRecorder()
			router.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/spotify/analysis/"+cachedSpotifyID+"/refresh", nil))
			expected := 503
			if kind == "rate" {
				expected = 429
			}
			if w.Code != expected || strings.Contains(w.Body.String(), "secret") || strings.Contains(w.Body.String(), "cookie") {
				t.Fatalf("unsafe response %d %s", w.Code, w.Body.String())
			}
			if kind == "rate" && w.Header().Get("Retry-After") != "7200" {
				t.Fatalf("retry-after %s", w.Header().Get("Retry-After"))
			}
		})
	}
}
func TestSpotifyRefreshAPIShutdownRejectsNewInstallation(t *testing.T) {
	a, _ := newBPMRouteTestAPI(t, true)
	provider := &spotifyRefreshFixtureProvider{fn: func(ctx context.Context, id, endpoint string) (spotifyanalysis.Observation, error) {
		return spotifyanalysis.Observation{}, errors.New("unused")
	}}
	var disconnects atomic.Int32
	service, err := spotifyrefresh.New(a.db, provider, spotifyrefresh.Options{Enabled: true, AdapterRevision: "fixture", Disconnect: func() { disconnects.Add(1) }})
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	if err = a.InstallSpotifyAnalysisService(service); err != nil {
		t.Fatal(err)
	}
	a.Close()
	if disconnects.Load() != 1 || service.Status().Configured {
		t.Fatal("shutdown did not retire session")
	}
	if err = a.InstallSpotifyAnalysisService(service); err == nil {
		t.Fatal("closed API accepted new service")
	}
}

func TestProductionReferenceCompositionFollowsCookieLifetime(t *testing.T) {
	a, _, calls := fixtureCookieRuntime(t)
	runtime := a.spotifyTokens()
	a.initSpotifyAnalysis()
	if a.spotifyAnalysis != nil {
		t.Fatal("disconnected runtime enabled reference")
	}
	if err := runtime.connect(context.Background(), "fixture-cookie"); err != nil {
		t.Fatal(err)
	}
	original := a.spotifyAnalysis
	if original == nil || !original.Status().Configured || calls.Load() != 1 {
		t.Fatal("connection did not install inert service")
	}
	now := time.Now().UTC()
	bpm := 108.022
	observation := spotifyanalysis.Observation{TrackID: cachedSpotifyID, Source: "spotify_internal", SourceEndpoint: "audio_features", RetrievedAt: now, BPM: &bpm}
	if err := a.db.PutExternalAnalysis(observation, "fixture", now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	restoredRuntime := newSpotifyAuthRuntime(a.db, spotifyauth.WebPlayerOptions{})
	restored := &API{db: a.db, spotifyAuth: restoredRuntime}
	restored.initSpotifyAnalysis()
	if restored.spotifyAnalysis == nil || calls.Load() != 1 {
		t.Fatal("restore performed auth I/O or did not compose service")
	}
	if cache, err := a.db.GetExternalAnalysis(cachedSpotifyID, "audio_features"); err != nil || cache == nil {
		t.Fatal("same-account startup purged reference")
	}
	restored.spotifyAnalysis.Close()
	restoredRuntime.close()
	if err := runtime.connect(context.Background(), "fixture-cookie"); err != nil {
		t.Fatal(err)
	}
	if a.spotifyAnalysis == original || a.spotifyAnalysis == nil || original.Status().Configured {
		t.Fatal("reconnect reused retired reference service")
	}
	if cache, err := a.db.GetExternalAnalysis(cachedSpotifyID, "audio_features"); err != nil || cache != nil {
		t.Fatal("account replacement kept old reference")
	}
	if err := runtime.disconnect(); err != nil {
		t.Fatal(err)
	}
	if a.spotifyAnalysis != nil {
		t.Fatal("logout retained reference service")
	}
	a.spotifyAnalysisClosed = true
	if err := runtime.connect(context.Background(), "fixture-cookie"); err != nil {
		t.Fatal(err)
	}
	if a.spotifyAnalysis != nil {
		t.Fatal("closed API reinstalled reference")
	}
}
