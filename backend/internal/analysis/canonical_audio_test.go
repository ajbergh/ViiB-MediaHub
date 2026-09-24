package analysis

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"io"
	"math"
	"os"
	"path/filepath"
	"testing"
)

func TestSourceAudioHashIgnoresWAVRetagging(t *testing.T) {
	root := t.TempDir()
	samples := []int16{-32768, 0, 32767, 16384, -8192, 4096}
	untagged := makePCM16WAV(t, 2, 44100, samples)
	tagged := wavWithListTag(untagged, "different embedded metadata")
	firstPath := filepath.Join(root, "first.wav")
	secondPath := filepath.Join(root, "second.wav")
	if err := os.WriteFile(firstPath, untagged, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(secondPath, tagged, 0600); err != nil {
		t.Fatal(err)
	}
	cache := NewSourceAudioHashCache()
	registry := NewDefaultDecoderRegistry()
	first, err := cache.SHA256(context.Background(), registry, ResolvedSource{Name: "first.wav", Path: firstPath}, 44100, 2)
	if err != nil {
		t.Fatal(err)
	}
	second, err := cache.SHA256(context.Background(), registry, ResolvedSource{Name: "second.wav", Path: secondPath}, 44100, 2)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("retagged source audio hash differs: %s != %s", first, second)
	}
	if first != canonicalPCM16Hash(samples) {
		t.Fatalf("hash = %s, want canonical int32LE hash %s", first, canonicalPCM16Hash(samples))
	}
}

func TestSourceAudioHashDetectsAudioMismatchAndRejectsGeometryConversion(t *testing.T) {
	root := t.TempDir()
	firstPath := filepath.Join(root, "first.wav")
	secondPath := filepath.Join(root, "different.wav")
	if err := os.WriteFile(firstPath, makePCM16WAV(t, 2, 44100, []int16{1000, -1000, 2000, -2000}), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(secondPath, makePCM16WAV(t, 2, 44100, []int16{1000, -1000, 2001, -2000}), 0600); err != nil {
		t.Fatal(err)
	}
	cache := NewSourceAudioHashCache()
	registry := NewDefaultDecoderRegistry()
	first, err := cache.SHA256(context.Background(), registry, ResolvedSource{Name: "first.wav", Path: firstPath}, 44100, 2)
	if err != nil {
		t.Fatal(err)
	}
	second, err := cache.SHA256(context.Background(), registry, ResolvedSource{Name: "different.wav", Path: secondPath}, 44100, 2)
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("audio hash matched after a source PCM sample changed")
	}
	if _, err := cache.SHA256(context.Background(), registry, ResolvedSource{Name: "first.wav", Path: firstPath}, 48000, 2); err != ErrCanonicalAudioGeometryUnsupported {
		t.Fatalf("sample-rate conversion error = %v, want unsupported geometry", err)
	}
	if _, err := cache.SHA256(context.Background(), registry, ResolvedSource{Name: "first.wav", Path: firstPath}, 44100, 1); err != ErrCanonicalAudioGeometryUnsupported {
		t.Fatalf("channel conversion error = %v, want unsupported geometry", err)
	}
}

func TestCanonicalSampleInt32LittleEndianRules(t *testing.T) {
	registry := NewDecoderRegistry()
	if err := registry.Register([]string{"wav"}, canonicalRuleDecoder{}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "rules.wav")
	if err := os.WriteFile(path, []byte("decoder fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	hash, err := NewSourceAudioHashCache().SHA256(context.Background(), registry, ResolvedSource{Name: "rules.wav", Path: path}, 44100, 2)
	if err != nil {
		t.Fatal(err)
	}
	want := canonicalHash([]int32{math.MinInt32 + 1, -1073741824, 1073741824, math.MaxInt32})
	if hash != want {
		t.Fatalf("hash = %s, want %s", hash, want)
	}
}

type canonicalRuleDecoder struct{}

func (canonicalRuleDecoder) ID() string { return "canonical-rule-test" }
func (canonicalRuleDecoder) Open(_ context.Context, source io.ReadCloser) (PCMStream, error) {
	return &canonicalRuleStream{source: source}, nil
}

type canonicalRuleStream struct {
	source io.ReadCloser
	done   bool
}

func (*canonicalRuleStream) Info() PCMInfo {
	return PCMInfo{SampleRate: 44100, Channels: 2, DeclaredFrames: 2}
}
func (s *canonicalRuleStream) Read(_ context.Context, out []float32) (int, error) {
	if s.done {
		return 0, io.EOF
	}
	s.done = true
	// Half-scale products are exact half-integers because the multiplier is
	// odd; endpoint values verify clamping before signed int32 encoding.
	return copy(out, []float32{-2, -.5, .5, 2}), nil
}
func (s *canonicalRuleStream) Close() error { return s.source.Close() }

func canonicalPCM16Hash(samples []int16) string {
	values := make([]int32, len(samples))
	for index, sample := range samples {
		value := float64(float32(sample) / 32768)
		values[index] = int32(math.Round(math.Max(-1, math.Min(1, value)) * 2147483647))
	}
	return canonicalHash(values)
}

func canonicalHash(values []int32) string {
	data := make([]byte, len(values)*4)
	for index, value := range values {
		binary.LittleEndian.PutUint32(data[index*4:], uint32(value))
	}
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

func wavWithListTag(wav []byte, tag string) []byte {
	metadata := make([]byte, 8+len(tag)+len(tag)%2)
	copy(metadata[:4], "LIST")
	binary.LittleEndian.PutUint32(metadata[4:8], uint32(len(tag)))
	copy(metadata[8:], tag)
	result := make([]byte, 0, len(wav)+len(metadata))
	result = append(result, wav[:36]...)
	result = append(result, metadata...)
	result = append(result, wav[36:]...)
	binary.LittleEndian.PutUint32(result[4:8], uint32(len(result)-8))
	return result
}
