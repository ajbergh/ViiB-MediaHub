package features

import (
	"math"
	"testing"
)

func TestEnergyLevelIsDeterministicAndUsesAbsoluteInputs(t *testing.T) {
	quiet, ok := EstimateEnergyLevel(EnergyLevelInputs{LoudnessProxyDB: -36, PeakDBFS: -28, OnsetCrestFactor: 35, HasAudio: true})
	if !ok {
		t.Fatal("expected a score")
	}
	repeated, _ := EstimateEnergyLevel(EnergyLevelInputs{LoudnessProxyDB: -36, PeakDBFS: -28, OnsetCrestFactor: 35, HasAudio: true})
	if quiet != repeated {
		t.Fatalf("score changed for equal inputs: %#v != %#v", quiet, repeated)
	}
	loud, ok := EstimateEnergyLevel(EnergyLevelInputs{LoudnessProxyDB: -18, PeakDBFS: -10, OnsetCrestFactor: 35, HasAudio: true})
	if !ok || loud.Level <= quiet.Level {
		t.Fatalf("absolute level should affect score: quiet=%#v loud=%#v", quiet, loud)
	}
	if quiet.AlgorithmVersion != EnergyLevelAlgorithmVersion || quiet.Confidence <= 0 || quiet.Confidence > 1 {
		t.Fatalf("missing provenance/confidence: %#v", quiet)
	}
}

func TestEnergyLevelRejectsMissingOrNonfiniteAudio(t *testing.T) {
	if _, ok := EstimateEnergyLevel(EnergyLevelInputs{HasAudio: false}); ok {
		t.Fatal("missing audio returned a score")
	}
	if _, ok := EstimateEnergyLevel(EnergyLevelInputs{HasAudio: true, LoudnessProxyDB: math.NaN()}); ok {
		t.Fatal("NaN input returned a score")
	}
}

func TestEnergyLevelDoesNotDependOnTempo(t *testing.T) {
	a, _ := EstimateEnergyLevel(EnergyLevelInputs{LoudnessProxyDB: -24, PeakDBFS: -14, OnsetCrestFactor: 80, HasAudio: true})
	b, _ := EstimateEnergyLevel(EnergyLevelInputs{LoudnessProxyDB: -24, PeakDBFS: -14, OnsetCrestFactor: 80, HasAudio: true})
	if a.Level != b.Level {
		t.Fatalf("same non-tempo evidence changed score: %d != %d", a.Level, b.Level)
	}
}

func TestEnergyLevelStaysInOneToTenRange(t *testing.T) {
	for _, input := range []EnergyLevelInputs{
		{LoudnessProxyDB: -200, PeakDBFS: -200, OnsetCrestFactor: 0, HasAudio: true},
		{LoudnessProxyDB: 20, PeakDBFS: 20, OnsetCrestFactor: 1e9, HasAudio: true},
	} {
		result, ok := EstimateEnergyLevel(input)
		if !ok || result.Level < 1 || result.Level > 10 {
			t.Fatalf("score out of range for %#v: %#v, %t", input, result, ok)
		}
	}
}
