package db

import (
	"database/sql"
	"errors"
	"time"
)

type preparationExecutor interface {
	Exec(string, ...any) (sql.Result, error)
	QueryRow(string, ...any) *sql.Row
}

type TrackPreparationPublication struct {
	ClaimToken   string
	Analysis     TrackAnalysis
	Artifacts    []TrackAnalysisArtifact
	Cues         []DJHotCue
	CueMode      GeneratedCueMode
	ApplyCues    bool
	Capabilities []TrackCapabilityStatus
}

// PublishTrackPreparation commits one source-bound result and its completion
// together. CPU encoding happens before this transaction. Manual grid locks,
// cue ownership and deletion suppressions are resolved inside the transaction.
func (d *DB) PublishTrackPreparation(p TrackPreparationPublication) error {
	if err := d.EnsureTrackAnalysisSchema(); err != nil {
		return err
	}
	if len(p.Artifacts) > 16 || len(p.Capabilities) > 64 {
		return errors.New("preparation publication too large")
	}
	tx, err := d.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// Take the SQLite write lock and consume the generation before any domain
	// writes. Rollback restores the token if a later artifact/cue/status fails.
	if p.ClaimToken != "" {
		result, err := tx.Exec(`UPDATE track_analysis SET claim_token=NULL WHERE song_id=? AND source_fingerprint=? AND status=? AND claim_token=? AND analyzed_at>?`, p.Analysis.SongID, p.Analysis.SourceFingerprint, TrackAnalysisRunning, p.ClaimToken, time.Now().UnixMilli()-TrackAnalysisLeaseMillis)
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
	} else {
		if _, err := tx.Exec(`UPDATE track_analysis SET claim_token=claim_token WHERE song_id=?`, p.Analysis.SongID); err != nil {
			return err
		}
		var managed bool
		if err := tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM track_analysis WHERE song_id=? AND status=? AND COALESCE(claim_token,'')!='')`, p.Analysis.SongID, TrackAnalysisRunning).Scan(&managed); err != nil {
			return err
		}
		if managed {
			return ErrTrackAnalysisLeaseLost
		}
	}
	if err := upsertTrackAnalysis(tx, p.Analysis); err != nil {
		return err
	}
	for _, a := range p.Artifacts {
		if a.SongID != p.Analysis.SongID || a.SourceFingerprint != p.Analysis.SourceFingerprint {
			return errors.New("preparation artifact source mismatch")
		}
		if a.Kind == "beatgrid" {
			var locked bool
			if err := tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM track_analysis_overrides WHERE song_id=? AND beatgrid_locked=1)`, a.SongID).Scan(&locked); err != nil {
				return err
			}
			if locked {
				continue
			}
		}
		if err := upsertTrackAnalysisArtifact(tx, a); err != nil {
			return err
		}
	}
	if p.ApplyCues {
		for _, cue := range p.Cues {
			if cue.SourceFingerprint != p.Analysis.SourceFingerprint {
				return errors.New("preparation cue source mismatch")
			}
		}
		if err := applyGeneratedDJHotCuesTx(tx, p.Analysis.SongID, p.Cues, p.CueMode); err != nil {
			return err
		}
	}
	lockedState, err := lockedBeatGridCapability(tx, p.Analysis.SongID, p.Analysis.SourceFingerprint)
	if err != nil {
		return err
	}
	if lockedState != nil {
		found := false
		for i := range p.Capabilities {
			if p.Capabilities[i].Capability == "local_beatgrid" {
				p.Capabilities[i] = *lockedState
				found = true
			}
		}
		if !found {
			p.Capabilities = append(p.Capabilities, *lockedState)
		}
	}
	for _, s := range p.Capabilities {
		if s.SongID != p.Analysis.SongID || s.SourceFingerprint != p.Analysis.SourceFingerprint {
			return errors.New("preparation capability source mismatch")
		}
	}
	if err := putTrackCapabilityStatusesTx(tx, p.Capabilities); err != nil {
		return err
	}
	return tx.Commit()
}
