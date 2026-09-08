package key

import (
	"math"
	"math/rand"
	"testing"

	"github.com/ajbergh/viib-mediahub/internal/analysisbench"
)

func TestNotationMappingsAll24Keys(t *testing.T) {
	expectedCamelotMajor := []string{"8B", "3B", "10B", "5B", "12B", "7B", "2B", "9B", "4B", "11B", "6B", "1B"}
	expectedCamelotMinor := []string{"5A", "12A", "7A", "2A", "9A", "4A", "11A", "6A", "1A", "8A", "3A", "10A"}

	expectedOpenMajor := []string{"1d", "8d", "3d", "10d", "5d", "12d", "7d", "2d", "9d", "4d", "11d", "6d"}
	expectedOpenMinor := []string{"10m", "5m", "12m", "7m", "2m", "9m", "4m", "11m", "6m", "1m", "8m", "3m"}

	for tonic := 0; tonic < 12; tonic++ {
		camMaj := Camelot(tonic, ModeMajor)
		if camMaj != expectedCamelotMajor[tonic] {
			t.Errorf("Camelot(%d, major) = %s, want %s", tonic, camMaj, expectedCamelotMajor[tonic])
		}
		camMin := Camelot(tonic, ModeMinor)
		if camMin != expectedCamelotMinor[tonic] {
			t.Errorf("Camelot(%d, minor) = %s, want %s", tonic, camMin, expectedCamelotMinor[tonic])
		}

		openMaj := OpenKey(tonic, ModeMajor)
		if openMaj != expectedOpenMajor[tonic] {
			t.Errorf("OpenKey(%d, major) = %s, want %s", tonic, openMaj, expectedOpenMajor[tonic])
		}
		openMin := OpenKey(tonic, ModeMinor)
		if openMin != expectedOpenMinor[tonic] {
			t.Errorf("OpenKey(%d, minor) = %s, want %s", tonic, openMin, expectedOpenMinor[tonic])
		}
	}
}

func TestHarmonicRelations(t *testing.T) {
	cases := []struct {
		keyA, keyB string
		wantRel    string
		wantComp   bool
	}{
		{"8A", "8A", "same", true},
		{"8A", "9A", "adjacent", true},
		{"8A", "7A", "adjacent", true},
		{"1A", "12A", "adjacent", true},
		{"12A", "1A", "adjacent", true},
		{"8A", "8B", "relative", true},
		{"8B", "8A", "relative", true},
		{"8A", "10A", "other", false},
		{"8A", "2B", "other", false},
		{"", "8A", "unknown", false},
		{"N/A", "8A", "unknown", false},
	}
	for _, tc := range cases {
		rel, comp := HarmonicRelation(tc.keyA, tc.keyB)
		if rel != tc.wantRel || comp != tc.wantComp {
			t.Errorf("HarmonicRelation(%q, %q) = (%q, %v), want (%q, %v)", tc.keyA, tc.keyB, rel, comp, tc.wantRel, tc.wantComp)
		}
	}
}

func TestEstimatePCMRecognizesAllSyntheticTriads(t *testing.T) {
	for tonic := 0; tonic < 12; tonic++ {
		for _, minor := range []bool{false, true} {
			fixture, err := analysisbench.NewTriad("triad", tonic, minor, 1.5, 22050, 1, 440)
			if err != nil {
				t.Fatal(err)
			}
			actual := EstimatePCM(fixture.Samples, fixture.SampleRate)
			if !actual.Known {
				t.Fatalf("%s: expected known key estimate", fixture.Expected.Key)
			}
			expectedMode := ModeMajor
			if minor {
				expectedMode = ModeMinor
			}
			if actual.Tonic != tonic || actual.Mode != expectedMode {
				t.Fatalf("%s: EstimatePCM() = tonic %d mode %s, want tonic %d mode %s (actual key=%q)",
					fixture.Expected.Key, actual.Tonic, actual.Mode, tonic, expectedMode, actual.Key)
			}
			if actual.Confidence <= 0 {
				t.Fatalf("%s: expected positive confidence, got %v", fixture.Expected.Key, actual.Confidence)
			}
			if actual.Camelot == "" || actual.OpenKey == "" {
				t.Fatalf("%s: expected Camelot and OpenKey notations populated", fixture.Expected.Key)
			}
		}
	}
}

func TestEstimatePCMReturnsUnknownForSilence(t *testing.T) {
	actual := EstimatePCM(make([]float32, 22050), 22050)
	if actual.Known {
		t.Fatalf("EstimatePCM(silence) = %+v, want unknown", actual)
	}
}

