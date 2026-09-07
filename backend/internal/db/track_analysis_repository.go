package db

import (
	"database/sql"
	"errors"
	"fmt"
)

const (
	TrackAnalysisPending     = "pending"
	TrackAnalysisRunning     = "running"
	TrackAnalysisComplete    = "complete"
	TrackAnalysisPartial     = "partial"
	TrackAnalysisFailed      = "failed"
	TrackAnalysisUnsupported = "unsupported"
)

// TrackAnalysis is the durable scalar result for one canonical song. Nullable
// measured values use pointers so zero is never confused with unknown.
type TrackAnalysis struct {
	SongID            string
	Status            string
	AnalysisVersion   int
	AlgorithmVersion  string
	DecoderID         *string
	SourceFingerprint string
	SourceSize        *int64
	SourceMtime       *int64
	SourceRevision    *string
	BPM               *float64
	BPMConfidence     *float64
	BPMAltCandidate   *float64
	TempoStability    *float64
	TempoKind         *string
	BPMSource         *string
	KeyTonic          *int
	KeyMode           *string
	KeyConfidence     *float64
	KeySource         *string
	CamelotKey        *string
	OpenKey           *string
	AnalyzedAt        *int64
	ErrorCode         *string
	ErrorMessage      *string
}

// UpsertTrackAnalysis atomically replaces one song's scalar analysis result.
// Callers must supply the source fingerprint they decoded so stale results can
// be identified without re-decoding the media.
func (d *DB) UpsertTrackAnalysis(analysis TrackAnalysis) error {
	if err := validateTrackAnalysis(analysis); err != nil {
		return err
	}
	if err := d.EnsureTrackAnalysisSchema(); err != nil {
		return err
	}
	_, err := d.conn.Exec(`
		INSERT INTO track_analysis(
			song_id, status, analysis_version, algorithm_version, decoder_id,
			source_fingerprint, source_size, source_mtime, source_revision,
			bpm, bpm_confidence, bpm_alt_candidate, tempo_stability, tempo_kind, bpm_source,
			key_tonic, key_mode, key_confidence, key_source, camelot_key, open_key,
			analyzed_at, error_code, error_message
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(song_id) DO UPDATE SET
			status=excluded.status, analysis_version=excluded.analysis_version,
			algorithm_version=excluded.algorithm_version, decoder_id=excluded.decoder_id,
			source_fingerprint=excluded.source_fingerprint, source_size=excluded.source_size,
			source_mtime=excluded.source_mtime, source_revision=excluded.source_revision,
			bpm=excluded.bpm, bpm_confidence=excluded.bpm_confidence,
			bpm_alt_candidate=excluded.bpm_alt_candidate, tempo_stability=excluded.tempo_stability,
			tempo_kind=excluded.tempo_kind, bpm_source=excluded.bpm_source,
			key_tonic=excluded.key_tonic, key_mode=excluded.key_mode,
			key_confidence=excluded.key_confidence, key_source=excluded.key_source,
			camelot_key=excluded.camelot_key, open_key=excluded.open_key,
			analyzed_at=excluded.analyzed_at, error_code=excluded.error_code,
			error_message=excluded.error_message`,
		analysis.SongID, analysis.Status, analysis.AnalysisVersion, analysis.AlgorithmVersion,
		analysis.DecoderID, analysis.SourceFingerprint, analysis.SourceSize, analysis.SourceMtime,
		analysis.SourceRevision, analysis.BPM, analysis.BPMConfidence, analysis.BPMAltCandidate,
		analysis.TempoStability, analysis.TempoKind, analysis.BPMSource, analysis.KeyTonic,
		analysis.KeyMode, analysis.KeyConfidence, analysis.KeySource, analysis.CamelotKey,
		analysis.OpenKey, analysis.AnalyzedAt, analysis.ErrorCode, analysis.ErrorMessage)
	return err
}

// GetTrackAnalysis loads the scalar result for a canonical song.
func (d *DB) GetTrackAnalysis(songID string) (TrackAnalysis, error) {
	if err := d.EnsureTrackAnalysisSchema(); err != nil {
		return TrackAnalysis{}, err
	}
	row := d.conn.QueryRow(`SELECT song_id, status, analysis_version, algorithm_version, decoder_id,
		source_fingerprint, source_size, source_mtime, source_revision,
		bpm, bpm_confidence, bpm_alt_candidate, tempo_stability, tempo_kind, bpm_source,
		key_tonic, key_mode, key_confidence, key_source, camelot_key, open_key,
		analyzed_at, error_code, error_message FROM track_analysis WHERE song_id = ?`, songID)
	return scanTrackAnalysis(row)
}

