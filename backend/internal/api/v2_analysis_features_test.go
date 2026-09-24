package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ajbergh/viib-mediahub/internal/analysis/beatgrid"
	"github.com/ajbergh/viib-mediahub/internal/analysis/features"
	"github.com/ajbergh/viib-mediahub/internal/db"
)

func TestV2TrackTempoEvidenceUsesMeasuredValuesOnly(t *testing.T) {
	bpm, alternate, stability, confidence := 128.5, 64.25, .8, .36
	source, kind := "measured", "dynamic-candidate"
	record := db.TrackAnalysis{Status: db.TrackAnalysisComplete, BPM: &bpm, BPMSource: &source, BPMAltCandidate: &alternate, TempoStability: &stability, BPMConfidence: &confidence, TempoKind: &kind}
	result := trackAnalysisFeatureResponse(record, db.TrackAnalysisOverride{})
	if result.BPMAltCandidate == nil || *result.BPMAltCandidate != alternate || result.TempoStability == nil || *result.TempoStability != stability || result.TempoKind == nil || *result.TempoKind != kind {
		t.Fatalf("missing measured tempo evidence: %#v", result)
	}
	result = trackAnalysisFeatureResponse(record, db.TrackAnalysisOverride{BPM: &bpm, BPMLocked: true})
	if result.BPMAltCandidate != nil || result.TempoStability != nil || result.BPMConfidence != nil || result.TempoKind != nil {
		t.Fatalf("measured evidence attributed to manual tempo: %#v", result)
	}
}

func TestV2TrackAnalysisExposesOnlyCurrentSettledEnergyLevel(t *testing.T) {
	level, confidence, version := 8, .8, features.EnergyLevelAlgorithmVersion
	analysis := db.TrackAnalysis{SongID: "song", Status: db.TrackAnalysisPartial, EnergyLevel: &level, EnergyLevelConfidence: &confidence, EnergyAlgorithmVersion: &version}
	response := trackAnalysisFeatureResponse(analysis, db.TrackAnalysisOverride{})
	if response.EnergyLevel == nil || *response.EnergyLevel != level || response.EnergyLevelConfidence == nil || *response.EnergyLevelConfidence != confidence || response.EnergyAlgorithmVersion == nil || *response.EnergyAlgorithmVersion != version {
		t.Fatalf("energy score/version missing from API response: %#v", response)
	}
	analysis.Status = db.TrackAnalysisRunning
	if running := trackAnalysisFeatureResponse(analysis, db.TrackAnalysisOverride{}); running.EnergyLevel != nil {
		t.Fatalf("running analysis exposed stale Energy Level: %#v", running)
	}
	analysis.Status = db.TrackAnalysisComplete
	staleVersion := "energy-level-v0"
	analysis.EnergyAlgorithmVersion = &staleVersion
	if stale := trackAnalysisFeatureResponse(analysis, db.TrackAnalysisOverride{}); stale.EnergyLevel != nil {
		t.Fatalf("stale algorithm score was exposed: %#v", stale)
	}
}

func TestV2TrackAnalysisFeatureResolvesManualValues(t *testing.T) {
	database, err := db.New(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := database.SaveSong(&db.Song{ID: "song", Title: "Song", Artist: "Artist", Album: "Album", FilePath: "song.mp3", AddedAt: 1}); err != nil {
		t.Fatal(err)
	}
	measuredBPM, measuredTonic := 128.25, 0
	measuredMode, measuredSource := "major", "measured"
	if err := database.UpsertTrackAnalysis(db.TrackAnalysis{
		SongID: "song", Status: db.TrackAnalysisComplete, AnalysisVersion: 1, AlgorithmVersion: "test-v1", SourceFingerprint: "source-v1",
		BPM: &measuredBPM, BPMSource: &measuredSource, KeyTonic: &measuredTonic, KeyMode: &measuredMode, KeySource: &measuredSource,
	}); err != nil {
		t.Fatal(err)
	}
	manualBPM, manualTonic := 127.5, 9
	manualMode := "minor"
	if err := database.UpsertTrackAnalysisOverride(db.TrackAnalysisOverride{
		SongID: "song", BPM: &manualBPM, BPMLocked: true, KeyTonic: &manualTonic, KeyMode: &manualMode, KeyLocked: true,
	}); err != nil {
		t.Fatal(err)
	}

	handler := (&API{db: database}).V2Routes()
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/analysis/song", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("GET /analysis/song = %d, want %d: %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	var response TrackAnalysisFeatureResponse
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}
	if response.BPM == nil || *response.BPM != manualBPM || response.BPMSource != db.EffectiveBPMManual || !response.SyncAllowed || response.BPMConfidence != nil {
		t.Fatalf("BPM response = %#v, want locked manual value", response)
	}
	if response.Key == nil || *response.Key != "A minor" || response.CamelotKey == nil || *response.CamelotKey != "8A" || response.KeySource != db.EffectiveKeyManual || response.KeyConfidence != nil {
		t.Fatalf("key response = %#v, want manual A minor / 8A", response)
	}
}

