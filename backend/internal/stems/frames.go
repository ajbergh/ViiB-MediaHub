package stems

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
)

// ReadPCMFrames returns normalized interleaved float32 samples for one
// contiguous range. The package validator checks the full-file checksum on
// cache miss; this function checks the current WAV geometry and stable file
// identity on the same open handle used to read the requested frames.
func ReadPCMFrames(path string, artifact Artifact, startFrame, frameCount int64) ([]float32, error) {
	if startFrame < 0 || frameCount <= 0 || startFrame > artifact.Frames || frameCount > artifact.Frames-startFrame {
		return nil, errors.New("requested frame range is outside the artifact")
	}
	if artifact.Channels != 1 && artifact.Channels != 2 {
		return nil, errors.New("unsupported artifact channel count")
	}
	pathBefore, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if pathBefore.Mode()&os.ModeSymlink != 0 || !pathBefore.Mode().IsRegular() {
		return nil, errors.New("stem artifact path is not a regular non-symlink file")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open stem artifact: %w", err)
	}
	defer f.Close()
	before, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !before.Mode().IsRegular() || before.Size() != artifact.SizeBytes {
		return nil, errors.New("stem artifact size or file type changed")
	}
	if !os.SameFile(pathBefore, before) {
		return nil, errors.New("stem artifact path changed before open")
	}

	dataOffset, dataBytes, geometry, encoding, err := wavDataRange(f)
	if err != nil {
		return nil, err
	}
	if geometry.SampleRate != artifact.SampleRate || geometry.Channels != artifact.Channels || geometry.Frames != artifact.Frames || encoding != artifact.Encoding {
		return nil, errors.New("stem WAV geometry or encoding changed")
	}
	bytesPerSample := int64(0)
	switch artifact.Encoding {
	case "pcm_s16le":
		bytesPerSample = 2
	case "float32le":
		bytesPerSample = 4
	default:
		return nil, errors.New("unsupported stem encoding")
	}
	frameBytes := int64(artifact.Channels) * bytesPerSample
	if dataBytes != uint64(artifact.Frames*frameBytes) {
		return nil, errors.New("stem WAV frame geometry changed")
	}
	byteCount := frameCount * frameBytes
	if byteCount > int64(int(^uint(0)>>1)) {
		return nil, errors.New("requested frame range is too large")
	}
	if _, err = f.Seek(dataOffset+startFrame*frameBytes, io.SeekStart); err != nil {
		return nil, err
	}
	data := make([]byte, int(byteCount))
	if _, err = io.ReadFull(f, data); err != nil {
		return nil, fmt.Errorf("read stem frames: %w", err)
	}
	after, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if after.Size() != before.Size() || !after.ModTime().Equal(before.ModTime()) {
		return nil, errors.New("stem artifact changed while reading frames")
	}
	pathAfter, err := os.Lstat(path)
	if err != nil || pathAfter.Mode()&os.ModeSymlink != 0 || !os.SameFile(after, pathAfter) {
		return nil, errors.New("stem artifact path changed while reading frames")
	}

	samples := make([]float32, int(frameCount)*artifact.Channels)
	if artifact.Encoding == "pcm_s16le" {
		for i := range samples {
			samples[i] = float32(int16(binary.LittleEndian.Uint16(data[i*2:i*2+2]))) / 32768
		}
	} else {
		for i := range samples {
			v := math.Float32frombits(binary.LittleEndian.Uint32(data[i*4 : i*4+4]))
			if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
				return nil, errors.New("stem artifact contains non-finite samples")
			}
			samples[i] = v
		}
	}
	return samples, nil
}

func wavDataRange(f *os.File) (int64, uint64, AudioGeometry, string, error) {
	var geometry AudioGeometry
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return 0, 0, geometry, "", err
	}
	var head [12]byte
	if _, err := io.ReadFull(f, head[:]); err != nil {
		return 0, 0, geometry, "", err
	}
	if string(head[:4]) != "RIFF" || string(head[8:]) != "WAVE" {
		return 0, 0, geometry, "", errors.New("stem artifact is not RIFF/WAVE")
	}
	end := int64(binary.LittleEndian.Uint32(head[4:8])) + 8
	info, err := f.Stat()
	if err != nil {
		return 0, 0, geometry, "", err
	}
	if end < 12 || end > info.Size() {
		return 0, 0, geometry, "", errors.New("invalid WAV RIFF boundary")
	}
	var haveFormat, found bool
	var offset int64
	var length uint32
	var audioFormat, bits, blockAlign uint16
	for {
		pos, err := f.Seek(0, io.SeekCurrent)
		if err != nil {
			return 0, 0, geometry, "", err
		}
		if pos == end {
			break
		}
		if pos+8 > end {
			return 0, 0, geometry, "", errors.New("truncated WAV chunk header")
		}
		var chunk [8]byte
		if _, err = io.ReadFull(f, chunk[:]); err != nil {
			return 0, 0, geometry, "", err
		}
		size := binary.LittleEndian.Uint32(chunk[4:])
		next := pos + 8 + int64(size) + int64(size%2)
		if next > end {
			return 0, 0, geometry, "", errors.New("WAV chunk exceeds RIFF boundary")
		}
		switch string(chunk[:4]) {
		case "fmt ":
			if haveFormat || size < 16 {
				return 0, 0, geometry, "", errors.New("invalid WAV fmt chunk")
			}
			var b [16]byte
			if _, err = io.ReadFull(f, b[:]); err != nil {
				return 0, 0, geometry, "", err
			}
			audioFormat = binary.LittleEndian.Uint16(b[:2])
			geometry.Channels = int(binary.LittleEndian.Uint16(b[2:4]))
			geometry.SampleRate = int(binary.LittleEndian.Uint32(b[4:8]))
			blockAlign = binary.LittleEndian.Uint16(b[12:14])
			bits = binary.LittleEndian.Uint16(b[14:16])
			haveFormat = true
		case "data":
			if found {
				return 0, 0, geometry, "", errors.New("multiple WAV data chunks are unsupported")
			}
			found = true
			offset = pos + 8
			length = size
		}
		if _, err = f.Seek(next, io.SeekStart); err != nil {
			return 0, 0, geometry, "", err
		}
	}
	if !haveFormat || !found {
		return 0, 0, geometry, "", errors.New("WAV requires fmt and data chunks")
	}
	if geometry.SampleRate <= 0 || (geometry.Channels != 1 && geometry.Channels != 2) {
		return 0, 0, geometry, "", errors.New("invalid WAV geometry")
	}
	bytesPer := uint16(0)
	encoding := ""
	if audioFormat == 1 && bits == 16 {
		bytesPer = 2
		encoding = "pcm_s16le"
	} else if audioFormat == 3 && bits == 32 {
		bytesPer = 4
		encoding = "float32le"
	} else {
		return 0, 0, geometry, "", errors.New("unsupported WAV encoding")
	}
	if blockAlign != uint16(geometry.Channels)*bytesPer || length == 0 || length%uint32(blockAlign) != 0 {
		return 0, 0, geometry, "", errors.New("WAV data is empty or not frame aligned")
	}
	geometry.Frames = int64(length / uint32(blockAlign))
	return offset, uint64(length), geometry, encoding, nil
}
