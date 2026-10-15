package api

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/analysis/features"
	"github.com/ajbergh/viib-mediahub/internal/db"
	"github.com/ajbergh/viib-mediahub/internal/playlist"
)

// A request owns one source-fenced snapshot across adaptive recall passes.
type playlistMetadataSnapshot struct {
	api          *API
	fingerprints map[string]string
	provider     map[string][]db.SpotifyScalarField
	analysis     map[string]TrackAnalysisFeatureResponse
	audio        map[string]playlist.AudioFeatures
	bpms         map[string]int
	runtime      *spotifyAuthRuntime
	fence        db.SpotifyMetadataReadFence
	private      bool
	artifacts    map[string]features.Result
}

func newPlaylistMetadataSnapshot(a *API) *playlistMetadataSnapshot {
	a.spotifyAuthMu.Lock()
	runtime := a.spotifyAuth
	a.spotifyAuthMu.Unlock()
	return &playlistMetadataSnapshot{api: a, runtime: runtime, fingerprints: make(map[string]string), provider: make(map[string][]db.SpotifyScalarField), analysis: make(map[string]TrackAnalysisFeatureResponse), audio: make(map[string]playlist.AudioFeatures), bpms: make(map[string]int), artifacts: make(map[string]features.Result)}
}

func (snapshot *playlistMetadataSnapshot) load(ctx context.Context, songs []db.Song) error {
	ids := make(map[string]struct{})
	byID := make(map[string]db.Song)
	for _, song := range songs {
		if _, known := snapshot.audio[song.ID]; !known {
			ids[song.ID] = struct{}{}
			byID[song.ID] = song
		}
	}
	if len(ids) == 0 {
		return ctx.Err()
	}
	fingerprints, err := snapshot.api.currentAnalysisSourceFingerprints(sortedSongIDs(ids))
	if err != nil {
		return err
	}
	for id := range ids {
		if _, known := fingerprints[id]; !known {
			fingerprints[id] = ""
		}
	}
	var provider map[string][]db.SpotifyScalarField
	if snapshot.runtime == nil {
		provider, err = snapshot.api.db.GetDownloadedSpotifyScalarCandidateBatchAll(fingerprints, time.Now())
	} else {
		readCtx, cancel := snapshot.runtime.requestContext(ctx)
		defer cancel()
		err = snapshot.runtime.withMetadataRead(readCtx, func(fence db.SpotifyMetadataReadFence) error {
			if snapshot.private && snapshot.fence != fence {
				return fmt.Errorf("playlist provider session changed during retrieval")
			}
			if fence.Pending {
				return nil
			}
			var readErr error
			provider, readErr = snapshot.api.db.GetSpotifyScalarCandidateBatchForRuntimeAll(fence, fingerprints, time.Now())
			if readErr == nil && hasPrivateDJScalarCandidate(provider) {
				snapshot.fence = fence
				snapshot.private = true
			}
			return readErr
		})
		// A disconnected optional provider must not prevent local/manual scoring.
		if err != nil && !snapshot.private && ctx.Err() == nil {
			provider, err = snapshot.api.db.GetDownloadedSpotifyScalarCandidateBatchAll(fingerprints, time.Now())
		}
	}
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	analysis, bpms, err := snapshot.api.resolveDJQueueAnalysis(fingerprints, provider)
	if err != nil {
		return err
	}
	for id, fingerprint := range fingerprints {
		feature := analysis[id]
		snapshot.fingerprints[id] = fingerprint
		snapshot.provider[id] = provider[id]
		snapshot.analysis[id] = feature
		snapshot.audio[id] = playlistAudioFeatures(feature, byID[id].Duration)
		if bpm, known := bpms[id]; known {
			snapshot.bpms[id] = bpm
		}
	}
	return nil
}

func scalarReliability(source string, confidence *float64) float64 {
	if source == "manual" {
		return 1
	}
	if confidence != nil && !math.IsNaN(*confidence) && !math.IsInf(*confidence, 0) {
		return math.Max(0, math.Min(1, *confidence))
	}
	// A source weight for current provider facts, not fabricated measurement confidence.
	if source == "spotify_private" || source == "spotify_download_import" || source == "spotify" {
		return .65
	}
	return 0
}

func playlistAudioFeatures(feature TrackAnalysisFeatureResponse, duration float64) playlist.AudioFeatures {
	value := playlist.AudioFeatures{DurationSeconds: duration, ProviderScores: make(map[string]playlist.Observation)}
	for _, effective := range feature.EffectiveFields {
		if effective.Selected == nil {
			continue
		}
		selected := effective.Selected
		reliability := scalarReliability(selected.Source, selected.Confidence)
		if effective.Key == "key_mode" {
			if feature.CamelotKey != nil && playlist.ValidCamelot(*feature.CamelotKey) {
				value.Key = &playlist.KeyObservation{Camelot: *feature.CamelotKey, Confidence: reliability, Source: selected.Source}
			}
			continue
		}
		var number float64
		if json.Unmarshal(selected.Value, &number) != nil || math.IsNaN(number) || math.IsInf(number, 0) {
			continue
		}
		observation := &playlist.Observation{Value: number, Confidence: reliability, Source: selected.Source}
		switch effective.Key {
		case "tempo_bpm":
			value.BPM = observation
		case "local_energy_level":
			value.EnergyLevel = observation
		case "local_duration_seconds":
			if number > 0 {
				value.DurationSeconds = number
			}
		case "duration_seconds":
			if value.DurationSeconds <= 0 && number > 0 {
				value.DurationSeconds = number
			}
		case "duration_milliseconds":
			if value.DurationSeconds <= 0 && number > 0 {
				value.DurationSeconds = number / 1000
			}
		default:
			if len(effective.Key) > 8 && effective.Key[:8] == "spotify_" {
				value.ProviderScores[effective.Key] = *observation
			}
		}
	}
	return value
}

