// Tests and fixtures for spotify features behavior.

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
		return spotifyanalysis.Observation{AccountContext: "api-fixture", TrackID: id, Source: "spotify_internal", SourceEndpoint: endpoint, RetrievedAt: time.Now().UTC(), BPM: &bpm}, nil
	}}
	if err := a.db.ActivateSpotifyMetadataContext("api-fixture"); err != nil {
		t.Fatal(err)
	}
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

func TestPreparationDetailedFailureKeepsFeatures(t *testing.T) {
	a, _ := newBPMRouteTestAPI(t, false)
	_ = a.spotifyTokens() // Initialize the session owner before fixture context.
	source, err := analysis.ResolveLocalSource(a.db, "song")
	if err != nil {
		t.Fatal(err)
	}
	if err := a.db.ActivateSpotifyMetadataContext("api-fixture"); err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	provider := &spotifyRefreshFixtureProvider{fn: func(ctx context.Context, id, endpoint string) (spotifyanalysis.Observation, error) {
		if endpoint == spotifyrefresh.Detailed {
			close(started)
			return spotifyanalysis.Observation{}, &spotifyanalysis.Error{Code: spotifyanalysis.NotFound}
		}
		select {
		case <-started:
		case <-ctx.Done():
			return spotifyanalysis.Observation{}, ctx.Err()
		}
		bpm := 111.0
		return spotifyanalysis.Observation{AccountContext: "api-fixture", TrackID: id, Source: "spotify_internal", SourceEndpoint: endpoint, RetrievedAt: time.Now().UTC(), BPM: &bpm}, nil
	}}
	service, err := spotifyrefresh.New(a.db, provider, spotifyrefresh.Options{Enabled: true, AdapterRevision: "fixture"})
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	if err := a.InstallSpotifyAnalysisService(service); err != nil {
		t.Fatal(err)
	}
	if err := a.db.RefreshTrackAnalysisSourceRevision("song", source.Fingerprint); err != nil {
		t.Fatal(err)
	}
	if _, err := a.db.ConfirmSpotifyRecording("song", cachedSpotifyID, source.Fingerprint, true); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	result := a.spotifyPreparationForSource(ctx, source)
	if result == nil || result.BPM == nil || *result.BPM != 111 || ctx.Err() != nil {
		t.Fatalf("independent feature result: %+v %v", result, ctx.Err())
	}
	status, err := a.db.GetExternalAnalysisStatus(cachedSpotifyID, spotifyrefresh.Detailed)
	if err != nil || status == nil || status.Code != spotifyanalysis.NotFound {
		t.Fatalf("detailed failure: %+v %v", status, err)
	}
}

func TestBackgroundPreparationValidatesOwnerBeforeServiceLookup(t *testing.T) {
	for _, bundle := range []bool{false, true} {
		t.Run(map[bool]string{false: "features", true: "bundle"}[bundle], func(t *testing.T) {
			a, _ := newBPMRouteTestAPI(t, false)
			runtime := a.spotifyTokens()
			source, err := analysis.ResolveLocalSource(a.db, "song")
			if err != nil {
				t.Fatal(err)
			}
			if err := a.db.RefreshTrackAnalysisSourceRevision("song", source.Fingerprint); err != nil {
				t.Fatal(err)
			}
			if _, err := a.db.ConfirmSpotifyRecording("song", cachedSpotifyID, source.Fingerprint, true); err != nil {
				t.Fatal(err)
			}
			if err := a.db.BindSpotifyMetadataOwner(runtime.metadataEpoch, "oauth", "account", runtime.metadataContext); err != nil {
				t.Fatal(err)
			}
			owner, err := a.db.ReserveSpotifyMetadataRuntime(runtime.metadataEpoch)
			if err != nil {
				t.Fatal(err)
			}
			runtime.pendingOwner = owner
			provider := &spotifyRefreshFixtureProvider{fn: func(ctx context.Context, id, endpoint string) (spotifyanalysis.Observation, error) {
				if endpoint == spotifyrefresh.Detailed {
					return spotifyanalysis.Observation{}, &spotifyanalysis.Error{Code: spotifyanalysis.NotFound}
				}
				bpm := 119.0
				return spotifyanalysis.Observation{AccountContext: runtime.metadataContext, TrackID: id, Source: "spotify_internal", SourceEndpoint: endpoint, RetrievedAt: time.Now(), BPM: &bpm}, nil
			}}
			service, err := spotifyrefresh.New(a.db, provider, spotifyrefresh.Options{Enabled: true, AdapterRevision: "fixture"})
			if err != nil {
				t.Fatal(err)
			}
			defer service.Close()
			verifications := 0
			runtime.ownerVerifier = func(ctx context.Context) error {
				verifications++
				if err := runtime.confirmProfileOwner(ctx, "account"); err != nil {
					return err
				}
				return a.InstallSpotifyAnalysisService(service)
			}
			var result *spotifyanalysis.Observation
			if bundle {
				result = a.spotifyPreparationForSource(t.Context(), source)
			} else {
				result = a.spotifyTrackFeatures(t.Context(), cachedSpotifyID)
			}
			if result == nil || result.BPM == nil || *result.BPM != 119 || verifications != 1 {
				t.Fatal("preparation skipped pending validation", result, verifications)
			}
		})
	}
}

