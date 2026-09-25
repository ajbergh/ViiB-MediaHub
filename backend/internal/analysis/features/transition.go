package features

import (
	"math"
	"strconv"

	analysiskey "github.com/ajbergh/viib-mediahub/internal/analysis/key"
)

const TransitionAlgorithmVersion = "transition-v3-structure-boundary-v1"

type TransitionIntent string

const (
	TransitionIntentHold     TransitionIntent = "hold"
	TransitionIntentLift     TransitionIntent = "lift"
	TransitionIntentReset    TransitionIntent = "reset"
	TransitionIntentHarmonic TransitionIntent = "harmonic"
)

// TransitionMetadata carries optional, provenance-resolved measurements.
// Nil or low-confidence dimensions are omitted from the score rather than
// treated as compatible or incompatible.
type TransitionMetadata struct {
	BPM                   *float64 `json:"bpm,omitempty"`
	BPMSource             string   `json:"bpmSource,omitempty"`
	BPMConfidence         *float64 `json:"bpmConfidence,omitempty"`
	CamelotKey            *string  `json:"camelotKey,omitempty"`
	KeySource             string   `json:"keySource,omitempty"`
	KeyConfidence         *float64 `json:"keyConfidence,omitempty"`
	EnergyLevel           *int     `json:"energyLevel,omitempty"`
	EnergyLevelConfidence *float64 `json:"energyLevelConfidence,omitempty"`
}

// TransitionVector is the compact, audio-derived evidence used to compare an
// outgoing track with an incoming one.  It remains separate from the score so
// callers can explain or override every recommendation dimension.
type TransitionVector struct {
	OutgoingTailEnergy float64  `json:"outgoingTailEnergy"`
	IncomingHeadEnergy float64  `json:"incomingHeadEnergy"`
	EnergyDelta        float64  `json:"energyDelta"`
	LoudnessDeltaLU    float64  `json:"loudnessDeltaLu"`
	OutgoingMixOut     float64  `json:"outgoingMixOutConfidence"`
	IncomingMixIn      float64  `json:"incomingMixInConfidence"`
	BPMDelta           *float64 `json:"bpmDelta,omitempty"`
	RequiredTempoShift *float64 `json:"requiredTempoShiftPercent,omitempty"`
	CamelotRelation    string   `json:"camelotRelation,omitempty"`
	EnergyLevelDelta   *int     `json:"energyLevelDelta,omitempty"`
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
		{Name: "loudness-proxy-match", Score: loudness, Weight: .20, Rationale: "Unweighted mono RMS loudness proxy differs by " + rounded(vector.LoudnessDeltaLU) + " dB"},
		{Name: "phrase-preparation", Score: phrase, Weight: .25, Rationale: "Measured mix-out/mix-in hints are advisory and can be accepted, moved, or ignored"},
	}
	score := 0.0
	for _, component := range components {
		score += component.Score * component.Weight
	}
	return TransitionScore{Score: clamp01(score), Vector: vector, Components: components}
}

