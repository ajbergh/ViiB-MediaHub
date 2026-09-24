package analysis

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sync"
)

// ErrCanonicalAudioGeometryUnsupported means the current decoder cannot
// reproduce the requested package geometry without an unspecified resampler
// or channel mapping. Callers should treat an audio identity fallback as
// unavailable and apply the package's full-file hash rule.
var ErrCanonicalAudioGeometryUnsupported = errors.New("canonical audio hash requires matching native sample rate and channels")

type sourceAudioHashKey struct {
	path       string
	size       int64
	mtimeNanos int64
	rate       int
	channels   int
}

// SourceAudioHashCache caches complete canonical source PCM digests. Geometry
// is part of the key because the package contract hashes at its declared rate
// and channel count.
type SourceAudioHashCache struct {
	mu      sync.Mutex
	entries map[sourceAudioHashKey]string
}

func NewSourceAudioHashCache() *SourceAudioHashCache {
	return &SourceAudioHashCache{entries: make(map[sourceAudioHashKey]string)}
}

// SHA256 hashes a complete local source as interleaved signed int32 LE PCM.
// The current decoders expose native-rate/native-channel float32; this method
// therefore refuses geometry conversion instead of choosing undocumented
// resampling or up/down-mix behavior.
func (c *SourceAudioHashCache) SHA256(ctx context.Context, registry *DecoderRegistry, source ResolvedSource, sampleRate, channels int) (string, error) {
	if c == nil {
		return "", errors.New("nil source audio hash cache")
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	name := source.Name
	if name == "" {
		name = source.Path
	}
	if registry == nil || !registry.Supports(name) {
		return "", ErrUnsupportedCodec
	}
	if sampleRate <= 0 || (channels != 1 && channels != 2) || source.Path == "" {
		return "", errors.New("invalid source audio hash geometry or path")
	}
	absolutePath, err := filepath.Abs(source.Path)
	if err != nil {
		return "", fmt.Errorf("resolve source path: %w", err)
	}
	absolutePath = filepath.Clean(absolutePath)
	before, err := os.Stat(absolutePath)
	if err != nil {
		return "", fmt.Errorf("stat source audio: %w", err)
	}
	if !before.Mode().IsRegular() {
		return "", errors.New("source audio is not a regular file")
	}
	key := sourceAudioHashKey{path: absolutePath, size: before.Size(), mtimeNanos: before.ModTime().UnixNano(), rate: sampleRate, channels: channels}
	c.mu.Lock()
	if cached, ok := c.entries[key]; ok {
		c.mu.Unlock()
		return cached, nil
	}
	c.mu.Unlock()

	file, err := os.Open(absolutePath)
	if err != nil {
		return "", fmt.Errorf("open source audio: %w", err)
	}
	openedInfo, err := file.Stat()
	if err != nil {
		file.Close()
		return "", fmt.Errorf("stat opened source audio: %w", err)
	}
	if !sameSourceRevision(before, openedInfo) || !os.SameFile(before, openedInfo) {
		file.Close()
		return "", errors.New("source audio changed before decoding")
	}
	stream, err := registry.Open(ctx, filepath.Base(absolutePath), file)
	if err != nil {
		file.Close()
		return "", fmt.Errorf("open source decoder: %w", err)
	}
	streamClosed := false
	defer func() {
		if !streamClosed {
			_ = stream.Close()
		}
	}()
	info := stream.Info()
	if info.SampleRate != sampleRate || info.Channels != channels {
		return "", ErrCanonicalAudioGeometryUnsupported
	}
	if info.Channels <= 0 {
		return "", errors.New("decoder returned invalid source channel count")
	}

	hash := sha256.New()
	const chunkValues = 8192 // divisible by mono and stereo frame widths
	samples := make([]float32, chunkValues)
	encoded := make([]byte, chunkValues*4)
	var valuesRead int64
	zeroProgressReads := 0
	for {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		count, readErr := stream.Read(ctx, samples)
		if count < 0 || count > len(samples) || count%info.Channels != 0 {
			return "", errors.New("decoder returned incomplete interleaved PCM frames")
		}
		if count > 0 {
			zeroProgressReads = 0
			for i, sample := range samples[:count] {
				if math.IsNaN(float64(sample)) || math.IsInf(float64(sample), 0) {
					return "", errors.New("decoder returned non-finite PCM samples")
				}
				value := math.Max(-1, math.Min(1, float64(sample)))
				quantized := int32(math.Round(value * 2147483647))
				binary.LittleEndian.PutUint32(encoded[i*4:], uint32(quantized))
			}
			if _, err := hash.Write(encoded[:count*4]); err != nil {
				return "", err
			}
			valuesRead += int64(count)
		} else if readErr == nil {
			zeroProgressReads++
			if zeroProgressReads >= 100 {
				return "", io.ErrNoProgress
			}
		}
		if readErr != nil {
			if !errors.Is(readErr, io.EOF) {
				return "", fmt.Errorf("decode source audio: %w", readErr)
			}
			break
		}
	}
	frames := valuesRead / int64(info.Channels)
	if valuesRead%int64(info.Channels) != 0 || frames == 0 {
		return "", errors.New("decoder returned incomplete or empty source audio")
	}
	if info.DeclaredFrames > 0 && frames != info.DeclaredFrames {
		return "", errors.New("decoder did not return its declared complete frame count")
	}
	openedAfter, err := file.Stat()
	if err != nil {
		return "", fmt.Errorf("restat decoded source: %w", err)
	}
	pathAfter, err := os.Stat(absolutePath)
	if err != nil {
		return "", fmt.Errorf("restat source path: %w", err)
	}
	if !sameSourceRevision(before, openedAfter) || !sameSourceRevision(before, pathAfter) || !os.SameFile(before, pathAfter) {
		return "", errors.New("source audio changed while decoding")
	}
	closeErr := stream.Close()
	streamClosed = true
	if closeErr != nil {
		return "", fmt.Errorf("close source decoder: %w", closeErr)
	}
	cacheInfo, err := os.Stat(absolutePath)
	if err != nil {
		return "", fmt.Errorf("confirm source audio before caching: %w", err)
	}
	if !sameSourceRevision(before, cacheInfo) || !os.SameFile(before, cacheInfo) {
		return "", errors.New("source audio changed before caching")
	}
	digest := hex.EncodeToString(hash.Sum(nil))
	c.mu.Lock()
	if c.entries == nil {
		c.entries = make(map[sourceAudioHashKey]string)
	}
	c.entries[key] = digest
	c.mu.Unlock()
	return digest, nil
}

func sameSourceRevision(a, b os.FileInfo) bool {
	return a != nil && b != nil && a.Size() == b.Size() && a.ModTime().Equal(b.ModTime())
}
