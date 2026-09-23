package tempo

import (
	"math"
	"sort"
)

type periodicityCandidate struct {
	bpm   float64
	score float64
}

// estimateMultiFeatureConsensus fuses two independently computed onset
// representations. Each representation supplies whole-track periodicity
// evidence; sixteen-second sections then vote only for their strongest tempo,
// limiting the influence of intros, breakdowns, and isolated transients.
func estimateMultiFeatureConsensus(energy, flux []float64, energyRate, fluxRate, minBPM, maxBPM, crestFactor float64, method Method) Estimate {
	findCandidates := periodicityCandidates
	if method == MethodMultiFeatureRefined {
		findCandidates = refinedPeriodicityCandidates
	}
	energyCandidates := findCandidates(energy, energyRate, minBPM, maxBPM)
	fluxCandidates := findCandidates(flux, fluxRate, minBPM, maxBPM)
	if len(energyCandidates) == 0 || len(fluxCandidates) == 0 {
		return Estimate{OnsetCrestFactor: crestFactor, AlgorithmVersion: AlgorithmVersion}
	}

	// Preserve a tenth-BPM grid through voting.  The 125 Hz periodicity
	// representation has integer lags roughly 2 BPM apart around 128 BPM;
	// rounding those lags to 0.5 BPM systematically misses otherwise stable
	// real-world tempos.  Candidate lag interpolation below supplies the
	// resolution, while this finer grid prevents the voter from discarding it.
	votes := map[int]float64{}
	addCandidates := func(candidates []periodicityCandidate, weight float64) {
		best := 1e-12
		for _, candidate := range candidates {
			best = math.Max(best, candidate.score)
		}
		for _, candidate := range candidates {
			bucket := int(math.Round(candidate.bpm * 10))
			votes[bucket] += weight * candidate.score / best
		}
	}
	addCandidates(energyCandidates, 0.55)
	addCandidates(fluxCandidates, 0.45)
	addSectionVotes(votes, energy, energyRate, minBPM, maxBPM, 0.20, findCandidates)
	addSectionVotes(votes, flux, fluxRate, minBPM, maxBPM, 0.20, findCandidates)

	primary, alternate, confidence := selectConsensusVotes(votes, method != MethodMultiFeatureConsensus)
	if method == MethodMultiFeatureClustered || method == MethodMultiFeatureRefined {
		primary, alternate, confidence = selectClusteredVotes(votes, 5)
	}
	if primary == 0 {
		return Estimate{OnsetCrestFactor: crestFactor, AlgorithmVersion: AlgorithmVersion}
	}
	return Estimate{
		BPM:              primary,
		Alternate:        alternate,
		Confidence:       confidence,
		Stability:        sectionAgreement(energy, energyRate, minBPM, maxBPM, primary, findCandidates),
		OnsetCrestFactor: crestFactor,
		Known:            true,
		AlgorithmVersion: AlgorithmVersion,
	}
}

// selectClusteredVotes measures disjoint neighborhoods, rather than counting
// almost-identical tenth-BPM estimates as competing rhythmic hypotheses. Each
// original vote contributes to at most one cluster. A fixed radius prevents
// chains of adjacent votes from merging distant tempos. Sorted accumulation
// keeps floating-point sums independent of Go map iteration order.
func selectClusteredVotes(votes map[int]float64, radius int) (primary, alternate, confidence float64) {
	buckets := make([]int, 0, len(votes))
	for bucket, vote := range votes {
		if vote > 0 {
			buckets = append(buckets, bucket)
		}
	}
	sort.Ints(buckets)
	clusters := make([]periodicityCandidate, 0, 2)
	for len(buckets) > 0 && len(clusters) < 2 {
		bestCenter, bestScore, bestWeighted := 0, 0.0, 0.0
		for _, center := range buckets {
			score, weighted := 0.0, 0.0
			for _, bucket := range buckets {
				if bucket >= center-radius && bucket <= center+radius {
					score += votes[bucket]
					weighted += float64(bucket) * votes[bucket]
				}
			}
			if score > bestScore {
				bestCenter, bestScore, bestWeighted = center, score, weighted
			}
		}
		if bestScore <= 0 {
			break
		}
		clusters = append(clusters, periodicityCandidate{bpm: math.Round(bestWeighted/bestScore/5) / 2, score: bestScore})
		remaining := buckets[:0]
		for _, bucket := range buckets {
			if bucket < bestCenter-radius || bucket > bestCenter+radius {
				remaining = append(remaining, bucket)
			}
		}
		buckets = remaining
	}
	if len(clusters) == 0 {
		return 0, 0, 0
	}
	primary, confidence = clusters[0].bpm, 1
	if len(clusters) > 1 {
		alternate = clusters[1].bpm
		confidence = math.Max(0, 1-clusters[1].score/clusters[0].score)
	}
	if alternate == 0 || alternate == primary {
		alternate = primary * 2
		if primary >= 120 {
			alternate = primary / 2
		}
		alternate = math.Round(alternate*2) / 2
	}
	return primary, alternate, confidence
}

