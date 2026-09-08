package analysisbench

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"sort"
	"strings"
)

const (
	// SplitTuning is visible to algorithm development; it is never a source of
	// release-facing accuracy metrics.
	SplitTuning = "tuning"
	// SplitHeldOut is reserved before tuning and used for Phase 0 tripwires.
	SplitHeldOut = "held_out"
	// EvidenceSyntheticCI identifies generated fixtures that are useful for
	// regression but cannot close the professional-quality gate.
	EvidenceSyntheticCI = "synthetic-ci"
	// EvidenceLawfulRealAudio identifies a local corpus whose audio and labels
	// have been reviewed for Phase 0 measurement. The declaration is retained
	// in raw evidence for human audit; it is not a license verdict by itself.
	EvidenceLawfulRealAudio = "lawful-real-audio"
)

// CorpusManifest is a local, label-only benchmark inventory. Audio files are
// intentionally referenced, not vendored, so copyright restrictions remain
// explicit and CI can use synthetic fixtures separately.
type CorpusManifest struct {
	Version       string        `json:"version"`
	EvidenceClass string        `json:"evidenceClass"`
	Tracks        []CorpusTrack `json:"tracks"`
}

// CorpusTrack mirrors roadmap §14.1 while adding the required split field.
type CorpusTrack struct {
	ID                string    `json:"id"`
	Path              string    `json:"path"`
	License           string    `json:"license"`
	LabelSource       string    `json:"labelSource"`
	Genre             string    `json:"genre"`
	Coverage          []string  `json:"coverage,omitempty"`
	Split             string    `json:"split"`
	ExpectedBPM       *float64  `json:"expectedBpm,omitempty"`
	AcceptedMetricBPM []float64 `json:"acceptedMetricBpm,omitempty"`
	ExpectedKey       string    `json:"expectedKey,omitempty"`
	ExpectedUnknown   bool      `json:"expectedUnknown,omitempty"`
	Notes             string    `json:"notes,omitempty"`
}

// ResultSet is an exported detector result, including the current browser
// implementation. Keeping it JSON-only prevents a benchmark from importing
// Web Audio code into the Go process.
type ResultSet struct {
	Algorithm  string             `json:"algorithm"`
	Results    []DetectorResult   `json:"results"`
	Throughput *ThroughputMetrics `json:"throughput,omitempty"`
}

// DetectorResult intentionally permits unknown BPM/key values. A missing
// result must be counted, not converted into a plausible default.
type DetectorResult struct {
	ID  string   `json:"id"`
	BPM *float64 `json:"bpm,omitempty"`
	Key string   `json:"key,omitempty"`
	// Confidence remains the legacy/browser confidence field. New producers
	// should use the dimension-specific fields so tempo and key calibration
	// cannot accidentally share an unrelated score.
	Confidence      *float64 `json:"confidence,omitempty"`
	TempoConfidence *float64 `json:"tempoConfidence,omitempty"`
	KeyConfidence   *float64 `json:"keyConfidence,omitempty"`
	Error           string   `json:"error,omitempty"`
	ErrorMessage    string   `json:"errorMessage,omitempty"`
}

// ComparisonReport contains metrics for exactly one manifest split.
type ComparisonReport struct {
	Algorithm   string                `json:"algorithm"`
	Split       string                `json:"split"`
	Corpus      CorpusCoverage        `json:"corpus"`
	Tempo       TempoMetrics          `json:"tempo"`
	Key         KeyMetrics            `json:"key"`
	Unknown     UnknownMetrics        `json:"unknown"`
	Calibration ConfidenceCalibration `json:"calibration"`
	Throughput  *ThroughputMetrics    `json:"throughput,omitempty"`
}

// ConfidenceCalibration shows whether reported confidence predicts actual
// correctness. The gate requires all three fixed buckets to be populated and
// non-decreasing; a sparse corpus is therefore visibly inconclusive.
type ConfidenceCalibration struct {
	Tempo ConfidenceCalibrationDimension `json:"tempo"`
	Key   ConfidenceCalibrationDimension `json:"key"`
}

