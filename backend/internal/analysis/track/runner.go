// Runs analysis over a song selection with progress, cancellation, source resolution, and
// optional Spotify enrichment.

package track

import (
	"context"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/analysis"
	"github.com/ajbergh/viib-mediahub/internal/analysis/threeband"
	"github.com/ajbergh/viib-mediahub/internal/db"
	"github.com/ajbergh/viib-mediahub/internal/logger"
	spotifyanalysis "github.com/ajbergh/viib-mediahub/internal/spotify/analysis"
)

// RunProgress is the aggregate state of one analysis run. Counts are derived
// from per-track outcomes, not from a single opaque counter, so a partially
// finished run can be reported honestly.
type RunProgress struct {
	Total     int
	Processed int
	Analyzed  int
	Skipped   int
	Failed    int
	SongID    string
}

// Done reports how many tracks are settled, which is what the job row's
// progress counter tracks.
func (p RunProgress) Done() int { return p.Processed }

// RunOptions configures one pass over a work list.
type RunOptions struct {
	// EnrichValid permits Spotify enrichment while reusing current local DSP.
	EnrichValid     bool
	SpotifyFeatures func(context.Context, analysis.ResolvedSource) *spotifyanalysis.Observation
	// ProviderPreparation enriches independent provider resources; it supersedes SpotifyFeatures.
	ProviderPreparation func(context.Context, analysis.ResolvedSource) *spotifyanalysis.Observation
	Analysis            Options
	// AutoCueMode is snapshotted on durable jobs. A zero/invalid value retains
	// the public runner's historical fill-empty behavior.
	AutoCueMode db.AutomaticCuePointMode
	// ResolveSource optionally supplies an authenticated or otherwise remote
	// stream source. Local catalog analysis uses ResolveLocalSource by default.
	ResolveSource func(context.Context, string) (analysis.ResolvedSource, error)
	// Canceled is consulted between tracks. Cancellation is cooperative at
	// track granularity: suspending DSP mid-track would buy a few seconds of
	// latency at the cost of a much harder invariant.
	Canceled func() bool
	// Progress is called after each track settles.
	Progress func(RunProgress)
	// Throttle is consulted before each track and may block, which is how DJ
	// playback reduces background analysis pressure without an audio dropout.
	Throttle func(context.Context) error
}

// Run analyzes each song in the work list, skipping tracks whose durable
// record already came from the current analyzer and the current source bytes.
// It returns the final progress; a canceled run returns what it completed
// along with context.Canceled.
func Run(ctx context.Context, database *db.DB, registry *analysis.DecoderRegistry, songIDs []string, opts RunOptions) (RunProgress, error) {
	progress := RunProgress{Total: len(songIDs)}
	if opts.Analysis == (Options{}) {
		opts.Analysis = DefaultOptions()
	}
	opts.AutoCueMode = db.NormalizeAutomaticCuePointMode(string(opts.AutoCueMode))
	for _, songID := range songIDs {
		if err := ctx.Err(); err != nil {
			return progress, err
		}
		if opts.Canceled != nil && opts.Canceled() {
			return progress, context.Canceled
		}
		if opts.Throttle != nil {
			if err := opts.Throttle(ctx); err != nil {
				return progress, err
			}
		}
		provider := opts.ProviderPreparation
		if provider == nil {
			provider = opts.SpotifyFeatures
		}
		settled := analyzeOne(ctx, database, registry, songID, opts.Analysis, opts.AutoCueMode, opts.ResolveSource, provider, opts.EnrichValid)
		// A cancellation that arrived mid-decode leaves the track outstanding
		// rather than failed, so it must not be counted before returning.
		if err := ctx.Err(); err != nil {
			return progress, err
		}
		progress.Processed++
		progress.SongID = songID
		switch settled {
		case outcomeSkipped:
			progress.Skipped++
		case outcomeFailed:
			progress.Failed++
		default:
			progress.Analyzed++
		}
		if opts.Progress != nil {
			opts.Progress(progress)
		}
	}
	return progress, nil
}

type outcome int

const (
	outcomeAnalyzed outcome = iota
	outcomeSkipped
	outcomeFailed
)

