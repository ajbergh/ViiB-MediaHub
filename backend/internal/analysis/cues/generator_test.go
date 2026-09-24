package cues

import (
	"testing"

	"github.com/ajbergh/viib-mediahub/internal/analysis/beatgrid"
	"github.com/ajbergh/viib-mediahub/internal/analysis/features"
)

func TestGenerateUsesEightSlotsAndOnlyQualifiedDownbeats(t *testing.T) {
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
	if len(generated) != 8 {
		t.Fatalf("generated %d cues, want eight: %#v", len(generated), generated)
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
	if generated[1].Kind != "mix-in" || generated[6].Kind != "mix-out" {
		t.Fatalf("mix cues missing from policy: %#v", generated)
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
