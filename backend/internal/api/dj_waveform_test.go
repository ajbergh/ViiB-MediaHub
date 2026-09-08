package api

import (
	"bytes"
	"encoding/binary"
	"errors"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ajbergh/viib-mediahub/internal/analysis"
)

// encodePCM16WAV builds a minimal mono RIFF/WAVE PCM16 file. The test owns this
// rather than importing the Phase 0 benchmark harness, so production test code
// does not depend on research tooling.
func encodePCM16WAV(samples []float32, sampleRate int) []byte {
	var body bytes.Buffer
	for _, sample := range samples {
		scaled := math.Round(float64(sample) * 32767)
		scaled = math.Max(-32768, math.Min(32767, scaled))
		_ = binary.Write(&body, binary.LittleEndian, int16(scaled))
	}
	data := body.Bytes()

	var out bytes.Buffer
	out.WriteString("RIFF")
	_ = binary.Write(&out, binary.LittleEndian, uint32(36+len(data)))
	out.WriteString("WAVE")
	out.WriteString("fmt ")
	_ = binary.Write(&out, binary.LittleEndian, uint32(16))
	_ = binary.Write(&out, binary.LittleEndian, uint16(1)) // PCM
	_ = binary.Write(&out, binary.LittleEndian, uint16(1)) // mono
	_ = binary.Write(&out, binary.LittleEndian, uint32(sampleRate))
	_ = binary.Write(&out, binary.LittleEndian, uint32(sampleRate*2)) // byte rate
	_ = binary.Write(&out, binary.LittleEndian, uint16(2))            // block align
	_ = binary.Write(&out, binary.LittleEndian, uint16(16))           // bits per sample
	out.WriteString("data")
	_ = binary.Write(&out, binary.LittleEndian, uint32(len(data)))
	out.Write(data)
	return out.Bytes()
}

// Formats with no pure-Go backend decoder must stay deferred to the renderer.
// Grouping them in one list keeps the capability claim honest: adding a decoder
// should require moving an entry out of this test, not silently widening it.
func TestGenerateWaveformDefersFormatsWithNoBackendDecoder(t *testing.T) {
	t.Parallel()

	for _, extension := range []string{".opus", ".flac", ".m4a", ".aac", ".wma", ".unknown"} {
		t.Run(extension, func(t *testing.T) {
			t.Parallel()
			_, err := generateWaveform(filepath.Join(t.TempDir(), "track"+extension))
			if !errors.Is(err, errClientWaveformRequired) {
				t.Fatalf("generateWaveform(%q) error = %v, want client generation marker", extension, err)
			}
		})
	}
}

// Opus rides in an Ogg container but is a different codec. The shared registry
// keys them separately, so Vorbis support must never imply Opus support.
func TestGenerateWaveformReportsOpusSeparatelyFromVorbis(t *testing.T) {
	t.Parallel()

	_, opusErr := generateWaveform(filepath.Join(t.TempDir(), "track.opus"))
	if !errors.Is(opusErr, errClientWaveformRequired) || !strings.Contains(opusErr.Error(), "opus format") {
		t.Fatalf("Opus error = %v, want distinct opus client-generation marker", opusErr)
	}
	// Ogg/Vorbis now has a backend decoder, so a missing file must surface as a
	// source error rather than as an unsupported-format deferral.
	_, vorbisErr := generateWaveform(filepath.Join(t.TempDir(), "missing.ogg"))
	if errors.Is(vorbisErr, errClientWaveformRequired) {
		t.Fatalf("Ogg/Vorbis was deferred to the browser despite a backend decoder: %v", vorbisErr)
	}
	if vorbisErr == nil || !strings.Contains(vorbisErr.Error(), "open local source") {
		t.Fatalf("Ogg/Vorbis error = %v, want source open failure", vorbisErr)
	}
}

// Extension matching must be case-insensitive; a capitalized MP3 is still an
// MP3 and must not fall back to the browser.
func TestGenerateWaveformNormalizesExtensionCase(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"missing.MP3", "missing.WAV", "missing.Ogg"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := generateWaveform(filepath.Join(t.TempDir(), name))
			if errors.Is(err, errClientWaveformRequired) {
				t.Fatalf("%s was incorrectly deferred to the browser: %v", name, err)
			}
			if err == nil || !strings.Contains(err.Error(), "open local source") {
				t.Fatalf("%s error = %v, want source open failure", name, err)
			}
		})
	}
}

// The point of the bridge: WAV now produces a real server-side waveform.
// Duration comes from decoded frames, and the peak count follows the declared
// resolution rather than the decoder's internal chunking.
func TestGenerateWaveformDecodesWAVServerSide(t *testing.T) {
	t.Parallel()

	const sampleRate = 22050
	const seconds = 2
	samples := make([]float32, sampleRate*seconds)
	for i := range samples {
		samples[i] = float32(math.Sin(2 * math.Pi * 440 * float64(i) / sampleRate))
	}

	path := filepath.Join(t.TempDir(), "tone.wav")
	if err := os.WriteFile(path, encodePCM16WAV(samples, sampleRate), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	waveform, err := generateWaveform(path)
	if err != nil {
		t.Fatalf("generateWaveform() error = %v", err)
	}
	if waveform.SampleRate != sampleRate {
		t.Fatalf("SampleRate = %d, want %d", waveform.SampleRate, sampleRate)
	}
	if waveform.Resolution != analysis.DefaultWaveformResolution {
		t.Fatalf("Resolution = %d, want %d", waveform.Resolution, analysis.DefaultWaveformResolution)
	}
	if math.Abs(waveform.Duration-seconds) > 0.01 {
		t.Fatalf("Duration = %f, want ~%d", waveform.Duration, seconds)
	}
	// Ceiling, not floor: the trailing partial window is flushed so the tail of
	// a track is never dropped from the overview.
	wantPeaks := (len(samples) + analysis.DefaultWaveformResolution - 1) / analysis.DefaultWaveformResolution
	if len(waveform.Peaks) != wantPeaks {
		t.Fatalf("len(Peaks) = %d, want %d", len(waveform.Peaks), wantPeaks)
	}
	// A full-scale sine must read near 1.0 in every window, and no peak may
	// exceed the normalized range.
	for i, peak := range waveform.Peaks {
		if peak < 0.9 || peak > 1.0 {
			t.Fatalf("Peaks[%d] = %f, want a normalized full-scale peak", i, peak)
		}
	}
}
