package db

import (
	"database/sql"
	"errors"
	"fmt"
	"math"
	"time"
)

// TrackAnalysisArtifact stores a versioned opaque payload such as a beat grid.
type TrackAnalysisArtifact struct {
	ID               string
	SongID           string
	Kind             string
	FormatVersion    int
	AlgorithmVersion string
	Encoding         string
	Data             []byte
	CreatedAt        int64
}

// TrackAnalysisOverride records explicit user choices independently of
// measured facts, so re-analysis never overwrites a locked value.
type TrackAnalysisOverride struct {
	SongID             string
	BPM                *float64
	KeyTonic           *int
	KeyMode            *string
	BeatgridArtifactID *string
	BPMLocked          bool
	KeyLocked          bool
	BeatgridLocked     bool
	UpdatedAt          int64
}

const (
	TrackAnalysisPending     = "pending"
	TrackAnalysisRunning     = "running"
	TrackAnalysisComplete    = "complete"
	TrackAnalysisPartial     = "partial"
	TrackAnalysisFailed      = "failed"
	TrackAnalysisUnsupported = "unsupported"
)

// Provenance tiers for a resolved BPM, strongest first. Every tier below
// measured is inferred rather than measured and must never reach Sync.
const (
	EffectiveBPMUnknown = "unknown"
	EffectiveBPMManual  = "manual"
	// EffectiveBPMMeasured covers audio-measured analysis and trustworthy
	// imported tags, both of which describe the audio itself.
	EffectiveBPMMeasured = "measured"
	// EffectiveBPMLegacyAI is songs.bpm, which the AI enrichment path may have
	// estimated from genre conventions rather than from audio.
	EffectiveBPMLegacyAI = "legacy-ai"
	// EffectiveBPMTempoDescriptor is a BPM synthesized from a free-text tempo
	// word ("fast"/"medium"/"slow") that was itself AI-inferred. It is the
	// weakest signal in the system: a coarse bucket wearing the costume of a
	// measurement. It is acceptable for soft AI DJ flow ordering, where being
	// wrong by 20 BPM degrades gracefully, and unacceptable anywhere a DJ could
	// read it as a tempo.
	EffectiveBPMTempoDescriptor = "tempo-descriptor"
)

// EffectiveKeyUnknown, EffectiveKeyManual, and EffectiveKeyMeasured mirror the
// BPM provenance ladder.  A key has no legacy fallback: a missing tonal
// measurement is deliberately represented as unknown instead of guessing from
// tags or genre metadata.
const (
	EffectiveKeyUnknown  = "unknown"
	EffectiveKeyManual   = "manual"
	EffectiveKeyMeasured = "measured"
)

// EffectiveBPMInputs collects every BPM tier a caller may hold. It is a struct
// rather than positional pointers because three of the tiers are numerically
// identical types; swapping two at a call site would silently promote an
// inferred value into Sync.
type EffectiveBPMInputs struct {
	Override  *TrackAnalysisOverride
	Analysis  *TrackAnalysis
	LegacyBPM *int
	// TempoDescriptorBPM must be supplied only by callers that have already
	// decided a coarse inferred ordering hint is acceptable.
	TempoDescriptorBPM *int
}

// EffectiveBPM always carries provenance and whether it is safe for DJ timing
// operations. Inferred values may help non-critical ordering but never enable
// Sync, and callers are expected to branch on Source rather than to re-derive
// this ladder.
type EffectiveBPM struct {
	Value       *float64
	Source      string
	SyncAllowed bool
}

// EffectiveKeyInputs collects the two sources that may authoritatively name a
// track's key. Keeping this separate from EffectiveBPM prevents a future
// caller from accidentally treating an inferred tempo tier as tonal evidence.
type EffectiveKeyInputs struct {
	Override *TrackAnalysisOverride
	Analysis *TrackAnalysis
}

// EffectiveKey carries the pitch-class representation rather than a display
// string. Notation is a presentation concern, so callers can choose Camelot,
// Open Key, or a traditional key name without changing the persisted fact.
type EffectiveKey struct {
	Tonic  *int
	Mode   *string
	Source string
}

// Inferred reports whether the value came from a tier that did not measure the
// audio. Anything inferred must be labeled as an estimate wherever it is shown.
func (b EffectiveBPM) Inferred() bool {
	return b.Source == EffectiveBPMLegacyAI || b.Source == EffectiveBPMTempoDescriptor
}