type ConfidenceCalibrationDimension struct {
	Buckets   []ConfidenceBucket `json:"buckets"`
	Monotonic bool               `json:"monotonic"`
	Verdict   string             `json:"verdict"`
}

type ConfidenceBucket struct {
	Label    string   `json:"label"`
	Minimum  float64  `json:"minimum"`
	Maximum  float64  `json:"maximum"`
	Labeled  int      `json:"labeled"`
	Correct  int      `json:"correct"`
	Accuracy *float64 `json:"accuracy,omitempty"`
}

// UnknownMetrics evaluates fixtures that must not produce a credible-looking
// result, such as silence. This is distinct from the unknown rate on labeled
// music, where an unknown output is a missed result rather than success.
type UnknownMetrics struct {
	Labeled   int      `json:"labeled"`
	Correct   int      `json:"correct"`
	Incorrect int      `json:"incorrect"`
	Accuracy  *float64 `json:"accuracy,omitempty"`
}

// CorpusCoverage makes it impossible for a tiny smoke-test manifest to look
// like a Phase 0 exit-gate result. The 200-track, one-third held-out rule comes
// directly from the roadmap and remains visible in every comparison report.
type CorpusCoverage struct {
	Tracks                  int      `json:"tracks"`
	TracksInSplit           int      `json:"tracksInSplit"`
	HeldOutTracks           int      `json:"heldOutTracks"`
	DistinctGenres          []string `json:"distinctGenres"`
	EvidenceClass           string   `json:"evidenceClass"`
	MissingRequiredCoverage []string `json:"missingRequiredCoverage"`
	Phase0Ready             bool     `json:"phase0Ready"`
	ReadinessMessage        string   `json:"readinessMessage"`
}

// TempoMetrics keeps raw counts beside percentages so a small corpus cannot
// disguise uncertainty behind a rounded score.
type TempoMetrics struct {
	Labeled          int      `json:"labeled"`
	Reported         int      `json:"reported"`
	StrictWithinHalf int      `json:"strictWithinHalfBpm"`
	MetricalMatch    int      `json:"acceptedMetricLevelMatch"`
	HalfDoubleErrors int      `json:"halfDoubleErrors"`
	Unknown          int      `json:"unknown"`
	StrictAccuracy   *float64 `json:"strictAccuracy,omitempty"`
	MetricalAccuracy *float64 `json:"acceptedMetricLevelAccuracy,omitempty"`
	HalfDoubleRate   *float64 `json:"halfDoubleErrorRate,omitempty"`
	UnknownRate      *float64 `json:"unknownRate,omitempty"`
}

// KeyMetrics uses the roadmap's conservative compatibility definition: same
// key, adjacent wheel position in the same mode, or relative major/minor.
type KeyMetrics struct {
	Labeled           int      `json:"labeled"`
	Reported          int      `json:"reported"`
	Exact             int      `json:"exact"`
	CamelotCompatible int      `json:"camelotCompatible"`
	Unknown           int      `json:"unknown"`
	ExactAccuracy     *float64 `json:"exactAccuracy,omitempty"`
	CompatibleRate    *float64 `json:"camelotCompatibleRate,omitempty"`
	UnknownRate       *float64 `json:"unknownRate,omitempty"`
}

// LoadManifest reads and validates a corpus manifest before any metric is
// calculated. This makes unlicensed or accidentally re-tuned labels visible.
func LoadManifest(path string) (CorpusManifest, error) {
	file, err := os.Open(path)
	if err != nil {
		return CorpusManifest{}, fmt.Errorf("open manifest: %w", err)
	}
	defer file.Close()
	var manifest CorpusManifest
	if err := json.NewDecoder(file).Decode(&manifest); err != nil {
		return CorpusManifest{}, fmt.Errorf("decode manifest: %w", err)
	}
	if err := manifest.Validate(); err != nil {
		return CorpusManifest{}, err
	}
	return manifest, nil
}

// LoadResultSet reads detector outputs produced by the browser export or a Go
// candidate. It rejects duplicate IDs so results cannot silently overwrite.
func LoadResultSet(path string) (ResultSet, error) {
	file, err := os.Open(path)
	if err != nil {
		return ResultSet{}, fmt.Errorf("open results: %w", err)
	}
	defer file.Close()
	var resultSet ResultSet
	if err := json.NewDecoder(file).Decode(&resultSet); err != nil {
		return ResultSet{}, fmt.Errorf("decode results: %w", err)
	}
	if err := resultSet.Validate(); err != nil {
		return ResultSet{}, err
	}
	return resultSet, nil
}

