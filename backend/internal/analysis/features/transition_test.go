package features

import (
	"math"
	"reflect"
	"strings"
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
	var loudness *TransitionComponent
	for index := range first.Components {
		if strings.Contains(first.Components[index].Name, "loudness") {
			loudness = &first.Components[index]
		}
	}
	if loudness == nil || loudness.Name != "loudness-proxy-match" || !strings.Contains(strings.ToLower(loudness.Rationale), "unweighted mono rms loudness proxy") || !strings.HasSuffix(loudness.Rationale, "dB") || strings.Contains(loudness.Rationale, "Integrated loudness") || strings.Contains(loudness.Rationale, " LU") {
		t.Fatalf("transition rationale still overstates loudness standard: %#v", loudness)
	}
}

func TestScoreTransitionWithMetadataGatesEvidenceAndRenormalizes(t *testing.T) {
	base := Result{IntegratedLUFS: -12, Energy: []EnergyPoint{{Value: .4}, {Value: .7}}}
	withoutEvidence, err := ScoreTransitionWithMetadata(base, base, TransitionMetadata{}, TransitionMetadata{}, TransitionIntentHold)
	if err != nil {
		t.Fatal(err)
	}
	legacy := ScoreTransition(base, base)
	if withoutEvidence.Score != legacy.Score || len(withoutEvidence.Components) != len(legacy.Components) {
		t.Fatalf("missing metadata changed legacy score: %#v vs %#v", withoutEvidence, legacy)
	}
	low := .2
	bpm := 128.0
	lowMetadata := TransitionMetadata{BPM: &bpm, BPMSource: "measured", BPMConfidence: &low}
	withLowEvidence, err := ScoreTransitionWithMetadata(base, base, lowMetadata, lowMetadata, TransitionIntentHold)
	if err != nil || withLowEvidence.Score != legacy.Score || len(withLowEvidence.Components) != 3 {
		t.Fatalf("low-confidence metadata affected score: %#v, err=%v", withLowEvidence, err)
	}
	weights := 0.0
	for _, component := range withoutEvidence.Components {
		weights += component.Weight
	}
	if math.Abs(weights-1) > 1e-9 {
		t.Fatalf("renormalized component weights sum to %f", weights)
	}
}

func TestScoreTransitionWithMetadataRanksTempoHarmonicAndDirection(t *testing.T) {
	base := Result{IntegratedLUFS: -12, Energy: []EnergyPoint{{Value: .5}, {Value: .5}}}
	confidence := .9
	levelOut, levelLift, levelReset := 5, 6, 4
	key := "8A"
	outgoing := TransitionMetadata{BPM: ptrFloat(128), BPMSource: "measured", BPMConfidence: &confidence, CamelotKey: &key, KeySource: "measured", KeyConfidence: &confidence, EnergyLevel: &levelOut, EnergyLevelConfidence: &confidence}
	closeTempo := TransitionMetadata{BPM: ptrFloat(130), BPMSource: "measured", BPMConfidence: &confidence, CamelotKey: &key, KeySource: "measured", KeyConfidence: &confidence, EnergyLevel: &levelLift, EnergyLevelConfidence: &confidence}
	farTempo := TransitionMetadata{BPM: ptrFloat(150), BPMSource: "measured", BPMConfidence: &confidence, CamelotKey: &key, KeySource: "measured", KeyConfidence: &confidence, EnergyLevel: &levelLift, EnergyLevelConfidence: &confidence}
	closeScore, err := ScoreTransitionWithMetadata(base, base, outgoing, closeTempo, TransitionIntentLift)
	if err != nil {
		t.Fatal(err)
	}
	farScore, err := ScoreTransitionWithMetadata(base, base, outgoing, farTempo, TransitionIntentLift)
	if err != nil {
		t.Fatal(err)
	}
	if closeScore.Score <= farScore.Score || closeScore.Vector.BPMDelta == nil || *closeScore.Vector.BPMDelta != 2 || closeScore.Vector.RequiredTempoShift == nil || closeScore.Vector.CamelotRelation != "same" || closeScore.Vector.EnergyLevelDelta == nil || *closeScore.Vector.EnergyLevelDelta != 1 {
		t.Fatalf("tempo/harmonic/energy evidence missing or misranked: close=%#v far=%#v", closeScore, farScore)
	}
	resetMeta := closeTempo
	resetMeta.EnergyLevel = &levelReset
	liftScore, err := ScoreTransitionWithMetadata(base, base, outgoing, closeTempo, TransitionIntentLift)
	if err != nil {
		t.Fatal(err)
	}
	resetScore, err := ScoreTransitionWithMetadata(base, base, outgoing, resetMeta, TransitionIntentLift)
	if err != nil {
		t.Fatal(err)
	}
	if liftScore.Score <= resetScore.Score {
		t.Fatalf("lift did not favor +1 Energy Level over reset candidate: lift=%f reset=%f", liftScore.Score, resetScore.Score)
	}
	resetIntentScore, err := ScoreTransitionWithMetadata(base, base, outgoing, resetMeta, TransitionIntentReset)
	if err != nil {
		t.Fatal(err)
	}
	liftForReset, err := ScoreTransitionWithMetadata(base, base, outgoing, closeTempo, TransitionIntentReset)
	if err != nil || resetIntentScore.Score <= liftForReset.Score {
		t.Fatalf("reset did not favor -1 Energy Level over lift candidate: reset=%#v lift=%#v err=%v", resetIntentScore, liftForReset, err)
	}
	invalid := TransitionMetadata{CamelotKey: ptrString("1X"), KeySource: "measured", KeyConfidence: &confidence}
	invalidScore, err := ScoreTransitionWithMetadata(base, base, outgoing, invalid, TransitionIntentHarmonic)
	if err != nil || invalidScore.Vector.CamelotRelation != "" {
		t.Fatalf("invalid Camelot input became evidence: %#v, err=%v", invalidScore, err)
	}
	if _, err := ScoreTransitionWithMetadata(base, base, outgoing, closeTempo, TransitionIntent("vocal-safe")); err != ErrUnsupportedTransitionIntent {
		t.Fatalf("unsupported intent error=%v", err)
	}
}

func ptrFloat(value float64) *float64 { return &value }

func ptrString(value string) *string { return &value }
