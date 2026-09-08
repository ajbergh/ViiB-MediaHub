// Package tempo provides the first backend-owned tempo candidate estimator.
package tempo

import (
	"math"

	"github.com/ajbergh/viib-mediahub/internal/analysis"
)

// RangePreset constrains tempo candidates to an expected DJ operating range.
type RangePreset string

// Method identifies the candidate-selection strategy after onset extraction.
// The established method remains the default while Phase 0 evaluates the
// periodicity candidate on the reserved tuning split.
type Method string

const (
	RangeAutomatic RangePreset = "auto"
	Range60to120   RangePreset = "60-120"
	Range70to140   RangePreset = "70-140"
	Range80to160   RangePreset = "80-160"
	Range100to200  RangePreset = "100-200"
	RangeCustom    RangePreset = "custom"
)

const (
	MethodPeakInterval          Method = "peak-interval"
	MethodOnsetAutocorrelation  Method = "onset-autocorrelation"
	MethodMultiFeatureConsensus Method = "multifeature-consensus"
)

// Options configure candidate bounds and metrical range priors.
type Options struct {
	Range               RangePreset
	Method              Method
	MinBPM              float64
	MaxBPM              float64
	MinOnsetCrestFactor float64
}

// DefaultOptions selects Automatic mode with standard DJ tempo priors.
func DefaultOptions() Options {
	return Options{
		Range:               RangeAutomatic,
		Method:              MethodPeakInterval,
		MinBPM:              90,
		MaxBPM:              180,
		MinOnsetCrestFactor: minOnsetCrestFactor,
	}
}

// Estimate retains primary and alternate candidates, stability, and confidence.
// Known is false when evidence is insufficient; callers must not invent BPM.
type Estimate struct {
	BPM              float64
	Confidence       float64
	Alternate        float64
	Stability        float64
	OnsetCrestFactor float64
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
	fluxSTFT                           *analysis.STFT
	fluxPending                        []float32
	previousFluxSpectrum               []float64
	fluxEnvelope                       []float64
}

func NewOnsetAccumulator(sampleRate int) *OnsetAccumulator {
	return NewOnsetAccumulatorWithOptions(sampleRate, DefaultOptions())
}

func NewOnsetAccumulatorWithOptions(sampleRate int, opts Options) *OnsetAccumulator {
	fluxWindow := 1024
	if sampleRate > 0 && sampleRate < 16000 {
		fluxWindow = 512
	}
	fluxSTFT, _ := analysis.NewSTFT(fluxWindow, fluxWindow/2)
	return &OnsetAccumulator{
		sampleRate: sampleRate,
		window:     max(1, sampleRate/200),
		hop:        max(1, sampleRate/1000),
		options:    opts,
		fluxSTFT:   fluxSTFT,
	}
}

