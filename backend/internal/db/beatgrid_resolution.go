package db

import (
	"database/sql"
	"errors"
	"github.com/ajbergh/viib-mediahub/internal/analysis/beatgrid"
	"math"
	"time"
)

type BeatGridResolution struct {
	Grid   *beatgrid.Grid
	Reason string
}

func (d *DB) ResolveBeatGrid(song, fp string) (BeatGridResolution, error) {
	if err := d.EnsureTrackAnalysisSchema(); err != nil {
		return BeatGridResolution{}, err
	}
	return resolveBeatGrid(d.conn, song, fp)
}
func resolveBeatGrid(q preparationExecutor, song, fp string) (BeatGridResolution, error) {
	if fp == "" {
		return BeatGridResolution{Reason: "source_unavailable"}, nil
	}
	var data []byte
	var source, encoding, provenance string
	err := q.QueryRow(`SELECT data,source_fingerprint,encoding,provenance FROM track_analysis_artifacts WHERE song_id=? AND kind=? AND format_version=? AND algorithm_version=?`, song, beatgrid.ArtifactKind, beatgrid.FormatVersion, beatgrid.AlgorithmVersion).Scan(&data, &source, &encoding, &provenance)
	if errors.Is(err, sql.ErrNoRows) {
		return BeatGridResolution{Reason: "missing"}, nil
	}
	if err != nil {
		return BeatGridResolution{}, err
	}
	if source == "" || source != fp {
		return BeatGridResolution{Reason: "source_mismatch"}, nil
	}
	if encoding != beatgrid.Encoding {
		return BeatGridResolution{Reason: "invalid_encoding"}, nil
	}
	grid, err := beatgrid.Decode(data)
	if err != nil {
		return BeatGridResolution{Reason: "corrupt"}, nil
	}
	p := beatgrid.Provenance(provenance)
	if !p.Valid() {
		return BeatGridResolution{Reason: "invalid_provenance"}, nil
	}
	grid.Provenance = p
	if len(grid.Beats) < 2 {
		return BeatGridResolution{Reason: "insufficient_beats"}, nil
	}
	return BeatGridResolution{Grid: &grid}, nil
}
func lockedBeatGridCapability(q preparationExecutor, song, fp string) (*TrackCapabilityStatus, error) {
	var locked bool
	if err := q.QueryRow(`SELECT EXISTS(SELECT 1 FROM track_analysis_overrides WHERE song_id=? AND beatgrid_locked=1)`, song).Scan(&locked); err != nil {
		return nil, err
	}
	if !locked {
		return nil, nil
	}
	r, err := resolveBeatGrid(q, song, fp)
	if err != nil {
		return nil, err
	}
	s := &TrackCapabilityStatus{SongID: song, SourceFingerprint: fp, Capability: "local_beatgrid", Version: beatgrid.AlgorithmVersion, State: "available"}
	if r.Grid == nil {
		s.State = "unavailable"
		s.Reason = "locked_beatgrid_" + r.Reason
	}
	return s, nil
}
func (d *DB) LockedBeatGridCapability(song, fp string) (*TrackCapabilityStatus, error) {
	if err := d.EnsureTrackAnalysisSchema(); err != nil {
		return nil, err
	}
	return lockedBeatGridCapability(d.conn, song, fp)
}

// SaveManualBeatGridIfSourceCurrent writes grid, lock, optional BPM and capability
// together. Only grid/BPM columns are updated; concurrent key edits are preserved.
func (d *DB) SaveManualBeatGridIfSourceCurrent(a TrackAnalysisArtifact, locked bool, bpm *float64) (bool, error) {
	if a.SourceFingerprint == "" || a.Kind != beatgrid.ArtifactKind {
		return false, errors.New("manual grid requires source")
	}
	if a.FormatVersion != beatgrid.FormatVersion || a.AlgorithmVersion != beatgrid.AlgorithmVersion || a.Encoding != beatgrid.Encoding || a.Provenance != string(beatgrid.ProvenanceManual) {
		return false, errors.New("invalid manual grid identity")
	}
	grid, decodeErr := beatgrid.Decode(a.Data)
	if decodeErr != nil || len(grid.Beats) < 2 {
		return false, errors.New("invalid manual grid payload")
	}
	if bpm != nil && (math.IsNaN(*bpm) || math.IsInf(*bpm, 0) || *bpm <= 0 || *bpm > 1000) {
		return false, errors.New("invalid BPM")
	}
	if err := d.EnsureTrackAnalysisSchema(); err != nil {
		return false, err
	}
	tx, err := d.conn.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	result, err := tx.Exec(`UPDATE track_analysis_source_revisions SET updated_at=updated_at WHERE song_id=? AND source_fingerprint=?`, a.SongID, a.SourceFingerprint)
	if err != nil {
		return false, err
	}
	n, err := result.RowsAffected()
	if err != nil || n != 1 {
		return false, err
	}
	if err := upsertTrackAnalysisArtifact(tx, a); err != nil {
		return false, err
	}
	_, err = tx.Exec(`INSERT INTO track_analysis_overrides(song_id,beatgrid_artifact_id,beatgrid_locked,updated_at) VALUES(?,?,?,?) ON CONFLICT(song_id) DO UPDATE SET beatgrid_artifact_id=excluded.beatgrid_artifact_id,beatgrid_locked=excluded.beatgrid_locked,updated_at=excluded.updated_at`, a.SongID, a.ID, locked, time.Now().UnixMilli())
	if err != nil {
		return false, err
	}
	if bpm != nil {
		if _, err := tx.Exec(`UPDATE track_analysis_overrides SET bpm=?,bpm_source_fingerprint=?,bpm_locked=1 WHERE song_id=?`, *bpm, a.SourceFingerprint, a.SongID); err != nil {
			return false, err
		}
	}
	if err := putTrackCapabilityStatusesTx(tx, []TrackCapabilityStatus{{SongID: a.SongID, SourceFingerprint: a.SourceFingerprint, Capability: "local_beatgrid", Version: beatgrid.AlgorithmVersion, State: "available"}}); err != nil {
		return false, err
	}
	return true, tx.Commit()
}
func (d *DB) ResetBeatGridIfSourceCurrent(song, fp string) (bool, error) {
	if err := d.EnsureTrackAnalysisSchema(); err != nil {
		return false, err
	}
	tx, err := d.conn.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	r, err := tx.Exec(`UPDATE track_analysis_source_revisions SET updated_at=updated_at WHERE song_id=? AND source_fingerprint=?`, song, fp)
	if err != nil {
		return false, err
	}
	n, err := r.RowsAffected()
	if err != nil || n != 1 {
		return false, err
	}
	if _, err := tx.Exec(`UPDATE track_analysis_overrides SET beatgrid_artifact_id=NULL,beatgrid_locked=0,updated_at=? WHERE song_id=?`, time.Now().UnixMilli(), song); err != nil {
		return false, err
	}
	if _, err := tx.Exec(`DELETE FROM track_analysis_artifacts WHERE song_id=? AND kind=?`, song, beatgrid.ArtifactKind); err != nil {
		return false, err
	}
	if _, err := tx.Exec(`DELETE FROM track_metadata_capability_status WHERE song_id=? AND capability='local_beatgrid'`, song); err != nil {
		return false, err
	}
	return true, tx.Commit()
}
