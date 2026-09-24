package analysis

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
)

const maxRIFFDataBytes = uint64(math.MaxUint32) - 36

// CanonicalAudioMetadata describes the decoded mono timeline written into a
// canonical PCM16 WAV. sourceAudioSHA256 intentionally hashes its PCM data
// bytes, matching the rhythm benchmark manifest's canonical source identity.
type CanonicalAudioMetadata struct {
	Decoder               string  `json:"decoder"`
	Encoding              string  `json:"encoding"`
	Channels              int     `json:"channels"`
	SampleRate            int     `json:"sampleRate"`
	Frames                int64   `json:"frames"`
	DurationSeconds       float64 `json:"durationSeconds"`
	DecoderVersion        string  `json:"decoderVersion"`
	SourceAudioSHA256     string  `json:"sourceAudioSHA256"`
	TimelineOriginSeconds float64 `json:"timelineOriginSeconds"`
	ResampleDelaySamples  int64   `json:"resampleDelaySamples"`
	StartTrimSamples      int64   `json:"startTrimSamples"`
}

// CanonicalWAVResult reports the PCM digest and timing metadata for a file
// decoded through MediaHub's shared bounded-memory decoder/downmix path.
type CanonicalWAVResult struct {
	Path           string                 `json:"canonicalWavPath"`
	PCM_SHA256     string                 `json:"pcmSHA256"`
	CanonicalAudio CanonicalAudioMetadata `json:"canonicalAudio"`
}

// WriteCanonicalWAV decodes path through the production decoder registry and
// writes bounded-memory mono PCM16 WAV at the decoded sample rate. It performs
// no trim or resampling. Unsupported codecs return ErrUnsupportedCodec.
func WriteCanonicalWAV(ctx context.Context, registry *DecoderRegistry, path, outputPath string) (result CanonicalWAVResult, err error) {
	if registry == nil {
		return result, errors.New("canonical WAV requires decoder registry")
	}
	decoderID := registry.IDFor(path)
	if decoderID == "" {
		return result, ErrUnsupportedCodec
	}
	if path == "" || outputPath == "" {
		return result, errors.New("canonical WAV requires input and output paths")
	}
	if filepath.Clean(path) == filepath.Clean(outputPath) {
		return result, errors.New("canonical WAV output must differ from input")
	}
	if sourceInfo, statErr := os.Stat(path); statErr == nil {
		if outputInfo, outputStatErr := os.Stat(outputPath); outputStatErr == nil && os.SameFile(sourceInfo, outputInfo) {
			return result, errors.New("canonical WAV output must differ from input")
		}
	}

	file, err := os.OpenFile(outputPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return result, fmt.Errorf("create canonical WAV: %w", err)
	}
	defer func() {
		closeErr := file.Close()
		if err == nil && closeErr != nil {
			err = fmt.Errorf("close canonical WAV: %w", closeErr)
		}
		if err != nil {
			_ = os.Remove(outputPath)
		}
	}()

	if _, err = file.Write(make([]byte, 44)); err != nil {
		return result, fmt.Errorf("reserve WAV header: %w", err)
	}
	digest := sha256.New()
	var sampleRate int
	var frames int64
	buffer := make([]byte, decodeChunkFrames*2)
	streamErr := StreamMonoFile(ctx, registry, path, func(chunk MonoChunk) error {
		if sampleRate == 0 {
			sampleRate = chunk.SampleRate
		} else if sampleRate != chunk.SampleRate {
			return errors.New("decoder changed sample rate during stream")
		}
		if chunk.SampleRate <= 0 || uint64(chunk.SampleRate) > uint64(^uint32(0))/2 {
			return errors.New("decoder returned invalid sample rate")
		}
		if uint64(frames+int64(len(chunk.Samples)))*2 > maxRIFFDataBytes {
			return errors.New("decoded audio exceeds RIFF/WAV size limit")
		}
		for offset := 0; offset < len(chunk.Samples); {
			count := min(len(chunk.Samples)-offset, len(buffer)/2)
			for i, sample := range chunk.Samples[offset : offset+count] {
				if math.IsNaN(float64(sample)) || math.IsInf(float64(sample), 0) {
					return errors.New("decoder produced non-finite PCM sample")
				}
				var quantized int16
				if sample <= -1 {
					quantized = math.MinInt16
				} else if sample >= 1 {
					quantized = math.MaxInt16
				} else {
					quantized = int16(math.Round(float64(sample) * math.MaxInt16))
				}
				binary.LittleEndian.PutUint16(buffer[i*2:], uint16(quantized))
			}
			bytes := count * 2
			if _, writeErr := file.Write(buffer[:bytes]); writeErr != nil {
				return fmt.Errorf("write canonical PCM: %w", writeErr)
			}
			_, _ = digest.Write(buffer[:bytes])
			offset += count
			frames += int64(count)
		}
		return nil
	})
	if streamErr != nil {
		return result, fmt.Errorf("decode canonical WAV source: %w", streamErr)
	}
	if frames == 0 || sampleRate == 0 {
		return result, errors.New("decoder produced no audio")
	}
	dataBytes := uint32(frames * 2)
	header := makePCM16WAVHeader(sampleRate, dataBytes)
	if _, err = file.Seek(0, 0); err != nil {
		return result, fmt.Errorf("seek canonical WAV header: %w", err)
	}
	if _, err = file.Write(header); err != nil {
		return result, fmt.Errorf("write canonical WAV header: %w", err)
	}
	if err = file.Sync(); err != nil {
		return result, fmt.Errorf("sync canonical WAV: %w", err)
	}
	pcmHash := hex.EncodeToString(digest.Sum(nil))
	result = CanonicalWAVResult{
		Path:       outputPath,
		PCM_SHA256: pcmHash,
		CanonicalAudio: CanonicalAudioMetadata{
			Decoder: "ViiB", Encoding: "pcm_s16le", Channels: 1,
			SampleRate: sampleRate, Frames: frames,
			DurationSeconds: float64(frames) / float64(sampleRate),
			DecoderVersion:  decoderID, SourceAudioSHA256: pcmHash,
			TimelineOriginSeconds: 0, ResampleDelaySamples: 0, StartTrimSamples: 0,
		},
	}
	return result, nil
}

func makePCM16WAVHeader(sampleRate int, dataBytes uint32) []byte {
	header := make([]byte, 44)
	copy(header[:4], "RIFF")
	binary.LittleEndian.PutUint32(header[4:8], 36+dataBytes)
	copy(header[8:12], "WAVE")
	copy(header[12:16], "fmt ")
	binary.LittleEndian.PutUint32(header[16:20], 16)
	binary.LittleEndian.PutUint16(header[20:22], 1)
	binary.LittleEndian.PutUint16(header[22:24], 1)
	binary.LittleEndian.PutUint32(header[24:28], uint32(sampleRate))
	binary.LittleEndian.PutUint32(header[28:32], uint32(sampleRate*2))
	binary.LittleEndian.PutUint16(header[32:34], 2)
	binary.LittleEndian.PutUint16(header[34:36], 16)
	copy(header[36:40], "data")
	binary.LittleEndian.PutUint32(header[40:44], dataBytes)
	return header
}
