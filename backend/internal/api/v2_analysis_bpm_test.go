package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/ajbergh/viib-mediahub/internal/analysis"
	"github.com/ajbergh/viib-mediahub/internal/analysis/beatgrid"
	"github.com/ajbergh/viib-mediahub/internal/db"
)

func newBPMRouteTestAPI(t *testing.T, includeAnalysis bool) (*API, string) {
	t.Helper()
	database, err := db.New(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	mediaPath := filepath.Join(t.TempDir(), "song.wav")
	if err := os.WriteFile(mediaPath, []byte("current media source"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := database.SaveSong(&db.Song{ID: "song", Title: "Song", Artist: "Artist", Album: "Album", FilePath: mediaPath, AddedAt: 1}); err != nil {
		t.Fatal(err)
	}
	if includeAnalysis {
		resolved, err := analysis.ResolveLocalSource(database, "song")
		if err != nil {
			t.Fatal(err)
		}
		measuredBPM := 128.375
		measuredSource := "measured"
		if err := database.UpsertTrackAnalysis(db.TrackAnalysis{
			SongID: "song", Status: db.TrackAnalysisComplete, AnalysisVersion: 1, AlgorithmVersion: "tempo-test",
			SourceFingerprint: resolved.Fingerprint, BPM: &measuredBPM, BPMSource: &measuredSource,
		}); err != nil {
			t.Fatal(err)
		}
	}
	return &API{db: database}, mediaPath
}

func getBPMSourceFingerprint(t *testing.T, handler http.Handler) string {
	t.Helper()
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/analysis/song/bpm", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("GET BPM = %d: %s", recorder.Code, recorder.Body.String())
	}
	var response TrackAnalysisFeatureResponse
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}
	if response.SourceFingerprint == "" {
		t.Fatal("GET BPM returned no current source fingerprint")
	}
	if recorder.Header().Get("ETag") != strconv.Quote(response.SourceFingerprint) {
		t.Fatalf("GET BPM ETag = %q, want %q", recorder.Header().Get("ETag"), strconv.Quote(response.SourceFingerprint))
	}
	return response.SourceFingerprint
}

func TestV2TrackBPMOverrideAndResetPreserveOtherOverrides(t *testing.T) {
	api, _ := newBPMRouteTestAPI(t, true)
	manualBPM := 127.625
	manualKeyTonic, manualKeyMode := 7, "minor"
	gridID := "song:" + beatgrid.AlgorithmVersion
	gridData, err := (beatgrid.Grid{Beats: []float64{0.125, 0.625, 1.125}, DownbeatIndices: []int{0}}).Encode()
	if err != nil {
		t.Fatal(err)
	}
	if err := api.db.UpsertTrackAnalysisArtifact(db.TrackAnalysisArtifact{
		ID: gridID, SongID: "song", Kind: beatgrid.ArtifactKind, FormatVersion: beatgrid.FormatVersion,
		AlgorithmVersion: beatgrid.AlgorithmVersion, Encoding: beatgrid.Encoding, Provenance: string(beatgrid.ProvenanceManual), Data: gridData,
	}); err != nil {
		t.Fatal(err)
	}
	if err := api.db.UpsertTrackAnalysisOverride(db.TrackAnalysisOverride{
		SongID: "song", KeyTonic: &manualKeyTonic, KeyMode: &manualKeyMode, KeyLocked: true,
		BeatgridArtifactID: &gridID, BeatgridLocked: true,
	}); err != nil {
		t.Fatal(err)
	}
	handler := api.V2Routes()
	fingerprint := getBPMSourceFingerprint(t, handler)
	put := httptest.NewRecorder()
	putRequest := httptest.NewRequest(http.MethodPut, "/analysis/song/bpm", strings.NewReader(`{"bpm":127.625}`))
	putRequest.Header.Set("If-Match", strconv.Quote(fingerprint))
	handler.ServeHTTP(put, putRequest)
	if put.Code != http.StatusOK {
		t.Fatalf("PUT BPM = %d: %s", put.Code, put.Body.String())
	}
	var saved TrackAnalysisFeatureResponse
	if err := json.NewDecoder(put.Body).Decode(&saved); err != nil {
		t.Fatal(err)
	}
	if saved.BPM == nil || *saved.BPM != manualBPM || saved.BPMSource != db.EffectiveBPMManual || saved.SourceFingerprint != fingerprint || saved.BPMConfidence != nil {
		t.Fatalf("saved feature = %#v, want fractional manual BPM without inherited confidence", saved)
	}
	grid, err := api.db.GetTrackAnalysisArtifact("song", beatgrid.ArtifactKind, beatgrid.FormatVersion, beatgrid.AlgorithmVersion)
	if err != nil {
		t.Fatal(err)
	}
	if grid.ID != gridID || string(grid.Data) != string(gridData) || grid.Provenance != string(beatgrid.ProvenanceManual) {
		t.Fatalf("BPM-only update changed beat-grid artifact: %#v", grid)
	}
	override, err := api.db.GetTrackAnalysisOverride("song")
	if err != nil || !override.BPMLocked || override.BPM == nil || *override.BPM != manualBPM || override.KeyTonic == nil || *override.KeyTonic != manualKeyTonic || !override.KeyLocked || override.BeatgridArtifactID == nil || *override.BeatgridArtifactID != gridID || !override.BeatgridLocked {
		t.Fatalf("BPM update did not preserve other overrides: %#v, %v", override, err)
	}
	reset := httptest.NewRecorder()
	resetRequest := httptest.NewRequest(http.MethodDelete, "/analysis/song/bpm", nil)
	resetRequest.Header.Set("If-Match", strconv.Quote(fingerprint))
	handler.ServeHTTP(reset, resetRequest)
	if reset.Code != http.StatusOK {
		t.Fatalf("DELETE BPM = %d: %s", reset.Code, reset.Body.String())
	}
	var measured TrackAnalysisFeatureResponse
	if err := json.NewDecoder(reset.Body).Decode(&measured); err != nil {
		t.Fatal(err)
	}
	if measured.BPM == nil || *measured.BPM != 128.375 || measured.BPMSource != db.EffectiveBPMMeasured {
		t.Fatalf("reset feature = %#v, want measured BPM", measured)
	}
	override, err = api.db.GetTrackAnalysisOverride("song")
	if err != nil || override.BPMLocked || override.BPM != nil || override.KeyTonic == nil || *override.KeyTonic != manualKeyTonic || !override.KeyLocked || override.BeatgridArtifactID == nil || *override.BeatgridArtifactID != gridID || !override.BeatgridLocked {
		t.Fatalf("BPM reset did not preserve key/grid overrides: %#v, %v", override, err)
	}
}

func TestV2TrackBPMCanBeSetBeforeAnalysisAndRequiresCurrentSource(t *testing.T) {
	api, mediaPath := newBPMRouteTestAPI(t, false)
	handler := api.V2Routes()
	fingerprint := getBPMSourceFingerprint(t, handler)
	manual := 132.25
	for _, request := range []struct {
		name   string
		bpm    string
		match  string
		status int
	}{
		{name: "missing precondition", bpm: `{"bpm":132.25}`, status: http.StatusPreconditionRequired},
		{name: "stale source", bpm: `{"bpm":132.25}`, match: `"previous-revision"`, status: http.StatusPreconditionFailed},
		{name: "missing BPM", bpm: `{}`, match: strconv.Quote(fingerprint), status: http.StatusBadRequest},
		{name: "non-positive BPM", bpm: `{"bpm":0}`, match: strconv.Quote(fingerprint), status: http.StatusBadRequest},
		{name: "too-large BPM", bpm: `{"bpm":1000.1}`, match: strconv.Quote(fingerprint), status: http.StatusBadRequest},
		{name: "unknown field", bpm: `{"bpm":132.25,"beats":[]}`, match: strconv.Quote(fingerprint), status: http.StatusBadRequest},
	} {
		t.Run(request.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			put := httptest.NewRequest(http.MethodPut, "/analysis/song/bpm", strings.NewReader(request.bpm))
			if request.match != "" {
				put.Header.Set("If-Match", request.match)
			}
			handler.ServeHTTP(recorder, put)
			if recorder.Code != request.status {
				t.Fatalf("PUT BPM = %d, want %d: %s", recorder.Code, request.status, recorder.Body.String())
			}
		})
	}
	if _, err := api.db.GetTrackAnalysisOverride("song"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("invalid or stale BPM request changed the override: %v", err)
	}
	put := httptest.NewRecorder()
	putRequest := httptest.NewRequest(http.MethodPut, "/analysis/song/bpm", strings.NewReader(`{"bpm":132.25}`))
	putRequest.Header.Set("If-Match", strconv.Quote(fingerprint))
	handler.ServeHTTP(put, putRequest)
	if put.Code != http.StatusOK {
		t.Fatalf("PUT BPM without analysis = %d: %s", put.Code, put.Body.String())
	}
	var saved TrackAnalysisFeatureResponse
	if err := json.NewDecoder(put.Body).Decode(&saved); err != nil {
		t.Fatal(err)
	}
	if saved.SongID != "song" || saved.Status != "not_analyzed" || saved.BPM == nil || *saved.BPM != manual || saved.BPMSource != db.EffectiveBPMManual {
		t.Fatalf("pre-analysis BPM response = %#v", saved)
	}
	if err := os.WriteFile(mediaPath, []byte("changed source revision"), 0o600); err != nil {
		t.Fatal(err)
	}
	staleReset := httptest.NewRecorder()
	staleRequest := httptest.NewRequest(http.MethodDelete, "/analysis/song/bpm", nil)
	staleRequest.Header.Set("If-Match", strconv.Quote(fingerprint))
	handler.ServeHTTP(staleReset, staleRequest)
	if staleReset.Code != http.StatusPreconditionFailed {
		t.Fatalf("DELETE BPM after source change = %d, want 412: %s", staleReset.Code, staleReset.Body.String())
	}
	override, err := api.db.GetTrackAnalysisOverride("song")
	if err != nil || override.BPM == nil || *override.BPM != manual || !override.BPMLocked {
		t.Fatalf("stale reset changed BPM override: %#v, %v", override, err)
	}
}