// selectConsensusVotes keeps the confidence runner-up separate from the
// displayed alternative. Nearby competitors still reduce confidence even when
// rounding would display them as the same BPM.
func selectConsensusVotes(votes map[int]float64, halfBPMGrid bool) (primary, alternate, confidence float64) {
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
		return 0, 0, 0
	}

	primary = float64(bestBucket) / 10
	if halfBPMGrid {
		primary = math.Round(primary*2) / 2
	}
	alternateBucket, alternateVote := 0, -math.MaxFloat64
	for bucket, vote := range votes {
		if bucket == bestBucket || math.Abs(float64(bucket-bestBucket)) < 2 {
			continue
		}
		candidate := float64(bucket) / 10
		if halfBPMGrid {
			candidate = math.Round(candidate*2) / 2
		}
		if candidate == primary {
			continue
		}
		if vote > alternateVote || (vote == alternateVote && bucket < alternateBucket) {
			alternate, alternateBucket, alternateVote = candidate, bucket, vote
		}
	}
	if alternate == 0 {
		if primary >= 120 {
			alternate = primary / 2
		} else {
			alternate = primary * 2
		}
	}
	if halfBPMGrid {
		alternate = math.Round(alternate*2) / 2
	}
	margin := bestVote - math.Max(0, runnerUpVote)
	confidence = math.Max(0, math.Min(1, margin/math.Max(0.05, bestVote)))
	return primary, alternate, confidence
}

func periodicityCandidates(values []float64, rate, minBPM, maxBPM float64) []periodicityCandidate {
	return periodicityCandidatesWithRefinement(values, rate, minBPM, maxBPM, false)
}

func refinedPeriodicityCandidates(values []float64, rate, minBPM, maxBPM float64) []periodicityCandidate {
	return periodicityCandidatesWithRefinement(values, rate, minBPM, maxBPM, true)
}

