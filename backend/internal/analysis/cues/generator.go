// Package cues generates conservative, explainable DJ hot-cue candidates.
package cues

import (
	"errors"
	"fmt"
	"math"
	"sort"

	"github.com/ajbergh/viib-mediahub/internal/analysis/beatgrid"
	"github.com/ajbergh/viib-mediahub/internal/analysis/features"
)

const GeneratorVersion = "auto-cues-v1"

// Cue is an analysis-owned candidate with enough metadata to persist and
// distinguish it from user-authored hot cues.
type Cue struct {
	Slot              int     `json:"slot"`
	Position          float64 `json:"position"`
	Label             string  `json:"label"`
	Color             string  `json:"color"`
	Origin            string  `json:"origin"`
	GeneratorVersion  string  `json:"generatorVersion"`
	Confidence        float64 `json:"confidence"`
	Kind              string  `json:"kind"`
	Locked            bool    `json:"locked"`
	Rationale         string  `json:"rationale"`
	SourceFingerprint string  `json:"sourceFingerprint"`
	DownbeatAligned   bool    `json:"downbeatAligned"`
}

type cueCandidate struct {
	position        float64
	confidence      float64
	salience        float64
	downbeatAligned bool
	rationale       string
}

var slotTargets = [...]float64{0.015, 0.10, 0.25, 0.40, 0.55, 0.70, 0.84, 0.96}
var slotKinds = [...]string{"intro", "mix-in", "section", "section", "section", "section", "mix-out", "outro"}
var slotLabels = [...]string{"Intro", "Mix In", "Section 1", "Section 2", "Section 3", "Section 4", "Mix Out", "Outro"}

// Generate proposes up to eight ordered cue points from measured sections and
// an optional beatgrid. It never treats inferred meter placeholders as
// musical downbeats: those grids may provide a lower-confidence nearest-beat
// snap, while downbeatAligned is true only for measured/manual provenance.
func Generate(duration float64, grid *beatgrid.Grid, structure features.Result, sourceFingerprint string) ([]Cue, error) {
	if !finitePositive(duration) || sourceFingerprint == "" {
		return nil, errors.New("cue generation requires a finite positive duration and source fingerprint")
	}
	if grid != nil {
		if err := grid.Validate(); err != nil {
			return nil, fmt.Errorf("cue generation received invalid beatgrid: %w", err)
		}
	}

	sections := validSections(structure.Sections, duration)
	candidates := make([]cueCandidate, 0, len(sections)*2+32)
	for _, section := range sections {
		salience := clamp01(section.Energy)
		candidates = append(candidates,
			cueCandidate{position: section.Start, confidence: .48 + salience*.20, salience: salience, rationale: "measured-section-boundary"},
			cueCandidate{position: section.End, confidence: .44 + salience*.18, salience: salience, rationale: "measured-section-boundary"},
		)
	}

	if grid != nil {
		provenance := grid.EffectiveProvenance()
		downbeatSet := make(map[int]struct{}, len(grid.DownbeatIndices))
		for _, index := range grid.DownbeatIndices {
			downbeatSet[index] = struct{}{}
		}
		qualifiedDownbeats := provenance == beatgrid.ProvenanceMeasured || provenance == beatgrid.ProvenanceManual
		for index, position := range grid.Beats {
			if position >= duration {
				break
			}
			if _, isDownbeat := downbeatSet[index]; isDownbeat && qualifiedDownbeats {
				candidates = append(candidates, cueCandidate{position: position, confidence: .88, salience: .55, downbeatAligned: true, rationale: fmt.Sprintf("qualified-%s-downbeat", provenance)})
			} else if _, isDownbeat := downbeatSet[index]; isDownbeat {
				// Meter-derived bar starts are only beat candidates. Reduce their
				// score/confidence and never describe them as downbeat-aligned.
				candidates = append(candidates, cueCandidate{position: position, confidence: .43, salience: .45, rationale: "nearest-beat-inferred-from-meter"})
			} else {
				candidates = append(candidates, cueCandidate{position: position, confidence: .54, salience: .48, rationale: "nearest-beat"})
			}
		}
	}
	if len(candidates) == 0 {
		return []Cue{}, nil
	}
	candidates = deduplicate(candidates)
	if len(candidates) == 0 {
		return []Cue{}, nil
	}

	minimumSpacing := math.Max(4, duration*.025)
	used := make([]bool, len(candidates))
	chosenPositions := make([]float64, 0, 8)
	result := make([]Cue, 0, 8)
	for slotIndex, fraction := range slotTargets {
		target := fraction * duration
		bestIndex, bestScore := -1, math.Inf(-1)
		for candidateIndex, candidate := range candidates {
			if used[candidateIndex] || tooClose(candidate.position, chosenPositions, minimumSpacing) {
				continue
			}
			if len(chosenPositions) > 0 && candidate.position <= chosenPositions[len(chosenPositions)-1] {
				continue
			}
			distance := math.Abs(candidate.position - target)
			proximity := 1 - math.Min(1, distance/math.Max(duration*.20, minimumSpacing))
			score := .62*proximity + .23*candidate.confidence + .15*candidate.salience
			if score > bestScore {
				bestIndex, bestScore = candidateIndex, score
			}
		}
		if bestIndex < 0 {
			continue
		}
		candidate := candidates[bestIndex]
		used[bestIndex] = true
		chosenPositions = append(chosenPositions, candidate.position)
		confidence := clamp01(candidate.confidence * (.72 + .28*bestScore))
		result = append(result, Cue{
			Slot: slotIndex + 1, Position: candidate.position, Label: slotLabels[slotIndex],
			Color: "#FF5500", Origin: "analysis", GeneratorVersion: GeneratorVersion,
			Confidence: confidence, Kind: slotKinds[slotIndex], Locked: false,
			Rationale: candidate.rationale, SourceFingerprint: sourceFingerprint, DownbeatAligned: candidate.downbeatAligned,
		})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Slot < result[j].Slot })
	return result, nil
}

