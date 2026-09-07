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
	if sampleRate <= 0 || len(samples) < sampleRate/2 {
		return Estimate{AlgorithmVersion: AlgorithmVersion}
	}
	window, hop := max(1, sampleRate/200), max(1, sampleRate/1000)
	envelope, positions := make([]float64, 0, len(samples)/hop), make([]int, 0, len(samples)/hop)
	for start := 0; start+window <= len(samples); start += hop {
		energy := 0.0
		for i := start; i < start+window; i++ {
			value := float64(samples[i])
			energy += value * value
		}
		envelope, positions = append(envelope, math.Sqrt(energy/float64(window))), append(positions, start)
	}
	if len(envelope) < 3 || maxValue(envelope) < 1e-7 {
		return Estimate{AlgorithmVersion: AlgorithmVersion}
	}
	onsets := make([]float64, len(envelope))
	for i := 1; i < len(envelope); i++ {
		onsets[i] = maxFloat(0, envelope[i]-envelope[i-1])
	}
	mean, deviation := meanDeviation(onsets)
	threshold := mean + 1.5*deviation
	minDistance := max(1, int(math.Round(.1*float64(sampleRate)/float64(hop))))
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
		delta := positions[peaks[i]] - positions[peaks[i-1]]
		if delta <= 0 {
			continue
		}
		bpm := normalize(60 * float64(sampleRate) / float64(delta))
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
