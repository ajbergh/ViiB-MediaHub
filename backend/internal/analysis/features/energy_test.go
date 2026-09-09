package features

import (
	"reflect"
	"testing"
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
}
