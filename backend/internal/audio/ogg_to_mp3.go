package audio

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	encodermp3 "github.com/braheezy/shine-mp3/pkg/mp3"
	decodermp3 "github.com/hajimehoshi/go-mp3"
	"github.com/jfreymuth/oggvorbis"
)

const (
	mp3BitrateKbps = 320
	mp3BitrateBps  = mp3BitrateKbps * 1000
)

// MP3Metadata contains the ID3 fields retained when a downloaded Ogg file is
// converted to MP3.
type MP3Metadata struct {
	Title       string
	Artist      string
	Album       string
	AlbumArtist string
	TrackNumber int
	DiscNumber  int
	Date        string
	Genre       string
}

// ActivityCallback is called periodically while audio is being decoded and
// encoded. Callers can use it to refresh a long-running job's watchdog.
type ActivityCallback func()

// ConvertOggToMP3 decodes an Ogg/Vorbis file and encodes a 320 kbps MP3 using
// pure Go codecs. The MP3 is written transactionally beside the source file;
// the Ogg source is removed only after the completed MP3 has been validated.
func ConvertOggToMP3(ctx context.Context, sourcePath string, metadata MP3Metadata, activity ActivityCallback) (string, error) {
	if !strings.EqualFold(filepath.Ext(sourcePath), ".ogg") {
		return "", fmt.Errorf("source is not an .ogg file: %s", sourcePath)
	}

	targetPath, err := availableMP3Target(sourcePath)
	if err != nil {
		return "", err
	}

	source, err := os.Open(sourcePath)
	if err != nil {
		return "", fmt.Errorf("open Ogg source: %w", err)
	}
	defer source.Close()

	decoder, err := oggvorbis.NewReader(source)
	if err != nil {
		return "", fmt.Errorf("open Ogg/Vorbis decoder: %w", err)
	}
	channels := decoder.Channels()
	if channels != 1 && channels != 2 {
		return "", fmt.Errorf("unsupported Ogg channel count %d: MP3 conversion supports mono or stereo", channels)
	}
	temp, err := os.CreateTemp(filepath.Dir(targetPath), ".viib-mp3-*.tmp")
	if err != nil {
		return "", fmt.Errorf("create temporary MP3: %w", err)
	}
	tempPath := temp.Name()
	committed := false
	defer func() {
		if !committed {
			_ = temp.Close()
			_ = os.Remove(tempPath)
		}
	}()

	if err := writeID3v24(temp, metadata); err != nil {
		return "", fmt.Errorf("write MP3 metadata: %w", err)
	}
	if err := encodeVorbisPCM(ctx, decoder, temp, activity); err != nil {
		return "", err
	}
	if err := source.Close(); err != nil {
		return "", fmt.Errorf("close Ogg source: %w", err)
	}
	if err := temp.Sync(); err != nil {
		return "", fmt.Errorf("sync temporary MP3: %w", err)
	}
	if err := temp.Close(); err != nil {
		return "", fmt.Errorf("close temporary MP3: %w", err)
	}
	if err := validateMP3(tempPath); err != nil {
		return "", fmt.Errorf("validate converted MP3: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}

	if err := os.Rename(tempPath, targetPath); err != nil {
		return "", fmt.Errorf("commit converted MP3: %w", err)
	}
	if err := os.Remove(sourcePath); err != nil {
		if cleanupErr := os.Remove(targetPath); cleanupErr != nil {
			return "", fmt.Errorf("remove converted Ogg source: %w (also failed to remove uncommitted MP3: %v)", err, cleanupErr)
		}
		return "", fmt.Errorf("remove converted Ogg source: %w", err)
	}
	committed = true

	return targetPath, nil
}

type vorbisPCMReader interface {
	Read([]float32) (int, error)
	Channels() int
	SampleRate() int
}

func encodeVorbisPCM(ctx context.Context, decoder vorbisPCMReader, output io.Writer, activity ActivityCallback) error {
	inputChannels := decoder.Channels()
	// Shine cannot byte-align 320 kbps mono frames when the per-granule main
	// data exceeds MP3's 4095-bit field. Duplicate mono into stereo so the bit
	// budget is divided between channels and every emitted frame stays valid.
	outputChannels := inputChannels
	if outputChannels == 1 {
		outputChannels = 2
	}
	encoder, err := newMP3Encoder(decoder.SampleRate(), outputChannels)
	if err != nil {
		return err
	}
	samplesPerFrame := int(encoder.Mpeg.GranulesPerFrame) * encodermp3.GRANULE_SIZE
	frame := make([]float32, samplesPerFrame*inputChannels)
	pcm := make([]int16, samplesPerFrame*outputChannels)

	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		read := 0
		reachedEOF := false
		for read < len(frame) {
			n, err := decoder.Read(frame[read:])
			read += n
			if activity != nil && n > 0 {
				activity()
			}
			if err != nil {
				if errors.Is(err, io.EOF) {
					reachedEOF = true
					break
				}
				return fmt.Errorf("decode Ogg/Vorbis audio: %w", err)
			}
			if n == 0 {
				return fmt.Errorf("decode Ogg/Vorbis audio: %w", io.ErrNoProgress)
			}
		}

		if read == 0 && reachedEOF {
			break
		}
		if read%inputChannels != 0 {
			return errors.New("decode Ogg/Vorbis audio: decoder returned an incomplete sample frame")
		}

		clear(pcm)
		samplesRead := read / inputChannels
		for sample := 0; sample < samplesRead; sample++ {
			for channel := 0; channel < outputChannels; channel++ {
				inputChannel := channel
				if inputChannels == 1 {
					inputChannel = 0
				}
				value := frame[sample*inputChannels+inputChannel]
				pcm[sample*outputChannels+channel] = floatToPCM16(value)
			}
		}

		encoded, written := encoder.EncodeBufferInterleaved(pcm)
		if _, err := output.Write(encoded[:written]); err != nil {
			return fmt.Errorf("encode MP3 audio: %w", err)
		}
		if reachedEOF {
			break
		}
	}
	return nil
}

func floatToPCM16(value float32) int16 {
	if value > 1 {
		value = 1
	} else if value < -1 {
		value = -1
	}
	return int16(value * 32767)
}

func newMP3Encoder(sampleRate, channels int) (*encodermp3.Encoder, error) {
	if _, err := encodermp3.CheckConfig(sampleRate, mp3BitrateKbps); err != nil {
		return nil, fmt.Errorf("configure %d kbps MP3 encoder for %d Hz audio: %w", mp3BitrateKbps, sampleRate, err)
	}
	encoder := encodermp3.NewEncoder(sampleRate, channels)

	// Shine's public constructor hardcodes 128 kbps. Changing only Bitrate
	// produces inconsistent frame headers and sizes, so recompute every field
	// derived from the requested 320 kbps MPEG-1 rate before encoding begins.
	encoder.Mpeg.Bitrate = mp3BitrateKbps
	encoder.Mpeg.BitrateIndex = 14 // MPEG-1 Layer III table index for 320 kbps.
	averageSlots := (float64(encoder.Mpeg.GranulesPerFrame) * encodermp3.GRANULE_SIZE / float64(sampleRate)) *
		(float64(mp3BitrateBps) / float64(encoder.Mpeg.BitsPerSlot))
	encoder.Mpeg.WholeSlotsPerFrame = int64(averageSlots)
	encoder.Mpeg.FracSlotsPerFrame = averageSlots - float64(encoder.Mpeg.WholeSlotsPerFrame)
	encoder.Mpeg.SlotLag = -encoder.Mpeg.FracSlotsPerFrame
	encoder.Mpeg.Padding = 0
	return encoder, nil
}

func availableMP3Target(sourcePath string) (string, error) {
	base := strings.TrimSuffix(sourcePath, filepath.Ext(sourcePath))
	for suffix := 0; ; suffix++ {
		targetPath := base + ".mp3"
		if suffix > 0 {
			targetPath = fmt.Sprintf("%s (%d).mp3", base, suffix)
		}
		if _, err := os.Stat(targetPath); errors.Is(err, os.ErrNotExist) {
			return targetPath, nil
		} else if err != nil {
			return "", fmt.Errorf("inspect MP3 target: %w", err)
		}
	}
}

func validateMP3(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	decoder, err := decodermp3.NewDecoder(file)
	if err != nil {
		return err
	}
	if decoder.SampleRate() <= 0 {
		return errors.New("MP3 contains no decodable audio frames")
	}
	return nil
}

func writeID3v24(output io.Writer, metadata MP3Metadata) error {
	var frames bytes.Buffer
	textFrames := []struct {
		id    string
		value string
	}{
		{"TIT2", metadata.Title},
		{"TPE1", metadata.Artist},
		{"TALB", metadata.Album},
		{"TPE2", metadata.AlbumArtist},
		{"TRCK", positiveNumber(metadata.TrackNumber)},
		{"TPOS", positiveNumber(metadata.DiscNumber)},
		{"TDRC", metadata.Date},
		{"TCON", metadata.Genre},
	}
	for _, frame := range textFrames {
		if frame.value == "" {
			continue
		}
		payload := append([]byte{3}, []byte(strings.ToValidUTF8(frame.value, ""))...)
		if _, err := frames.WriteString(frame.id); err != nil {
			return err
		}
		if _, err := frames.Write(syncSafe(len(payload))); err != nil {
			return err
		}
		if err := binary.Write(&frames, binary.BigEndian, uint16(0)); err != nil {
			return err
		}
		if _, err := frames.Write(payload); err != nil {
			return err
		}
	}

	header := []byte{'I', 'D', '3', 4, 0, 0}
	header = append(header, syncSafe(frames.Len())...)
	if _, err := output.Write(header); err != nil {
		return err
	}
	_, err := frames.WriteTo(output)
	return err
}

func positiveNumber(value int) string {
	if value <= 0 {
		return ""
	}
	return strconv.Itoa(value)
}

func syncSafe(value int) []byte {
	return []byte{
		byte((value >> 21) & 0x7f),
		byte((value >> 14) & 0x7f),
		byte((value >> 7) & 0x7f),
		byte(value & 0x7f),
	}
}