func TestChromaAccumulatorMatchesOneShotAcrossChunks(t *testing.T) {
	fixture, err := analysisbench.NewTriad("chunked-c-maj", 0, false, 2.0, 22050, 1, 440)
	if err != nil {
		t.Fatal(err)
	}
	oneShot := EstimatePCM(fixture.Samples, fixture.SampleRate)

	acc := NewChromaAccumulator(fixture.SampleRate)
	chunkSize := 512
	for start := 0; start < len(fixture.Samples); start += chunkSize {
		end := start + chunkSize
		if end > len(fixture.Samples) {
			end = len(fixture.Samples)
		}
		acc.Feed(fixture.Samples[start:end])
	}
	chunked := acc.Estimate()

	if !chunked.Known || chunked.Tonic != oneShot.Tonic || chunked.Mode != oneShot.Mode {
		t.Fatalf("chunked = %+v, want oneShot = %+v", chunked, oneShot)
	}
	if math.Abs(chunked.Confidence-oneShot.Confidence) > 1e-4 {
		t.Fatalf("confidence delta too large: chunked=%v oneShot=%v", chunked.Confidence, oneShot.Confidence)
	}
}

func BenchmarkEstimateKeyTriad(b *testing.B) {
	fixture, err := analysisbench.NewTriad("benchmark", 0, false, 1.5, 22050, 1, 440)
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		if result := EstimatePCM(fixture.Samples, fixture.SampleRate); !result.Known {
			b.Fatal("key unexpectedly returned unknown")
		}
	}
}

// Refusing to answer is a feature. Broadband material has no tonal centre, so
// the 24-profile correlation is ranking noise; whichever key "wins" is an
// artifact. Before the tonality gate, white noise reported A minor at
// confidence 0.377 — higher than every correctly identified triad below.
func TestEstimateRefusesAtonalMaterial(t *testing.T) {
	const sampleRate = 22050

	clicks, err := analysisbench.NewClickTrack("clicks", 128, 12, sampleRate, 1)
	if err != nil {
		t.Fatal(err)
	}
	rng := rand.New(rand.NewSource(7))
	noise := make([]float32, sampleRate*6)
	for i := range noise {
		noise[i] = float32(rng.Float64()*2 - 1)
	}

	for _, test := range []struct {
		name    string
		samples []float32
	}{
		{"percussive clicks", clicks.Samples},
		{"white noise", noise},
	} {
		t.Run(test.name, func(t *testing.T) {
			estimate := EstimatePCM(test.samples, sampleRate)
			if estimate.Known {
				t.Fatalf("reported %q at confidence %.3f for atonal material; want unknown", estimate.Key, estimate.Confidence)
			}
			if estimate.Confidence != 0 {
				t.Fatalf("confidence = %v, want 0 when no key is claimed", estimate.Confidence)
			}
			if estimate.Camelot != "" || estimate.OpenKey != "" {
				t.Fatalf("refused estimate must not carry notations, got %q/%q", estimate.Camelot, estimate.OpenKey)
			}
			// The diagnostic is retained on refusal so Phase 0 calibration can
			// see how far a rejected track sat from the bound.
			if estimate.Flatness <= maxTonalChromaFlatness {
				t.Fatalf("flatness = %v, want > %v", estimate.Flatness, maxTonalChromaFlatness)
			}
		})
	}
}

// The gate must not cost real detections: every synthetic triad has to stay
// comfortably inside the tonal side of the bound.
func TestTonalMaterialStaysWellInsideTheTonalityBound(t *testing.T) {
	const sampleRate = 22050
	worst := 0.0
	for tonic := 0; tonic < 12; tonic++ {
		for _, minor := range []bool{false, true} {
			fixture, err := analysisbench.NewTriad("t", tonic, minor, 4, sampleRate, 1, 440)
			if err != nil {
				t.Fatal(err)
			}
			estimate := EstimatePCM(fixture.Samples, sampleRate)
			if !estimate.Known {
				t.Fatalf("tonic %d minor=%v was refused at flatness %v", tonic, minor, estimate.Flatness)
			}
			worst = math.Max(worst, estimate.Flatness)
		}
	}
	// Keep a wide margin so a small profile or geometry change cannot silently
	// push real triads over the bound.
	if worst > maxTonalChromaFlatness/2 {
		t.Fatalf("worst tonal flatness %v is uncomfortably close to the %v bound", worst, maxTonalChromaFlatness)
	}
}
