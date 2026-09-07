package key

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

// AnalyzeAndPersistLocalSong records the measured musical key separately from
// legacy song metadata. Unknown output is durable and explicit, so callers do
// not retry forever or convert a lack of evidence into a default key.
// Any pre-existing tempo analysis on the same source is preserved.
func AnalyzeAndPersistLocalSong(ctx context.Context, database *db.DB, registry *analysis.DecoderRegistry, songID string, analysisVersion int) (Estimate, error) {
	estimate, source, err := EstimateLocalSong(ctx, database, registry, songID)
	if err != nil {
		return estimate, err
	}
	record := db.TrackAnalysis{
		SongID:            songID,
		AnalysisVersion:   analysisVersion,
		AlgorithmVersion:  estimate.AlgorithmVersion,
		SourceFingerprint: source.Fingerprint,
		SourceSize:        &source.Size,
		SourceMtime:       &source.Mtime,
		AnalyzedAt:        ptr(time.Now().UnixMilli()),
	}
	// Preserve existing tempo analysis if source fingerprint matches
	if existing, err := database.GetTrackAnalysis(songID); err == nil && existing.SourceFingerprint == source.Fingerprint {
		record.BPM = existing.BPM
		record.BPMConfidence = existing.BPMConfidence
		record.BPMAltCandidate = existing.BPMAltCandidate
		record.BPMSource = existing.BPMSource
		record.TempoStability = existing.TempoStability
		record.TempoKind = existing.TempoKind
	}
	if estimate.Known {
		record.Status = db.TrackAnalysisComplete
		record.KeyTonic = &estimate.Tonic
		record.KeyMode = &estimate.Mode
		record.KeyConfidence = &estimate.Confidence
		record.CamelotKey = &estimate.Camelot
		record.OpenKey = &estimate.OpenKey
		sourceName := "measured"
		record.KeySource = &sourceName
	} else {
		record.Status = db.TrackAnalysisPartial
	}
	if err := database.UpsertTrackAnalysis(record); err != nil {
		return estimate, err
	}
	return estimate, nil
}

func ptr[T any](value T) *T { return &value }