func TestV2TrackAnalysisFeatureReturnsNotFoundWhenAbsent(t *testing.T) {
	database, err := db.New(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	recorder := httptest.NewRecorder()
	(&API{db: database}).V2Routes().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/analysis/missing", nil))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("GET /analysis/missing = %d, want %d", recorder.Code, http.StatusNotFound)
	}
}

func TestV2TrackAnalysisFeaturesListsResolvedRecords(t *testing.T) {
	database, err := db.New(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	for _, id := range []string{"second", "first"} {
		if err := database.SaveSong(&db.Song{ID: id, Title: id, Artist: "Artist", Album: "Album", FilePath: id + ".mp3", AddedAt: 1}); err != nil {
			t.Fatal(err)
		}
	}
	bpm, tonic := 128.0, 0
	mode, source := "major", "measured"
	for _, id := range []string{"second", "first"} {
		if err := database.UpsertTrackAnalysis(db.TrackAnalysis{SongID: id, Status: db.TrackAnalysisComplete, AnalysisVersion: 1, AlgorithmVersion: "test-v1", SourceFingerprint: id, BPM: &bpm, BPMSource: &source, KeyTonic: &tonic, KeyMode: &mode, KeySource: &source}); err != nil {
			t.Fatal(err)
		}
	}
	recorder := httptest.NewRecorder()
	(&API{db: database}).V2Routes().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/analysis", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("GET /analysis = %d, want %d: %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	var response []TrackAnalysisFeatureResponse
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}
	if len(response) != 2 || response[0].SongID != "first" || response[1].SongID != "second" || response[0].CamelotKey == nil || *response[0].CamelotKey != "8B" {
		t.Fatalf("feature list = %#v, want sorted resolved records", response)
	}
}

func TestV2BeatGridUpdateRoundTripsAndLocksWithoutLosingManualValues(t *testing.T) {
	database, err := db.New(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := database.SaveSong(&db.Song{ID: "song", Title: "Song", Artist: "Artist", Album: "Album", FilePath: "song.mp3", AddedAt: 1}); err != nil {
		t.Fatal(err)
	}
	manual := 126.0
	if err := database.UpsertTrackAnalysisOverride(db.TrackAnalysisOverride{SongID: "song", BPM: &manual, BPMLocked: true}); err != nil {
		t.Fatal(err)
	}
	handler := (&API{db: database}).V2Routes()
	body := `{"beats":[0.125,0.625,1.125,1.625],"downbeatIndices":[0],"locked":true}`
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPut, "/analysis/song/beatgrid", strings.NewReader(body)))
	if recorder.Code != http.StatusOK {
		t.Fatalf("PUT beatgrid = %d: %s", recorder.Code, recorder.Body.String())
	}
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/analysis/song/beatgrid", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("GET beatgrid = %d: %s", recorder.Code, recorder.Body.String())
	}
	var response BeatGridResponse
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}
	if response.Source != "manual" || response.Provenance != "manual" || !response.Locked || len(response.Beats) != 4 || response.Beats[0] != .125 {
		t.Fatalf("beatgrid = %#v", response)
	}
	artifact, err := database.GetTrackAnalysisArtifact("song", beatgrid.ArtifactKind, beatgrid.FormatVersion, beatgrid.AlgorithmVersion)
	if err != nil || artifact.Provenance != string(beatgrid.ProvenanceManual) {
		t.Fatalf("manual artifact provenance = %q, err=%v", artifact.Provenance, err)
	}
	override, err := database.GetTrackAnalysisOverride("song")
	if err != nil || override.BPM == nil || *override.BPM != manual || !override.BPMLocked {
		t.Fatalf("override = %#v, err = %v", override, err)
	}
	// Applying a detected tempo must survive reloading along with its grid.
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPut, "/analysis/song/beatgrid", strings.NewReader(`{"beats":[0.13,0.63,1.13],"downbeatIndices":[],"locked":true,"bpm":120}`)))
	if recorder.Code != http.StatusOK {
		t.Fatalf("save corrected tempo: %d %s", recorder.Code, recorder.Body.String())
	}
	override, err = database.GetTrackAnalysisOverride("song")
	if err != nil || override.BPM == nil || *override.BPM != 120 || !override.BPMLocked || !override.BeatgridLocked {
		t.Fatalf("corrected tempo did not persist: %#v, %v", override, err)
	}
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPut, "/analysis/song/beatgrid", strings.NewReader(`{"beats":[0,0.5,1],"locked":true,"bpm":-1}`)))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("invalid tempo accepted: %d", recorder.Code)
	}
}

