package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ajbergh/viib-mediahub/internal/analysis/features"
	"github.com/ajbergh/viib-mediahub/internal/db"
)

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
	if !response.Locked || len(response.Beats) != 4 || response.Beats[0] != .125 {
		t.Fatalf("beatgrid = %#v", response)
	}
	override, err := database.GetTrackAnalysisOverride("song")
	if err != nil || override.BPM == nil || *override.BPM != manual || !override.BPMLocked {
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
