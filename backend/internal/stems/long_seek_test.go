package stems

import (
	"context"
	"errors"
	"math"
	"os"
	"path/filepath"
	"testing"
)

func longCodecArtifact(t testing.TB, format string) (string, Artifact) {
	t.Helper()
	path := filepath.Join("..", "..", "..", "fixtures", "stems", "codecs", "long-seek."+format)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	codec := format
	if format == "ogg" {
		codec = "vorbis"
	}
	return path, Artifact{Path: filepath.Base(path), Encoding: codec, SampleRate: 44100, Channels: 2, Frames: 700003, SizeBytes: info.Size()}
}

func TestCompressedLongSeekPreroll(t *testing.T) {
	for _, format := range []string{"mp3", "ogg"} {
		t.Run(format, func(t *testing.T) {
			path, a := longCodecArtifact(t, format)
			full, err := ReadPCMFrames(path, a, 0, a.Frames)
			if err != nil {
				t.Fatal(err)
			}
			for _, start := range []int64{649950, a.Frames - 128} {
				samples, err := ReadPCMFrames(path, a, start, 128)
				if err != nil {
					t.Fatal(err)
				}
				for i, v := range samples {
					if math.Abs(float64(v-full[int(start)*2+i])) > .003 {
						t.Fatalf("warm seek drift at %d value %d", start, i)
					}
				}
			}
			pos := 649950
			peak := float32(0)
			for i := 649950; i < 650050; i++ {
				if value := float32(math.Abs(float64(full[i*2]))); value > peak {
					peak = value
					pos = i
				}
			}
			if pos != 650000 || peak < .3 {
				t.Fatalf("late impulse at %d amplitude %f", pos, peak)
			}
		})
	}
}

func TestCompressedDecoderIOStopsOnCancellation(t *testing.T) {
	path, _ := longCodecArtifact(t, "mp3")
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	ctx, cancel := context.WithCancel(context.Background())
	reader := contextAudioFile{ctx, file}
	if _, err = reader.Read(make([]byte, 10)); err != nil {
		t.Fatal(err)
	}
	cancel()
	if _, err = reader.Read(make([]byte, 10)); !errors.Is(err, context.Canceled) {
		t.Fatalf("read error %v", err)
	}
	if _, err = reader.Seek(0, 0); !errors.Is(err, context.Canceled) {
		t.Fatalf("seek error %v", err)
	}
	if _, err = openCompressed(ctx, file, "mp3"); !errors.Is(err, context.Canceled) {
		t.Fatalf("decoder init error %v", err)
	}
}

func BenchmarkCompressedStemRange(b *testing.B) {
	for _, format := range []string{"mp3", "ogg"} {
		b.Run(format, func(b *testing.B) {
			path, a := longCodecArtifact(b, format)
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := ReadPCMFrames(path, a, 649950, 44100); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
