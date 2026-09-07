// Package tempo provides the first backend-owned tempo candidate estimator.
package tempo

import "math"

// RangePreset constrains tempo candidates to an expected DJ operating range.
type RangePreset string

const (
	RangeAutomatic RangePreset = "auto"
	Range60to120   RangePreset = "60-120"
	Range70to140   RangePreset = "70-140"
	Range80to160   RangePreset = "80-160"
	Range100to200  RangePreset = "100-200"
	RangeCustom    RangePreset = "custom"
)

// Options configure candidate bounds and metrical range priors.
type Options struct {
	Range  RangePreset
	MinBPM float64
	MaxBPM float64
}

// DefaultOptions selects Automatic mode with standard DJ tempo priors.
func DefaultOptions() Options {
	return Options{
		Range:  RangeAutomatic,
		MinBPM: 90,
		MaxBPM: 180,
	}
}

// Estimate retains primary and alternate candidates, stability, and confidence.
// Known is false when evidence is insufficient; callers must not invent BPM.
type Estimate struct {
	BPM              float64
	Confidence       float64
	Alternate        float64
	Stability        float64
	Known            bool
	AlgorithmVersion string
}

const AlgorithmVersion = "tempo-v1-onset-interval"

// minOnsetCrestFactor is the least impulsive an onset envelope may be before
// tempo is refused. A sustained source — a drone, pad, sine tone, or spoken
// word — has a nearly uniform rectified energy difference, so its largest
// "onset" is only a few times the average and any peak picking degenerates
// into reading floating-point noise. Measured on the Phase 0 synthetic
// fixtures, percussive material lands between 102 and 574 while sustained
// material lands between 3.1 and 9.5, so this bound has roughly a fourfold
// margin on both sides. Reporting a confident BPM for audio that has no beats
// is worse than reporting none.
const minOnsetCrestFactor = 25

// EstimatePCM estimates a static tempo from a mono normalized PCM segment.
// It is deliberately fractional and conservative; Phase 0 corpus gates still
// determine whether its algorithm is adequate for professional release.
func EstimatePCM(samples []float32, sampleRate int) Estimate {
	return EstimatePCMWithOptions(samples, sampleRate, DefaultOptions())
}

// EstimatePCMWithOptions estimates tempo with caller-specified range priors.
func EstimatePCMWithOptions(samples []float32, sampleRate int, opts Options) Estimate {
	accumulator := NewOnsetAccumulatorWithOptions(sampleRate, opts)
	accumulator.Feed(samples)
	return accumulator.Estimate()
}

// OnsetAccumulator retains only a low-rate onset envelope; Feed may be called
// with bounded decoded PCM chunks from the shared analysis service.
type OnsetAccumulator struct {
	sampleRate, window, hop, nextStart int
	options                            Options
	pending                            []float32
	envelope                           []float64
	positions                          []int
}

func NewOnsetAccumulator(sampleRate int) *OnsetAccumulator {
	return NewOnsetAccumulatorWithOptions(sampleRate, DefaultOptions())
}

func NewOnsetAccumulatorWithOptions(sampleRate int, opts Options) *OnsetAccumulator {
	return &OnsetAccumulator{
		sampleRate: sampleRate,
		window:     max(1, sampleRate/200),
		hop:        max(1, sampleRate/1000),
		options:    opts,
	}
}

func (a *OnsetAccumulator) Feed(samples []float32) {
	if a.sampleRate <= 0 {
		return
	}
	a.pending = append(a.pending, samples...)
	for len(a.pending) >= a.window {
		energy := 0.
		for _, sample := range a.pending[:a.window] {
			v := float64(sample)
			energy += v * v
		}
		a.envelope = append(a.envelope, math.Sqrt(energy/float64(a.window)))
		a.positions = append(a.positions, a.nextStart)
		a.pending = a.pending[a.hop:]
		a.nextStart += a.hop
	}
}

