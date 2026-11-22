// Expands track preparation selections and checks claims, cancellation, and throttling.
package track

import (
	"context"
	"errors"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/analysis"
	"github.com/ajbergh/viib-mediahub/internal/analysis/beatgrid"
	"github.com/ajbergh/viib-mediahub/internal/db"
)

const selectionBatchSize = 300

// SelectionOptions configures preparation selection discovery.
type SelectionOptions struct {
	ResolveSource func(context.Context, string) (analysis.ResolvedSource, error)
	Canceled      func() bool
	Throttle      func(context.Context) error
}

func checkSelectionControl(ctx context.Context, canceled func() bool, throttle func(context.Context) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if canceled != nil && canceled() {
		return context.Canceled
	}
	if throttle != nil {
		if err := throttle(ctx); err != nil {
			return err
		}
	}
	return nil
}

func filterCapabilityStatuses(byCap map[string]db.TrackCapabilityStatus, fingerprint string) map[string]db.TrackCapabilityStatus {
	if len(byCap) == 0 {
		return nil
	}
	filtered := make(map[string]db.TrackCapabilityStatus, len(byCap))
	for capName, status := range byCap {
		if status.SourceFingerprint == fingerprint {
			filtered[capName] = status
		}
	}
	return filtered
}

// ExpandPreparationSelection adds source and bounded payload validation to the
// coarse SQL missing scan. It never opens audio or calls provider enrichment.
// A corrupt nonempty blob cannot be classified from identity/length alone.
func ExpandPreparationSelection(ctx context.Context, database *db.DB, selection db.AnalysisSelection, resolve func(context.Context, string) (analysis.ResolvedSource, error)) ([]string, error) {
	return ExpandPreparationSelectionWithOptions(ctx, database, selection, SelectionOptions{ResolveSource: resolve})
}

