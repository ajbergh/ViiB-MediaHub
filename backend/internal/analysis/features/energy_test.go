package features

import (
	"bytes"
	"compress/gzip"
	"reflect"
	"testing"

	"github.com/ajbergh/viib-mediahub/internal/analysis/beatgrid"
)

func TestEnergyResultAndArtifactAreDeterministic(t *testing.T) {
	accumulator := NewAccumulator(100)
	samples := make([]float32, 1_000)
	for index := 400; index < len(samples); index++ {
		samples[index] = .8
	}
	accumulator.Feed(samples[:333])
	accumulator.Feed(samples[333:])
	result, err := accumulator.Result()
	if err != nil {
		t.Fatal(err)
	}
	result.AnnotateStructure(nil)
	if len(result.Energy) != 20 || len(result.Sections) < 2 || result.Energy[0].Value >= result.Energy[len(result.Energy)-1].Value {
		t.Fatalf("unexpected measured structure: %#v", result)
	}
	first, err := result.Encode()
	if err != nil {
		t.Fatal(err)
	}
	second, err := result.Encode()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatal("artifact encoding is not deterministic")
	}
	decoded, err := Decode(first)
	if err != nil || !reflect.DeepEqual(decoded, result) {
		t.Fatalf("Decode() = %#v, %v; want %#v", decoded, err, result)
	}
	if decoded.LoudnessKind != LoudnessKind || decoded.PeakKind != PeakKind || decoded.ChannelScope != "mono" || decoded.Standard != "none" {
		t.Fatalf("artifact does not state measurement semantics: %#v", decoded)
	}
	if decoded.IntegratedLUFS != result.IntegratedLUFS || decoded.TruePeakDBFS != result.TruePeakDBFS {
		t.Fatalf("compatibility numeric aliases changed: %#v", decoded)
	}
}

func TestAnnotateStructureIsDeterministicAndConservative(t *testing.T) {
	makeResult := func() Result {
		result := Result{Sections: []Section{
			{Start: 0, End: 8, Energy: .15},
			{Start: 8, End: 16, Energy: .45},
			{Start: 16, End: 24, Energy: 1},
			{Start: 24, End: 32, Energy: .18},
		}}
		result.AnnotateStructure(nil)
		return result
	}
	first, second := makeResult(), makeResult()
	if !reflect.DeepEqual(first.Sections, second.Sections) {
		t.Fatalf("structure annotation is not deterministic: %#v != %#v", first.Sections, second.Sections)
	}
	got := []string{first.Sections[0].Label, first.Sections[1].Label, first.Sections[2].Label, first.Sections[3].Label}
	want := []string{StructureIntro, StructureBuild, StructureDrop, StructureOutro}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("labels = %v, want %v", got, want)
	}
	for _, section := range first.Sections {
		if section.Confidence <= 0 || section.Confidence > .5 || section.TimingProvenance != TimingEnergyWindows {
			t.Fatalf("section overstates available evidence: %#v", section)
		}
	}

	ambiguous := Result{Sections: []Section{{Start: 0, End: 8, Energy: .6}, {Start: 8, End: 16, Energy: .6}, {Start: 16, End: 24, Energy: .6}}}
	ambiguous.AnnotateStructure(nil)
	for _, section := range ambiguous.Sections {
		if section.Label != StructureUnknown || section.Confidence > .2 {
			t.Fatalf("ambiguous flat energy was forced into a label: %#v", section)
		}
	}
}

func TestAnnotateStructureShortAndSilentTracksStayUnknown(t *testing.T) {
	for name, sections := range map[string][]Section{
		"short":   {{Start: 0, End: .5, Energy: 1}},
		"silence": {{Start: 0, End: 5, Energy: 0}, {Start: 5, End: 10, Energy: 0}},
	} {
		t.Run(name, func(t *testing.T) {
			result := Result{Sections: sections}
			result.AnnotateStructure(nil)
			for _, section := range result.Sections {
				if section.Label != StructureUnknown || section.Confidence > .2 {
					t.Fatalf("weak/absent evidence produced a semantic label: %#v", section)
				}
			}
		})
	}
}

