package tempo

import "math"

// estimateBeatIntervalConsistency selects among independently measured
// periodicity hypotheses using only observed amplitude-onset peak intervals.
// It intentionally does not inspect a generated beat grid: a grid is created
// after tempo selection and would make this score circular.
func estimateBeatIntervalConsistency(onsets []float64, positions []int, flux []float64, onsetRate, fluxRate, sampleRate, minBPM, maxBPM, crestFactor float64) Estimate {
	// Keep at most one observed peak per 100 ms. Without this refractory
	// period, a dense burst produces a quadratic number of near-duplicate
	// pairs and makes a full-track benchmark impractical.
	peaks := onsetPeaks(onsets, max(1, int(math.Round(0.1*onsetRate))))
	if len(peaks) < 4 || len(positions) != len(onsets) || sampleRate <= 0 {
		return Estimate{OnsetCrestFactor: crestFactor, AlgorithmVersion: AlgorithmVersion}
	}
	candidates := append(periodicityCandidates(onsets, onsetRate, minBPM, maxBPM), periodicityCandidates(flux, fluxRate, minBPM, maxBPM)...)
	if len(candidates) == 0 {
		return Estimate{OnsetCrestFactor: crestFactor, AlgorithmVersion: AlgorithmVersion}
	}

	bestScore := 0.0
	for _, candidate := range candidates {
		bestScore = math.Max(bestScore, candidate.score)
	}
	best, alternate := intervalCandidate{}, intervalCandidate{}
	for _, candidate := range candidates {
		intervalScore, observations, refinedBPM := intervalConsistencyScore(peaks, positions, sampleRate, candidate.bpm)
		if observations < 4 {
			continue
		}
		if refinedBPM < minBPM || refinedBPM > maxBPM {
			continue
		}
		// Preserve measured periodicity strength while selection is based on
		// actual peak timing. Neither source is a rounded-vote result.
		score := intervalScore * (0.5 + 0.5*candidate.score/maxFloat(bestScore, 1e-12))
		current := intervalCandidate{bpm: refinedBPM, score: score, consistency: intervalScore}
		if betterIntervalCandidate(current, best) {
			alternate, best = best, current
		} else if math.Abs(current.bpm-best.bpm) >= 1 && betterIntervalCandidate(current, alternate) {
			alternate = current
		}
	}
	if best.score <= 0 {
		return Estimate{OnsetCrestFactor: crestFactor, AlgorithmVersion: AlgorithmVersion}
	}
	confidence := best.score
	if alternate.score > 0 {
		confidence = maxFloat(0, math.Min(1, (best.score-alternate.score)/best.score))
	}
	return Estimate{BPM: best.bpm, Alternate: alternate.bpm, Confidence: confidence, Stability: best.consistency, OnsetCrestFactor: crestFactor, Known: true, AlgorithmVersion: AlgorithmVersion}
}

type intervalCandidate struct {
	bpm, score, consistency float64
}

func betterIntervalCandidate(candidate, incumbent intervalCandidate) bool {
	return candidate.score > incumbent.score+1e-12 || (math.Abs(candidate.score-incumbent.score) <= 1e-12 && candidate.bpm < incumbent.bpm)
}

func onsetPeaks(onsets []float64, minimumDistance int) []int {
	mean, deviation := meanDeviation(onsets)
	threshold := mean + 1.5*deviation
	peaks := make([]int, 0, len(onsets)/8)
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
	return peaks
}

func intervalConsistencyScore(peaks, positions []int, sampleRate, bpm float64) (float64, int, float64) {
	if bpm <= 0 || sampleRate <= 0 {
		return 0, 0, 0
	}
	period := 60 * sampleRate / bpm
	if period <= 0 {
		return 0, 0, 0
	}
	const maximumSkippedBeats = 8
	const relativeTolerance = 0.08
	score, observations, weightedPeriod, totalWeight := 0.0, 0, 0.0, 0.0
	for left := 0; left < len(peaks); left++ {
		for right := left + 1; right < len(peaks); right++ {
			delta := float64(positions[peaks[right]] - positions[peaks[left]])
			cycles := int(math.Round(delta / period))
			if cycles > maximumSkippedBeats {
				break
			}
			if cycles < 1 {
				continue
			}
			residual := math.Abs(delta-float64(cycles)*period) / (float64(cycles) * period)
			weight := math.Exp(-0.5 * math.Pow(residual/relativeTolerance, 2))
			score += weight
			weightedPeriod += weight * delta / float64(cycles)
			totalWeight += weight
			observations++
		}
	}
	if observations == 0 {
		return 0, 0, 0
	}
	return score / float64(observations), observations, 60 * sampleRate / (weightedPeriod / totalWeight)
}
