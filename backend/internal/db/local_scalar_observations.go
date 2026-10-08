package db

import (
	"errors"
	"github.com/ajbergh/viib-mediahub/internal/analysis/featurecontract"
	"math"
	"strings"
)

// LocalScalarObservation retains local alternatives independently of the
// provider-selected compatibility projection. Nil values mean abstention.
type LocalScalarObservation struct {
	Fields            []SpotifyScalarField `json:"fields,omitempty"`
	SourceFingerprint string               `json:"sourceFingerprint"`
	AlgorithmVersion  string               `json:"algorithmVersion"`
	MeasuredAt        int64                `json:"measuredAt"`
	BPM               *float64             `json:"bpm,omitempty"`
	BPMConfidence     *float64             `json:"bpmConfidence,omitempty"`
	BPMAltCandidate   *float64             `json:"bpmAltCandidate,omitempty"`
	TempoStability    *float64             `json:"tempoStability,omitempty"`
	TempoKind         *string              `json:"tempoKind,omitempty"`
	KeyTonic          *int                 `json:"keyTonic,omitempty"`
	KeyMode           *string              `json:"keyMode,omitempty"`
	KeyConfidence     *float64             `json:"keyConfidence,omitempty"`
}

func ensureLocalScalarColumn(d *DB) error { return ensureAnalysisJSONColumn(d, "local_scalar_json") }

func ensureAnalysisJSONColumn(d *DB, column string) error {
	return ensureTextColumn(d, "track_analysis", column)
}

