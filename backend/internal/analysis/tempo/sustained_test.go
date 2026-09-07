package tempo

import (
	"testing"

	"github.com/ajbergh/viib-mediahub/internal/analysisbench"
)

// Sustained audio has no beats. Returning a confident BPM for it would put a
// fabricated tempo on drones, pads, and spoken word, and would enable Sync
// against a value that was never measured from a transient.
func TestEstimatePCMRefusesTempoForSustainedAudio(t *testing.T) {
	sine, err := analysisbench.NewSineTone("sine", 440, 6, 22050, 1)
	if err != nil {
		t.Fatal(err)
	}
	triad, err := analysisbench.NewTriad("triad", 0, false, 6, 22050, 1, 440)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name    string
		fixture analysisbench.PCMFixture
	}{
		{name: "sustained sine tone", fixture: sine},
		{name: "sustained major triad", fixture: triad},
	} {
		estimate := EstimatePCM(test.fixture.Samples, test.fixture.SampleRate)
		if estimate.Known {
			t.Errorf("%s: estimate = %#v, want unknown", test.name, estimate)
		}
		if estimate.BPM != 0 || estimate.Confidence != 0 {
			t.Errorf("%s: unknown estimate must not carry values, got %#v", test.name, estimate)
		}
	}
}

// A monotonically decaying source never rises, so every frame difference is
// zero. Peak picking against a zero threshold would otherwise mark every frame
// as an onset and emit an arbitrary tempo.
func TestEstimatePCMRefusesTempoForMonotonicDecay(t *testing.T) {
	sampleRate := 22050
	samples := make([]float32, sampleRate*4)
	for i := range samples {
		samples[i] = float32(1.0 - float64(i)/float64(len(samples)))
	}
	if estimate := EstimatePCM(samples, sampleRate); estimate.Known {
		t.Fatalf("estimate = %#v, want unknown for a monotonic decay", estimate)
	}
}

// Percussive fixtures must keep passing the transient gate; the gate exists to
// reject sustained audio, not to suppress genuine beats.
func TestEstimatePCMStillMeasuresPercussiveFixtures(t *testing.T) {
	noisy, err := analysisbench.NewNoisyClickTrack("noisy", 128, 12, 22050, 1, 0.2)
	if err != nil {
		t.Fatal(err)
	}
	quiet, err := analysisbench.NewQuietIntroClickTrack("quiet", 128, 12, 4, 22050, 1)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name    string
		fixture analysisbench.PCMFixture
	}{
		{name: "noisy click track", fixture: noisy},
		{name: "quiet-intro click track", fixture: quiet},
	} {
		estimate := EstimatePCM(test.fixture.Samples, test.fixture.SampleRate)
		if !estimate.Known {
			t.Errorf("%s: estimate = %#v, want a measured tempo", test.name, estimate)
		}
	}
}
