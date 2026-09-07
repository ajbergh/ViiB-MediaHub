package analysis

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
)

type testDecoder struct{ opened bool }

func (d *testDecoder) ID() string { return "test" }
func (d *testDecoder) Open(_ context.Context, source io.ReadCloser) (PCMStream, error) {
	d.opened = true
	return &testStream{source: source}, nil
}

type testStream struct{ source io.ReadCloser }

func (*testStream) Info() PCMInfo                                { return PCMInfo{SampleRate: 44100, Channels: 2} }
func (*testStream) Read(context.Context, []float32) (int, error) { return 0, io.EOF }
func (s *testStream) Close() error                               { return s.source.Close() }

func TestDecoderRegistryUsesCaseInsensitiveExtensions(t *testing.T) {
	registry := NewDecoderRegistry()
	decoder := &testDecoder{}
	if err := registry.Register([]string{"wav", ".wave"}, decoder); err != nil {
		t.Fatal(err)
	}
	stream, err := registry.Open(context.Background(), "TRACK.WAV", io.NopCloser(strings.NewReader("pcm")))
	if err != nil || !decoder.opened || stream.Info().SampleRate != 44100 {
		t.Fatalf("open = %#v, %v", stream, err)
	}
	if err := stream.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestDecoderRegistryRejectsUnsupportedAndDuplicateFormats(t *testing.T) {
	registry := NewDecoderRegistry()
	decoder := &testDecoder{}
	if err := registry.Register([]string{".wav"}, decoder); err != nil {
		t.Fatal(err)
	}
	if err := registry.Register([]string{"WAV"}, &testDecoder{}); err == nil {
		t.Fatal("duplicate extension accepted")
	}
	_, err := registry.Open(context.Background(), "track.opus", io.NopCloser(strings.NewReader("opus")))
	if !errors.Is(err, ErrUnsupportedCodec) {
		t.Fatalf("unsupported open error = %v", err)
	}
}