// ScoreTransitionWithMetadata adds only evidence available at usable
// confidence. Optional evidence weights are renormalized with the established
// energy/loudness/phrase components, so a missing key or BPM never penalizes a
// candidate. Direction intents express preferences, not mixing instructions.
func ScoreTransitionWithMetadata(outgoing, incoming Result, outgoingMeta, incomingMeta TransitionMetadata, intent TransitionIntent) (TransitionScore, error) {
	if intent == "" {
		intent = TransitionIntentHold
	}
	if intent != TransitionIntentHold && intent != TransitionIntentLift && intent != TransitionIntentReset && intent != TransitionIntentHarmonic {
		return TransitionScore{}, ErrUnsupportedTransitionIntent
	}
	base := ScoreTransition(outgoing, incoming)
	vector := base.Vector
	components := append([]TransitionComponent(nil), base.Components...)
	weights := []float64{.55, .20, .25}
	if transitionEvidenceUsable(outgoingMeta.BPM, outgoingMeta.BPMConfidence, outgoingMeta.BPMSource) && transitionEvidenceUsable(incomingMeta.BPM, incomingMeta.BPMConfidence, incomingMeta.BPMSource) {
		outBPM, inBPM := *outgoingMeta.BPM, *incomingMeta.BPM
		delta := inBPM - outBPM
		shift := (outBPM/inBPM - 1) * 100
		vector.BPMDelta, vector.RequiredTempoShift = &delta, &shift
		confidence := math.Min(evidenceConfidence(outgoingMeta.BPMConfidence, outgoingMeta.BPMSource), evidenceConfidence(incomingMeta.BPMConfidence, incomingMeta.BPMSource))
		score := clamp01(1 - math.Abs(shift)/15)
		components = append(components, TransitionComponent{Name: "tempo-compatibility", Score: score, Weight: .12 * confidence, Rationale: "Matching the incoming tempo to the outgoing track requires " + signedRounded(shift) + "% tempo change"})
		weights = append(weights, .12*confidence)
	}
	if validCamelot(outgoingMeta.CamelotKey) && validCamelot(incomingMeta.CamelotKey) && evidenceConfidence(outgoingMeta.KeyConfidence, outgoingMeta.KeySource) >= .5 && evidenceConfidence(incomingMeta.KeyConfidence, incomingMeta.KeySource) >= .5 {
		relation, _ := analysiskey.HarmonicRelation(*outgoingMeta.CamelotKey, *incomingMeta.CamelotKey)
		vector.CamelotRelation = relation
		confidence := math.Min(evidenceConfidence(outgoingMeta.KeyConfidence, outgoingMeta.KeySource), evidenceConfidence(incomingMeta.KeyConfidence, incomingMeta.KeySource))
		score := harmonicScore(relation)
		weight := .12
		if intent == TransitionIntentHarmonic {
			weight = .30
		}
		components = append(components, TransitionComponent{Name: "harmonic-compatibility", Score: score, Weight: weight * confidence, Rationale: "Camelot relation is " + relation + " (advisory; intentional dissonance may be useful)"})
		weights = append(weights, weight*confidence)
	}
	if validEnergyLevel(outgoingMeta) && validEnergyLevel(incomingMeta) {
		confidence := math.Min(*outgoingMeta.EnergyLevelConfidence, *incomingMeta.EnergyLevelConfidence)
		if confidence >= .5 {
			delta := *incomingMeta.EnergyLevel - *outgoingMeta.EnergyLevel
			vector.EnergyLevelDelta = &delta
			target := 0
			name := "energy-level-hold"
			switch intent {
			case TransitionIntentLift:
				target, name = 1, "energy-level-lift"
			case TransitionIntentReset:
				target, name = -1, "energy-level-reset"
			case TransitionIntentHarmonic:
				name = "energy-level-balance"
			}
			score := 1 - math.Min(1, math.Abs(float64(delta-target))/3)
			components = append(components, TransitionComponent{Name: name, Score: score, Weight: .12 * confidence, Rationale: "Incoming Energy Level changes by " + signedInt(delta) + " from the outgoing track"})
			weights = append(weights, .12*confidence)
		}
	}
	if score, confidence, rationale, ok := structureBoundaryCompatibility(outgoing, incoming); ok {
		weight := .12 * confidence
		components = append(components, TransitionComponent{Name: "intro-outro-compatibility", Score: score, Weight: weight, Rationale: rationale})
		weights = append(weights, weight)
	}
	totalWeight, weightedScore := 0.0, 0.0
	for i, component := range components {
		weight := component.Weight
		if i < len(weights) {
			weight = weights[i]
			component.Weight = weight
			components[i] = component
		}
		totalWeight += weight
		weightedScore += component.Score * weight
	}
	if totalWeight > 0 {
		for i := range components {
			components[i].Weight /= totalWeight
		}
		weightedScore /= totalWeight
	}
	return TransitionScore{Score: clamp01(weightedScore), Vector: vector, Components: components}, nil
}

// Structure V1 emits known labels with confidence from .40 to .48. Keep those
// hints usable, but let their low confidence proportionally limit their weight.
const minimumStructureConfidence = .4