// Validate checks metadata required to make benchmark measurements auditable.
func (manifest CorpusManifest) Validate() error {
	if strings.TrimSpace(manifest.Version) == "" {
		return fmt.Errorf("manifest version is required")
	}
	if manifest.EvidenceClass != EvidenceSyntheticCI && manifest.EvidenceClass != EvidenceLawfulRealAudio {
		return fmt.Errorf("manifest evidence class must be %q or %q", EvidenceSyntheticCI, EvidenceLawfulRealAudio)
	}
	if len(manifest.Tracks) == 0 {
		return fmt.Errorf("manifest must contain at least one track")
	}
	seen := make(map[string]struct{}, len(manifest.Tracks))
	for index, track := range manifest.Tracks {
		context := fmt.Sprintf("manifest track %d", index)
		if strings.TrimSpace(track.ID) == "" || strings.TrimSpace(track.Path) == "" || strings.TrimSpace(track.License) == "" || strings.TrimSpace(track.LabelSource) == "" || strings.TrimSpace(track.Genre) == "" {
			return fmt.Errorf("%s requires id, path, license, label source, and genre", context)
		}
		if track.Split != SplitTuning && track.Split != SplitHeldOut {
			return fmt.Errorf("%s has invalid split %q", context, track.Split)
		}
		if _, exists := seen[track.ID]; exists {
			return fmt.Errorf("duplicate manifest track id %q", track.ID)
		}
		seen[track.ID] = struct{}{}
		if track.ExpectedBPM != nil && (*track.ExpectedBPM <= 0 || math.IsNaN(*track.ExpectedBPM) || math.IsInf(*track.ExpectedBPM, 0)) {
			return fmt.Errorf("%s has invalid expected BPM", context)
		}
		for _, bpm := range track.AcceptedMetricBPM {
			if bpm <= 0 || math.IsNaN(bpm) || math.IsInf(bpm, 0) {
				return fmt.Errorf("%s has invalid accepted metric BPM", context)
			}
		}
		if track.ExpectedUnknown && (track.ExpectedBPM != nil || strings.TrimSpace(track.ExpectedKey) != "") {
			return fmt.Errorf("%s cannot combine expected unknown with BPM or key labels", context)
		}
		if !track.ExpectedUnknown && track.ExpectedBPM == nil && strings.TrimSpace(track.ExpectedKey) == "" {
			return fmt.Errorf("%s requires an expected BPM, key, or unknown label", context)
		}
		if strings.TrimSpace(track.ExpectedKey) != "" {
			if _, err := parseKey(track.ExpectedKey); err != nil {
				return fmt.Errorf("%s expected key: %w", context, err)
			}
		}
		for _, tag := range track.Coverage {
			if normalizeCoverageTag(tag) == "" {
				return fmt.Errorf("%s has an empty coverage tag", context)
			}
		}
	}
	return nil
}

// Validate checks structural problems in a detector result without treating an
// absent BPM or key as invalid; those are meaningful unknown outputs.
func (resultSet ResultSet) Validate() error {
	if strings.TrimSpace(resultSet.Algorithm) == "" {
		return fmt.Errorf("result algorithm is required")
	}
	seen := make(map[string]struct{}, len(resultSet.Results))
	for index, result := range resultSet.Results {
		if strings.TrimSpace(result.ID) == "" {
			return fmt.Errorf("result %d has no id", index)
		}
		if _, exists := seen[result.ID]; exists {
			return fmt.Errorf("duplicate result id %q", result.ID)
		}
		seen[result.ID] = struct{}{}
		if result.BPM != nil && (*result.BPM <= 0 || math.IsNaN(*result.BPM) || math.IsInf(*result.BPM, 0)) {
			return fmt.Errorf("result %q has invalid BPM", result.ID)
		}
		for label, confidence := range map[string]*float64{"confidence": result.Confidence, "tempo confidence": result.TempoConfidence, "key confidence": result.KeyConfidence} {
			if confidence != nil && (*confidence < 0 || *confidence > 1 || math.IsNaN(*confidence) || math.IsInf(*confidence, 0)) {
				return fmt.Errorf("result %q has invalid %s", result.ID, label)
			}
		}
		if strings.TrimSpace(result.Key) != "" && !strings.EqualFold(strings.TrimSpace(result.Key), "unknown") {
			if _, err := parseKey(result.Key); err != nil {
				return fmt.Errorf("result %q key: %w", result.ID, err)
			}
		}
	}
	return nil
}

