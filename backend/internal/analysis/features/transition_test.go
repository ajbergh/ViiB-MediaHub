package features

import (
	"reflect"
	"testing"
)

func TestScoreTransitionPrefersCompatibleMeasuredBoundary(t *testing.T) {
	outgoing := Result{
		IntegratedLUFS: -10,
		Energy:         []EnergyPoint{{Value: .3}, {Value: .7}, {Value: .8}},
		CueSuggestions: []CueSuggestion{{Kind: "mix-out", Confidence: .8}},
	}
	compatible := Result{
		IntegratedLUFS: -10.5,
		Energy:         []EnergyPoint{{Value: .78}, {Value: .7}},
		CueSuggestions: []CueSuggestion{{Kind: "mix-in", Confidence: .8}},
	}
	incompatible := Result{
		IntegratedLUFS: -25,
		Energy:         []EnergyPoint{{Value: .05}, {Value: .1}},
	}
	good, bad := ScoreTransition(outgoing, compatible), ScoreTransition(outgoing, incompatible)
	if good.Score <= bad.Score {
		t.Fatalf("compatible score %v <= incompatible score %v", good.Score, bad.Score)
	}
	if len(good.Components) != 3 || good.Components[0].Name != "energy-continuity" || good.Vector.EnergyDelta >= bad.Vector.EnergyDelta {
		t.Fatalf("score does not expose expected measured rationale: %#v", good)
	}
}

func TestScoreTransitionIsDeterministicAndBounded(t *testing.T) {
	result := Result{Energy: []EnergyPoint{{Value: .5}}}
	first, second := ScoreTransition(result, result), ScoreTransition(result, result)
	if !reflect.DeepEqual(first, second) || first.Score < 0 || first.Score > 1 {
		t.Fatalf("transition score must be stable and normalized: %#v / %#v", first, second)
	}
}