func TestV2BeatGridResetClearsOnlyGridOverride(t *testing.T) {
	database, err := db.New(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := database.SaveSong(&db.Song{ID: "song", Title: "Song", Artist: "Artist", Album: "Album", FilePath: "song.mp3", AddedAt: 1}); err != nil {
		t.Fatal(err)
	}
	manual := 126.0
	if err := database.UpsertTrackAnalysisOverride(db.TrackAnalysisOverride{SongID: "song", BPM: &manual, BPMLocked: true}); err != nil {
		t.Fatal(err)
	}
	handler := (&API{db: database}).V2Routes()
	body := `{"beats":[0.125,0.625,1.125,1.625],"downbeatIndices":[0],"locked":true}`
	put := httptest.NewRecorder()
	handler.ServeHTTP(put, httptest.NewRequest(http.MethodPut, "/analysis/song/beatgrid", strings.NewReader(body)))
	if put.Code != http.StatusOK {
		t.Fatalf("PUT beatgrid = %d: %s", put.Code, put.Body.String())
	}
	reset := httptest.NewRecorder()
	handler.ServeHTTP(reset, httptest.NewRequest(http.MethodDelete, "/analysis/song/beatgrid", nil))
	if reset.Code != http.StatusNoContent {
		t.Fatalf("DELETE beatgrid = %d: %s", reset.Code, reset.Body.String())
	}
	if _, err := database.GetTrackAnalysisArtifact("song", "beatgrid", 1, "beatgrid-v1-phase-v1"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("artifact err = %v, want sql.ErrNoRows", err)
	}
	override, err := database.GetTrackAnalysisOverride("song")
	if err != nil || override.BeatgridLocked || override.BeatgridArtifactID != nil || override.BPM == nil || *override.BPM != manual || !override.BPMLocked {
		t.Fatalf("override = %#v, err = %v", override, err)
	}
}

func TestV2EnergyFeaturesReturnsVersionedMeasurement(t *testing.T) {
	database, err := db.New(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := database.SaveSong(&db.Song{ID: "song", Title: "Song", Artist: "Artist", Album: "Album", FilePath: "song.mp3", AddedAt: 1}); err != nil {
		t.Fatal(err)
	}
	result := features.Result{IntegratedLUFS: -12.4, TruePeakDBFS: -.3, Energy: []features.EnergyPoint{{Time: 0, Value: .2}, {Time: .5, Value: .8}}, Sections: []features.Section{{Start: 0, End: 1, Energy: .5}}}
	encoded, err := result.Encode()
	if err != nil {
		t.Fatal(err)
	}
	if err := database.UpsertTrackAnalysisArtifact(db.TrackAnalysisArtifact{ID: "song:energy", SongID: "song", Kind: features.ArtifactKind, FormatVersion: features.FormatVersion, AlgorithmVersion: features.AlgorithmVersion, Encoding: features.Encoding, Data: encoded}); err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	(&API{db: database}).V2Routes().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/analysis/song/energy", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("GET energy = %d: %s", recorder.Code, recorder.Body.String())
	}
	var response EnergyFeaturesResponse
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}
	if response.AlgorithmVersion != features.AlgorithmVersion || len(response.Energy) != 2 || response.IntegratedLUFS != result.IntegratedLUFS {
		t.Fatalf("energy response = %#v", response)
	}
}

