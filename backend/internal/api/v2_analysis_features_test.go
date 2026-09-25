package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/analysis"
	"github.com/ajbergh/viib-mediahub/internal/analysis/beatgrid"
	"github.com/ajbergh/viib-mediahub/internal/analysis/features"
	"github.com/ajbergh/viib-mediahub/internal/db"
)

func TestV2TrackTempoEvidenceUsesMeasuredValuesOnly(t *testing.T) {
	bpm, alternate, stability, confidence := 128.5, 64.25, .8, .36
	source, kind := "measured", "dynamic-candidate"
	record := db.TrackAnalysis{Status: db.TrackAnalysisComplete, SourceFingerprint: "source-v1", BPM: &bpm, BPMSource: &source, BPMAltCandidate: &alternate, TempoStability: &stability, BPMConfidence: &confidence, TempoKind: &kind}
	result := trackAnalysisFeatureResponseWithCurrentSource(record, db.TrackAnalysisOverride{}, "source-v1")
	if result.BPMAltCandidate == nil || *result.BPMAltCandidate != alternate || result.TempoStability == nil || *result.TempoStability != stability || result.TempoKind == nil || *result.TempoKind != kind {
		t.Fatalf("missing measured tempo evidence: %#v", result)
	}
	result = trackAnalysisFeatureResponseWithCurrentSource(record, db.TrackAnalysisOverride{BPM: &bpm, BPMSourceFingerprint: "source-v1", BPMLocked: true}, "source-v1")
	if result.BPMAltCandidate != nil || result.TempoStability != nil || result.BPMConfidence != nil || result.TempoKind != nil {
		t.Fatalf("measured evidence attributed to manual tempo: %#v", result)
	}
}

func saveAnalysisTestSong(t *testing.T, database *db.DB, id, title string, genres []string, addedAt, lastPlayed int64) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), id+".mp3")
	if err := os.WriteFile(path, []byte("test audio source for "+id), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := database.SaveSong(&db.Song{ID: id, Title: title, Artist: "Artist", Album: "Album", Genre: genres, FilePath: path, AddedAt: addedAt, LastPlayed: lastPlayed}); err != nil {
		t.Fatal(err)
	}
	source, err := analysis.ResolveLocalSource(database, id)
	if err != nil {
		t.Fatal(err)
	}
	return source.Fingerprint
}

func TestV2TrackAnalysisExposesOnlyCurrentSettledEnergyLevel(t *testing.T) {
	level, confidence, version := 8, .8, features.EnergyLevelAlgorithmVersion
	analysis := db.TrackAnalysis{SongID: "song", Status: db.TrackAnalysisPartial, SourceFingerprint: "source-v1", EnergyLevel: &level, EnergyLevelConfidence: &confidence, EnergyAlgorithmVersion: &version}
	response := trackAnalysisFeatureResponseWithCurrentSource(analysis, db.TrackAnalysisOverride{}, "source-v1")
	if response.EnergyLevel == nil || *response.EnergyLevel != level || response.EnergyLevelConfidence == nil || *response.EnergyLevelConfidence != confidence || response.EnergyAlgorithmVersion == nil || *response.EnergyAlgorithmVersion != version {
		t.Fatalf("energy score/version missing from API response: %#v", response)
	}
	analysis.Status = db.TrackAnalysisRunning
	if running := trackAnalysisFeatureResponseWithCurrentSource(analysis, db.TrackAnalysisOverride{}, "source-v1"); running.EnergyLevel != nil {
		t.Fatalf("running analysis exposed stale Energy Level: %#v", running)
	}
	analysis.Status = db.TrackAnalysisComplete
	staleVersion := "energy-level-v0"
	analysis.EnergyAlgorithmVersion = &staleVersion
	if stale := trackAnalysisFeatureResponseWithCurrentSource(analysis, db.TrackAnalysisOverride{}, "source-v1"); stale.EnergyLevel != nil {
		t.Fatalf("stale algorithm score was exposed: %#v", stale)
	}
}

