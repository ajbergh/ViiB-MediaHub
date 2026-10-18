// Tests and fixtures for generator behavior.

package cues

import (
	"math"
	"testing"

	"github.com/ajbergh/viib-mediahub/internal/analysis/beatgrid"
	"github.com/ajbergh/viib-mediahub/internal/analysis/features"
)

func TestGenerateUsesSectionEventsAndOnlyQualifiedDownbeats(t *testing.T) {
	grid, err := beatgrid.BuildStraight(120, 0, 128, 4)
	if err != nil {
		t.Fatal(err)
	}
	grid.Provenance = beatgrid.ProvenanceMeasured
	structure := features.Result{Sections: []features.Section{
		{Start: 0, End: 24, Energy: .3}, {Start: 24, End: 48, Energy: .8},
		{Start: 48, End: 72, Energy: .2}, {Start: 72, End: 104, Energy: 1}, {Start: 104, End: 128, Energy: .5},
	}}
	generated, err := Generate(128, &grid, structure, "source-hash")
	if err != nil {
		t.Fatal(err)
	}
	if len(generated) != len(structure.Sections) {
		t.Fatalf("generated %d cues, want section starts only: %#v", len(generated), generated)
	}
	previousPosition := -1.0
	for index, cue := range generated {
		if cue.Slot != index+1 || cue.Position <= previousPosition || cue.Position >= 128 {
			t.Fatalf("cue ordering/position invalid at %d: %#v", index, cue)
		}
		if cue.Origin != "analysis" || cue.GeneratorVersion != GeneratorVersion || cue.SourceFingerprint != "source-hash" || cue.Locked || cue.Confidence <= 0 || cue.Confidence > 1 {
			t.Fatalf("cue provenance invalid: %#v", cue)
		}
		previousPosition = cue.Position
	}
	if generated[0].Kind != "intro" || generated[1].Kind != "section" {
		t.Fatalf("cue roles do not describe observed events: %#v", generated)
	}
	if !generated[0].DownbeatAligned || generated[0].Rationale != "qualified-measured-downbeat" {
		t.Fatalf("qualified downbeat not recorded: %#v", generated[0])
	}
	grid.Provenance = beatgrid.ProvenanceInferredFromMeter
	inferred, err := Generate(128, &grid, structure, "source-hash")
	if err != nil {
		t.Fatal(err)
	}
	for _, cue := range inferred {
		if cue.DownbeatAligned || cue.Rationale == "qualified-inferred-from-meter-downbeat" {
			t.Fatalf("inferred grid was advertised as downbeat-aligned: %#v", cue)
		}
	}
}

func TestGeneratePrefersActualEventsToDurationFractions(t *testing.T) {
	structure := features.Result{DurationSeconds: 240, EventVersion: features.EventVersion, Events: []features.StructureEvent{
		{Position: 3.25, Kind: "start", Strength: .7},
		{Position: 43.35, Kind: "energy-rise", Strength: .9},
		{Position: 117.1, Kind: "section", Strength: .65},
		{Position: 193.2, Kind: "energy-fall", Strength: .8},
	}}
	grid, _ := beatgrid.BuildStraight(120, 0, 240, 4)
	grid.Provenance = beatgrid.ProvenanceManual
	generated, err := Generate(240, &grid, structure, "source")
	if err != nil || len(generated) != 4 {
		t.Fatalf("events=%#v err=%v", generated, err)
	}
	for i, c := range generated {
		if math.Abs(c.Position-structure.Events[i].Position) > .175 || c.Slot != i+1 {
			t.Fatalf("cue moved away from musical event: %#v", c)
		}
	}
	if generated[1].Label != "Energy rise" || generated[3].Label != "Energy fall" {
		t.Fatalf("labels not evidence-based: %#v", generated)
	}
}

func TestGenerateAbstainsWithoutEventsAndDoesNotPadSlots(t *testing.T) {
	grid, _ := beatgrid.BuildStraight(120, 0, 240, 4)
	for _, events := range [][]features.StructureEvent{nil, {{Position: 1, Kind: "start", Strength: .7}}} {
		cues, err := Generate(240, &grid, features.Result{DurationSeconds: 240, EventVersion: features.EventVersion, Events: events, Sections: []features.Section{{Start: 0, End: 240, Energy: 1}}}, "source")
		if err != nil || len(cues) != len(events) {
			t.Fatalf("invented cues: %#v, %v", cues, err)
		}
	}
}

func TestGenerateBoundsSnapsAndRetainsDownbeatProvenance(t *testing.T) {
	for _, p := range []beatgrid.Provenance{beatgrid.ProvenanceManual, beatgrid.ProvenanceMeasured, beatgrid.ProvenanceInferredFromMeter, beatgrid.ProvenanceSpotify} {
		grid, _ := beatgrid.BuildStraight(120, 0, 80, 4)
		grid.Provenance = p
		structure := features.Result{DurationSeconds: 80, EventVersion: features.EventVersion, Events: []features.StructureEvent{{Position: 8.1, Kind: "section", Strength: .8}, {Position: 25.05, Kind: "energy-rise", Strength: .8}, {Position: 42.25, Kind: "section", Strength: .8}}}
		cues, err := Generate(80, &grid, structure, "source")
		if err != nil || len(cues) != 3 {
			t.Fatalf("%s cues=%#v err=%v", p, cues, err)
		}
		qualified := p == beatgrid.ProvenanceManual || p == beatgrid.ProvenanceMeasured
		if cues[0].Position != 8 || cues[0].DownbeatAligned != qualified {
			t.Fatalf("%s downbeat provenance lost: %#v", p, cues[0])
		}
		if cues[1].Position != 25 || cues[1].DownbeatAligned {
			t.Fatalf("beat halfway through bar moved to downbeat: %#v", cues[1])
		}
		if cues[2].Position != 42.25 || cues[2].DownbeatAligned {
			t.Fatalf("out-of-tolerance event was snapped: %#v", cues[2])
		}
	}
}