func validSections(sections []features.Section, duration float64) []features.Section {
	result := make([]features.Section, 0, len(sections))
	for _, section := range sections {
		if !finiteNonNegative(section.Start) || !finitePositive(section.End) || section.Start >= section.End || section.Start >= duration {
			continue
		}
		section.End = math.Min(section.End, math.Nextafter(duration, 0))
		if section.Start < section.End && !math.IsNaN(section.Energy) && !math.IsInf(section.Energy, 0) {
			result = append(result, section)
		}
	}
	sort.SliceStable(result, func(i, j int) bool { return result[i].Start < result[j].Start })
	return result
}

func deduplicate(candidates []cueCandidate) []cueCandidate {
	sort.SliceStable(candidates, func(i, j int) bool { return candidates[i].position < candidates[j].position })
	result := make([]cueCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if !finiteNonNegative(candidate.position) || !finiteNonNegative(candidate.confidence) || !finiteNonNegative(candidate.salience) {
			continue
		}
		if len(result) > 0 && candidate.position-result[len(result)-1].position < .05 {
			if candidate.confidence > result[len(result)-1].confidence {
				result[len(result)-1] = candidate
			}
			continue
		}
		result = append(result, candidate)
	}
	return result
}

func tooClose(position float64, chosen []float64, minimum float64) bool {
	for _, existing := range chosen {
		if math.Abs(position-existing) < minimum {
			return true
		}
	}
	return false
}

func finitePositive(value float64) bool {
	return value > 0 && !math.IsNaN(value) && !math.IsInf(value, 0)
}

func finiteNonNegative(value float64) bool {
	return value >= 0 && !math.IsNaN(value) && !math.IsInf(value, 0)
}

func clamp01(value float64) float64 { return math.Max(0, math.Min(1, value)) }
