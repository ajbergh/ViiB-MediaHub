package db

import (
	"errors"
	"github.com/google/uuid"
	"time"
)

var ErrWaveformLeaseBusy = errors.New("waveform decode lease busy")

// ClaimWaveformLease shares exclusion with scalar preparation without creating
// or changing a scalar row. One SQL write arbitrates concurrent claimers.
func (d *DB) ClaimWaveformLease(song, fingerprint string) (string, bool, error) {
	if song == "" || fingerprint == "" {
		return "", false, errors.New("waveform lease requires song and source")
	}
	if err := d.EnsureTrackAnalysisSchema(); err != nil {
		return "", false, err
	}
	now := time.Now().UnixMilli()
	token := uuid.NewString()
	result, err := d.conn.Exec(`INSERT INTO track_waveform_leases(song_id,source_fingerprint,token,renewed_at)
 SELECT ?,?,?,? WHERE NOT EXISTS(SELECT 1 FROM track_analysis WHERE song_id=? AND status='running' AND analyzed_at>?)
 ON CONFLICT(song_id) DO UPDATE SET source_fingerprint=excluded.source_fingerprint,token=excluded.token,renewed_at=excluded.renewed_at
 WHERE track_waveform_leases.renewed_at<=?`, song, fingerprint, token, now, song, now-TrackAnalysisLeaseMillis, now-TrackAnalysisLeaseMillis)
	if err != nil {
		return "", false, err
	}
	n, err := result.RowsAffected()
	if n != 1 || err != nil {
		return "", false, err
	}
	return token, true, nil
}
func (d *DB) RenewWaveformLease(song, fingerprint, token string) error {
	now := time.Now().UnixMilli()
	result, err := d.conn.Exec(`UPDATE track_waveform_leases SET renewed_at=? WHERE song_id=? AND source_fingerprint=? AND token=? AND renewed_at>?`, now, song, fingerprint, token, now-TrackAnalysisLeaseMillis)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrTrackAnalysisLeaseLost
	}
	return nil
}
func (d *DB) ReleaseWaveformLease(song, token string) error {
	result, err := d.conn.Exec(`DELETE FROM track_waveform_leases WHERE song_id=? AND token=?`, song, token)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrTrackAnalysisLeaseLost
	}
	return nil
}

// PublishWaveformLease atomically consumes ownership and publishes only amplitude.
func (d *DB) PublishWaveformLease(token string, a TrackAnalysisArtifact) error {
	if token == "" || a.Kind != "local_amplitude" {
		return ErrTrackAnalysisLeaseLost
	}
	tx, err := d.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.Exec(`DELETE FROM track_waveform_leases WHERE song_id=? AND source_fingerprint=? AND token=? AND renewed_at>?`, a.SongID, a.SourceFingerprint, token, time.Now().UnixMilli()-TrackAnalysisLeaseMillis)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrTrackAnalysisLeaseLost
	}
	if err := upsertTrackAnalysisArtifact(tx, a); err != nil {
		return err
	}
	return tx.Commit()
}
