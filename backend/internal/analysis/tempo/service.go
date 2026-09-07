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
	return EstimateLocalSongWithOptions(ctx, database, registry, songID, DefaultOptions())
}

// EstimateLocalSongWithOptions streams a local song with explicit candidate range priors.
func EstimateLocalSongWithOptions(ctx context.Context, database *db.DB, registry *analysis.DecoderRegistry, songID string, opts Options) (Estimate, analysis.ResolvedSource, error) {
	var accumulator *OnsetAccumulator
	source, err := analysis.StreamLocalMono(ctx, database, registry, songID, func(chunk analysis.MonoChunk) error {
		if accumulator == nil {
			accumulator = NewOnsetAccumulatorWithOptions(chunk.SampleRate, opts)
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
	return AnalyzeAndPersistLocalSongWithOptions(ctx, database, registry, songID, analysisVersion, DefaultOptions())
}

// AnalyzeAndPersistLocalSongWithOptions estimates tempo with range priors and persists
// measured scalar facts, alternate metrical candidate, stability, and provenance.
func AnalyzeAndPersistLocalSongWithOptions(ctx context.Context, database *db.DB, registry *analysis.DecoderRegistry, songID string, analysisVersion int, opts Options) (Estimate, error) {
	estimate, source, err := EstimateLocalSongWithOptions(ctx, database, registry, songID, opts)
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
	if existing, err := database.GetTrackAnalysis(songID); err == nil && existing.SourceFingerprint == source.Fingerprint {
		record.KeyTonic = existing.KeyTonic
		record.KeyMode = existing.KeyMode
		record.KeyConfidence = existing.KeyConfidence
		record.KeySource = existing.KeySource
		record.CamelotKey = existing.CamelotKey
		record.OpenKey = existing.OpenKey
	}
	if estimate.Known {
		record.Status = db.TrackAnalysisComplete
		record.BPM = &estimate.BPM
		record.BPMConfidence = &estimate.Confidence
		if estimate.Alternate > 0 {
			record.BPMAltCandidate = &estimate.Alternate
		}
		record.TempoStability = &estimate.Stability
		sourceName := "measured"
		record.BPMSource = &sourceName
		tempoKind := "static"
		if estimate.Stability < 0.85 {
			tempoKind = "dynamic-candidate"
		}
		record.TempoKind = &tempoKind
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
