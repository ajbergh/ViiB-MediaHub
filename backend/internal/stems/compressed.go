package stems

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"

	"github.com/hajimehoshi/go-mp3"
	"github.com/jfreymuth/oggvorbis"
)

const CompressedStemsFeature = "compressed-stems-v1"

// Compressed stems use the container's gapless timeline. No decoded WAV is saved.
type compressedAudio struct {
	geometry AudioGeometry
	read     func([]float32) (int, error)
	seek     func(int64) error
}

type contextAudioFile struct {
	ctx  context.Context
	file *os.File
}

func (f contextAudioFile) Read(buffer []byte) (int, error) {
	if err := f.ctx.Err(); err != nil {
		return 0, err
	}
	return f.file.Read(buffer)
}
func (f contextAudioFile) Seek(offset int64, whence int) (int64, error) {
	if err := f.ctx.Err(); err != nil {
		return 0, err
	}
	return f.file.Seek(offset, whence)
}

func artifactCodecMatches(a Artifact) bool {
	switch strings.ToLower(filepath.Ext(a.Path)) {
	case ".wav", ".wave":
		return a.Encoding == "pcm_s16le" || a.Encoding == "float32le"
	case ".ogg":
		return a.Encoding == "vorbis"
	case ".mp3":
		return a.Encoding == "mp3"
	}
	return false
}

func openCompressed(ctx context.Context, f *os.File, encoding string) (compressedAudio, error) {
	var result compressedAudio
	if encoding == "vorbis" {
		decoder, err := oggvorbis.NewReader(contextAudioFile{ctx, f})
		if err != nil {
			return result, err
		}
		result.geometry = AudioGeometry{SampleRate: decoder.SampleRate(), Channels: decoder.Channels(), Frames: decoder.Length()}
		result.read = decoder.Read
		result.seek = decoder.SetPosition
	} else if encoding == "mp3" {
		geometry, skip, rawFrames, err := mp3GaplessGeometry(f)
		if err != nil {
			return result, err
		}
		if _, err = f.Seek(0, io.SeekStart); err != nil {
			return result, err
		}
		decoder, err := mp3.NewDecoder(contextAudioFile{ctx, f})
		if err != nil {
			return result, err
		}
		if decoder.SampleRate() != geometry.SampleRate || decoder.Length()/4 != rawFrames {
			return result, errors.New("MP3 frame count does not match Xing gapless metadata")
		}
		result.geometry = geometry
		remaining := geometry.Frames
		result.seek = func(frame int64) error {
			if frame < 0 || frame > geometry.Frames {
				return errors.New("invalid MP3 seek")
			}
			// go-mp3's single-frame seek preroll is insufficient for the bit
			// reservoir and synthesis history. Warm from up to 512 MPEG frames
			// earlier (the maximum reservoir is 511 bytes, and a valid frame
			// contributes at least one byte). Reads remain bounded in memory.
			target := skip + frame
			mpegSamples := int64(1152)
			if geometry.SampleRate < 32000 {
				mpegSamples = 576
			}
			warm := max(int64(0), target-512*mpegSamples)
			warm = warm / mpegSamples * mpegSamples
			if _, err := decoder.Seek(warm*4, io.SeekStart); err != nil {
				return err
			}
			buffer := make([]byte, 8192)
			for bytes := (target - warm) * 4; bytes > 0; {
				if err := ctx.Err(); err != nil {
					return err
				}
				n, err := io.ReadFull(decoder, buffer[:min(int64(len(buffer)), bytes)])
				if err != nil {
					return err
				}
				bytes -= int64(n)
			}
			remaining = geometry.Frames - frame
			return nil
		}
		if err = result.seek(0); err != nil {
			return result, err
		}
		var buffer []byte
		result.read = func(out []float32) (int, error) {
			frames := min(int64(len(out)/geometry.Channels), remaining)
			if frames == 0 {
				return 0, io.EOF
			}
			bytes := int(frames) * 4 // go-mp3 expands mono to stereo.
			if cap(buffer) < bytes {
				buffer = make([]byte, bytes)
			}
			n, err := io.ReadFull(decoder, buffer[:bytes])
			if n%4 != 0 {
				return 0, errors.New("MP3 returned incomplete PCM frames")
			}
			for i := 0; i < n/4; i++ {
				for channel := 0; channel < geometry.Channels; channel++ {
					offset := i*4 + channel*2
					out[i*geometry.Channels+channel] = float32(int16(binary.LittleEndian.Uint16(buffer[offset:offset+2]))) / 32768
				}
			}
			remaining -= int64(n / 4)
			return n / 4 * geometry.Channels, err
		}
	} else {
		return result, errors.New("unsupported compressed stem codec")
	}
	if result.geometry.SampleRate <= 0 || (result.geometry.Channels != 1 && result.geometry.Channels != 2) || result.geometry.Frames <= 0 {
		return result, errors.New("invalid compressed audio geometry")
	}
	return result, nil
}

