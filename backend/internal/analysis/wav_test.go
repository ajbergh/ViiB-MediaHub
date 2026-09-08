package analysis

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"testing"
)

func TestWAVPCM16DecoderStreamsNormalizedSamples(t *testing.T) {
	samples := []int16{math.MinInt16, 0, math.MaxInt16, 16384}
	decoder := WAVPCM16Decoder{}
	stream, err := decoder.Open(context.Background(), io.NopCloser(bytes.NewReader(makePCM16WAV(t, 1, 44100, samples))))
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	if stream.Info() != (PCMInfo{SampleRate: 44100, Channels: 1}) {
		t.Fatalf("info = %#v", stream.Info())
	}
	out := make([]float32, 3)
	n, err := stream.Read(context.Background(), out)
	if n != 3 || err != nil || out[0] != -1 || out[1] != 0 || out[2] <= .9999 {
		t.Fatalf("first read = %d, %v, %#v", n, err, out)
	}
	n, err = stream.Read(context.Background(), out)
	if n != 1 || err != io.EOF || math.Abs(float64(out[0]-.5)) > .0001 {
		t.Fatalf("last read = %d, %v, %#v", n, err, out)
	}
}

func TestDefaultDecoderRegistrySupportsOnlyWAVPCM16(t *testing.T) {
	registry := NewDefaultDecoderRegistry()
	stream, err := registry.Open(context.Background(), "track.WAVE", io.NopCloser(bytes.NewReader(makePCM16WAV(t, 2, 48000, []int16{0, 0}))))
	if err != nil {
		t.Fatal(err)
	}
	stream.Close()
	if _, err := registry.Open(context.Background(), "track.flac", io.NopCloser(bytes.NewReader(nil))); err != ErrUnsupportedCodec {
		t.Fatalf("flac open error = %v", err)
	}
}

func TestMP3DecoderRejectsMalformedInput(t *testing.T) {
	_, err := MP3Decoder{}.Open(context.Background(), io.NopCloser(bytes.NewReader([]byte("not an mp3"))))
	if err == nil {
		t.Fatal("malformed MP3 accepted")
	}
}

func TestVorbisDecoderRejectsMalformedInputAndOpusStaysUnsupported(t *testing.T) {
	if _, err := (VorbisDecoder{}).Open(context.Background(), io.NopCloser(bytes.NewReader([]byte("not ogg")))); err == nil {
		t.Fatal("malformed Vorbis accepted")
	}
	if _, err := NewDefaultDecoderRegistry().Open(context.Background(), "track.opus", io.NopCloser(bytes.NewReader(nil))); err != ErrUnsupportedCodec {
		t.Fatalf("opus open error = %v", err)
	}
}

func TestNormalizeTerminalVorbisEOFPreservesEarlyFailure(t *testing.T) {
	if err := normalizeTerminalVorbisEOF(io.ErrUnexpectedEOF, 100, 100); err != io.EOF {
		t.Fatalf("complete stream error = %v, want EOF", err)
	}
	if err := normalizeTerminalVorbisEOF(io.ErrUnexpectedEOF, 99, 100); err != io.ErrUnexpectedEOF {
		t.Fatalf("early stream error = %v, want unexpected EOF", err)
	}
	if err := normalizeTerminalVorbisEOF(io.ErrUnexpectedEOF, 100, 0); err != io.ErrUnexpectedEOF {
		t.Fatalf("unknown-length error = %v, want unexpected EOF", err)
	}
	if err := normalizeTerminalVorbisEOF(fmt.Errorf("decoder: %w", io.ErrUnexpectedEOF), 100, 100); err != io.EOF {
		t.Fatalf("wrapped complete stream error = %v, want EOF", err)
	}
}

func TestWAVPCM16DecoderAlsoStreamsIEEEFloat32(t *testing.T) {
	decoder := WAVPCM16Decoder{}
	stream, err := decoder.Open(context.Background(), io.NopCloser(bytes.NewReader(makeFloat32WAV(t, 1, 22050, []float32{-.5, .25}))))
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	out := make([]float32, 2)
	n, err := stream.Read(context.Background(), out)
	if n != 2 || err != io.EOF || out[0] != -.5 || out[1] != .25 {
		t.Fatalf("float read = %d, %v, %#v", n, err, out)
	}
}

func makePCM16WAV(t *testing.T, channels, sampleRate int, samples []int16) []byte {
	t.Helper()
	var payload bytes.Buffer
	for _, sample := range samples {
		if err := binary.Write(&payload, binary.LittleEndian, sample); err != nil {
			t.Fatal(err)
		}
	}
	var result bytes.Buffer
	result.WriteString("RIFF")
	if err := binary.Write(&result, binary.LittleEndian, uint32(36+payload.Len())); err != nil {
		t.Fatal(err)
	}
	result.WriteString("WAVEfmt ")
	for _, value := range []any{uint32(16), uint16(1), uint16(channels), uint32(sampleRate), uint32(sampleRate * channels * 2), uint16(channels * 2), uint16(16)} {
		if err := binary.Write(&result, binary.LittleEndian, value); err != nil {
			t.Fatal(err)
		}
	}
	result.WriteString("data")
	if err := binary.Write(&result, binary.LittleEndian, uint32(payload.Len())); err != nil {
		t.Fatal(err)
	}
	result.Write(payload.Bytes())
	return result.Bytes()
}

func makeFloat32WAV(t *testing.T, channels, sampleRate int, samples []float32) []byte {
	t.Helper()
	var payload bytes.Buffer
	for _, sample := range samples {
		if err := binary.Write(&payload, binary.LittleEndian, sample); err != nil {
			t.Fatal(err)
		}
	}
	var result bytes.Buffer
	result.WriteString("RIFF")
	if err := binary.Write(&result, binary.LittleEndian, uint32(36+payload.Len())); err != nil {
		t.Fatal(err)
	}
	result.WriteString("WAVEfmt ")
	for _, value := range []any{uint32(16), uint16(3), uint16(channels), uint32(sampleRate), uint32(sampleRate * channels * 4), uint16(channels * 4), uint16(32)} {
		if err := binary.Write(&result, binary.LittleEndian, value); err != nil {
			t.Fatal(err)
		}
	}
	result.WriteString("data")
	if err := binary.Write(&result, binary.LittleEndian, uint32(payload.Len())); err != nil {
		t.Fatal(err)
	}
	result.Write(payload.Bytes())
	return result.Bytes()
}
