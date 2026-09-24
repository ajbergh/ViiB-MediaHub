package features

import (
	"reflect"
	"testing"

	"github.com/ajbergh/viib-mediahub/internal/analysis/beatgrid"
)

func TestEnergyResultAndArtifactAreDeterministic(t *testing.T) {
	accumulator := NewAccumulator(100)
	samples := make([]float32, 1_000)
	for index := 400; index < len(samples); index++ {
		samples[index] = .8
	}
	accumulator.Feed(samples[:333])
	accumulator.Feed(samples[333:])
	result, err := accumulator.Result()
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Energy) != 20 || len(result.Sections) < 2 || result.Energy[0].Value >= result.Energy[len(result.Energy)-1].Value {
		t.Fatalf("unexpected measured structure: %#v", result)
	}
	first, err := result.Encode()
	if err != nil {
		t.Fatal(err)
	}
	second, err := result.Encode()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatal("artifact encoding is not deterministic")
	}
	decoded, err := Decode(first)
	if err != nil || !reflect.DeepEqual(decoded, result) {
		t.Fatalf("Decode() = %#v, %v; want %#v", decoded, err, result)
	}
	if decoded.LoudnessKind != LoudnessKind || decoded.PeakKind != PeakKind || decoded.ChannelScope != "mono" || decoded.Standard != "none" {
		t.Fatalf("artifact does not state measurement semantics: %#v", decoded)
	}
	if decoded.IntegratedLUFS != result.IntegratedLUFS || decoded.TruePeakDBFS != result.TruePeakDBFS {
		t.Fatalf("compatibility numeric aliases changed: %#v", decoded)
	}
}

func TestCueSuggestionsSnapToDownbeats(t *testing.T) {
	result := Result{Energy: []EnergyPoint{{Time: 0, Value: .2}}, Sections: []Section{{Start: .12, End: 3.9, Energy: .2}, {Start: 4.13, End: 8.1, Energy: .8}}}
	grid := &beatgrid.Grid{Beats: []float64{.1, .6, 1.1, 1.6, 2.1, 2.6, 3.1, 3.6, 4.1, 4.6, 5.1, 5.6, 6.1, 6.6, 7.1, 7.6, 8.1}, DownbeatIndices: []int{0, 8, 16}}
	result.AddCueSuggestions(grid)
	if len(result.CueSuggestions) != 3 || result.CueSuggestions[0].Position != .1 || result.CueSuggestions[2].Position != 4.1 {
		t.Fatalf("cue suggestions = %#v", result.CueSuggestions)
	}
	if result.CueSuggestions[0].Confidence >= .5 || result.CueSuggestions[0].Rationale == "" {
		t.Fatalf("inferred grid cue did not carry conservative rationale/confidence: %#v", result.CueSuggestions[0])
	}
	grid.Provenance = beatgrid.ProvenanceManual
	result.AddCueSuggestions(grid)
	if result.CueSuggestions[0].Confidence < .5 {
		t.Fatalf("manual downbeat cue confidence unexpectedly low: %#v", result.CueSuggestions[0])
	}
}