// mp3GaplessGeometry reads the first MPEG Layer III frame's Xing/Info and
// LAME-compatible extension. Its delay/padding describe the original PCM length.
// go-mp3 includes the Xing frame and has 529 synthesis-delay samples; neither
// belongs to the source timeline. Files without gapless metadata are rejected
// because guessing would shift every stem relative to its source.
func mp3GaplessGeometry(f *os.File) (AudioGeometry, int64, int64, error) {
	var zero AudioGeometry
	fail := func() (AudioGeometry, int64, int64, error) {
		return zero, 0, 0, errors.New("MP3 stems require valid Xing/LAME gapless metadata")
	}
	var tag [10]byte
	if _, err := f.ReadAt(tag[:], 0); err != nil {
		return fail()
	}
	offset := int64(0)
	if string(tag[:3]) == "ID3" {
		for _, b := range tag[6:10] {
			if b&0x80 != 0 {
				return fail()
			}
		}
		offset = 10 + int64(tag[6])<<21 + int64(tag[7])<<14 + int64(tag[8])<<7 + int64(tag[9])
		if tag[5]&0x10 != 0 {
			offset += 10
		}
	}
	var header [4]byte
	if _, err := f.ReadAt(header[:], offset); err != nil {
		return fail()
	}
	h := binary.BigEndian.Uint32(header[:])
	version, layer := (h>>19)&3, (h>>17)&3
	rateIndex, bitIndex := (h>>10)&3, (h>>12)&15
	if h>>21 != 0x7ff || (version != 3 && version != 2) || layer != 1 || rateIndex == 3 || bitIndex == 0 || bitIndex == 15 {
		return fail()
	}
	rate := []int{44100, 48000, 32000}[rateIndex]
	channels := 2
	if (h>>6)&3 == 3 {
		channels = 1
	}
	bitrates := []int{0, 32, 40, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320}
	samples, multiplier, side := 1152, 144000, 32
	if channels == 1 {
		side = 17
	}
	if version == 2 {
		rate /= 2
		samples = 576
		multiplier = 72000
		side = 17
		if channels == 1 {
			side = 9
		}
		bitrates = []int{0, 8, 16, 24, 32, 40, 48, 56, 64, 80, 96, 112, 128, 144, 160}
	}
	size := multiplier*bitrates[bitIndex]/rate + int((h>>9)&1)
	frame := make([]byte, size)
	if _, err := f.ReadAt(frame, offset); err != nil {
		return fail()
	}
	at := 4 + side
	if (h>>16)&1 == 0 {
		at += 2
	}
	if at+12 > len(frame) || (string(frame[at:at+4]) != "Xing" && string(frame[at:at+4]) != "Info") {
		return fail()
	}
	flags := binary.BigEndian.Uint32(frame[at+4 : at+8])
	at += 8
	if flags&1 == 0 {
		return fail()
	}
	count := int64(binary.BigEndian.Uint32(frame[at : at+4]))
	at += 4
	if flags&2 != 0 {
		at += 4
	}
	if flags&4 != 0 {
		at += 100
	}
	if flags&8 != 0 {
		at += 4
	}
	if at+24 > len(frame) {
		return fail()
	}
	encoder := string(frame[at : at+4])
	if encoder != "LAME" && encoder != "Lavc" && encoder != "Lavf" {
		return fail()
	}
	delay := int64(frame[at+21])<<4 | int64(frame[at+22]>>4)
	padding := int64(frame[at+22]&15)<<8 | int64(frame[at+23])
	frames := count*int64(samples) - delay - padding
	if count <= 0 || frames <= 0 || padding < 529 {
		return fail()
	}
	return AudioGeometry{SampleRate: rate, Channels: channels, Frames: frames}, int64(samples) + delay + 529, (count + 1) * int64(samples), nil
}

