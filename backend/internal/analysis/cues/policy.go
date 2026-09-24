package cues

import (
	"errors"
	"math"
	"sort"
)

type ApplyMode string

const (
	ApplyFillEmpty        ApplyMode = "fill-empty"
	ApplyReplaceGenerated ApplyMode = "replace-generated"
)

type Suppression struct {
	Slot int
	Kind string
}

// ApplyPolicy merges new analysis candidates into the eight user slots.
// Missing origin is treated as user-owned for legacy safety. Suppressed slots
// represent persisted user-deletion tombstones and are never regenerated.
func ApplyPolicy(existing, generated []Cue, mode ApplyMode, suppressions []Suppression) ([]Cue, error) {
	if mode != ApplyFillEmpty && mode != ApplyReplaceGenerated {
		return nil, errors.New("cue apply mode must be fill-empty or replace-generated")
	}
	bySlot := make(map[int]Cue, 8)
	for _, cue := range existing {
		if cue.Slot < 1 || cue.Slot > 8 {
			return nil, errors.New("existing cue slot must be between 1 and 8")
		}
		if _, exists := bySlot[cue.Slot]; exists {
			return nil, errors.New("existing cues contain duplicate slots")
		}
		if cue.Origin == "" {
			cue.Origin = "user"
		}
		bySlot[cue.Slot] = cue
	}
	suppressed := make(map[int]map[string]bool, len(suppressions))
	for _, item := range suppressions {
		if item.Slot < 1 || item.Slot > 8 || item.Kind == "" {
			return nil, errors.New("cue suppression has invalid slot or kind")
		}
		if suppressed[item.Slot] == nil {
			suppressed[item.Slot] = make(map[string]bool)
		}
		suppressed[item.Slot][item.Kind] = true
	}
	generatedSlots := make(map[int]struct{}, len(generated))
	for _, cue := range generated {
		if cue.Slot < 1 || cue.Slot > 8 || cue.Origin != "analysis" || cue.GeneratorVersion == "" || cue.Kind == "" || math.IsNaN(cue.Confidence) || math.IsInf(cue.Confidence, 0) || cue.Confidence < 0 || cue.Confidence > 1 || math.IsNaN(cue.Position) || math.IsInf(cue.Position, 0) || cue.Position < 0 {
			return nil, errors.New("generated cue has invalid slot or provenance")
		}
		if _, exists := generatedSlots[cue.Slot]; exists {
			return nil, errors.New("generated cues contain duplicate slots")
		}
		generatedSlots[cue.Slot] = struct{}{}
	}
	if mode == ApplyReplaceGenerated {
		for slot, cue := range bySlot {
			if cue.Origin == "analysis" && !cue.Locked {
				delete(bySlot, slot)
			}
		}
	}
	for _, cue := range generated {
		if suppressed[cue.Slot][cue.Kind] {
			continue
		}
		current, exists := bySlot[cue.Slot]
		if exists {
			if current.Origin != "analysis" || current.Locked || mode == ApplyFillEmpty {
				continue
			}
		}
		bySlot[cue.Slot] = cue
	}
	result := make([]Cue, 0, len(bySlot))
	for _, cue := range bySlot {
		result = append(result, cue)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Slot < result[j].Slot })
	return result, nil
}
