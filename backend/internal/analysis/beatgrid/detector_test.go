package beatgrid

import (
	"math"
	"testing"
)

func TestBuildDynamicRetainsExactTempoAnchors(t *testing.T) {
	grid, err := BuildDynamic([]TempoAnchor{{Time: 0, BPM: 120}, {Time: 1.5, BPM: 60}}, 4, 4)
	if err != nil {
		t.Fatal(err)
	}
	want := []float64{0, .5, 1, 1.5, 2.5, 3.5}
	if !closeEnough(grid.Beats, want) {
		t.Fatalf("beats = %#v, want %#v", grid.Beats, want)
	}
}

func TestPhaseAccumulatorFindsNonZeroBeatPhase(t *testing.T) {
	accumulator := NewPhaseAccumulator(1_000)
	// Four 120 BPM impulses at 125 ms phase.  Each 5 ms window has one
	// energetic sample, exercising chunk-independent onset accumulation.
	samples := make([]float32, 2_000)
	for _, index := range []int{125, 625, 1125, 1625} {
		samples[index] = 1
	}
	accumulator.Feed(samples[:731])
	accumulator.Feed(samples[731:])
	grid, err := accumulator.Build(120, 1.9, 4)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(grid.Beats[0]-.125) > .011 {
		t.Fatalf("first beat = %f, want roughly .125", grid.Beats[0])
	}
}
