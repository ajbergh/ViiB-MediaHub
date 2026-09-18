package analysisbench

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"testing"
)

func TestCompareSeparatesStrictMetricalHalfDoubleAndUnknownTempo(t *testing.T) {
	manifest := CorpusManifest{
		Version: "phase0-v1", EvidenceClass: EvidenceSyntheticCI,
		Tracks: []CorpusTrack{
			track("strict", SplitHeldOut, 128, []float64{64, 128}, "C major"),
			track("half-double", SplitHeldOut, 140, []float64{140}, "A minor"),
			track("unknown", SplitHeldOut, 100, nil, "D minor"),
			{ID: "must-be-unknown", Path: "fixture:must-be-unknown", License: "generated", LabelSource: "fixture generator", Genre: "silence", Split: SplitHeldOut, ExpectedUnknown: true},
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
	if report.Unknown.Labeled != 1 || report.Unknown.Correct != 0 || report.Unknown.Incorrect != 1 {
		t.Fatalf("unknown report = %+v", report.Unknown)
	}
	if report.Corpus.Phase0Ready || report.Corpus.Tracks != 5 || report.Corpus.HeldOutTracks != 4 {
		t.Fatalf("corpus coverage = %+v, want a visible non-ready small corpus", report.Corpus)
	}
}

func TestManifestForSplitRetainsOnlyReservedTracks(t *testing.T) {
	manifest := CorpusManifest{Version: "phase0-v1", EvidenceClass: EvidenceSyntheticCI, Tracks: []CorpusTrack{
		track("tuning", SplitTuning, 128, nil, "C major"),
		track("held-out", SplitHeldOut, 126, nil, "A minor"),
	}}
	filtered, err := ManifestForSplit(manifest, SplitTuning)
	if err != nil {
		t.Fatal(err)
	}
	if len(filtered.Tracks) != 1 || filtered.Tracks[0].ID != "tuning" || filtered.EvidenceClass != manifest.EvidenceClass {
		t.Fatalf("filtered manifest = %#v", filtered)
	}
	if _, err := ManifestForSplit(manifest, "all"); err == nil {
		t.Fatal("unsupported split was accepted")
	}
}

func TestExpectedUnknownRequiresObservedRefusal(t *testing.T) {
	manifest := CorpusManifest{Version: "test", EvidenceClass: EvidenceSyntheticCI, Tracks: []CorpusTrack{{
		ID: "silence", Path: "fixture:silence", License: "generated", LabelSource: "generator", Genre: "silence", Split: SplitHeldOut, ExpectedUnknown: true,
	}}}
	for _, tc := range []struct {
		name    string
		results []DetectorResult
		correct int
	}{
		{"missing", nil, 0},
		{"decode failure", []DetectorResult{{ID: "silence", Status: "failed", Error: "decode_failed"}}, 0},
		{"failed without code", []DetectorResult{{ID: "silence", Status: "failed"}}, 0},
		{"pending", []DetectorResult{{ID: "silence", Status: "pending"}}, 0},
		{"error without status", []DetectorResult{{ID: "silence", Error: "source_unavailable"}}, 0},
		{"error message without code", []DetectorResult{{ID: "silence", ErrorMessage: "decoder failed"}}, 0},
		{"refusal with error message", []DetectorResult{{ID: "silence", Status: "unknown", ErrorMessage: "decoder failed"}}, 0},
		{"empty complete", []DetectorResult{{ID: "silence", Status: "complete"}}, 0},
		{"empty partial", []DetectorResult{{ID: "silence", Status: "partial"}}, 0},
		{"refusal", []DetectorResult{{ID: "silence", Status: "unknown"}}, 1},
		{"browser refusal", []DetectorResult{{ID: "silence", Key: "unknown"}}, 1},
		{"invented BPM", []DetectorResult{{ID: "silence", BPM: float64Ptr(120)}}, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			report, err := Compare(manifest, ResultSet{Algorithm: "test", Results: tc.results}, SplitHeldOut)
			if err != nil || report.Unknown.Correct != tc.correct || report.Unknown.Incorrect != 1-tc.correct {
				t.Fatalf("unknown=%+v err=%v", report.Unknown, err)
			}
		})
	}
}

func TestValidateBenchmarkConfigurationAndAlternate(t *testing.T) {
	for _, configuration := range []string{`null`, `[]`, `{}`, `{"Tempo":`} {
		if err := (ResultSet{Algorithm: "test", Configuration: json.RawMessage(configuration)}).Validate(); err == nil {
			t.Fatalf("invalid configuration accepted: %s", configuration)
		}
	}
	for _, alternate := range []float64{0, -1, math.NaN(), math.Inf(1)} {
		result := ResultSet{Algorithm: "test", Results: []DetectorResult{{ID: "track", BPM: float64Ptr(128), AlternateBPM: float64Ptr(alternate)}}}
		if err := result.Validate(); err == nil {
			t.Fatalf("invalid alternate accepted: %v", alternate)
		}
	}
	if err := (ResultSet{Algorithm: "test", Results: []DetectorResult{{ID: "track", AlternateBPM: float64Ptr(64)}}}).Validate(); err == nil {
		t.Fatal("alternate without primary accepted")
	}
}

func TestManifestAndResultsValidationRejectSilentBenchmarkCorruption(t *testing.T) {
	validTrack := track("same", SplitHeldOut, 128, nil, "Db major")
	manifest := CorpusManifest{Version: "phase0-v1", EvidenceClass: EvidenceSyntheticCI, Tracks: []CorpusTrack{validTrack, validTrack}}
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
	manifest := CorpusManifest{Version: "phase0-v1", EvidenceClass: EvidenceSyntheticCI, Tracks: []CorpusTrack{track("fixture", SplitHeldOut, 128, nil, "F# minor")}}
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

func TestCompareReportsDimensionSpecificConfidenceCalibration(t *testing.T) {
	manifest := CorpusManifest{Version: "phase0-v1", EvidenceClass: EvidenceSyntheticCI, Tracks: []CorpusTrack{
		track("low", SplitHeldOut, 100, nil, "C major"),
		track("medium", SplitHeldOut, 110, nil, "D major"),
		track("high", SplitHeldOut, 120, nil, "E major"),
	}}
	results := ResultSet{Algorithm: "candidate", Results: []DetectorResult{
		{ID: "low", BPM: float64Ptr(80), Key: "C major", TempoConfidence: float64Ptr(.2), KeyConfidence: float64Ptr(.2)},
		{ID: "medium", BPM: float64Ptr(110), Key: "D major", TempoConfidence: float64Ptr(.5), KeyConfidence: float64Ptr(.5)},
		{ID: "high", BPM: float64Ptr(120), Key: "E major", TempoConfidence: float64Ptr(.9), KeyConfidence: float64Ptr(.9)},
	}}
	report, err := Compare(manifest, results, SplitHeldOut)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Calibration.Tempo.Monotonic || !report.Calibration.Key.Monotonic {
		t.Fatalf("calibration = %#v, want three populated monotonic buckets", report.Calibration)
	}
	if report.Calibration.Tempo.Buckets[0].Correct != 0 || report.Calibration.Tempo.Buckets[2].Correct != 1 {
		t.Fatalf("tempo buckets = %#v", report.Calibration.Tempo.Buckets)
	}
}

func TestCoverageRequiresEveryRoadmapCorpusCase(t *testing.T) {
	manifest := readyCorpusManifest()
	coverage := Coverage(manifest, SplitHeldOut)
	if !coverage.Phase0Ready || len(coverage.MissingRequiredCoverage) != 0 {
		t.Fatalf("coverage = %#v, want Phase 0 ready corpus", coverage)
	}
	for index := range manifest.Tracks {
		if len(manifest.Tracks[index].Coverage) == 1 && manifest.Tracks[index].Coverage[0] == RequiredCorpusCoverage()[0] {
			manifest.Tracks[index].Coverage = nil
		}
	}
	coverage = Coverage(manifest, SplitHeldOut)
	if coverage.Phase0Ready || len(coverage.MissingRequiredCoverage) != 1 || coverage.MissingRequiredCoverage[0] != RequiredCorpusCoverage()[0] {
		t.Fatalf("coverage = %#v, want the missing required category visible", coverage)
	}
}

func readyCorpusManifest() CorpusManifest {
	required := RequiredCorpusCoverage()
	manifest := CorpusManifest{Version: "phase0-v1", EvidenceClass: EvidenceLawfulRealAudio, Tracks: make([]CorpusTrack, 0, 200)}
	for index := 0; index < 200; index++ {
		split := SplitTuning
		if index < 67 {
			split = SplitHeldOut
		}
		manifest.Tracks = append(manifest.Tracks, CorpusTrack{
			ID: fmt.Sprintf("track-%03d", index), Path: fmt.Sprintf("local-%03d.mp3", index), License: "private-local",
			RecordingGroup: fmt.Sprintf("recording-%03d", index),
			LabelSource:    "authoritative label", Genre: "benchmark", Coverage: []string{required[index%len(required)]},
			Split: split, ExpectedBPM: float64Ptr(120),
		})
	}
	return manifest
}

func TestCoverageRejectsDuplicateRecordingsAndSplitLeakage(t *testing.T) {
	manifest := readyCorpusManifest()
	manifest.Tracks[100].RecordingGroup = manifest.Tracks[0].RecordingGroup
	coverage := Coverage(manifest, SplitHeldOut)
	if coverage.Phase0Ready || coverage.RecordingIdentity.CrossSplitGroups != 1 || coverage.RecordingIdentity.DistinctGroups != 199 || len(coverage.RecordingIdentity.RepeatedGroups) != 1 {
		t.Fatalf("cross-split overlap not caught: %+v", coverage)
	}
	if manifest.Tracks[100].Split != SplitTuning || manifest.Tracks[0].Split != SplitHeldOut {
		t.Fatal("audit changed frozen split assignments")
	}
	manifest = readyCorpusManifest()
	manifest.Tracks[1].RecordingGroup = manifest.Tracks[0].RecordingGroup
	coverage = Coverage(manifest, SplitHeldOut)
	if coverage.Phase0Ready || coverage.RecordingIdentity.CrossSplitGroups != 0 || len(coverage.RecordingIdentity.RepeatedGroups) != 1 {
		t.Fatalf("same-split duplicates incorrectly treated as independent: %+v", coverage)
	}
	manifest = readyCorpusManifest()
	manifest.Tracks[0].RecordingGroup = ""
	coverage = Coverage(manifest, SplitHeldOut)
	if coverage.Phase0Ready || coverage.RecordingIdentity.UnidentifiedTracks != 1 {
		t.Fatalf("missing identity silently accepted: %+v", coverage)
	}
}

func track(id, split string, bpm float64, accepted []float64, key string) CorpusTrack {
	return CorpusTrack{
		ID: id, Path: "fixture:" + id, License: "generated", LabelSource: "fixture generator", Genre: "electronic", Split: split,
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
