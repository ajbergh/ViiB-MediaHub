// Package features derives compact, measured musical features from the shared
// PCM stream.  It deliberately has no database or AI dependency so the DJ UI
// and AI-DJ score the identical evidence.
package features

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"math"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/analysis/beatgrid"
)

const (
	ArtifactKind                = "energy-structure"
	FormatVersion               = 1
	AlgorithmVersion            = "energy-structure-v1"
	EnergyLevelAlgorithmVersion = "energy-level-v1-fixed-reference"
	Encoding                    = "gzip-json-v1"
	LoudnessKind                = "unweighted-mono-rms-proxy"
	PeakKind                    = "sample-plus-midpoint-peak-proxy"
	ChannelScope                = "mono"
	MeasurementStandard         = "none"
)

// EnergyPoint is a fixed-time, normalized intensity sample.  Time is the
// start of the analysis window in seconds.
type EnergyPoint struct {
	Time  float64 `json:"time"`
	Value float64 `json:"value"`
}

// Section is a conservative novelty-based region.  Labels intentionally stay
// descriptive rather than asserting genre-specific musical form.
type Section struct {
	Start  float64 `json:"start"`
	End    float64 `json:"end"`
	Energy float64 `json:"energy"`
}

// CueSuggestion is a non-authoritative, measured preparation hint. DJs can
// accept, move, or ignore it; it never changes stored hot cues by itself.
type CueSuggestion struct {
	Position   float64 `json:"position"`
	Kind       string  `json:"kind"`
	Confidence float64 `json:"confidence"`
	Rationale  string  `json:"rationale"`
}

// Result contains measured loudness and compact structure suitable for a
// library row, a waveform overlay, or explainable recommendation scoring.
type Result struct {
	// Deprecated: compatibility alias for unweighted mono RMS dB proxy.
	IntegratedLUFS float64 `json:"integratedLufs"`
	// Deprecated: compatibility alias for sample-plus-midpoint peak dBFS proxy.
	TruePeakDBFS   float64         `json:"truePeakDbfs"`
	LoudnessKind   string          `json:"loudnessKind"`
	PeakKind       string          `json:"peakKind"`
	ChannelScope   string          `json:"channelScope"`
	Standard       string          `json:"standard"`
	Energy         []EnergyPoint   `json:"energy"`
	Sections       []Section       `json:"sections"`
	CueSuggestions []CueSuggestion `json:"cueSuggestions"`
}

func qualifyMeasurement(result Result) Result {
	// Every artifact written by this analyzer has the same mono, unweighted
	// measurement path. Filling omitted values also qualifies older artifacts
	// when they are decoded and returned by the compatibility API.
	result.LoudnessKind = LoudnessKind
	result.PeakKind = PeakKind
	result.ChannelScope = ChannelScope
	result.Standard = MeasurementStandard
	return result
}

// EnergyLevelInputs are fixed-reference, absolute features used for the
// sortable track-level score. LoudnessProxyDB is the existing unweighted RMS
// proxy (not BS.1770 LUFS); PeakDBFS is a sample/interpolated peak proxy (not
// BS.1770 true peak). OnsetCrestFactor is a dimensionless rhythmic activity
// measure. These inputs and weights are versioned by EnergyLevelAlgorithmVersion.
type EnergyLevelInputs struct {
	LoudnessProxyDB  float64
	PeakDBFS         float64
	OnsetCrestFactor float64
	HasAudio         bool
}

// EnergyLevelEstimate is deterministic given the same inputs. Confidence is
// an evidence-availability heuristic, not a statistically calibrated
// probability. The fixed reference ranges deliberately avoid per-library
// normalization. This first version is a navigation aid and has not been
// calibrated against a lawful, genre-diverse reference corpus.
type EnergyLevelEstimate struct {
	Level            int
	Confidence       float64
	AlgorithmVersion string
}

