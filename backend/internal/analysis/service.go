package analysis

import (
	"context"
	"fmt"
	"io"

	"github.com/ajbergh/viib-mediahub/internal/db"
)

// MonoChunk is a borrowed PCM view valid only for the duration of the callback.
// It prevents whole-track retention in the shared analysis pipeline.
type MonoChunk struct {
	SampleRate int
	Samples    []float32
}

// StreamLocalMono resolves, decodes, and downmixes a canonical local song in
// bounded chunks. A later resampling stage can consume exactly this stream
// without adding a second decoder path.
func StreamLocalMono(ctx context.Context, database *db.DB, registry *DecoderRegistry, songID string, consume func(MonoChunk) error) (ResolvedSource, error) {
	if registry == nil || consume == nil {
		return ResolvedSource{}, fmt.Errorf("analysis stream requires decoder registry and consumer")
	}
	source, err := ResolveLocalSource(database, songID)
	if err != nil {
		return ResolvedSource{}, err
	}
	reader, err := source.Open()
	if err != nil {
		return ResolvedSource{}, fmt.Errorf("open local source: %w", err)
	}
	stream, err := registry.Open(ctx, source.Name, reader)
	if err != nil {
		reader.Close()
		return ResolvedSource{}, err
	}
	defer stream.Close()
	info := stream.Info()
	if info.Channels <= 0 || info.SampleRate <= 0 {
		return ResolvedSource{}, fmt.Errorf("decoder returned invalid PCM geometry")
	}
	interleaved := make([]float32, 8192*info.Channels)
	for {
		if err := ctx.Err(); err != nil {
			return ResolvedSource{}, err
		}
		count, readErr := stream.Read(ctx, interleaved)
		if count > 0 {
			if count%info.Channels != 0 {
				return ResolvedSource{}, fmt.Errorf("decoder returned incomplete PCM frame")
			}
			mono, err := DownmixInterleaved(interleaved[:count], info.Channels)
			if err != nil {
				return ResolvedSource{}, err
			}
			if err := consume(MonoChunk{SampleRate: info.SampleRate, Samples: mono}); err != nil {
				return ResolvedSource{}, err
			}
		}
		if readErr == io.EOF {
			return source, nil
		}
		if readErr != nil {
			return ResolvedSource{}, readErr
		}
		if count == 0 {
			return ResolvedSource{}, fmt.Errorf("decoder returned no samples without EOF")
		}
	}
}
