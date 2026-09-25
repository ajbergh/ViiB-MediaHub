// Package track runs every scalar analyzer over one decoded PCM pass and
// persists a single combined result. Running tempo and key independently
// against the same row makes each one overwrite the other's status and
// algorithm version, so the durable record could claim "complete" while one
// dimension was never measured.
package track

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/analysis"
	"github.com/ajbergh/viib-mediahub/internal/analysis/beatgrid"
	analysiscues "github.com/ajbergh/viib-mediahub/internal/analysis/cues"
	"github.com/ajbergh/viib-mediahub/internal/analysis/features"
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
const AlgorithmVersion = "track-v1;" + tempo.AlgorithmVersion + ";" + key.AlgorithmVersion + ";" + features.EnergyLevelAlgorithmVersion + ";" + features.BS1770AlgorithmVersion

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
	SongID          string
	Status          string
	Tempo           tempo.Estimate
	Key             key.Estimate
	DurationSeconds float64
	// BeatGrid is optional: scalar analysis remains useful when audio has no
	// sufficiently periodic onset evidence for safe phase alignment.
	BeatGrid    *beatgrid.Grid
	Features    *features.Result
	Loudness    *features.BS1770Result
	EnergyLevel *features.EnergyLevelEstimate
	Source      analysis.ResolvedSource
}

// Options selects analyzer priors for a pass.
type Options struct {
	Tempo tempo.Options
	Key   key.Options
}

// FileTiming separates decoder/streaming work from DSP work for a Phase 0
// benchmark run. It is measurement metadata, not a product performance claim.
type FileTiming struct {
	AudioSeconds           float64
	DeclaredAudioSeconds   float64
	WallSeconds            float64
	DecodeAndStreamSeconds float64
	DSPSeconds             float64
	SampleRate             int
	SourceChannels         int
}

// DefaultOptions uses the standard DJ tempo priors.
func DefaultOptions() Options {
	return Options{Tempo: tempo.DefaultOptions(), Key: key.DefaultOptions()}
}

// Analyze streams a canonical local song once, feeding both accumulators from
// the same borrowed PCM chunks, and returns the combined result without
// persisting it.
func Analyze(ctx context.Context, database *db.DB, registry *analysis.DecoderRegistry, songID string, opts Options) (Result, error) {
	source, err := analysis.ResolveLocalSource(database, songID)
	if err != nil {
		return Result{SongID: songID}, err
	}
	return AnalyzeResolved(ctx, registry, source, opts)
}

// AnalyzeResolved runs the combined analyzer over a source that has already
// been resolved by the caller. Remote source adapters use this to keep their
// authenticated stream setup outside the DSP package.
func AnalyzeResolved(ctx context.Context, registry *analysis.DecoderRegistry, source analysis.ResolvedSource, opts Options) (Result, error) {
	result, _, err := analyzeSource(ctx, registry, source.Name, source.Open, source.SongID, opts)
	result.Source = source
	return result, err
}

// AnalyzeFile runs the same combined track analyzer as Analyze without a
// catalog or persistence side effect. Phase 0 uses it for local
// manifest-referenced corpora, which must never be copied into the library.
func AnalyzeFile(ctx context.Context, registry *analysis.DecoderRegistry, path string, opts Options) (Result, FileTiming, error) {
	return analyzeSource(ctx, registry, filepath.Base(path), func() (io.ReadCloser, error) { return os.Open(path) }, filepath.Base(path), opts)
}

type sourceOpener func() (io.ReadCloser, error)

