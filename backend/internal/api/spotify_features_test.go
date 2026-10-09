// Tests and fixtures for spotify features behavior.

package api

import (
	"context"
	"encoding/json"
	"github.com/ajbergh/viib-mediahub/internal/analysis"
	"github.com/ajbergh/viib-mediahub/internal/analysis/beatgrid"
	"github.com/ajbergh/viib-mediahub/internal/db"
	spotifyanalysis "github.com/ajbergh/viib-mediahub/internal/spotify/analysis"
	spotifyrefresh "github.com/ajbergh/viib-mediahub/internal/spotify/refresh"
	"os"
	"testing"
	"time"
)

func TestDownloadedPreparationUsesFreshSpotifyFieldsAndPreservesProvenance(t *testing.T) {
	confidence := .8
	fields := []db.SpotifyScalarField{
		{Key: "tempo_bpm", Metric: "tempo", Units: "bpm", Value: json.RawMessage(`123.5`), Confidence: &confidence, Endpoint: "audio_features", RetrievedAt: time.Now(), DurableImport: true},
		{Key: "key_mode", Metric: "tonic_and_mode", Units: "pitch_class_and_mode", Value: json.RawMessage(`{"tonic":9,"mode":0}`), Endpoint: "audio_analysis", RetrievedAt: time.Now(), DurableImport: true},
		{Key: "tempo_bpm", Metric: "tempo", Units: "bpm", Value: json.RawMessage(`111`), Endpoint: "audio_features", RetrievedAt: time.Now().Add(-time.Hour), ExpiresAt: time.Now().Add(-time.Minute), Stale: true, DurableImport: true},
	}
	observation := downloadedPreparationObservation(fields, cachedSpotifyID)
	if observation == nil || observation.BPM == nil || *observation.BPM != 123.5 || observation.Key == nil || *observation.Key != 9 || observation.Mode == nil || *observation.Mode != 0 {
		t.Fatalf("downloaded provider preparation: %+v", observation)
	}
	record := db.TrackAnalysis{SongID: "song", SourceFingerprint: "current"}
	db.ApplySpotifyScalars(&record, *observation)
	if record.SpotifyBindings == nil || record.SpotifyBindings.BPM == nil || !record.SpotifyBindings.BPM.Durable || record.SpotifyBindings.BPM.Endpoint != "audio_features" || record.SpotifyBindings.Key == nil || !record.SpotifyBindings.Key.Durable || record.SpotifyBindings.Key.Endpoint != "audio_analysis" {
		t.Fatalf("durable source provenance was lost: %+v", record.SpotifyBindings)
	}
	staleOnly := downloadedPreparationObservation(fields[2:], cachedSpotifyID)
	if staleOnly != nil {
		t.Fatalf("stale durable fields must leave room for local fallback: %+v", staleOnly)
	}
}

func TestPreparationObservationUsesFreshSpotifyFieldsBeforeLocalFallback(t *testing.T) {
	primaryBPM, fallbackBPM, fallbackKey, fallbackMode := 100.0, 120.0, 4, 1
	primary := &spotifyanalysis.Observation{TrackID: cachedSpotifyID, BPM: &primaryBPM}
	providerGrid := &beatgrid.Grid{Beats: []float64{0, .5, 1}, DownbeatIndices: []int{0, 2}, Provenance: beatgrid.ProvenanceSpotify}
	fallback := &spotifyanalysis.Observation{TrackID: cachedSpotifyID, BPM: &fallbackBPM, Key: &fallbackKey, Mode: &fallbackMode, ProviderThreeBandAvailable: true, ProviderBeatGrid: providerGrid}
	got := mergePreparationObservations(primary, fallback)
	if got.BPM == nil || *got.BPM != primaryBPM || got.Key == nil || *got.Key != fallbackKey || !got.ProviderThreeBandAvailable || got.ProviderBeatGrid != providerGrid {
		t.Fatalf("provider precedence/fallback merge: %+v", got)
	}
}

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

func TestSpotifyBeatGridRequiresUsableOrderedProviderBeatsAndBars(t *testing.T) {
	valid := spotifyBeatGridFromArtifacts(
		[]byte("{\"beats\":[{\"start\":0},{\"start\":0.5},{\"start\":1},{\"start\":1.5}]}"),
		[]byte("{\"bars\":[{\"start\":0},{\"start\":1}]}"),
	)
	if valid == nil || valid.Provenance != beatgrid.ProvenanceSpotify || len(valid.Beats) != 4 || len(valid.DownbeatIndices) != 2 || valid.DownbeatIndices[1] != 2 {
		t.Fatalf("provider beat grid projection: %+v", valid)
	}
	for name, fixture := range map[string][2]string{
		"missing-bars":        {"{\"beats\":[{\"start\":0},{\"start\":0.5}]}", "{\"bars\":[]}"},
		"unsorted-beats":      {"{\"beats\":[{\"start\":0.5},{\"start\":0.4}]}", "{\"bars\":[{\"start\":0}]}"},
		"unmatched-bar":       {"{\"beats\":[{\"start\":0},{\"start\":0.5}]}", "{\"bars\":[{\"start\":0.2}]}"},
		"duplicate-downbeats": {"{\"beats\":[{\"start\":0},{\"start\":0.5},{\"start\":1}]}", "{\"bars\":[{\"start\":0},{\"start\":0.0005}]}"},
	} {
		t.Run(name, func(t *testing.T) {
			if got := spotifyBeatGridFromArtifacts([]byte(fixture[0]), []byte(fixture[1])); got != nil {
				t.Fatalf("invalid provider timing accepted: %+v", got)
			}
		})
	}
}
