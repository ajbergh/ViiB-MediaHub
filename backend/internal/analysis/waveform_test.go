package analysis

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"testing"
)

// Peak windows must be a property of the resolution, not of how the decoder
// chunked its output. Without carrying window state across Feed calls, a
// waveform's time axis would depend on codec frame sizes.
func TestPeakAccumulatorIsIndependentOfChunkBoundaries(t *testing.T) {
	t.Parallel()

	samples := make([]float32, 4000)
	for i := range samples {
		samples[i] = float32(math.Sin(float64(i) / 7))
	}

	oneShot := NewPeakAccumulator(256)
	oneShot.Feed(samples)
	want := oneShot.Overview(44100)

	for _, chunk := range []int{1, 7, 100, 256, 257, 1024, 4000} {
		chunked := NewPeakAccumulator(256)
		for start := 0; start < len(samples); start += chunk {
			end := min(start+chunk, len(samples))
			chunked.Feed(samples[start:end])
		}
		got := chunked.Overview(44100)
		if got.Frames != want.Frames {
			t.Fatalf("chunk %d: Frames = %d, want %d", chunk, got.Frames, want.Frames)
		}
		if len(got.Peaks) != len(want.Peaks) {
			t.Fatalf("chunk %d: len(Peaks) = %d, want %d", chunk, len(got.Peaks), len(want.Peaks))
		}
		for i := range want.Peaks {
			if got.Peaks[i] != want.Peaks[i] {
				t.Fatalf("chunk %d: Peaks[%d] = %v, want %v", chunk, i, got.Peaks[i], want.Peaks[i])
			}
		}
	}
}

// A track shorter than one resolution window is still a real track.
func TestPeakAccumulatorFlushesShortTrailingWindow(t *testing.T) {
	t.Parallel()

	accumulator := NewPeakAccumulator(256)
	accumulator.Feed([]float32{0, 0.5, -0.75})
	overview := accumulator.Overview(48000)

	if len(overview.Peaks) != 1 || overview.Peaks[0] != 0.75 {
		t.Fatalf("Peaks = %v, want a single 0.75 peak", overview.Peaks)
	}
	if overview.Frames != 3 {
		t.Fatalf("Frames = %d, want 3", overview.Frames)
	}
	if math.Abs(overview.Duration()-3.0/48000) > 1e-9 {
		t.Fatalf("Duration = %v, want %v", overview.Duration(), 3.0/48000)
	}
}

// An empty accumulator must report no peaks rather than a fabricated silent one.
func TestPeakAccumulatorReportsNoPeaksForNoAudio(t *testing.T) {
	t.Parallel()

	overview := NewPeakAccumulator(256).Overview(44100)
	if len(overview.Peaks) != 0 || overview.Frames != 0 || overview.Duration() != 0 {
		t.Fatalf("overview = %#v, want empty", overview)
	}
}

// Duration must come from decoded frames, so a container header that overstates
// length cannot stretch the waveform's time axis.
func TestGenerateWaveformOverviewDerivesDurationFromDecodedFrames(t *testing.T) {
	t.Parallel()

	const sampleRate = 8000
	samples := make([]float32, sampleRate) // exactly one second
	for i := range samples {
		samples[i] = 1
	}
	path := filepath.Join(t.TempDir(), "tone.wav")
	if err := os.WriteFile(path, makeFloat32WAV(t, 1, sampleRate, samples), 0o600); err != nil {
		t.Fatal(err)
	}

	overview, err := GenerateWaveformOverview(context.Background(), NewDefaultDecoderRegistry(), path, DefaultWaveformResolution)
	if err != nil {
		t.Fatalf("GenerateWaveformOverview() error = %v", err)
	}
	if overview.Frames != int64(sampleRate) {
		t.Fatalf("Frames = %d, want %d", overview.Frames, sampleRate)
	}
	if math.Abs(overview.Duration()-1) > 1e-9 {
		t.Fatalf("Duration = %v, want 1", overview.Duration())
	}
}

// An unregistered codec must be reported as a capability gap before the file is
// opened, so a missing file and an unsupported format stay distinguishable.
func TestGenerateWaveformOverviewReportsUnsupportedCodecWithoutOpening(t *testing.T) {
	t.Parallel()

	_, err := GenerateWaveformOverview(context.Background(), NewDefaultDecoderRegistry(), filepath.Join(t.TempDir(), "absent.flac"), 0)
	if err != ErrUnsupportedCodec {
		t.Fatalf("error = %v, want ErrUnsupportedCodec", err)
	}
}

func TestDecoderRegistrySupports(t *testing.T) {
	t.Parallel()

	registry := NewDefaultDecoderRegistry()
	for _, name := range []string{"a.wav", "a.WAVE", "a.mp3", "a.MP3", "a.ogg", "a.oga"} {
		if !registry.Supports(name) {
			t.Fatalf("Supports(%q) = false, want true", name)
		}
	}
	// Opus shares the Ogg container but not the codec; it must stay unsupported
	// until a real Opus decoder lands.
	for _, name := range []string{"a.opus", "a.flac", "a.m4a", "a.aac", "a.wma", "a", "a.txt"} {
		if registry.Supports(name) {
			t.Fatalf("Supports(%q) = true, want false", name)
		}
	}
}