// Compare evaluates one result set against a single predeclared split.
func Compare(manifest CorpusManifest, resultSet ResultSet, split string) (ComparisonReport, error) {
	if err := manifest.Validate(); err != nil {
		return ComparisonReport{}, err
	}
	if err := resultSet.Validate(); err != nil {
		return ComparisonReport{}, err
	}
	if split != SplitTuning && split != SplitHeldOut {
		return ComparisonReport{}, fmt.Errorf("invalid comparison split %q", split)
	}

	results := make(map[string]DetectorResult, len(resultSet.Results))
	for _, result := range resultSet.Results {
		results[result.ID] = result
	}
	report := ComparisonReport{Algorithm: resultSet.Algorithm, Split: split, Corpus: Coverage(manifest, split), Throughput: resultSet.Throughput}
	for _, track := range manifest.Tracks {
		if track.Split != split {
			continue
		}
		result, present := results[track.ID]
		if track.ExpectedUnknown {
			report.Unknown.Labeled++
			if !present || (result.BPM == nil && (strings.TrimSpace(result.Key) == "" || strings.EqualFold(strings.TrimSpace(result.Key), "unknown"))) {
				report.Unknown.Correct++
			} else {
				report.Unknown.Incorrect++
			}
		}
		if track.ExpectedBPM != nil {
			report.Tempo.Labeled++
			if !present || result.BPM == nil {
				report.Tempo.Unknown++
			} else {
				report.Tempo.Reported++
				if math.Abs(*result.BPM-*track.ExpectedBPM) <= 0.5 {
					report.Tempo.StrictWithinHalf++
				}
				if matchesAcceptedMetricLevel(*result.BPM, track) {
					report.Tempo.MetricalMatch++
				} else if isHalfDouble(*result.BPM, *track.ExpectedBPM) {
					report.Tempo.HalfDoubleErrors++
				}
			}
		}
		if strings.TrimSpace(track.ExpectedKey) != "" {
			report.Key.Labeled++
			if !present || strings.TrimSpace(result.Key) == "" || strings.EqualFold(strings.TrimSpace(result.Key), "unknown") {
				report.Key.Unknown++
				continue
			}
			report.Key.Reported++
			expected, _ := parseKey(track.ExpectedKey)
			actual, _ := parseKey(result.Key)
			if actual == expected {
				report.Key.Exact++
			}
			if keysCompatible(expected, actual) {
				report.Key.CamelotCompatible++
			}
		}
	}
	report.Tempo.StrictAccuracy = percentage(report.Tempo.StrictWithinHalf, report.Tempo.Labeled)
	report.Tempo.MetricalAccuracy = percentage(report.Tempo.MetricalMatch, report.Tempo.Labeled)
	report.Tempo.HalfDoubleRate = percentage(report.Tempo.HalfDoubleErrors, report.Tempo.Labeled)
	report.Tempo.UnknownRate = percentage(report.Tempo.Unknown, report.Tempo.Labeled)
	report.Key.ExactAccuracy = percentage(report.Key.Exact, report.Key.Labeled)
	report.Key.CompatibleRate = percentage(report.Key.CamelotCompatible, report.Key.Labeled)
	report.Key.UnknownRate = percentage(report.Key.Unknown, report.Key.Labeled)
	report.Unknown.Accuracy = percentage(report.Unknown.Correct, report.Unknown.Labeled)
	report.Calibration.Tempo = calibrateConfidence(manifest, results, split, false)
	report.Calibration.Key = calibrateConfidence(manifest, results, split, true)
	return report, nil
}