// ResolveEffectiveBPM applies the documented precedence ladder without
// mutating either measured analysis or legacy song metadata.
func ResolveEffectiveBPM(inputs EffectiveBPMInputs) EffectiveBPM {
	if inputs.Override != nil && inputs.Override.BPMLocked && inputs.Override.BPM != nil {
		return EffectiveBPM{Value: inputs.Override.BPM, Source: EffectiveBPMManual, SyncAllowed: true}
	}
	if measured := measuredBPM(inputs.Analysis); measured != nil {
		return EffectiveBPM{Value: measured, Source: EffectiveBPMMeasured, SyncAllowed: true}
	}
	if inputs.LegacyBPM != nil && *inputs.LegacyBPM > 0 {
		value := float64(*inputs.LegacyBPM)
		return EffectiveBPM{Value: &value, Source: EffectiveBPMLegacyAI}
	}
	if inputs.TempoDescriptorBPM != nil && *inputs.TempoDescriptorBPM > 0 {
		value := float64(*inputs.TempoDescriptorBPM)
		return EffectiveBPM{Value: &value, Source: EffectiveBPMTempoDescriptor}
	}
	return EffectiveBPM{Source: EffectiveBPMUnknown}
}

// ResolveEffectiveKey applies the same manual-over-measured precedence as
// BPM. A partial analysis is sufficient when its key dimension is present.
func ResolveEffectiveKey(inputs EffectiveKeyInputs) EffectiveKey {
	if inputs.Override != nil && inputs.Override.KeyLocked && inputs.Override.KeyTonic != nil && inputs.Override.KeyMode != nil {
		return EffectiveKey{Tonic: inputs.Override.KeyTonic, Mode: inputs.Override.KeyMode, Source: EffectiveKeyManual}
	}
	if tonic, mode := measuredKey(inputs.Analysis); tonic != nil && mode != nil {
		return EffectiveKey{Tonic: tonic, Mode: mode, Source: EffectiveKeyMeasured}
	}
	return EffectiveKey{Source: EffectiveKeyUnknown}
}

// measuredBPM returns an audio-derived tempo only when the record actually
// carries one.
//
// A `partial` record is as authoritative for tempo as a `complete` one: partial
// means one dimension was not measured, and a track whose key could not be
// determined — a drum loop, a percussion tool — still has a perfectly good
// measured BPM. Requiring `complete` here would discard that measurement and
// silently fall back to an AI-estimated tempo.
//
// `pending` and `running` are excluded because a claimed row keeps the previous
// pass's scalar values while already carrying the new source fingerprint, so its
// BPM may describe bytes that are no longer there.
func measuredBPM(analysis *TrackAnalysis) *float64 {
	if analysis == nil || analysis.BPM == nil || analysis.BPMSource == nil {
		return nil
	}
	switch analysis.Status {
	case TrackAnalysisComplete, TrackAnalysisPartial:
	default:
		return nil
	}
	switch *analysis.BPMSource {
	case "measured", "imported":
		return analysis.BPM
	default:
		return nil
	}
}

func measuredKey(analysis *TrackAnalysis) (*int, *string) {
	if analysis == nil || analysis.KeyTonic == nil || analysis.KeyMode == nil || analysis.KeySource == nil {
		return nil, nil
	}
	switch analysis.Status {
	case TrackAnalysisComplete, TrackAnalysisPartial:
	default:
		return nil, nil
	}
	switch *analysis.KeySource {
	case "measured", "imported":
		return analysis.KeyTonic, analysis.KeyMode
	default:
		return nil, nil
	}
}

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