// TrackAnalysisStale reports whether a completed result was made from a
// different source fingerprint. Missing analysis is stale by definition.
func (d *DB) TrackAnalysisStale(songID, sourceFingerprint string) (bool, error) {
	analysis, err := d.GetTrackAnalysis(songID)
	if errors.Is(err, sql.ErrNoRows) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	return analysis.SourceFingerprint != sourceFingerprint, nil
}

type trackAnalysisScanner interface{ Scan(dest ...any) error }

func scanTrackAnalysis(row trackAnalysisScanner) (TrackAnalysis, error) {
	var analysis TrackAnalysis
	var decoderID, sourceRevision, tempoKind, bpmSource, keyMode, keySource, camelotKey, openKey, errorCode, errorMessage sql.NullString
	var sourceSize, sourceMtime, analyzedAt sql.NullInt64
	var bpm, bpmConfidence, bpmAltCandidate, tempoStability, keyConfidence sql.NullFloat64
	var keyTonic sql.NullInt64
	err := row.Scan(&analysis.SongID, &analysis.Status, &analysis.AnalysisVersion, &analysis.AlgorithmVersion,
		&decoderID, &analysis.SourceFingerprint, &sourceSize, &sourceMtime, &sourceRevision,
		&bpm, &bpmConfidence, &bpmAltCandidate, &tempoStability, &tempoKind, &bpmSource,
		&keyTonic, &keyMode, &keyConfidence, &keySource, &camelotKey, &openKey,
		&analyzedAt, &errorCode, &errorMessage)
	if err != nil {
		return TrackAnalysis{}, err
	}
	analysis.DecoderID = optionalString(decoderID)
	analysis.SourceSize = optionalInt64(sourceSize)
	analysis.SourceMtime = optionalInt64(sourceMtime)
	analysis.SourceRevision = optionalString(sourceRevision)
	analysis.BPM = optionalFloat64(bpm)
	analysis.BPMConfidence = optionalFloat64(bpmConfidence)
	analysis.BPMAltCandidate = optionalFloat64(bpmAltCandidate)
	analysis.TempoStability = optionalFloat64(tempoStability)
	analysis.TempoKind = optionalString(tempoKind)
	analysis.BPMSource = optionalString(bpmSource)
	if keyTonic.Valid {
		value := int(keyTonic.Int64)
		analysis.KeyTonic = &value
	}
	analysis.KeyMode = optionalString(keyMode)
	analysis.KeyConfidence = optionalFloat64(keyConfidence)
	analysis.KeySource = optionalString(keySource)
	analysis.CamelotKey = optionalString(camelotKey)
	analysis.OpenKey = optionalString(openKey)
	analysis.AnalyzedAt = optionalInt64(analyzedAt)
	analysis.ErrorCode = optionalString(errorCode)
	analysis.ErrorMessage = optionalString(errorMessage)
	return analysis, nil
}

func optionalString(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	result := value.String
	return &result
}
func optionalInt64(value sql.NullInt64) *int64 {
	if !value.Valid {
		return nil
	}
	result := value.Int64
	return &result
}
func optionalFloat64(value sql.NullFloat64) *float64 {
	if !value.Valid {
		return nil
	}
	result := value.Float64
	return &result
}

func validateTrackAnalysis(analysis TrackAnalysis) error {
	if analysis.SongID == "" || analysis.SourceFingerprint == "" || analysis.AlgorithmVersion == "" || analysis.AnalysisVersion <= 0 {
		return errors.New("track analysis requires song ID, source fingerprint, and versions")
	}
	switch analysis.Status {
	case TrackAnalysisPending, TrackAnalysisRunning, TrackAnalysisComplete, TrackAnalysisPartial, TrackAnalysisFailed, TrackAnalysisUnsupported:
	default:
		return fmt.Errorf("invalid track analysis status %q", analysis.Status)
	}
	return nil
}
