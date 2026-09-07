package tempo

import (
	"github.com/ajbergh/viib-mediahub/internal/analysisbench"
	"math"
	"testing"
)

func TestEstimatePCMRecognizesFractionalSyntheticTempo(t *testing.T) {
	fixture, err := analysisbench.NewClickTrack("fractional", 128.5, 8, 44100, 1)
	if err != nil {
		t.Fatal(err)
	}
	actual := EstimatePCM(fixture.Samples, fixture.SampleRate)
	if !actual.Known || math.Abs(actual.BPM-128.5) > .5 || actual.AlgorithmVersion != AlgorithmVersion {
		t.Fatalf("estimate = %#v", actual)
	}
}
func TestEstimatePCMReturnsUnknownForSilence(t *testing.T) {
	if actual := EstimatePCM(make([]float32, 44100), 44100); actual.Known {
		t.Fatalf("silence = %#v", actual)
	}
}
