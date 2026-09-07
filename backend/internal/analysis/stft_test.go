package analysis

import (
	"math"
	"testing"
)

func TestSTFTProducesOverlappingNonRedundantSpectra(t *testing.T) {
	stft, err := NewSTFT(8, 4)
	if err != nil {
		t.Fatal(err)
	}
	samples := make([]float32, 16)
	for i := range samples {
		samples[i] = float32(math.Sin(2 * math.Pi * float64(i) / 8))
	}
	count := 0
	if err := stft.Frames(samples, func(spectrum []complex128) error {
		count++
		if len(spectrum) != 5 {
			t.Fatalf("spectrum bins = %d", len(spectrum))
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if count != 3 {
		t.Fatalf("frames = %d, want 3", count)
	}
}

func TestSTFTRejectsInvalidGeometry(t *testing.T) {
	for _, geometry := range [][2]int{{0, 1}, {7, 1}, {8, 0}, {8, 9}} {
		if _, err := NewSTFT(geometry[0], geometry[1]); err == nil {
			t.Fatalf("accepted %#v", geometry)
		}
	}
}