func calibrateConfidence(manifest CorpusManifest, results map[string]DetectorResult, split string, keyDimension bool) ConfidenceCalibrationDimension {
	buckets := []ConfidenceBucket{
		{Label: "low", Minimum: 0, Maximum: 1.0 / 3.0},
		{Label: "medium", Minimum: 1.0 / 3.0, Maximum: 2.0 / 3.0},
		{Label: "high", Minimum: 2.0 / 3.0, Maximum: 1},
	}
	for _, track := range manifest.Tracks {
		if track.Split != split {
			continue
		}
		result, present := results[track.ID]
		if !present {
			continue
		}
		confidence := result.TempoConfidence
		if keyDimension {
			confidence = result.KeyConfidence
		}
		if confidence == nil {
			confidence = result.Confidence
		}
		if confidence == nil {
			continue
		}
		correct := false
		if keyDimension {
			if strings.TrimSpace(track.ExpectedKey) == "" || strings.TrimSpace(result.Key) == "" || strings.EqualFold(strings.TrimSpace(result.Key), "unknown") {
				continue
			}
			expected, _ := parseKey(track.ExpectedKey)
			actual, _ := parseKey(result.Key)
			correct = expected == actual
		} else {
			if track.ExpectedBPM == nil || result.BPM == nil {
				continue
			}
			correct = math.Abs(*result.BPM-*track.ExpectedBPM) <= 0.5
		}
		index := 0
		if *confidence >= 2.0/3.0 {
			index = 2
		} else if *confidence >= 1.0/3.0 {
			index = 1
		}
		buckets[index].Labeled++
		if correct {
			buckets[index].Correct++
		}
	}
	allPopulated := true
	monotonic := true
	previous := -1.0
	for index := range buckets {
		buckets[index].Accuracy = percentage(buckets[index].Correct, buckets[index].Labeled)
		if buckets[index].Accuracy == nil {
			allPopulated = false
			continue
		}
		if previous > *buckets[index].Accuracy {
			monotonic = false
		}
		previous = *buckets[index].Accuracy
	}
	verdict := "requires observations in all three confidence buckets"
	if allPopulated {
		verdict = "confidence decreases between one or more buckets"
		if monotonic {
			verdict = "monotonic across all three confidence buckets"
		}
	}
	return ConfidenceCalibrationDimension{Buckets: buckets, Monotonic: allPopulated && monotonic, Verdict: verdict}
}

// Coverage reports whether a manifest can support the Phase 0 exit gate. It
// does not check audio file existence because corpora are intentionally local.
func Coverage(manifest CorpusManifest, split string) CorpusCoverage {
	coverage := CorpusCoverage{Tracks: len(manifest.Tracks), EvidenceClass: manifest.EvidenceClass, DistinctGenres: RequiredGenres(manifest, split)}
	covered := make(map[string]struct{})
	for _, track := range manifest.Tracks {
		if track.Split == split {
			coverage.TracksInSplit++
		}
		if track.Split == SplitHeldOut {
			coverage.HeldOutTracks++
		}
		covered[normalizeCoverageTag(track.Genre)] = struct{}{}
		for _, tag := range track.Coverage {
			covered[normalizeCoverageTag(tag)] = struct{}{}
		}
	}
	for _, required := range RequiredCorpusCoverage() {
		if _, ok := covered[required]; !ok {
			coverage.MissingRequiredCoverage = append(coverage.MissingRequiredCoverage, required)
		}
	}
	minimumHeldOut := (coverage.Tracks + 2) / 3
	deficits := make([]string, 0, 4)
	if manifest.EvidenceClass != EvidenceLawfulRealAudio {
		deficits = append(deficits, fmt.Sprintf("evidence class must be %q; %q is regression evidence only", EvidenceLawfulRealAudio, manifest.EvidenceClass))
	}
	if coverage.Tracks < 200 {
		deficits = append(deficits, fmt.Sprintf("needs %d additional tracks to meet the 200-track Phase 0 minimum", 200-coverage.Tracks))
	}
	if coverage.HeldOutTracks < minimumHeldOut {
		deficits = append(deficits, fmt.Sprintf("needs %d additional held-out tracks to reserve one third of the corpus", minimumHeldOut-coverage.HeldOutTracks))
	}
	if len(coverage.MissingRequiredCoverage) > 0 {
		deficits = append(deficits, fmt.Sprintf("missing required Phase 0 coverage: %s", strings.Join(coverage.MissingRequiredCoverage, ", ")))
	}
	if len(deficits) > 0 {
		coverage.ReadinessMessage = strings.Join(deficits, "; ")
		return coverage
	}
	coverage.Phase0Ready = true
	coverage.ReadinessMessage = "meets the Phase 0 corpus-size, held-out-split, and required-coverage minimum"
	return coverage
}