// structureBoundaryCompatibility compares only the final outgoing section
// with the initial incoming section. At confidence >= .4 and finite timing, its matrix is:
// outro -> intro = 1; every other known label pair = 0. Unknown/missing labels
// and lower-confidence pairs are omitted. Energy-derived labels are advisory;
// this score says nothing about vocals or phrases.
func structureBoundaryCompatibility(outgoing, incoming Result) (score, confidence float64, rationale string, ok bool) {
	if len(outgoing.Sections) == 0 || len(incoming.Sections) == 0 {
		return 0, 0, "", false
	}
	last := outgoing.Sections[len(outgoing.Sections)-1]
	first := incoming.Sections[0]
	if !knownStructureLabel(last.Label) || !knownStructureLabel(first.Label) ||
		!usableStructureTiming(last) || !usableStructureTiming(first) ||
		!usableStructureConfidence(last.Confidence) || !usableStructureConfidence(first.Confidence) {
		return 0, 0, "", false
	}
	confidence = math.Min(last.Confidence, first.Confidence)
	if last.Label == StructureOutro && first.Label == StructureIntro {
		return 1, confidence, "Final outgoing outro to opening incoming intro is a structural match (energy-derived hints only; no vocal or phrase evidence)", true
	}
	return 0, confidence, "Final outgoing " + last.Label + " to opening incoming " + first.Label + " is not the outro-to-intro match (energy-derived hints only; no vocal or phrase evidence)", true
}

func usableStructureTiming(section Section) bool {
	return finiteNumber(section.Start) && finiteNumber(section.End) && section.Start >= 0 && section.End > section.Start
}

func usableStructureConfidence(confidence float64) bool {
	return finiteNumber(confidence) && confidence >= minimumStructureConfidence && confidence <= 1
}

func knownStructureLabel(label string) bool {
	switch label {
	case StructureIntro, StructureBuild, StructureDrop, StructureBreakdown, StructureOutro:
		return true
	default:
		return false
	}
}

var ErrUnsupportedTransitionIntent = &transitionIntentError{}

type transitionIntentError struct{}

func (*transitionIntentError) Error() string { return "unsupported transition intent" }

func transitionEvidenceUsable(value, confidence *float64, source string) bool {
	if value == nil || !finiteNumber(*value) || *value <= 0 {
		return false
	}
	return evidenceConfidence(confidence, source) >= .5
}

func evidenceConfidence(confidence *float64, source string) float64 {
	if source == "manual" {
		return 1
	}
	if source != "measured" || confidence == nil || !finiteNumber(*confidence) {
		return 0
	}
	return clamp01(*confidence)
}

func validCamelot(value *string) bool {
	if value == nil || len(*value) < 2 || (*value)[len(*value)-1] != 'A' && (*value)[len(*value)-1] != 'B' {
		return false
	}
	number, err := strconv.Atoi((*value)[:len(*value)-1])
	return err == nil && number >= 1 && number <= 12
}

func harmonicScore(relation string) float64 {
	switch relation {
	case "same":
		return 1
	case "adjacent":
		return .9
	case "relative":
		return .85
	case "other":
		return .25
	default:
		return .5
	}
}

func validEnergyLevel(meta TransitionMetadata) bool {
	return meta.EnergyLevel != nil && *meta.EnergyLevel >= 1 && *meta.EnergyLevel <= 10 && meta.EnergyLevelConfidence != nil && finiteNumber(*meta.EnergyLevelConfidence) && *meta.EnergyLevelConfidence >= 0 && *meta.EnergyLevelConfidence <= 1
}

func finiteNumber(value float64) bool { return !math.IsNaN(value) && !math.IsInf(value, 0) }

func signedRounded(value float64) string {
	if value > 0 {
		return "+" + rounded(value)
	}
	return rounded(value)
}

func signedInt(value int) string {
	if value > 0 {
		return "+" + strconv.Itoa(value)
	}
	return strconv.Itoa(value)
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