func (a *OnsetAccumulator) Estimate() Estimate {
	if a.sampleRate <= 0 || len(a.envelope) < 3 || maxValue(a.envelope) < 1e-7 {
		return Estimate{AlgorithmVersion: AlgorithmVersion}
	}
	onsets := make([]float64, len(a.envelope))
	for i := 1; i < len(a.envelope); i++ {
		onsets[i] = maxFloat(0, a.envelope[i]-a.envelope[i-1])
	}
	mean, deviation := meanDeviation(onsets)
	// Require transient evidence before picking peaks. Without this, a purely
	// sustained source has no beats to find and the relative threshold below
	// adapts down onto its numerical noise floor, inventing a tempo. A
	// non-positive mean means the envelope never rose, which is the same
	// no-evidence case.
	if mean <= 0 || maxValue(onsets) < minOnsetCrestFactor*mean {
		return Estimate{AlgorithmVersion: AlgorithmVersion}
	}
	threshold := mean + 1.5*deviation
	minDistance := max(1, int(math.Round(.1*float64(a.sampleRate)/float64(a.hop))))
	peaks := make([]int, 0)
	for i := 1; i+1 < len(onsets); i++ {
		if onsets[i] < threshold || onsets[i] < onsets[i-1] || onsets[i] < onsets[i+1] {
			continue
		}
		if len(peaks) > 0 && i-peaks[len(peaks)-1] < minDistance {
			if onsets[i] > onsets[peaks[len(peaks)-1]] {
				peaks[len(peaks)-1] = i
			}
			continue
		}
		peaks = append(peaks, i)
	}
	if len(peaks) < 3 {
		return Estimate{AlgorithmVersion: AlgorithmVersion}
	}

	minBPM, maxBPM := resolveBounds(a.options)

	type candidate struct {
		count int
		sum   float64
		bpms  []float64
	}
	candidates := map[int]candidate{}
	total := 0
	for i := 1; i < len(peaks); i++ {
		delta := a.positions[peaks[i]] - a.positions[peaks[i-1]]
		if delta <= 0 {
			continue
		}
		rawBPM := 60 * float64(a.sampleRate) / float64(delta)
		bpm := normalizeWithBounds(rawBPM, minBPM, maxBPM)
		if bpm == 0 {
			continue
		}
		bucket := int(math.Round(bpm * 10))
		current := candidates[bucket]
		current.count++
		current.sum += bpm
		current.bpms = append(current.bpms, bpm)
		candidates[bucket] = current
		total++
	}
	if total == 0 {
		return Estimate{AlgorithmVersion: AlgorithmVersion}
	}

	bestBucket, best := 0, candidate{}
	for bucket, value := range candidates {
		if value.count > best.count || (value.count == best.count && bucket < bestBucket) {
			bestBucket, best = bucket, value
		}
	}

	primaryBPM := best.sum / float64(best.count)
	confidence := float64(best.count) / float64(total)

	// Identify runner-up candidate separated from winning bucket by at least 1.0 BPM.
	runnerUpBucket, runnerUp := 0, candidate{}
	for bucket, value := range candidates {
		if math.Abs(float64(bucket-bestBucket)) < 10 {
			continue
		}
		if value.count > runnerUp.count || (value.count == runnerUp.count && bucket < runnerUpBucket) {
			runnerUpBucket, runnerUp = bucket, value
		}
	}

	alternate := 0.0
	if runnerUp.count > 0 {
		alternate = runnerUp.sum / float64(runnerUp.count)
	} else if primaryBPM >= 120 {
		alternate = primaryBPM / 2
	} else {
		alternate = primaryBPM * 2
	}

	// Stability reflects the ratio of intervals within a 1.0 BPM tolerance of the dominant
	// tempo, penalized by dispersion within that cluster.
	inClusterCount := 0
	variance := 0.0
	for _, cand := range candidates {
		for _, b := range cand.bpms {
			diff := b - primaryBPM
			if math.Abs(diff) <= 1.0 {
				inClusterCount++
				variance += diff * diff
			}
		}
	}

	stability := 0.0
	if inClusterCount > 0 {
		stdDev := math.Sqrt(variance / float64(inClusterCount))
		consistency := maxFloat(0, 1.0-stdDev)
		clusterRatio := float64(inClusterCount) / float64(total)
		stability = clusterRatio * consistency
	}

	return Estimate{
		BPM:              primaryBPM,
		Confidence:       confidence,
		Alternate:        alternate,
		Stability:        stability,
		Known:            true,
		AlgorithmVersion: AlgorithmVersion,
	}
}

func resolveBounds(opts Options) (float64, float64) {
	switch opts.Range {
	case Range60to120:
		return 60, 120
	case Range70to140:
		return 70, 140
	case Range80to160:
		return 80, 160
	case Range100to200:
		return 100, 200
	case RangeCustom:
		minB, maxB := opts.MinBPM, opts.MaxBPM
		if minB <= 0 {
			minB = 60
		}
		if maxB <= minB {
			maxB = minB * 2
		}
		return minB, maxB
	default:
		return 90, 180
	}
}

func normalizeWithBounds(bpm, minBPM, maxBPM float64) float64 {
	if bpm <= 0 {
		return 0
	}
	for bpm < minBPM {
		bpm *= 2
	}
	for bpm > maxBPM {
		bpm /= 2
	}
	if bpm < minBPM || bpm > maxBPM {
		return 0
	}
	return bpm
}

func meanDeviation(values []float64) (float64, float64) {
	mean := 0.
	for _, v := range values {
		mean += v
	}
	mean /= float64(len(values))
	variance := 0.
	for _, v := range values {
		d := v - mean
		variance += d * d
	}
	return mean, math.Sqrt(variance / float64(len(values)))
}

func maxValue(values []float64) float64 {
	result := 0.
	for _, v := range values {
		result = maxFloat(result, v)
	}
	return result
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
