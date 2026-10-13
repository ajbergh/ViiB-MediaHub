package tempo

import (
	"fmt"
	"math"
	"testing"

	"github.com/ajbergh/viib-mediahub/internal/analysisbench"
)

func TestPhaseSupportWideRangeClicks(t *testing.T) {
	for _, bpm := range []float64{60, 68, 80, 85, 128, 180, 187, 200} {
		t.Run(fmt.Sprint(bpm), func(t *testing.T) {
			fixture, err := analysisbench.NewClickTrack("wide", bpm, 16, 22050, 1)
			if err != nil {
				t.Fatal(err)
			}
			options := DefaultOptions()
			options.Method, options.Range = MethodPhaseSupport, RangeCustom
			options.MinBPM, options.MaxBPM = 40, 240
			got := EstimatePCMWithOptions(fixture.Samples, fixture.SampleRate, options)
			if !got.Known || math.Abs(got.BPM-bpm) > .5 {
				t.Fatalf("target %v estimate %+v", bpm, got)
			}
		})
	}
}

func TestPhaseSupportExplainsFundamentalAndRequiresCycles(t *testing.T) {
	values := make([]float64, 1600)
	peaks := []int{}
	for i := 50; i < len(values); i += 50 {
		values[i] = 1
		peaks = append(peaks, i)
	}
	fundamental := phaseOnsetSupport(values, peaks, 100, 120)
	for _, alias := range []float64{40, 60, 240} {
		if got := phaseOnsetSupport(values, peaks, 100, alias); got >= fundamental {
			t.Errorf("alias %v support %v >= fundamental %v", alias, got, fundamental)
		}
	}
	if got := phaseOnsetSupport(values, peaks[:3], 100, 120); got != 0 {
		t.Fatalf("three observations became tempo evidence: %v", got)
	}
}

func TestPhaseSupportRhythmsAndRefusals(t *testing.T) {
	constructors := []func() (analysisbench.PCMFixture, error){
		func() (analysisbench.PCMFixture, error) {
			return analysisbench.NewSyncopatedClickTrack("syncopated", 128, 16, 22050, 1)
		},
		func() (analysisbench.PCMFixture, error) {
			return analysisbench.NewMissingBeatClickTrack("missing", 124, 16, 22050, 1, 4)
		},
		func() (analysisbench.PCMFixture, error) {
			return analysisbench.NewMeteredClickTrack("three", 128, 16, 22050, 1, 3)
		},
		func() (analysisbench.PCMFixture, error) {
			return analysisbench.NewMeteredClickTrack("six", 128, 16, 22050, 1, 6)
		},
		func() (analysisbench.PCMFixture, error) {
			return analysisbench.NewNoisyClickTrack("noisy", 128, 16, 22050, 1, .2)
		},
		func() (analysisbench.PCMFixture, error) {
			return analysisbench.NewSineTone("sustained", 440, 8, 22050, 1)
		},
		func() (analysisbench.PCMFixture, error) { return analysisbench.NewSilence("silence", 8, 22050, 1) },
	}
	options := DefaultOptions()
	options.Method, options.Range = MethodPhaseSupport, RangeCustom
	options.MinBPM, options.MaxBPM = 40, 240
	for _, create := range constructors {
		fixture, err := create()
		if err != nil {
			t.Fatal(err)
		}
		t.Run(fixture.Name, func(t *testing.T) {
			got := EstimatePCMWithOptions(fixture.Samples, fixture.SampleRate, options)
			if fixture.Expected.BPM != nil && (!got.Known || math.Abs(got.BPM-*fixture.Expected.BPM) > .5) {
				t.Fatalf("estimate %+v want %v", got, *fixture.Expected.BPM)
			}
			if fixture.Expected.BPM == nil && got.Known {
				t.Fatalf("non-rhythmic estimate %+v", got)
			}
		})
	}
}

func TestPhaseSupportRefusesShortNoisyExcerpts(t *testing.T) {
	options := DefaultOptions()
	options.Method, options.Range = MethodPhaseSupport, RangeCustom
	options.MinBPM, options.MaxBPM = 40, 240
	for _, bpm := range []float64{96, 110, 200} {
		fixture, err := analysisbench.NewNoisyClickTrack("short", bpm, 1, 22050, 1, .2)
		if err != nil {
			t.Fatal(err)
		}
		if got := EstimatePCMWithOptions(fixture.Samples, fixture.SampleRate, options); got.Known {
			t.Errorf("one-second target %v produced %+v", bpm, got)
		}
	}
}
