package analysisbench

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

const stableElectronicCoverageTag = "stable-electronic"

// Phase0GateReport is the recorded go/no-go evidence required before Phase 5.
// It evaluates the provisional §14.6 thresholds without silently turning
// missing evidence into a pass.
type Phase0GateReport struct {
	Split           string            `json:"split"`
	Candidate       string            `json:"candidate"`
	BrowserBaseline string            `json:"browserBaseline"`
	Corpus          CorpusCoverage    `json:"corpus"`
	Tripwires       []GateTripwire    `json:"tripwires"`
	Determinism     DeterminismReport `json:"determinism"`
	Passed          bool              `json:"passed"`
	Decision        string            `json:"decision"`
}

// GateTripwire has an explicit status so an incomplete corpus or absent
// benchmark cannot look like a passing result.
type GateTripwire struct {
	Metric   string `json:"metric"`
	Target   string `json:"target"`
	Observed string `json:"observed"`
	Status   string `json:"status"` // pass, fail, or unproven
	Reason   string `json:"reason,omitempty"`
}

// DeterminismReport compares the same corpus output across required desktop
// platforms. Scalars must be within 1e-6 and categorical outputs must match.
type DeterminismReport struct {
	RequiredPlatforms []string                `json:"requiredPlatforms"`
	ObservedPlatforms []string                `json:"observedPlatforms"`
	Differences       []DeterminismDifference `json:"differences,omitempty"`
	Status            string                  `json:"status"` // pass, fail, or unproven
	Reason            string                  `json:"reason,omitempty"`
}

type DeterminismDifference struct {
	Algorithm string `json:"algorithm"`
	TrackID   string `json:"trackId,omitempty"`
	Field     string `json:"field"`
	Expected  string `json:"expected"`
	Actual    string `json:"actual"`
}

// EvaluatePhase0Gate measures the candidate against the held-out corpus and
// browser baseline. A caller must supply a cross-platform DeterminismReport;
// its zero value intentionally keeps the gate unproven.
func EvaluatePhase0Gate(manifest CorpusManifest, candidate, browserBaseline ResultSet, determinism DeterminismReport) (Phase0GateReport, error) {
	if err := manifest.Validate(); err != nil {
		return Phase0GateReport{}, err
	}
	candidateReport, err := Compare(manifest, candidate, SplitHeldOut)
	if err != nil {
		return Phase0GateReport{}, fmt.Errorf("compare candidate: %w", err)
	}
	baselineReport, err := Compare(manifest, browserBaseline, SplitHeldOut)
	if err != nil {
		return Phase0GateReport{}, fmt.Errorf("compare browser baseline: %w", err)
	}
	report := Phase0GateReport{
		Split: SplitHeldOut, Candidate: candidate.Algorithm, BrowserBaseline: browserBaseline.Algorithm,
		Corpus: candidateReport.Corpus, Determinism: determinism,
	}

	stableManifest := stableElectronicManifest(manifest)
	stableCandidate, err := Compare(stableManifest, candidate, SplitHeldOut)
	if err != nil {
		return Phase0GateReport{}, fmt.Errorf("compare stable-electronic candidate: %w", err)
	}
	report.Tripwires = append(report.Tripwires,
		minimumRateTripwire("BPM strict (±0.5) on stable electronic material", "≥ 95%", stableCandidate.Tempo.StrictAccuracy, .95, "tag held-out stable electronic tracks with coverage: stable-electronic"),
		minimumRateTripwire("BPM strict (±0.5) across full stratified corpus", "≥ 85%", candidateReport.Tempo.StrictAccuracy, .85, "requires held-out BPM labels"),
		maximumRateTripwire("BPM half/double error rate", "≤ 5%", candidateReport.Tempo.HalfDoubleRate, .05, "requires held-out BPM labels"),
		improvementTripwire("BPM strict improvement over browser baseline", "≥ 20 percentage points", candidateReport.Tempo.StrictAccuracy, baselineReport.Tempo.StrictAccuracy, .20),
		minimumRateTripwire("Key exact tonic+mode", "≥ 65%", candidateReport.Key.ExactAccuracy, .65, "requires held-out key labels"),
		minimumRateTripwire("Key Camelot-compatible", "≥ 85%", candidateReport.Key.CompatibleRate, .85, "requires held-out key labels"),
		improvementTripwire("Key exact improvement over browser baseline", "≥ 10 percentage points", candidateReport.Key.ExactAccuracy, baselineReport.Key.ExactAccuracy, .10),
		calibrationTripwire(candidateReport.Calibration),
		minimumRateTripwire("Deliberately unclassifiable fixtures returning unknown", "≥ 90%", candidateReport.Unknown.Accuracy, .90, "requires expectedUnknown entries in the held-out split"),
		throughputTripwire(throughputForSplit(manifest, candidate, SplitHeldOut)),
		determinismTripwire(determinism),
	)
	report.Passed = report.Corpus.Phase0Ready
	for _, tripwire := range report.Tripwires {
		if tripwire.Status != "pass" {
			report.Passed = false
		}
	}
	report.Decision = "do not open Phase 5: one or more Phase 0 tripwires failed or remain unproven"
	if report.Passed {
		report.Decision = "Phase 0 evidence gate passed; record the reviewed go/no-go decision before opening Phase 5"
	}
	return report, nil
}

