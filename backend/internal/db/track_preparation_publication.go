package db

import (
	"database/sql"
	"errors"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/analysis/featurecontract"
	"github.com/ajbergh/viib-mediahub/internal/analysis/threeband"
)

type preparationExecutor interface {
	Exec(string, ...any) (sql.Result, error)
	QueryRow(string, ...any) *sql.Row
}

type TrackPreparationPublication struct {
	// Set by the analyzer only after qualifying the replacement event payload.
	RetireLegacyEnergy bool
	// Retire only historical provider claims for this source, preserving local
	// capability status. The retirement shares publication ownership/rollback.
	ClearProviderThreeBand bool
	ClaimToken             string
	Analysis               TrackAnalysis
	Artifacts              []TrackAnalysisArtifact
	Cues                   []DJHotCue
	CueMode                GeneratedCueMode
	ApplyCues              bool
	Capabilities           []TrackCapabilityStatus
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
		// Retire only the known superseded energy representation, and only
		// after its source-bound replacement is in this same transaction.
		if p.RetireLegacyEnergy && a.Kind == featurecontract.ArtifactKind && a.FormatVersion == featurecontract.FormatVersion && a.AlgorithmVersion == featurecontract.AlgorithmVersion && a.Encoding == featurecontract.Encoding && a.Provenance == "measured" {
			if _, err := tx.Exec(`DELETE FROM track_analysis_artifacts WHERE song_id=? AND source_fingerprint=? AND kind=? AND format_version=? AND algorithm_version='energy-structure-v1'`, a.SongID, a.SourceFingerprint, a.Kind, a.FormatVersion); err != nil {
				return err
			}
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
	providerBands := false
	for _, s := range p.Capabilities {
		if s.SongID != p.Analysis.SongID || s.SourceFingerprint != p.Analysis.SourceFingerprint {
			return errors.New("preparation capability source mismatch")
		}
		if s.Capability == threeband.Kind && s.Version == threeband.AlgorithmVersion && s.State == "available" && s.Reason == "provider_three_band" {
			providerBands = true
		}
	}
	if providerBands {
		if p.ClearProviderThreeBand {
			return errors.New("conflicting provider three-band decisions")
		}
		for _, artifact := range p.Artifacts {
			if artifact.Kind == threeband.Kind && artifact.FormatVersion == threeband.FormatVersion && artifact.AlgorithmVersion == threeband.AlgorithmVersion {
				return errors.New("provider preparation cannot also publish local three-band output")
			}
		}
		// Provider selection and removal of redundant local output are one
		// publication. A late/lost claimant or a later SQL failure must never
		// delete a current worker's fallback artifact.
		if _, err := tx.Exec(`DELETE FROM track_analysis_artifacts WHERE song_id=? AND source_fingerprint=? AND kind=? AND format_version=? AND algorithm_version=?`, p.Analysis.SongID, p.Analysis.SourceFingerprint, threeband.Kind, threeband.FormatVersion, threeband.AlgorithmVersion); err != nil {
			return err
		}
	}
	if p.ClearProviderThreeBand {
		if _, err := tx.Exec(`DELETE FROM track_metadata_capability_status WHERE song_id=? AND source_fingerprint=? AND capability=? AND reason='provider_three_band'`, p.Analysis.SongID, p.Analysis.SourceFingerprint, threeband.Kind); err != nil {
			return err
		}
	}
	if err := putTrackCapabilityStatusesTx(tx, p.Capabilities); err != nil {
		return err
	}
	return tx.Commit()
}
