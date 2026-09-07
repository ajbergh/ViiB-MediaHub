package analysisbench

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
)

// WriteWAVPCM16 serializes a generated fixture as standard RIFF/WAVE PCM. It
// is intentionally a benchmark artifact writer, not the future production WAV
// decoder. Each sample is clamped to protect the fixture contract from noise
// additions or experimental generators.
func WriteWAVPCM16(writer io.Writer, fixture PCMFixture) error {
	if fixture.SampleRate <= 0 || fixture.Channels <= 0 || len(fixture.Samples)%fixture.Channels != 0 {
		return fmt.Errorf("fixture has invalid PCM geometry")
	}
	dataBytes := uint64(len(fixture.Samples)) * 2
	if dataBytes > math.MaxUint32 {
		return fmt.Errorf("fixture PCM exceeds WAV 32-bit data limit")
	}
	bytesPerFrame := fixture.Channels * 2
	byteRate := fixture.SampleRate * bytesPerFrame
	if byteRate <= 0 || byteRate > math.MaxUint32 {
		return fmt.Errorf("fixture WAV byte rate is out of range")
	}

	if _, err := io.WriteString(writer, "RIFF"); err != nil {
		return fmt.Errorf("write RIFF marker: %w", err)
	}
	if err := binary.Write(writer, binary.LittleEndian, uint32(4+8+16+8+dataBytes)); err != nil {
		return fmt.Errorf("write RIFF size: %w", err)
	}
	if _, err := io.WriteString(writer, "WAVEfmt "); err != nil {
		return fmt.Errorf("write WAVE format marker: %w", err)
	}
	for _, field := range []any{
		uint32(16), uint16(1), uint16(fixture.Channels), uint32(fixture.SampleRate),
		uint32(byteRate), uint16(bytesPerFrame), uint16(16),
	} {
		if err := binary.Write(writer, binary.LittleEndian, field); err != nil {
			return fmt.Errorf("write WAV format: %w", err)
		}
	}
	if _, err := io.WriteString(writer, "data"); err != nil {
		return fmt.Errorf("write WAV data marker: %w", err)
	}
	if err := binary.Write(writer, binary.LittleEndian, uint32(dataBytes)); err != nil {
		return fmt.Errorf("write WAV data size: %w", err)
	}

	buffer := make([]byte, 8192)
	for start := 0; start < len(fixture.Samples); {
		sampleCount := min((len(buffer) / 2), len(fixture.Samples)-start)
		for offset := 0; offset < sampleCount; offset++ {
			binary.LittleEndian.PutUint16(buffer[offset*2:], uint16(floatToPCM16(fixture.Samples[start+offset])))
		}
		if _, err := writer.Write(buffer[:sampleCount*2]); err != nil {
			return fmt.Errorf("write WAV samples: %w", err)
		}
		start += sampleCount
	}
	return nil
}

func floatToPCM16(sample float32) int16 {
	if sample <= -1 {
		return math.MinInt16
	}
	if sample >= 1 {
		return math.MaxInt16
	}
	return int16(math.Round(float64(sample) * math.MaxInt16))
}