func TestV2TransitionRecommendationsExposeMeasuredRationale(t *testing.T) {
	database, err := db.New(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	for _, id := range []string{"source", "compatible", "incompatible"} {
		if err := database.SaveSong(&db.Song{ID: id, Title: id, Artist: "Artist", Album: "Album", FilePath: id + ".mp3", AddedAt: 1}); err != nil {
			t.Fatal(err)
		}
	}
	results := map[string]features.Result{
		"source":       {IntegratedLUFS: -10, Energy: []features.EnergyPoint{{Value: .2}, {Value: .8}}, CueSuggestions: []features.CueSuggestion{{Kind: "mix-out", Confidence: .8}}},
		"compatible":   {IntegratedLUFS: -10.5, Energy: []features.EnergyPoint{{Value: .75}, {Value: .7}}, CueSuggestions: []features.CueSuggestion{{Kind: "mix-in", Confidence: .8}}},
		"incompatible": {IntegratedLUFS: -25, Energy: []features.EnergyPoint{{Value: .05}, {Value: .1}}},
	}
	for id, result := range results {
		encoded, err := result.Encode()
		if err != nil {
			t.Fatal(err)
		}
		if err := database.UpsertTrackAnalysisArtifact(db.TrackAnalysisArtifact{ID: id + ":energy", SongID: id, Kind: features.ArtifactKind, FormatVersion: features.FormatVersion, AlgorithmVersion: features.AlgorithmVersion, Encoding: features.Encoding, Data: encoded}); err != nil {
			t.Fatal(err)
		}
	}
	measuredSource, keyMode := "measured", "major"
	energyVersion := features.EnergyLevelAlgorithmVersion
	confidence := .9
	for _, metadata := range []struct {
		id    string
		bpm   float64
		key   int
		level int
	}{{id: "source", bpm: 128, key: 0, level: 5}, {id: "compatible", bpm: 130, key: 0, level: 6}, {id: "incompatible", bpm: 150, key: 5, level: 2}} {
		bpm, tonic, level := metadata.bpm, metadata.key, metadata.level
		if err := database.UpsertTrackAnalysis(db.TrackAnalysis{SongID: metadata.id, Status: db.TrackAnalysisComplete, AnalysisVersion: 1, AlgorithmVersion: "test-v1", SourceFingerprint: metadata.id, BPM: &bpm, BPMConfidence: &confidence, BPMSource: &measuredSource, KeyTonic: &tonic, KeyMode: &keyMode, KeyConfidence: &confidence, KeySource: &measuredSource, EnergyLevel: &level, EnergyLevelConfidence: &confidence, EnergyAlgorithmVersion: &energyVersion}); err != nil {
			t.Fatal(err)
		}
	}
	recorder := httptest.NewRecorder()
	(&API{db: database}).V2Routes().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/analysis/source/recommendations?limit=1", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("GET recommendations = %d: %s", recorder.Code, recorder.Body.String())
	}
	var response TransitionRecommendationsResponse
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}
	if len(response.Recommendations) != 1 || response.Recommendations[0].SongID != "compatible" || len(response.Recommendations[0].Components) != 6 || response.Intent != features.TransitionIntentHold || response.AlgorithmVersion != features.TransitionAlgorithmVersion || response.Recommendations[0].Vector.CamelotRelation != "same" || response.Recommendations[0].Vector.BPMDelta == nil {
		t.Fatalf("recommendations = %#v", response)
	}
	unsupported := httptest.NewRecorder()
	(&API{db: database}).V2Routes().ServeHTTP(unsupported, httptest.NewRequest(http.MethodGet, "/analysis/source/recommendations?intent=vocal-safe", nil))
	if unsupported.Code != http.StatusBadRequest {
		t.Fatalf("unsupported vocal-safe intent status=%d, want 400", unsupported.Code)
	}
	if err := database.UpsertStemSet(db.StemSet{ID: "compatible-stems", SongID: "compatible", Status: "ready", PackagePath: "compatible.stems"}); err != nil {
		t.Fatal(err)
	}
	filtered := httptest.NewRecorder()
	(&API{db: database}).V2Routes().ServeHTTP(filtered, httptest.NewRequest(http.MethodGet, "/analysis/source/recommendations?minBpm=129&maxBpm=131&minEnergyLevel=6&maxEnergyLevel=6&stemsAvailable=true", nil))
	if filtered.Code != http.StatusOK {
		t.Fatalf("GET filtered recommendations = %d: %s", filtered.Code, filtered.Body.String())
	}
	if err := json.NewDecoder(filtered.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}
	if response.CandidatesBeforeFilters != 2 || response.CandidatesAfterFilters != 1 || len(response.Recommendations) != 1 || response.Recommendations[0].SongID != "compatible" {
		t.Fatalf("filtered recommendation counts/results = %#v", response)
	}
	if response.Filters.MinBPM == nil || *response.Filters.MinBPM != 129 || response.Recommendations[0].FilterEvidence.BPM == nil || *response.Recommendations[0].FilterEvidence.BPM != 130 || response.Recommendations[0].FilterEvidence.StemsAvailable == nil || !*response.Recommendations[0].FilterEvidence.StemsAvailable {
		t.Fatalf("filter evidence not echoed: %#v", response)
	}
	withoutStems := httptest.NewRecorder()
	(&API{db: database}).V2Routes().ServeHTTP(withoutStems, httptest.NewRequest(http.MethodGet, "/analysis/source/recommendations?stemsAvailable=false", nil))
	if withoutStems.Code != http.StatusOK {
		t.Fatalf("GET no-stems recommendations = %d: %s", withoutStems.Code, withoutStems.Body.String())
	}
	if err := json.NewDecoder(withoutStems.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}
	if len(response.Recommendations) != 1 || response.Recommendations[0].SongID != "incompatible" {
		t.Fatalf("stemsAvailable=false results = %#v", response.Recommendations)
	}
}

