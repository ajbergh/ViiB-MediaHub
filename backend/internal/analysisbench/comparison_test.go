package analysisbench

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"
)

func TestCompareSeparatesStrictMetricalHalfDoubleAndUnknownTempo(t *testing.T) {
	manifest := CorpusManifest{
		Version: "phase0-v1",
		Tracks: []CorpusTrack{
			track("strict", SplitHeldOut, 128, []float64{64, 128}, "C major"),
			track("half-double", SplitHeldOut, 140, []float64{140}, "A minor"),
			track("unknown", SplitHeldOut, 100, nil, "D minor"),
			{ID: "must-be-unknown", Path: "fixture:must-be-unknown", License: "generated", Genre: "silence", Split: SplitHeldOut, ExpectedUnknown: true},
			track("tuning-only", SplitTuning, 120, nil, "F major"),
		},
	}
	resultSet := ResultSet{Algorithm: "browser-baseline", Results: []DetectorResult{
		{ID: "strict", BPM: float64Ptr(128.2), Key: "C"},
		{ID: "half-double", BPM: float64Ptr(70), Key: "C major"},
		{ID: "tuning-only", BPM: float64Ptr(120), Key: "F"},
	}}

	report, err := Compare(manifest, resultSet, SplitHeldOut)
	if err != nil {
		t.Fatalf("Compare() error = %v", err)
	}
	if report.Tempo.Labeled != 3 || report.Tempo.Reported != 2 || report.Tempo.StrictWithinHalf != 1 || report.Tempo.MetricalMatch != 1 || report.Tempo.HalfDoubleErrors != 1 || report.Tempo.Unknown != 1 {
		t.Fatalf("tempo report = %+v", report.Tempo)
	}
	if report.Tempo.StrictAccuracy == nil || math.Abs(*report.Tempo.StrictAccuracy-1.0/3.0) > 1e-9 {
		t.Fatalf("strict accuracy = %v, want 1/3", report.Tempo.StrictAccuracy)
	}
	if report.Key.Labeled != 3 || report.Key.Reported != 2 || report.Key.Exact != 1 || report.Key.CamelotCompatible != 2 || report.Key.Unknown != 1 {
		t.Fatalf("key report = %+v", report.Key)
	}
	if report.Unknown.Labeled != 1 || report.Unknown.Correct != 1 || report.Unknown.Incorrect != 0 {
		t.Fatalf("unknown report = %+v", report.Unknown)
	}
	if report.Corpus.Phase0Ready || report.Corpus.Tracks != 5 || report.Corpus.HeldOutTracks != 4 {
		t.Fatalf("corpus coverage = %+v, want a visible non-ready small corpus", report.Corpus)
	}
}

func TestManifestAndResultsValidationRejectSilentBenchmarkCorruption(t *testing.T) {
	validTrack := track("same", SplitHeldOut, 128, nil, "Db major")
	manifest := CorpusManifest{Version: "phase0-v1", Tracks: []CorpusTrack{validTrack, validTrack}}
	if err := manifest.Validate(); err == nil {
		t.Fatal("duplicate manifest ID was accepted")
	}
	resultSet := ResultSet{Algorithm: "candidate", Results: []DetectorResult{
		{ID: "same", Key: "C# major"},
		{ID: "same", Key: "C# major"},
	}}
	if err := resultSet.Validate(); err == nil {
		t.Fatal("duplicate result ID was accepted")
	}
}

func TestRequiredGenresIsStablePerSplit(t *testing.T) {
	manifest := CorpusManifest{Tracks: []CorpusTrack{
		track("a", SplitHeldOut, 100, nil, "C major"),
		track("b", SplitHeldOut, 100, nil, "C major"),
		track("c", SplitTuning, 100, nil, "C major"),
	}}
	manifest.Tracks[0].Genre = "techno"
	manifest.Tracks[1].Genre = "house"
	manifest.Tracks[2].Genre = "ambient"
	genres := RequiredGenres(manifest, SplitHeldOut)
	if len(genres) != 2 || genres[0] != "house" || genres[1] != "techno" {
		t.Fatalf("RequiredGenres() = %v, want [house techno]", genres)
	}
}

func TestLoadManifestAndResultSetRoundTripJSON(t *testing.T) {
	directory := t.TempDir()
	manifestPath := filepath.Join(directory, "manifest.json")
	resultsPath := filepath.Join(directory, "results.json")
	manifest := CorpusManifest{Version: "phase0-v1", Tracks: []CorpusTrack{track("fixture", SplitHeldOut, 128, nil, "F# minor")}}
	results := ResultSet{Algorithm: "current-js-export", Results: []DetectorResult{{ID: "fixture", BPM: float64Ptr(128), Key: "Gb minor"}}}
	writeJSON(t, manifestPath, manifest)
	writeJSON(t, resultsPath, results)

	loadedManifest, err := LoadManifest(manifestPath)
	if err != nil {
		t.Fatalf("LoadManifest() error = %v", err)
	}
	loadedResults, err := LoadResultSet(resultsPath)
	if err != nil {
		t.Fatalf("LoadResultSet() error = %v", err)
	}
	report, err := Compare(loadedManifest, loadedResults, SplitHeldOut)
	if err != nil {
		t.Fatalf("Compare() error = %v", err)
	}
	if report.Tempo.StrictWithinHalf != 1 || report.Key.Exact != 1 {
		t.Fatalf("JSON-loaded report = %+v", report)
	}
}

func track(id, split string, bpm float64, accepted []float64, key string) CorpusTrack {
	return CorpusTrack{
		ID: id, Path: "fixture:" + id, License: "generated", Genre: "electronic", Split: split,
		ExpectedBPM: float64Ptr(bpm), AcceptedMetricBPM: accepted, ExpectedKey: key,
	}
}

func float64Ptr(value float64) *float64 { return &value }

func writeJSON(t *testing.T, path string, value any) {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal JSON: %v", err)
	}
	if err := os.WriteFile(path, encoded, 0600); err != nil {
		t.Fatalf("write JSON: %v", err)
	}
}
