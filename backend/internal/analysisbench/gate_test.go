package analysisbench

import "testing"

func TestEvaluatePhase0GateDoesNotPassIncompleteEvidence(t *testing.T) {
	manifest := CorpusManifest{Version: "phase0-v1", EvidenceClass: EvidenceSyntheticCI, Tracks: []CorpusTrack{{
		ID: "track", Path: "local.mp3", License: "private-local", LabelSource: "manual beatgrid", Genre: "house",
		Coverage: []string{stableElectronicCoverageTag}, Split: SplitHeldOut, ExpectedBPM: float64Ptr(128), ExpectedKey: "C major",
	}}}
	candidate := ResultSet{Algorithm: "go-track", Results: []DetectorResult{{ID: "track", BPM: float64Ptr(128), Key: "C major", TempoConfidence: float64Ptr(.9), KeyConfidence: float64Ptr(.9)}}}
	baseline := ResultSet{Algorithm: "browser", Results: []DetectorResult{{ID: "track", BPM: float64Ptr(128), Key: "C major", Confidence: float64Ptr(.9)}}}
	report, err := EvaluatePhase0Gate(manifest, candidate, baseline, DeterminismReport{})
	if err != nil {
		t.Fatal(err)
	}
	if report.Passed || report.Decision == "" || len(report.Tripwires) != 11 {
		t.Fatalf("gate report = %#v, want an explicit non-passing decision", report)
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
}

func deterministicResult(platform string, bpm float64) ResultSet {
	return ResultSet{
		Algorithm:  "go-track",
		Results:    []DetectorResult{{ID: "track", BPM: float64Ptr(bpm), Key: "C major", TempoConfidence: float64Ptr(.8), KeyConfidence: float64Ptr(.7)}},
		Throughput: &ThroughputMetrics{Environment: BenchmarkEnvironment{OS: platform}},
	}
}
