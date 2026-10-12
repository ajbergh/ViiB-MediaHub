package tempo

import "math"

// estimatePhaseSupport tests whether a periodicity hypothesis explains actual
// onset mass without inserting beats. A slower alias explains fewer events;
// a faster alias has empty/weak beat positions. Both reduce the support score.
// This is a static-tempo experiment, not a meter or downbeat measurement.
func estimatePhaseSupport(energy, flux []float64, energyRate, fluxRate, minBPM, maxBPM, crest float64) Estimate {
	unknown := Estimate{OnsetCrestFactor: crest, AlgorithmVersion: AlgorithmVersion}
	energyCandidates := refinedPeriodicityCandidates(energy, energyRate, minBPM, maxBPM)
	fluxCandidates := refinedPeriodicityCandidates(flux, fluxRate, minBPM, maxBPM)
	if len(energyCandidates) == 0 || len(fluxCandidates) == 0 {
		return unknown
	}
	energyPeaks := onsetPeaks(energy, max(1, int(math.Round(.05*energyRate))))
	fluxPeaks := onsetPeaks(flux, max(1, int(math.Round(.05*fluxRate))))
	best, alternate := intervalCandidate{}, intervalCandidate{}
	scored := make([]intervalCandidate, 0, len(energyCandidates)+len(fluxCandidates))
	for _, candidate := range append(energyCandidates, fluxCandidates...) {
		// A candidate must have periodicity evidence in both representations.
		energyCorrelation := nearbyPeriodicityScore(energyCandidates, candidate.bpm)
		fluxCorrelation := nearbyPeriodicityScore(fluxCandidates, candidate.bpm)
		if energyCorrelation <= 0 || fluxCorrelation <= 0 {
			continue
		}
		e := phaseOnsetSupport(energy, energyPeaks, energyRate, candidate.bpm)
		f := phaseOnsetSupport(flux, fluxPeaks, fluxRate, candidate.bpm)
		// Correlation strength can break ties, but cannot override absent phase
		// support. 1.65 is the sum of the periodicity harmonic weights.
		support := math.Sqrt(e * f)
		score := support * (.5 + .5*math.Min(1, math.Sqrt(energyCorrelation*fluxCorrelation)/1.65))
		current := intervalCandidate{bpm: candidate.bpm, score: score, consistency: support}
		scored = append(scored, current)
		if betterIntervalCandidate(current, best) {
			best = current
		}
	}
	if best.score <= 0 {
		return unknown
	}
	for _, candidate := range scored {
		if math.Abs(candidate.bpm-best.bpm) >= 1 && betterIntervalCandidate(candidate, alternate) {
			alternate = candidate
		}
	}
	confidence := 0.0
	if alternate.score > 0 {
		confidence = math.Max(0, 1-alternate.score/best.score)
	}
	return Estimate{BPM: best.bpm, Alternate: alternate.bpm, Confidence: confidence, Stability: best.consistency, OnsetCrestFactor: crest, Known: true, AlgorithmVersion: AlgorithmVersion}
}

func nearbyPeriodicityScore(candidates []periodicityCandidate, bpm float64) float64 {
	best := 0.0
	for _, candidate := range candidates {
		if math.Abs(candidate.bpm-bpm) <= math.Max(1, .02*bpm) {
			best = math.Max(best, candidate.score)
		}
	}
	return best
}

// phaseOnsetSupport maximizes a circular onset histogram over beat phase.
// The score combines the fraction of observed onset mass explained with mean
// mass per expected beat. A one-to-one beat train scores one; every-other-beat
// and twice-as-fast hypotheses score less. Histogram work is bounded per
// candidate, rather than comparing every pair of peaks in long recordings.
func phaseOnsetSupport(values []float64, peaks []int, rate, bpm float64) float64 {
	if rate <= 0 || bpm <= 0 || len(peaks) < 4 {
		return 0
	}
	period := 60 * rate / bpm
	span := float64(peaks[len(peaks)-1] - peaks[0])
	if span < 3*period {
		return 0
	}
	bins := min(2048, max(16, int(math.Ceil(period))))
	histogram := make([]float64, bins)
	total, maximum := 0.0, 0.0
	for _, peak := range peaks {
		weight := values[peak]
		phase := math.Mod(float64(peak)/period, 1) * float64(bins)
		lower := int(phase)
		fraction := phase - float64(lower)
		histogram[lower] += weight * (1 - fraction)
		histogram[(lower+1)%bins] += weight * fraction
		total += weight
		maximum = math.Max(maximum, weight)
	}
	if total <= 0 || maximum <= 0 {
		return 0
	}
	// At most 40 ms or 8% of a beat; this is timing tolerance, not a
	// subdivision or a generated phase claim exposed to consumers.
	radius := max(1, int(math.Ceil(math.Min(.04*rate/period, .08)*float64(bins))))
	aligned := 0.0
	for center := range histogram {
		mass := 0.0
		for delta := -radius; delta <= radius; delta++ {
			mass += histogram[(center+delta+bins)%bins] * (1 - math.Abs(float64(delta))/float64(radius+1))
		}
		aligned = math.Max(aligned, mass)
	}
	expectedBeats := span/period + 1
	return math.Min(1, aligned/total) * math.Min(1, aligned/(expectedBeats*maximum))
}