func ensureTextColumn(d *DB, table, column string) error {
	rows, err := d.conn.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		return err
	}
	found := false
	for rows.Next() {
		var cid, notnull, pk int
		var name, typ string
		var defaultValue any
		if err := rows.Scan(&cid, &name, &typ, &notnull, &defaultValue, &pk); err != nil {
			rows.Close()
			return err
		}
		if name == column {
			found = true
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	if found {
		return nil
	}
	_, err = d.conn.Exec(`ALTER TABLE ` + table + ` ADD COLUMN ` + column + ` TEXT`)
	return err
}

func validateLocalScalars(local *LocalScalarObservation) error {
	if local == nil {
		return nil
	}
	if local.SourceFingerprint == "" || local.AlgorithmVersion == "" || len(local.SourceFingerprint) > 1024 || len(local.AlgorithmVersion) > 1024 || local.MeasuredAt < 0 {
		return errors.New("invalid local scalar provenance")
	}
	if len(local.Fields) > 32 {
		return errors.New("too many local scalar fields")
	}
	seen := map[string]bool{}
	for _, field := range local.Fields {
		if !validLocalScalarField(field) || seen[field.Key] {
			return errors.New("invalid local scalar field")
		}
		seen[field.Key] = true
	}
	finite := func(v *float64, min, max float64) bool {
		return v == nil || (!math.IsNaN(*v) && !math.IsInf(*v, 0) && *v >= min && *v <= max)
	}
	if !finite(local.BPM, 1, 1000) || !finite(local.BPMAltCandidate, 1, 1000) || !finite(local.BPMConfidence, 0, 1) || !finite(local.TempoStability, 0, 1) || !finite(local.KeyConfidence, 0, 1) {
		return errors.New("invalid local scalar value")
	}
	if (local.KeyTonic == nil) != (local.KeyMode == nil) {
		return errors.New("incomplete local key")
	}
	if local.KeyTonic != nil && (*local.KeyTonic < 0 || *local.KeyTonic > 11 || (*local.KeyMode != "major" && *local.KeyMode != "minor")) {
		return errors.New("invalid local key")
	}
	if local.TempoKind != nil && *local.TempoKind != "unknown" && *local.TempoKind != "static" && *local.TempoKind != "dynamic-candidate" && *local.TempoKind != "dynamic" {
		return errors.New("invalid local tempo kind")
	}
	return nil
}

// Only explicitly measured legacy dimensions can be recovered. Spotify slots
// never prove that a local measurement exists.
func legacyLocalScalars(a TrackAnalysis) *LocalScalarObservation {
	if a.Status != TrackAnalysisComplete && a.Status != TrackAnalysisPartial {
		return nil
	}
	l := &LocalScalarObservation{SourceFingerprint: a.SourceFingerprint, AlgorithmVersion: a.AlgorithmVersion}
	if a.AnalyzedAt != nil {
		l.MeasuredAt = *a.AnalyzedAt
	}
	if a.BPMSource != nil && *a.BPMSource == EffectiveBPMMeasured {
		l.BPM = a.BPM
		l.BPMConfidence = a.BPMConfidence
		l.BPMAltCandidate = a.BPMAltCandidate
		l.TempoStability = a.TempoStability
		l.TempoKind = a.TempoKind
	}
	if a.KeySource != nil && *a.KeySource == EffectiveKeyMeasured {
		l.KeyTonic = a.KeyTonic
		l.KeyMode = a.KeyMode
		l.KeyConfidence = a.KeyConfidence
	}
	if l.BPM == nil && l.KeyTonic == nil {
		return nil
	}
	if validateLocalScalars(l) != nil {
		return nil
	}
	return l
}

// CurrentLocalScalars excludes retained observations from a previous revision
// and rows with a live claim whose compatibility values may still be old.
func CurrentLocalScalars(a *TrackAnalysis) *LocalScalarObservation {
	if a == nil || (a.Status != TrackAnalysisComplete && a.Status != TrackAnalysisPartial) {
		return nil
	}
	l := a.Local
	if l == nil {
		l = legacyLocalScalars(*a)
	}
	if l == nil || l.SourceFingerprint != a.SourceFingerprint {
		return nil
	}
	qualified := qualifyLocalScalarFields(*l)
	if validateLocalScalars(&qualified) != nil {
		return nil
	}
	return &qualified
}

// Local metrics have their own registry: provider definitions cannot substitute.
func validLocalScalarField(f SpotifyScalarField) bool {
	metric, units := "", ""
	switch f.Key {
	case "local_duration_seconds":
		metric, units = "decoded_file_duration", "seconds"
	case "local_energy_level":
		metric, units = "local_energy_level", "level_1_10"
	case "integrated_lufs_bs1770":
		metric, units = "bs1770_integrated_loudness", "LUFS"
	case "true_peak_dbtp":
		metric, units = "bs1770_true_peak", "dBTP"
	default:
		return false
	}
	var value float64
	if strings.TrimSpace(string(f.Value)) == "null" {
		return false
	}
	if f.Metric != metric || f.Units != units || f.AdapterRevision == "" || f.RetrievedAt.IsZero() || f.Endpoint != "" || f.DurableImport || f.Stale || !decodeFiniteScalar(f.Value, &value) {
		return false
	}
	if f.Confidence != nil && (math.IsNaN(*f.Confidence) || math.IsInf(*f.Confidence, 0) || *f.Confidence < 0 || *f.Confidence > 1) {
		return false
	}
	switch f.Key {
	case "local_duration_seconds":
		return value > 0
	case "local_energy_level":
		return value >= 1 && value <= 10 && math.Trunc(value) == value
	}
	return true
}

// Reads isolate optional field corruption; writes still reject invalid data.
// Duplicate keys are ambiguous, so neither duplicate is applied.
func qualifyLocalScalarFields(local LocalScalarObservation) LocalScalarObservation {
	fields := local.Fields
	local.Fields = nil
	if len(fields) > 32 {
		return local
	}
	counts := map[string]int{}
	for _, field := range fields {
		counts[field.Key]++
	}
	for _, field := range fields {
		if counts[field.Key] == 1 && validLocalScalarField(field) {
			local.Fields = append(local.Fields, field)
		}
	}
	return local
}

// Decoded duration measures PCM frames, independently of musical estimators.
const LocalDurationAlgorithmVersion = "decoded-pcm-duration-v1"

func currentLocalScalarField(f SpotifyScalarField) bool {
	if !validLocalScalarField(f) {
		return false
	}
	switch f.Key {
	case "local_duration_seconds":
		return f.AdapterRevision == LocalDurationAlgorithmVersion
	case "local_energy_level":
		return f.AdapterRevision == featurecontract.EnergyLevelAlgorithmVersion
	case "integrated_lufs_bs1770", "true_peak_dbtp":
		return f.AdapterRevision == featurecontract.BS1770AlgorithmVersion
	}
	return false
}

// HasCurrentLocalScalarField checks persisted preparation evidence independently
// of claim status. Effective consumers must still use CurrentLocalScalars.
func HasCurrentLocalScalarField(local *LocalScalarObservation, fingerprint, key string) bool {
	if local == nil || fingerprint == "" || local.SourceFingerprint != fingerprint {
		return false
	}
	qualified := qualifyLocalScalarFields(*local)
	if validateLocalScalars(&qualified) != nil {
		return false
	}
	for _, field := range qualified.Fields {
		if field.Key == key && currentLocalScalarField(field) {
			return true
		}
	}
	return false
}