// Bound artifact memory and keep file/audio IO out of ranking loops. Missing,
// corrupt, stale, and excess artifacts remain unknown, not synthetic silence.
func (snapshot *playlistMetadataSnapshot) loadArtifacts(songs []db.Song) error {
	rows, err := snapshot.api.db.ListTrackAnalysis()
	if err != nil {
		return err
	}
	byID := make(map[string]db.TrackAnalysis, len(rows))
	for _, row := range rows {
		byID[row.SongID] = row
	}
	metadata, err := snapshot.api.db.ListTrackAnalysisArtifactMetadata(features.ArtifactKind, features.FormatVersion, features.AlgorithmVersion)
	if err != nil {
		return err
	}
	wanted := make(map[string]bool)
	for _, song := range songs {
		wanted[song.ID] = true
	}
	var ids []string
	var bytes int64
	for _, artifact := range metadata {
		if !wanted[artifact.SongID] || len(ids) >= 128 || artifact.PayloadBytes <= 0 || artifact.PayloadBytes > features.MaxStructureStatusArtifactBytes || bytes+artifact.PayloadBytes > 64*1024*1024 {
			continue
		}
		if !currentEnergyArtifactMetadataMatchesSource(byID[artifact.SongID], snapshot.fingerprints[artifact.SongID], artifact) {
			continue
		}
		ids = append(ids, artifact.SongID)
		bytes += artifact.PayloadBytes
	}
	return snapshot.api.db.VisitTrackAnalysisArtifactPayloadsBounded(ids, features.ArtifactKind, features.FormatVersion, features.AlgorithmVersion, features.MaxStructureStatusArtifactBytes, func(id, fingerprint string, data []byte) error {
		if fingerprint == "" || snapshot.fingerprints[id] != fingerprint {
			return nil
		}
		result, err := features.DecodeBounded(data, features.MaxStructureStatusArtifactBytes)
		if err != nil {
			return nil
		}
		snapshot.artifacts[id] = result
		return nil
	})
}

func transitionMetadata(value playlist.AudioFeatures) features.TransitionMetadata {
	var metadata features.TransitionMetadata
	if value.BPM != nil {
		metadata.BPM = &value.BPM.Value
		metadata.BPMConfidence = &value.BPM.Confidence
		metadata.BPMSource = compatibilityScalarSource(value.BPM.Source)
	}
	if value.Key != nil {
		metadata.CamelotKey = &value.Key.Camelot
		metadata.KeyConfidence = &value.Key.Confidence
		metadata.KeySource = compatibilityScalarSource(value.Key.Source)
	}
	if value.EnergyLevel != nil {
		level := int(math.Round(value.EnergyLevel.Value))
		metadata.EnergyLevel = &level
		metadata.EnergyLevelConfidence = &value.EnergyLevel.Confidence
		metadata.EnergyLevelSource = value.EnergyLevel.Source
	}
	return metadata
}

func (snapshot *playlistMetadataSnapshot) transition(from, to string, intent playlist.AudioIntent) (float64, bool) {
	out, okOut := snapshot.artifacts[from]
	in, okIn := snapshot.artifacts[to]
	if !okOut || !okIn {
		return .5, false
	}
	kind := features.TransitionIntentHold
	switch intent.EnergyDirection {
	case "build":
		kind = features.TransitionIntentLift
	case "cooldown":
		kind = features.TransitionIntentReset
	}
	if intent.HarmonicMix && kind == features.TransitionIntentHold {
		kind = features.TransitionIntentHarmonic
	}
	score, err := features.ScoreTransitionWithMetadata(out, in, transitionMetadata(snapshot.audio[from]), transitionMetadata(snapshot.audio[to]), kind)
	if err == nil && intent.HarmonicMix && kind != features.TransitionIntentHarmonic {
		if harmonic, harmonicErr := features.ScoreTransitionWithMetadata(out, in, transitionMetadata(snapshot.audio[from]), transitionMetadata(snapshot.audio[to]), features.TransitionIntentHarmonic); harmonicErr == nil {
			score.Score = (score.Score + harmonic.Score) / 2
		}
	}
	return score.Score, err == nil
}

func (snapshot *playlistMetadataSnapshot) publish(ctx context.Context, emit func() error) error {
	validate := func() error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := snapshot.api.validateDJQueuePublication(snapshot.fingerprints, snapshot.provider, snapshot.analysis, snapshot.bpms); err != nil {
			return err
		}
		return emit()
	}
	if snapshot.private && snapshot.runtime != nil {
		readCtx, cancel := snapshot.runtime.requestContext(ctx)
		defer cancel()
		return snapshot.runtime.withMetadataRead(readCtx, func(fence db.SpotifyMetadataReadFence) error {
			if fence != snapshot.fence {
				return fmt.Errorf("playlist provider session changed before publication")
			}
			return validate()
		})
	}
	return validate()
}