func TestPreparationFeaturesUnavailableUsesDetailedCache(t *testing.T) {
	a, _ := newBPMRouteTestAPI(t, false)
	_ = a.spotifyTokens()
	source, err := analysis.ResolveLocalSource(a.db, "song")
	if err != nil {
		t.Fatal(err)
	}
	if err := a.db.ActivateSpotifyMetadataContext("api-fixture"); err != nil {
		t.Fatal(err)
	}
	provider := &spotifyRefreshFixtureProvider{fn: func(ctx context.Context, id, endpoint string) (spotifyanalysis.Observation, error) {
		if endpoint == spotifyrefresh.Features {
			return spotifyanalysis.Observation{}, &spotifyanalysis.Error{Code: spotifyanalysis.NotFound}
		}
		bpm, confidence := 117.0, .75
		return spotifyanalysis.Observation{AccountContext: "api-fixture", TrackID: id, Source: "spotify_internal", SourceEndpoint: endpoint, RetrievedAt: time.Now().UTC(), BPM: &bpm, BPMConfidence: &confidence}, nil
	}}
	service, err := spotifyrefresh.New(a.db, provider, spotifyrefresh.Options{Enabled: true, AdapterRevision: "fixture"})
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	if err := a.InstallSpotifyAnalysisService(service); err != nil {
		t.Fatal(err)
	}
	if err := a.db.RefreshTrackAnalysisSourceRevision("song", source.Fingerprint); err != nil {
		t.Fatal(err)
	}
	if _, err := a.db.ConfirmSpotifyRecording("song", cachedSpotifyID, source.Fingerprint, true); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	result := a.spotifyPreparationForSource(ctx, source)
	if result == nil || result.BPM == nil || *result.BPM != 117 || result.SourceEndpoint != spotifyrefresh.Detailed || result.BPMConfidence == nil || *result.BPMConfidence != .75 {
		t.Fatalf("detailed fallback: %+v", result)
	}
	featureCache, err := a.db.GetExternalAnalysis(cachedSpotifyID, spotifyrefresh.Features)
	if err != nil || featureCache != nil {
		t.Fatalf("fabricated features cache: %+v %v", featureCache, err)
	}
	detailedCache, err := a.db.GetExternalAnalysis(cachedSpotifyID, spotifyrefresh.Detailed)
	if err != nil || detailedCache == nil || detailedCache.Observation.SourceEndpoint != spotifyrefresh.Detailed {
		t.Fatalf("original detailed cache: %+v %v", detailedCache, err)
	}
}

