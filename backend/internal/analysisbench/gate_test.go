package analysisbench

import (
	"math"
	"path/filepath"
	"testing"
)

func TestEvaluatePhase0GateDoesNotPassIncompleteEvidence(t *testing.T) {
	manifest := CorpusManifest{Version: "phase0-v1", EvidenceClass: EvidenceSyntheticCI, Tracks: []CorpusTrack{{
		ID: "track", Path: "local.mp3", License: "private-local", LabelSource: "manual beatgrid", Genre: "house",
		Coverage: []string{stableElectronicCoverageTag}, Split: SplitHeldOut, ExpectedBPM: float64Ptr(128), ExpectedKey: "C major",
	}}}
	candidate := ResultSet{Algorithm: "go-track", Results: []DetectorResult{{ID: "track", BPM: float64Ptr(128), Key: "C major", TempoConfidence: float64Ptr(.9), KeyConfidence: float64Ptr(.9)}}}
	candidate.Configuration = []byte(`{"Tempo":{"Method":"multifeature-half-bpm"}}`)
	baseline := ResultSet{Algorithm: "browser", Results: []DetectorResult{{ID: "track", BPM: float64Ptr(128), Key: "C major", Confidence: float64Ptr(.9)}}}
	report, err := EvaluatePhase0Gate(manifest, candidate, baseline, DeterminismReport{})
	if err != nil {
		t.Fatal(err)
	}
	if report.Passed || report.Decision == "" || len(report.Tripwires) != 11 {
		t.Fatalf("gate report = %#v, want an explicit non-passing decision", report)
	}
	if string(report.CandidateConfiguration) != string(candidate.Configuration) {
		t.Fatal("gate report lost candidate configuration")
	}
	foundUnproven := false
	for _, tripwire := range report.Tripwires {
		foundUnproven = foundUnproven || tripwire.Status == "unproven"
	}
	if !foundUnproven {
		t.Fatalf("tripwires = %#v, want missing evidence reported as unproven", report.Tripwires)
	}
}

func TestEvaluateDeterminismRequiresThreeMatchingPlatforms(t *testing.T) {
	base := deterministicResult("windows", 128)
	matchingMac := deterministicResult("darwin", 128)
	matchingLinux := deterministicResult("linux", 128)
	if report := EvaluateDeterminism([]ResultSet{base, matchingMac, matchingLinux}); report.Status != "pass" {
		t.Fatalf("matching platforms = %#v, want pass", report)
	}
	differentLinux := deterministicResult("linux", 127.9)
	if report := EvaluateDeterminism([]ResultSet{base, matchingMac, differentLinux}); report.Status != "fail" || len(report.Differences) == 0 {
		t.Fatalf("different platforms = %#v, want mismatch failure", report)
	}
	if report := EvaluateDeterminism([]ResultSet{base, matchingMac}); report.Status != "unproven" {
		t.Fatalf("two platforms = %#v, want unproven", report)
	}
	if report := EvaluateDeterminism([]ResultSet{base, differentLinux}); report.Status != "fail" || len(report.Differences) == 0 {
		t.Fatalf("missing macOS hid a measured disagreement: %+v", report)
	}
}

func TestWritePhase0GateReportDoesNotOverwrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gate.json")
	report := Phase0GateReport{Decision: "do not open Phase 5"}
	if err := WritePhase0GateReport(path, report); err != nil {
		t.Fatalf("WritePhase0GateReport() error = %v", err)
	}
	if err := WritePhase0GateReport(path, report); err == nil {
		t.Fatal("WritePhase0GateReport() unexpectedly overwrote an existing report")
	}
}

func deterministicResult(platform string, bpm float64) ResultSet {
	return ResultSet{
		Algorithm:  "go-track",
		Results:    []DetectorResult{{ID: "track", BPM: float64Ptr(bpm), Key: "C major", TempoConfidence: float64Ptr(.8), KeyConfidence: float64Ptr(.7)}},
		Throughput: &ThroughputMetrics{Environment: BenchmarkEnvironment{OS: platform}},
	}
}

func TestDeterminismChecksConfigurationAndAllRecordedScalars(t *testing.T) {
	base := deterministicResult("windows", 128)
	base.Configuration = []byte(`{"Tempo":{"Method":"a","MinBPM":90}}`)
	mac := deterministicResult("darwin", 128)
	mac.Configuration = []byte(`{"Tempo":{"MinBPM":90.0,"Method":"a"}}`)
	linux := deterministicResult("linux", 128)
	linux.Configuration = base.Configuration
	if report := EvaluateDeterminism([]ResultSet{base, mac, linux}); report.Status != "pass" {
		t.Fatalf("equivalent configurations rejected: %+v", report)
	}
	for _, mutate := range []func(*ResultSet){
		func(r *ResultSet) { r.Configuration = []byte(`{"Tempo":{"Method":"b"}}`) },
		func(r *ResultSet) { r.Configuration = nil },
		func(r *ResultSet) { r.Results[0].AlternateBPM = float64Ptr(64) },
		func(r *ResultSet) { r.Results[0].TempoStability = float64Ptr(.8) },
		func(r *ResultSet) { r.Results[0].TempoCrestFactor = float64Ptr(15) },
		func(r *ResultSet) { r.Results[0].KeyFlatness = float64Ptr(.7) },
		func(r *ResultSet) { r.Results[0].Status = "partial" },
		func(r *ResultSet) { r.Results[0].BPM = float64Ptr(math.NaN()) },
		func(r *ResultSet) { r.Results[0].TempoStability = float64Ptr(math.NaN()) },
		func(r *ResultSet) { r.Results[0].TempoCrestFactor = float64Ptr(math.Inf(1)) },
		func(r *ResultSet) { r.Results[0].KeyFlatness = float64Ptr(2) },
		func(r *ResultSet) { r.Results = nil },
	} {
		changed := deterministicResult("linux", 128)
		changed.Configuration = base.Configuration
		mutate(&changed)
		if report := EvaluateDeterminism([]ResultSet{base, mac, changed}); report.Status != "fail" {
			t.Fatalf("mismatch passed: %+v", report)
		}
	}
}
