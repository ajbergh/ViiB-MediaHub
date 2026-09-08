package analysis

import (
	"context"
	"fmt"
	"io"
	"path/filepath"

	"github.com/ajbergh/viib-mediahub/internal/db"
)

// MonoChunk is a borrowed PCM view valid only for the duration of the callback.
// It prevents whole-track retention in the shared analysis pipeline.
type MonoChunk struct {
	SampleRate int
	Samples    []float32
}

// decodeChunkFrames bounds one decode/downmix step so memory stays
// O(chunk + compact feature timeline) rather than O(track length).
const decodeChunkFrames = 8192

// StreamLocalMono resolves, decodes, and downmixes a canonical local song in
// bounded chunks. A later resampling stage can consume exactly this stream
// without adding a second decoder path.
func StreamLocalMono(ctx context.Context, database *db.DB, registry *DecoderRegistry, songID string, consume func(MonoChunk) error) (ResolvedSource, error) {
	source, err := ResolveLocalSource(database, songID)
	if err != nil {
		return ResolvedSource{}, err
	}
	if err := streamMono(ctx, registry, source.Name, source.Open, consume); err != nil {
		return ResolvedSource{}, err
	}
	return source, nil
}

// StreamMonoFile decodes one audio file by path through the same decoder,
// downmix, and bounded-chunk path used for canonical song analysis. It exists
// so derived overviews such as the DJ waveform cannot drift onto a second
// decoder implementation with its own codec matrix.
func StreamMonoFile(ctx context.Context, registry *DecoderRegistry, path string, consume func(MonoChunk) error) error {
	source := ResolvedSource{Name: filepath.Base(path), Path: path}
	return streamMono(ctx, registry, source.Name, source.Open, consume)
}

// streamMono is the single decode loop shared by every entry point. Callers
// supply the source name used for codec selection and an opener, so source
// resolution policy stays out of the decode path.
func streamMono(ctx context.Context, registry *DecoderRegistry, name string, open func() (io.ReadCloser, error), consume func(MonoChunk) error) error {
	if registry == nil || consume == nil {
		return fmt.Errorf("analysis stream requires decoder registry and consumer")
	}
	// Reject an unsupported codec before opening the file. Opening first would
	// report a missing file as an unsupported format and an unsupported format
	// as a source error, which are different lifecycle outcomes.
	if !registry.Supports(name) {
		return ErrUnsupportedCodec
	}
	reader, err := open()
	if err != nil {
		return fmt.Errorf("open local source: %w", err)
	}
	stream, err := registry.Open(ctx, name, reader)
	if err != nil {
		reader.Close()
		return err
	}
	defer stream.Close()
	info := stream.Info()
	if info.Channels <= 0 || info.SampleRate <= 0 {
		return fmt.Errorf("decoder returned invalid PCM geometry")
	}
	interleaved := make([]float32, decodeChunkFrames*info.Channels)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		count, readErr := stream.Read(ctx, interleaved)
		if count > 0 {
			if count%info.Channels != 0 {
				return fmt.Errorf("decoder returned incomplete PCM frame")
			}
			mono, err := DownmixInterleaved(interleaved[:count], info.Channels)
			if err != nil {
				return err
			}
			if err := consume(MonoChunk{SampleRate: info.SampleRate, Samples: mono}); err != nil {
				return err
			}
		}
		if readErr == io.EOF {
			return nil
		}
		if readErr != nil {
			return readErr
		}
		if count == 0 {
			return fmt.Errorf("decoder returned no samples without EOF")
		}
	}
}