func TestV2TrackAnalysisFeatureResolvesManualValues(t *testing.T) {
	database, err := db.New(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	currentFingerprint := saveAnalysisTestSong(t, database, "song", "Song", nil, 1, 0)
	measuredBPM, measuredTonic := 128.25, 0
	measuredMode, measuredSource := "major", "measured"
	if err := database.UpsertTrackAnalysis(db.TrackAnalysis{
		SongID: "song", Status: db.TrackAnalysisComplete, AnalysisVersion: 1, AlgorithmVersion: "test-v1", SourceFingerprint: currentFingerprint,
		BPM: &measuredBPM, BPMSource: &measuredSource, KeyTonic: &measuredTonic, KeyMode: &measuredMode, KeySource: &measuredSource,
	}); err != nil {
		t.Fatal(err)
	}
	manualBPM, manualTonic := 127.5, 9
	manualMode := "minor"
	if err := database.UpsertTrackAnalysisOverride(db.TrackAnalysisOverride{
		SongID: "song", BPM: &manualBPM, BPMSourceFingerprint: currentFingerprint, BPMLocked: true, KeyTonic: &manualTonic, KeyMode: &manualMode, KeyLocked: true,
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
	analyzedAt := int64(1720000000123)
	mode, source := "major", "measured"
	for _, id := range []string{"second", "first"} {
		if err := database.UpsertTrackAnalysis(db.TrackAnalysis{SongID: id, Status: db.TrackAnalysisComplete, AnalysisVersion: 1, AlgorithmVersion: "test-v1", SourceFingerprint: id, AnalyzedAt: &analyzedAt, BPM: &bpm, BPMSource: &source, KeyTonic: &tonic, KeyMode: &mode, KeySource: &source}); err != nil {
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
	if len(response) != 2 || response[0].SongID != "first" || response[1].SongID != "second" || response[0].CamelotKey == nil || *response[0].CamelotKey != "8B" || response[0].AnalyzedAt == nil || *response[0].AnalyzedAt != analyzedAt {
		t.Fatalf("feature list = %#v, want sorted resolved records", response)
	}
}

func TestV2TrackAnalysisFeatureListReportsOnlyCurrentSettledStructure(t *testing.T) {
	database, err := db.New(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	const analyzedAt = int64(1000)
	base, err := (features.Result{
		Energy:   []features.EnergyPoint{{Time: 0, Value: .5}},
		Sections: []features.Section{{Start: 0, End: 1, Energy: .5, Label: features.StructureUnknown, Confidence: .2}},
	}).Encode()
	if err != nil {
		t.Fatal(err)
	}
	legacy, err := (features.Result{
		Energy:   []features.EnergyPoint{{Time: 0, Value: .5}},
		Sections: []features.Section{{Start: 0, End: 1, Energy: .5}},
	}).Encode()
	if err != nil {
		t.Fatal(err)
	}
	invalid, err := (features.Result{
		Energy:   []features.EnergyPoint{{Time: 0, Value: .5}},
		Sections: []features.Section{{Start: 1, End: 1, Energy: .5, Label: features.StructureUnknown}},
	}).Encode()
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		id                  string
		status              string
		artifact            []byte
		artifactFingerprint string
		legacyFingerprint   bool
		artifactFormat      int
		artifactAlgorithm   string
		changeSource        bool
		removeSource        bool
		withArtifact        bool
		artifactAt          int64
		wantAvailable       bool
	}{
		{id: "complete", status: db.TrackAnalysisComplete, artifact: base, withArtifact: true, artifactAt: analyzedAt + 1, wantAvailable: true},
		{id: "partial", status: db.TrackAnalysisPartial, artifact: base, withArtifact: true, artifactAt: analyzedAt + 1, wantAvailable: true},
		{id: "pending", status: db.TrackAnalysisPending, artifact: base, withArtifact: true, artifactAt: analyzedAt + 1},
		{id: "running", status: db.TrackAnalysisRunning, artifact: base, withArtifact: true, artifactAt: analyzedAt + 1},
		{id: "failed-current-source", status: db.TrackAnalysisFailed, artifact: base, withArtifact: true, artifactAt: analyzedAt + 1, wantAvailable: true},
		{id: "failed-old-source", status: db.TrackAnalysisFailed, artifact: base, artifactFingerprint: "old-source", withArtifact: true, artifactAt: analyzedAt + 1},
		{id: "unsupported", status: db.TrackAnalysisUnsupported, artifact: base, withArtifact: true, artifactAt: analyzedAt + 1},
		{id: "missing", status: db.TrackAnalysisComplete},
		{id: "corrupt", status: db.TrackAnalysisComplete, artifact: []byte("not gzip"), withArtifact: true, artifactAt: analyzedAt + 1},
		{id: "stale-source", status: db.TrackAnalysisComplete, artifact: base, artifactFingerprint: "old-source", withArtifact: true, artifactAt: analyzedAt + 1},
		{id: "source-changed-before-reanalysis", status: db.TrackAnalysisComplete, artifact: base, withArtifact: true, artifactAt: analyzedAt + 1, changeSource: true},
		{id: "source-unavailable", status: db.TrackAnalysisComplete, artifact: base, withArtifact: true, artifactAt: analyzedAt + 1, removeSource: true},
		{id: "legacy-no-fingerprint", status: db.TrackAnalysisComplete, artifact: base, legacyFingerprint: true, withArtifact: true, artifactAt: analyzedAt + 1},
		{id: "legacy-structure", status: db.TrackAnalysisComplete, artifact: legacy, withArtifact: true, artifactAt: analyzedAt + 1},
		{id: "invalid-section", status: db.TrackAnalysisComplete, artifact: invalid, withArtifact: true, artifactAt: analyzedAt + 1},
		{id: "unsupported-version", status: db.TrackAnalysisComplete, artifact: base, artifactFormat: features.FormatVersion + 1, withArtifact: true, artifactAt: analyzedAt + 1},
	}
	for _, test := range cases {
		path := filepath.Join(t.TempDir(), test.id+".mp3")
		if err := os.WriteFile(path, []byte("source audio placeholder"), 0o600); err != nil {
			t.Fatalf("write source %s: %v", test.id, err)
		}
		if err := database.SaveSong(&db.Song{ID: test.id, Title: test.id, Artist: "Artist", Album: "Album", FilePath: path, AddedAt: 1}); err != nil {
			t.Fatalf("save song %s: %v", test.id, err)
		}
		resolvedSource, err := analysis.ResolveLocalSource(database, test.id)
		if err != nil {
			t.Fatalf("resolve source %s: %v", test.id, err)
		}
		analyzed := analyzedAt
		if err := database.UpsertTrackAnalysis(db.TrackAnalysis{SongID: test.id, Status: test.status, AnalysisVersion: 1, AlgorithmVersion: "test-v1", SourceFingerprint: resolvedSource.Fingerprint, AnalyzedAt: &analyzed}); err != nil {
			t.Fatalf("save analysis %s: %v", test.id, err)
		}
		if test.withArtifact {
			fingerprint := test.artifactFingerprint
			formatVersion := test.artifactFormat
			algorithmVersion := test.artifactAlgorithm
			if fingerprint == "" && !test.legacyFingerprint {
				fingerprint = resolvedSource.Fingerprint
			}
			if formatVersion == 0 {
				formatVersion = features.FormatVersion
			}
			if algorithmVersion == "" {
				algorithmVersion = features.AlgorithmVersion
			}
			if err := database.UpsertTrackAnalysisArtifact(db.TrackAnalysisArtifact{
				ID: test.id + ":" + features.AlgorithmVersion, SongID: test.id, Kind: features.ArtifactKind,
				FormatVersion: formatVersion, AlgorithmVersion: algorithmVersion,
				Encoding: features.Encoding, Provenance: "measured", SourceFingerprint: fingerprint, Data: test.artifact, CreatedAt: test.artifactAt,
			}); err != nil {
				t.Fatalf("save artifact %s: %v", test.id, err)
			}
		}
		if test.changeSource {
			if err := os.WriteFile(path, []byte("source audio placeholder with a changed byte length"), 0o600); err != nil {
				t.Fatalf("change source %s: %v", test.id, err)
			}
		}
		if test.removeSource {
			if err := os.Remove(path); err != nil {
				t.Fatalf("remove source %s: %v", test.id, err)
			}
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
	got := make(map[string]bool, len(response))
	for _, feature := range response {
		got[feature.SongID] = feature.StructureAvailable
	}
	for _, test := range cases {
		if got[test.id] != test.wantAvailable {
			t.Errorf("structureAvailable[%s] = %v, want %v", test.id, got[test.id], test.wantAvailable)
		}
	}
}

func TestV2TrackAnalysisFeatureListReportsOnlyCurrentMeasuredBS1770Measurements(t *testing.T) {
	database, err := db.New(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	value := -14.25
	peakValue := -0.42
	valid := features.BS1770Result{
		IntegratedLUFS: &value, TruePeakDBTP: &peakValue, Standard: features.BS1770Standard, Algorithm: features.BS1770AlgorithmVersion,
		LoudnessAlgorithm: features.BS1770LoudnessAlgorithm, TruePeakAlgorithm: features.BS1770TruePeakAlgorithm,
		Layout: "stereo", Weighting: "L=1;R=1", LoudnessStatus: "available", TruePeakStatus: "available",
	}
	silent := valid
	silent.IntegratedLUFS, silent.LoudnessStatus = nil, "silence"
	silent.TruePeakDBTP, silent.TruePeakStatus = nil, "silence"
	short := valid
	short.IntegratedLUFS, short.LoudnessStatus = nil, "insufficient-duration"
	short.TruePeakDBTP, short.TruePeakStatus = nil, "insufficient-duration"
	belowLoudnessGate := valid
	belowLoudnessGate.IntegratedLUFS, belowLoudnessGate.LoudnessStatus = nil, "below-absolute-gate"
	missingTruePeakValue := valid
	missingTruePeakValue.TruePeakDBTP = nil
	unavailableTruePeak := valid
	unavailableTruePeak.TruePeakStatus = "insufficient-duration"
	badMetadata := valid
	badMetadata.Standard = "unknown standard"
	encode := func(result features.BS1770Result) []byte {
		t.Helper()
		data, err := features.EncodeBS1770(result)
		if err != nil {
			t.Fatal(err)
		}
		return data
	}

	cases := []struct {
		id                string
		status            string
		result            features.BS1770Result
		withArtifact      bool
		corrupt           bool
		staleFingerprint  bool
		legacyFingerprint bool
		formatVersion     int
		algorithmVersion  string
		encoding          string
		provenance        string
		changeSource      bool
		removeSource      bool
		wantLufs          bool
		wantTruePeak      bool
	}{
		{id: "complete", status: db.TrackAnalysisComplete, result: valid, withArtifact: true, wantLufs: true, wantTruePeak: true},
		{id: "partial", status: db.TrackAnalysisPartial, result: valid, withArtifact: true, wantLufs: true, wantTruePeak: true},
		{id: "failed-settled", status: db.TrackAnalysisFailed, result: valid, withArtifact: true, wantLufs: true, wantTruePeak: true},
		{id: "true-peak-independent-of-loudness", status: db.TrackAnalysisComplete, result: belowLoudnessGate, withArtifact: true, wantTruePeak: true},
		{id: "missing-true-peak-value", status: db.TrackAnalysisComplete, result: missingTruePeakValue, withArtifact: true, wantLufs: true},
		{id: "unavailable-true-peak-status", status: db.TrackAnalysisComplete, result: unavailableTruePeak, withArtifact: true, wantLufs: true},
		{id: "pending", status: db.TrackAnalysisPending, result: valid, withArtifact: true},
		{id: "running", status: db.TrackAnalysisRunning, result: valid, withArtifact: true},
		{id: "unsupported-analysis", status: db.TrackAnalysisUnsupported, result: valid, withArtifact: true},
		{id: "missing", status: db.TrackAnalysisComplete},
		{id: "corrupt", status: db.TrackAnalysisComplete, withArtifact: true, corrupt: true},
		{id: "unsupported-payload-metadata", status: db.TrackAnalysisComplete, result: badMetadata, withArtifact: true},
		{id: "silent", status: db.TrackAnalysisComplete, result: silent, withArtifact: true},
		{id: "short", status: db.TrackAnalysisComplete, result: short, withArtifact: true},
		{id: "stale-fingerprint", status: db.TrackAnalysisComplete, result: valid, withArtifact: true, staleFingerprint: true},
		{id: "legacy-fingerprint", status: db.TrackAnalysisComplete, result: valid, withArtifact: true, legacyFingerprint: true},
		{id: "unsupported-version", status: db.TrackAnalysisComplete, result: valid, withArtifact: true, formatVersion: features.BS1770FormatVersion + 1},
		{id: "unsupported-algorithm", status: db.TrackAnalysisComplete, result: valid, withArtifact: true, algorithmVersion: "old-bs1770"},
		{id: "wrong-encoding", status: db.TrackAnalysisComplete, result: valid, withArtifact: true, encoding: "json"},
		{id: "wrong-provenance", status: db.TrackAnalysisComplete, result: valid, withArtifact: true, provenance: "unknown"},
		{id: "source-changed", status: db.TrackAnalysisComplete, result: valid, withArtifact: true, changeSource: true},
		{id: "source-unavailable", status: db.TrackAnalysisComplete, result: valid, withArtifact: true, removeSource: true},
	}

	for _, test := range cases {
		fingerprint := saveAnalysisTestSong(t, database, test.id, test.id, nil, 1, 0)
		if err := database.UpsertTrackAnalysis(db.TrackAnalysis{SongID: test.id, Status: test.status, AnalysisVersion: 1, AlgorithmVersion: "test-v1", SourceFingerprint: fingerprint}); err != nil {
			t.Fatalf("save analysis %s: %v", test.id, err)
		}
		if test.withArtifact {
			artifactFingerprint := fingerprint
			if test.staleFingerprint {
				artifactFingerprint = "previous-source"
			}
			if test.legacyFingerprint {
				artifactFingerprint = ""
			}
			formatVersion := test.formatVersion
			if formatVersion == 0 {
				formatVersion = features.BS1770FormatVersion
			}
			algorithmVersion := test.algorithmVersion
			if algorithmVersion == "" {
				algorithmVersion = features.BS1770AlgorithmVersion
			}
			encoding := test.encoding
			if encoding == "" {
				encoding = features.BS1770Encoding
			}
			provenance := test.provenance
			if provenance == "" {
				provenance = "measured"
			}
			data := encode(test.result)
			if test.corrupt {
				data = []byte("not gzip")
			}
			if err := database.UpsertTrackAnalysisArtifact(db.TrackAnalysisArtifact{
				ID: test.id + ":bs1770", SongID: test.id, Kind: features.BS1770ArtifactKind,
				FormatVersion: formatVersion, AlgorithmVersion: algorithmVersion, Encoding: encoding,
				Provenance: provenance, SourceFingerprint: artifactFingerprint, Data: data,
			}); err != nil {
				t.Fatalf("save artifact %s: %v", test.id, err)
			}
		}
		song, err := database.GetSongByID(test.id)
		if err != nil {
			t.Fatal(err)
		}
		if test.changeSource {
			if err := os.WriteFile(song.FilePath, []byte("changed source identity"), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		if test.removeSource {
			if err := os.Remove(song.FilePath); err != nil {
				t.Fatal(err)
			}
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
	type measurements struct{ lufs, truePeak *float64 }
	got := make(map[string]measurements, len(response))
	for _, feature := range response {
		got[feature.SongID] = measurements{lufs: feature.IntegratedLUFSBS1770, truePeak: feature.TruePeakDBTP}
	}
	for _, test := range cases {
		actual := got[test.id].lufs
		if test.wantLufs {
			if actual == nil || math.Abs(*actual-value) > 1e-9 {
				t.Errorf("integratedLufsBs1770[%s] = %v, want %v", test.id, actual, value)
			}
		} else if actual != nil {
			t.Errorf("integratedLufsBs1770[%s] = %v, want unavailable", test.id, *actual)
		}
		actualPeak := got[test.id].truePeak
		if test.wantTruePeak {
			if actualPeak == nil || math.Abs(*actualPeak-peakValue) > 1e-9 {
				t.Errorf("truePeakDbtp[%s] = %v, want %v", test.id, actualPeak, peakValue)
			}
		} else if actualPeak != nil {
			t.Errorf("truePeakDbtp[%s] = %v, want unavailable", test.id, *actualPeak)
		}
	}
}

func TestV2BeatGridUpdateRoundTripsAndLocksWithoutLosingManualValues(t *testing.T) {
	database, err := db.New(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	mediaPath := filepath.Join(t.TempDir(), "song.wav")
	if err := os.WriteFile(mediaPath, []byte("grid edit source"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := database.SaveSong(&db.Song{ID: "song", Title: "Song", Artist: "Artist", Album: "Album", FilePath: mediaPath, AddedAt: 1}); err != nil {
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
	fingerprint := saveAnalysisTestSong(t, database, "song", "Song", nil, 1, 0)
	if err := database.UpsertTrackAnalysis(db.TrackAnalysis{SongID: "song", Status: db.TrackAnalysisComplete, AnalysisVersion: 1, AlgorithmVersion: "test-v1", SourceFingerprint: fingerprint}); err != nil {
		t.Fatal(err)
	}
	downbeat := .0
	result := features.Result{IntegratedLUFS: -12.4, TruePeakDBFS: -.3, Energy: []features.EnergyPoint{{Time: 0, Value: .2}, {Time: .5, Value: .8}}, Sections: []features.Section{{Start: 0, End: 1, Energy: .5, Label: features.StructureIntro, Confidence: .48, TimingProvenance: features.TimingDownbeatGrid, DownbeatStart: &downbeat}}}
	encoded, err := result.Encode()
	if err != nil {
		t.Fatal(err)
	}
	if err := database.UpsertTrackAnalysisArtifact(db.TrackAnalysisArtifact{ID: "song:energy", SongID: "song", Kind: features.ArtifactKind, FormatVersion: features.FormatVersion, AlgorithmVersion: features.AlgorithmVersion, Encoding: features.Encoding, Provenance: "measured", SourceFingerprint: fingerprint, Data: encoded}); err != nil {
		t.Fatal(err)
	}
	standardsLoudness, standardsPeak := -11.25, -.42
	standards := features.BS1770Result{IntegratedLUFS: &standardsLoudness, TruePeakDBTP: &standardsPeak, Standard: features.BS1770Standard, Algorithm: features.BS1770AlgorithmVersion, LoudnessAlgorithm: features.BS1770LoudnessAlgorithm, TruePeakAlgorithm: features.BS1770TruePeakAlgorithm, Layout: "stereo", Weighting: "L=1;R=1", LoudnessStatus: "available", TruePeakStatus: "available"}
	standardsEncoded, err := features.EncodeBS1770(standards)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.UpsertTrackAnalysisArtifact(db.TrackAnalysisArtifact{ID: "song:" + features.BS1770AlgorithmVersion, SongID: "song", Kind: features.BS1770ArtifactKind, FormatVersion: features.BS1770FormatVersion, AlgorithmVersion: features.BS1770AlgorithmVersion, Encoding: features.BS1770Encoding, Provenance: "measured", SourceFingerprint: fingerprint, Data: standardsEncoded}); err != nil {
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
	if response.AlgorithmVersion != features.AlgorithmVersion || len(response.Energy) != 2 || response.IntegratedLUFS != result.IntegratedLUFS || response.TruePeakDBFS != result.TruePeakDBFS {
		t.Fatalf("energy response = %#v", response)
	}
	if response.LoudnessKind != features.LoudnessKind || response.PeakKind != features.PeakKind || response.ChannelScope != "mono" || response.Standard != "none" {
		t.Fatalf("energy API did not expose qualified measurement metadata: %#v", response)
	}
	if response.IntegratedLUFSBS1770 == nil || *response.IntegratedLUFSBS1770 != standardsLoudness || response.TruePeakDBTP == nil || *response.TruePeakDBTP != standardsPeak || response.LoudnessStandard == nil || *response.LoudnessStandard != features.BS1770Standard || response.LoudnessAlgorithm == nil || *response.LoudnessAlgorithm != features.BS1770LoudnessAlgorithm || response.TruePeakAlgorithm == nil || *response.TruePeakAlgorithm != features.BS1770TruePeakAlgorithm || response.LoudnessLayout == nil || *response.LoudnessLayout != "stereo" || response.LoudnessWeighting == nil || *response.LoudnessWeighting != "L=1;R=1" || response.LoudnessStatus == nil || *response.LoudnessStatus != "available" || response.TruePeakStatus == nil || *response.TruePeakStatus != "available" {
		t.Fatalf("energy API did not expose the separately versioned BS.1770 artifact: %#v", response)
	}
	if response.IntegratedLUFS != result.IntegratedLUFS || response.TruePeakDBFS != result.TruePeakDBFS {
		t.Fatalf("adding the standards artifact changed legacy proxy aliases: %#v", response)
	}
	if len(response.Sections) != 1 || response.Sections[0].Label != features.StructureIntro || response.Sections[0].Confidence != .48 || response.Sections[0].TimingProvenance != features.TimingDownbeatGrid || response.Sections[0].DownbeatStart == nil || *response.Sections[0].DownbeatStart != 0 {
		t.Fatalf("energy API did not serialize structure semantics and timing provenance: %#v", response.Sections)
	}
	standardsArtifact, err := database.GetTrackAnalysisArtifact("song", features.BS1770ArtifactKind, features.BS1770FormatVersion, features.BS1770AlgorithmVersion)
	if err != nil {
		t.Fatal(err)
	}
	standardsArtifact.SourceFingerprint = "previous-source"
	if err := database.UpsertTrackAnalysisArtifact(standardsArtifact); err != nil {
		t.Fatal(err)
	}
	staleStandardsRecorder := httptest.NewRecorder()
	(&API{db: database}).V2Routes().ServeHTTP(staleStandardsRecorder, httptest.NewRequest(http.MethodGet, "/analysis/song/energy", nil))
	var staleStandardsResponse EnergyFeaturesResponse
	if staleStandardsRecorder.Code != http.StatusOK || json.NewDecoder(staleStandardsRecorder.Body).Decode(&staleStandardsResponse) != nil || staleStandardsResponse.IntegratedLUFSBS1770 != nil || staleStandardsResponse.LoudnessStatus != nil {
		t.Fatalf("stale BS.1770 artifact leaked through energy detail: status=%d response=%#v", staleStandardsRecorder.Code, staleStandardsResponse)
	}
	standardsArtifact.SourceFingerprint = fingerprint
	if err := database.UpsertTrackAnalysisArtifact(standardsArtifact); err != nil {
		t.Fatal(err)
	}
	independentPeak := -.8
	monoMeasurement := features.BS1770Result{TruePeakDBTP: &independentPeak, Standard: features.BS1770Standard, Algorithm: features.BS1770AlgorithmVersion, LoudnessAlgorithm: features.BS1770LoudnessAlgorithm, TruePeakAlgorithm: features.BS1770TruePeakAlgorithm, Layout: "mono", Weighting: "M=1", LoudnessStatus: "below-absolute-gate", TruePeakStatus: "available"}
	monoEncoded, err := features.EncodeBS1770(monoMeasurement)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.UpsertTrackAnalysisArtifact(db.TrackAnalysisArtifact{ID: "song:" + features.BS1770AlgorithmVersion, SongID: "song", Kind: features.BS1770ArtifactKind, FormatVersion: features.BS1770FormatVersion, AlgorithmVersion: features.BS1770AlgorithmVersion, Encoding: features.BS1770Encoding, Provenance: "measured", SourceFingerprint: fingerprint, Data: monoEncoded}); err != nil {
		t.Fatal(err)
	}
	monoRecorder := httptest.NewRecorder()
	(&API{db: database}).V2Routes().ServeHTTP(monoRecorder, httptest.NewRequest(http.MethodGet, "/analysis/song/energy", nil))
	var monoResponse EnergyFeaturesResponse
	if monoRecorder.Code != http.StatusOK || json.NewDecoder(monoRecorder.Body).Decode(&monoResponse) != nil || monoResponse.LoudnessLayout == nil || *monoResponse.LoudnessLayout != "mono" || monoResponse.LoudnessWeighting == nil || *monoResponse.LoudnessWeighting != "M=1" || monoResponse.IntegratedLUFSBS1770 != nil || monoResponse.TruePeakDBTP == nil || *monoResponse.TruePeakDBTP != independentPeak {
		t.Fatalf("mono standards artifact/API round trip = status %d, response %#v", monoRecorder.Code, monoResponse)
	}
	song, err := database.GetSongByID("song")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(song.FilePath, []byte("the source file changed after analysis"), 0o600); err != nil {
		t.Fatal(err)
	}
	staleRecorder := httptest.NewRecorder()
	(&API{db: database}).V2Routes().ServeHTTP(staleRecorder, httptest.NewRequest(http.MethodGet, "/analysis/song/energy", nil))
	if staleRecorder.Code != http.StatusConflict {
		t.Fatalf("GET stale energy = %d, want %d: %s", staleRecorder.Code, http.StatusConflict, staleRecorder.Body.String())
	}
}

func TestV2EnergyFeaturesNormalizesNilSlices(t *testing.T) {
	database, err := db.New(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	fingerprint := saveAnalysisTestSong(t, database, "nil-slices", "Nil Slices", nil, 1, 0)
	if err := database.UpsertTrackAnalysis(db.TrackAnalysis{SongID: "nil-slices", Status: db.TrackAnalysisComplete, AnalysisVersion: 1, AlgorithmVersion: "test-v1", SourceFingerprint: fingerprint}); err != nil {
		t.Fatal(err)
	}
	encoded, err := (features.Result{Energy: []features.EnergyPoint{{Time: 0, Value: .5}}}).Encode()
	if err != nil {
		t.Fatal(err)
	}
	if err := database.UpsertTrackAnalysisArtifact(db.TrackAnalysisArtifact{
		ID: "nil-slices:energy", SongID: "nil-slices", Kind: features.ArtifactKind,
		FormatVersion: features.FormatVersion, AlgorithmVersion: features.AlgorithmVersion,
		Encoding: features.Encoding, Provenance: "measured", SourceFingerprint: fingerprint, Data: encoded,
	}); err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()
	(&API{db: database}).V2Routes().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/analysis/nil-slices/energy", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("GET energy = %d: %s", recorder.Code, recorder.Body.String())
	}
	var response EnergyFeaturesResponse
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}
	if response.Energy == nil || response.Sections == nil || response.CueSuggestions == nil {
		t.Fatalf("nil energy response slices: energy=%#v sections=%#v cues=%#v", response.Energy, response.Sections, response.CueSuggestions)
	}
}

func TestV2TransitionRecommendationsExposeMeasuredRationale(t *testing.T) {
	database, err := db.New(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	fingerprints := make(map[string]string)
	for _, id := range []string{"source", "compatible", "incompatible"} {
		genres := []string(nil)
		if id == "compatible" {
			genres = []string{"  rOcK "}
		} else if id == "incompatible" {
			genres = []string{"Rockabilly"}
		}
		fingerprints[id] = saveAnalysisTestSong(t, database, id, id, genres, 1, 0)
	}
	if err := database.SavePlaylist(&db.Playlist{ID: "mix", Name: "Mix", SongIDs: []string{"compatible", "stale-song-id"}, CreatedAt: 1}); err != nil {
		t.Fatal(err)
	}
	if err := database.SavePlaylist(&db.Playlist{ID: "warmup", Name: "Warmup", SongIDs: []string{"incompatible"}, CreatedAt: 2}); err != nil {
		t.Fatal(err)
	}
	results := map[string]features.Result{
		"source":       {IntegratedLUFS: -10, Energy: []features.EnergyPoint{{Value: .2}, {Value: .8}}, Sections: []features.Section{{Start: 0, End: 30, Label: features.StructureIntro, Confidence: .48}, {Start: 30, End: 60, Label: features.StructureOutro, Confidence: .48}}, CueSuggestions: []features.CueSuggestion{{Kind: "mix-out", Confidence: .8}}},
		"compatible":   {IntegratedLUFS: -10.5, Energy: []features.EnergyPoint{{Value: .75}, {Value: .7}}, Sections: []features.Section{{Start: 0, End: 30, Label: features.StructureIntro, Confidence: .48}, {Start: 30, End: 60, Label: features.StructureOutro, Confidence: .48}}, CueSuggestions: []features.CueSuggestion{{Kind: "mix-in", Confidence: .8}}},
		"incompatible": {IntegratedLUFS: -25, Energy: []features.EnergyPoint{{Value: .05}, {Value: .1}}, Sections: []features.Section{{Start: 0, End: 30, Label: features.StructureBreakdown, Confidence: .42}}},
	}
	for id, result := range results {
		encoded, err := result.Encode()
		if err != nil {
			t.Fatal(err)
		}
		if err := database.UpsertTrackAnalysisArtifact(db.TrackAnalysisArtifact{ID: id + ":energy", SongID: id, Kind: features.ArtifactKind, FormatVersion: features.FormatVersion, AlgorithmVersion: features.AlgorithmVersion, Encoding: features.Encoding, Provenance: "measured", SourceFingerprint: fingerprints[id], Data: encoded}); err != nil {
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
		if err := database.UpsertTrackAnalysis(db.TrackAnalysis{SongID: metadata.id, Status: db.TrackAnalysisComplete, AnalysisVersion: 1, AlgorithmVersion: "test-v1", SourceFingerprint: fingerprints[metadata.id], BPM: &bpm, BPMConfidence: &confidence, BPMSource: &measuredSource, KeyTonic: &tonic, KeyMode: &keyMode, KeyConfidence: &confidence, KeySource: &measuredSource, EnergyLevel: &level, EnergyLevelConfidence: &confidence, EnergyAlgorithmVersion: &energyVersion}); err != nil {
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
	if len(response.Recommendations) != 1 || response.Recommendations[0].SongID != "compatible" || len(response.Recommendations[0].Components) != 7 || response.Intent != features.TransitionIntentHold || response.AlgorithmVersion != features.TransitionAlgorithmVersion || response.Recommendations[0].Vector.CamelotRelation != "same" || response.Recommendations[0].Vector.BPMDelta == nil {
		t.Fatalf("recommendations = %#v", response)
	}
	var structureComponent *features.TransitionComponent
	for index := range response.Recommendations[0].Components {
		if response.Recommendations[0].Components[index].Name == "intro-outro-compatibility" {
			structureComponent = &response.Recommendations[0].Components[index]
			break
		}
	}
	if structureComponent == nil || structureComponent.Score != 1 || !strings.Contains(structureComponent.Rationale, "outro to opening incoming intro") {
		t.Fatalf("recommendation omitted explainable structure compatibility: %#v", response.Recommendations[0].Components)
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
	getFiltered := func(query string) TransitionRecommendationsResponse {
		t.Helper()
		recorder := httptest.NewRecorder()
		(&API{db: database}).V2Routes().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/analysis/source/recommendations"+query, nil))
		if recorder.Code != http.StatusOK {
			t.Fatalf("GET library-filtered recommendations = %d: %s", recorder.Code, recorder.Body.String())
		}
		var result TransitionRecommendationsResponse
		if err := json.NewDecoder(recorder.Body).Decode(&result); err != nil {
			t.Fatal(err)
		}
		return result
	}
	libraryFiltered := getFiltered("?playlistId=mix&genre=rock")
	if libraryFiltered.CandidatesBeforeFilters != 2 || libraryFiltered.CandidatesAfterFilters != 1 || len(libraryFiltered.Recommendations) != 1 || libraryFiltered.Recommendations[0].SongID != "compatible" {
		t.Fatalf("AND library filters/counts = %#v", libraryFiltered)
	}
	if libraryFiltered.Filters.PlaylistID == nil || *libraryFiltered.Filters.PlaylistID != "mix" || libraryFiltered.Filters.Genre == nil || *libraryFiltered.Filters.Genre != "Rock" {
		t.Fatalf("library filter echo = %#v", libraryFiltered.Filters)
	}
	multiPlaylist := getFiltered("?playlistIds=mix&playlistIds=warmup")
	if multiPlaylist.CandidatesBeforeFilters != 2 || multiPlaylist.CandidatesAfterFilters != 2 || len(multiPlaylist.Recommendations) != 2 || multiPlaylist.Filters.PlaylistID != nil || len(multiPlaylist.Filters.PlaylistIDs) != 2 {
		t.Fatalf("OR playlist membership = %#v", multiPlaylist)
	}
	playlistAndGenre := getFiltered("?playlistIds=mix&playlistIds=warmup&genre=rock")
	if playlistAndGenre.CandidatesAfterFilters != 1 || len(playlistAndGenre.Recommendations) != 1 || playlistAndGenre.Recommendations[0].SongID != "compatible" {
		t.Fatalf("playlist OR set should remain ANDed with genre: %#v", playlistAndGenre)
	}
	validAndUnknownPlaylist := getFiltered("?playlistIds=unknown&playlistIds=mix")
	if validAndUnknownPlaylist.CandidatesAfterFilters != 1 || validAndUnknownPlaylist.Recommendations[0].SongID != "compatible" {
		t.Fatalf("unknown playlist should add no members to the OR set: %#v", validAndUnknownPlaylist)
	}
	unknownPlaylists := getFiltered("?playlistIds=unknown&playlistIds=missing")
	if unknownPlaylists.CandidatesAfterFilters != 0 || len(unknownPlaylists.Recommendations) != 0 {
		t.Fatalf("unknown playlist IDs should match no candidates: %#v", unknownPlaylists)
	}
	if exactGenre := getFiltered("?genre=rock"); exactGenre.CandidatesAfterFilters != 1 || exactGenre.Recommendations[0].SongID != "compatible" {
		t.Fatalf("exact normalized genre must not substring-match Rockabilly: %#v", exactGenre)
	}
	for _, query := range []string{"?playlistId=unknown", "?genre=Unknown"} {
		if unmatched := getFiltered(query); unmatched.CandidatesBeforeFilters != 2 || unmatched.CandidatesAfterFilters != 0 || len(unmatched.Recommendations) != 0 {
			t.Fatalf("unknown library filter %q should match nothing: %#v", query, unmatched)
		}
	}
	unfiltered := getFiltered("")
	if unfiltered.CandidatesBeforeFilters != 2 || unfiltered.CandidatesAfterFilters != 2 || unfiltered.Filters.PlaylistID != nil || len(unfiltered.Filters.PlaylistIDs) != 0 || unfiltered.Filters.Genre != nil {
		t.Fatalf("omitted library filters changed behavior: %#v", unfiltered)
	}
}

func TestV2TransitionRecommendationFiltersRejectInvalidQueries(t *testing.T) {
	database, err := db.New(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	tooManyPlaylists := make([]string, 21)
	for index := range tooManyPlaylists {
		tooManyPlaylists[index] = "playlistIds=p" + strconv.Itoa(index)
	}
	invalidQueries := []string{
		"minBpm=59", "maxBpm=191", "minBpm=NaN", "minEnergyLevel=0", "maxEnergyLevel=11",
		"minBpm=130&maxBpm=120", "minEnergyLevel=8&maxEnergyLevel=4", "stemsAvailable=yes", "minBpm=120&minBpm=121",
		"camelotCompatible=yes", "camelotCompatible=true&camelotCompatible=false",
		"notRecentlyPlayedHours=", "notRecentlyPlayedHours=abc", "notRecentlyPlayedHours=0", "notRecentlyPlayedHours=-1",
		"notRecentlyPlayedHours=169", "notRecentlyPlayedHours=24.5", "notRecentlyPlayedHours=24&notRecentlyPlayedHours=48",
		"playlistId=", "playlistId=mix&playlistId=other", "playlistIds=", "playlistIds=mix&playlistIds=mix",
		"playlistId=mix&playlistIds=warmup", "genre=", "genre=rock&genre=pop",
	}
	invalidQueries = append(invalidQueries, strings.Join(tooManyPlaylists, "&"))
	for _, query := range invalidQueries {
		t.Run(query, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			(&API{db: database}).V2Routes().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/analysis/source/recommendations?"+query, nil))
			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("status=%d, want 400: %s", recorder.Code, recorder.Body.String())
			}
		})
	}
}

func TestV2NotRecentlyPlayedRecommendationFilter(t *testing.T) {
	database, err := db.New(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	now := time.Now().UnixMilli()
	oldPlayedAt := now - int64((25*time.Hour)/time.Millisecond)
	tracks := []struct {
		id         string
		lastPlayed int64
	}{
		{id: "source"},
		{id: "recent", lastPlayed: now},
		{id: "old", lastPlayed: oldPlayedAt},
		{id: "never"},
	}
	for _, track := range tracks {
		fingerprint := saveAnalysisTestSong(t, database, track.id, track.id, nil, now, track.lastPlayed)
		if err := database.UpsertTrackAnalysis(db.TrackAnalysis{SongID: track.id, Status: db.TrackAnalysisComplete, AnalysisVersion: 1, AlgorithmVersion: "test-v1", SourceFingerprint: fingerprint}); err != nil {
			t.Fatal(err)
		}
		encoded, encodeErr := (features.Result{IntegratedLUFS: -12, Energy: []features.EnergyPoint{{Value: .4}, {Value: .7}}}).Encode()
		if encodeErr != nil {
			t.Fatal(encodeErr)
		}
		if err := database.UpsertTrackAnalysisArtifact(db.TrackAnalysisArtifact{ID: track.id + ":energy", SongID: track.id, Kind: features.ArtifactKind, FormatVersion: features.FormatVersion, AlgorithmVersion: features.AlgorithmVersion, Encoding: features.Encoding, Provenance: "measured", SourceFingerprint: fingerprint, Data: encoded}); err != nil {
			t.Fatal(err)
		}
	}

	get := func(query string) TransitionRecommendationsResponse {
		t.Helper()
		recorder := httptest.NewRecorder()
		(&API{db: database}).V2Routes().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/analysis/source/recommendations"+query, nil))
		if recorder.Code != http.StatusOK {
			t.Fatalf("GET recommendations%s = %d: %s", query, recorder.Code, recorder.Body.String())
		}
		var response TransitionRecommendationsResponse
		if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
			t.Fatal(err)
		}
		return response
	}

	unfiltered := get("")
	if unfiltered.CandidatesBeforeFilters != 3 || unfiltered.CandidatesAfterFilters != 3 || unfiltered.Filters.NotRecentlyPlayedHours != nil {
		t.Fatalf("omitted recency filter changed behavior: %#v", unfiltered)
	}
	for _, recommendation := range unfiltered.Recommendations {
		if recommendation.FilterEvidence.LastPlayed != nil {
			t.Fatalf("omitted recency filter returned lastPlayed evidence: %#v", recommendation.FilterEvidence)
		}
	}

	filtered := get("?notRecentlyPlayedHours=24")
	if filtered.CandidatesBeforeFilters != 3 || filtered.CandidatesAfterFilters != 2 || len(filtered.Recommendations) != 2 {
		t.Fatalf("recency filter counts before=%d after=%d recommendations=%d", filtered.CandidatesBeforeFilters, filtered.CandidatesAfterFilters, len(filtered.Recommendations))
	}
	if filtered.Filters.NotRecentlyPlayedHours == nil || *filtered.Filters.NotRecentlyPlayedHours != 24 {
		t.Fatalf("recency filter was not echoed: %#v", filtered.Filters)
	}
	seen := map[string]TransitionCandidateEvidence{}
	for _, recommendation := range filtered.Recommendations {
		seen[recommendation.SongID] = recommendation.FilterEvidence
	}
	if _, ok := seen["recent"]; ok {
		t.Fatal("candidate completed within the selected period was not excluded")
	}
	if _, ok := seen["old"]; !ok || seen["old"].LastPlayed == nil || *seen["old"].LastPlayed != oldPlayedAt {
		t.Fatalf("old completed-play evidence missing: %#v", seen["old"])
	}
	if _, ok := seen["never"]; !ok || seen["never"].LastPlayed == nil || *seen["never"].LastPlayed != 0 {
		t.Fatalf("never-played evidence missing: %#v", seen["never"])
	}
}

func TestV2CamelotCompatibleRecommendationFilter(t *testing.T) {
	database, err := db.New(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	candidates := []struct {
		id         string
		status     string
		tonic      *int
		mode       *string
		keySource  string
		confidence float64
	}{
		{id: "same", status: db.TrackAnalysisComplete, tonic: intPointer(0), mode: stringPointer("major"), keySource: "measured", confidence: .9},
		{id: "adjacent", status: db.TrackAnalysisComplete, tonic: intPointer(7), mode: stringPointer("major"), keySource: "measured", confidence: .9},
		{id: "relative", status: db.TrackAnalysisComplete, tonic: intPointer(9), mode: stringPointer("minor"), keySource: "measured", confidence: .9},
		{id: "other", status: db.TrackAnalysisComplete, tonic: intPointer(2), mode: stringPointer("major"), keySource: "measured", confidence: .9},
		{id: "missing", status: db.TrackAnalysisComplete},
		{id: "low-confidence", status: db.TrackAnalysisComplete, tonic: intPointer(0), mode: stringPointer("major"), keySource: "measured", confidence: .2},
		{id: "imported", status: db.TrackAnalysisComplete, tonic: intPointer(0), mode: stringPointer("major"), keySource: "imported", confidence: .9},
		{id: "unsettled", status: db.TrackAnalysisPending, tonic: intPointer(0), mode: stringPointer("major"), keySource: "measured", confidence: .9},
	}
	allIDs := make([]string, 0, len(candidates)+1)
	allIDs = append(allIDs, "source")
	for _, candidate := range candidates {
		allIDs = append(allIDs, candidate.id)
	}
	fingerprints := make(map[string]string)
	for _, id := range allIDs {
		fingerprints[id] = saveAnalysisTestSong(t, database, id, id, nil, 1, 0)
		encoded, err := (features.Result{IntegratedLUFS: -12, Energy: []features.EnergyPoint{{Value: .4}, {Value: .7}}}).Encode()
		if err != nil {
			t.Fatal(err)
		}
		if err := database.UpsertTrackAnalysisArtifact(db.TrackAnalysisArtifact{ID: id + ":energy", SongID: id, Kind: features.ArtifactKind, FormatVersion: features.FormatVersion, AlgorithmVersion: features.AlgorithmVersion, Encoding: features.Encoding, Provenance: "measured", SourceFingerprint: fingerprints[id], Data: encoded}); err != nil {
			t.Fatal(err)
		}
	}
	measured := "measured"
	confidence := .9
	if err := database.UpsertTrackAnalysis(db.TrackAnalysis{SongID: "source", Status: db.TrackAnalysisComplete, AnalysisVersion: 1, AlgorithmVersion: "test-v1", SourceFingerprint: fingerprints["source"], KeyTonic: intPointer(0), KeyMode: stringPointer("major"), KeyConfidence: &confidence, KeySource: &measured}); err != nil {
		t.Fatal(err)
	}
	for _, candidate := range candidates {
		keyConfidence, keySource := candidate.confidence, candidate.keySource
		analysis := db.TrackAnalysis{SongID: candidate.id, Status: candidate.status, AnalysisVersion: 1, AlgorithmVersion: "test-v1", SourceFingerprint: fingerprints[candidate.id], KeyTonic: candidate.tonic, KeyMode: candidate.mode}
		if candidate.tonic != nil && candidate.mode != nil {
			analysis.KeyConfidence, analysis.KeySource = &keyConfidence, &keySource
		}
		if err := database.UpsertTrackAnalysis(analysis); err != nil {
			t.Fatalf("save analysis %s: %v", candidate.id, err)
		}
	}
	get := func(query string) TransitionRecommendationsResponse {
		t.Helper()
		recorder := httptest.NewRecorder()
		(&API{db: database}).V2Routes().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/analysis/source/recommendations"+query, nil))
		if recorder.Code != http.StatusOK {
			t.Fatalf("GET recommendations%s = %d: %s", query, recorder.Code, recorder.Body.String())
		}
		var response TransitionRecommendationsResponse
		if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
			t.Fatal(err)
		}
		return response
	}

	filtered := get("?camelotCompatible=true")
	accepted := map[string]bool{}
	for _, recommendation := range filtered.Recommendations {
		accepted[recommendation.SongID] = true
	}
	for _, id := range []string{"same", "adjacent", "relative"} {
		if !accepted[id] {
			t.Errorf("compatible candidate %q was filtered out; recommendations=%v", id, accepted)
		}
	}
	if len(accepted) != 3 {
		t.Errorf("compatible filter accepted %d candidates, want same/adjacent/relative only: %v", len(accepted), accepted)
	}
	eligibleCandidates := len(candidates) - 1 // the pending row is excluded by freshness gating
	if filtered.CandidatesBeforeFilters != eligibleCandidates || filtered.CandidatesAfterFilters != 3 || filtered.Filters.CamelotCompatible == nil || !*filtered.Filters.CamelotCompatible {
		t.Errorf("camelot filter echo/counts = before %d after %d filters %#v", filtered.CandidatesBeforeFilters, filtered.CandidatesAfterFilters, filtered.Filters)
	}
	for _, query := range []string{"", "?camelotCompatible=false"} {
		unfiltered := get(query)
		if unfiltered.CandidatesBeforeFilters != eligibleCandidates || unfiltered.CandidatesAfterFilters != eligibleCandidates || len(unfiltered.Recommendations) != eligibleCandidates {
			t.Errorf("query %q changed unfiltered result/counts: before %d after %d recommendations %d", query, unfiltered.CandidatesBeforeFilters, unfiltered.CandidatesAfterFilters, len(unfiltered.Recommendations))
		}
		if query == "" && unfiltered.Filters.CamelotCompatible != nil {
			t.Errorf("omitted camelotCompatible echoed as %#v", unfiltered.Filters.CamelotCompatible)
		}
		if query != "" && (unfiltered.Filters.CamelotCompatible == nil || *unfiltered.Filters.CamelotCompatible) {
			t.Errorf("false camelotCompatible echo = %#v", unfiltered.Filters.CamelotCompatible)
		}
	}
	staleSong, err := database.GetSongByID("other")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(staleSong.FilePath, []byte("candidate source changed after analysis"), 0o600); err != nil {
		t.Fatal(err)
	}
	withoutStaleCandidate := get("")
	if withoutStaleCandidate.CandidatesBeforeFilters != eligibleCandidates-1 || len(withoutStaleCandidate.Recommendations) != eligibleCandidates-1 {
		t.Fatalf("stale candidate remained eligible: before=%d recommendations=%d", withoutStaleCandidate.CandidatesBeforeFilters, len(withoutStaleCandidate.Recommendations))
	}

	lowSourceConfidence := .2
	if err := database.UpsertTrackAnalysis(db.TrackAnalysis{SongID: "source", Status: db.TrackAnalysisComplete, AnalysisVersion: 1, AlgorithmVersion: "test-v1", SourceFingerprint: fingerprints["source"], KeyTonic: intPointer(0), KeyMode: stringPointer("major"), KeyConfidence: &lowSourceConfidence, KeySource: &measured}); err != nil {
		t.Fatal(err)
	}
	withoutTrustedSource := get("?camelotCompatible=true")
	if withoutTrustedSource.CandidatesAfterFilters != 0 || len(withoutTrustedSource.Recommendations) != 0 {
		t.Fatalf("compatible filter used candidates without a trusted source key: %#v", withoutTrustedSource)
	}
}

func intPointer(value int) *int { return &value }

func stringPointer(value string) *string { return &value }

func TestValidTransitionKeyRejectsMalformedEffectiveKey(t *testing.T) {
	measured, major := "measured", "major"
	invalidTonic := db.TrackAnalysis{Status: db.TrackAnalysisComplete, KeyTonic: intPointer(-1), KeyMode: &major, KeySource: &measured}
	invalidMode := db.TrackAnalysis{Status: db.TrackAnalysisComplete, KeyTonic: intPointer(0), KeyMode: stringPointer("unknown-mode"), KeySource: &measured}
	validTonic := db.TrackAnalysis{Status: db.TrackAnalysisComplete, KeyTonic: intPointer(0), KeyMode: &major, KeySource: &measured}
	inferred := "inferred"
	untrusted := db.TrackAnalysis{Status: db.TrackAnalysisComplete, KeyTonic: intPointer(0), KeyMode: &major, KeySource: &inferred}
	if validTransitionKey(invalidTonic, db.TrackAnalysisOverride{}) {
		t.Fatal("out-of-range effective key was accepted")
	}
	if validTransitionKey(invalidMode, db.TrackAnalysisOverride{}) {
		t.Fatal("unknown effective key mode was accepted")
	}
	if !validTransitionKey(validTonic, db.TrackAnalysisOverride{}) {
		t.Fatal("valid measured effective key was rejected")
	}
	if validTransitionKey(untrusted, db.TrackAnalysisOverride{}) {
		t.Fatal("inferred effective key was accepted")
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
