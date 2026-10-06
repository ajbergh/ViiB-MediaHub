package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/db"
	"github.com/ajbergh/viib-mediahub/internal/logger"
	spotifyanalysis "github.com/ajbergh/viib-mediahub/internal/spotify/analysis"
	spotifyrefresh "github.com/ajbergh/viib-mediahub/internal/spotify/refresh"
)

func TestDownloadBundlePartialResourcesAndWarmReuse(t *testing.T) {
	for _, missing := range []string{"audio_analysis", "audio_features"} {
		t.Run(missing, func(t *testing.T) {
			logDir := t.TempDir()
			if err := logger.Init(logDir); err != nil {
				t.Fatal(err)
			}
			defer logger.Close()
			a, _, _ := fixtureCookieRuntime(t)
			runtime := a.spotifyTokens()
			if err := runtime.connect(context.Background(), "fixture-cookie"); err != nil {
				t.Fatal(err)
			}
			runtime.mu.Lock()
			runtime.ownerVerifier = func(context.Context) error { return nil }
			runtime.mu.Unlock()
			provider := &spotifyRefreshFixtureProvider{fn: func(_ context.Context, id, endpoint string) (spotifyanalysis.Observation, error) {
				if endpoint == missing {
					return spotifyanalysis.Observation{}, &spotifyanalysis.Error{Code: spotifyanalysis.NotFound, HTTPStatus: 404}
				}
				if endpoint == "audio_analysis" {
					o, err := spotifyanalysis.ValidateDomainPayload(id, endpoint, []byte(`{"track":{},"beats":[{"start":0,"duration":1,"confidence":0}]}`))
					o.AccountContext = runtime.metadataContext
					o.RetrievedAt = time.Now()
					return o, err
				}
				bpm := 120.0
				return spotifyanalysis.Observation{TrackID: id, AccountContext: runtime.metadataContext, Source: "spotify_internal", SourceEndpoint: endpoint, RetrievedAt: time.Now(), BPM: &bpm}, nil
			}}
			service, err := spotifyrefresh.New(a.db, provider, spotifyrefresh.Options{Enabled: true, AdapterRevision: "fixture"})
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
			var waveformCalls atomic.Int32
			a.spotifyHTTPClient = &http.Client{Transport: sessionTransport(func(r *http.Request) (*http.Response, error) {
				if r.URL.Host != "spclient.wg.spotify.com" {
					return &http.Response{StatusCode: 404, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"error":"catalog unavailable"}`))}, nil
				}
				waveformCalls.Add(1)
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(string(waveformWireFixture(200))))}, nil
			})}
			features := a.spotifyDownloadFeatures(t.Context(), cachedSpotifyID)
			if missing == "audio_analysis" && (features == nil || features.BPM == nil || *features.BPM != 120) {
				t.Fatalf("features lost: %+v", features)
			}
			if missing == "audio_features" {
				if features != nil {
					t.Fatal("failed features supplied tags")
				}
				detailed, err := a.db.GetSpotifyAudioArtifact(cachedSpotifyID, "audio_analysis", "beats", runtime.metadataContext)
				if err != nil || detailed == nil {
					t.Fatalf("detailed sibling lost: %v %v", detailed, err)
				}
			}
			artifact, err := a.db.GetSpotifyAudioArtifact(cachedSpotifyID, "three_band_waveform", "spotify_three_band", runtime.metadataContext)
			if err != nil || artifact == nil {
				status, statusErr := a.db.GetSpotifyMetadataResourceStatus(db.SpotifySnapshotKey{EntityType: "track", SpotifyID: cachedSpotifyID, Resource: "three_band_waveform", ContextKey: runtime.metadataContext})
				logData, _ := os.ReadFile(filepath.Join(logDir, "scan.log"))
				t.Fatalf("waveform lost: artifact=%v err=%v requests=%d status=%+v statusErr=%v diagnostics=%s", artifact, err, waveformCalls.Load(), status, statusErr, logData)
			}
			if provider.calls.Load() != 2 {
				t.Fatalf("expected independent feature/detailed dispatch: %d", provider.calls.Load())
			}
			a.spotifyDownloadFeatures(t.Context(), cachedSpotifyID)
			if provider.calls.Load() != 2 || waveformCalls.Load() != 1 {
				t.Fatalf("warm cache redispatch: scalar=%d waveform=%d", provider.calls.Load(), waveformCalls.Load())
			}
		})
	}
}

func TestDownloadBundleCapturesSanitizedCatalog(t *testing.T) {
	database, err := db.New(filepath.Join(t.TempDir(), "download.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	credentials, _ := json.Marshal(SpotifyCredentials{ClientId: "client", AccessToken: "fixture", RefreshToken: "refresh", Expiry: time.Now().Add(time.Hour).UnixMilli()})
	if err = database.SetSetting("spotify_credentials", string(credentials)); err != nil {
		t.Fatal(err)
	}
	var catalogCalls atomic.Int32
	raw := `{"type":"track","id":"` + cachedSpotifyID + `","uri":"spotify:track:` + cachedSpotifyID + `","popularity":0,"future_domain":false,"token":"secret"}`
	a := &API{db: database, spotifyHTTPClient: &http.Client{Transport: sessionTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/v1/tracks/"+cachedSpotifyID {
			catalogCalls.Add(1)
			return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(raw))}, nil
		}
		return &http.Response{StatusCode: 404, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{}`))}, nil
	})}}
	runtime := a.spotifyTokens()
	defer runtime.close()
	provider := &spotifyRefreshFixtureProvider{fn: func(_ context.Context, id, endpoint string) (spotifyanalysis.Observation, error) {
		return spotifyanalysis.Observation{}, &spotifyanalysis.Error{Code: spotifyanalysis.NotFound, HTTPStatus: 404}
	}}
	service, err := spotifyrefresh.New(database, provider, spotifyrefresh.Options{Enabled: true, AdapterRevision: "fixture"})
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	a.spotifyAnalysisMu.Lock()
	a.spotifyAnalysis = service
	a.spotifyAnalysisMu.Unlock()
	a.spotifyDownloadFeatures(t.Context(), cachedSpotifyID)
	snapshot, err := database.GetSpotifyEntitySnapshot(db.SpotifySnapshotKey{EntityType: "track", SpotifyID: cachedSpotifyID, Resource: "rest:/v1/tracks/" + cachedSpotifyID + ":page::", ContextKey: runtime.metadataContext})
	if err != nil || snapshot == nil {
		t.Fatalf("catalog absent: %v %v", snapshot, err)
	}
	if !strings.Contains(string(snapshot.Payload), `"popularity":0`) || !strings.Contains(string(snapshot.Payload), `"future_domain":false`) || strings.Contains(string(snapshot.Payload), "secret") {
		t.Fatalf("unsafe/lossy catalog: %s", snapshot.Payload)
	}
	a.spotifyDownloadFeatures(t.Context(), cachedSpotifyID)
	if catalogCalls.Load() != 1 {
		t.Fatalf("catalog calls %d", catalogCalls.Load())
	}
}
