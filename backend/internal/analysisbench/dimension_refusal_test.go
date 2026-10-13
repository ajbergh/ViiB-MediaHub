package analysisbench

import "testing"

func TestDimensionRefusalKeepsValidOtherDimensionAndRejectsMissingEvidence(t *testing.T) {
	base := CorpusTrack{ID: "percussion", Path: "fixture.wav", License: "generated", LabelSource: "test", Genre: "percussion", Split: SplitHeldOut, ExpectedBPM: float64Ptr(120), ExpectedKeyUnknown: true}
	manifest := CorpusManifest{Version: "dimension-refusal-v1", EvidenceClass: EvidenceSyntheticCI, Tracks: []CorpusTrack{base}}
	for _, tc := range []struct {
		name    string
		results []DetectorResult
		correct int
	}{
		{"observed partial", []DetectorResult{{ID: base.ID, BPM: float64Ptr(120), Status: "partial"}}, 1},
		{"observed complete", []DetectorResult{{ID: base.ID, BPM: float64Ptr(120), Status: "complete", Key: "unknown"}}, 1},
		{"invented key", []DetectorResult{{ID: base.ID, BPM: float64Ptr(120), Key: "C major"}}, 0},
		{"missing output", nil, 0},
		{"untagged output is not refusal evidence", []DetectorResult{{ID: base.ID, BPM: float64Ptr(120)}}, 0},
		{"decode failure", []DetectorResult{{ID: base.ID, Status: "failed", Error: "decode"}}, 0},
		{"failed without error code", []DetectorResult{{ID: base.ID, Status: "failed"}}, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			report, err := Compare(manifest, ResultSet{Algorithm: "test", Results: tc.results}, SplitHeldOut)
			if err != nil {
				t.Fatal(err)
			}
			if report.KeyRefusal.Labeled != 1 || report.KeyRefusal.Correct != tc.correct || report.KeyRefusal.Incorrect != 1-tc.correct || report.Key.Labeled != 0 || report.Tempo.Labeled != 1 || report.Unknown.Labeled != 0 {
				t.Fatalf("unexpected per-dimension accounting: %+v", report)
			}
		})
	}
	base.ExpectedBPM, base.ExpectedKeyUnknown, base.ExpectedBPMUnknown, base.ExpectedKey = nil, false, true, "C major"
	manifest.Tracks = []CorpusTrack{base}
	report, err := Compare(manifest, ResultSet{Algorithm: "test", Results: []DetectorResult{{ID: base.ID, Key: "C major", Status: "partial"}}}, SplitHeldOut)
	if err != nil || report.TempoRefusal.Correct != 1 || report.Key.Exact != 1 {
		t.Fatalf("key-only refusal counterpart: %+v, %v", report, err)
	}
}

func TestDimensionRefusalTripwireSupplementsOriginalUnknownRequirement(t *testing.T) {
	manifest := CorpusManifest{Version: "test", EvidenceClass: EvidenceSyntheticCI, Tracks: []CorpusTrack{{
		ID: "percussion", Path: "fixture.wav", License: "generated", LabelSource: "test", Genre: "percussion", Split: SplitHeldOut,
		ExpectedBPM: float64Ptr(120), ExpectedKeyUnknown: true,
	}}}
	for _, key := range []string{"", "C major"} {
		candidate := ResultSet{Algorithm: "test", Results: []DetectorResult{{ID: "percussion", BPM: float64Ptr(120), Key: key, Status: "partial"}}}
		report, err := EvaluatePhase0Gate(manifest, candidate, candidate, DeterminismReport{})
		if err != nil {
			t.Fatal(err)
		}
		foundLegacy, foundKey := false, false
		for _, wire := range report.Tripwires {
			if wire.Metric == "Deliberately unclassifiable fixtures returning unknown" {
				foundLegacy = wire.Status == "unproven"
			}
			if wire.Metric == "Explicit key refusal labels returning unknown" {
				foundKey = wire.Status == map[bool]string{true: "pass", false: "fail"}[key == ""]
			}
		}
		if !foundLegacy || !foundKey || report.Passed {
			t.Fatalf("new per-field metric replaced or weakened original gate: %+v", report)
		}
	}
}

func TestManifestRejectsConflictingDimensionRefusalLabels(t *testing.T) {
	base := CorpusTrack{ID: "one", Path: "fixture.wav", License: "generated", LabelSource: "test", Genre: "test", Split: SplitHeldOut}
	for _, mutate := range []func(*CorpusTrack){
		func(track *CorpusTrack) { track.ExpectedBPM = float64Ptr(120); track.ExpectedBPMUnknown = true },
		func(track *CorpusTrack) { track.ExpectedKey = "C major"; track.ExpectedKeyUnknown = true },
		func(track *CorpusTrack) { track.ExpectedUnknown = true; track.ExpectedKeyUnknown = true },
	} {
		entry := base
		mutate(&entry)
		if (CorpusManifest{Version: "test", EvidenceClass: EvidenceSyntheticCI, Tracks: []CorpusTrack{entry}}).Validate() == nil {
			t.Fatal("conflicting labels accepted")
		}
	}
}

func TestFailedScalarOutputsCannotInflateAccuracyOrCalibration(t *testing.T) {
	manifest := CorpusManifest{Version: "test", EvidenceClass: EvidenceSyntheticCI, Tracks: []CorpusTrack{{
		ID: "one", Path: "fixture.wav", License: "generated", LabelSource: "test", Genre: "test", Split: SplitHeldOut,
		ExpectedBPM: float64Ptr(120), ExpectedKey: "C major",
	}}}
	for _, status := range []string{"failed", "unsupported", "canceled", "source_unavailable", "complete-with-error"} {
		result := DetectorResult{ID: "one", BPM: float64Ptr(120), Key: "C major", TempoConfidence: float64Ptr(.95), KeyConfidence: float64Ptr(.95), Status: status}
		if status == "complete-with-error" {
			result.Status = "complete"
			result.Error = "decode_failed"
		}
		report, err := Compare(manifest, ResultSet{Algorithm: "test", Results: []DetectorResult{result}}, SplitHeldOut)
		if err != nil {
			t.Fatal(err)
		}
		if report.Tempo.StrictWithinHalf != 0 || report.Tempo.Unknown != 1 || report.Key.Exact != 0 || report.Key.Unknown != 1 || report.Calibration.Tempo.Buckets[2].Labeled != 0 || report.Calibration.Key.Buckets[2].Labeled != 0 {
			t.Fatalf("%s failed values counted as valid evidence: %+v", status, report)
		}
	}
}
