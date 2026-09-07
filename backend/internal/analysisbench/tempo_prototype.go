package analysisbench

import (
	"math"
)

// TempoEstimate is an experimental Phase 0 result. Known is false when the
// onset evidence is insufficient; callers must never convert that into a
// default BPM.
type TempoEstimate struct {
	BPM        float64 `json:"bpm,omitempty"`
	Confidence float64 `json:"confidence"`
	Known      bool    `json:"known"`
}

// EstimatePrototypeTempo is a small, isolated onset-interval baseline. It is
// intentionally not wired into the catalog or deck state. Its value is making
// half/double behavior and unknown handling measurable before Phase 2 chooses
// a production tempogram/candidate-scoring design.
func EstimatePrototypeTempo(samples []float32, sampleRate, channels int) TempoEstimate {
	if sampleRate <= 0 || channels <= 0 || len(samples) < sampleRate*channels/2 {
		return TempoEstimate{}
	}
	frames := len(samples) / channels
	window := max(1, sampleRate/200) // 5 ms energy window
	hop := max(1, sampleRate/1000)   // 1 ms onset resolution
	var envelope []float64
	var positions []int
	for start := 0; start+window <= frames; start += hop {
		energy := 0.0
		for frame := start; frame < start+window; frame++ {
			mono := 0.0
			for channel := 0; channel < channels; channel++ {
				mono += float64(samples[frame*channels+channel])
			}
			mono /= float64(channels)
			energy += mono * mono
		}
		envelope = append(envelope, math.Sqrt(energy/float64(window)))
		positions = append(positions, start)
	}
	if len(envelope) < 3 {
		return TempoEstimate{}
	}
	maxEnergy := 0.0
	for _, energy := range envelope {
		maxEnergy = maxFloat(maxEnergy, energy)
	}
	if maxEnergy < 1e-7 {
		return TempoEstimate{}
	}
	onsets := make([]float64, len(envelope))
	for index := 1; index < len(envelope); index++ {
		onsets[index] = maxFloat(0, envelope[index]-envelope[index-1])
	}
	mean, deviation := meanAndDeviation(onsets)
	threshold := mean + deviation*1.5
	minimumDistance := max(1, int(math.Round(0.10*float64(sampleRate)/float64(hop))))
	peaks := make([]int, 0)
	for index := 1; index+1 < len(onsets); index++ {
		if onsets[index] < threshold || onsets[index] < onsets[index-1] || onsets[index] < onsets[index+1] {
			continue
		}
		if len(peaks) > 0 && index-peaks[len(peaks)-1] < minimumDistance {
			if onsets[index] > onsets[peaks[len(peaks)-1]] {
				peaks[len(peaks)-1] = index
			}
			continue
		}
		peaks = append(peaks, index)
	}
	if len(peaks) < 3 {
		return TempoEstimate{}
	}

	type candidate struct {
		count int
		sum   float64
	}
	candidates := make(map[int]candidate)
	intervals := 0
	for index := 1; index < len(peaks); index++ {
		deltaFrames := positions[peaks[index]] - positions[peaks[index-1]]
		if deltaFrames <= 0 {
			continue
		}
		bpm := normalizePrototypeTempo(60 * float64(sampleRate) / float64(deltaFrames))
		if bpm == 0 {
			continue
		}
		bucket := int(math.Round(bpm * 10))
		entry := candidates[bucket]
		entry.count++
		entry.sum += bpm
		candidates[bucket] = entry
		intervals++
	}
	if intervals == 0 {
		return TempoEstimate{}
	}
	bestBucket := 0
	best := candidate{}
	for bucket, entry := range candidates {
		if entry.count > best.count || (entry.count == best.count && bucket < bestBucket) {
			bestBucket, best = bucket, entry
		}
	}
	return TempoEstimate{
		BPM:        best.sum / float64(best.count),
		Confidence: float64(best.count) / float64(intervals),
		Known:      true,
	}
}

func normalizePrototypeTempo(bpm float64) float64 {
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

func meanAndDeviation(values []float64) (float64, float64) {
	mean := 0.0
	for _, value := range values {
		mean += value
	}
	mean /= float64(len(values))
	variance := 0.0
	for _, value := range values {
		delta := value - mean
		variance += delta * delta
	}
	return mean, math.Sqrt(variance / float64(len(values)))
}

func maxFloat(left, right float64) float64 {
	if left > right {
		return left
	}
	return right
}
