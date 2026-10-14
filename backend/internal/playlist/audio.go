// Package playlist provides provider-independent musical intent and scoring.
package playlist

import (
	"math"
	"regexp"
	"strconv"
	"strings"

	analysiskey "github.com/ajbergh/viib-mediahub/internal/analysis/key"
)

// EnergyLevel always means the local/manual 1–10 metric, never a provider
// score or an energy-curve sample. Bounds and preferences are distinct.
type AudioIntent struct {
	MinBPM                float64            `json:"minBPM,omitempty"`
	MaxBPM                float64            `json:"maxBPM,omitempty"`
	BPMConstraintHard     bool               `json:"bpmConstraintHard,omitempty"`
	MinEnergyLevel        int                `json:"minEnergyLevel,omitempty"`
	MaxEnergyLevel        int                `json:"maxEnergyLevel,omitempty"`
	EnergyConstraintHard  bool               `json:"energyConstraintHard,omitempty"`
	TargetEnergy          string             `json:"targetEnergy,omitempty"`
	PreferredMoods        []string           `json:"preferredMoods,omitempty"`
	CamelotKeys           []string           `json:"camelotKeys,omitempty"`
	KeyConstraintHard     bool               `json:"keyConstraintHard,omitempty"`
	HarmonicMix           bool               `json:"harmonicMix,omitempty"`
	EnergyDirection       string             `json:"energyDirection,omitempty"`
	TargetDurationMinutes int                `json:"targetDurationMinutes,omitempty"`
	ProviderPreferences   map[string]float64 `json:"providerPreferences,omitempty"`
}

type Observation struct {
	Value      float64
	Confidence float64
	Source     string
}
type KeyObservation struct {
	Camelot    string
	Confidence float64
	Source     string
}
type AudioFeatures struct {
	BPM             *Observation
	EnergyLevel     *Observation
	EnergyCurve     *Observation
	Key             *KeyObservation
	DurationSeconds float64
	ProviderScores  map[string]Observation
}

func ValidCamelot(key string) bool {
	if len(key) < 2 || (key[len(key)-1] != 'A' && key[len(key)-1] != 'B') {
		return false
	}
	n, err := strconv.Atoi(key[:len(key)-1])
	return err == nil && n >= 1 && n <= 12
}

func (intent AudioIntent) Hard() bool {
	return intent.BPMConstraintHard || intent.EnergyConstraintHard || intent.KeyConstraintHard
}
func (intent AudioIntent) HasPreferences() bool {
	return intent.MinBPM > 0 || intent.MaxBPM > 0 || intent.MinEnergyLevel > 0 || intent.MaxEnergyLevel > 0 || intent.TargetEnergy != "" || len(intent.PreferredMoods) > 0 || len(intent.CamelotKeys) > 0 || len(intent.ProviderPreferences) > 0
}

