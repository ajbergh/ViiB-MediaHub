package analysis

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestWriteCanonicalWAVDeterministicPCMAndMetadata(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "stereo.wav")
	input := makePCM16WAV(t, 2, 44100, []int16{16384, 16384, -16384, -16384, 0, 0})
	if err := os.WriteFile(source, input, 0o600); err != nil {
		t.Fatal(err)
	}
	firstPath, secondPath := filepath.Join(dir, "first.wav"), filepath.Join(dir, "second.wav")
	first, err := WriteCanonicalWAV(context.Background(), NewDefaultDecoderRegistry(), source, firstPath)
	if err != nil {
		t.Fatal(err)
	}
	_, err = WriteCanonicalWAV(context.Background(), NewDefaultDecoderRegistry(), source, secondPath)
	if err != nil {
		t.Fatal(err)
	}
	firstBytes, err := os.ReadFile(firstPath)
	if err != nil {
		t.Fatal(err)
	}
	secondBytes, err := os.ReadFile(secondPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(firstBytes) != string(secondBytes) {
		t.Fatal("canonical WAV bytes changed between runs")
	}
	if len(firstBytes) != 50 || firstBytes[44] != 0 || firstBytes[45] != 64 || firstBytes[46] != 0 || firstBytes[47] != 192 || firstBytes[48] != 0 || firstBytes[49] != 0 {
		t.Fatalf("unexpected canonical WAV data: %v", firstBytes[44:])
	}
	pcmHash := sha256.Sum256(firstBytes[44:])
	meta := first.CanonicalAudio
	if meta.Decoder != "ViiB" || meta.Encoding != "pcm_s16le" || meta.Channels != 1 || meta.SampleRate != 44100 || meta.Frames != 3 || meta.DurationSeconds != 3.0/44100 || meta.DecoderVersion != "wav-pcm16-v1" {
		t.Fatalf("canonical metadata = %#v", meta)
	}
	wantHash := hex.EncodeToString(pcmHash[:])
	if first.PCM_SHA256 != wantHash || meta.SourceAudioSHA256 != wantHash {
		t.Fatalf("hashes = result %s, source %s; want %s", first.PCM_SHA256, meta.SourceAudioSHA256, wantHash)
	}
	if meta.TimelineOriginSeconds != 0 || meta.ResampleDelaySamples != 0 || meta.StartTrimSamples != 0 {
		t.Fatalf("unexpected timeline remapping metadata: %#v", meta)
	}
}

func TestWriteCanonicalWAVReportsUnsupportedAndDecodeErrors(t *testing.T) {
	dir := t.TempDir()
	unsupportedOutput := filepath.Join(dir, "unsupported.wav")
	if _, err := WriteCanonicalWAV(context.Background(), NewDefaultDecoderRegistry(), "source.flac", unsupportedOutput); err != ErrUnsupportedCodec {
		t.Fatalf("unsupported codec error = %v", err)
	}
	if _, err := os.Stat(unsupportedOutput); !os.IsNotExist(err) {
		t.Fatalf("unsupported source created output: stat error = %v", err)
	}
	badSource := filepath.Join(dir, "bad.wav")
	if err := os.WriteFile(badSource, []byte("not wave"), 0o600); err != nil {
		t.Fatal(err)
	}
	badOutput := filepath.Join(dir, "bad-output.wav")
	if _, err := WriteCanonicalWAV(context.Background(), NewDefaultDecoderRegistry(), badSource, badOutput); err == nil {
		t.Fatal("malformed WAV unexpectedly decoded")
	}
	if _, err := os.Stat(badOutput); !os.IsNotExist(err) {
		t.Fatalf("failed decode left partial output: stat error = %v", err)
	}
}

func TestWriteCanonicalWAVRefusesToOverwriteExistingOutput(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source.wav")
	if err := os.WriteFile(source, makePCM16WAV(t, 1, 8000, []int16{123}), 0o600); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(dir, "existing.wav")
	original := []byte("keep this file")
	if err := os.WriteFile(output, original, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := WriteCanonicalWAV(context.Background(), NewDefaultDecoderRegistry(), source, output); !errors.Is(err, os.ErrExist) {
		t.Fatalf("existing output error = %v, want os.ErrExist", err)
	}
	actual, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if string(actual) != string(original) {
		t.Fatalf("existing output was modified: %q", actual)
	}
}

func TestDecoderRegistryIDFor(t *testing.T) {
	registry := NewDefaultDecoderRegistry()
	if got := registry.IDFor("song.MP3"); got != "go-mp3-v0.3.4" {
		t.Fatalf("decoder ID = %q", got)
	}
	if got := registry.IDFor("song.opus"); got != "" {
		t.Fatalf("unsupported decoder ID = %q", got)
	}
	var nilRegistry *DecoderRegistry
	if nilRegistry.IDFor("song.wav") != "" {
		t.Fatal("nil registry returned a decoder ID")
	}
}

func TestWriteCanonicalWAVHonorsCanceledContext(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "input.wav")
	if err := os.WriteFile(source, makePCM16WAV(t, 1, 8000, []int16{1}), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := WriteCanonicalWAV(ctx, NewDefaultDecoderRegistry(), source, filepath.Join(dir, "out.wav")); err == nil || err == io.EOF {
		t.Fatalf("canceled decode error = %v", err)
	}
}
