package analysis

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"

	"github.com/hajimehoshi/go-mp3"
)

// MP3Decoder adapts the existing pure-Go go-mp3 decoder to the shared PCM
// stream contract. go-mp3 emits interleaved signed 16-bit stereo PCM.
type MP3Decoder struct{}

func (MP3Decoder) ID() string { return "go-mp3-v0.3.4" }

func (MP3Decoder) Open(ctx context.Context, source io.ReadCloser) (PCMStream, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	decoder, err := mp3.NewDecoder(source)
	if err != nil {
		return nil, fmt.Errorf("open MP3 decoder: %w", err)
	}
	if decoder.SampleRate() <= 0 {
		return nil, fmt.Errorf("MP3 has no decodable audio frames")
	}
	return &mp3Stream{source: source, decoder: decoder, info: PCMInfo{SampleRate: decoder.SampleRate(), Channels: 2}}, nil
}

type mp3Stream struct {
	source  io.ReadCloser
	decoder *mp3.Decoder
	info    PCMInfo
	buffer  []byte
}

func (s *mp3Stream) Info() PCMInfo { return s.info }
func (s *mp3Stream) Close() error  { return s.source.Close() }
func (s *mp3Stream) Read(ctx context.Context, out []float32) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if len(out) == 0 {
		return 0, nil
	}
	bytes := len(out) * 2
	if cap(s.buffer) < bytes {
		s.buffer = make([]byte, bytes)
	}
	n, err := io.ReadFull(s.decoder, s.buffer[:bytes])
	if n%2 != 0 {
		return 0, fmt.Errorf("MP3 decoder returned unaligned sample data")
	}
	for i := 0; i < n/2; i++ {
		out[i] = float32(int16(binary.LittleEndian.Uint16(s.buffer[i*2:]))) / 32768
	}
	if err == io.ErrUnexpectedEOF {
		err = io.EOF
	}
	return n / 2, err
}
