package db

import (
	"database/sql"
	"errors"
	"time"
)

const CorePreparationVersion = "core-preparation-v1"

type TrackCapabilityStatus struct {
	SongID            string
	SourceFingerprint string
	Capability        string
	Version           string
	State             string
	Reason            string
	RetryAt           int64
	UpdatedAt         int64
}

func ensureTrackCapabilitySchema(d *DB) error {
	_, err := d.conn.Exec(`CREATE TABLE IF NOT EXISTS track_metadata_capability_status (
 song_id TEXT NOT NULL REFERENCES songs(id) ON DELETE CASCADE,
 source_fingerprint TEXT NOT NULL, capability TEXT NOT NULL, version TEXT NOT NULL,
 state TEXT NOT NULL CHECK(state IN ('available','unavailable','unsupported','failed')),
 reason TEXT NOT NULL DEFAULT '', retry_at INTEGER NOT NULL DEFAULT 0, updated_at INTEGER NOT NULL,
 PRIMARY KEY(song_id, capability));`)
	return err
}

// Replace only current state for the supplied capabilities; this is not an
// unbounded attempt log. Completion follows validated output publication.
func (d *DB) PutTrackCapabilityStatuses(states []TrackCapabilityStatus) error {
	if err := d.EnsureTrackAnalysisSchema(); err != nil {
		return err
	}
	tx, err := d.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := putTrackCapabilityStatusesTx(tx, states); err != nil {
		return err
	}
	return tx.Commit()
}

func putTrackCapabilityStatusesTx(tx *sql.Tx, states []TrackCapabilityStatus) error {
	if len(states) > 64 {
		return errors.New("too many capability states")
	}
	for _, s := range states {
		if s.SongID == "" || s.SourceFingerprint == "" || s.Capability == "" || s.Version == "" || len(s.Reason) > 128 || len(s.SongID) > 1024 || len(s.SourceFingerprint) > 1024 || len(s.Capability) > 128 || len(s.Version) > 1024 || s.RetryAt < 0 {
			return errors.New("invalid capability identity")
		}
		switch s.State {
		case "available", "unavailable", "unsupported", "failed":
		default:
			return errors.New("invalid capability state")
		}
	}
	for _, s := range states {
		if s.UpdatedAt == 0 {
			s.UpdatedAt = time.Now().UnixMilli()
		}
		_, err := tx.Exec(`INSERT INTO track_metadata_capability_status(song_id, source_fingerprint, capability, version, state, reason, retry_at, updated_at) VALUES(?,?,?,?,?,?,?,?) ON CONFLICT(song_id,capability) DO UPDATE SET version=excluded.version,source_fingerprint=excluded.source_fingerprint,state=excluded.state,reason=excluded.reason,retry_at=excluded.retry_at,updated_at=excluded.updated_at WHERE excluded.updated_at>=track_metadata_capability_status.updated_at`, s.SongID, s.SourceFingerprint, s.Capability, s.Version, s.State, s.Reason, s.RetryAt, s.UpdatedAt)
		if err != nil {
			return err
		}
	}
	return nil
}

func (d *DB) GetTrackCapabilityStatuses(songID, fingerprint string) (map[string]TrackCapabilityStatus, error) {
	if err := d.EnsureTrackAnalysisSchema(); err != nil {
		return nil, err
	}
	rows, err := d.conn.Query(`SELECT capability,version,state,reason,retry_at,updated_at FROM track_metadata_capability_status WHERE song_id=? AND source_fingerprint=?`, songID, fingerprint)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := map[string]TrackCapabilityStatus{}
	for rows.Next() {
		s := TrackCapabilityStatus{SongID: songID, SourceFingerprint: fingerprint}
		if err := rows.Scan(&s.Capability, &s.Version, &s.State, &s.Reason, &s.RetryAt, &s.UpdatedAt); err != nil {
			return nil, err
		}
		result[s.Capability] = s
	}
	return result, rows.Err()
}
