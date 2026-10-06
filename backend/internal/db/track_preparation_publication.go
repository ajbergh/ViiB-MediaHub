package db

import (
	"database/sql"
	"errors"
)

type preparationExecutor interface {
	Exec(string, ...any) (sql.Result, error)
	QueryRow(string, ...any) *sql.Row
}

type TrackPreparationPublication struct {
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
