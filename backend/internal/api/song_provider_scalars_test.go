package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/analysis"
	"github.com/ajbergh/viib-mediahub/internal/db"
	spotifyanalysis "github.com/ajbergh/viib-mediahub/internal/spotify/analysis"
	"github.com/go-chi/chi/v5"
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

	var energy *db.EffectiveScalar
	for i := range response.EffectiveFields {
		if response.EffectiveFields[i].Key == "spotify_energy_score" {
			energy = &response.EffectiveFields[i]
		}
	}
	if energy == nil || energy.State != "unknown" || energy.Selected != nil || energy.LastGood == nil || string(energy.LastGood.Value) != "0" {
		t.Fatalf("stale effective field contract: %+v", energy)
	}
	o.RetrievedAt = now
	if err := a.db.PutExternalAnalysis(o, "fixture", now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	w = httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/analysis/song", nil))
	response = TrackAnalysisFeatureResponse{}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil || w.Code != 200 {
		t.Fatalf("fresh detail: %d %s", w.Code, w.Body.String())
	}
	energy = nil
	for i := range response.EffectiveFields {
		if response.EffectiveFields[i].Key == "spotify_energy_score" {
			energy = &response.EffectiveFields[i]
		}
	}
	if energy == nil || energy.State != "available" || energy.Selected == nil || string(energy.Selected.Value) != "0" || energy.Selected.Source != "spotify_private" {
		t.Fatalf("fresh effective zero contract: %+v", energy)
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
	fresh := db.SpotifyScalarField{Key: "tempo_bpm", Metric: "tempo", Units: "bpm", Value: json.RawMessage(`120`), Endpoint: "audio_features", RetrievedAt: at}
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

func TestProviderScalarSelectionRejectsInvalidKnownCandidates(t *testing.T) {
	at := time.Now()
	valid := db.SpotifyScalarField{Key: "tempo_bpm", Metric: "tempo", Units: "bpm", Value: json.RawMessage(`120`), Endpoint: "audio_features", RetrievedAt: at}
	wrongUnits := valid
	wrongUnits.Units = "milliseconds"
	wrongUnits.RetrievedAt = at.Add(time.Hour)
	wrongMetric := valid
	wrongMetric.Metric = "duration"
	wrongMetric.RetrievedAt = at.Add(2 * time.Hour)
	malformed := valid
	malformed.Value = json.RawMessage(`"fast"`)
	malformed.RetrievedAt = at.Add(3 * time.Hour)
	nilValue := valid
	nilValue.Value = json.RawMessage(`null`)
	nilValue.RetrievedAt = at.Add(4 * time.Hour)

	selected := selectProviderScalarFields([]db.SpotifyScalarField{valid, wrongUnits, wrongMetric, malformed, nilValue})
	if len(selected) != 1 || selected[0].Metric != "tempo" || selected[0].Units != "bpm" || string(selected[0].Value) != "120" {
		t.Fatalf("invalid candidates displaced valid tempo: %+v", selected)
	}

	badScore := db.SpotifyScalarField{Key: "spotify_energy_score", Metric: "spotify_energy", Units: "unit_interval", Value: json.RawMessage(`1.1`), Endpoint: "audio_features", RetrievedAt: at.Add(time.Hour)}
	zeroScore := badScore
	zeroScore.Value = json.RawMessage(`0`)
	selected = selectProviderScalarFields([]db.SpotifyScalarField{badScore, zeroScore})
	if len(selected) != 1 || string(selected[0].Value) != "0" {
		t.Fatalf("invalid score displaced valid zero score: %+v", selected)
	}
	overflowScore := badScore
	overflowScore.Value = json.RawMessage(`1e10000`)
	overflowScore.RetrievedAt = at.Add(2 * time.Hour)
	selected = selectProviderScalarFields([]db.SpotifyScalarField{zeroScore, overflowScore})
	if len(selected) != 1 || string(selected[0].Value) != "0" {
		t.Fatalf("overflow score displaced valid zero score: %+v", selected)
	}

	for _, raw := range []json.RawMessage{json.RawMessage(`0`), json.RawMessage(`33`), json.RawMessage(`1e10000`)} {
		badMeter := db.SpotifyScalarField{Key: "time_signature", Metric: "measured_meter", Units: "beats_per_bar", Value: raw, Endpoint: "audio_features", RetrievedAt: at}
		if selected := selectProviderScalarFields([]db.SpotifyScalarField{badMeter}); len(selected) != 0 {
			t.Fatalf("invalid meter %s selected: %+v", raw, selected)
		}
	}
	partialKey := db.SpotifyScalarField{Key: "key_mode", Metric: "tonic_and_mode", Units: "pitch_class_and_mode", Value: json.RawMessage(`{"tonic":0}`), Endpoint: "audio_features", RetrievedAt: at}
	if selected := selectProviderScalarFields([]db.SpotifyScalarField{partialKey}); len(selected) != 0 {
		t.Fatalf("incomplete key/mode candidate selected: %+v", selected)
	}
}

func TestSongProviderFieldsExposeLatestRejectedAttemptWithStaleLastGoodValue(t *testing.T) {
	a, _ := newBPMRouteTestAPI(t, false)
	source, err := analysis.ResolveLocalSource(a.db, "song")
	if err != nil {
		t.Fatal(err)
	}
	if err := a.db.ActivateSpotifyMetadataContext("owner"); err != nil {
		t.Fatal(err)
	}
	first := time.Now().Add(-3 * time.Hour)
	zero := 0.0
	good := spotifyanalysis.Observation{TrackID: cachedSpotifyID, AccountContext: "owner", Source: "spotify_internal", SourceEndpoint: "audio_features", RetrievedAt: first, Energy: &zero}
	if err := a.db.PutExternalAnalysis(good, "fixture", first.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	second := good
	second.RetrievedAt = first.Add(2 * time.Hour)
	second.Energy = nil
	validBPM := 126.0
	second.BPM = &validBPM
	second.SourceEndpoint = "audio_analysis"
	second.RejectedFields = []spotifyanalysis.FieldRejection{{Path: "energy", Reason: "out_of_range"}}
	if err := a.db.PutExternalAnalysis(second, "fixture", second.RetrievedAt.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := a.db.RefreshTrackAnalysisSourceRevision("song", source.Fingerprint); err != nil {
		t.Fatal(err)
	}
	if ok, err := a.db.ConfirmSpotifyRecording("song", cachedSpotifyID, source.Fingerprint, true); err != nil || !ok {
		t.Fatal(ok, err)
	}
	result, err := a.songProviderScalars("song", source.Fingerprint)
	if err != nil || result == nil || len(result.Selected) != 2 {
		t.Fatalf("last-good selection = %+v, err=%v", result, err)
	}
	foundStaleEnergy := false
	for _, field := range result.Selected {
		if field.Key == "spotify_energy_score" && string(field.Value) == "0" && field.Stale {
			foundStaleEnergy = true
		}
	}
	if !foundStaleEnergy {
		t.Fatalf("stale last-good energy missing: %+v", result.Selected)
	}
	foundRejected := false
	for _, attempt := range result.Attempts {
		if attempt.Key == "spotify_energy_score" && attempt.Endpoint == "audio_analysis" && attempt.State == "invalid_field" && attempt.Reason == "out_of_range" {
			foundRejected = true
		}
	}
	if !foundRejected {
		t.Fatalf("latest rejected field attempt absent: %+v", result.Attempts)
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

func TestSongProviderFieldsPendingOwnerDoesNotExposeDurableDownloadCandidates(t *testing.T) {
	a, path := newBPMRouteTestAPI(t, false)
	database := a.db
	if err := os.WriteFile(path, []byte("final confirmed download revision"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := database.ReserveSpotifyMetadataRuntime("old-runtime"); err != nil {
		t.Fatal(err)
	}
	if _, err := database.ConfirmSpotifyMetadataOwner("old-runtime", "oauth", "old-account", "download-owner"); err != nil {
		t.Fatal(err)
	}
	zero := 0.0
	now := time.Now()
	observation := spotifyanalysis.Observation{TrackID: cachedSpotifyID, AccountContext: "download-owner", Source: "spotify_internal", SourceEndpoint: "audio_features", RetrievedAt: now, Energy: &zero}
	if err := database.PutExternalAnalysis(observation, "fixture", now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	source, err := analysis.ResolveLocalSource(database, "song")
	if err != nil {
		t.Fatal(err)
	}
	if err := database.RefreshTrackAnalysisSourceRevision("song", source.Fingerprint); err != nil {
		t.Fatal(err)
	}
	if ok, err := database.ConfirmSpotifyRecording("song", cachedSpotifyID, source.Fingerprint, true); err != nil || !ok {
		t.Fatal(ok, err)
	}
	if err := database.AddDownload(&db.SpotifyDownload{ID: "pending-import", SpotifyID: cachedSpotifyID, SpotifyURI: "spotify:track:" + cachedSpotifyID, Type: "track", Title: "Track", Status: "queued", FilePath: path, AddedAt: now.UnixMilli()}); err != nil {
		t.Fatal(err)
	}
	if changed, err := database.MarkDownloadStarted("pending-import"); err != nil || !changed {
		t.Fatal(changed, err)
	}
	if changed, err := database.MarkDownloadConverting("pending-import", path); err != nil || !changed {
		t.Fatal(changed, err)
	}
	if changed, err := database.MarkDownloadCompletedWithEvidence(t.Context(), "pending-import", path); err != nil || !changed {
		t.Fatal(changed, err)
	}
	if _, err := database.ReserveSpotifyMetadataRuntime("new-runtime"); err != nil {
		t.Fatal(err)
	}
	runtime := a.spotifyTokens()
	if runtime.pendingOwner == nil {
		t.Fatal("expected prior owner awaiting profile confirmation")
	}
	fields, err := a.songProviderScalars("song", source.Fingerprint)
	if err != nil || fields == nil || !fields.Unverified || fields.Provenance != "spotify_private_cache" || fields.DurableImportStatus != nil {
		t.Fatalf("pending owner private candidate state: %+v, err=%v", fields, err)
	}
	for _, field := range fields.Fields {
		if field.DurableImport {
			t.Fatalf("pending owner exposed durable scalar import: %+v", field)
		}
	}
	for _, attempt := range fields.Attempts {
		if attempt.DurableImport {
			t.Fatalf("pending owner exposed durable attempt import: %+v", attempt)
		}
	}
}

func TestSongProviderScalarDetailFallsBackToDurableDownloadImportOffline(t *testing.T) {
	a, path := newBPMRouteTestAPI(t, false)
	database := a.db
	if err := database.ActivateSpotifyMetadataContext("download-owner"); err != nil {
		t.Fatal(err)
	}
	zero := 0.0
	feature := spotifyanalysis.Observation{TrackID: cachedSpotifyID, AccountContext: "download-owner", Source: "spotify_internal", SourceEndpoint: "audio_features", RetrievedAt: time.Now(), Energy: &zero}
	if err := database.PutExternalAnalysis(feature, "fixture", time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("completed tagged download bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	initialSource, err := analysis.ResolveLocalSource(database, "song")
	if err != nil {
		t.Fatal(err)
	}
	if err := database.RefreshTrackAnalysisSourceRevision("song", initialSource.Fingerprint); err != nil {
		t.Fatal(err)
	}
	source, err := analysis.ResolveLocalSource(database, "song")
	if err != nil {
		t.Fatal(err)
	}
	if ok, err := database.ConfirmSpotifyRecording("song", cachedSpotifyID, source.Fingerprint, true); err != nil || !ok {
		t.Fatal(ok, err)
	}
	if err := database.AddDownload(&db.SpotifyDownload{ID: "durable", SpotifyID: cachedSpotifyID, SpotifyURI: "spotify:track:" + cachedSpotifyID, Type: "track", Title: "Track", Status: "queued", FilePath: path, AddedAt: time.Now().UnixMilli()}); err != nil {
		t.Fatal(err)
	}
	if changed, err := database.MarkDownloadStarted("durable"); err != nil || !changed {
		t.Fatal(changed, err)
	}
	if changed, err := database.MarkDownloadConverting("durable", path); err != nil || !changed {
		t.Fatal(changed, err)
	}
	if changed, err := database.MarkDownloadCompletedWithEvidence(t.Context(), "durable", path); err != nil || !changed {
		t.Fatal(changed, err)
	}
	if err := database.ActivateSpotifyMetadataContext(""); err != nil {
		t.Fatal(err)
	}
	if err := database.RetireSpotifyMetadataContext("download-owner"); err != nil {
		t.Fatal(err)
	}
	directFields, directAttempts, directRecording, directStatus, directErr := database.GetDownloadedSpotifyScalarCandidates(t.Context(), "song", source.Fingerprint)
	if directErr != nil || directRecording != cachedSpotifyID || len(directFields) == 0 || len(directAttempts) == 0 || directStatus == nil || directStatus.State != "available" {
		t.Fatalf("direct import read: fields=%d attempts=%d recording=%s err=%v", len(directFields), len(directAttempts), directRecording, directErr)
	}
	fields, err := a.songProviderScalars("song", source.Fingerprint)
	if err != nil || fields == nil || fields.Provenance != "spotify_download_import" || len(fields.Selected) != 1 || !fields.Selected[0].DurableImport || string(fields.Selected[0].Value) != "0" {
		t.Fatalf("offline durable candidates = %+v, err=%v", fields, err)
	}
}

func TestSongProviderScalarDetailMergesDurableAndPrivateCandidatesPerField(t *testing.T) {
	a, path := newBPMRouteTestAPI(t, false)
	database := a.db
	if err := database.ActivateSpotifyMetadataContext("mix-owner"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("final mixed-source download bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	initialSource, err := analysis.ResolveLocalSource(database, "song")
	if err != nil {
		t.Fatal(err)
	}
	if err := database.RefreshTrackAnalysisSourceRevision("song", initialSource.Fingerprint); err != nil {
		t.Fatal(err)
	}
	if ok, err := database.ConfirmSpotifyRecording("song", cachedSpotifyID, initialSource.Fingerprint, true); err != nil || !ok {
		t.Fatal(ok, err)
	}
	old := time.Now().Add(-3 * time.Hour)
	staleZero := 0.0
	if err := database.PutExternalAnalysis(spotifyanalysis.Observation{TrackID: cachedSpotifyID, AccountContext: "mix-owner", Source: "spotify_internal", SourceEndpoint: "audio_features", RetrievedAt: old, Energy: &staleZero}, "fixture", old.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	bpm := 124.0
	newer := time.Now()
	if err := database.PutExternalAnalysis(spotifyanalysis.Observation{TrackID: cachedSpotifyID, AccountContext: "mix-owner", Source: "spotify_internal", SourceEndpoint: "audio_analysis", RetrievedAt: newer, BPM: &bpm}, "fixture", newer.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := database.AddDownload(&db.SpotifyDownload{ID: "mixed", SpotifyID: cachedSpotifyID, SpotifyURI: "spotify:track:" + cachedSpotifyID, Type: "track", Title: "Track", Status: "converting", FilePath: path, AddedAt: time.Now().UnixMilli()}); err != nil {
		t.Fatal(err)
	}
	if changed, err := database.MarkDownloadCompletedWithEvidence(t.Context(), "mixed", path); err != nil || !changed {
		t.Fatal(changed, err)
	}
	importedFields, importedAttempts, importedRecording, importedStatus, importErr := database.GetDownloadedSpotifyScalarCandidates(t.Context(), "song", initialSource.Fingerprint)
	if importErr != nil || len(importedFields) == 0 || importedRecording != cachedSpotifyID || importedStatus == nil || importedStatus.State != "available" {
		t.Fatalf("completed download scalar import = %d fields, recording=%s err=%v; attempts=%d", len(importedFields), importedRecording, importErr, len(importedAttempts))
	}
	if err := database.SetSetting("spotify_metadata_active_context", ""); err != nil {
		t.Fatal(err)
	}
	if err := database.RetireSpotifyMetadataContext("mix-owner"); err != nil {
		t.Fatal(err)
	}
	if err := database.SetSetting("spotify_metadata_active_context", "new-private-owner"); err != nil {
		t.Fatal(err)
	}
	privateEnergy := .7
	privateBPM := 125.0
	refreshed := time.Now()
	if err := database.PutExternalAnalysis(spotifyanalysis.Observation{TrackID: cachedSpotifyID, AccountContext: "new-private-owner", Source: "spotify_internal", SourceEndpoint: "audio_features", RetrievedAt: refreshed, Energy: &privateEnergy, BPM: &privateBPM}, "fixture", refreshed.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	fields, err := a.songProviderScalars("song", initialSource.Fingerprint)
	if err != nil || fields == nil {
		t.Fatalf("mixed candidates = %+v, err=%v", fields, err)
	}
	if fields.Provenance != "spotify_mixed_private_and_download_import" || len(fields.Fields) < 3 {
		t.Fatalf("mixed provenance/alternatives = %+v", fields)
	}
	selected := map[string]db.SpotifyScalarField{}
	for _, field := range fields.Selected {
		selected[field.Key] = field
	}
	if selected["tempo_bpm"].DurableImport || string(selected["tempo_bpm"].Value) != "125" || selected["spotify_energy_score"].DurableImport || string(selected["spotify_energy_score"].Value) != "0.7" {
		t.Fatalf("per-field freshness/provenance = %+v", fields.Selected)
	}
	var importedTempoFound bool
	for _, candidate := range fields.Fields {
		if candidate.Key == "tempo_bpm" && candidate.DurableImport && string(candidate.Value) == "124" {
			importedTempoFound = true
		}
	}
	if !importedTempoFound {
		t.Fatalf("durable tempo alternative missing: %+v", fields.Fields)
	}
	if fields.Provenance != "spotify_mixed_private_and_download_import" {
		t.Fatalf("mixed candidate provenance = %q", fields.Provenance)
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
func TestSongProviderDurableImportStatusOnlyAndSourceFence(t *testing.T) {
	a, path := newBPMRouteTestAPI(t, false)
	source, err := analysis.ResolveLocalSource(a.db, "song")
	if err != nil {
		t.Fatal(err)
	}
	if err := a.db.RefreshTrackAnalysisSourceRevision("song", source.Fingerprint); err != nil {
		t.Fatal(err)
	}
	if ok, err := a.db.ConfirmSpotifyRecording("song", cachedSpotifyID, source.Fingerprint, true); err != nil || !ok {
		t.Fatal(ok, err)
	}
	if err := a.db.AddDownload(&db.SpotifyDownload{ID: "status-only", SpotifyID: cachedSpotifyID, Type: "track", Title: "Track", Status: "queued", FilePath: path, AddedAt: time.Now().UnixMilli()}); err != nil {
		t.Fatal(err)
	}
	if changed, err := a.db.MarkDownloadStarted("status-only"); err != nil || !changed {
		t.Fatal(changed, err)
	}
	if changed, err := a.db.MarkDownloadConverting("status-only", path); err != nil || !changed {
		t.Fatal(changed, err)
	}
	if changed, err := a.db.MarkDownloadCompletedWithEvidence(t.Context(), "status-only", path); err != nil || !changed {
		t.Fatal(changed, err)
	}

	fields, err := a.songProviderScalars("song", source.Fingerprint)
	if err != nil || fields == nil || fields.DurableImportStatus == nil || fields.DurableImportStatus.State != "not_available" || len(fields.Fields) != 0 || len(fields.Attempts) != 0 || len(fields.Selected) != 0 {
		t.Fatalf("status-only not-available response = %+v err=%v", fields, err)
	}
	if err := a.db.DeleteSpotifyRecording("song"); err != nil {
		t.Fatal(err)
	}
	if fields, err = a.songProviderScalars("song", source.Fingerprint); err != nil || fields != nil {
		t.Fatalf("unlinked import status = %+v err=%v", fields, err)
	}
	if ok, err := a.db.ConfirmSpotifyRecording("song", cachedSpotifyID, source.Fingerprint, true); err != nil || !ok {
		t.Fatal(ok, err)
	}
	if fields, err = a.songProviderScalars("song", source.Fingerprint); err != nil || fields == nil || fields.DurableImportStatus == nil || fields.DurableImportStatus.State != "not_available" {
		t.Fatalf("explicitly relinked status = %+v err=%v", fields, err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	replacement := append([]byte(nil), original...)
	replacement[0] ^= 0xff
	if err := os.WriteFile(path, replacement, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
	fields, err = a.songProviderScalars("song", source.Fingerprint)
	if err != nil || fields != nil {
		t.Fatalf("same-size/mtime replacement exposed durable status: %+v err=%v", fields, err)
	}
}