func (intent AudioIntent) Normalize() AudioIntent {
	if !finite(intent.MinBPM) || intent.MinBPM < 20 || intent.MinBPM > 400 {
		intent.MinBPM = 0
	}
	if !finite(intent.MaxBPM) || intent.MaxBPM < 20 || intent.MaxBPM > 400 {
		intent.MaxBPM = 0
	}
	if intent.MinBPM > 0 && intent.MaxBPM > 0 && intent.MinBPM > intent.MaxBPM {
		intent.MinBPM, intent.MaxBPM = intent.MaxBPM, intent.MinBPM
	}
	if intent.MinBPM == 0 && intent.MaxBPM == 0 {
		intent.BPMConstraintHard = false
	}
	if intent.MinEnergyLevel < 1 || intent.MinEnergyLevel > 10 {
		intent.MinEnergyLevel = 0
	}
	if intent.MaxEnergyLevel < 1 || intent.MaxEnergyLevel > 10 {
		intent.MaxEnergyLevel = 0
	}
	if intent.MinEnergyLevel > 0 && intent.MaxEnergyLevel > 0 && intent.MinEnergyLevel > intent.MaxEnergyLevel {
		intent.MinEnergyLevel, intent.MaxEnergyLevel = intent.MaxEnergyLevel, intent.MinEnergyLevel
	}
	if intent.MinEnergyLevel == 0 && intent.MaxEnergyLevel == 0 {
		intent.EnergyConstraintHard = false
	}
	intent.TargetEnergy = strings.ToLower(strings.TrimSpace(intent.TargetEnergy))
	switch intent.TargetEnergy {
	case "low", "medium", "high":
	default:
		intent.TargetEnergy = ""
	}
	intent.EnergyDirection = strings.ToLower(strings.TrimSpace(intent.EnergyDirection))
	switch intent.EnergyDirection {
	case "build", "cooldown", "arc", "steady":
	default:
		intent.EnergyDirection = ""
	}
	keys := make([]string, 0, len(intent.CamelotKeys))
	seen := make(map[string]bool)
	for _, key := range intent.CamelotKeys {
		key = strings.ToUpper(strings.TrimSpace(key))
		if ValidCamelot(key) && !seen[key] && len(keys) < 24 {
			keys = append(keys, key)
			seen[key] = true
		}
	}
	intent.CamelotKeys = keys
	if len(keys) == 0 {
		intent.KeyConstraintHard = false
	}
	if intent.TargetDurationMinutes < 1 || intent.TargetDurationMinutes > 1440 {
		intent.TargetDurationMinutes = 0
	}
	preferences := make(map[string]float64)
	for key, value := range intent.ProviderPreferences {
		switch key {
		case "spotify_energy_score", "spotify_danceability_score", "spotify_valence_score", "spotify_acousticness_score", "spotify_instrumentalness_score", "spotify_liveness_score", "spotify_speechiness_score":
			if finite(value) && value >= 0 && value <= 1 {
				preferences[key] = value
			}
		}
	}
	intent.ProviderPreferences = preferences
	moods := make([]string, 0, len(intent.PreferredMoods))
	for _, mood := range intent.PreferredMoods {
		mood = strings.ToLower(strings.TrimSpace(mood))
		if mood != "" && len(mood) < 80 && len(moods) < 12 {
			moods = append(moods, mood)
		}
	}
	intent.PreferredMoods = moods
	return intent
}

var (
	bpmRange            = regexp.MustCompile(`(?i)(?:(between|from|around|roughly|about|prefer(?:ably)?)\s+)?(\d{2,3}(?:\.\d+)?)\s*(?:-|–|to|and)\s*(\d{2,3}(?:\.\d+)?)\s*bpm\b`)
	bpmSingle           = regexp.MustCompile(`(?i)(?:(around|roughly|about|prefer(?:ably)?|exactly|under|below|over|above|at least|at most)\s+)?(\d{2,3}(?:\.\d+)?)\s*bpm\b`)
	energyRange         = regexp.MustCompile(`(?i)energy(?:\s+level)?\s*(?:(between|from|around|about)\s*)?(\d{1,2})\s*(?:-|–|to|and)\s*(\d{1,2})`)
	energySingle        = regexp.MustCompile(`(?i)energy(?:\s+level)?\s*(?:(around|about|exactly|under|below|over|above|at least|at most)\s*)?(\d{1,2})(?:\s*/\s*10)?\b`)
	durationPattern     = regexp.MustCompile(`(?i)\b(\d{1,3})[ -]*(minutes?|mins?|hours?|hrs?)\b`)
	keyPattern          = regexp.MustCompile(`(?i)\b(?:camelot|key)\s+(1[0-2]|[1-9])([ab])\b`)
	negativeBPMUpper    = regexp.MustCompile(`(?i)\b(?:no|not|without|avoid|exclude|excluding)\s+(?:(?:tracks?|songs?|music)\s+)?(?:over|above|more than)\s+(\d{2,3}(?:\.\d+)?)\s*bpm\b`)
	negativeBPMLower    = regexp.MustCompile(`(?i)\b(?:no|not|without|avoid|exclude|excluding)\s+(?:(?:tracks?|songs?|music)\s+)?(?:under|below|less than)\s+(\d{2,3}(?:\.\d+)?)\s*bpm\b`)
	negativeAudioPrefix = regexp.MustCompile(`(?i)\b(?:no|not|without|avoid|exclude|excluding)(?:\s+[\w.-]+){0,6}\s*$`)
)