// RequiredCorpusCoverage names the §14.2 cases that must be represented by a
// lawful local corpus before Phase 0 can close. Genres remain free-form; the
// coverage tags make cross-cutting cases (meter and tempo ambiguity) auditable.
func RequiredCorpusCoverage() []string {
	return []string{
		"house", "techno", "drum-and-bass", "hip-hop", "breakbeat",
		"rock-live-drums", "disco", "ambient", "acoustic", "sparse-no-percussion",
		"meter-3-4-or-6-8", "tempo-ramp-or-switch", "tempo-70-140-ambiguity", "tempo-85-170-ambiguity",
	}
}

func normalizeCoverageTag(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(value)), "-"))
}

func isHalfDouble(actual, expected float64) bool {
	return math.Abs(actual*2-expected) <= 0.5 || math.Abs(actual-expected*2) <= 0.5
}

func matchesAcceptedMetricLevel(actual float64, track CorpusTrack) bool {
	accepted := track.AcceptedMetricBPM
	if len(accepted) == 0 && track.ExpectedBPM != nil {
		accepted = []float64{*track.ExpectedBPM}
	}
	for _, bpm := range accepted {
		if math.Abs(actual-bpm) <= 0.5 {
			return true
		}
	}
	return false
}

func percentage(numerator, denominator int) *float64 {
	if denominator == 0 {
		return nil
	}
	value := float64(numerator) / float64(denominator)
	return &value
}

type musicalKey struct {
	pitchClass int
	minor      bool
}

func parseKey(value string) (musicalKey, error) {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if strings.HasSuffix(normalized, "m") && !strings.Contains(normalized, " ") {
		normalized = strings.TrimSuffix(normalized, "m") + " minor"
	}
	parts := strings.Fields(normalized)
	if len(parts) == 1 {
		parts = append(parts, "major")
	}
	if len(parts) != 2 || (parts[1] != "major" && parts[1] != "minor") {
		return musicalKey{}, fmt.Errorf("must be '<tonic> major' or '<tonic> minor'")
	}
	pitchClass, ok := map[string]int{
		"c": 0, "b#": 0, "c#": 1, "db": 1, "d": 2, "d#": 3, "eb": 3,
		"e": 4, "fb": 4, "f": 5, "e#": 5, "f#": 6, "gb": 6, "g": 7,
		"g#": 8, "ab": 8, "a": 9, "a#": 10, "bb": 10, "b": 11, "cb": 11,
	}[parts[0]]
	if !ok {
		return musicalKey{}, fmt.Errorf("unsupported tonic %q", parts[0])
	}
	return musicalKey{pitchClass: pitchClass, minor: parts[1] == "minor"}, nil
}

func keysCompatible(expected, actual musicalKey) bool {
	if expected == actual {
		return true
	}
	if expected.minor != actual.minor {
		return expected.pitchClass == (actual.pitchClass+3)%12 || actual.pitchClass == (expected.pitchClass+3)%12
	}
	delta := (actual.pitchClass - expected.pitchClass + 12) % 12
	return delta == 5 || delta == 7
}

// RequiredGenres returns a stable list suitable for displaying corpus gaps in
// CI output or a future dashboard.
func RequiredGenres(manifest CorpusManifest, split string) []string {
	genres := make(map[string]struct{})
	for _, track := range manifest.Tracks {
		if track.Split == split {
			genres[track.Genre] = struct{}{}
		}
	}
	result := make([]string, 0, len(genres))
	for genre := range genres {
		result = append(result, genre)
	}
	sort.Strings(result)
	return result
}
