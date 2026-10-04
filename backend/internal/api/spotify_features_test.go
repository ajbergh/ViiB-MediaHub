package api

import (
	"context"
	"github.com/ajbergh/viib-mediahub/internal/analysis"
	spotifyanalysis "github.com/ajbergh/viib-mediahub/internal/spotify/analysis"
	spotifyrefresh "github.com/ajbergh/viib-mediahub/internal/spotify/refresh"
	"os"
	"testing"
	"time"
)

func TestAutomaticSpotifyFeaturesRequireCurrentRecordingAndReuseCache(t *testing.T) {
	a, path := newBPMRouteTestAPI(t, false)
	source, err := analysis.ResolveLocalSource(a.db, "song")
	if err != nil {
		t.Fatal(err)
	}
	if a.spotifyFeaturesForSource(t.Context(), source) != nil {
		t.Fatal("disabled session supplied data")
	}
	provider := &spotifyRefreshFixtureProvider{fn: func(_ context.Context, id, endpoint string) (spotifyanalysis.Observation, error) {
		if id != cachedSpotifyID || endpoint != "audio_features" {
			t.Fatalf("unexpected lookup %s %s", id, endpoint)
		}
		bpm := 109.724
		return spotifyanalysis.Observation{TrackID: id, Source: "spotify_internal", SourceEndpoint: endpoint, RetrievedAt: time.Now().UTC(), BPM: &bpm}, nil
	}}
	service, err := spotifyrefresh.New(a.db, provider, spotifyrefresh.Options{Enabled: true, AdapterRevision: "fixture"})
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	if err := a.InstallSpotifyAnalysisService(service); err != nil {
		t.Fatal(err)
	}
	if a.spotifyFeaturesForSource(t.Context(), source) != nil || provider.calls.Load() != 0 {
		t.Fatal("unlinked recording fetched")
	}
	if err := a.db.RefreshTrackAnalysisSourceRevision("song", source.Fingerprint); err != nil {
		t.Fatal(err)
	}
	if changed, err := a.db.ConfirmSpotifyRecording("song", cachedSpotifyID, source.Fingerprint, true); !changed || err != nil {
		t.Fatalf("confirm: %v %v", changed, err)
	}
	for range 2 {
		if o := a.spotifyFeaturesForSource(t.Context(), source); o == nil || o.BPM == nil || *o.BPM != 109.724 {
			t.Fatalf("lookup: %+v", o)
		}
	}
	if provider.calls.Load() != 1 {
		t.Fatal("cache not reused")
	}
	if err := os.WriteFile(path, []byte("replacement audio with a different size"), 0600); err != nil {
		t.Fatal(err)
	}
	source, err = analysis.ResolveLocalSource(a.db, "song")
	if err != nil {
		t.Fatal(err)
	}
	if a.spotifyFeaturesForSource(t.Context(), source) != nil || provider.calls.Load() != 1 {
		t.Fatal("stale identity used")
	}
}
