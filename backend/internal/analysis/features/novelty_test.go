package features

import (
	"math"
	"reflect"
	"testing"
)

func noveltySignal(seconds float64, signal func(float64) float64) []float32 {
	const rate = 8000
	samples := make([]float32, int(seconds*rate))
	for i := range samples {
		samples[i] = float32(signal(float64(i) / rate))
	}
	return samples
}

func analyzeNovelty(t *testing.T, samples []float32) Result {
	t.Helper()
	a := NewAccumulator(8000)
	for start := 0; start < len(samples); start += 8191 {
		end := start + 8191
		if end > len(samples) {
			end = len(samples)
		}
		a.Feed(samples[start:end])
	}
	r, err := a.Result()
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestNoveltyDetectsSustainedArrangementChanges(t *testing.T) {
	for _, tc := range []struct {
		name   string
		signal func(float64) float64
		kind   string
	}{
		{"energy return", func(s float64) float64 {
			amp := .12
			if s >= 13.35 {
				amp = .6
			}
			return amp * math.Sin(2*math.Pi*100*s)
		}, "energy-rise"},
		{"energy reduction", func(s float64) float64 {
			amp := .6
			if s >= 13.35 {
				amp = .12
			}
			return amp * math.Sin(2*math.Pi*100*s)
		}, "energy-fall"},
		{"equal loudness texture", func(s float64) float64 {
			freq := 100.0
			if s >= 13.35 {
				freq = 3000
			}
			return .4 * math.Sin(2*math.Pi*freq*s)
		}, "section"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := analyzeNovelty(t, noveltySignal(30, tc.signal))
			if len(r.Events) != 2 || r.Events[0].Kind != "start" || r.Events[1].Kind != tc.kind || math.Abs(r.Events[1].Position-13.35) > .15 {
				t.Fatalf("expected opening and change at 13.35s, got %#v", r.Events)
			}
		})
	}
}

func TestNoveltyRejectsSilenceAndIsolatedTransient(t *testing.T) {
	for _, tc := range []struct {
		name   string
		signal func(float64) float64
		count  int
	}{
		{"silence", func(s float64) float64 { return 0 }, 0},
		{"steady", func(s float64) float64 { return .2 * math.Sin(2*math.Pi*100*s) }, 1},
		{"single fill", func(s float64) float64 {
			amp := .2
			if s >= 10 && s < 10.1 {
				amp = .8
			}
			return amp * math.Sin(2*math.Pi*100*s)
		}, 1},
		{"isolated sound", func(s float64) float64 {
			if s >= 10 && s < 10.1 {
				return .8 * math.Sin(2*math.Pi*100*s)
			}
			return 0
		}, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := analyzeNovelty(t, noveltySignal(24, tc.signal))
			if len(r.Events) != tc.count {
				t.Fatalf("events=%#v, want %d", r.Events, tc.count)
			}
		})
	}
}

func TestNoveltyFindsOpeningAfterSilenceAndIgnoresTrailingSilence(t *testing.T) {
	r := analyzeNovelty(t, noveltySignal(30, func(s float64) float64 {
		if s < 3.25 || s >= 26 {
			return 0
		}
		return .3 * math.Sin(2*math.Pi*200*s)
	}))
	if len(r.Events) != 1 || math.Abs(r.Events[0].Position-3.25) > .05 || r.DurationSeconds != 30 {
		t.Fatalf("unexpected opening/end events: %#v", r)
	}
}

func TestNoveltyFindsSparsePercussiveOpening(t *testing.T) {
	r := analyzeNovelty(t, noveltySignal(24, func(s float64) float64 {
		if s < 3 {
			return 0
		}
		phase := math.Mod(s-3, .5)
		if phase >= .05 {
			return 0
		}
		return .4 * math.Sin(2*math.Pi*100*s)
	}))
	if len(r.Events) != 1 || math.Abs(r.Events[0].Position-3) > .05 {
		t.Fatalf("sparse rhythm was lost or made false sections: %#v", r.Events)
	}
}

func TestNoveltyFindsGradualChange(t *testing.T) {
	r := analyzeNovelty(t, noveltySignal(40, func(s float64) float64 {
		amp := .08 + .6*math.Max(0, math.Min(1, (s-12)/8))
		return amp * math.Sin(2*math.Pi*100*s)
	}))
	if len(r.Events) < 2 {
		t.Fatalf("gradual build was missed: %#v", r.Events)
	}
	for _, e := range r.Events[1:] {
		if e.Position < 12 || e.Position > 20 || e.Kind != "energy-rise" {
			t.Fatalf("event outside build: %#v", e)
		}
	}
}

func TestNoveltyChunkInvarianceAndArtifactRoundTrip(t *testing.T) {
	samples := noveltySignal(24.123, func(s float64) float64 {
		freq := 100.0
		if s > 11.2 {
			freq = 2400
		}
		return .3 * math.Sin(2*math.Pi*freq*s)
	})
	chunked := analyzeNovelty(t, samples)
	a := NewAccumulator(8000)
	a.Feed(samples)
	whole, err := a.Result()
	if err != nil || !reflect.DeepEqual(chunked, whole) {
		t.Fatalf("chunk-dependent analysis: %v", err)
	}
	data, err := chunked.Encode()
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeBounded(data, MaxStructureStatusArtifactBytes)
	if err != nil || !reflect.DeepEqual(decoded, chunked) {
		t.Fatalf("events/timing failed round trip: %#v, %v", decoded, err)
	}
}

// Five minutes of 44.1 kHz audio, fed in bounded chunks. The benchmark includes
// RMS and event extraction but no decoder, key, tempo, database or model runtime.
func BenchmarkNativeCueFeatures(b *testing.B) {
	const rate = 44100
	chunk := make([]float32, rate)
	for i := range chunk {
		chunk[i] = float32(.3 * math.Sin(2*math.Pi*100*float64(i)/rate))
	}
	b.ReportAllocs()
	b.SetBytes(300 * rate * 4)
	b.ResetTimer()
	for range b.N {
		a := NewAccumulator(rate)
		for second := 0; second < 300; second++ {
			a.Feed(chunk)
		}
		if _, err := a.Result(); err != nil {
			b.Fatal(err)
		}
	}
}