// ListTrackAnalysis returns the current scalar record for every analyzed
// song. It is intentionally one ordered query so library consumers do not
// issue one database read per visible row.
func (d *DB) ListTrackAnalysis() ([]TrackAnalysis, error) {
	if err := d.EnsureTrackAnalysisSchema(); err != nil {
		return nil, err
	}
	rows, err := d.conn.Query(`SELECT song_id, status, analysis_version, algorithm_version, decoder_id,
		source_fingerprint, source_size, source_mtime, source_revision,
		bpm, bpm_confidence, bpm_alt_candidate, tempo_stability, tempo_kind, bpm_source,
		key_tonic, key_mode, key_confidence, key_source, camelot_key, open_key,
		analyzed_at, error_code, error_message FROM track_analysis ORDER BY song_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	results := make([]TrackAnalysis, 0)
	for rows.Next() {
		analysis, err := scanTrackAnalysis(rows)
		if err != nil {
			return nil, err
		}
		results = append(results, analysis)
	}
	return results, rows.Err()
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

// UpsertTrackAnalysisArtifact stores a compact versioned artifact. Its unique
// key replaces only the same kind/format/algorithm representation.
func (d *DB) UpsertTrackAnalysisArtifact(artifact TrackAnalysisArtifact) error {
	if artifact.ID == "" || artifact.SongID == "" || artifact.Kind == "" || artifact.FormatVersion <= 0 || artifact.AlgorithmVersion == "" || artifact.Encoding == "" || len(artifact.Data) == 0 {
		return errors.New("track analysis artifact requires identity, version, encoding, and data")
	}
	if err := d.EnsureTrackAnalysisSchema(); err != nil {
		return err
	}
	if artifact.CreatedAt == 0 {
		artifact.CreatedAt = time.Now().UnixMilli()
	}
	_, err := d.conn.Exec(`INSERT INTO track_analysis_artifacts(id, song_id, kind, format_version, algorithm_version, encoding, data, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(song_id, kind, format_version, algorithm_version) DO UPDATE SET
			id=excluded.id, encoding=excluded.encoding, data=excluded.data, created_at=excluded.created_at`,
		artifact.ID, artifact.SongID, artifact.Kind, artifact.FormatVersion, artifact.AlgorithmVersion, artifact.Encoding, artifact.Data, artifact.CreatedAt)
	return err
}

// GetTrackAnalysisArtifact returns the requested persisted representation.
func (d *DB) GetTrackAnalysisArtifact(songID, kind string, formatVersion int, algorithmVersion string) (TrackAnalysisArtifact, error) {
	if err := d.EnsureTrackAnalysisSchema(); err != nil {
		return TrackAnalysisArtifact{}, err
	}
	var artifact TrackAnalysisArtifact
	err := d.conn.QueryRow(`SELECT id, song_id, kind, format_version, algorithm_version, encoding, data, created_at
		FROM track_analysis_artifacts WHERE song_id = ? AND kind = ? AND format_version = ? AND algorithm_version = ?`, songID, kind, formatVersion, algorithmVersion).
		Scan(&artifact.ID, &artifact.SongID, &artifact.Kind, &artifact.FormatVersion, &artifact.AlgorithmVersion, &artifact.Encoding, &artifact.Data, &artifact.CreatedAt)
	return artifact, err
}

// ListTrackAnalysisArtifacts returns one representation per song for a
// versioned feature family.  AI and UI callers use this bounded metadata read
// instead of decoding a file or issuing an N+1 artifact query per library row.
func (d *DB) ListTrackAnalysisArtifacts(kind string, formatVersion int, algorithmVersion string) ([]TrackAnalysisArtifact, error) {
	if err := d.EnsureTrackAnalysisSchema(); err != nil {
		return nil, err
	}
	rows, err := d.conn.Query(`SELECT id, song_id, kind, format_version, algorithm_version, encoding, data, created_at
		FROM track_analysis_artifacts WHERE kind = ? AND format_version = ? AND algorithm_version = ? ORDER BY song_id`, kind, formatVersion, algorithmVersion)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	artifacts := make([]TrackAnalysisArtifact, 0)
	for rows.Next() {
		var artifact TrackAnalysisArtifact
		if err := rows.Scan(&artifact.ID, &artifact.SongID, &artifact.Kind, &artifact.FormatVersion, &artifact.AlgorithmVersion, &artifact.Encoding, &artifact.Data, &artifact.CreatedAt); err != nil {
			return nil, err
		}
		artifacts = append(artifacts, artifact)
	}
	return artifacts, rows.Err()
}

// UpsertTrackAnalysisOverride persists manual values and their independent
// locks. Supplying a zero UpdatedAt assigns the write time.
func (d *DB) UpsertTrackAnalysisOverride(override TrackAnalysisOverride) error {
	if override.SongID == "" {
		return errors.New("track analysis override requires song ID")
	}
	if err := d.EnsureTrackAnalysisSchema(); err != nil {
		return err
	}
	if override.UpdatedAt == 0 {
		override.UpdatedAt = time.Now().UnixMilli()
	}
	_, err := d.conn.Exec(`INSERT INTO track_analysis_overrides(song_id, bpm, key_tonic, key_mode, beatgrid_artifact_id, bpm_locked, key_locked, beatgrid_locked, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(song_id) DO UPDATE SET bpm=excluded.bpm, key_tonic=excluded.key_tonic,
			key_mode=excluded.key_mode, beatgrid_artifact_id=excluded.beatgrid_artifact_id,
			bpm_locked=excluded.bpm_locked, key_locked=excluded.key_locked,
			beatgrid_locked=excluded.beatgrid_locked, updated_at=excluded.updated_at`,
		override.SongID, override.BPM, override.KeyTonic, override.KeyMode, override.BeatgridArtifactID,
		boolToInt(override.BPMLocked), boolToInt(override.KeyLocked), boolToInt(override.BeatgridLocked), override.UpdatedAt)
	return err
}

// GetTrackAnalysisOverride returns a song's manual analysis choices.
func (d *DB) GetTrackAnalysisOverride(songID string) (TrackAnalysisOverride, error) {
	if err := d.EnsureTrackAnalysisSchema(); err != nil {
		return TrackAnalysisOverride{}, err
	}
	var result TrackAnalysisOverride
	var bpm sql.NullFloat64
	var keyTonic sql.NullInt64
	var keyMode, artifactID sql.NullString
	var bpmLocked, keyLocked, gridLocked int
	err := d.conn.QueryRow(`SELECT song_id, bpm, key_tonic, key_mode, beatgrid_artifact_id, bpm_locked, key_locked, beatgrid_locked, updated_at
		FROM track_analysis_overrides WHERE song_id = ?`, songID).
		Scan(&result.SongID, &bpm, &keyTonic, &keyMode, &artifactID, &bpmLocked, &keyLocked, &gridLocked, &result.UpdatedAt)
	if err != nil {
		return TrackAnalysisOverride{}, err
	}
	result.BPM = optionalFloat64(bpm)
	if keyTonic.Valid {
		value := int(keyTonic.Int64)
		result.KeyTonic = &value
	}
	result.KeyMode = optionalString(keyMode)
	result.BeatgridArtifactID = optionalString(artifactID)
	result.BPMLocked = bpmLocked != 0
	result.KeyLocked = keyLocked != 0
	result.BeatgridLocked = gridLocked != 0
	return result, nil
}

// ListTrackAnalysisOverrides returns every manual override keyed by song ID.
// Pair it with ListTrackAnalysis when rendering a library-wide feature map.
func (d *DB) ListTrackAnalysisOverrides() (map[string]TrackAnalysisOverride, error) {
	if err := d.EnsureTrackAnalysisSchema(); err != nil {
		return nil, err
	}
	rows, err := d.conn.Query(`SELECT song_id, bpm, key_tonic, key_mode, beatgrid_artifact_id, bpm_locked, key_locked, beatgrid_locked, updated_at
		FROM track_analysis_overrides`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	results := make(map[string]TrackAnalysisOverride)
	for rows.Next() {
		var result TrackAnalysisOverride
		var bpm sql.NullFloat64
		var keyTonic sql.NullInt64
		var keyMode, artifactID sql.NullString
		var bpmLocked, keyLocked, gridLocked int
		if err := rows.Scan(&result.SongID, &bpm, &keyTonic, &keyMode, &artifactID, &bpmLocked, &keyLocked, &gridLocked, &result.UpdatedAt); err != nil {
			return nil, err
		}
		result.BPM = optionalFloat64(bpm)
		if keyTonic.Valid {
			value := int(keyTonic.Int64)
			result.KeyTonic = &value
		}
		result.KeyMode = optionalString(keyMode)
		result.BeatgridArtifactID = optionalString(artifactID)
		result.BPMLocked = bpmLocked != 0
		result.KeyLocked = keyLocked != 0
		result.BeatgridLocked = gridLocked != 0
		results[result.SongID] = result
	}
	return results, rows.Err()
}

// ListEffectiveBPM returns only manual or audio-measured tempo rounded for
// AI-DJ scoring. It intentionally omits legacy and descriptor values: callers
// retain their existing song metadata fallback when this map has no entry.
func (d *DB) ListEffectiveBPM() (map[string]int, error) {
	analyses, err := d.ListTrackAnalysis()
	if err != nil {
		return nil, err
	}
	overrides, err := d.ListTrackAnalysisOverrides()
	if err != nil {
		return nil, err
	}
	results := make(map[string]int, len(analyses))
	for _, analysis := range analyses {
		effective := ResolveEffectiveBPM(EffectiveBPMInputs{Override: ptrTrackAnalysisOverride(overrides, analysis.SongID), Analysis: &analysis})
		if effective.Value != nil && effective.SyncAllowed {
			results[analysis.SongID] = int(math.Round(*effective.Value))
		}
	}
	return results, nil
}

func ptrTrackAnalysisOverride(overrides map[string]TrackAnalysisOverride, songID string) *TrackAnalysisOverride {
	override, ok := overrides[songID]
	if !ok {
		return nil
	}
	return &override
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

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
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
