package db

import (
	"encoding/json"
	"errors"
	"time"
)

// ManualScalarCandidate permits corrections only for named editable fields.
// Physical duration/loudness and provider-native scores remain observations.
func ManualScalarCandidate(key, fingerprint string, raw json.RawMessage, at int64) (ScalarCandidate, error) {
	f := SpotifyScalarField{Key: key, Value: raw, AdapterRevision: "manual-v1", RetrievedAt: time.UnixMilli(at).UTC()}
	switch key {
	case "time_signature":
		f.Metric, f.Units = "measured_meter", "beats_per_bar"
	case "local_energy_level":
		f.Metric, f.Units = "local_energy_level", "level_1_10"
	default:
		return ScalarCandidate{}, errors.New("field is not editable")
	}
	c := ScalarCandidate{SpotifyScalarField: f, Source: "manual", SourceFingerprint: fingerprint, Locked: true}
	if fingerprint == "" || len(raw) > 256 || !validEffectiveScalarCandidate(c) {
		return ScalarCandidate{}, errors.New("invalid manual field value")
	}
	return c, nil
}

// SetTrackMetadataOverrideIfSourceCurrent touches only the requested field.
func (d *DB) SetTrackMetadataOverrideIfSourceCurrent(song, key, fp string, value json.RawMessage, reset bool) (bool, error) {
	if _, err := ManualScalarCandidate(key, fp, json.RawMessage("4"), time.Now().UnixMilli()); err != nil {
		return false, err
	}
	if !reset {
		if _, err := ManualScalarCandidate(key, fp, value, time.Now().UnixMilli()); err != nil {
			return false, err
		}
	}
	if err := d.EnsureTrackAnalysisSchema(); err != nil {
		return false, err
	}
	tx, err := d.conn.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	result, err := tx.Exec(`UPDATE track_analysis_source_revisions SET source_fingerprint=source_fingerprint WHERE song_id=? AND source_fingerprint=?`, song, fp)
	if err != nil {
		return false, err
	}
	n, err := result.RowsAffected()
	if err != nil || n != 1 {
		return false, err
	}
	if reset {
		_, err = tx.Exec(`DELETE FROM track_metadata_overrides WHERE song_id=? AND field_key=?`, song, key)
	} else {
		_, err = tx.Exec(`INSERT INTO track_metadata_overrides(song_id,field_key,value_json,source_fingerprint,updated_at) VALUES(?,?,?,?,?) ON CONFLICT(song_id,field_key) DO UPDATE SET value_json=excluded.value_json,source_fingerprint=excluded.source_fingerprint,updated_at=excluded.updated_at`, song, key, string(value), fp, time.Now().UnixMilli())
	}
	if err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

// ListTrackMetadataOverrides is one bounded scalar read for library assembly.
func (d *DB) ListTrackMetadataOverrides() (map[string][]ScalarCandidate, error) {
	if err := d.EnsureTrackAnalysisSchema(); err != nil {
		return nil, err
	}
	rows, err := d.conn.Query(`SELECT song_id,field_key,value_json,source_fingerprint,updated_at FROM track_metadata_overrides ORDER BY song_id,field_key`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := map[string][]ScalarCandidate{}
	count := 0
	for rows.Next() {
		count++
		if count > 100000 {
			return nil, errors.New("manual field row limit exceeded")
		}
		var song, key, raw, fp string
		var at int64
		if err := rows.Scan(&song, &key, &raw, &fp, &at); err != nil {
			return nil, err
		}
		if c, err := ManualScalarCandidate(key, fp, json.RawMessage(raw), at); err == nil {
			result[song] = append(result[song], c)
		}
	}
	return result, rows.Err()
}

func (d *DB) GetTrackMetadataOverrides(song string) ([]ScalarCandidate, error) {
	if err := d.EnsureTrackAnalysisSchema(); err != nil {
		return nil, err
	}
	rows, err := d.conn.Query(`SELECT field_key,value_json,source_fingerprint,updated_at FROM track_metadata_overrides WHERE song_id=? ORDER BY field_key`, song)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []ScalarCandidate{}
	for rows.Next() {
		var key, raw, fp string
		var at int64
		if err := rows.Scan(&key, &raw, &fp, &at); err != nil {
			return nil, err
		}
		if c, err := ManualScalarCandidate(key, fp, json.RawMessage(raw), at); err == nil {
			result = append(result, c)
		}
	}
	return result, rows.Err()
}