// analyzeOne settles exactly one track. Every terminal condition is persisted,
// so a source that cannot be analyzed is not retried on the next run.
func analyzeOne(ctx context.Context, database *db.DB, registry *analysis.DecoderRegistry, songID string, opts Options, autoCueMode db.AutomaticCuePointMode, resolveSource func(context.Context, string) (analysis.ResolvedSource, error), spotifyFeatures func(context.Context, analysis.ResolvedSource) *spotifyanalysis.Observation, enrichValid bool) outcome {
	if resolveSource == nil {
		resolveSource = func(_ context.Context, id string) (analysis.ResolvedSource, error) {
			return analysis.ResolveLocalSource(database, id)
		}
	}
	source, err := resolveSource(ctx, songID)
	if err != nil {
		code, message := ClassifyError(err)
		if ctx.Err() != nil {
			return outcomeFailed
		}
		previous, previousErr := database.GetTrackAnalysis(songID)
		fingerprint := "unresolved:" + songID
		if previousErr == nil {
			fingerprint = previous.SourceFingerprint
		}
		priorStates, stateErr := database.GetTrackCapabilityStatuses(songID, fingerprint)
		if stateErr != nil {
			return outcomeFailed
		}
		if attempt, ok := priorStates["core_preparation"]; ok && attempt.Version == db.CorePreparationVersion && attempt.Reason == ErrorSourceUnavailable && attempt.RetryAt > time.Now().UnixMilli() {
			return outcomeSkipped
		}
		token, claimed, claimErr := database.ClaimTrackAnalysisLease(songID, fingerprint, AnalysisVersion, AlgorithmVersion)
		if claimErr != nil {
			return outcomeFailed
		}
		if !claimed {
			return outcomeSkipped
		}
		defer database.ReleaseTrackAnalysisLease(songID, token)
		var persistErr error
		if previousErr == nil {
			// A failed attempt is separate from the last-good source-bound projection.
			// The claim temporarily changed its row; restore exactly the prior facts.
			if previous.Status == db.TrackAnalysisRunning {
				previous.Status = db.TrackAnalysisPending
			}
			persistErr = database.PublishTrackPreparation(db.TrackPreparationPublication{Analysis: previous, ClaimToken: token, Capabilities: preparationStatuses(Result{SongID: songID, Source: analysis.ResolvedSource{Fingerprint: fingerprint}, PreparationError: code})})
		} else {
			persistErr = persistFailureClaimed(database, songID, analysis.ResolvedSource{}, code, message, token)
		}
		if persistErr != nil {
			logger.Analysis("track failed song_id=%q code=%q error=%q persist_error=%q", songID, code, message, persistErr)
		} else {
			logger.Analysis("track failed song_id=%q code=%q error=%q", songID, code, message)
		}
		return outcomeFailed
	}
	logger.Scan("analysis_track song_id=%q file=%q", songID, source.Name)
	states, stateErr := database.GetTrackCapabilityStatuses(songID, source.Fingerprint)
	if stateErr != nil {
		return outcomeFailed
	}
	deferred := true
	for capability, version := range preparationVersions() {
		s, ok := states[capability]
		if !ok || s.Version != version || (s.State != "unsupported" && !(s.State == "failed" && s.RetryAt > time.Now().UnixMilli())) {
			deferred = false
			break
		}
	}
	if deferred {
		logger.Scan("local_preparation song_id=%q status=deferred", songID)
		return outcomeSkipped
	}
	var repairMissing map[string]bool
	valid, err := database.TrackAnalysisValid(songID, source.Fingerprint, AnalysisVersion, AlgorithmVersion)
	if err == nil && valid {
		record, readErr := database.GetTrackAnalysis(songID)
		if readErr != nil {
			return outcomeFailed
		}
		missing, planErr := missingPreparation(database, source, record)
		if planErr != nil {
			return outcomeFailed
		}
		if len(missing) == 1 && missing["local_amplitude"] {
			return repairCurrentWaveform(ctx, database, registry, source, record, resolveSource)
		}
		if len(missing) > 0 {
			repairMissing = missing
			logger.Scan("local_preparation song_id=%q action=shared_repair missing=%d", songID, len(missing))
		} else {
			core := states["core_preparation"]
			lockedState, stateErr := database.LockedBeatGridCapability(songID, source.Fingerprint)
			if stateErr != nil {
				return outcomeFailed
			}
			lockedSettled := true
			if lockedState != nil {
				stored := states["local_beatgrid"]
				lockedSettled = stored.Version == lockedState.Version && stored.State == lockedState.State && stored.Reason == lockedState.Reason
			}
			if core.Version == db.CorePreparationVersion && core.State == "available" && lockedSettled {
				if enrichValid && spotifyFeatures != nil {
					return enrichCurrentScalars(ctx, database, source, spotifyFeatures, resolveSource)
				}
				logScanRecord(record, "already_current", "skipped")
				return outcomeSkipped
			}
			token, claimed, err := claimPreparation(ctx, database, songID, source.Fingerprint)
			if err != nil {
				return outcomeFailed
			}
			if !claimed {
				return outcomeSkipped
			}
			if err := database.PublishTrackPreparation(db.TrackPreparationPublication{Analysis: record, ClaimToken: token, Capabilities: []db.TrackCapabilityStatus{{SongID: songID, SourceFingerprint: source.Fingerprint, Capability: "core_preparation", Version: db.CorePreparationVersion, State: "available"}}}); err != nil {
				_ = database.ReleaseTrackAnalysisLease(songID, token)
				return outcomeFailed
			}
			if enrichValid && spotifyFeatures != nil {
				return enrichCurrentScalars(ctx, database, source, spotifyFeatures, resolveSource)
			}
			if record, readErr := database.GetTrackAnalysis(songID); readErr == nil {
				logScanRecord(record, "already_current", "skipped")
			}
			return outcomeSkipped
		}
	}
	if err != nil {
		logger.Analysis("validity check failed song_id=%q path=%q error=%q", songID, source.Path, err)
	}
	// Two overlapping jobs can expand the same selection. The claim ensures only
	// one of them decodes the file; the loser treats the track as another
	// worker's responsibility rather than duplicating the work.
	previous, _ := database.GetTrackAnalysis(songID)
	token, claimed, err := claimPreparation(ctx, database, songID, source.Fingerprint)
	if err != nil {
		logger.Analysis("track claim failed song_id=%q path=%q error=%q", songID, source.Path, err)
		return outcomeFailed
	}
	if !claimed {
		logger.Scan("analysis_skipped song_id=%q reason=claimed_by_another_worker", songID)
		return outcomeSkipped
	}

	ctx, stopLease := maintainTrackAnalysisLease(ctx, database, songID, source.Fingerprint, token, claimHeartbeatInterval)
	defer func() { stopLease(); _ = database.ReleaseTrackAnalysisLease(songID, token) }()

	// Resolve Spotify's source-bound scalar fields before choosing which local
	// scalar estimators to run. Local-only outputs still share the same PCM pass.
	lookupCtx, cancelLookup := context.WithTimeout(ctx, 45*time.Second)
	defer cancelLookup()
	var observation *spotifyanalysis.Observation
	if spotifyFeatures != nil {
		observation = spotifyFeatures(lookupCtx, source)
	}
	if ctx.Err() != nil {
		logger.Scan("analysis_canceled song_id=%q stage=provider_preparation", songID)
		_ = database.ReleaseTrackAnalysisLease(songID, token)
		return outcomeFailed
	}
	if previous.SourceFingerprint == source.Fingerprint {
		observation = retainSpotifyScalars(observation, previous)
	}
	analysisOpts := opts
	if observation != nil {
		analysisOpts.SkipTempo = observation.BPM != nil
		if analysisOpts.SkipTempo {
			analysisOpts.GridBPM = observation.BPM
		}
		analysisOpts.SkipKey = observation.Key != nil && observation.Mode != nil
		analysisOpts.ProviderBeatGrid = observation.ProviderBeatGrid
		analysisOpts.SkipThreeBand = observation.ProviderThreeBandAvailable
	}
	if override, overrideErr := database.GetTrackAnalysisOverride(songID); overrideErr == nil && override.BeatgridLocked {
		if locked, resolveErr := database.ResolveBeatGrid(songID, source.Fingerprint); resolveErr == nil && locked.Grid != nil {
			analysisOpts.ProviderBeatGrid = locked.Grid
		}
	}
	if analysisOpts.SkipThreeBand {
		if err := database.DeleteTrackAnalysisArtifactForSource(songID, source.Fingerprint, threeband.Kind, threeband.FormatVersion, threeband.AlgorithmVersion); err != nil {
			_ = database.ReleaseTrackAnalysisLease(songID, token)
			return outcomeFailed
		}
	}
	localEngine := "run"
	result, err := AnalyzeResolved(ctx, registry, source, analysisOpts)
	result.ProviderThreeBandAvailable = analysisOpts.SkipThreeBand
	if ctx.Err() != nil {
		logger.Scan("analysis_canceled song_id=%q stage=local_analysis", songID)
		_ = database.ReleaseTrackAnalysisLease(songID, token)
		return outcomeFailed
	}
	engine, reason := scanEngineDecision(observation, spotifyFeatures != nil)
	logger.Scan("analysis_start song_id=%q file=%q engine=%q reason=%q", songID, source.Name, engine, reason)
	if previous.SourceFingerprint == source.Fingerprint {
		result.PreviousSpotifyBindings = previous.SpotifyBindings
	}
	if err != nil {
		code, _ := ClassifyError(err)
		result.PreparationError = code
		logger.Scan("local_analysis_failed song_id=%q code=%q", songID, code)
	}
	if observation != nil {
		result.Spotify = observation
		result.Status = combinedStatus(result.Tempo.Known || observation.BPM != nil, result.Key.Known || (observation.Key != nil && observation.Mode != nil))
		// Keep usable Spotify dimensions even if the fallback decoder fails.
		if ctx.Err() == nil && (result.Tempo.Known || result.Key.Known || observation.BPM != nil || (observation.Key != nil && observation.Mode != nil)) {
			err = nil
			result.SongID = songID
			result.Source = source
		}
	}
	if err != nil {
		if ctx.Err() != nil {
			// Do not persist a cancellation as a track-level failure; the work
			// is still outstanding, so the claim has to be given back or an
			// immediate resume would skip it for the whole lease window.
			_ = database.ReleaseTrackAnalysisLease(songID, token)
			return outcomeFailed
		}
		code, message := ClassifyError(err)
		if persistErr := persistFailureClaimed(database, songID, source, code, message, token); persistErr != nil {
			logger.Analysis("track failed song_id=%q path=%q code=%q error=%q persist_error=%q", songID, source.Path, code, message, persistErr)
		} else {
			logger.Analysis("track failed song_id=%q path=%q code=%q error=%q", songID, source.Path, code, message)
		}
		return outcomeFailed
	}
	current, sourceErr := resolveSource(ctx, songID)
	if sourceErr != nil || ctx.Err() != nil || current.Fingerprint != source.Fingerprint {
		_ = database.ReleaseTrackAnalysisLease(songID, token)
		logger.Scan("analysis_canceled song_id=%q stage=persistence reason=source_changed_or_unavailable", songID)
		return outcomeFailed
	}
	if len(repairMissing) > 0 {
		result.RepairPrevious = &previous
		result.RepairCapabilities = repairMissing
	}
	if err := persistWithAutoCueModeClaimed(database, result, autoCueMode, token); err != nil {
		_ = database.ReleaseTrackAnalysisLease(songID, token)
		logger.Analysis("track persistence failed song_id=%q path=%q status=%q error=%q", songID, source.Path, result.Status, err)
		return outcomeFailed
	}
	logScanResult(result, localEngine)
	logger.Scan("local_artifacts song_id=%q energy=%t loudness=%t beatgrid=%t cue_suggestions=%t auto_cue_mode=%q", songID, result.EnergyLevel != nil, result.Loudness != nil, result.BeatGrid != nil, result.Features != nil && len(result.Features.CueSuggestions) > 0, autoCueMode)
	if code, message := resultIssue(result); code != "" {
		logger.Analysis("track incomplete song_id=%q path=%q status=%q code=%q error=%q tempo_known=%t tempo_crest=%.3f key_known=%t key_flatness=%.6f",
			songID, source.Path, result.Status, code, message, result.Tempo.Known, result.Tempo.OnsetCrestFactor, result.Key.Known, result.Key.Flatness)
	}
	if result.Status == db.TrackAnalysisFailed {
		return outcomeFailed
	}
	return outcomeAnalyzed
}