func stableElectronicManifest(manifest CorpusManifest) CorpusManifest {
	filtered := CorpusManifest{Version: manifest.Version, EvidenceClass: manifest.EvidenceClass}
	for _, track := range manifest.Tracks {
		if track.Split != SplitHeldOut || track.ExpectedBPM == nil || !hasCoverageTag(track, stableElectronicCoverageTag) {
			continue
		}
		filtered.Tracks = append(filtered.Tracks, track)
	}
	// Compare validates its input, and an empty manifest is invalid. Preserve a
	// harmless labeled track only when no explicit stable-electronic evidence
	// exists so the tripwire can be reported as unproven rather than erroring.
	if len(filtered.Tracks) == 0 {
		filtered.Tracks = []CorpusTrack{{ID: "missing-stable-electronic-evidence", Path: "evidence:missing", License: "not-applicable", LabelSource: "gate placeholder", Genre: "missing", Split: SplitHeldOut, ExpectedUnknown: true}}
	}
	return filtered
}

func hasCoverageTag(track CorpusTrack, want string) bool {
	if normalizeCoverageTag(track.Genre) == want {
		return true
	}
	for _, tag := range track.Coverage {
		if normalizeCoverageTag(tag) == want {
			return true
		}
	}
	return false
}

func minimumRateTripwire(metric, target string, actual *float64, minimum float64, missingReason string) GateTripwire {
	if actual == nil {
		return GateTripwire{Metric: metric, Target: target, Status: "unproven", Reason: missingReason}
	}
	status := "fail"
	if *actual >= minimum {
		status = "pass"
	}
	return GateTripwire{Metric: metric, Target: target, Observed: formatRate(*actual), Status: status}
}

func maximumRateTripwire(metric, target string, actual *float64, maximum float64, missingReason string) GateTripwire {
	if actual == nil {
		return GateTripwire{Metric: metric, Target: target, Status: "unproven", Reason: missingReason}
	}
	status := "fail"
	if *actual <= maximum {
		status = "pass"
	}
	return GateTripwire{Metric: metric, Target: target, Observed: formatRate(*actual), Status: status}
}

func improvementTripwire(metric, target string, candidate, baseline *float64, minimum float64) GateTripwire {
	if candidate == nil || baseline == nil {
		return GateTripwire{Metric: metric, Target: target, Status: "unproven", Reason: "requires candidate and browser-baseline held-out measurements"}
	}
	improvement := *candidate - *baseline
	status := "fail"
	if improvement >= minimum {
		status = "pass"
	}
	return GateTripwire{Metric: metric, Target: target, Observed: formatRate(improvement), Status: status}
}

