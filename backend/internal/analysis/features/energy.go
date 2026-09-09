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
)

const (
	ArtifactKind     = "energy-structure"
	FormatVersion    = 1
	AlgorithmVersion = "energy-structure-v1"
	Encoding         = "gzip-json-v1"
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

// Result contains measured loudness and compact structure suitable for a
// library row, a waveform overlay, or explainable recommendation scoring.
type Result struct {
	IntegratedLUFS float64       `json:"integratedLufs"`
	TruePeakDBFS   float64       `json:"truePeakDbfs"`
	Energy         []EnergyPoint `json:"energy"`
	Sections       []Section     `json:"sections"`
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
		// The -0.691 calibration is the standard full-scale RMS-to-LUFS
		// offset.  This streaming v1 intentionally omits gated K-weighting,
		// so consumers receive a measured loudness proxy, never a claim of
		// broadcast compliance.
		integrated = 10*math.Log10(meanSquare) - 0.691
	}
	peak := 0.0
	if a.peak > 0 {
		peak = 20 * math.Log10(a.peak)
	}
	return Result{IntegratedLUFS: integrated, TruePeakDBFS: peak, Energy: points, Sections: segment(points)}, nil
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
	payload, err := json.Marshal(r)
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
	return result, nil
}

var unixEpoch = time.Unix(0, 0).UTC()

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
