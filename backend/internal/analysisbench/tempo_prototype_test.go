package analysisbench

import (
	"math"
	"testing"
)

func TestPrototypeTempoRecognizesStableSyntheticCases(t *testing.T) {
	straight, straightErr := NewClickTrack("straight", 120, 8, 44100, 2)
	fractional, fractionalErr := NewClickTrack("fractional", 128.5, 8, 44100, 2)
	syncopated, syncopatedErr := NewSyncopatedClickTrack("syncopated", 128, 8, 44100, 2)
	missing, missingErr := NewMissingBeatClickTrack("missing", 124, 8, 44100, 2, 4)
	quietIntro, quietIntroErr := NewQuietIntroClickTrack("intro", 122, 8, 2, 44100, 2)
	noisy, noisyErr := NewNoisyClickTrack("noisy", 126, 8, 44100, 2, 0.06)
	cases := []struct {
		name    string
		fixture PCMFixture
		want    float64
	}{
		{name: "straight", fixture: mustClickFixture(t, straight, straightErr), want: 120},
		{name: "fractional", fixture: mustClickFixture(t, fractional, fractionalErr), want: 128.5},
		{name: "syncopated", fixture: mustClickFixture(t, syncopated, syncopatedErr), want: 128},
		{name: "missing", fixture: mustClickFixture(t, missing, missingErr), want: 124},
		{name: "quiet-intro", fixture: mustClickFixture(t, quietIntro, quietIntroErr), want: 122},
		{name: "noisy", fixture: mustClickFixture(t, noisy, noisyErr), want: 126},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			actual := EstimatePrototypeTempo(testCase.fixture.Samples, testCase.fixture.SampleRate, testCase.fixture.Channels)
			if !actual.Known || math.Abs(actual.BPM-testCase.want) > 0.5 {
				t.Fatalf("EstimatePrototypeTempo() = %+v, want %.1f BPM within 0.5", actual, testCase.want)
			}
		})
	}
}

func TestPrototypeTempoReturnsUnknownForSilence(t *testing.T) {
	fixture, err := NewSilence("silence", 2, 44100, 2)
	if err != nil {
		t.Fatalf("NewSilence() error = %v", err)
	}
	if actual := EstimatePrototypeTempo(fixture.Samples, fixture.SampleRate, fixture.Channels); actual.Known {
		t.Fatalf("EstimatePrototypeTempo(silence) = %+v, want unknown", actual)
	}
}

func BenchmarkPrototypeTempoClick128(b *testing.B) {
	fixture, err := NewClickTrack("benchmark", 128, 8, 44100, 2)
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		if result := EstimatePrototypeTempo(fixture.Samples, fixture.SampleRate, fixture.Channels); !result.Known {
			b.Fatal("prototype unexpectedly returned unknown")
		}
	}
}

func mustClickFixture(t *testing.T, fixture PCMFixture, err error) PCMFixture {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
	return fixture
}