// ExpandPreparationSelectionWithOptions adds cooperative cancellation and
// throttle checks to chunked preparation discovery.
func ExpandPreparationSelectionWithOptions(ctx context.Context, database *db.DB, selection db.AnalysisSelection, opts SelectionOptions) ([]string, error) {
	if err := checkSelectionControl(ctx, opts.Canceled, opts.Throttle); err != nil {
		return nil, err
	}
	coarse, err := database.ExpandAnalysisSelection(selection, AnalysisVersion, AlgorithmVersion)
	if err != nil || selection.Mode != db.AnalysisSelectionMissing {
		return coarse, err
	}
	if err := checkSelectionControl(ctx, opts.Canceled, opts.Throttle); err != nil {
		return nil, err
	}
	candidates := selection
	candidates.Mode = db.AnalysisSelectionAll
	all, err := database.ExpandAnalysisSelection(candidates, AnalysisVersion, AlgorithmVersion)
	if err != nil {
		return nil, err
	}
	included := make(map[string]bool, len(coarse))
	for _, id := range coarse {
		included[id] = true
	}
	result := []string{}
	for start := 0; start < len(all); start += selectionBatchSize {
		if err := checkSelectionControl(ctx, opts.Canceled, opts.Throttle); err != nil {
			return nil, err
		}
		end := start + selectionBatchSize
		if end > len(all) {
			end = len(all)
		}
		chunk := all[start:end]

		sources := make(map[string]analysis.ResolvedSource, len(chunk))
		sourceErrs := make(map[string]error)
		if opts.ResolveSource == nil {
			songs, songErr := database.GetSongsByIDs(chunk)
			if songErr != nil {
				return nil, songErr
			}
			songByID := make(map[string]db.Song, len(songs))
			for _, song := range songs {
				songByID[song.ID] = song
			}
			for _, id := range chunk {
				if song, ok := songByID[id]; ok {
					src, sErr := analysis.ResolveLocalSongSource(song)
					if sErr != nil {
						sourceErrs[id] = sErr
					} else {
						sources[id] = src
					}
				} else {
					src, sErr := analysis.ResolveLocalSource(database, id)
					if sErr != nil {
						sourceErrs[id] = sErr
					} else {
						sources[id] = src
					}
				}
			}
		} else {
			for _, id := range chunk {
				if err := checkSelectionControl(ctx, opts.Canceled, opts.Throttle); err != nil {
					return nil, err
				}
				src, sErr := opts.ResolveSource(ctx, id)
				if errors.Is(sErr, context.Canceled) || errors.Is(sErr, context.DeadlineExceeded) {
					return nil, sErr
				}
				if err := ctx.Err(); err != nil {
					return nil, err
				}
				if sErr != nil {
					sourceErrs[id] = sErr
				} else {
					sources[id] = src
				}
			}
		}

		records, recordErr := database.GetTrackAnalysisBatch(chunk)
		if recordErr != nil {
			return nil, recordErr
		}
		nowMillis := time.Now().UnixMilli()
		liveCutoff := nowMillis - db.TrackAnalysisLeaseMillis
		activeIDs := make([]string, 0, len(chunk))
		fingerprints := make(map[string]string, len(chunk))
		for _, id := range chunk {
			record, hasRecord := records[id]
			if hasRecord && record.Status == db.TrackAnalysisRunning && record.AnalyzedAt != nil && *record.AnalyzedAt > liveCutoff {
				continue
			}
			var fp string
			if sErr := sourceErrs[id]; sErr != nil {
				fp = record.SourceFingerprint
				if fp == "" {
					fp = "unresolved:" + id
				}
			} else {
				fp = sources[id].Fingerprint
			}
			fingerprints[id] = fp
			activeIDs = append(activeIDs, id)
		}
		if len(activeIDs) == 0 {
			continue
		}

		allStates, stateErr := database.ListTrackCapabilityStatusesBatch(activeIDs)
		if stateErr != nil {
			return nil, stateErr
		}
		inspectIDs := make([]string, 0, len(activeIDs))
		filteredStates := make(map[string]map[string]db.TrackCapabilityStatus, len(activeIDs))
		for _, id := range activeIDs {
			fp := fingerprints[id]
			states := filterCapabilityStatuses(allStates[id], fp)
			core := states["core_preparation"]
			// An unavailable source is retryable, but not on every missing scan.
			if core.Version == db.CorePreparationVersion && core.State == "failed" && core.RetryAt > nowMillis {
				continue
			}
			record, hasRecord := records[id]
			if sourceErrs[id] != nil || !hasRecord {
				result = append(result, id)
				continue
			}
			if !record.ValidFor(fp, AnalysisVersion, AlgorithmVersion) || included[id] {
				result = append(result, id)
				continue
			}
			filteredStates[id] = states
			inspectIDs = append(inspectIDs, id)
		}
		if len(inspectIDs) == 0 {
			continue
		}

		overrides, ovErr := database.ListTrackAnalysisOverridesBatch(inspectIDs)
		if ovErr != nil {
			return nil, ovErr
		}
		needsPreparation := make(map[string]bool)
		err := database.VisitTrackAnalysisArtifactsBounded(ctx, inspectIDs, preparationArtifactSpecs(), func(id string, artifacts map[db.TrackAnalysisArtifactKey]db.TrackAnalysisArtifactLookup) error {
			source := sources[id]
			record := records[id]
			states := filteredStates[id]
			var overridePtr *db.TrackAnalysisOverride
			if o, ok := overrides[id]; ok {
				overridePtr = &o
			}
			needsPreparation[id] = len(missingPreparationWithLookup(source, record, states, artifacts, overridePtr)) > 0
			if overridePtr != nil && overridePtr.BeatgridLocked {
				bgKey := db.TrackAnalysisArtifactKey{
					SongID:           id,
					Kind:             beatgrid.ArtifactKind,
					FormatVersion:    beatgrid.FormatVersion,
					AlgorithmVersion: beatgrid.AlgorithmVersion,
				}
				bgLookup, bgExists := artifacts[bgKey]
				locked := db.LockedBeatGridCapabilityFromArtifact(id, fingerprints[id], overridePtr, bgLookup, bgExists)
				stored := states["local_beatgrid"]
				needsPreparation[id] = needsPreparation[id] || stored.Version != locked.Version || stored.State != locked.State || stored.Reason != locked.Reason
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
		for _, id := range inspectIDs {
			if needsPreparation[id] {
				result = append(result, id)
			}
		}
	}
	return result, nil
}

// ErrPreparationBusy keeps a durable job outstanding while a different worker
// owns matching work; counting that work as skipped would lose crash recovery.
var ErrPreparationBusy = errors.New("preparation is owned by another live worker")

// ExpandPreparationJobSelection discovers durable job work with a source resolver.
func ExpandPreparationJobSelection(ctx context.Context, database *db.DB, selection db.AnalysisSelection, resolve func(context.Context, string) (analysis.ResolvedSource, error)) ([]string, error) {
	return ExpandPreparationJobSelectionWithOptions(ctx, database, selection, SelectionOptions{ResolveSource: resolve})
}

// ExpandPreparationJobSelectionWithOptions discovers work and checks for live
// claims while honoring cancellation and throttling.
func ExpandPreparationJobSelectionWithOptions(ctx context.Context, database *db.DB, selection db.AnalysisSelection, opts SelectionOptions) ([]string, error) {
	selected, err := ExpandPreparationSelectionWithOptions(ctx, database, selection, opts)
	if err != nil {
		return nil, err
	}
	if err := checkPreparationSelectionClaims(ctx, database, selection, opts.Canceled, opts.Throttle); err != nil {
		return nil, err
	}
	return selected, nil
}

// CheckPreparationSelectionClaims detects claims acquired after discovery,
// before a durable job can report completion.
func CheckPreparationSelectionClaims(ctx context.Context, database *db.DB, selection db.AnalysisSelection) error {
	return checkPreparationSelectionClaims(ctx, database, selection, nil, nil)
}

func checkPreparationSelectionClaims(ctx context.Context, database *db.DB, selection db.AnalysisSelection, canceled func() bool, throttle func(context.Context) error) error {
	if err := checkSelectionControl(ctx, canceled, throttle); err != nil {
		return err
	}
	candidates := selection
	if candidates.Mode == db.AnalysisSelectionMissing {
		candidates.Mode = db.AnalysisSelectionAll
	}
	ids, err := database.ExpandAnalysisSelection(candidates, AnalysisVersion, AlgorithmVersion)
	if err != nil {
		return err
	}
	for start := 0; start < len(ids); start += selectionBatchSize {
		if err := checkSelectionControl(ctx, canceled, throttle); err != nil {
			return err
		}
		end := start + selectionBatchSize
		if end > len(ids) {
			end = len(ids)
		}
		chunk := ids[start:end]
		records, err := database.GetTrackAnalysisBatch(chunk)
		if err != nil {
			return err
		}
		liveCutoff := time.Now().UnixMilli() - db.TrackAnalysisLeaseMillis
		runningIDs := make([]string, 0)
		for _, id := range chunk {
			record, ok := records[id]
			if !ok || record.Status != db.TrackAnalysisRunning || record.AnalyzedAt == nil || *record.AnalyzedAt <= liveCutoff {
				continue
			}
			// An optional scalar enrichment can own the row while all required
			// preparation facts remain current. That does not leave core work unfinished.
			if record.AnalysisVersion != AnalysisVersion || record.AlgorithmVersion != AlgorithmVersion {
				return ErrPreparationBusy
			}
			runningIDs = append(runningIDs, id)
		}
		if len(runningIDs) == 0 {
			continue
		}
		allStates, stateErr := database.ListTrackCapabilityStatusesBatch(runningIDs)
		if stateErr != nil {
			return stateErr
		}
		overrides, ovErr := database.ListTrackAnalysisOverridesBatch(runningIDs)
		if ovErr != nil {
			return ovErr
		}
		err = database.VisitTrackAnalysisArtifactsBounded(ctx, runningIDs, preparationArtifactSpecs(), func(id string, artifacts map[db.TrackAnalysisArtifactKey]db.TrackAnalysisArtifactLookup) error {
			record := records[id]
			states := filterCapabilityStatuses(allStates[id], record.SourceFingerprint)
			core := states["core_preparation"]
			if core.Version != db.CorePreparationVersion || core.State != "available" {
				return ErrPreparationBusy
			}
			var overridePtr *db.TrackAnalysisOverride
			if o, ok := overrides[id]; ok {
				overridePtr = &o
			}
			missing := missingPreparationWithLookup(analysis.ResolvedSource{SongID: id, Fingerprint: record.SourceFingerprint}, record, states, artifacts, overridePtr)
			if len(missing) > 0 {
				return ErrPreparationBusy
			}
			return nil
		})
		if err != nil {
			return err
		}
	}
	return nil
}