func TestPreparationRejectsSourceReplacedDuringProviderWork(t *testing.T) {
	a, path := newBPMRouteTestAPI(t, false)
	_ = a.spotifyTokens()
	source, err := analysis.ResolveLocalSource(a.db, "song")
	if err != nil {
		t.Fatal(err)
	}
	if err := a.db.ActivateSpotifyMetadataContext("api-fixture"); err != nil {
		t.Fatal(err)
	}
	started, release := make(chan struct{}), make(chan struct{})
	provider := &spotifyRefreshFixtureProvider{fn: func(ctx context.Context, id, endpoint string) (spotifyanalysis.Observation, error) {
		if endpoint == spotifyrefresh.Features {
			close(started)
			select {
			case <-release:
			case <-ctx.Done():
				return spotifyanalysis.Observation{}, ctx.Err()
			}
		}
		bpm := 118.0
		return spotifyanalysis.Observation{AccountContext: "api-fixture", TrackID: id, Source: "spotify_internal", SourceEndpoint: endpoint, RetrievedAt: time.Now().UTC(), BPM: &bpm}, nil
	}}
	service, err := spotifyrefresh.New(a.db, provider, spotifyrefresh.Options{Enabled: true, AdapterRevision: "fixture"})
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	if err := a.InstallSpotifyAnalysisService(service); err != nil {
		t.Fatal(err)
	}
	if err := a.db.RefreshTrackAnalysisSourceRevision("song", source.Fingerprint); err != nil {
		t.Fatal(err)
	}
	if _, err := a.db.ConfirmSpotifyRecording("song", cachedSpotifyID, source.Fingerprint, true); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	result := make(chan *spotifyanalysis.Observation, 1)
	go func() { result <- a.spotifyPreparationForSource(ctx, source) }()
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	writeErr := os.WriteFile(path, []byte("replacement source with changed byte length"), 0600)
	close(release)
	if writeErr != nil {
		t.Fatal(writeErr)
	}
	select {
	case got := <-result:
		if got != nil {
			t.Fatalf("stale source accepted: %+v", got)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	cache, err := a.db.GetExternalAnalysis(cachedSpotifyID, spotifyrefresh.Features)
	if err != nil || cache == nil {
		t.Fatalf("recording-scoped cache should survive source change: %+v %v", cache, err)
	}
}

func TestPreparationAccountRetirementDiscardsBundle(t *testing.T) {
	a, _ := newBPMRouteTestAPI(t, false)
	runtime := a.spotifyTokens()
	source, err := analysis.ResolveLocalSource(a.db, "song")
	if err != nil {
		t.Fatal(err)
	}
	if err := a.db.ActivateSpotifyMetadataContext("api-fixture"); err != nil {
		t.Fatal(err)
	}
	started, stopped := make(chan struct{}), make(chan struct{})
	provider := &spotifyRefreshFixtureProvider{fn: func(ctx context.Context, id, endpoint string) (spotifyanalysis.Observation, error) {
		if endpoint == spotifyrefresh.Features {
			close(started)
			<-ctx.Done()
			close(stopped)
		}
		bpm := 119.0
		return spotifyanalysis.Observation{AccountContext: "api-fixture", TrackID: id, Source: "spotify_internal", SourceEndpoint: endpoint, RetrievedAt: time.Now().UTC(), BPM: &bpm}, nil
	}}
	service, err := spotifyrefresh.New(a.db, provider, spotifyrefresh.Options{Enabled: true, AdapterRevision: "fixture"})
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	if err := a.InstallSpotifyAnalysisService(service); err != nil {
		t.Fatal(err)
	}
	if err := a.db.RefreshTrackAnalysisSourceRevision("song", source.Fingerprint); err != nil {
		t.Fatal(err)
	}
	if _, err := a.db.ConfirmSpotifyRecording("song", cachedSpotifyID, source.Fingerprint, true); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	result := make(chan *spotifyanalysis.Observation, 1)
	go func() { result <- a.spotifyPreparationForSource(ctx, source) }()
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	runtime.beginRetirement()
	select {
	case got := <-result:
		if got != nil {
			t.Fatalf("retired account bundle applied: %+v", got)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	select {
	case <-stopped:
	case <-ctx.Done():
		t.Fatal("last waiter did not cancel provider")
	}
}

func TestPreparationRejectsRecordingRelinkedDuringProviderWork(t *testing.T) {
	a, _ := newBPMRouteTestAPI(t, false)
	_ = a.spotifyTokens()
	source, err := analysis.ResolveLocalSource(a.db, "song")
	if err != nil {
		t.Fatal(err)
	}
	if err := a.db.ActivateSpotifyMetadataContext("api-fixture"); err != nil {
		t.Fatal(err)
	}
	started, release := make(chan struct{}), make(chan struct{})
	provider := &spotifyRefreshFixtureProvider{fn: func(ctx context.Context, id, endpoint string) (spotifyanalysis.Observation, error) {
		if endpoint == spotifyrefresh.Features {
			close(started)
			select {
			case <-release:
			case <-ctx.Done():
				return spotifyanalysis.Observation{}, ctx.Err()
			}
		}
		bpm := 118.0
		return spotifyanalysis.Observation{AccountContext: "api-fixture", TrackID: id, Source: "spotify_internal", SourceEndpoint: endpoint, RetrievedAt: time.Now().UTC(), BPM: &bpm}, nil
	}}
	service, err := spotifyrefresh.New(a.db, provider, spotifyrefresh.Options{Enabled: true, AdapterRevision: "fixture"})
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	if err := a.InstallSpotifyAnalysisService(service); err != nil {
		t.Fatal(err)
	}
	if err := a.db.RefreshTrackAnalysisSourceRevision("song", source.Fingerprint); err != nil {
		t.Fatal(err)
	}
	if _, err := a.db.ConfirmSpotifyRecording("song", cachedSpotifyID, source.Fingerprint, true); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	result := make(chan *spotifyanalysis.Observation, 1)
	go func() { result <- a.spotifyPreparationForSource(ctx, source) }()
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	_, writeErr := a.db.ConfirmSpotifyRecording("song", "abcdefghijkl0123456789", source.Fingerprint, true)
	close(release)
	if writeErr != nil {
		t.Fatal(writeErr)
	}
	select {
	case got := <-result:
		if got != nil {
			t.Fatalf("superseded recording accepted: %+v", got)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	cache, err := a.db.GetExternalAnalysis(cachedSpotifyID, spotifyrefresh.Features)
	if err != nil || cache == nil {
		t.Fatalf("old recording cache should survive relinking: %+v %v", cache, err)
	}
}
