package playlist

import (
	"math"
	"testing"
)

func observation(value float64) *Observation {
	return &Observation{Value: value, Confidence: 1, Source: "manual"}
}

func TestAudioIntentSeparatesHardBoundsAndSoftPreferences(t *testing.T) {
	for _, test := range []struct {
		prompt   string
		min, max float64
		hard     bool
	}{
		{"instrumental music between 120 and 128 BPM", 120, 128, true},
		{"dance around 125 BPM", 125, 125, false},
		{"at most 128 BPM", 0, 128, true},
		{"roughly 100–110 BPM", 100, 110, false},
		{"exactly 95 BPM", 95, 95, true},
		{"no tracks over 140 BPM", 0, 140, true},
		{"prefer 125 BPM", 125, 125, false},
	} {
		intent := ParseAudioIntent(test.prompt)
		if intent.MinBPM != test.min || intent.MaxBPM != test.max || intent.BPMConstraintHard != test.hard {
			t.Fatalf("%q: %+v", test.prompt, intent)
		}
	}
	intent := ParseAudioIntent("45 minutes of high-energy music, energy level 7–9, Camelot 8A, building gradually, harmonic mixing")
	if intent.TargetDurationMinutes != 45 || intent.MinEnergyLevel != 7 || intent.MaxEnergyLevel != 9 || !intent.EnergyConstraintHard || !intent.KeyConstraintHard || intent.CamelotKeys[0] != "8A" || !intent.HarmonicMix || intent.EnergyDirection != "build" {
		t.Fatalf("%+v", intent)
	}
	merged := MergeAudioIntent(AudioIntent{MinBPM: 80, MaxBPM: 90, BPMConstraintHard: false}, "between 120 and 128 BPM")
	if merged.MinBPM != 120 || !merged.BPMConstraintHard {
		t.Fatal("model dropped explicit constraint")
	}
	if intent := ParseAudioIntent("music without high energy"); intent.TargetEnergy != "" {
		t.Fatal("negative phrase became a positive preference")
	}
	if intent := ParseAudioIntent("avoid key 8A"); intent.KeyConstraintHard {
		t.Fatal("excluded key became required key")
	}
}

func TestHardRangesRejectUnknownAndKeepEnergyMetricsSeparate(t *testing.T) {
	intent := AudioIntent{MaxBPM: 128, BPMConstraintHard: true, MaxEnergyLevel: 7, EnergyConstraintHard: true}
	for _, value := range []AudioFeatures{{}, {BPM: observation(125)}, {BPM: observation(125), ProviderScores: map[string]Observation{"spotify_energy_score": {Value: .5, Confidence: 1}}}, {BPM: observation(140), EnergyLevel: observation(5)}} {
		if Allowed(intent, value) {
			t.Fatalf("unverified range admitted: %+v", value)
		}
	}
	if !Allowed(intent, AudioFeatures{BPM: observation(125), EnergyLevel: observation(6)}) {
		t.Fatal("eligible track rejected")
	}
	if Allowed(AudioIntent{CamelotKeys: []string{"8A"}, KeyConstraintHard: true}, AudioFeatures{}) {
		t.Fatal("unknown key satisfied strict key")
	}
}

func TestPreferencesAreConfidenceAwareAndUnknownIsNeutral(t *testing.T) {
	intent := AudioIntent{MinBPM: 120, MaxBPM: 128, MinEnergyLevel: 7, MaxEnergyLevel: 9}
	good := PreferenceScore(intent, AudioFeatures{BPM: observation(125), EnergyLevel: observation(8)}, "", "")
	unknown := PreferenceScore(intent, AudioFeatures{}, "", "")
	bad := PreferenceScore(intent, AudioFeatures{BPM: observation(80), EnergyLevel: observation(2)}, "", "")
	if !(good > unknown && unknown > bad) || unknown != .5 {
		t.Fatalf("good=%f unknown=%f bad=%f", good, unknown, bad)
	}
	weak := &Observation{Value: 125, Confidence: .2}
	if PreferenceScore(AudioIntent{MinBPM: 120}, AudioFeatures{BPM: weak}, "", "") != .5 {
		t.Fatal("low confidence became strong evidence")
	}
	intent = AudioIntent{ProviderPreferences: map[string]float64{"spotify_danceability_score": .9}}
	if PreferenceScore(intent, AudioFeatures{EnergyLevel: observation(9)}, "", "") != .5 {
		t.Fatal("local energy became danceability")
	}
}

