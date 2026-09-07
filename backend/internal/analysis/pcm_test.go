package analysis

import "testing"

func TestDownmixInterleavedAveragesEveryChannel(t *testing.T) {
	mono, err := DownmixInterleaved([]float32{1, -1, .5, .5, -.5, .5}, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(mono) != 3 || mono[0] != 0 || mono[1] != .5 || mono[2] != 0 {
		t.Fatalf("mono = %#v", mono)
	}
	if _, err := DownmixInterleaved([]float32{1, 2, 3}, 2); err == nil {
		t.Fatal("misaligned input accepted")
	}
}

func TestResampleLinearMonoPreservesEndpointsAndCopiesEqualRate(t *testing.T) {
	input := []float32{0, 1, 0, -1}
	doubled, err := ResampleLinearMono(input, 4, 8)
	if err != nil {
		t.Fatal(err)
	}
	if len(doubled) != 8 || doubled[0] != 0 || doubled[2] != 1 || doubled[4] != 0 || doubled[7] != -1 {
		t.Fatalf("doubled = %#v", doubled)
	}
	copyAtRate, err := ResampleLinearMono(input, 4, 4)
	if err != nil || len(copyAtRate) != len(input) || &copyAtRate[0] == &input[0] {
		t.Fatalf("equal rate = %#v, %v", copyAtRate, err)
	}
}