func periodicityCandidatesWithRefinement(values []float64, rate, minBPM, maxBPM float64, refineCycles bool) []periodicityCandidate {
	if rate <= 0 || minBPM <= 0 || maxBPM < minBPM || len(values) < 8 {
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
	if refineCycles {
		// Measure guard lags outside the requested range so a peak at a
		// boundary still has two measured neighbors. Do not turn a monotonic
		// boundary slope into an invented one-sided peak.
		minLag = max(1, int(math.Floor(60*rate/maxBPM))-1)
		maxLag = min(len(collapsed)-1, int(math.Ceil(60*rate/minBPM))+1)
	}
	if minLag > maxLag {
		return nil
	}
	scores := make([]float64, maxLag-minLag+1)
	correlations := make(map[int]float64)
	correlation := func(lag int) float64 {
		if value, present := correlations[lag]; present {
			return value
		}
		value := normalizedAutocorrelation(collapsed, lag)
		correlations[lag] = value
		return value
	}
	for lag := minLag; lag <= maxLag; lag++ {
		score := correlation(lag)
		if 2*lag < len(collapsed) {
			score += 0.45 * correlation(2*lag)
		}
		if 3*lag < len(collapsed) {
			score += 0.20 * correlation(3*lag)
		}
		scores[lag-minLag] = score
	}

	// Retain only local maxima and refine each one with a quadratic fit.  A
	// peak between two discrete lags is common at this frame rate, especially
	// around club tempos.  The fit is bounded to its neighbouring samples so
	// it cannot invent a tempo outside measured evidence.
	candidates := make([]periodicityCandidate, 0, len(scores)/3)
	for index := 1; index+1 < len(scores); index++ {
		current := scores[index]
		if current <= 0 || current < scores[index-1] || current < scores[index+1] {
			continue
		}
		denominator := scores[index-1] - 2*current + scores[index+1]
		offset := 0.0
		if denominator < -1e-12 {
			offset = 0.5 * (scores[index-1] - scores[index+1]) / denominator
			offset = math.Max(-0.5, math.Min(0.5, offset))
		}
		lag := float64(minLag+index) + offset
		if refineCycles {
			lag = refinePeriodAcrossCycles(lag, len(collapsed), correlation)
		}
		bpm := 60 * rate / lag
		if refineCycles {
			// Allow one tenth-BPM voting bin of interpolation uncertainty at
			// an exact boundary, not the much wider half-BPM display grid.
			// Otherwise an out-of-range half-tempo (e.g. 89.75 for 179.5)
			// can round into the range and displace the in-range hypothesis.
			if bpm < minBPM-.1 || bpm > maxBPM+.1 {
				continue
			}
			bpm = math.Max(minBPM, math.Min(maxBPM, bpm))
		}
		candidates = append(candidates, periodicityCandidate{bpm: bpm, score: current})
	}
	return candidates
}

// refinePeriodAcrossCycles uses longer observed cycles for sub-frame timing.
// An interpolation error at four beats is divided by four when estimating a
// single beat. Each search stays near the original measured hypothesis and
// requires a positive, two-sided autocorrelation maximum.
func refinePeriodAcrossCycles(period float64, length int, correlation func(int) float64) float64 {
	weighted, weight := 0.0, 0.0
	for multiple := 2; multiple <= 4; multiple++ {
		center := int(math.Round(period * float64(multiple)))
		radius := multiple
		bestLag, bestScore := 0, 0.0
		for lag := max(2, center-radius); lag <= min(length-2, center+radius); lag++ {
			score := correlation(lag)
			if score > bestScore && score >= correlation(lag-1) && score >= correlation(lag+1) {
				bestLag, bestScore = lag, score
			}
		}
		if bestLag == 0 {
			continue
		}
		left, right := correlation(bestLag-1), correlation(bestLag+1)
		denominator := left - 2*bestScore + right
		offset := 0.0
		if denominator < -1e-12 {
			offset = math.Max(-.5, math.Min(.5, .5*(left-right)/denominator))
		}
		w := bestScore * float64(multiple)
		weighted += w * (float64(bestLag) + offset) / float64(multiple)
		weight += w
	}
	if weight == 0 {
		return period
	}
	return weighted / weight
}

type periodicityFinder func([]float64, float64, float64, float64) []periodicityCandidate

func addSectionVotes(votes map[int]float64, values []float64, rate, minBPM, maxBPM, weight float64, findCandidates periodicityFinder) {
	sectionLength := max(1, int(math.Round(16*rate)))
	sections := 0
	for start := 0; start+sectionLength/2 <= len(values); start += sectionLength {
		end := min(start+sectionLength, len(values))
		candidates := findCandidates(values[start:end], rate, minBPM, maxBPM)
		if len(candidates) == 0 {
			continue
		}
		best := candidates[0]
		for _, candidate := range candidates[1:] {
			if candidate.score > best.score {
				best = candidate
			}
		}
		votes[int(math.Round(best.bpm*10))] += weight
		sections++
	}
	if sections == 0 {
		return
	}
}

func sectionAgreement(values []float64, rate, minBPM, maxBPM, primary float64, findCandidates periodicityFinder) float64 {
	sectionLength := max(1, int(math.Round(16*rate)))
	matched, total := 0, 0
	for start := 0; start+sectionLength/2 <= len(values); start += sectionLength {
		end := min(start+sectionLength, len(values))
		candidates := findCandidates(values[start:end], rate, minBPM, maxBPM)
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
