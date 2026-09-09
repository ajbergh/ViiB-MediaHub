package features

import (
	"math"
	"strconv"
)

// TransitionVector is the compact, audio-derived evidence used to compare an
// outgoing track with an incoming one.  It remains separate from the score so
// callers can explain or override every recommendation dimension.
type TransitionVector struct {
	OutgoingTailEnergy float64 `json:"outgoingTailEnergy"`
	IncomingHeadEnergy float64 `json:"incomingHeadEnergy"`
	EnergyDelta        float64 `json:"energyDelta"`
	LoudnessDeltaLU    float64 `json:"loudnessDeltaLu"`
	OutgoingMixOut     float64 `json:"outgoingMixOutConfidence"`
	IncomingMixIn      float64 `json:"incomingMixInConfidence"`
}

// TransitionComponent is one transparent contribution to a recommendation.
// Scores are normalized 0..1 and never represent an instruction to mix.
type TransitionComponent struct {
	Name      string  `json:"name"`
	Score     float64 `json:"score"`
	Weight    float64 `json:"weight"`
	Rationale string  `json:"rationale"`
}

// TransitionScore combines the vector with weighted, human-readable
// components.  The weights are deliberately stable v1 heuristics so ranking
// is deterministic and can be calibrated against future labeled pairs.
type TransitionScore struct {
	Score      float64               `json:"score"`
	Vector     TransitionVector      `json:"vector"`
	Components []TransitionComponent `json:"components"`
}

// ScoreTransition compares the tail of the outgoing curve with the head of
// the incoming curve.  It favors compatible energy and loudness and rewards
// (but never requires) explicit measured mix-out/mix-in preparation hints.
func ScoreTransition(outgoing, incoming Result) TransitionScore {
	vector := TransitionVector{
		OutgoingTailEnergy: tailEnergy(outgoing.Energy),
		IncomingHeadEnergy: headEnergy(incoming.Energy),
		LoudnessDeltaLU:    math.Abs(outgoing.IntegratedLUFS - incoming.IntegratedLUFS),
		OutgoingMixOut:     cueConfidence(outgoing.CueSuggestions, "mix-out"),
		IncomingMixIn:      cueConfidence(incoming.CueSuggestions, "mix-in"),
	}
	vector.EnergyDelta = math.Abs(vector.OutgoingTailEnergy - vector.IncomingHeadEnergy)

	energy := clamp01(1 - vector.EnergyDelta/.65)
	loudness := clamp01(1 - vector.LoudnessDeltaLU/12)
	// Missing suggestions are neutral evidence, not a penalty: older artifacts
	// and tracks without a reliable downbeat remain eligible to recommend.
	phrase := (defaultConfidence(vector.OutgoingMixOut) + defaultConfidence(vector.IncomingMixIn)) / 2
	components := []TransitionComponent{
		{Name: "energy-continuity", Score: energy, Weight: .55, Rationale: "Outgoing tail and incoming head differ by " + rounded(vector.EnergyDelta) + " normalized energy"},
		{Name: "loudness-match", Score: loudness, Weight: .20, Rationale: "Integrated loudness differs by " + rounded(vector.LoudnessDeltaLU) + " LU"},
		{Name: "phrase-preparation", Score: phrase, Weight: .25, Rationale: "Measured mix-out/mix-in hints are advisory and can be accepted, moved, or ignored"},
	}
	score := 0.0
	for _, component := range components {
		score += component.Score * component.Weight
	}
	return TransitionScore{Score: clamp01(score), Vector: vector, Components: components}
}

func headEnergy(points []EnergyPoint) float64 { return meanEnergy(points[:min(len(points), 8)]) }

func tailEnergy(points []EnergyPoint) float64 {
	start := max(0, len(points)-8)
	return meanEnergy(points[start:])
}

func meanEnergy(points []EnergyPoint) float64 {
	if len(points) == 0 {
		return 0
	}
	total := 0.0
	for _, point := range points {
		total += point.Value
	}
	return total / float64(len(points))
}

func cueConfidence(cues []CueSuggestion, kind string) float64 {
	best := 0.0
	for _, cue := range cues {
		if cue.Kind == kind && cue.Confidence > best {
			best = cue.Confidence
		}
	}
	return clamp01(best)
}

func defaultConfidence(value float64) float64 {
	if value == 0 {
		return .5
	}
	return value
}

func clamp01(value float64) float64 { return math.Max(0, math.Min(1, value)) }

func rounded(value float64) string {
	return strconv.FormatFloat(math.Round(value*100)/100, 'f', 2, 64)
}