func inspectAudio(ctx context.Context, path, encoding string) (AudioGeometry, string, error) {
	if encoding != "mp3" && encoding != "vorbis" {
		return inspectWAV(path)
	}
	f, err := os.Open(path)
	if err != nil {
		return AudioGeometry{}, "", err
	}
	defer f.Close()
	decoder, err := openCompressed(ctx, f, encoding)
	if err != nil {
		return AudioGeometry{}, "", err
	}
	buffer := make([]float32, 8192)
	var values int64
	for {
		if err = ctx.Err(); err != nil {
			return AudioGeometry{}, "", err
		}
		n, readErr := decoder.read(buffer)
		if n < 0 || n > len(buffer) || n%decoder.geometry.Channels != 0 {
			return AudioGeometry{}, "", errors.New("incomplete compressed PCM frames")
		}
		for _, sample := range buffer[:n] {
			if math.IsNaN(float64(sample)) || math.IsInf(float64(sample), 0) {
				return AudioGeometry{}, "", errors.New("non-finite compressed samples")
			}
		}
		values += int64(n)
		if readErr != nil {
			if !(errors.Is(readErr, io.EOF) || (encoding == "vorbis" && errors.Is(readErr, io.ErrUnexpectedEOF) && values == decoder.geometry.Frames*int64(decoder.geometry.Channels))) {
				return AudioGeometry{}, "", readErr
			}
			break
		}
		if n == 0 {
			return AudioGeometry{}, "", io.ErrNoProgress
		}
	}
	if values != decoder.geometry.Frames*int64(decoder.geometry.Channels) {
		return AudioGeometry{}, "", errors.New("compressed audio is truncated or has inconsistent frame count")
	}
	return decoder.geometry, encoding, nil
}

func readCompressedFrames(ctx context.Context, f *os.File, a Artifact, start, count int64) ([]float32, error) {
	decoder, err := openCompressed(ctx, f, a.Encoding)
	if err != nil {
		return nil, err
	}
	if decoder.geometry != (AudioGeometry{SampleRate: a.SampleRate, Channels: a.Channels, Frames: a.Frames}) {
		return nil, errors.New("compressed stem geometry changed")
	}
	if count > int64(int(^uint(0)>>1))/int64(a.Channels) {
		return nil, errors.New("frame range too large")
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	if err = decoder.seek(start); err != nil {
		return nil, err
	}
	samples := make([]float32, int(count)*a.Channels)
	for pos := 0; pos < len(samples); {
		if err = ctx.Err(); err != nil {
			return nil, err
		}
		n, readErr := decoder.read(samples[pos:min(pos+8192, len(samples))])
		pos += n
		if readErr != nil && !(pos == len(samples) && (errors.Is(readErr, io.EOF) || errors.Is(readErr, io.ErrUnexpectedEOF))) {
			return nil, fmt.Errorf("read compressed frames: %w", readErr)
		}
		if n == 0 {
			return nil, io.ErrNoProgress
		}
	}
	for _, sample := range samples {
		if math.IsNaN(float64(sample)) || math.IsInf(float64(sample), 0) {
			return nil, errors.New("non-finite compressed samples")
		}
	}
	return samples, nil
}