func TestHarmonicAndEnergyOrderingPreservesTracks(t *testing.T) {
	tracks := []Track{{ID: "high", Features: AudioFeatures{EnergyLevel: observation(9)}}, {ID: "low", Features: AudioFeatures{EnergyLevel: observation(2)}}, {ID: "medium", Features: AudioFeatures{EnergyLevel: observation(5)}}}
	for _, direction := range []string{"build", "cooldown"} {
		ordered := Order(tracks, AudioIntent{EnergyDirection: direction}, nil)
		for i := 1; i < len(ordered); i++ {
			delta := ordered[i].Features.EnergyLevel.Value - ordered[i-1].Features.EnergyLevel.Value
			if direction == "build" && delta < 0 || direction == "cooldown" && delta > 0 {
				t.Fatalf("%s order=%+v", direction, ordered)
			}
		}
	}
	from := AudioFeatures{Key: &KeyObservation{Camelot: "8A", Confidence: 1}}
	compatible := AudioFeatures{Key: &KeyObservation{Camelot: "9A", Confidence: 1}}
	incompatible := AudioFeatures{Key: &KeyObservation{Camelot: "3B", Confidence: 1}}
	a, _ := PairScore(from, compatible, AudioIntent{HarmonicMix: true})
	b, _ := PairScore(from, incompatible, AudioIntent{HarmonicMix: true})
	if a <= b {
		t.Fatal("Camelot compatibility did not affect ordering")
	}
	predecessor := &Track{ID: "previous", Features: from}
	ordered := OrderAfter([]Track{{ID: "bad", Features: incompatible}, {ID: "good", Features: compatible}}, AudioIntent{HarmonicMix: true}, nil, predecessor)
	if ordered[0].ID != "good" {
		t.Fatal("cross-phase harmonic continuity ignored")
	}
	if score, known := PairScore(AudioFeatures{}, AudioFeatures{}, AudioIntent{HarmonicMix: true}); known || score != .5 {
		t.Fatal("missing metadata became compatibility")
	}
	strong, _ := PairScore(AudioFeatures{BPM: observation(125)}, AudioFeatures{BPM: observation(125)}, AudioIntent{})
	weak, _ := PairScore(AudioFeatures{BPM: &Observation{Value: 125, Confidence: .5}}, AudioFeatures{BPM: observation(125)}, AudioIntent{})
	if weak >= strong || weak <= .5 {
		t.Fatal("confidence did not soften pair compatibility")
	}
}

func TestDurationUsesWholeTracksAndMarksEstimates(t *testing.T) {
	tracks := []Track{{ID: "a", Features: AudioFeatures{DurationSeconds: 200}}, {ID: "b", Features: AudioFeatures{DurationSeconds: 190}}, {ID: "c", Features: AudioFeatures{DurationSeconds: 220}}}
	selected := ClosestDurationPrefix(tracks, 400)
	seconds, estimated := Duration(selected)
	if len(selected) != 2 || seconds != 390 || estimated {
		t.Fatalf("count=%d duration=%f estimated=%v", len(selected), seconds, estimated)
	}
	if seconds, estimated := Duration([]Track{{ID: "unknown"}}); seconds != 180 || !estimated {
		t.Fatal("missing duration was not disclosed")
	}
	intent := AudioIntent{MinBPM: math.NaN(), MaxBPM: math.Inf(1), TargetDurationMinutes: 99999, CamelotKeys: []string{"13Z"}}.Normalize()
	if intent.MinBPM != 0 || intent.MaxBPM != 0 || intent.TargetDurationMinutes != 0 || len(intent.CamelotKeys) != 0 {
		t.Fatal("invalid intent escaped normalization")
	}
}
