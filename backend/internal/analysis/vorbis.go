package analysis

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/jfreymuth/oggvorbis"
)

// VorbisDecoder adapts the existing MIT-licensed Ogg/Vorbis decoder. It does
// not accept .opus: an Ogg container alone is not evidence of Vorbis audio.
type VorbisDecoder struct{}

func (VorbisDecoder) ID() string { return "oggvorbis-v1.0.5" }
func (VorbisDecoder) Open(ctx context.Context, source io.ReadCloser) (PCMStream, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	decoder, err := oggvorbis.NewReader(source)
	if err != nil {
		return nil, fmt.Errorf("open Ogg/Vorbis decoder: %w", err)
	}
	if decoder.SampleRate() <= 0 || decoder.Channels() <= 0 {
		return nil, fmt.Errorf("vorbis stream has invalid PCM geometry")
	}
	declaredFrames := decoder.Length()
	return &vorbisStream{
		source: source, decoder: decoder, info: PCMInfo{SampleRate: decoder.SampleRate(), Channels: decoder.Channels(), DeclaredFrames: declaredFrames},
		expectedValues: declaredFrames * int64(decoder.Channels()),
	}, nil
}

type vorbisStream struct {
	source         io.ReadCloser
	decoder        *oggvorbis.Reader
	info           PCMInfo
	expectedValues int64
	emittedValues  int64
}

func (s *vorbisStream) Info() PCMInfo { return s.info }
func (s *vorbisStream) Close() error  { return s.source.Close() }
func (s *vorbisStream) Read(ctx context.Context, out []float32) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	n, err := s.decoder.Read(out)
	s.emittedValues += int64(n)
	return n, normalizeTerminalVorbisEOF(err, s.emittedValues, s.expectedValues)
}

// normalizeTerminalVorbisEOF preserves an unexpected EOF unless a seekable
// source has already yielded every sample declared by its final Ogg granule.
// Some playable Ogg/Vorbis files trigger that library error after the complete
// stream has been emitted; treating it as EOF is safe only in that exact case.
func normalizeTerminalVorbisEOF(err error, emittedValues, expectedValues int64) error {
	if errors.Is(err, io.ErrUnexpectedEOF) && expectedValues > 0 && emittedValues >= expectedValues {
		return io.EOF
	}
	return err
}