func negatedAudioMatch(prompt string, pattern *regexp.Regexp) bool {
	match := pattern.FindStringIndex(prompt)
	return match != nil && negativeAudioPrefix.MatchString(prompt[:match[0]])
}

// ParseAudioIntent preserves explicit numeric requirements even when the LLM
// is offline, malformed, or tries to soften the user's bounds.
func ParseAudioIntent(prompt string) AudioIntent {
	var intent AudioIntent
	prompt = negativeBPMUpper.ReplaceAllString(prompt, "at most ${1} BPM")
	prompt = negativeBPMLower.ReplaceAllString(prompt, "at least ${1} BPM")
	if m := bpmRange.FindStringSubmatch(prompt); m != nil && !negatedAudioMatch(prompt, bpmRange) {
		intent.MinBPM, _ = strconv.ParseFloat(m[2], 64)
		intent.MaxBPM, _ = strconv.ParseFloat(m[3], 64)
		intent.BPMConstraintHard = !soft(m[1])
	} else if m := bpmSingle.FindStringSubmatch(prompt); m != nil && !negatedAudioMatch(prompt, bpmSingle) {
		value, _ := strconv.ParseFloat(m[2], 64)
		intent.MinBPM, intent.MaxBPM = value, value
		intent.BPMConstraintHard = !soft(m[1])
		switch strings.ToLower(m[1]) {
		case "under", "below", "at most":
			intent.MinBPM = 0
		case "over", "above", "at least":
			intent.MaxBPM = 0
		}
	}
	if m := energyRange.FindStringSubmatch(prompt); m != nil && !negatedAudioMatch(prompt, energyRange) {
		intent.MinEnergyLevel, _ = strconv.Atoi(m[2])
		intent.MaxEnergyLevel, _ = strconv.Atoi(m[3])
		intent.EnergyConstraintHard = !soft(m[1])
	} else if m := energySingle.FindStringSubmatch(prompt); m != nil && !negatedAudioMatch(prompt, energySingle) {
		value, _ := strconv.Atoi(m[2])
		intent.MinEnergyLevel, intent.MaxEnergyLevel = value, value
		intent.EnergyConstraintHard = !soft(m[1])
		switch strings.ToLower(m[1]) {
		case "under", "below", "at most":
			intent.MinEnergyLevel = 0
		case "over", "above", "at least":
			intent.MaxEnergyLevel = 0
		}
	}
	if m := durationPattern.FindStringSubmatch(prompt); m != nil {
		intent.TargetDurationMinutes, _ = strconv.Atoi(m[1])
		if strings.HasPrefix(strings.ToLower(m[2]), "h") {
			intent.TargetDurationMinutes *= 60
		}
	}
	if m := keyPattern.FindStringSubmatch(prompt); m != nil && !negatedAudioMatch(prompt, keyPattern) {
		intent.CamelotKeys = []string{strings.ToUpper(m[1] + m[2])}
		intent.KeyConstraintHard = true
	}
	lower := strings.ToLower(prompt)
	lower = regexp.MustCompile(`(?i)\b(?:no|without|avoid|exclude|excluding)\s+(?:high[- ]energy|low[- ]energy|harmonic(?: mixing)?|calm|happy|sad|dreamy|aggressive|energetic|uplifting|romantic|nostalgic)\b`).ReplaceAllString(lower, "")
	if strings.Contains(lower, "high energy") || strings.Contains(lower, "high-energy") || strings.Contains(lower, "energetic") {
		intent.TargetEnergy = "high"
	} else if strings.Contains(lower, "low energy") || strings.Contains(lower, "low-energy") || strings.Contains(lower, "relaxing") {
		intent.TargetEnergy = "low"
	}
	for _, mood := range []string{"calm", "peaceful", "happy", "sad", "dreamy", "melancholic", "romantic", "aggressive", "energetic", "uplifting", "nostalgic"} {
		if regexp.MustCompile(`\b` + mood + `\b`).MatchString(lower) {
			intent.PreferredMoods = append(intent.PreferredMoods, mood)
		}
	}
	if strings.Contains(lower, "harmonic") || strings.Contains(lower, "mix in key") {
		intent.HarmonicMix = true
	}
	switch {
	case strings.Contains(lower, "build gradually") || strings.Contains(lower, "building") || strings.Contains(lower, "rising energy"):
		intent.EnergyDirection = "build"
	case strings.Contains(lower, "cool down") || strings.Contains(lower, "cooldown") || strings.Contains(lower, "winding down"):
		intent.EnergyDirection = "cooldown"
	case strings.Contains(lower, "energy arc"):
		intent.EnergyDirection = "arc"
	case strings.Contains(lower, "steady energy"):
		intent.EnergyDirection = "steady"
	}
	return intent.Normalize()
}

