// Package tempo provides the first backend-owned tempo candidate estimator.
package tempo

import "math"

// Estimate retains an alternate candidate slot for later tempogram scoring.
// Known is false when evidence is insufficient; callers must not invent BPM.
type Estimate struct {
	BPM, Confidence, Alternate float64
	Known                      bool
	AlgorithmVersion           string
}

const AlgorithmVersion = "tempo-v1-onset-interval"

// EstimatePCM estimates a static tempo from a mono normalized PCM segment.
// It is deliberately fractional and conservative; Phase 0 corpus gates still
// determine whether its algorithm is adequate for professional release.
func EstimatePCM(samples []float32, sampleRate int) Estimate {
	accumulator := NewOnsetAccumulator(sampleRate)
	accumulator.Feed(samples)
	return accumulator.Estimate()
}

// OnsetAccumulator retains only a low-rate onset envelope; Feed may be called
// with bounded decoded PCM chunks from the shared analysis service.
type OnsetAccumulator struct {
	sampleRate, window, hop, nextStart int
	pending                            []float32
	envelope                           []float64
	positions                          []int
}

func NewOnsetAccumulator(sampleRate int) *OnsetAccumulator {
	return &OnsetAccumulator{sampleRate: sampleRate, window: max(1, sampleRate/200), hop: max(1, sampleRate/1000)}
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
	}
	candidates := map[int]candidate{}
	total := 0
	for i := 1; i < len(peaks); i++ {
		delta := a.positions[peaks[i]] - a.positions[peaks[i-1]]
		if delta <= 0 {
			continue
		}
		bpm := normalize(60 * float64(a.sampleRate) / float64(delta))
		if bpm == 0 {
			continue
		}
		bucket := int(math.Round(bpm * 10))
		current := candidates[bucket]
		current.count++
		current.sum += bpm
		candidates[bucket] = current
		total++
	}
	if total == 0 {
		return Estimate{AlgorithmVersion: AlgorithmVersion}
	}
	bestBucket, best := 0, candidate{}
	for bucket, value := range candidates {
		if value.count > best.count || value.count == best.count && bucket < bestBucket {
			bestBucket, best = bucket, value
		}
	}
	return Estimate{BPM: best.sum / float64(best.count), Confidence: float64(best.count) / float64(total), Known: true, AlgorithmVersion: AlgorithmVersion}
}
func normalize(bpm float64) float64 {
	for bpm < 90 {
		bpm *= 2
	}
	for bpm > 180 {
		bpm /= 2
	}
	if bpm < 90 || bpm > 180 {
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
