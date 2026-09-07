package analysisbench

import (
	"encoding/binary"
	"fmt"
	"io"
)

// WAVPCM16Reader is the Phase 0 streaming decode spike for standard signed
// 16-bit PCM WAV. It deliberately supports one narrowly specified format so
// decoder correctness and bounded reads can be measured before Phase 1 builds
// the production registry.
type WAVPCM16Reader struct {
	info      WAVInfo
	source    io.Reader
	remaining int64
	buffer    []byte
}

// OpenWAVPCM16 positions a reader at the data chunk and refuses formats that
// need a separate Phase 1 decoding decision.
func OpenWAVPCM16(source io.Reader) (*WAVPCM16Reader, error) {
	info, err := readWAVHeader(source)
	if err != nil {
		return nil, err
	}
	if info.AudioFormat != 1 || info.BitsPerSample != 16 {
		return nil, fmt.Errorf("WAV PCM16 decoder requires format 1 / 16-bit samples; got format %d / %d-bit", info.AudioFormat, info.BitsPerSample)
	}
	return &WAVPCM16Reader{info: info, source: source, remaining: int64(info.DataBytes)}, nil
}

// Info returns the immutable PCM geometry for the current data chunk.
func (reader *WAVPCM16Reader) Info() WAVInfo { return reader.info }

// Read decodes interleaved PCM samples into out. It preserves bounded memory:
// the only internal storage is sized to the caller's largest requested chunk.
func (reader *WAVPCM16Reader) Read(out []float32) (int, error) {
	if len(out) == 0 {
		return 0, nil
	}
	if reader.remaining == 0 {
		return 0, io.EOF
	}
	sampleCount := min(len(out), int(reader.remaining/2))
	byteCount := sampleCount * 2
	if cap(reader.buffer) < byteCount {
		reader.buffer = make([]byte, byteCount)
	}
	buffer := reader.buffer[:byteCount]
	if _, err := io.ReadFull(reader.source, buffer); err != nil {
		return 0, fmt.Errorf("read WAV PCM data: %w", err)
	}
	reader.remaining -= int64(byteCount)
	for index := 0; index < sampleCount; index++ {
		out[index] = float32(int16(binary.LittleEndian.Uint16(buffer[index*2:]))) / 32768
	}
	if reader.remaining == 0 {
		return sampleCount, io.EOF
	}
	return sampleCount, nil
}
