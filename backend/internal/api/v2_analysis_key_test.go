package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ajbergh/viib-mediahub/internal/db"
)

func newKeyRouteTestDB(t *testing.T) *db.DB {
	t.Helper()
	database, err := db.New(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err := database.SaveSong(&db.Song{ID: "song", Title: "Song", Artist: "Artist", Album: "Album", FilePath: "song.mp3", AddedAt: 1}); err != nil {
		t.Fatal(err)
	}
	measuredTonic := 0
	measuredMode, measuredSource := "major", "measured"
	if err := database.UpsertTrackAnalysis(db.TrackAnalysis{
		SongID: "song", Status: db.TrackAnalysisComplete, AnalysisVersion: 1,
		AlgorithmVersion: "test-v1", SourceFingerprint: "source-v1",
		KeyTonic: &measuredTonic, KeyMode: &measuredMode, KeySource: &measuredSource,
	}); err != nil {
		t.Fatal(err)
	}
	return database
}

func TestV2TrackKeyOverrideAndResetPreserveOtherOverrides(t *testing.T) {
	database := newKeyRouteTestDB(t)
	manualBPM := 126.0
	gridID := "song:grid-v1"
	if err := database.UpsertTrackAnalysisOverride(db.TrackAnalysisOverride{
		SongID: "song", BPM: &manualBPM, BPMLocked: true,
		BeatgridArtifactID: &gridID, BeatgridLocked: true,
	}); err != nil {
		t.Fatal(err)
	}
	handler := (&API{db: database}).V2Routes()
	put := httptest.NewRecorder()
	handler.ServeHTTP(put, httptest.NewRequest(http.MethodPut, "/analysis/song/key", strings.NewReader(`{"tonic":9,"mode":"minor"}`)))
	if put.Code != http.StatusOK {
		t.Fatalf("PUT key = %d, want 200: %s", put.Code, put.Body.String())
	}
	var saved TrackAnalysisFeatureResponse
	if err := json.NewDecoder(put.Body).Decode(&saved); err != nil {
		t.Fatal(err)
	}
	if saved.Key == nil || *saved.Key != "A minor" || saved.KeyTonic == nil || *saved.KeyTonic != 9 || saved.KeyMode == nil || *saved.KeyMode != "minor" || saved.KeySource != db.EffectiveKeyManual || saved.MeasuredKeyTonic == nil || *saved.MeasuredKeyTonic != 0 || saved.MeasuredKeyMode == nil || *saved.MeasuredKeyMode != "major" {
		t.Fatalf("saved feature = %#v, want manual A minor", saved)
	}
	reset := httptest.NewRecorder()
	handler.ServeHTTP(reset, httptest.NewRequest(http.MethodDelete, "/analysis/song/key", nil))
	if reset.Code != http.StatusOK {
		t.Fatalf("DELETE key = %d, want 200: %s", reset.Code, reset.Body.String())
	}
	var measured TrackAnalysisFeatureResponse
	if err := json.NewDecoder(reset.Body).Decode(&measured); err != nil {
		t.Fatal(err)
	}
	if measured.Key == nil || *measured.Key != "C major" || measured.KeySource != db.EffectiveKeyMeasured || measured.KeyTonic == nil || *measured.KeyTonic != 0 {
		t.Fatalf("reset feature = %#v, want measured C major", measured)
	}
	override, err := database.GetTrackAnalysisOverride("song")
	if err != nil || override.KeyLocked || override.KeyTonic != nil || override.KeyMode != nil || override.BPM == nil || *override.BPM != manualBPM || !override.BPMLocked || override.BeatgridArtifactID == nil || *override.BeatgridArtifactID != gridID || !override.BeatgridLocked {
		t.Fatalf("reset override = %#v, err = %v; key reset should preserve BPM and beat-grid edits", override, err)
	}
}

func TestV2TrackKeyRejectsInvalidValuesAndUnknownFields(t *testing.T) {
	database := newKeyRouteTestDB(t)
	handler := (&API{db: database}).V2Routes()
	for _, body := range []string{
		`{"mode":"major"}`,
		`{"tonic":null,"mode":"major"}`,
		`{"tonic":-1,"mode":"major"}`,
		`{"tonic":12,"mode":"major"}`,
		`{"tonic":4,"mode":"dorian"}`,
		`{"tonic":4,"mode":"major","locked":false}`,
	} {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPut, "/analysis/song/key", strings.NewReader(body)))
		if recorder.Code != http.StatusBadRequest {
			t.Errorf("PUT key %s = %d, want 400: %s", body, recorder.Code, recorder.Body.String())
		}
	}
}

func TestV2TrackKeyRequiresAnalysis(t *testing.T) {
	database, err := db.New(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := database.SaveSong(&db.Song{ID: "song", Title: "Song", Artist: "Artist", Album: "Album", FilePath: "song.mp3", AddedAt: 1}); err != nil {
		t.Fatal(err)
	}
	handler := (&API{db: database}).V2Routes()
	for _, request := range []struct{ method, path, body string }{
		{http.MethodPut, "/analysis/song/key", `{"tonic":0,"mode":"major"}`},
		{http.MethodDelete, "/analysis/song/key", ""},
	} {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(request.method, request.path, strings.NewReader(request.body)))
		if recorder.Code != http.StatusNotFound {
			t.Errorf("%s %s = %d, want 404: %s", request.method, request.path, recorder.Code, recorder.Body.String())
		}
	}
}
