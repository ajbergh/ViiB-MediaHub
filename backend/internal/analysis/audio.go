// Package analysis contains reusable, backend-owned track analysis primitives.
package analysis

import (
	"context"
	"errors"
	"io"
	"path/filepath"
	"strings"
)

var ErrUnsupportedCodec = errors.New("analysis decoder is not available for this format")

// PCMInfo describes normalized interleaved float32 PCM. Samples must be in
// [-1, 1], with bounded reads supplied by PCMStream.Read.
type PCMInfo struct {
	SampleRate int
	Channels   int
}

// PCMStream is the shared decoder output used by tempo, key, waveform, and
// later structure analysis. A caller owns Close and must stop on context
// cancellation between chunks.
type PCMStream interface {
	Info() PCMInfo
	Read(context.Context, []float32) (int, error)
	Close() error
}

// Decoder opens one source into normalized PCM. The registry deliberately
// keeps container/codec ownership at this boundary rather than in analyzers.
type Decoder interface {
	ID() string
	Open(context.Context, io.ReadCloser) (PCMStream, error)
}

// DecoderRegistry selects decoders by case-insensitive filename extension.
// It is constructed explicitly so tests and future source adapters can choose
// a known capability set without global mutable registrations.
type DecoderRegistry struct{ byExtension map[string]Decoder }

func NewDecoderRegistry() *DecoderRegistry {
	return &DecoderRegistry{byExtension: make(map[string]Decoder)}
}

func (r *DecoderRegistry) Register(extensions []string, decoder Decoder) error {
	if decoder == nil || decoder.ID() == "" || len(extensions) == 0 {
		return errors.New("decoder registration requires ID and extension")
	}
	for _, extension := range extensions {
		normalized := normalizeExtension(extension)
		if normalized == "" || r.byExtension[normalized] != nil {
			return errors.New("invalid or duplicate decoder extension")
		}
		r.byExtension[normalized] = decoder
	}
	return nil
}

func (r *DecoderRegistry) Open(ctx context.Context, name string, source io.ReadCloser) (PCMStream, error) {
	decoder := r.byExtension[normalizeExtension(filepath.Ext(name))]
	if decoder == nil {
		return nil, ErrUnsupportedCodec
	}
	return decoder.Open(ctx, source)
}

func normalizeExtension(extension string) string {
	extension = strings.TrimSpace(strings.ToLower(extension))
	if extension == "" {
		return ""
	}
	if !strings.HasPrefix(extension, ".") {
		extension = "." + extension
	}
	return extension
}