func EstimateEnergyLevel(input EnergyLevelInputs) (EnergyLevelEstimate, bool) {
	if !input.HasAudio || math.IsNaN(input.LoudnessProxyDB) || math.IsInf(input.LoudnessProxyDB, 0) || math.IsNaN(input.PeakDBFS) || math.IsInf(input.PeakDBFS, 0) {
		return EnergyLevelEstimate{}, false
	}
	// Fixed broad reference transforms; loudness has only 15% influence.
	loudness := energyClamp01((input.LoudnessProxyDB + 48) / 38)
	peak := energyClamp01((input.PeakDBFS + 36) / 30)
	crestDB := math.Max(0, input.PeakDBFS-input.LoudnessProxyDB-0.691)
	transient := energyClamp01(crestDB / 18)
	rhythmic := 0.0
	if !math.IsNaN(input.OnsetCrestFactor) && !math.IsInf(input.OnsetCrestFactor, 0) && input.OnsetCrestFactor > 0 {
		rhythmic = energyClamp01(math.Log1p(input.OnsetCrestFactor) / math.Log1p(300))
	}
	score := 0.15*loudness + 0.35*peak + 0.20*transient + 0.30*rhythmic
	level := 1 + int(math.Round(score*9))
	if level < 1 {
		level = 1
	}
	if level > 10 {
		level = 10
	}
	confidence := energyClamp01(0.35 + 0.25*boolFloat(input.OnsetCrestFactor > 0) + 0.20*boolFloat(input.PeakDBFS > -36) + 0.20*boolFloat(input.LoudnessProxyDB > -48))
	return EnergyLevelEstimate{Level: level, Confidence: confidence, AlgorithmVersion: EnergyLevelAlgorithmVersion}, true
}

func energyClamp01(value float64) float64 { return math.Max(0, math.Min(1, value)) }
func boolFloat(value bool) float64 {
	if value {
		return 1
	}
	return 0
}

// AddCueSuggestions derives compact mix-in, section, and mix-out suggestions.
// It uses measured/manual downbeats only when the grid provenance supports
// that claim. Inferred grids fall back to nearest-beat suggestions with lower
// confidence and explicit rationale.
func (r *Result) AddCueSuggestions(grid *beatgrid.Grid) {
	r.CueSuggestions = []CueSuggestion{}
	if grid == nil || len(grid.Beats) == 0 || len(r.Sections) == 0 {
		return
	}
	qualifiedDownbeats := (grid.EffectiveProvenance() == beatgrid.ProvenanceMeasured || grid.EffectiveProvenance() == beatgrid.ProvenanceManual) && len(grid.DownbeatIndices) > 0
	anchors := make([]float64, 0, len(grid.Beats))
	if qualifiedDownbeats {
		for _, index := range grid.DownbeatIndices {
			if index >= 0 && index < len(grid.Beats) {
				anchors = append(anchors, grid.Beats[index])
			}
		}
	} else {
		anchors = append(anchors, grid.Beats...)
	}
	if len(anchors) == 0 {
		return
	}
	snap := func(time float64) float64 {
		best := anchors[0]
		for _, candidate := range anchors[1:] {
			if math.Abs(candidate-time) < math.Abs(best-time) {
				best = candidate
			}
		}
		return best
	}
	first, last := r.Sections[0], r.Sections[len(r.Sections)-1]
	detail, mixInConfidence, mixOutConfidence, sectionConfidence := "nearest beat; downbeat provenance unavailable", .46, .42, .38
	if qualifiedDownbeats {
		detail, mixInConfidence, mixOutConfidence, sectionConfidence = "measured/manual downbeat", .70, .65, .60
	}
	r.CueSuggestions = append(r.CueSuggestions,
		CueSuggestion{Position: snap(first.Start), Kind: "mix-in", Confidence: mixInConfidence, Rationale: "First measured section, snapped to " + detail},
		CueSuggestion{Position: snap(last.End), Kind: "mix-out", Confidence: mixOutConfidence, Rationale: "Final measured section boundary, snapped to " + detail},
	)
	for _, section := range r.Sections[1:] {
		r.CueSuggestions = append(r.CueSuggestions, CueSuggestion{Position: snap(section.Start), Kind: "section", Confidence: sectionConfidence, Rationale: "Measured energy novelty boundary, snapped to " + detail})
	}
}

// Accumulator keeps only 500 ms RMS windows and a few scalars; full PCM is
// never retained.  It is fed during the same decode pass as tempo/key/grid.
type Accumulator struct {
	sampleRate  int
	window      int
	filled      int
	sumSquares  float64
	frames      int64
	pointEnergy []float64
	peak        float64
	previous    float64
	previousSet bool
}

func NewAccumulator(sampleRate int) *Accumulator {
	return &Accumulator{sampleRate: sampleRate, window: max(1, sampleRate/2)}
}

