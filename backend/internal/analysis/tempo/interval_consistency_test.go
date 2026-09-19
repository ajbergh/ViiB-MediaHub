package tempo

import (
	"math"
	"testing"

	"github.com/ajbergh/viib-mediahub/internal/analysisbench"
)

func TestBeatIntervalConsistencyMeasuresSteadyAndMissingBeatFixtures(t *testing.T) {
	for _, create := range []func() (analysisbench.PCMFixture, error){
		func() (analysisbench.PCMFixture, error) {
			return analysisbench.NewClickTrack("steady", 128, 12, 44100, 1)
		},
		func() (analysisbench.PCMFixture, error) {
			return analysisbench.NewMissingBeatClickTrack("missing", 124, 12, 44100, 1, 4)
		},
	} {
		fixture, err := create()
		if err != nil {
			t.Fatal(err)
		}
		options := DefaultOptions()
		options.Method = MethodBeatIntervalConsistency
		got := EstimatePCMWithOptions(fixture.Samples, fixture.SampleRate, options)
		if !got.Known || got.BPM == 0 || math.Abs(got.BPM-*fixture.Expected.BPM) > .5 {
			t.Fatalf("%s = %#v, want BPM %.1f", fixture.Name, got, *fixture.Expected.BPM)
		}
	}
}

func TestBeatIntervalConsistencyRefusesSilence(t *testing.T) {
	options := DefaultOptions()
	options.Method = MethodBeatIntervalConsistency
	if got := EstimatePCMWithOptions(make([]float32, 44100*8), 44100, options); got.Known {
		t.Fatalf("silence = %#v, want refusal", got)
	}
}

func TestBeatIntervalConsistencyPreservesGeneratedTempoExpectations(t *testing.T) {
	fixtures, err := analysisbench.Phase0SyntheticFixtures()
	if err != nil {
		t.Fatal(err)
	}
	options := DefaultOptions()
	options.Method = MethodBeatIntervalConsistency
	for _, fixture := range fixtures {
		got := EstimatePCMWithOptions(fixture.Samples, fixture.SampleRate, options)
		if fixture.Expected.BPM != nil && (!got.Known || math.Abs(got.BPM-*fixture.Expected.BPM) > .5) {
			t.Fatalf("%s = %#v, want BPM %.1f", fixture.Name, got, *fixture.Expected.BPM)
		}
		if fixture.Expected.IsUnknown && got.Known {
			t.Fatalf("%s = %#v, want tempo refusal", fixture.Name, got)
		}
	}
}
