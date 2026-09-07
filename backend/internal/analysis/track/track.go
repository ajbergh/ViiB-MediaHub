// Package track runs every scalar analyzer over one decoded PCM pass and
// persists a single combined result. Running tempo and key independently
// against the same row makes each one overwrite the other's status and
// algorithm version, so the durable record could claim "complete" while one
// dimension was never measured.
package track

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/analysis"
	"github.com/ajbergh/viib-mediahub/internal/analysis/key"
	"github.com/ajbergh/viib-mediahub/internal/analysis/tempo"
	"github.com/ajbergh/viib-mediahub/internal/db"
)

// AnalysisVersion is the scalar-result schema version. Increment it when the
// meaning of a persisted column changes, which makes existing rows stale.
const AnalysisVersion = 1

// AlgorithmVersion identifies the exact analyzer combination that produced a
// row. It is composite because one row carries both dimensions; a change in
// either analyzer must invalidate the record.
const AlgorithmVersion = "track-v1;" + tempo.AlgorithmVersion + ";" + key.AlgorithmVersion

// Stable failure codes from the analysis lifecycle contract. They are part of
// the persisted record and must not be reworded per call site.
const (
	ErrorNoReliableTempo   = "no_reliable_tempo"
	ErrorNoReliableKey     = "no_reliable_key"
	ErrorInsufficientAudio = "insufficient_audio"
	ErrorSourceUnavailable = "source_unavailable"
	ErrorUnsupportedCodec  = "unsupported_codec"
	ErrorDecodeFailed      = "decode_failed"
	ErrorCanceled          = "canceled"
)

// Result is the combined outcome of one analysis pass. Tempo and key carry
// their own Known flags, so an unmeasured dimension is never mistaken for zero.
type Result struct {
	SongID string
	Status string
	Tempo  tempo.Estimate
	Key    key.Estimate
	Source analysis.ResolvedSource
}

// Options selects analyzer priors for a pass.
type Options struct {
	Tempo tempo.Options
}

// DefaultOptions uses the standard DJ tempo priors.
func DefaultOptions() Options { return Options{Tempo: tempo.DefaultOptions()} }

// Analyze streams a canonical local song once, feeding both accumulators from
// the same borrowed PCM chunks, and returns the combined result without
// persisting it.
func Analyze(ctx context.Context, database *db.DB, registry *analysis.DecoderRegistry, songID string, opts Options) (Result, error) {
	var onsets *tempo.OnsetAccumulator
	var chroma *key.ChromaAccumulator
	sampleRate := 0

	source, err := analysis.StreamLocalMono(ctx, database, registry, songID, func(chunk analysis.MonoChunk) error {
		if sampleRate == 0 {
			sampleRate = chunk.SampleRate
			onsets = tempo.NewOnsetAccumulatorWithOptions(chunk.SampleRate, opts.Tempo)
			chroma = key.NewChromaAccumulator(chunk.SampleRate)
		}
		if sampleRate != chunk.SampleRate {
			return fmt.Errorf("analysis stream sample rate changed")
		}
		onsets.Feed(chunk.Samples)
		chroma.Feed(chunk.Samples)
		return nil
	})
	if err != nil {
		return Result{SongID: songID}, err
	}

	result := Result{SongID: songID, Source: source}
	if onsets == nil {
		// A decodable source that yielded no PCM at all.
		result.Tempo = tempo.Estimate{AlgorithmVersion: tempo.AlgorithmVersion}
		result.Key = key.Estimate{AlgorithmVersion: key.AlgorithmVersion}
	} else {
		result.Tempo = onsets.Estimate()
		result.Key = chroma.Estimate()
	}
	result.Status = combinedStatus(result.Tempo.Known, result.Key.Known)
	return result, nil
}

// combinedStatus reports what the pass actually measured. Only a pass that
// measured both dimensions is complete.
func combinedStatus(tempoKnown, keyKnown bool) string {
	switch {
	case tempoKnown && keyKnown:
		return db.TrackAnalysisComplete
	case tempoKnown || keyKnown:
		return db.TrackAnalysisPartial
	default:
		return db.TrackAnalysisFailed
	}
}