func (a *Accumulator) Feed(samples []float32) {
	for _, sample := range samples {
		value := float64(sample)
		magnitude := math.Abs(value)
		if magnitude > a.peak {
			a.peak = magnitude
		}
		// Linear interpolation is a deterministic 2x true-peak approximation;
		// it catches inter-sample overs that sample peak alone misses.
		if a.previousSet {
			midpoint := math.Abs((a.previous + value) / 2)
			if midpoint > a.peak {
				a.peak = midpoint
			}
		}
		a.previous, a.previousSet = value, true
		a.sumSquares += value * value
		a.filled++
		a.frames++
		if a.filled == a.window {
			a.pointEnergy = append(a.pointEnergy, math.Sqrt(a.sumSquares/float64(a.filled)))
			a.sumSquares, a.filled = 0, 0
		}
	}
}

func (a *Accumulator) Result() (Result, error) {
	if a.sampleRate <= 0 || a.frames == 0 {
		return Result{}, errors.New("energy analysis requires decoded audio")
	}
	if a.filled > 0 {
		a.pointEnergy = append(a.pointEnergy, math.Sqrt(a.sumSquares/float64(a.filled)))
	}
	if len(a.pointEnergy) == 0 {
		return Result{}, errors.New("energy analysis produced no windows")
	}
	maximum := 0.0
	for _, value := range a.pointEnergy {
		if value > maximum {
			maximum = value
		}
	}
	points := make([]EnergyPoint, len(a.pointEnergy))
	for index, value := range a.pointEnergy {
		if maximum > 0 {
			value /= maximum
		}
		points[index] = EnergyPoint{Time: float64(index) * float64(a.window) / float64(a.sampleRate), Value: value}
	}
	meanSquare := 0.0
	for _, value := range a.pointEnergy {
		meanSquare += value * value
	}
	meanSquare /= float64(len(a.pointEnergy))
	// Silence is represented as 0, not -Inf: artifacts must be valid JSON and
	// callers can distinguish it by its all-zero energy curve.
	integrated := 0.0
	if meanSquare > 0 {
		// Preserve the legacy full-scale RMS proxy's numeric range. This offset
		// does not make the result BS.1770/R128-compliant; there is no K-weighting
		// or gating in this streaming v1.
		integrated = 10*math.Log10(meanSquare) - 0.691
	}
	peak := 0.0
	if a.peak > 0 {
		peak = 20 * math.Log10(a.peak)
	}
	return qualifyMeasurement(Result{IntegratedLUFS: integrated, TruePeakDBFS: peak, Energy: points, Sections: segment(points)}), nil
}

func segment(points []EnergyPoint) []Section {
	if len(points) == 0 {
		return []Section{}
	}
	start := 0
	sections := make([]Section, 0, 4)
	for index := 1; index < len(points); index++ {
		// Require four seconds from the prior split.  This is intentionally
		// conservative so a transient does not become a fake musical section.
		if points[index].Time-points[start].Time < 4 || math.Abs(points[index].Value-points[index-1].Value) < .30 {
			continue
		}
		sections = append(sections, summarize(points[start:index]))
		start = index
	}
	sections = append(sections, summarize(points[start:]))
	return sections
}

func summarize(points []EnergyPoint) Section {
	var total float64
	for _, point := range points {
		total += point.Value
	}
	end := points[len(points)-1].Time + .5
	return Section{Start: points[0].Time, End: end, Energy: total / float64(len(points))}
}

// Encode is deterministic: the gzip header has no clock or host metadata.
func (r Result) Encode() ([]byte, error) {
	if len(r.Energy) == 0 {
		return nil, errors.New("energy artifact requires points")
	}
	payload, err := json.Marshal(qualifyMeasurement(r))
	if err != nil {
		return nil, err
	}
	var output bytes.Buffer
	writer, err := gzip.NewWriterLevel(&output, gzip.BestCompression)
	if err != nil {
		return nil, err
	}
	writer.Header.ModTime = unixEpoch
	writer.Header.OS = 255
	if _, err := writer.Write(payload); err != nil {
		return nil, err
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}

func Decode(data []byte) (Result, error) {
	reader, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return Result{}, err
	}
	defer reader.Close()
	var result Result
	if err := json.NewDecoder(reader).Decode(&result); err != nil {
		return Result{}, err
	}
	if len(result.Energy) == 0 {
		return Result{}, errors.New("energy artifact has no points")
	}
	return qualifyMeasurement(result), nil
}

var unixEpoch = time.Unix(0, 0).UTC()

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