func analyzeSource(ctx context.Context, registry *analysis.DecoderRegistry, name string, open sourceOpener, songID string, opts Options) (Result, FileTiming, error) {
	started := time.Now()
	var timing FileTiming
	var onsets *tempo.OnsetAccumulator
	var chroma *key.ChromaAccumulator
	var phase *beatgrid.PhaseAccumulator
	var energy *features.Accumulator
	var loudness *features.BS1770Accumulator
	sampleRate := 0

	err := analysis.StreamMonoFileWithOpener(ctx, registry, name, open, func(chunk analysis.MonoChunk) error {
		if sampleRate == 0 {
			sampleRate = chunk.SampleRate
			timing.SampleRate = chunk.SampleRate
			timing.SourceChannels = chunk.SourceChannels
			if chunk.DeclaredFrames > 0 {
				timing.DeclaredAudioSeconds = float64(chunk.DeclaredFrames) / float64(chunk.SampleRate)
			}
			onsets = tempo.NewOnsetAccumulatorWithOptions(chunk.SampleRate, opts.Tempo)
			chroma = key.NewChromaAccumulatorWithOptions(chunk.SampleRate, opts.Key)
			phase = beatgrid.NewPhaseAccumulator(chunk.SampleRate)
			energy = features.NewAccumulator(chunk.SampleRate)
			var loudnessErr error
			loudness, loudnessErr = features.NewBS1770Accumulator(chunk.SampleRate, chunk.SourceChannels)
			if loudnessErr != nil {
				return loudnessErr
			}
		}
		if sampleRate != chunk.SampleRate {
			return fmt.Errorf("analysis stream sample rate changed")
		}
		timing.AudioSeconds += float64(len(chunk.Samples)) / float64(chunk.SampleRate)
		dspStarted := time.Now()
		onsets.Feed(chunk.Samples)
		chroma.Feed(chunk.Samples)
		phase.Feed(chunk.Samples)
		energy.Feed(chunk.Samples)
		if err := loudness.Feed(chunk.Interleaved); err != nil {
			return err
		}
		timing.DSPSeconds += time.Since(dspStarted).Seconds()
		return nil
	})
	if err != nil {
		finishTiming(&timing, started)
		return Result{SongID: songID}, timing, err
	}

	result := Result{SongID: songID, DurationSeconds: timing.AudioSeconds}
	dspStarted := time.Now()
	if onsets == nil {
		// A decodable source that yielded no PCM at all.
		result.Tempo = tempo.Estimate{AlgorithmVersion: tempo.AlgorithmVersion}
		result.Key = key.Estimate{AlgorithmVersion: key.AlgorithmVersion}
	} else {
		measuredLoudness := loudness.Result()
		result.Loudness = &measuredLoudness
		result.Tempo = onsets.Estimate()
		result.Key = chroma.Estimate()
		if result.Tempo.Known {
			if grid, gridErr := phase.Build(result.Tempo.BPM, timing.AudioSeconds, 4); gridErr == nil {
				result.BeatGrid = &grid
			}
		}
		if measured, featureErr := energy.Result(); featureErr == nil {
			measured.AnnotateStructure(result.BeatGrid)
			measured.AddCueSuggestions(result.BeatGrid)
			result.Features = &measured
			hasSignal := false
			for _, point := range measured.Energy {
				if point.Value > 0 {
					hasSignal = true
					break
				}
			}
			if level, ok := features.EstimateEnergyLevel(features.EnergyLevelInputs{LoudnessProxyDB: measured.IntegratedLUFS, PeakDBFS: measured.TruePeakDBFS, OnsetCrestFactor: result.Tempo.OnsetCrestFactor, HasAudio: hasSignal}); ok {
				result.EnergyLevel = &level
			}
		}
	}
	timing.DSPSeconds += time.Since(dspStarted).Seconds()
	result.Status = combinedStatus(result.Tempo.Known, result.Key.Known)
	finishTiming(&timing, started)
	return result, timing, nil
}

func finishTiming(timing *FileTiming, started time.Time) {
	timing.WallSeconds = time.Since(started).Seconds()
	timing.DecodeAndStreamSeconds = timing.WallSeconds - timing.DSPSeconds
	if timing.DecodeAndStreamSeconds < 0 {
		timing.DecodeAndStreamSeconds = 0
	}
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
	return AnalyzeAndPersistWithAutoCueMode(ctx, database, registry, songID, opts, db.AutomaticCuePointsFillEmpty)
}

