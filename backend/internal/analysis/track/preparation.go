package track

import (
	"github.com/ajbergh/viib-mediahub/internal/analysis"
	"github.com/ajbergh/viib-mediahub/internal/analysis/beatgrid"
	"github.com/ajbergh/viib-mediahub/internal/analysis/features"
	"github.com/ajbergh/viib-mediahub/internal/analysis/threeband"
	"github.com/ajbergh/viib-mediahub/internal/analysis/waveformartifact"
	"github.com/ajbergh/viib-mediahub/internal/db"
	"time"
)

func preparationVersions() map[string]string {
	return map[string]string{threeband.Kind: threeband.AlgorithmVersion, "local_duration": db.LocalDurationAlgorithmVersion, "local_scalars": AlgorithmVersion, "local_energy": features.EnergyLevelAlgorithmVersion, "local_features": features.AlgorithmVersion, "local_loudness": features.BS1770AlgorithmVersion, "local_beatgrid": beatgrid.AlgorithmVersion, "local_amplitude": waveformartifact.AlgorithmVersion}
}

// A settled abstention is output too. Missing/corrupt available artifacts are
// repairable; unavailable output stays settled until bytes or version change.
func missingPreparation(database *db.DB, source analysis.ResolvedSource, record db.TrackAnalysis) (map[string]bool, error) {
	states, err := database.GetTrackCapabilityStatuses(source.SongID, source.Fingerprint)
	if err != nil {
		return nil, err
	}
	missing := map[string]bool{}
	versions := preparationVersions()
	artifactValid := func(kind string, format int, algorithm, encoding string, decode func([]byte) bool) bool {
		a, err := database.GetTrackAnalysisArtifact(source.SongID, kind, format, algorithm)
		return err == nil && a.SourceFingerprint == source.Fingerprint && a.Encoding == encoding && a.Provenance != "unknown" && decode(a.Data)
	}
	durationAvailable := db.HasCurrentLocalScalarField(record.Local, source.Fingerprint, "local_duration_seconds")
	localScalarsAvailable := record.Local != nil && record.Local.AlgorithmVersion == AlgorithmVersion && db.ValidLocalScalarObservation(record.Local, source.Fingerprint)
	localEnergyAvailable := record.EnergyLevel != nil && record.EnergyLevelConfidence != nil && record.EnergyAlgorithmVersion != nil && *record.EnergyAlgorithmVersion == features.EnergyLevelAlgorithmVersion
	if record.LocalScalarEnvelopePresent {
		localEnergyAvailable = localEnergyAvailable && db.HasCurrentLocalScalarField(record.Local, source.Fingerprint, "local_energy_level")
	}
	loudnessAvailable := false
	if artifact, artifactErr := database.GetTrackAnalysisArtifact(source.SongID, features.BS1770ArtifactKind, features.BS1770FormatVersion, features.BS1770AlgorithmVersion); artifactErr == nil && artifact.SourceFingerprint == source.Fingerprint && artifact.Encoding == features.BS1770Encoding && artifact.Provenance != "unknown" {
		if measurements, decodeErr := features.DecodeBS1770(artifact.Data); decodeErr == nil {
			loudnessAvailable = true
			if measurements.LoudnessStatus == "available" && measurements.IntegratedLUFS != nil {
				loudnessAvailable = loudnessAvailable && db.HasCurrentLocalScalarField(record.Local, source.Fingerprint, "integrated_lufs_bs1770")
			}
			if measurements.TruePeakStatus == "available" && measurements.TruePeakDBTP != nil {
				loudnessAvailable = loudnessAvailable && db.HasCurrentLocalScalarField(record.Local, source.Fingerprint, "true_peak_dbtp")
			}
		}
	}
	providerThreeBand := false
	if state, ok := states[threeband.Kind]; ok && state.Version == versions[threeband.Kind] && state.State == "available" && state.Reason == "provider_three_band" {
		providerThreeBand = true
	}
	valid := map[string]bool{
		"local_duration":  durationAvailable,
		threeband.Kind:    providerThreeBand || artifactValid(threeband.Kind, threeband.FormatVersion, threeband.AlgorithmVersion, threeband.Encoding, func(b []byte) bool { _, e := threeband.Decode(b); return e == nil }),
		"local_scalars":   localScalarsAvailable,
		"local_energy":    localEnergyAvailable,
		"local_amplitude": currentWaveform(database, source),
		"local_features": artifactValid(features.ArtifactKind, features.FormatVersion, features.AlgorithmVersion, features.Encoding, func(b []byte) bool {
			_, e := features.DecodeBounded(b, features.MaxStructureStatusArtifactBytes)
			return e == nil
		}),
		"local_loudness": loudnessAvailable,
		"local_beatgrid": artifactValid(beatgrid.ArtifactKind, beatgrid.FormatVersion, beatgrid.AlgorithmVersion, beatgrid.Encoding, func(b []byte) bool { _, e := beatgrid.Decode(b); return e == nil }),
	}
	locked := false
	if o, e := database.GetTrackAnalysisOverride(source.SongID); e == nil {
		locked = o.BeatgridLocked
	}
	for capability, version := range versions {
		if capability == "local_beatgrid" && locked {
			continue
		}
		if valid[capability] {
			continue
		}
		s, ok := states[capability]
		if ok && s.Version == version && (s.State == "unavailable" || s.State == "unsupported" || (s.State == "failed" && s.RetryAt > time.Now().UnixMilli())) {
			continue
		}
		missing[capability] = true
	}
	return missing, nil
}

