package analysis

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
)

// WAVPCM16Decoder decodes standard RIFF/WAVE signed PCM16 and IEEE float32
// into normalized PCM. Other widths remain explicit future decoder decisions.
type WAVPCM16Decoder struct{}

func (WAVPCM16Decoder) ID() string { return "wav-pcm16-v1" }

func (WAVPCM16Decoder) Open(ctx context.Context, source io.ReadCloser) (PCMStream, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	info, dataBytes, audioFormat, bitsPerSample, err := readWAVPCM16Header(source)
	if err != nil {
		return nil, err
	}
	return &wavPCM16Stream{source: source, info: info, remaining: int64(dataBytes), audioFormat: audioFormat, bytesPerSample: int(bitsPerSample / 8)}, nil
}

// NewDefaultDecoderRegistry returns the currently production-approved decoder
// set. Unsupported formats stay explicit rather than falling through to a
// browser-only path.
func NewDefaultDecoderRegistry() *DecoderRegistry {
	registry := NewDecoderRegistry()
	_ = registry.Register([]string{".wav", ".wave"}, WAVPCM16Decoder{})
	_ = registry.Register([]string{".mp3"}, MP3Decoder{})
	return registry
}

type wavPCM16Stream struct {
	source         io.ReadCloser
	info           PCMInfo
	remaining      int64
	buffer         []byte
	audioFormat    uint16
	bytesPerSample int
}

func (s *wavPCM16Stream) Info() PCMInfo { return s.info }
func (s *wavPCM16Stream) Close() error  { return s.source.Close() }

func (s *wavPCM16Stream) Read(ctx context.Context, out []float32) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if len(out) == 0 {
		return 0, nil
	}
	if s.remaining == 0 {
		return 0, io.EOF
	}
	samples := min(len(out), int(s.remaining/int64(s.bytesPerSample)))
	bytes := samples * s.bytesPerSample
	if cap(s.buffer) < bytes {
		s.buffer = make([]byte, bytes)
	}
	if _, err := io.ReadFull(s.source, s.buffer[:bytes]); err != nil {
		return 0, fmt.Errorf("read WAV PCM: %w", err)
	}
	s.remaining -= int64(bytes)
	for i := 0; i < samples; i++ {
		if s.audioFormat == 1 {
			out[i] = float32(int16(binary.LittleEndian.Uint16(s.buffer[i*2:]))) / 32768
		} else {
			out[i] = math.Float32frombits(binary.LittleEndian.Uint32(s.buffer[i*4:]))
		}
	}
	if s.remaining == 0 {
		return samples, io.EOF
	}
	return samples, nil
}

func readWAVPCM16Header(source io.Reader) (PCMInfo, uint32, uint16, uint16, error) {
	var header [12]byte
	if _, err := io.ReadFull(source, header[:]); err != nil {
		return PCMInfo{}, 0, 0, 0, fmt.Errorf("read RIFF header: %w", err)
	}
	if string(header[:4]) != "RIFF" || string(header[8:]) != "WAVE" {
		return PCMInfo{}, 0, 0, 0, errors.New("not a RIFF/WAVE file")
	}
	var info PCMInfo
	formatSeen := false
	var audioFormat, bitsPerSample uint16
	for {
		var chunk [8]byte
		if _, err := io.ReadFull(source, chunk[:]); err != nil {
			return PCMInfo{}, 0, 0, 0, fmt.Errorf("read WAV chunk: %w", err)
		}
		size := binary.LittleEndian.Uint32(chunk[4:])
		switch string(chunk[:4]) {
		case "fmt ":
			if size < 16 {
				return PCMInfo{}, 0, 0, 0, errors.New("WAV format chunk is too short")
			}
			var format [16]byte
			if _, err := io.ReadFull(source, format[:]); err != nil {
				return PCMInfo{}, 0, 0, 0, err
			}
			if _, err := io.CopyN(io.Discard, source, int64(size-16)); err != nil {
				return PCMInfo{}, 0, 0, 0, err
			}
			audioFormat, bitsPerSample = binary.LittleEndian.Uint16(format[:2]), binary.LittleEndian.Uint16(format[14:])
			if (audioFormat != 1 || bitsPerSample != 16) && (audioFormat != 3 || bitsPerSample != 32) {
				return PCMInfo{}, 0, 0, 0, errors.New("WAV decoder requires PCM16 or float32")
			}
			info = PCMInfo{Channels: int(binary.LittleEndian.Uint16(format[2:4])), SampleRate: int(binary.LittleEndian.Uint32(format[4:8]))}
			if info.Channels <= 0 || info.SampleRate <= 0 {
				return PCMInfo{}, 0, 0, 0, errors.New("WAV has invalid PCM geometry")
			}
			formatSeen = true
		case "data":
			if !formatSeen || size%uint32(info.Channels*int(bitsPerSample/8)) != 0 {
				return PCMInfo{}, 0, 0, 0, errors.New("WAV data is not PCM frame aligned")
			}
			return info, size, audioFormat, bitsPerSample, nil
		default:
			if _, err := io.CopyN(io.Discard, source, int64(size)); err != nil {
				return PCMInfo{}, 0, 0, 0, err
			}
		}
		if size%2 == 1 {
			if _, err := io.CopyN(io.Discard, source, 1); err != nil {
				return PCMInfo{}, 0, 0, 0, err
			}
		}
	}
}