func soft(word string) bool {
	switch strings.ToLower(word) {
	case "around", "roughly", "about", "prefer", "preferably":
		return true
	}
	return false
}

func MergeAudioIntent(model AudioIntent, prompt string) AudioIntent {
	model = model.Normalize()
	explicit := ParseAudioIntent(prompt)
	if explicit.MinBPM > 0 || explicit.MaxBPM > 0 {
		model.MinBPM, model.MaxBPM, model.BPMConstraintHard = explicit.MinBPM, explicit.MaxBPM, explicit.BPMConstraintHard
	}
	if explicit.MinEnergyLevel > 0 || explicit.MaxEnergyLevel > 0 {
		model.MinEnergyLevel, model.MaxEnergyLevel, model.EnergyConstraintHard = explicit.MinEnergyLevel, explicit.MaxEnergyLevel, explicit.EnergyConstraintHard
	}
	if len(explicit.CamelotKeys) > 0 {
		model.CamelotKeys, model.KeyConstraintHard = explicit.CamelotKeys, explicit.KeyConstraintHard
	}
	if explicit.TargetDurationMinutes > 0 {
		model.TargetDurationMinutes = explicit.TargetDurationMinutes
	}
	if len(explicit.PreferredMoods) > 0 {
		model.PreferredMoods = explicit.PreferredMoods
	}
	if explicit.TargetEnergy != "" {
		model.TargetEnergy = explicit.TargetEnergy
	}
	if explicit.EnergyDirection != "" {
		model.EnergyDirection = explicit.EnergyDirection
	}
	model.HarmonicMix = model.HarmonicMix || explicit.HarmonicMix
	return model
}

func usable(value *Observation) bool {
	return value != nil && finite(value.Value) && finite(value.Confidence) && value.Confidence >= .5
}
func finite(value float64) bool { return !math.IsNaN(value) && !math.IsInf(value, 0) }
func within(value, minValue, maxValue float64) bool {
	return (minValue == 0 || value >= minValue) && (maxValue == 0 || value <= maxValue)
}
func Allowed(intent AudioIntent, value AudioFeatures) bool {
	if intent.BPMConstraintHard && (!usable(value.BPM) || !within(value.BPM.Value, intent.MinBPM, intent.MaxBPM)) {
		return false
	}
	if intent.EnergyConstraintHard && (!usable(value.EnergyLevel) || !within(value.EnergyLevel.Value, float64(intent.MinEnergyLevel), float64(intent.MaxEnergyLevel))) {
		return false
	}
	if intent.KeyConstraintHard {
		if value.Key == nil || value.Key.Confidence < .5 {
			return false
		}
		found := false
		for _, key := range intent.CamelotKeys {
			found = found || key == value.Key.Camelot
		}
		if !found {
			return false
		}
	}
	return true
}

func rangeFit(value, minValue, maxValue, scale float64) float64 {
	if within(value, minValue, maxValue) {
		return 1
	}
	distance := 0.0
	if minValue > 0 && value < minValue {
		distance = minValue - value
	}
	if maxValue > 0 && value > maxValue {
		distance = value - maxValue
	}
	return math.Max(0, 1-distance/scale)
}
func confidenceScore(score, confidence float64) float64 {
	return .5 + (score-.5)*math.Max(0, math.Min(1, confidence))
}

