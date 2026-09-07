package analysisbench

import "testing"

func TestPrototypeKeyRecognizesAllSyntheticTriads(t *testing.T) {
	for tonic := 0; tonic < 12; tonic++ {
		for _, minor := range []bool{false, true} {
			fixture, err := NewTriad("triad", tonic, minor, 1, 22050, 1, 440)
			if err != nil {
				t.Fatal(err)
			}
			actual := EstimatePrototypeKey(fixture.Samples, fixture.SampleRate, fixture.Channels)
			if !actual.Known || actual.Key != fixture.Expected.Key {
				t.Fatalf("%s: EstimatePrototypeKey() = %+v, want %q", fixture.Expected.Key, actual, fixture.Expected.Key)
			}
		}
	}
}

func TestPrototypeKeyReturnsUnknownForSilence(t *testing.T) {
	fixture, err := NewSilence("silence", 1, 22050, 1)
	if err != nil {
		t.Fatal(err)
	}
	if actual := EstimatePrototypeKey(fixture.Samples, fixture.SampleRate, fixture.Channels); actual.Known {
		t.Fatalf("EstimatePrototypeKey(silence) = %+v, want unknown", actual)
	}
}

func BenchmarkPrototypeKeyTriad(b *testing.B) {
	fixture, err := NewTriad("benchmark", 0, false, 1, 22050, 1, 440)
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		if result := EstimatePrototypeKey(fixture.Samples, fixture.SampleRate, fixture.Channels); !result.Known {
			b.Fatal("prototype unexpectedly returned unknown")
		}
	}
}