func retainSpotifyScalars(observation *spotifyanalysis.Observation, previous db.TrackAnalysis) *spotifyanalysis.Observation {
	retained := spotifyanalysis.Observation{}
	if observation != nil {
		retained = *observation
	}
	if retained.BPM == nil && previous.BPMSource != nil && *previous.BPMSource == db.EffectiveBPMSpotify {
		if previous.SpotifyBindings != nil && (previous.SpotifyBindings.BPM != nil && previous.SpotifyBindings.BPM.Eligible && (observation == nil || observation.TrackID == "" || observation.TrackID == previous.SpotifyBindings.BPM.TrackID)) {
			retained.BPM = previous.BPM
			retained.BPMRetained = true
		}
	}
	if (retained.Key == nil || retained.Mode == nil) && previous.KeySource != nil && *previous.KeySource == db.EffectiveKeySpotify && previous.KeyTonic != nil && previous.KeyMode != nil && previous.SpotifyBindings != nil && previous.SpotifyBindings.Key != nil && previous.SpotifyBindings.Key.Eligible && (observation == nil || observation.TrackID == "" || observation.TrackID == previous.SpotifyBindings.Key.TrackID) {
		retained.Key = previous.KeyTonic
		retained.KeyRetained = true
		mode := 1
		if *previous.KeyMode == "minor" {
			mode = 0
		}
		retained.Mode = &mode
	}
	if observation == nil && retained.BPM == nil && retained.Key == nil {
		return nil
	}
	return &retained
}

