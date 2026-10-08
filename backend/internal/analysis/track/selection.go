package track

import (
	"context"
	"database/sql"
	"errors"
	"github.com/ajbergh/viib-mediahub/internal/analysis"
	"github.com/ajbergh/viib-mediahub/internal/db"
	"time"
)

// ExpandPreparationSelection adds source and bounded payload validation to the
// coarse SQL missing scan. It never opens audio or calls provider enrichment.
// A corrupt nonempty blob cannot be classified from identity/length alone.
func ExpandPreparationSelection(ctx context.Context, database *db.DB, selection db.AnalysisSelection, resolve func(context.Context, string) (analysis.ResolvedSource, error)) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	coarse, err := database.ExpandAnalysisSelection(selection, AnalysisVersion, AlgorithmVersion)
	if err != nil || selection.Mode != db.AnalysisSelectionMissing {
		return coarse, err
	}
	candidates := selection
	candidates.Mode = db.AnalysisSelectionAll
	all, err := database.ExpandAnalysisSelection(candidates, AnalysisVersion, AlgorithmVersion)
	if err != nil {
		return nil, err
	}
	included := map[string]bool{}
	for _, id := range coarse {
		included[id] = true
	}
	if resolve == nil {
		resolve = func(_ context.Context, id string) (analysis.ResolvedSource, error) {
			return analysis.ResolveLocalSource(database, id)
		}
	}
	result := []string{}
	for _, id := range all {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		source, sourceErr := resolve(ctx, id)
		if errors.Is(sourceErr, context.Canceled) || errors.Is(sourceErr, context.DeadlineExceeded) {
			return nil, sourceErr
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		record, recordErr := database.GetTrackAnalysis(id)
		if recordErr != nil && !errors.Is(recordErr, sql.ErrNoRows) {
			return nil, recordErr
		}
		if recordErr == nil && record.Status == db.TrackAnalysisRunning && record.AnalyzedAt != nil && *record.AnalyzedAt > time.Now().UnixMilli()-db.TrackAnalysisLeaseMillis {
			continue
		}
		fp := source.Fingerprint
		if sourceErr != nil {
			fp = record.SourceFingerprint
			if fp == "" {
				fp = "unresolved:" + id
			}
		}
		states, stateErr := database.GetTrackCapabilityStatuses(id, fp)
		if stateErr != nil {
			return nil, stateErr
		}
		core := states["core_preparation"]
		// An unavailable source is retryable, but not on every missing scan.
		if core.Version == db.CorePreparationVersion && core.State == "failed" && core.RetryAt > time.Now().UnixMilli() {
			continue
		}
		if sourceErr != nil || recordErr != nil {
			result = append(result, id)
			continue
		}
		valid, err := database.TrackAnalysisValid(id, fp, AnalysisVersion, AlgorithmVersion)
		if err != nil {
			return nil, err
		}
		if !valid || included[id] {
			result = append(result, id)
			continue
		}
		missing, err := missingPreparation(database, source, record)
		if err != nil {
			return nil, err
		}
		lockedState, stateErr := database.LockedBeatGridCapability(id, fp)
		if stateErr != nil {
			return nil, stateErr
		}
		if lockedState != nil {
			stored := states["local_beatgrid"]
			if stored.Version != lockedState.Version || stored.State != lockedState.State || stored.Reason != lockedState.Reason {
				missing["locked_beatgrid_status"] = true
			}
		}
		if len(missing) > 0 {
			result = append(result, id)
		}
	}
	return result, nil
}

// ErrPreparationBusy keeps a durable job outstanding while a different worker
// owns matching work; counting that work as skipped would lose crash recovery.
var ErrPreparationBusy = errors.New("preparation is owned by another live worker")

func ExpandPreparationJobSelection(ctx context.Context, database *db.DB, selection db.AnalysisSelection, resolve func(context.Context, string) (analysis.ResolvedSource, error)) ([]string, error) {
	selected, err := ExpandPreparationSelection(ctx, database, selection, resolve)
	if err != nil {
		return nil, err
	}
	if err := CheckPreparationSelectionClaims(ctx, database, selection); err != nil {
		return nil, err
	}
	return selected, nil
}

// Check again before a durable job completes: a claimant can arrive after
// discovery and make the runner skip work that still belongs to that owner.
func CheckPreparationSelectionClaims(ctx context.Context, database *db.DB, selection db.AnalysisSelection) error {
	candidates := selection
	if candidates.Mode == db.AnalysisSelectionMissing {
		candidates.Mode = db.AnalysisSelectionAll
	}
	ids, err := database.ExpandAnalysisSelection(candidates, AnalysisVersion, AlgorithmVersion)
	if err != nil {
		return err
	}
	for _, id := range ids {
		if err := ctx.Err(); err != nil {
			return err
		}
		record, err := database.GetTrackAnalysis(id)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if err == nil && record.Status == db.TrackAnalysisRunning && record.AnalyzedAt != nil && *record.AnalyzedAt > time.Now().UnixMilli()-db.TrackAnalysisLeaseMillis {
			// An optional scalar enrichment can own the row while all required
			// preparation facts remain current. That does not leave core work unfinished.
			if record.AnalysisVersion == AnalysisVersion && record.AlgorithmVersion == AlgorithmVersion {
				states, stateErr := database.GetTrackCapabilityStatuses(id, record.SourceFingerprint)
				if stateErr != nil {
					return stateErr
				}
				core := states["core_preparation"]
				if core.Version == db.CorePreparationVersion && core.State == "available" {
					missing, err := missingPreparation(database, analysis.ResolvedSource{SongID: id, Fingerprint: record.SourceFingerprint}, record)
					if err != nil {
						return err
					}
					if len(missing) == 0 {
						continue
					}
				}
			}
			return ErrPreparationBusy
		}
	}
	return nil
}