func TestV2TransitionRecommendationFiltersRejectInvalidQueries(t *testing.T) {
	database, err := db.New(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	for _, query := range []string{
		"minBpm=59", "maxBpm=191", "minBpm=NaN", "minEnergyLevel=0", "maxEnergyLevel=11",
		"minBpm=130&maxBpm=120", "minEnergyLevel=8&maxEnergyLevel=4", "stemsAvailable=yes", "minBpm=120&minBpm=121",
	} {
		t.Run(query, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			(&API{db: database}).V2Routes().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/analysis/source/recommendations?"+query, nil))
			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("status=%d, want 400: %s", recorder.Code, recorder.Body.String())
			}
		})
	}
}

func TestV2BeatGridProvenance(t *testing.T) {
	for _, test := range []struct {
		name       string
		override   bool
		locked     bool
		provenance string
	}{
		{"native phase grid", false, false, "inferred-from-meter"},
		{"future measured detector", false, false, "measured"},
		{"unlocked legacy edit", true, false, "unknown"},
		{"reviewed manual", true, true, "manual"},
	} {
		t.Run(test.name, func(t *testing.T) {
			database, err := db.New(filepath.Join(t.TempDir(), "library.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer database.Close()
			if err := database.SaveSong(&db.Song{ID: "song", Title: "Song", Artist: "Artist", Album: "Album", FilePath: "song.mp3", AddedAt: 1}); err != nil {
				t.Fatal(err)
			}
			grid := beatgrid.Grid{Beats: []float64{0.1, 0.6, 1.1}, DownbeatIndices: []int{0}}
			encoded, err := grid.Encode()
			if err != nil {
				t.Fatal(err)
			}
			id := "song:" + beatgrid.AlgorithmVersion
			if err := database.UpsertTrackAnalysisArtifact(db.TrackAnalysisArtifact{ID: id, SongID: "song", Kind: beatgrid.ArtifactKind, FormatVersion: beatgrid.FormatVersion, AlgorithmVersion: beatgrid.AlgorithmVersion, Encoding: beatgrid.Encoding, Provenance: test.provenance, Data: encoded}); err != nil {
				t.Fatal(err)
			}
			if test.override {
				if err := database.UpsertTrackAnalysisOverride(db.TrackAnalysisOverride{SongID: "song", BeatgridArtifactID: &id, BeatgridLocked: test.locked}); err != nil {
					t.Fatal(err)
				}
			}
			response := httptest.NewRecorder()
			(&API{db: database}).V2Routes().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/analysis/song/beatgrid", nil))
			if response.Code != http.StatusOK {
				t.Fatalf("GET: %d %s", response.Code, response.Body.String())
			}
			var gridResponse BeatGridResponse
			if err := json.NewDecoder(response.Body).Decode(&gridResponse); err != nil {
				t.Fatal(err)
			}
			if gridResponse.Provenance != test.provenance || gridResponse.Source != test.provenance {
				t.Fatalf("source=%s provenance=%s, want %s", gridResponse.Source, gridResponse.Provenance, test.provenance)
			}
		})
	}
}