// Refreshing provider scalars must not run DSP again or replace measured
// loudness, energy, structure, or artifacts that already describe these bytes.
func enrichCurrentScalars(ctx context.Context, database *db.DB, source analysis.ResolvedSource, lookup func(context.Context, analysis.ResolvedSource) *spotifyanalysis.Observation, resolve func(context.Context, string) (analysis.ResolvedSource, error)) outcome {
	record, err := database.GetTrackAnalysis(source.SongID)
	if err != nil {
		return outcomeFailed
	}
	if record.SourceFingerprint != source.Fingerprint {
		return outcomeSkipped
	}
	switch record.Status {
	case db.TrackAnalysisComplete, db.TrackAnalysisPartial, db.TrackAnalysisFailed, db.TrackAnalysisUnsupported:
	default:
		return outcomeSkipped
	}
	if record.BPMSource != nil && *record.BPMSource == db.EffectiveBPMSpotify && record.KeySource != nil && *record.KeySource == db.EffectiveKeySpotify {
		if _, bandErr := database.GetTrackAnalysisArtifact(source.SongID, threeband.Kind, threeband.FormatVersion, threeband.AlgorithmVersion); bandErr != nil {
			logScanRecord(record, "already_current", "skipped")
			return outcomeSkipped
		}
	}
	token, claimed, err := claimPreparation(ctx, database, source.SongID, source.Fingerprint)
	if err != nil {
		return outcomeFailed
	}
	if !claimed {
		return outcomeSkipped
	}
	ctx, stopLease := maintainTrackAnalysisLease(ctx, database, source.SongID, source.Fingerprint, token, claimHeartbeatInterval)
	defer func() { stopLease(); _ = database.ReleaseTrackAnalysisLease(source.SongID, token) }()
	observation := lookup(ctx, source)
	current, sourceErr := resolve(ctx, source.SongID)
	if ctx.Err() != nil || sourceErr != nil || current.Fingerprint != source.Fingerprint {
		return outcomeFailed
	}
	if observation != nil && observation.ProviderThreeBandAvailable {
		if err := database.DeleteTrackAnalysisArtifactForSource(source.SongID, source.Fingerprint, threeband.Kind, threeband.FormatVersion, threeband.AlgorithmVersion); err != nil {
			_ = database.ReleaseTrackAnalysisLease(source.SongID, token)
			return outcomeFailed
		}
	}
	if observation == nil {
		if err := database.PublishTrackPreparation(db.TrackPreparationPublication{Analysis: record, ClaimToken: token}); err != nil {
			_ = database.ReleaseTrackAnalysisLease(source.SongID, token)
			return outcomeFailed
		}
		if ctx.Err() != nil {
			logger.Scan("analysis_canceled song_id=%q stage=spotify_enrichment", source.SongID)
		}
		logScanRecord(record, "spotify_unavailable_retained_existing", "reused")
		return outcomeSkipped
	}
	db.ApplySpotifyScalars(&record, *observation)
	if record.BPM != nil && record.KeyTonic != nil && record.KeyMode != nil {
		record.Status = db.TrackAnalysisComplete
		record.ErrorCode, record.ErrorMessage = nil, nil
	} else if record.BPM != nil || (record.KeyTonic != nil && record.KeyMode != nil) {
		record.Status = db.TrackAnalysisPartial
	}
	publication := db.TrackPreparationPublication{Analysis: record, ClaimToken: token}
	if observation.ProviderThreeBandAvailable {
		publication.Capabilities = []db.TrackCapabilityStatus{{SongID: source.SongID, SourceFingerprint: source.Fingerprint, Capability: threeband.Kind, Version: threeband.AlgorithmVersion, State: "available", Reason: "provider_three_band"}}
	}
	if err := database.PublishTrackPreparation(publication); err != nil {
		_ = database.ReleaseTrackAnalysisLease(source.SongID, token)
		return outcomeFailed
	}
	logScanRecord(record, "spotify_enriched", "reused")
	return outcomeAnalyzed
}
