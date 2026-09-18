package tempo

import (
	"math"
	"testing"

	"github.com/ajbergh/viib-mediahub/internal/analysisbench"
)

func TestClusteredVotesCombineNearbyEvidence(t *testing.T) {
	votes := map[int]float64{1278: .6, 1281: .6, 1600: 1}
	for i := 0; i < 100; i++ {
		bpm, alternate, confidence := selectClusteredVotes(votes, 5)
		if bpm != 128 || alternate != 160 || math.Abs(confidence-1.0/6.0) > 1e-12 {
			t.Fatalf("clustered votes: %v %v %v", bpm, alternate, confidence)
		}
	}
}

func TestClusteredVotesDoNotMergeDistantTemposThroughChains(t *testing.T) {
	votes := map[int]float64{1000: 1, 1005: 1, 1010: 1, 1015: 1, 1020: 1, 1280: 4}
	bpm, _, _ := selectClusteredVotes(votes, 5)
	if bpm != 128 {
		t.Fatalf("transitive chain overwhelmed isolated evidence: %v", bpm)
	}
}

func TestClusteredMethodRecognizesFractionalTempoAndRefusesSilence(t *testing.T) {
	options := DefaultOptions()
	options.Method = MethodMultiFeatureClustered
	fixture, err := analysisbench.NewNoisyClickTrack("clustered", 128.3, 12, 44100, 1, .2)
	if err != nil {
		t.Fatal(err)
	}
	estimate := EstimatePCMWithOptions(fixture.Samples, fixture.SampleRate, options)
	if !estimate.Known || math.Abs(estimate.BPM-128.3) > .5 || estimate.Alternate == estimate.BPM {
		t.Fatalf("fractional estimate=%+v", estimate)
	}
	if estimate := EstimatePCMWithOptions(make([]float32, 44100*8), 44100, options); estimate.Known {
		t.Fatalf("silence=%+v", estimate)
	}
}

func TestRefinedMethodRecognizesRangeEdges(t *testing.T) {
	for _, rate := range []int{22050, 44100} {
		for _, bpm := range []float64{90, 90.5, 128.3, 179.5, 180} {
			fixture, err := analysisbench.NewClickTrack("boundary", bpm, 16, rate, 1)
			if err != nil {
				t.Fatal(err)
			}
			options := DefaultOptions()
			options.Method = MethodMultiFeatureRefined
			estimate := EstimatePCMWithOptions(fixture.Samples, rate, options)
			if !estimate.Known || math.Abs(estimate.BPM-bpm) > .5 || estimate.BPM < options.MinBPM || estimate.BPM > options.MaxBPM {
				t.Errorf("rate=%d target=%v estimate=%+v", rate, bpm, estimate)
			}
		}
	}
}

func TestRefinedPeriodicityRejectsOutOfRangePeaks(t *testing.T) {
	const rate = 125.0
	values := make([]float64, 4000)
	for i := range values {
		// Smooth periodic evidence whose closest peak lies outside 90–100.
		values[i] = 1 + math.Cos(2*math.Pi*float64(i)*89/(60*rate))
	}
	if candidates := refinedPeriodicityCandidates(values, rate, 90, 100); len(candidates) != 0 {
		t.Fatalf("out-of-range peak became an artificial boundary candidate: %+v", candidates)
	}
}

func TestPeriodicityCandidatesRefineOffGridTempo(t *testing.T) {
	const (
		rate   = 125.0
		target = 128.0
	)
	values := make([]float64, 4000)
	for index := range values {
		phase := math.Mod(float64(index)*target/(60*rate), 1)
		if phase < 0.04 {
			values[index] = 1
		}
	}
	candidates := periodicityCandidates(values, rate, 90, 180)
	closest := math.Inf(1)
	for _, candidate := range candidates {
		closest = math.Min(closest, math.Abs(candidate.bpm-target))
	}
	if closest > 0.25 {
		t.Fatalf("expected a refined periodicity candidate within 0.25 BPM of %v, closest error was %v", target, closest)
	}
}

func TestConsensusVoteRankingIsDeterministic(t *testing.T) {
	// A tied primary and tied alternatives must resolve by BPM, regardless of
	// map iteration order. The nearby 128.1 competitor still lowers confidence.
	votes := map[int]float64{1280: 1, 1281: 1, 1000: .6, 1400: .6, 1600: .3}
	for i := 0; i < 1000; i++ {
		bpm, alternate, confidence := selectConsensusVotes(votes, true)
		if bpm != 128 || alternate != 100 || confidence != 0 {
			t.Fatalf("iteration %d: BPM=%v alternate=%v confidence=%v", i, bpm, alternate, confidence)
		}
	}
}

func TestConsensusAlternateDoesNotDuplicateRoundedPrimary(t *testing.T) {
	for _, grid := range []bool{false, true} {
		bpm, alternate, confidence := selectConsensusVotes(map[int]float64{1280: 1, 1282: .9, 1400: .4}, grid)
		wantAlternate := 128.2
		if grid {
			wantAlternate = 140
		}
		if bpm != 128 || alternate != wantAlternate || math.Abs(confidence-.1) > 1e-12 {
			t.Fatalf("half grid=%v: BPM=%v alternate=%v confidence=%v", grid, bpm, alternate, confidence)
		}
	}
}

func TestConsensusConfidenceIncludesNearbyRunnerUp(t *testing.T) {
	_, _, confidence := selectConsensusVotes(map[int]float64{1280: 1, 1281: .9, 1400: .2}, false)
	if math.Abs(confidence-.1) > 1e-12 {
		t.Fatalf("confidence=%v, nearby runner-up must not be replaced by distant alternative", confidence)
	}
}
