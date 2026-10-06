package api

import (
	"context"
	"encoding/json"
	"github.com/ajbergh/viib-mediahub/internal/db"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	spotifyanalysis "github.com/ajbergh/viib-mediahub/internal/spotify/analysis"
	"github.com/go-chi/chi/v5"
)

func TestSpotifyArtifactRouteIsBoundedAccountScopedAndReadOnly(t *testing.T) {
	a, _, calls := fixtureCookieRuntime(t)
	runtime := a.spotifyTokens()
	if err := runtime.connect(context.Background(), "fixture-cookie"); err != nil {
		t.Fatal(err)
	}
	o, err := spotifyanalysis.ValidateDomainPayload(cachedSpotifyID, "audio_analysis", []byte(`{"track":{},"beats":[{"start":0,"duration":1,"confidence":0}]}`))
	if err != nil {
		t.Fatal(err)
	}
	o.AccountContext = runtime.metadataContext
	o.RetrievedAt = time.Now().Add(-2 * time.Hour)
	if err = a.db.PutExternalAnalysis(o, "fixture", time.Now().Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	router := chi.NewRouter()
	router.Get("/spotify/analysis/{trackID}/artifact", a.getSpotifyAnalysisArtifact)
	router.Get("/spotify/analysis/{trackID}", a.getSpotifyAnalysisCache)
	router.Get("/spotify/status", a.getSpotifyAnalysisStatus)
	path := "/spotify/analysis/" + cachedSpotifyID + "/artifact?endpoint=audio_analysis&kind=beats"
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"stale":true`) || !strings.Contains(w.Body.String(), `"confidence":0`) {
		t.Fatalf("artifact response: %d %s", w.Code, w.Body.String())
	}

	if err := a.db.BindSpotifyMetadataOwner(runtime.metadataEpoch, "webplayer", "retained-account", runtime.metadataContext); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	if err := a.db.PutExternalAnalysisStatusForRuntime(db.SpotifyMetadataFence{Epoch: runtime.metadataEpoch, ContextKey: runtime.metadataContext}, cachedSpotifyID, "audio_analysis", db.ExternalAnalysisStatus{Code: spotifyanalysis.RateLimited, CheckedAt: now, RetryAt: now.Add(time.Minute)}); err != nil {
		t.Fatal(err)
	}
	owner, err := a.db.ReserveSpotifyMetadataRuntime(runtime.metadataEpoch)
	if err != nil {
		t.Fatal(err)
	}
	runtime.mu.Lock()
	runtime.pendingOwner = owner
	runtime.mu.Unlock()
	beforeOffline := calls.Load()
	offline := httptest.NewRecorder()
	router.ServeHTTP(offline, httptest.NewRequest(http.MethodGet, path, nil))
	if offline.Code != 200 || !strings.Contains(offline.Body.String(), `"unverified":true`) || !strings.Contains(offline.Body.String(), `"readOnly":true`) || calls.Load() != beforeOffline {
		t.Fatalf("offline retained read: %d %s", offline.Code, offline.Body.String())
	}
	status := httptest.NewRecorder()
	router.ServeHTTP(status, httptest.NewRequest(http.MethodGet, "/spotify/status", nil))
	if status.Code != 200 || !strings.Contains(status.Body.String(), `"state":"owner_confirmation_pending"`) || calls.Load() != beforeOffline {
		t.Fatal("pending status", status.Code, status.Body.String())
	}
	cached := httptest.NewRecorder()
	router.ServeHTTP(cached, httptest.NewRequest(http.MethodGet, "/spotify/analysis/"+cachedSpotifyID+"?endpoint=audio_analysis", nil))
	var retained spotifyCacheResponse
	if err := json.Unmarshal(cached.Body.Bytes(), &retained); err != nil {
		t.Fatal(err)
	}
	if cached.Code != 200 || !retained.Unverified || !retained.ReadOnly || retained.Cache == nil || retained.LastFailure == nil || calls.Load() != beforeOffline {
		t.Fatalf("retained cache %d %s", cached.Code, cached.Body.String())
	}
	runtime.mu.Lock()
	runtime.pendingOwner.Provider = "oauth"
	runtime.mu.Unlock()
	denied := httptest.NewRecorder()
	router.ServeHTTP(denied, httptest.NewRequest(http.MethodGet, path, nil))
	if denied.Code != 503 || calls.Load() != beforeOffline {
		t.Fatal("provider mismatch read accepted", denied.Code)
	}
	runtime.mu.Lock()
	runtime.pendingOwner.Provider = "webplayer"
	runtime.mu.Unlock()
	before := calls.Load()
	etag := offline.Header().Get("ETag")
	if etag == w.Header().Get("ETag") {
		t.Fatal("verification transition reused ETag")
	}
	request := httptest.NewRequest(http.MethodGet, path, nil)
	request.Header.Set("If-None-Match", etag)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, request)
	if w.Code != 304 || calls.Load() != before {
		t.Fatal("artifact read fetched provider")
	}
	w = httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path+"evil", nil))
	if w.Code != 400 {
		t.Fatal("unbounded kind accepted")
	}
	if err := runtime.connect(context.Background(), "fixture-cookie"); err != nil {
		t.Fatal(err)
	}
	w = httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
	if w.Code != 404 {
		t.Fatal("account switch exposed old artifact")
	}
}