func TestGenerateRanksDeduplicatesAndUsesDecodedDuration(t *testing.T) {
	structure := features.Result{DurationSeconds: 200, EventVersion: features.EventVersion, Events: []features.StructureEvent{{Position: 0, Kind: "start", Strength: .7}, {Position: 19, Kind: "section", Strength: .3}, {Position: 19.02, Kind: "section", Strength: .95}}}
	for i := 1; i <= 9; i++ {
		structure.Events = append(structure.Events, features.StructureEvent{Position: float64(20 * i), Kind: "energy-rise", Strength: .8})
	}
	cues, err := Generate(60, nil, structure, "source")
	if err != nil || len(cues) != 8 {
		t.Fatalf("cues=%#v err=%v", cues, err)
	}
	if cues[0].Position != 0 || cues[1].Position != 19.02 {
		t.Fatalf("start/strong duplicate not retained: %#v", cues)
	}
	for _, c := range cues {
		if c.Position >= 200 || c.Position == 19 || c.Position == 20 {
			t.Fatalf("invalid/duplicate/weak event selected: %#v", c)
		}
	}
	if cues[len(cues)-1].Position <= 60 {
		t.Fatal("catalog duration truncated decoded track")
	}
}

func TestGenerateWithoutGridIsUnalignedAndHonorsShortTracks(t *testing.T) {
	generated, err := Generate(20, nil, features.Result{Sections: []features.Section{{Start: 0, End: 20, Energy: .7}}}, "source-hash")
	if err != nil {
		t.Fatal(err)
	}
	if len(generated) == 0 || len(generated) > 5 {
		t.Fatalf("short track produced unexpected cue count %d: %#v", len(generated), generated)
	}
	for _, cue := range generated {
		if cue.DownbeatAligned || cue.Rationale != "measured-section-boundary" {
			t.Fatalf("no-grid cue claims beat alignment: %#v", cue)
		}
	}
	if _, err := Generate(0, nil, features.Result{}, "source-hash"); err == nil {
		t.Fatal("zero-duration input accepted")
	}
}

func TestGenerateRejectsUnknownEventSchemaAndBadDuration(t *testing.T) {
	for _, r := range []features.Result{{EventVersion: "future", DurationSeconds: 20}, {EventVersion: features.EventVersion}, {EventVersion: features.EventVersion, DurationSeconds: math.NaN()}, {EventVersion: features.EventVersion, DurationSeconds: 20, Events: []features.StructureEvent{{Position: 21, Kind: "start", Strength: .7}}}} {
		if _, err := Generate(20, nil, r, "source"); err == nil {
			t.Fatalf("invalid native schema silently accepted: %#v", r)
		}
	}
}

func TestApplyPolicyPreservesUserLockedAndSuppressedCues(t *testing.T) {
	confidence := .7
	existing := []Cue{
		{Slot: 1, Position: 4, Origin: "user", Label: "Manual"},
		{Slot: 2, Position: 8, Origin: "analysis", Kind: "mix-in", Locked: true},
		{Slot: 3, Position: 12, Origin: "analysis", Kind: "section", GeneratorVersion: GeneratorVersion, Confidence: confidence},
	}
	generated := []Cue{
		{Slot: 1, Position: 5, Origin: "analysis", GeneratorVersion: GeneratorVersion, Confidence: .8, Kind: "intro"},
		{Slot: 2, Position: 9, Origin: "analysis", GeneratorVersion: GeneratorVersion, Confidence: .8, Kind: "mix-in"},
		{Slot: 3, Position: 13, Origin: "analysis", GeneratorVersion: GeneratorVersion, Confidence: .8, Kind: "section"},
		{Slot: 4, Position: 16, Origin: "analysis", GeneratorVersion: GeneratorVersion, Confidence: .8, Kind: "section"},
	}
	merged, err := ApplyPolicy(existing, generated, ApplyReplaceGenerated, []Suppression{{Slot: 3, Kind: "section"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(merged) != 3 || merged[0].Position != 4 || merged[1].Position != 8 || merged[2].Slot != 4 {
		t.Fatalf("merged cues did not preserve user/locked/tombstoned slots: %#v", merged)
	}
	merged, err = ApplyPolicy(existing, generated, ApplyFillEmpty, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(merged) != 4 || merged[2].Position != 12 || merged[3].Slot != 4 {
		t.Fatalf("fill-empty replaced an existing generated cue: %#v", merged)
	}
}

func TestEventSlotsRespectSuppressionAcrossRoleChanges(t *testing.T) {
	generated := []Cue{{Slot: 2, Position: 17, Origin: "analysis", GeneratorVersion: GeneratorVersion, Kind: "section", Confidence: .7}}
	for _, mode := range []ApplyMode{ApplyFillEmpty, ApplyReplaceGenerated} {
		got, err := ApplyPolicy(nil, generated, mode, []Suppression{{Slot: 2, Kind: "mix-in"}})
		if err != nil || len(got) != 0 {
			t.Fatalf("new role resurrected deleted pad: %#v, %v", got, err)
		}
	}
}
