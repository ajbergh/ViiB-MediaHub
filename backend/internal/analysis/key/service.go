package key

import (
	"context"
	"fmt"

	"github.com/ajbergh/viib-mediahub/internal/analysis"
	"github.com/ajbergh/viib-mediahub/internal/db"
)

// EstimateLocalSong streams a canonical local song through the shared source,
// decoder, and downmix path. Raw PCM is never retained beyond accumulator
// overlap, while the returned source fingerprint can be persisted with result.
func EstimateLocalSong(ctx context.Context, database *db.DB, registry *analysis.DecoderRegistry, songID string) (Estimate, analysis.ResolvedSource, error) {
	var accumulator *ChromaAccumulator
	source, err := analysis.StreamLocalMono(ctx, database, registry, songID, func(chunk analysis.MonoChunk) error {
		if accumulator == nil {
			accumulator = NewChromaAccumulator(chunk.SampleRate)
		}
		if accumulator.sampleRate != chunk.SampleRate {
			return fmt.Errorf("analysis stream sample rate changed")
		}
		accumulator.Feed(chunk.Samples)
		return nil
	})
	if err != nil {
		return Estimate{AlgorithmVersion: AlgorithmVersion}, analysis.ResolvedSource{}, err
	}
	if accumulator == nil {
		return Estimate{AlgorithmVersion: AlgorithmVersion}, source, nil
	}
	return accumulator.Estimate(), source, nil
}

// Persistence deliberately does not live here. One track_analysis row carries
// both key and tempo, so a key-only writer has to read-modify-write the tempo
// columns to avoid destroying them — which races another analyzer and leaves
// algorithm_version naming only one of the two analyzers that produced the row.
// analysis/track owns the combined single-pass write instead.