func calibrationTripwire(calibration ConfidenceCalibration) GateTripwire {
	if !calibration.Tempo.Monotonic || !calibration.Key.Monotonic {
		return GateTripwire{Metric: "Confidence calibration", Target: "monotonic across ≥ 3 buckets", Status: "unproven", Reason: "both tempo and key require populated, monotonic low/medium/high buckets"}
	}
	return GateTripwire{Metric: "Confidence calibration", Target: "monotonic across ≥ 3 buckets", Observed: "tempo and key monotonic", Status: "pass"}
}

func throughputTripwire(metrics *ThroughputMetrics) GateTripwire {
	if metrics == nil || metrics.TotalRealtimeMultiple == nil {
		return GateTripwire{Metric: "One-worker decode + BPM + key throughput", Target: "≥ 5× real-time", Status: "unproven", Reason: "requires decoded held-out audio timing"}
	}
	status := "fail"
	if *metrics.TotalRealtimeMultiple >= 5 {
		status = "pass"
	}
	return GateTripwire{Metric: "One-worker decode + BPM + key throughput", Target: "≥ 5× real-time", Observed: fmt.Sprintf("%.2fx real-time", *metrics.TotalRealtimeMultiple), Status: status}
}

func determinismTripwire(report DeterminismReport) GateTripwire {
	if report.Status == "pass" {
		return GateTripwire{Metric: "Windows/macOS/Linux determinism", Target: "scalars within 1e-6", Observed: strings.Join(report.ObservedPlatforms, ", "), Status: "pass"}
	}
	reason := report.Reason
	if reason == "" {
		reason = "requires equivalent results from Windows, macOS, and Linux"
	}
	return GateTripwire{Metric: "Windows/macOS/Linux determinism", Target: "scalars within 1e-6", Status: report.Status, Reason: reason}
}

func throughputForSplit(manifest CorpusManifest, resultSet ResultSet, split string) *ThroughputMetrics {
	if resultSet.Throughput == nil {
		return nil
	}
	tracks := make(map[string]CorpusTrack, len(manifest.Tracks))
	for _, track := range manifest.Tracks {
		tracks[track.ID] = track
	}
	metrics := &ThroughputMetrics{Environment: resultSet.Throughput.Environment}
	for _, timing := range resultSet.Throughput.Tracks {
		track, exists := tracks[timing.ID]
		if !exists || track.Split != split {
			continue
		}
		metrics.Tracks = append(metrics.Tracks, timing)
		metrics.AudioSeconds += timing.AudioSeconds
		metrics.WallSeconds += timing.WallSeconds
		metrics.DecodeAndStreamSeconds += timing.DecodeAndStreamSeconds
		metrics.DSPSeconds += timing.DSPSeconds
	}
	metrics.TotalRealtimeMultiple = realtimeMultiple(metrics.AudioSeconds, metrics.WallSeconds)
	metrics.DecodeRealtimeMultiple = realtimeMultiple(metrics.AudioSeconds, metrics.DecodeAndStreamSeconds)
	metrics.DSPRealtimeMultiple = realtimeMultiple(metrics.AudioSeconds, metrics.DSPSeconds)
	return metrics
}

