package waveformartifact

import (
	"math"
	"testing"
)

func TestWaveformArtifactBoundsAndRoundTrip(t *testing.T) {
	o := Overview{SampleRate: 44100, Resolution: 256, Frames: 300, Peaks: []float64{0, .5}}
	raw, err := Encode(o)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Decode(raw)
	if err != nil || got.Frames != 300 || got.Peaks[1] != .5 || got.Duration() != o.Duration() {
		t.Fatalf("round trip: %+v %v", got, err)
	}
	for _, value := range []float64{-1, math.NaN(), math.Inf(1)} {
		invalid := o
		invalid.Peaks = []float64{0, value}
		if _, err := Encode(invalid); err == nil {
			t.Fatal("invalid peak accepted")
		}
	}
	if _, err := Decode(raw[:len(raw)-1]); err == nil {
		t.Fatal("truncated peaks accepted")
	}
	o.Frames = 100
	if _, err := Encode(o); err == nil {
		t.Fatal("inconsistent frame count accepted")
	}
}