// AnalyzeAndPersistWithAutoCueMode is the mode-aware variant used by durable
// jobs. The legacy public wrapper above intentionally remains fill-empty.
func AnalyzeAndPersistWithAutoCueMode(ctx context.Context, database *db.DB, registry *analysis.DecoderRegistry, songID string, opts Options, autoCueMode db.AutomaticCuePointMode) (Result, error) {
	result, err := Analyze(ctx, database, registry, songID, opts)
	if err != nil {
		return result, err
	}
	if err := PersistWithAutoCueMode(database, result, autoCueMode); err != nil {
		return result, err
	}
	return result, nil
}

// Persist writes one combined scalar record. Measured values are always
// recorded; manual locks take precedence at read time through
// db.ResolveEffectiveBPM rather than by suppressing measurement here.
func Persist(database *db.DB, result Result) error {
	return PersistWithAutoCueMode(database, result, db.AutomaticCuePointsFillEmpty)
}

// PersistWithAutoCueMode writes analysis artifacts and applies generated cues
// according to the snapshotted installation preference. Suggest/off retain the
// artifacts needed for candidate GETs but do not persist new generated cues.
func PersistWithAutoCueMode(database *db.DB, result Result, autoCueMode db.AutomaticCuePointMode) error {
	autoCueMode = db.NormalizeAutomaticCuePointMode(string(autoCueMode))
	record := db.TrackAnalysis{
		SongID:            result.SongID,
		Status:            result.Status,
		AnalysisVersion:   AnalysisVersion,
		AlgorithmVersion:  AlgorithmVersion,
		SourceFingerprint: result.Source.Fingerprint,
		AnalyzedAt:        ptr(time.Now().UnixMilli()),
	}
	if result.EnergyLevel != nil {
		record.EnergyLevel = &result.EnergyLevel.Level
		record.EnergyLevelConfidence = &result.EnergyLevel.Confidence
		record.EnergyAlgorithmVersion = &result.EnergyLevel.AlgorithmVersion
	}
	if result.Source.Size > 0 {
		record.SourceSize = ptr(result.Source.Size)
	}
	if result.Source.Mtime > 0 {
		record.SourceMtime = ptr(result.Source.Mtime)
	}
	if result.Source.SourceRevision != "" {
		record.SourceRevision = ptr(result.Source.SourceRevision)
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
	if code, message := resultIssue(result); code != "" {
		record.ErrorCode = &code
		record.ErrorMessage = &message
	}
	if err := database.UpsertTrackAnalysis(record); err != nil {
		return err
	}
	if err := persistBeatGrid(database, result); err != nil {
		return err
	}
	if err := persistFeatures(database, result); err != nil {
		return err
	}
	if autoCueMode != db.AutomaticCuePointsOff && autoCueMode != db.AutomaticCuePointsSuggest && result.Features != nil && result.DurationSeconds > 0 && result.Source.Fingerprint != "" {
		generated, err := analysiscues.Generate(result.DurationSeconds, result.BeatGrid, *result.Features, result.Source.Fingerprint)
		if err != nil {
			return fmt.Errorf("generate DJ hot cues: %w", err)
		}
		hotCues := make([]db.DJHotCue, 0, len(generated))
		for _, cue := range generated {
			confidence := cue.Confidence
			hotCues = append(hotCues, db.DJHotCue{
				Slot: cue.Slot, Position: cue.Position, Label: cue.Label, Color: cue.Color,
				Origin: cue.Origin, GeneratorVersion: cue.GeneratorVersion, Confidence: &confidence,
				Kind: cue.Kind, Locked: cue.Locked, Rationale: cue.Rationale,
				SourceFingerprint: cue.SourceFingerprint, DownbeatAligned: cue.DownbeatAligned,
			})
		}
		applyMode := db.GeneratedCueFillEmpty
		if autoCueMode == db.AutomaticCuePointsReplaceGenerated {
			applyMode = db.GeneratedCueReplaceGenerated
		}
		if err := database.ApplyGeneratedDJHotCues(result.SongID, hotCues, applyMode); err != nil {
			return fmt.Errorf("persist generated DJ hot cues: %w", err)
		}
	}
	return nil
}

// resultIssue returns the same stable diagnostics Persist writes, allowing
// the runner to mirror an incomplete result into the durable application log
// without duplicating or drifting from the database contract.
func resultIssue(result Result) (string, string) {
	switch {
	case !result.Tempo.Known && !result.Key.Known:
		return ErrorInsufficientAudio, "No reliable tempo or key evidence in the decoded audio"
	case !result.Tempo.Known:
		return ErrorNoReliableTempo, "No reliable tempo evidence in the decoded audio"
	case !result.Key.Known:
		return ErrorNoReliableKey, "No reliable tonal evidence in the decoded audio"
	default:
		return "", ""
	}
}

func persistBeatGrid(database *db.DB, result Result) error {
	if result.BeatGrid == nil {
		return nil
	}
	// A locked grid is an explicit performance decision.  Re-analysis may
	// refresh measured BPM/key but must not replace the DJ's timing edits.
	override, err := database.GetTrackAnalysisOverride(result.SongID)
	if err == nil && override.BeatgridLocked {
		return nil
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	encoded, err := result.BeatGrid.Encode()
	if err != nil {
		return err
	}
	return database.UpsertTrackAnalysisArtifact(db.TrackAnalysisArtifact{
		ID:               result.SongID + ":" + beatgrid.AlgorithmVersion,
		SongID:           result.SongID,
		Kind:             beatgrid.ArtifactKind,
		FormatVersion:    beatgrid.FormatVersion,
		AlgorithmVersion: beatgrid.AlgorithmVersion,
		Encoding:         beatgrid.Encoding,
		Provenance:       string(result.BeatGrid.EffectiveProvenance()),
		Data:             encoded,
	})
}

func persistFeatures(database *db.DB, result Result) error {
	if result.Features != nil {
		encoded, err := result.Features.Encode()
		if err != nil {
			return err
		}
		if err := database.UpsertTrackAnalysisArtifact(db.TrackAnalysisArtifact{
			ID:               result.SongID + ":" + features.AlgorithmVersion,
			SongID:           result.SongID,
			Kind:             features.ArtifactKind,
			FormatVersion:    features.FormatVersion,
			AlgorithmVersion: features.AlgorithmVersion,
			Encoding:         features.Encoding,
			Provenance:       "measured",
			Data:             encoded,
		}); err != nil {
			return err
		}
	}
	if result.Loudness != nil {
		encoded, err := features.EncodeBS1770(*result.Loudness)
		if err != nil {
			return err
		}
		return database.UpsertTrackAnalysisArtifact(db.TrackAnalysisArtifact{
			ID:               result.SongID + ":" + features.BS1770AlgorithmVersion,
			SongID:           result.SongID,
			Kind:             features.BS1770ArtifactKind,
			FormatVersion:    features.BS1770FormatVersion,
			AlgorithmVersion: features.BS1770AlgorithmVersion,
			Encoding:         features.BS1770Encoding,
			Provenance:       "measured",
			Data:             encoded,
		})
	}
	return nil
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
	if source.SourceRevision != "" {
		record.SourceRevision = ptr(source.SourceRevision)
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
	for _, marker := range []string{"no local analysis source", "stat local source", "not a regular file", "Plex source adapter", "Plex source unavailable", "open local source"} {
		if strings.Contains(message, marker) {
			return ErrorSourceUnavailable, message
		}
	}
	return ErrorDecodeFailed, message
}

func ptr[T any](value T) *T { return &value }