// EvaluateDeterminism validates one result set per required platform. It does
// not claim the files came from those platforms; their recorded environment is
// retained in the raw output for reviewer audit.
func EvaluateDeterminism(resultSets []ResultSet) DeterminismReport {
	report := DeterminismReport{RequiredPlatforms: []string{"windows", "darwin", "linux"}, Status: "unproven"}
	if len(resultSets) == 0 {
		report.Reason = "no platform result sets provided"
		return report
	}
	platforms := make(map[string]struct{})
	for _, resultSet := range resultSets {
		if resultSet.Throughput != nil && resultSet.Throughput.Environment.OS != "" {
			platforms[resultSet.Throughput.Environment.OS] = struct{}{}
		}
	}
	for platform := range platforms {
		report.ObservedPlatforms = append(report.ObservedPlatforms, platform)
	}
	sort.Strings(report.ObservedPlatforms)
	for _, required := range report.RequiredPlatforms {
		if _, present := platforms[required]; !present {
			report.Reason = "requires result sets from Windows, macOS (darwin), and Linux"
			return report
		}
	}
	reference := resultSets[0]
	for _, actual := range resultSets[1:] {
		compareDeterminism(&report, reference, actual)
	}
	if len(report.Differences) > 0 {
		report.Status = "fail"
		report.Reason = "detector outputs differ across platforms"
		return report
	}
	report.Status = "pass"
	return report
}

func compareDeterminism(report *DeterminismReport, expected, actual ResultSet) {
	if expected.Algorithm != actual.Algorithm {
		report.Differences = append(report.Differences, DeterminismDifference{Algorithm: actual.Algorithm, Field: "algorithm", Expected: expected.Algorithm, Actual: actual.Algorithm})
		return
	}
	expectedByID := make(map[string]DetectorResult, len(expected.Results))
	for _, result := range expected.Results {
		expectedByID[result.ID] = result
	}
	actualByID := make(map[string]DetectorResult, len(actual.Results))
	for _, result := range actual.Results {
		actualByID[result.ID] = result
	}
	for id, expectedResult := range expectedByID {
		actualResult, exists := actualByID[id]
		if !exists {
			report.Differences = append(report.Differences, DeterminismDifference{Algorithm: actual.Algorithm, TrackID: id, Field: "result", Expected: "present", Actual: "missing"})
			continue
		}
		compareDetectorResult(report, actual.Algorithm, id, expectedResult, actualResult)
	}
	for id := range actualByID {
		if _, exists := expectedByID[id]; !exists {
			report.Differences = append(report.Differences, DeterminismDifference{Algorithm: actual.Algorithm, TrackID: id, Field: "result", Expected: "missing", Actual: "present"})
		}
	}
}

func compareDetectorResult(report *DeterminismReport, algorithm, id string, expected, actual DetectorResult) {
	for _, scalar := range []struct {
		name string
		a, b *float64
	}{
		{"bpm", expected.BPM, actual.BPM}, {"confidence", expected.Confidence, actual.Confidence},
		{"tempoConfidence", expected.TempoConfidence, actual.TempoConfidence}, {"keyConfidence", expected.KeyConfidence, actual.KeyConfidence},
	} {
		if scalar.a == nil && scalar.b == nil {
			continue
		}
		if scalar.a == nil || scalar.b == nil || math.Abs(*scalar.a-*scalar.b) > 1e-6 {
			report.Differences = append(report.Differences, DeterminismDifference{Algorithm: algorithm, TrackID: id, Field: scalar.name, Expected: formatScalar(scalar.a), Actual: formatScalar(scalar.b)})
		}
	}
	if !strings.EqualFold(strings.TrimSpace(expected.Key), strings.TrimSpace(actual.Key)) {
		report.Differences = append(report.Differences, DeterminismDifference{Algorithm: algorithm, TrackID: id, Field: "key", Expected: expected.Key, Actual: actual.Key})
	}
	if expected.Error != actual.Error {
		report.Differences = append(report.Differences, DeterminismDifference{Algorithm: algorithm, TrackID: id, Field: "error", Expected: expected.Error, Actual: actual.Error})
	}
}

func formatRate(value float64) string { return fmt.Sprintf("%.2f%%", value*100) }

func realtimeMultiple(audioSeconds, wallSeconds float64) *float64 {
	if audioSeconds <= 0 || wallSeconds <= 0 {
		return nil
	}
	value := audioSeconds / wallSeconds
	return &value
}

func formatScalar(value *float64) string {
	if value == nil {
		return "unknown"
	}
	return fmt.Sprintf("%.9f", *value)
}
