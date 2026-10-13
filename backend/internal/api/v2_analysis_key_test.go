// Tests and fixtures for v2 analysis key behavior.

package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/ajbergh/viib-mediahub/internal/analysis"
	"github.com/ajbergh/viib-mediahub/internal/db"
)

func newKeyRouteTestDB(t *testing.T) *db.DB {
	t.Helper()
	a, _ := newBPMRouteTestAPI(t, false)
	database := a.db
	source, err := analysis.ResolveLocalSource(database, "song")
	if err != nil {
		t.Fatal(err)
	}
	measuredTonic := 0
	measuredMode, measuredSource := "major", "measured"
	if err := database.UpsertTrackAnalysis(db.TrackAnalysis{
		SongID: "song", Status: db.TrackAnalysisComplete, AnalysisVersion: 1,
		AlgorithmVersion: "test-v1", SourceFingerprint: source.Fingerprint,
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
	fingerprint := getBPMSourceFingerprint(t, handler)
	putRequest := httptest.NewRequest(http.MethodPut, "/analysis/song/key", strings.NewReader(`{"tonic":9,"mode":"minor"}`))
	putRequest.Header.Set("If-Match", strconv.Quote(fingerprint))
	handler.ServeHTTP(put, putRequest)
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
	resetRequest := httptest.NewRequest(http.MethodDelete, "/analysis/song/key", nil)
	resetRequest.Header.Set("If-Match", strconv.Quote(fingerprint))
	handler.ServeHTTP(reset, resetRequest)
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

func TestV2TrackKeyRequiresSourcePrecondition(t *testing.T) {
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
		if recorder.Code != http.StatusPreconditionRequired {
			t.Errorf("%s %s = %d, want 428: %s", request.method, request.path, recorder.Code, recorder.Body.String())
		}
	}
}

func TestV2TrackKeyEditsRequireCurrentSourceAndHideReplacedValues(t *testing.T) {
	database := newKeyRouteTestDB(t)
	handler := (&API{db: database}).V2Routes()
	fingerprint := getBPMSourceFingerprint(t, handler)
	for _, test := range []struct {
		match  string
		status int
	}{{"", http.StatusPreconditionRequired}, {`"old-source"`, http.StatusPreconditionFailed}} {
		r := httptest.NewRequest(http.MethodPut, "/analysis/song/key", strings.NewReader(`{"tonic":9,"mode":"minor"}`))
		r.Header.Set("If-Match", test.match)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != test.status {
			t.Fatalf("precondition %q: %d", test.match, w.Code)
		}
	}
	save := func(fp string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPut, "/analysis/song/key", strings.NewReader(`{"tonic":9,"mode":"minor"}`))
		r.Header.Set("If-Match", strconv.Quote(fp))
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	if w := save(fingerprint); w.Code != http.StatusOK || w.Header().Get("ETag") != strconv.Quote(fingerprint) {
		t.Fatalf("save key: %d %s", w.Code, w.Body.String())
	}
	o, err := database.GetTrackAnalysisOverride("song")
	if err != nil || o.KeySourceFingerprint != fingerprint {
		t.Fatal("key source binding not stored")
	}
	song, err := database.GetSongByID("song")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(song.FilePath, []byte("replacement with different audio bytes and size"), 0600); err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/analysis/song", nil))
	var feature TrackAnalysisFeatureResponse
	if err := json.NewDecoder(w.Body).Decode(&feature); err != nil {
		t.Fatal(err)
	}
	if feature.Key != nil || feature.KeySource != "unknown" || feature.MeasuredKeyTonic != nil || feature.SourceFingerprint == fingerprint || feature.SourceFingerprint == "" {
		t.Fatalf("stale key exposed: %+v", feature)
	}
	if w := save(fingerprint); w.Code != http.StatusPreconditionFailed {
		t.Fatal("stale save accepted")
	}
	r := httptest.NewRequest(http.MethodDelete, "/analysis/song/key", nil)
	r.Header.Set("If-Match", strconv.Quote(fingerprint))
	reset := httptest.NewRecorder()
	handler.ServeHTTP(reset, r)
	if reset.Code != http.StatusPreconditionFailed {
		t.Fatal("stale reset accepted")
	}
	o, _ = database.GetTrackAnalysisOverride("song")
	if o.KeySourceFingerprint != fingerprint || !o.KeyLocked {
		t.Fatal("stale edit changed prior override")
	}
	record, _ := database.GetTrackAnalysis("song")
	if validTransitionKey(record, o, feature.SourceFingerprint) {
		t.Fatal("stale key entered transition scoring")
	}
	if w := save(feature.SourceFingerprint); w.Code != http.StatusOK {
		t.Fatalf("new-source manual key rejected: %d", w.Code)
	}
}

func TestV2KeyCanBeVerifiedBeforeAnalysis(t *testing.T) {
	a, _ := newBPMRouteTestAPI(t, false)
	handler := a.V2Routes()
	detail := httptest.NewRecorder()
	handler.ServeHTTP(detail, httptest.NewRequest(http.MethodGet, "/analysis/song", nil))
	var feature TrackAnalysisFeatureResponse
	if detail.Code != http.StatusOK || json.NewDecoder(detail.Body).Decode(&feature) != nil || feature.SourceFingerprint == "" || feature.Status != "not_analyzed" {
		t.Fatal("manual key editor cannot load current source before analysis")
	}
	r := httptest.NewRequest(http.MethodPut, "/analysis/song/key", strings.NewReader(`{"tonic":0,"mode":"major"}`))
	r.Header.Set("If-Match", strconv.Quote(feature.SourceFingerprint))
	saved := httptest.NewRecorder()
	handler.ServeHTTP(saved, r)
	var manual TrackAnalysisFeatureResponse
	if saved.Code != http.StatusOK || json.NewDecoder(saved.Body).Decode(&manual) != nil || manual.KeySource != "manual" || manual.KeyTonic == nil || *manual.KeyTonic != 0 || manual.Status != "not_analyzed" {
		t.Fatal("pre-analysis manual key rejected")
	}
}
