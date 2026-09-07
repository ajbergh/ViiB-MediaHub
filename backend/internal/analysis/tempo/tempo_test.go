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

func TestOnsetAccumulatorMatchesOneShotAcrossChunkBoundaries(t *testing.T) {
	fixture, err := analysisbench.NewClickTrack("chunked", 124, 8, 44100, 1)
	if err != nil {
		t.Fatal(err)
	}
	want := EstimatePCM(fixture.Samples, fixture.SampleRate)
	accumulator := NewOnsetAccumulator(fixture.SampleRate)
	for start := 0; start < len(fixture.Samples); {
		end := min(start+1733, len(fixture.Samples))
		accumulator.Feed(fixture.Samples[start:end])
		start = end
	}
	got := accumulator.Estimate()
	if !got.Known || math.Abs(got.BPM-want.BPM) > .001 || got.Confidence != want.Confidence {
		t.Fatalf("chunked = %#v, one-shot = %#v", got, want)
	}
}
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