func (a *OnsetAccumulator) Feed(samples []float32) {
	if a.sampleRate <= 0 {
		return
	}
	a.feedSpectralFlux(samples)
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

// feedSpectralFlux retains a second, independent onset representation. Its
// positive spectral-magnitude changes complement amplitude-envelope changes,
// which are easily dominated by vocals, drops, and mastering dynamics.
func (a *OnsetAccumulator) feedSpectralFlux(samples []float32) {
	if a.fluxSTFT == nil || len(samples) == 0 {
		return
	}
	a.fluxPending = append(a.fluxPending, samples...)
	if len(a.fluxPending) < a.fluxSTFT.WindowSize {
		return
	}
	frames := (len(a.fluxPending)-a.fluxSTFT.WindowSize)/a.fluxSTFT.HopSize + 1
	consumed := (frames-1)*a.fluxSTFT.HopSize + a.fluxSTFT.WindowSize
	_ = a.fluxSTFT.Frames(a.fluxPending[:consumed], func(spectrum []complex128) error {
		magnitudes := make([]float64, len(spectrum))
		flux := 0.0
		for index := 1; index < len(spectrum); index++ {
			magnitude := math.Hypot(real(spectrum[index]), imag(spectrum[index]))
			magnitudes[index] = magnitude
			if index < len(a.previousFluxSpectrum) && magnitude > a.previousFluxSpectrum[index] {
				flux += magnitude - a.previousFluxSpectrum[index]
			}
		}
		a.previousFluxSpectrum = magnitudes
		a.fluxEnvelope = append(a.fluxEnvelope, flux)
		return nil
	})
	a.fluxPending = a.fluxPending[frames*a.fluxSTFT.HopSize:]
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
	// Require transient evidence before selecting a periodic candidate. Without this, a purely
	// sustained source has no beats to find and the relative threshold below
	// adapts down onto its numerical noise floor, inventing a tempo. A
	// non-positive mean means the envelope never rose, which is the same
	// no-evidence case.
	crestFactor := 0.0
	if mean > 0 {
		crestFactor = maxValue(onsets) / mean
	}
	minimumCrestFactor := a.options.MinOnsetCrestFactor
	if minimumCrestFactor <= 0 {
		minimumCrestFactor = minOnsetCrestFactor
	}
	if mean <= 0 || crestFactor < minimumCrestFactor {
		return Estimate{OnsetCrestFactor: crestFactor, AlgorithmVersion: AlgorithmVersion}
	}
	minBPM, maxBPM := resolveBounds(a.options)
	if a.options.Method == MethodMultiFeatureConsensus {
		return estimateMultiFeatureConsensus(
			onsets,
			a.fluxEnvelope,
			float64(a.sampleRate)/float64(a.hop),
			float64(a.sampleRate)/float64(a.fluxSTFT.HopSize),
			minBPM,
			maxBPM,
			crestFactor,
		)
	}
	if a.options.Method == MethodOnsetAutocorrelation {
		return estimateOnsetAutocorrelation(onsets, float64(a.sampleRate)/float64(a.hop), minBPM, maxBPM, crestFactor)
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
		OnsetCrestFactor: crestFactor,
		Known:            true,
		AlgorithmVersion: AlgorithmVersion,
	}
}

// estimateOnsetAutocorrelation scores repeating patterns across the complete
// onset envelope. It is less sensitive to a single off-beat transient than
// consecutive-peak voting, but remains a Phase 0 candidate until it passes
// tuning and held-out evaluation.
func estimateOnsetAutocorrelation(onsets []float64, envelopeRate, minBPM, maxBPM, crestFactor float64) Estimate {
	const stride = 4
	if envelopeRate <= 0 || minBPM <= 0 || maxBPM < minBPM {
		return Estimate{OnsetCrestFactor: crestFactor, AlgorithmVersion: AlgorithmVersion}
	}
	values := make([]float64, 0, (len(onsets)+stride-1)/stride)
	for start := 0; start < len(onsets); start += stride {
		end := min(start+stride, len(onsets))
		sum := 0.0
		for _, value := range onsets[start:end] {
			sum += value
		}
		values = append(values, sum/float64(end-start))
	}
	if len(values) < 8 {
		return Estimate{OnsetCrestFactor: crestFactor, AlgorithmVersion: AlgorithmVersion}
	}

	rate := envelopeRate / stride
	minLag := max(1, int(math.Ceil(60*rate/maxBPM)))
	maxLag := min(len(values)-1, int(math.Floor(60*rate/minBPM)))
	if minLag > maxLag {
		return Estimate{OnsetCrestFactor: crestFactor, AlgorithmVersion: AlgorithmVersion}
	}
	mean := 0.0
	for _, value := range values {
		mean += value
	}
	mean /= float64(len(values))
	for i := range values {
		values[i] -= mean
	}

	type candidate struct {
		lag   int
		score float64
	}
	best := candidate{score: -math.MaxFloat64}
	runnerUp := candidate{score: -math.MaxFloat64}
	for lag := minLag; lag <= maxLag; lag++ {
		score := normalizedAutocorrelation(values, lag)
		if 2*lag < len(values) {
			score += 0.45 * normalizedAutocorrelation(values, 2*lag)
		}
		if 3*lag < len(values) {
			score += 0.20 * normalizedAutocorrelation(values, 3*lag)
		}
		current := candidate{lag: lag, score: score}
		if current.score > best.score {
			runnerUp = best
			best = current
		} else if current.score > runnerUp.score {
			runnerUp = current
		}
	}
	if best.lag == 0 || best.score <= 0 {
		return Estimate{OnsetCrestFactor: crestFactor, AlgorithmVersion: AlgorithmVersion}
	}
	margin := best.score - runnerUp.score
	confidence := math.Max(0, math.Min(1, margin/math.Max(0.05, math.Abs(best.score))))
	return Estimate{
		BPM:              60 * rate / float64(best.lag),
		Confidence:       confidence,
		OnsetCrestFactor: crestFactor,
		Known:            true,
		AlgorithmVersion: AlgorithmVersion,
	}
}

func normalizedAutocorrelation(values []float64, lag int) float64 {
	if lag <= 0 || lag >= len(values) {
		return 0
	}
	product, leftEnergy, rightEnergy := 0.0, 0.0, 0.0
	for index := lag; index < len(values); index++ {
		left, right := values[index], values[index-lag]
		product += left * right
		leftEnergy += left * left
		rightEnergy += right * right
	}
	if leftEnergy <= 0 || rightEnergy <= 0 {
		return 0
	}
	return product / math.Sqrt(leftEnergy*rightEnergy)
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
