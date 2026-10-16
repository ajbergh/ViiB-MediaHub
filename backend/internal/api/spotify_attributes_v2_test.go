package api

import (
	"context"
	"github.com/ajbergh/viib-mediahub/internal/analysis"
	"github.com/ajbergh/viib-mediahub/internal/db"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	spotifyanalysis "github.com/ajbergh/viib-mediahub/internal/spotify/analysis"
	spotifyrefresh "github.com/ajbergh/viib-mediahub/internal/spotify/refresh"
)

type v2APIProvider struct{ *spotifyRefreshFixtureProvider }

func (p v2APIProvider) FetchAudioAttributesV2(ctx context.Context, id string) (spotifyanalysis.Observation, error) {
	return p.fn(ctx, id, spotifyanalysis.AttributesV2)
}

func TestV2ProductionPreparationAndDownloadTags(t *testing.T) {
	a, _, _ := fixtureCookieRuntime(t)
	runtime := a.spotifyTokens()
	if err := runtime.connect(t.Context(), "fixture-cookie"); err != nil {
		t.Fatal(err)
	}
	runtime.mu.Lock()
	runtime.ownerVerifier = func(context.Context) error { return nil }
	runtime.mu.Unlock()
	provider := v2APIProvider{&spotifyRefreshFixtureProvider{fn: func(ctx context.Context, id, endpoint string) (spotifyanalysis.Observation, error) {
		bpm := 137.98
		tonic, mode, camelot := 8, 1, "4B"
		if endpoint == spotifyanalysis.AttributesV2 {
			bpm = 70.4827
			tonic = 3
			camelot = "5B"
		}
		return spotifyanalysis.Observation{TrackID: id, Source: "spotify_internal", SourceEndpoint: endpoint, AccountContext: runtime.metadataContext, RetrievedAt: time.Now().UTC(), BPM: &bpm, Key: &tonic, Mode: &mode, Camelot: &camelot}, nil
	}}}
	service, err := spotifyrefresh.New(a.db, provider, spotifyrefresh.Options{Enabled: true, AdapterRevision: "extension222"})
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	a.spotifyAnalysisMu.Lock()
	previous := a.spotifyAnalysis
	a.spotifyAnalysis = service
	a.spotifyAnalysisMu.Unlock()
	if previous != nil {
		previous.Close()
	}
	a.spotifyHTTPClient = &http.Client{Transport: sessionTransport(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 404, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("{}"))}, nil
	})}
	path := filepath.Join(t.TempDir(), "song.wav")
	if err := os.WriteFile(path, []byte("source bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := a.db.SaveSong(&db.Song{ID: "v2-song", Title: "The First Snowfall", Artist: "Bing Crosby", FilePath: path, AddedAt: 1}); err != nil {
		t.Fatal(err)
	}
	source, err := analysis.ResolveLocalSource(a.db, "v2-song")
	if err != nil {
		t.Fatal(err)
	}
	if err := a.db.RefreshTrackAnalysisSourceRevision("v2-song", source.Fingerprint); err != nil {
		t.Fatal(err)
	}
	if _, err := a.db.ConfirmSpotifyRecording("v2-song", cachedSpotifyID, source.Fingerprint, true); err != nil {
		t.Fatal(err)
	}
	for _, o := range []*spotifyanalysis.Observation{a.spotifyDownloadFeatures(t.Context(), cachedSpotifyID), a.spotifyPreparationForSource(t.Context(), source)} {
		if o == nil || o.BPM == nil || *o.BPM != 70.4827 || o.Camelot == nil || *o.Camelot != "5B" || o.BPMOrigin == nil || o.BPMOrigin.SourceEndpoint != spotifyanalysis.AttributesV2 || o.KeyOrigin == nil || o.KeyOrigin.SourceEndpoint != spotifyanalysis.AttributesV2 {
			t.Fatalf("production V2 lost: %+v", o)
		}
	}
	cache, err := a.db.GetExternalAnalysis(cachedSpotifyID, spotifyanalysis.AttributesV2)
	if err != nil || cache == nil || *cache.Observation.BPM != 70.4827 {
		t.Fatalf("V2 not persisted: %+v %v", cache, err)
	}
}

func TestSpotifyAudioAttributesV2API(t *testing.T) {
	a, _ := newBPMRouteTestAPI(t, true)
	if err := a.db.ActivateSpotifyMetadataContext("v2-api"); err != nil {
		t.Fatal(err)
	}
	provider := v2APIProvider{&spotifyRefreshFixtureProvider{fn: func(ctx context.Context, id, endpoint string) (spotifyanalysis.Observation, error) {
		bpm := 70.4827
		tonic, mode, camelot := 3, 1, "5B"
		return spotifyanalysis.Observation{TrackID: id, Source: "spotify_internal", SourceEndpoint: endpoint, AccountContext: "v2-api", RetrievedAt: time.Now().UTC(), BPM: &bpm, Key: &tonic, Mode: &mode, Camelot: &camelot}, nil
	}}}
	service, err := spotifyrefresh.New(a.db, provider, spotifyrefresh.Options{Enabled: true, AdapterRevision: "extension222"})
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	if err := a.InstallSpotifyAnalysisService(service); err != nil {
		t.Fatal(err)
	}
	router := a.Routes()
	for _, request := range []struct{ method, path string }{{http.MethodPost, "/spotify/analysis/" + cachedSpotifyID + "/refresh?endpoint=audio_attributes_v2"}, {http.MethodGet, "/spotify/analysis/" + cachedSpotifyID + "?endpoint=audio_attributes_v2"}} {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(request.method, request.path, nil))
		if w.Code != 200 || !strings.Contains(w.Body.String(), "70.4827") || !strings.Contains(w.Body.String(), "5B") || !strings.Contains(w.Body.String(), "audio_attributes_v2") {
			t.Fatalf("V2 API: %d %s", w.Code, w.Body.String())
		}
	}
}
