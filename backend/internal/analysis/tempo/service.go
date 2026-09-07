package tempo

import (
	"context"
	"fmt"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/analysis"
	"github.com/ajbergh/viib-mediahub/internal/db"
)

// EstimateLocalSong streams a canonical local song through the shared source,
// decoder, and downmix path. Raw PCM is never retained beyond accumulator
// overlap, while the returned source fingerprint can be persisted with result.
func EstimateLocalSong(ctx context.Context, database *db.DB, registry *analysis.DecoderRegistry, songID string) (Estimate, analysis.ResolvedSource, error) {
	var accumulator *OnsetAccumulator
	source, err := analysis.StreamLocalMono(ctx, database, registry, songID, func(chunk analysis.MonoChunk) error {
		if accumulator == nil {
			accumulator = NewOnsetAccumulator(chunk.SampleRate)
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

// AnalyzeAndPersistLocalSong records the measured result separately from
// legacy song metadata. Unknown output is durable and explicit, so callers do
// not retry forever or convert a lack of evidence into a default BPM.
func AnalyzeAndPersistLocalSong(ctx context.Context, database *db.DB, registry *analysis.DecoderRegistry, songID string, analysisVersion int) (Estimate, error) {
	estimate, source, err := EstimateLocalSong(ctx, database, registry, songID)
	if err != nil {
		return estimate, err
	}
	record := db.TrackAnalysis{SongID: songID, AnalysisVersion: analysisVersion, AlgorithmVersion: estimate.AlgorithmVersion, SourceFingerprint: source.Fingerprint, SourceSize: &source.Size, SourceMtime: &source.Mtime, AnalyzedAt: ptr(time.Now().UnixMilli())}
	if estimate.Known {
		record.Status, record.BPM, record.BPMConfidence = db.TrackAnalysisComplete, &estimate.BPM, &estimate.Confidence
		sourceName, tempoKind := "measured", "static"
		record.BPMSource, record.TempoKind = &sourceName, &tempoKind
	} else {
		record.Status = db.TrackAnalysisPartial
		tempoKind := "unknown"
		record.TempoKind = &tempoKind
	}
	if err := database.UpsertTrackAnalysis(record); err != nil {
		return estimate, err
	}
	return estimate, nil
}

func ptr[T any](value T) *T { return &value }
