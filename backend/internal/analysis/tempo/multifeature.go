package tempo

import "math"

type periodicityCandidate struct {
	bpm   float64
	score float64
}

// estimateMultiFeatureConsensus fuses two independently computed onset
// representations. Each representation supplies whole-track periodicity
// evidence; sixteen-second sections then vote only for their strongest tempo,
// limiting the influence of intros, breakdowns, and isolated transients.
func estimateMultiFeatureConsensus(energy, flux []float64, energyRate, fluxRate, minBPM, maxBPM, crestFactor float64) Estimate {
	energyCandidates := periodicityCandidates(energy, energyRate, minBPM, maxBPM)
	fluxCandidates := periodicityCandidates(flux, fluxRate, minBPM, maxBPM)
	if len(energyCandidates) == 0 || len(fluxCandidates) == 0 {
		return Estimate{OnsetCrestFactor: crestFactor, AlgorithmVersion: AlgorithmVersion}
	}

	votes := map[int]float64{}
	addCandidates := func(candidates []periodicityCandidate, weight float64) {
		best := 1e-12
		for _, candidate := range candidates {
			best = math.Max(best, candidate.score)
		}
		for _, candidate := range candidates {
			bucket := int(math.Round(candidate.bpm * 2))
			votes[bucket] += weight * candidate.score / best
		}
	}
	addCandidates(energyCandidates, 0.55)
	addCandidates(fluxCandidates, 0.45)
	addSectionVotes(votes, energy, energyRate, minBPM, maxBPM, 0.20)
	addSectionVotes(votes, flux, fluxRate, minBPM, maxBPM, 0.20)

	bestBucket, bestVote := 0, -math.MaxFloat64
	runnerUpVote := -math.MaxFloat64
	for bucket, vote := range votes {
		if vote > bestVote || (vote == bestVote && bucket < bestBucket) {
			runnerUpVote = bestVote
			bestBucket, bestVote = bucket, vote
		} else if vote > runnerUpVote {
			runnerUpVote = vote
		}
	}
	if bestBucket == 0 || bestVote <= 0 {
		return Estimate{OnsetCrestFactor: crestFactor, AlgorithmVersion: AlgorithmVersion}
	}

	primary := float64(bestBucket) / 2
	alternate := 0.0
	for bucket, vote := range votes {
		if bucket == bestBucket || math.Abs(float64(bucket-bestBucket)) < 2 {
			continue
		}
		if alternate == 0 || vote > runnerUpVote {
			alternate = float64(bucket) / 2
			runnerUpVote = vote
		}
	}
	if alternate == 0 {
		if primary >= 120 {
			alternate = primary / 2
		} else {
			alternate = primary * 2
		}
	}
	margin := bestVote - math.Max(0, runnerUpVote)
	confidence := math.Max(0, math.Min(1, margin/math.Max(0.05, bestVote)))
	return Estimate{
		BPM:              primary,
		Alternate:        alternate,
		Confidence:       confidence,
		Stability:        sectionAgreement(energy, energyRate, minBPM, maxBPM, primary),
		OnsetCrestFactor: crestFactor,
		Known:            true,
		AlgorithmVersion: AlgorithmVersion,
	}
}

func periodicityCandidates(values []float64, rate, minBPM, maxBPM float64) []periodicityCandidate {
	if rate <= 0 || len(values) < 8 {
		return nil
	}
	stride := max(1, int(math.Round(rate/125)))
	collapsed := make([]float64, 0, (len(values)+stride-1)/stride)
	for start := 0; start < len(values); start += stride {
		end := min(start+stride, len(values))
		sum := 0.0
		for _, value := range values[start:end] {
			sum += value
		}
		collapsed = append(collapsed, sum/float64(end-start))
	}
	rate /= float64(stride)
	if len(collapsed) < 8 {
		return nil
	}
	mean := 0.0
	for _, value := range collapsed {
		mean += value
	}
	mean /= float64(len(collapsed))
	for index := range collapsed {
		collapsed[index] -= mean
	}
	minLag := max(1, int(math.Ceil(60*rate/maxBPM)))
	maxLag := min(len(collapsed)-1, int(math.Floor(60*rate/minBPM)))
	if minLag > maxLag {
		return nil
	}
	candidates := make([]periodicityCandidate, 0, maxLag-minLag+1)
	for lag := minLag; lag <= maxLag; lag++ {
		score := normalizedAutocorrelation(collapsed, lag)
		if 2*lag < len(collapsed) {
			score += 0.45 * normalizedAutocorrelation(collapsed, 2*lag)
		}
		if 3*lag < len(collapsed) {
			score += 0.20 * normalizedAutocorrelation(collapsed, 3*lag)
		}
		if score > 0 {
			candidates = append(candidates, periodicityCandidate{bpm: 60 * rate / float64(lag), score: score})
		}
	}
	return candidates
}

func addSectionVotes(votes map[int]float64, values []float64, rate, minBPM, maxBPM, weight float64) {
	sectionLength := max(1, int(math.Round(16*rate)))
	sections := 0
	for start := 0; start+sectionLength/2 <= len(values); start += sectionLength {
		end := min(start+sectionLength, len(values))
		candidates := periodicityCandidates(values[start:end], rate, minBPM, maxBPM)
		if len(candidates) == 0 {
			continue
		}
		best := candidates[0]
		for _, candidate := range candidates[1:] {
			if candidate.score > best.score {
				best = candidate
			}
		}
		votes[int(math.Round(best.bpm*2))] += weight
		sections++
	}
	if sections == 0 {
		return
	}
}

func sectionAgreement(values []float64, rate, minBPM, maxBPM, primary float64) float64 {
	sectionLength := max(1, int(math.Round(16*rate)))
	matched, total := 0, 0
	for start := 0; start+sectionLength/2 <= len(values); start += sectionLength {
		end := min(start+sectionLength, len(values))
		candidates := periodicityCandidates(values[start:end], rate, minBPM, maxBPM)
		if len(candidates) == 0 {
			continue
		}
		best := candidates[0]
		for _, candidate := range candidates[1:] {
			if candidate.score > best.score {
				best = candidate
			}
		}
		total++
		if math.Abs(best.bpm-primary) <= 1 {
			matched++
		}
	}
	if total == 0 {
		return 0
	}
	return float64(matched) / float64(total)
}