func TestAnnotateStructureAddsOnlySupportedDownbeatBoundaries(t *testing.T) {
	result := Result{Sections: []Section{{Start: .1, End: 4.1, Energy: .2}, {Start: 4.1, End: 8.1, Energy: .9}}}
	grid := &beatgrid.Grid{Beats: []float64{.12, 1.12, 2.12, 3.12, 4.12, 5.12, 6.12, 7.12}, DownbeatIndices: []int{0, 4}, Provenance: beatgrid.ProvenanceMeasured}
	result.AnnotateStructure(grid)
	if result.Sections[0].DownbeatStart == nil || result.Sections[0].DownbeatEnd == nil || *result.Sections[0].DownbeatStart != .12 || *result.Sections[0].DownbeatEnd != 4.12 {
		t.Fatalf("nearby measured downbeats not preserved: %#v", result.Sections[0])
	}
	if result.Sections[0].Start != .1 || result.Sections[0].End != 4.1 || result.Sections[0].TimingProvenance != TimingDownbeatGrid {
		t.Fatalf("annotation changed section bounds or lost provenance: %#v", result.Sections[0])
	}

	inferred := Result{Sections: []Section{{Start: .1, End: 4.1, Energy: .2}}}
	inferred.AnnotateStructure(&beatgrid.Grid{Beats: grid.Beats, DownbeatIndices: grid.DownbeatIndices, Provenance: beatgrid.ProvenanceInferredFromMeter})
	if inferred.Sections[0].DownbeatStart != nil || inferred.Sections[0].TimingProvenance != TimingEnergyWindows {
		t.Fatalf("inferred meter grid was promoted to downbeat timing: %#v", inferred.Sections[0])
	}
}

func TestDecodeLegacyArtifactWithoutStructureFields(t *testing.T) {
	var payload bytes.Buffer
	writer := gzip.NewWriter(&payload)
	if _, err := writer.Write([]byte(`{"energy":[{"time":0,"value":0.5}],"sections":[{"start":0,"end":0.5,"energy":0.5}]}`)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	decoded, err := Decode(payload.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Sections[0].Label != "" || decoded.Sections[0].TimingProvenance != "" || decoded.Sections[0].Confidence != 0 {
		t.Fatalf("legacy section did not retain zero-value compatibility: %#v", decoded.Sections[0])
	}
	encoded, err := decoded.Encode()
	if err != nil {
		t.Fatal(err)
	}
	roundTrip, err := Decode(encoded)
	if err != nil || !reflect.DeepEqual(roundTrip, decoded) {
		t.Fatalf("legacy artifact round trip = %#v, %v; want %#v", roundTrip, err, decoded)
	}
}

func TestCueSuggestionsSnapToDownbeats(t *testing.T) {
	result := Result{Energy: []EnergyPoint{{Time: 0, Value: .2}}, Sections: []Section{{Start: .12, End: 3.9, Energy: .2}, {Start: 4.13, End: 8.1, Energy: .8}}}
	grid := &beatgrid.Grid{Beats: []float64{.1, .6, 1.1, 1.6, 2.1, 2.6, 3.1, 3.6, 4.1, 4.6, 5.1, 5.6, 6.1, 6.6, 7.1, 7.6, 8.1}, DownbeatIndices: []int{0, 8, 16}}
	result.AddCueSuggestions(grid)
	if len(result.CueSuggestions) != 3 || result.CueSuggestions[0].Position != .1 || result.CueSuggestions[2].Position != 4.1 {
		t.Fatalf("cue suggestions = %#v", result.CueSuggestions)
	}
	if result.CueSuggestions[0].Confidence >= .5 || result.CueSuggestions[0].Rationale == "" {
		t.Fatalf("inferred grid cue did not carry conservative rationale/confidence: %#v", result.CueSuggestions[0])
	}
	grid.Provenance = beatgrid.ProvenanceManual
	result.AddCueSuggestions(grid)
	if result.CueSuggestions[0].Confidence < .5 {
		t.Fatalf("manual downbeat cue confidence unexpectedly low: %#v", result.CueSuggestions[0])
	}
}
