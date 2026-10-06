package db

import (
	"errors"
	"math"
)

// LocalScalarObservation retains local alternatives independently of the
// provider-selected compatibility projection. Nil values mean abstention.
type LocalScalarObservation struct {
	SourceFingerprint string   `json:"sourceFingerprint"`
	AlgorithmVersion  string   `json:"algorithmVersion"`
	MeasuredAt        int64    `json:"measuredAt"`
	BPM               *float64 `json:"bpm,omitempty"`
	BPMConfidence     *float64 `json:"bpmConfidence,omitempty"`
	BPMAltCandidate   *float64 `json:"bpmAltCandidate,omitempty"`
	TempoStability    *float64 `json:"tempoStability,omitempty"`
	TempoKind         *string  `json:"tempoKind,omitempty"`
	KeyTonic          *int     `json:"keyTonic,omitempty"`
	KeyMode           *string  `json:"keyMode,omitempty"`
	KeyConfidence     *float64 `json:"keyConfidence,omitempty"`
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
	if l == nil || l.SourceFingerprint != a.SourceFingerprint || validateLocalScalars(l) != nil {
		return nil
	}
	return l
}
