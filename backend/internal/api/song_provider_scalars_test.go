package api

import (
	"context"
	"encoding/json"
	"github.com/ajbergh/viib-mediahub/internal/analysis"
	"github.com/ajbergh/viib-mediahub/internal/db"
	spotifyanalysis "github.com/ajbergh/viib-mediahub/internal/spotify/analysis"
	"github.com/go-chi/chi/v5"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func TestSongProviderFieldsRequireLiveSourceLinkAndOwner(t *testing.T) {
	a, path := newBPMRouteTestAPI(t, false)
	source, err := analysis.ResolveLocalSource(a.db, "song")
	if err != nil {
		t.Fatal(err)
	}
	if err := a.db.ActivateSpotifyMetadataContext("owner"); err != nil {
		t.Fatal(err)
	}
	zero := 0.0
	now := time.Now()
	o := spotifyanalysis.Observation{TrackID: cachedSpotifyID, AccountContext: "owner", Source: "spotify_internal", SourceEndpoint: "audio_features", RetrievedAt: now.Add(-2 * time.Hour), Energy: &zero}
	if err := a.db.PutExternalAnalysis(o, "fixture", now.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	if fields, err := a.songProviderScalars("song", source.Fingerprint); err != nil || fields != nil {
		t.Fatal("unlinked recording exposed")
	}
	if err := a.db.RefreshTrackAnalysisSourceRevision("song", source.Fingerprint); err != nil {
		t.Fatal(err)
	}
	if ok, err := a.db.ConfirmSpotifyRecording("song", cachedSpotifyID, source.Fingerprint, true); err != nil || !ok {
		t.Fatal(err)
	}
	fields, err := a.songProviderScalars("song", source.Fingerprint)
	if err != nil || fields == nil || len(fields.Fields) != 1 || string(fields.Fields[0].Value) != "0" || !fields.Fields[0].Stale {
		t.Fatalf("provider candidates: %+v %v", fields, err)
	}
	router := chi.NewRouter()
	router.Get("/analysis/{songID}", a.getTrackAnalysisFeatureV2)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/analysis/song", nil))
	var response TrackAnalysisFeatureResponse
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil || w.Code != 200 || response.ProviderScalars == nil || response.EnergyLevel != nil || response.IntegratedLUFSBS1770 != nil {
		t.Fatalf("detail response conflated semantics: %d %s", w.Code, w.Body.String())
	}
	if err := a.db.ActivateSpotifyMetadataContext("replacement"); err != nil {
		t.Fatal(err)
	}
	if fields, err := a.songProviderScalars("song", source.Fingerprint); err != nil || fields != nil {
		t.Fatal("retired account exposed")
	}
	if err := a.db.ActivateSpotifyMetadataContext("owner"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("changed audio bytes with another length"), 0600); err != nil {
		t.Fatal(err)
	}
	if fields, err := a.songProviderScalars("song", source.Fingerprint); err != nil || fields != nil {
		t.Fatal("replaced source borrowed fields")
	}
}

func TestProviderScalarSelectionUsesIndependentFreshness(t *testing.T) {
	at := time.Now()
	fresh := db.SpotifyScalarField{Key: "tempo_bpm", Metric: "tempo", Units: "bpm", Endpoint: "audio_features", RetrievedAt: at}
	newerStale := fresh
	newerStale.Endpoint = "audio_analysis"
	newerStale.RetrievedAt = at.Add(time.Hour)
	newerStale.Stale = true
	selected := selectProviderScalarFields([]db.SpotifyScalarField{newerStale, fresh})
	if len(selected) != 1 || selected[0].Endpoint != "audio_features" {
		t.Fatal("stale endpoint displaced fresh field")
	}
	newerStale.Stale = false
	selected = selectProviderScalarFields([]db.SpotifyScalarField{fresh, newerStale})
	if selected[0].Endpoint != "audio_analysis" {
		t.Fatal("newer fresh field lost")
	}
	tie := fresh
	tie.Endpoint = "audio_analysis"
	selected = selectProviderScalarFields([]db.SpotifyScalarField{tie, fresh})
	if selected[0].Endpoint != "audio_analysis" {
		t.Fatal("equal-time endpoint choice depends on input order")
	}
	fresh.Stale = true
	newerStale.Stale = true
	selected = selectProviderScalarFields([]db.SpotifyScalarField{newerStale, fresh})
	if !selected[0].Stale || selected[0].Endpoint != "audio_analysis" {
		t.Fatal("last-good lost staleness")
	}
	incompatible := newerStale
	incompatible.Units = "LUFS"
	incompatible.RetrievedAt = at.Add(2 * time.Hour)
	selected = selectProviderScalarFields([]db.SpotifyScalarField{fresh, incompatible})
	if selected[0].Units != "bpm" {
		t.Fatal("incompatible units selected")
	}
}

func TestSongProviderFieldsRetainedWhileOwnerPending(t *testing.T) {
	a, _ := newBPMRouteTestAPI(t, false)
	source, err := analysis.ResolveLocalSource(a.db, "song")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.db.ReserveSpotifyMetadataRuntime("seed"); err != nil {
		t.Fatal(err)
	}
	if _, err := a.db.ConfirmSpotifyMetadataOwner("seed", "oauth", "account", "owner"); err != nil {
		t.Fatal(err)
	}
	zero := 0.0
	now := time.Now()
	observation := spotifyanalysis.Observation{TrackID: cachedSpotifyID, AccountContext: "owner", Source: "spotify_internal", SourceEndpoint: "audio_features", RetrievedAt: now, Energy: &zero}
	if err := a.db.PutExternalAnalysis(observation, "fixture", now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := a.db.RefreshTrackAnalysisSourceRevision("song", source.Fingerprint); err != nil {
		t.Fatal(err)
	}
	if ok, err := a.db.ConfirmSpotifyRecording("song", cachedSpotifyID, source.Fingerprint, true); err != nil || !ok {
		t.Fatal(ok, err)
	}
	runtime := a.spotifyTokens()
	if runtime.pendingOwner == nil {
		t.Fatal("owner not pending")
	}
	fields, err := a.songProviderScalars("song", source.Fingerprint)
	if err != nil || fields == nil || !fields.Unverified || !fields.ReadOnly || len(fields.Fields) != 1 || string(fields.Fields[0].Value) != "0" {
		t.Fatal(fields, err)
	}
	if active, err := a.db.GetSetting("spotify_metadata_active_context"); err != nil || active != "" {
		t.Fatal("candidate read activated provider", active, err)
	}
}

func TestSongProviderFieldsCanceledRequest(t *testing.T) {
	a, _ := newBPMRouteTestAPI(t, false)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if fields, err := a.songProviderScalarsContext(ctx, "song", "fingerprint"); fields != nil || err != context.Canceled {
		t.Fatal(fields, err)
	}
}
