package tempo

import (
	"math"
	"testing"
)

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
