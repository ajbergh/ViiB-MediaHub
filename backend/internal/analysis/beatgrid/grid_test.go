package beatgrid

import (
	"math"
	"reflect"
	"testing"
)

func TestBuildStraightPreservesFirstDownbeatAndMeter(t *testing.T) {
	grid, err := BuildStraight(120, 0.125, 2.2, 4)
	if err != nil {
		t.Fatal(err)
	}
	wantBeats := []float64{0.125, 0.625, 1.125, 1.625, 2.125}
	if !closeEnough(grid.Beats, wantBeats) {
		t.Fatalf("beats = %#v, want %#v", grid.Beats, wantBeats)
	}
	if !reflect.DeepEqual(grid.DownbeatIndices, []int{0, 4}) {
		t.Fatalf("downbeats = %#v, want [0 4]", grid.DownbeatIndices)
	}
}

func TestEncodeDecodeRoundTripIsDeterministic(t *testing.T) {
	original := Grid{Beats: []float64{0.012345, 0.481234, 0.950123, 1.419012}, DownbeatIndices: []int{0}}
	first, err := original.Encode()
	if err != nil {
		t.Fatal(err)
	}
	second, err := original.Encode()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatal("encoding must be deterministic")
	}
	decoded, err := Decode(first)
	if err != nil {
		t.Fatal(err)
	}
	if !closeEnough(decoded.Beats, original.Beats) || !reflect.DeepEqual(decoded.DownbeatIndices, original.DownbeatIndices) {
		t.Fatalf("decoded = %#v, want %#v", decoded, original)
	}
}

func TestDecodeRejectsMalformedArtifacts(t *testing.T) {
	for _, data := range [][]byte{
		nil,
		{'V', 'B', 'G', 2},
		{'V', 'B', 'G', 1, 0},
		{'V', 'B', 'G', 1, 1, 0, 1, 1}, // one beat but an out-of-range downbeat
	} {
		if _, err := Decode(data); err == nil {
			t.Fatalf("Decode(%v) succeeded, want error", data)
		}
	}
}

func TestValidateRejectsAmbiguousTiming(t *testing.T) {
	invalid := []Grid{
		{},
		{Beats: []float64{0, 0}},
		{Beats: []float64{0, math.NaN()}},
		{Beats: []float64{0, 0.5}, DownbeatIndices: []int{2}},
	}
	for _, grid := range invalid {
		if err := grid.Validate(); err == nil {
			t.Fatalf("Validate(%#v) succeeded, want error", grid)
		}
	}
}

func TestEncodeRejectsTimestampsThatCollapseAtMicrosecondPrecision(t *testing.T) {
	grid := Grid{Beats: []float64{0.0000001, 0.0000002}}
	if _, err := grid.Encode(); err == nil {
		t.Fatal("Encode() succeeded for timestamps that collapse at microsecond precision")
	}
}

func closeEnough(got, want []float64) bool {
	if len(got) != len(want) {
		return false
	}
	for index := range got {
		if math.Abs(got[index]-want[index]) > 0.000001 {
			return false
		}
	}
	return true
}
