package track

import (
	"context"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/analysis"
	"github.com/ajbergh/viib-mediahub/internal/analysis/threeband"
	"github.com/ajbergh/viib-mediahub/internal/analysis/waveformartifact"
	"github.com/ajbergh/viib-mediahub/internal/db"
	"github.com/ajbergh/viib-mediahub/internal/logger"
)

func persistWaveform(database artifactWriter, result Result) error {
	if result.Waveform == nil {
		return nil
	}
	encoded, err := waveformartifact.Encode(*result.Waveform)
	if err != nil {
		return err
	}
	return database.UpsertTrackAnalysisArtifact(db.TrackAnalysisArtifact{ID: result.SongID + ":" + waveformartifact.AlgorithmVersion, SongID: result.SongID, Kind: waveformartifact.Kind, FormatVersion: waveformartifact.FormatVersion, AlgorithmVersion: waveformartifact.AlgorithmVersion, Encoding: waveformartifact.Encoding, Provenance: "measured", SourceFingerprint: result.Source.Fingerprint, Data: encoded})
}
func currentWaveform(database *db.DB, source analysis.ResolvedSource) bool {
	artifact, err := database.GetTrackAnalysisArtifact(source.SongID, waveformartifact.Kind, waveformartifact.FormatVersion, waveformartifact.AlgorithmVersion)
	if err != nil || artifact.Encoding != waveformartifact.Encoding || artifact.Provenance != "measured" || source.Fingerprint == "" || artifact.SourceFingerprint != source.Fingerprint {
		return false
	}
	_, err = waveformartifact.Decode(artifact.Data)
	return err == nil
}

// Repair only the missing waveform; current scalars, other DSP artifacts, cues
// and their timestamps are preserved. The existing claim fences overlapping jobs.
func repairCurrentWaveform(ctx context.Context, database *db.DB, registry *analysis.DecoderRegistry, source analysis.ResolvedSource, record db.TrackAnalysis, resolve func(context.Context, string) (analysis.ResolvedSource, error)) (result outcome) {
	token, claimed, err := claimPreparation(ctx, database, source.SongID, source.Fingerprint)
	if err != nil {
		return outcomeFailed
	}
	if !claimed {
		return outcomeSkipped
	}
	ctx, stopLease := maintainTrackAnalysisLease(ctx, database, source.SongID, source.Fingerprint, token, claimHeartbeatInterval)
	defer func() { stopLease(); _ = database.ReleaseTrackAnalysisLease(source.SongID, token) }()
	if currentWaveform(database, source) {
		current, err := resolve(ctx, source.SongID)
		if err != nil || ctx.Err() != nil || current.Fingerprint != source.Fingerprint {
			return outcomeFailed
		}
		if err := database.PublishTrackPreparation(db.TrackPreparationPublication{Analysis: record, ClaimToken: token}); err != nil {
			return outcomeFailed
		}
		return outcomeSkipped
	}
	overview, err := analysis.GenerateWaveformOverviewWithOpener(ctx, registry, source.Name, source.Open, analysis.DefaultWaveformResolution)
	if err != nil {
		code, _ := ClassifyError(err)
		current, sourceErr := resolve(ctx, source.SongID)
		if ctx.Err() == nil && sourceErr == nil && current.Fingerprint == source.Fingerprint {
			status := db.TrackCapabilityStatus{SongID: source.SongID, SourceFingerprint: source.Fingerprint, Capability: "local_amplitude", Version: waveformartifact.AlgorithmVersion, State: "failed", Reason: code, RetryAt: time.Now().Add(24 * time.Hour).UnixMilli()}
			if code == ErrorUnsupportedCodec {
				status.State = "unsupported"
				status.RetryAt = 0
			}
			_ = database.PublishTrackPreparation(db.TrackPreparationPublication{Analysis: record, ClaimToken: token, Capabilities: []db.TrackCapabilityStatus{status}})
		}
		logger.Scan("local_artifact song_id=%q capability=local_amplitude status=failed", source.SongID)
		return outcomeFailed
	}
	current, err := resolve(ctx, source.SongID)
	if err != nil || ctx.Err() != nil || current.Fingerprint != source.Fingerprint {
		logger.Scan("local_artifact song_id=%q capability=local_amplitude status=canceled reason=source_changed", source.SongID)
		return outcomeFailed
	}
	collector := &preparationArtifacts{}
	if err := persistWaveform(collector, Result{SongID: source.SongID, Source: source, Waveform: &overview}); err != nil {
		return outcomeFailed
	}
	statuses := []db.TrackCapabilityStatus{{SongID: source.SongID, SourceFingerprint: source.Fingerprint, Capability: "local_amplitude", Version: waveformartifact.AlgorithmVersion, State: "available"}, {SongID: source.SongID, SourceFingerprint: source.Fingerprint, Capability: "core_preparation", Version: db.CorePreparationVersion, State: "available"}}
	if err := database.PublishTrackPreparation(db.TrackPreparationPublication{Analysis: record, ClaimToken: token, Artifacts: collector.artifacts, Capabilities: statuses}); err != nil {
		return outcomeFailed
	}
	logger.Scan("local_artifact song_id=%q capability=local_amplitude status=available action=waveform_only_repair", source.SongID)
	return outcomeAnalyzed
}

func persistThreeBand(database artifactWriter, result Result) error {
	if result.LocalThreeBand == nil {
		return nil
	}
	encoded, err := threeband.Encode(*result.LocalThreeBand)
	if err != nil {
		return err
	}
	return database.UpsertTrackAnalysisArtifact(db.TrackAnalysisArtifact{ID: result.SongID + ":" + threeband.AlgorithmVersion, SongID: result.SongID, Kind: threeband.Kind, FormatVersion: threeband.FormatVersion, AlgorithmVersion: threeband.AlgorithmVersion, Encoding: threeband.Encoding, Provenance: "measured", SourceFingerprint: result.Source.Fingerprint, Data: encoded})
}
