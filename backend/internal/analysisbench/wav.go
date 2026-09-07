package analysisbench

import (
	"encoding/binary"
	"fmt"
	"io"
)

// WAVInfo is the bounded metadata needed to benchmark a WAV decoder before the
// production PCM registry exists. InspectWAV never loads the data chunk.
type WAVInfo struct {
	AudioFormat   uint16 `json:"audioFormat"`
	Channels      uint16 `json:"channels"`
	SampleRate    uint32 `json:"sampleRate"`
	BitsPerSample uint16 `json:"bitsPerSample"`
	DataBytes     uint32 `json:"dataBytes"`
	Frames        uint64 `json:"frames"`
}

// InspectWAV validates a RIFF/WAVE PCM or IEEE-float header and returns stream
// geometry. It is deliberately Phase 0-only: it verifies codec assumptions but
// does not yet expose decoded samples to production analysis code.
func InspectWAV(reader io.Reader) (WAVInfo, error) {
	return readWAVHeader(reader)
}

func readWAVHeader(reader io.Reader) (WAVInfo, error) {
	var header [12]byte
	if _, err := io.ReadFull(reader, header[:]); err != nil {
		return WAVInfo{}, fmt.Errorf("read RIFF header: %w", err)
	}
	if string(header[0:4]) != "RIFF" || string(header[8:12]) != "WAVE" {
		return WAVInfo{}, fmt.Errorf("not a RIFF/WAVE file")
	}

	var info WAVInfo
	var haveFormat bool
	for {
		var chunkHeader [8]byte
		if _, err := io.ReadFull(reader, chunkHeader[:]); err != nil {
			return WAVInfo{}, fmt.Errorf("read WAV chunk header: %w", err)
		}
		chunkSize := binary.LittleEndian.Uint32(chunkHeader[4:])
		switch string(chunkHeader[0:4]) {
		case "fmt ":
			if chunkSize < 16 {
				return WAVInfo{}, fmt.Errorf("WAV format chunk is %d bytes; need at least 16", chunkSize)
			}
			format := make([]byte, chunkSize)
			if _, err := io.ReadFull(reader, format); err != nil {
				return WAVInfo{}, fmt.Errorf("read WAV format chunk: %w", err)
			}
			info.AudioFormat = binary.LittleEndian.Uint16(format[0:2])
			info.Channels = binary.LittleEndian.Uint16(format[2:4])
			info.SampleRate = binary.LittleEndian.Uint32(format[4:8])
			info.BitsPerSample = binary.LittleEndian.Uint16(format[14:16])
			if err := validateWAVFormat(info); err != nil {
				return WAVInfo{}, err
			}
			haveFormat = true
		case "data":
			if !haveFormat {
				return WAVInfo{}, fmt.Errorf("WAV data chunk occurs before format chunk")
			}
			bytesPerFrame := uint64(info.Channels) * uint64(info.BitsPerSample) / 8
			if bytesPerFrame == 0 || uint64(chunkSize)%bytesPerFrame != 0 {
				return WAVInfo{}, fmt.Errorf("WAV data length %d is not frame-aligned", chunkSize)
			}
			info.DataBytes = chunkSize
			info.Frames = uint64(chunkSize) / bytesPerFrame
			return info, nil
		default:
			if _, err := io.CopyN(io.Discard, reader, int64(chunkSize)); err != nil {
				return WAVInfo{}, fmt.Errorf("skip WAV %q chunk: %w", string(chunkHeader[0:4]), err)
			}
		}
		if chunkSize%2 == 1 {
			if _, err := io.CopyN(io.Discard, reader, 1); err != nil {
				return WAVInfo{}, fmt.Errorf("skip WAV chunk padding: %w", err)
			}
		}
	}
}

func validateWAVFormat(info WAVInfo) error {
	if info.AudioFormat != 1 && info.AudioFormat != 3 {
		return fmt.Errorf("unsupported WAV format code %d", info.AudioFormat)
	}
	if info.Channels == 0 || info.SampleRate == 0 {
		return fmt.Errorf("WAV has invalid channel count or sample rate")
	}
	if info.BitsPerSample == 0 || info.BitsPerSample%8 != 0 {
		return fmt.Errorf("WAV has unsupported %d-bit samples", info.BitsPerSample)
	}
	return nil
}