// AnalyzeAndPersist runs one pass and writes both dimensions in a single
// record, so neither analyzer can clobber the other's provenance.
func AnalyzeAndPersist(ctx context.Context, database *db.DB, registry *analysis.DecoderRegistry, songID string, opts Options) (Result, error) {
	result, err := Analyze(ctx, database, registry, songID, opts)
	if err != nil {
		return result, err
	}
	if err := Persist(database, result); err != nil {
		return result, err
	}
	return result, nil
}

// Persist writes one combined scalar record. Measured values are always
// recorded; manual locks take precedence at read time through
// db.ResolveEffectiveBPM rather than by suppressing measurement here.
func Persist(database *db.DB, result Result) error {
	record := db.TrackAnalysis{
		SongID:            result.SongID,
		Status:            result.Status,
		AnalysisVersion:   AnalysisVersion,
		AlgorithmVersion:  AlgorithmVersion,
		SourceFingerprint: result.Source.Fingerprint,
		SourceSize:        &result.Source.Size,
		SourceMtime:       &result.Source.Mtime,
		AnalyzedAt:        ptr(time.Now().UnixMilli()),
	}
	if result.Tempo.Known {
		record.BPM = &result.Tempo.BPM
		record.BPMConfidence = &result.Tempo.Confidence
		if result.Tempo.Alternate > 0 {
			record.BPMAltCandidate = &result.Tempo.Alternate
		}
		record.TempoStability = &result.Tempo.Stability
		record.BPMSource = ptr("measured")
		kind := "static"
		if result.Tempo.Stability < 0.85 {
			kind = "dynamic-candidate"
		}
		record.TempoKind = &kind
	} else {
		record.TempoKind = ptr("unknown")
	}
	if result.Key.Known {
		record.KeyTonic = &result.Key.Tonic
		record.KeyMode = &result.Key.Mode
		record.KeyConfidence = &result.Key.Confidence
		record.CamelotKey = &result.Key.Camelot
		record.OpenKey = &result.Key.OpenKey
		record.KeySource = ptr("measured")
	}
	switch {
	case !result.Tempo.Known && !result.Key.Known:
		record.ErrorCode = ptr(ErrorInsufficientAudio)
		record.ErrorMessage = ptr("No reliable tempo or key evidence in the decoded audio")
	case !result.Tempo.Known:
		record.ErrorCode = ptr(ErrorNoReliableTempo)
		record.ErrorMessage = ptr("No reliable tempo evidence in the decoded audio")
	case !result.Key.Known:
		record.ErrorCode = ptr(ErrorNoReliableKey)
		record.ErrorMessage = ptr("No reliable tonal evidence in the decoded audio")
	}
	return database.UpsertTrackAnalysis(record)
}

// PersistFailure records a durable terminal failure so a poison source is not
// retried on every pass. The fingerprint is required by the record contract, so
// an unresolvable source is recorded against a synthetic unavailable marker.
func PersistFailure(database *db.DB, songID string, source analysis.ResolvedSource, code, message string) error {
	fingerprint := source.Fingerprint
	if fingerprint == "" {
		fingerprint = "unresolved:" + songID
	}
	record := db.TrackAnalysis{
		SongID:            songID,
		Status:            db.TrackAnalysisFailed,
		AnalysisVersion:   AnalysisVersion,
		AlgorithmVersion:  AlgorithmVersion,
		SourceFingerprint: fingerprint,
		AnalyzedAt:        ptr(time.Now().UnixMilli()),
		ErrorCode:         &code,
		ErrorMessage:      &message,
	}
	if code == ErrorUnsupportedCodec {
		record.Status = db.TrackAnalysisUnsupported
	}
	return database.UpsertTrackAnalysis(record)
}

// ClassifyError maps a pass error onto a stable persisted failure code.
func ClassifyError(err error) (string, string) {
	if err == nil {
		return "", ""
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return ErrorCanceled, err.Error()
	}
	if errors.Is(err, analysis.ErrUnsupportedCodec) {
		return ErrorUnsupportedCodec, err.Error()
	}
	message := err.Error()
	for _, marker := range []string{"no local analysis source", "stat local source", "not a regular file", "Plex source adapter", "open local source"} {
		if strings.Contains(message, marker) {
			return ErrorSourceUnavailable, message
		}
	}
	return ErrorDecodeFailed, message
}

func ptr[T any](value T) *T { return &value }