// PreferenceScore is neutral for missing evidence, rather than inventing zero
// BPM/energy. Scores from different energy metrics are never substituted.
func PreferenceScore(intent AudioIntent, value AudioFeatures, mood, energyTag string) float64 {
	var scores []float64
	if intent.MinBPM > 0 || intent.MaxBPM > 0 {
		score := .5
		if usable(value.BPM) {
			score = confidenceScore(rangeFit(value.BPM.Value, intent.MinBPM, intent.MaxBPM, 20), value.BPM.Confidence)
		}
		scores = append(scores, score)
	}
	if intent.MinEnergyLevel > 0 || intent.MaxEnergyLevel > 0 {
		score := .5
		if usable(value.EnergyLevel) {
			score = confidenceScore(rangeFit(value.EnergyLevel.Value, float64(intent.MinEnergyLevel), float64(intent.MaxEnergyLevel), 3), value.EnergyLevel.Confidence)
		}
		scores = append(scores, score)
	}
	if intent.TargetEnergy != "" {
		score := .5
		target := map[string]float64{"low": 2, "medium": 5.5, "high": 9}[intent.TargetEnergy]
		if usable(value.EnergyLevel) {
			score = confidenceScore(math.Max(0, 1-math.Abs(value.EnergyLevel.Value-target)/7), value.EnergyLevel.Confidence)
		} else if energyTag == "low" || energyTag == "medium" || energyTag == "high" {
			if strings.EqualFold(energyTag, intent.TargetEnergy) {
				score = .8
			} else {
				score = .2
			}
		}
		scores = append(scores, score)
	}
	if len(intent.PreferredMoods) > 0 {
		score := .5
		if mood != "" {
			score = 0
			for _, preferred := range intent.PreferredMoods {
				if strings.EqualFold(mood, preferred) {
					score = 1
				}
			}
		}
		scores = append(scores, score)
	}
	if len(intent.CamelotKeys) > 0 {
		score := .5
		if value.Key != nil && value.Key.Confidence >= .5 {
			score = 0
			for _, key := range intent.CamelotKeys {
				if key == value.Key.Camelot {
					score = 1
				}
			}
			score = confidenceScore(score, value.Key.Confidence)
		}
		scores = append(scores, score)
	}
	for key, target := range intent.ProviderPreferences {
		score := .5
		if observation, ok := value.ProviderScores[key]; ok && usable(&observation) {
			score = confidenceScore(1-math.Abs(observation.Value-target), observation.Confidence)
		}
		scores = append(scores, score)
	}
	if len(scores) == 0 {
		return .5
	}
	sum := 0.0
	for _, score := range scores {
		sum += score
	}
	return sum / float64(len(scores))
}

// PairScore is advisory. Effective provider observations can inform ordering,
// but this never grants beat-sync authority or relabels them as local analysis.
func PairScore(from, to AudioFeatures, intent AudioIntent) (float64, bool) {
	sum, weight, availableWeight := 0.0, 0.0, 0.0
	if usable(from.BPM) && usable(to.BPM) {
		availableWeight += .25
		w := .25 * math.Min(from.BPM.Confidence, to.BPM.Confidence)
		sum += math.Max(0, 1-math.Abs(from.BPM.Value-to.BPM.Value)/25) * w
		weight += w
	}
	if usable(from.EnergyLevel) && usable(to.EnergyLevel) {
		availableWeight += .3
		delta := 0.0
		switch intent.EnergyDirection {
		case "build":
			delta = 1
		case "cooldown":
			delta = -1
		}
		w := .3 * math.Min(from.EnergyLevel.Confidence, to.EnergyLevel.Confidence)
		sum += math.Max(0, 1-math.Abs(to.EnergyLevel.Value-from.EnergyLevel.Value-delta)/4) * w
		weight += w
	}
	if intent.HarmonicMix && from.Key != nil && to.Key != nil && from.Key.Confidence >= .5 && to.Key.Confidence >= .5 && ValidCamelot(from.Key.Camelot) && ValidCamelot(to.Key.Camelot) {
		availableWeight += .45
		relation, _ := analysiskey.HarmonicRelation(from.Key.Camelot, to.Key.Camelot)
		score := map[string]float64{"same": 1, "adjacent": .9, "relative": .85, "other": .1}[relation]
		w := .45 * math.Min(from.Key.Confidence, to.Key.Confidence)
		sum += score * w
		weight += w
	}
	if weight == 0 {
		return .5, false
	}
	return confidenceScore(sum/weight, weight/availableWeight), true
}
