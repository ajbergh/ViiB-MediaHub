// Package cues generates conservative, explainable DJ hot-cue candidates.
package cues

import (
	"errors"
	"fmt"
	"math"
	"sort"

	"github.com/ajbergh/viib-mediahub/internal/analysis/beatgrid"
	"github.com/ajbergh/viib-mediahub/internal/analysis/features"
	"github.com/ajbergh/viib-mediahub/internal/analysis/localcontract"
)

const GeneratorVersion = localcontract.CueGeneratorVersion

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
	position, confidence, salience float64
	kind, label, rationale         string
	downbeatAligned                bool
}

// Generate selects observed events before assigning chronological pad slots.
// A beat is an alignment anchor, never by itself evidence of a useful cue.
// Confidence remains an evidence heuristic, not calibrated DJ acceptance.
func Generate(duration float64, grid *beatgrid.Grid, structure features.Result, sourceFingerprint string) ([]Cue, error) {
	// Persisted and regenerated cues must use the identical decoded timeline,
	// even when catalog duration was rounded or supplied by a metadata provider.
	if finitePositive(structure.DurationSeconds) {
		duration = structure.DurationSeconds
	}
	if !finitePositive(duration) || sourceFingerprint == "" {
		return nil, errors.New("cue generation requires a finite positive duration and source fingerprint")
	}
	if grid != nil {
		if err := grid.Validate(); err != nil {
			return nil, fmt.Errorf("cue generation received invalid beatgrid: %w", err)
		}
	}
	candidates := []cueCandidate{}
	if structure.EventVersion != "" {
		for _, event := range structure.Events {
			if !finiteNonNegative(event.Position) || event.Position >= duration || !finiteNonNegative(event.Strength) || event.Strength > 1 || event.Strength < .30 {
				continue
			}
			candidate := cueCandidate{position: event.Position, confidence: .45 + .30*event.Strength, salience: event.Strength, kind: "section", label: "Section change", rationale: "native-sustained-" + event.Kind}
			switch event.Kind {
			case "start":
				candidate.kind, candidate.label = "intro", "First sound"
			case "energy-rise":
				candidate.label = "Energy rise"
			case "energy-fall":
				candidate.label = "Energy fall"
			case "section":
				candidate.label = "Texture change"
			default:
				continue
			}
			candidates = append(candidates, candidate)
		}
	} else {
		// Legacy/manual feature fixtures have only sections. Use starts rather
		// than track-end points; never invent eight arbitrary beat candidates.
		for i, section := range validSections(structure.Sections, duration) {
			if section.Energy <= 0 {
				continue
			}
			c := cueCandidate{position: section.Start, confidence: .48, salience: .4, kind: "section", label: "Section change", rationale: "measured-section-boundary"}
			if i == 0 {
				c.kind, c.label = "intro", "First section"
			}
			candidates = append(candidates, c)
		}
	}
	for i := range candidates {
		alignCandidate(&candidates[i], grid, duration)
	}
	candidates = deduplicate(candidates)
	// Keep the opening event, then prefer strong distinct events. No duration
	// fractions or forced semantic roles: flat material can yield just one cue.
	sort.SliceStable(candidates, func(i, j int) bool {
		if (candidates[i].kind == "intro") != (candidates[j].kind == "intro") {
			return candidates[i].kind == "intro"
		}
		return .7*candidates[i].salience+.3*candidates[i].confidence > .7*candidates[j].salience+.3*candidates[j].confidence
	})
	chosen := []cueCandidate{}
	for _, c := range candidates {
		close := false
		for _, existing := range chosen {
			spacing := math.Max(cueSpacing(grid, c.position), cueSpacing(grid, existing.position))
			if math.Abs(c.position-existing.position) < spacing {
				close = true
				break
			}
		}
		if !close {
			chosen = append(chosen, c)
		}
		if len(chosen) == 8 {
			break
		}
	}
	sort.Slice(chosen, func(i, j int) bool { return chosen[i].position < chosen[j].position })
	result := make([]Cue, 0, len(chosen))
	for i, c := range chosen {
		result = append(result, Cue{Slot: i + 1, Position: c.position, Label: c.label, Kind: c.kind, Color: "#FF5500", Origin: "analysis", GeneratorVersion: GeneratorVersion, Confidence: clamp01(c.confidence), Rationale: c.rationale, SourceFingerprint: sourceFingerprint, DownbeatAligned: c.downbeatAligned})
	}
	return result, nil
}

func nearestBeat(grid *beatgrid.Grid, position float64) int {
	i := sort.SearchFloat64s(grid.Beats, position)
	if i == len(grid.Beats) || i > 0 && position-grid.Beats[i-1] <= grid.Beats[i]-position {
		i--
	}
	return i
}

func localBeatInterval(grid *beatgrid.Grid, position float64) float64 {
	if grid == nil || len(grid.Beats) < 2 {
		return .5
	}
	i := nearestBeat(grid, position)
	if i == len(grid.Beats)-1 {
		i--
	}
	return grid.Beats[i+1] - grid.Beats[i]
}

func cueSpacing(grid *beatgrid.Grid, position float64) float64 {
	return math.Max(1, math.Min(8, 4*localBeatInterval(grid, position)))
}

func alignCandidate(c *cueCandidate, grid *beatgrid.Grid, duration float64) {
	if grid == nil {
		return
	}
	// Refuse large snaps even when the nearest downbeat has high provenance.
	// A missing bar-one detector cannot be repaired by moving an event a bar.
	tolerance := math.Min(.2, .35*localBeatInterval(grid, c.position))
	p := grid.EffectiveProvenance()
	if p == beatgrid.ProvenanceMeasured || p == beatgrid.ProvenanceManual {
		i := sort.Search(len(grid.DownbeatIndices), func(i int) bool { return grid.Beats[grid.DownbeatIndices[i]] >= c.position })
		best, distance := -1, math.Inf(1)
		for _, j := range []int{i - 1, i} {
			if j < 0 || j >= len(grid.DownbeatIndices) {
				continue
			}
			b := grid.DownbeatIndices[j]
			if d := math.Abs(grid.Beats[b] - c.position); d < distance && grid.Beats[b] < duration {
				best, distance = b, d
			}
		}
		if best >= 0 && distance <= tolerance {
			c.position, c.downbeatAligned = grid.Beats[best], true
			// These exact provenance strings are validated by cue storage.
			c.rationale = fmt.Sprintf("qualified-%s-downbeat", p)
			return
		}
	}
	i := nearestBeat(grid, c.position)
	if math.Abs(grid.Beats[i]-c.position) <= tolerance && grid.Beats[i] < duration {
		c.position = grid.Beats[i]
		c.rationale += ";nearest-beat"
	} else {
		c.rationale += ";unaligned-event"
	}
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

func finitePositive(value float64) bool {
	return value > 0 && !math.IsNaN(value) && !math.IsInf(value, 0)
}

func finiteNonNegative(value float64) bool {
	return value >= 0 && !math.IsNaN(value) && !math.IsInf(value, 0)
}

func clamp01(value float64) float64 { return math.Max(0, math.Min(1, value)) }