func preparationStatuses(result Result) []db.TrackCapabilityStatus {
	available := map[string]bool{"local_duration": result.PreparationError == "" && result.DurationSeconds > 0, threeband.Kind: result.LocalThreeBand != nil || result.ProviderThreeBandAvailable, "local_scalars": result.PreparationError == "", "local_energy": result.EnergyLevel != nil, "local_features": result.Features != nil, "local_loudness": result.Loudness != nil, "local_amplitude": result.Waveform != nil, "local_beatgrid": result.BeatGrid != nil}
	states := []db.TrackCapabilityStatus{}
	for capability, version := range preparationVersions() {
		if result.RepairPrevious != nil && !result.RepairCapabilities[capability] {
			continue
		}
		s := db.TrackCapabilityStatus{SongID: result.SongID, SourceFingerprint: result.Source.Fingerprint, Capability: capability, Version: version, State: "unavailable", Reason: "insufficient_evidence"}
		if capability == "local_duration" && result.PreparationError == "" && result.DurationSeconds <= 0 {
			s.Reason = "no_decoded_duration"
		}
		if available[capability] {
			s.State = "available"
			s.Reason = ""
			if capability == threeband.Kind && result.ProviderThreeBandAvailable && result.LocalThreeBand == nil {
				s.Reason = "provider_three_band"
			}
		}
		if capability == threeband.Kind && !result.ProviderThreeBandAvailable && result.LocalThreeBand == nil && result.Waveform != nil && result.Waveform.SampleRate < 10000 {
			s.State = "unsupported"
			s.Reason = "sample_rate_unsupported"
		}
		if result.PreparationError != "" && !(capability == threeband.Kind && result.ProviderThreeBandAvailable) {
			s.State = "failed"
			s.Reason = result.PreparationError
			s.RetryAt = time.Now().Add(24 * time.Hour).UnixMilli()
			if result.PreparationError == ErrorUnsupportedCodec {
				s.State = "unsupported"
				s.RetryAt = 0
			}
		}
		states = append(states, s)
	}
	core := db.TrackCapabilityStatus{SongID: result.SongID, SourceFingerprint: result.Source.Fingerprint, Capability: "core_preparation", Version: db.CorePreparationVersion, State: "available"}
	if result.PreparationError != "" {
		core.State = "failed"
		core.Reason = result.PreparationError
		core.RetryAt = time.Now().Add(24 * time.Hour).UnixMilli()
		if result.PreparationError == ErrorUnsupportedCodec {
			core.State = "unsupported"
			core.RetryAt = 0
		}
	}
	return append(states, core)
}
