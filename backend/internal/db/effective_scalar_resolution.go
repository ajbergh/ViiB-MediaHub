package db

import (
	"encoding/json"
	"time"
)

// ScalarCandidate is an already-admitted observation. Acquisition must enforce
// account/recording eligibility; resolution independently fences media identity.
type ScalarCandidate struct {
	SpotifyScalarField
	Source            string `json:"source"`
	SourceFingerprint string `json:"sourceFingerprint"`
	Locked            bool   `json:"locked,omitempty"`
}

type EffectiveScalar struct {
	Key      string           `json:"key"`
	State    string           `json:"state"`
	Selected *ScalarCandidate `json:"selected,omitempty"`
	LastGood *ScalarCandidate `json:"lastGood,omitempty"`
}

// ResolveEffectiveScalar applies manual > fresh provider > qualified local.
// Expired provider facts remain explicit inspection evidence, never silently
// replacing a current local fact. Different metrics and units cannot compete.
func ResolveEffectiveScalar(key, fingerprint string, candidates []ScalarCandidate) EffectiveScalar {
	result := EffectiveScalar{Key: key, State: "unknown"}
	if fingerprint == "" {
		return result
	}
	var manual, provider, local *ScalarCandidate
	for _, c := range candidates {
		if c.Key != key || c.SourceFingerprint != fingerprint || !validEffectiveScalarCandidate(c) {
			continue
		}
		candidate := c
		switch c.Source {
		case "manual":
			if c.Locked && (manual == nil || c.RetrievedAt.After(manual.RetrievedAt)) {
				manual = &candidate
			}
		case "spotify_private", "spotify_download_import":
			if c.Stale {
				if result.LastGood == nil || betterProviderScalarCandidate(c.SpotifyScalarField, result.LastGood.SpotifyScalarField) {
					result.LastGood = &candidate
				}
			} else if provider == nil || betterProviderScalarCandidate(c.SpotifyScalarField, provider.SpotifyScalarField) {
				provider = &candidate
			}
		case "local":
			if !c.Stale && c.AdapterRevision != "" && (local == nil || c.RetrievedAt.After(local.RetrievedAt)) {
				local = &candidate
			}
		}
	}
	switch {
	case manual != nil:
		result.Selected = manual
	case provider != nil:
		result.Selected = provider
	case local != nil:
		result.Selected = local
	}
	if result.Selected != nil {
		result.State = "available"
	}
	return result
}

// AnalysisScalarCandidates adapts existing source-bound BPM/key stores without
// copying provider compatibility projections into local observations.
func AnalysisScalarCandidates(a TrackAnalysis, o TrackAnalysisOverride, fingerprint string) []ScalarCandidate {
	result := append([]ScalarCandidate{}, o.Fields...)
	add := func(key, source, fp, algorithm string, locked bool, value any, confidence *float64, at int64) {
		raw, err := json.Marshal(value)
		if err != nil {
			return
		}
		metric, units, _ := providerScalarSemantics(key)
		result = append(result, ScalarCandidate{SpotifyScalarField: SpotifyScalarField{Key: key, Metric: metric, Units: units, Value: raw, Confidence: confidence, AdapterRevision: algorithm, RetrievedAt: time.UnixMilli(at).UTC()}, Source: source, SourceFingerprint: fp, Locked: locked})
	}
	keyValue := func(tonic *int, mode *string) any {
		if tonic == nil || mode == nil || (*mode != "major" && *mode != "minor") {
			return nil
		}
		m := 0
		if *mode == "major" {
			m = 1
		}
		return struct {
			Tonic int `json:"tonic"`
			Mode  int `json:"mode"`
		}{*tonic, m}
	}
	if o.BPMLocked {
		add("tempo_bpm", "manual", o.BPMSourceFingerprint, "", true, o.BPM, nil, 0)
	}
	if o.KeyLocked {
		add("key_mode", "manual", o.KeySourceFingerprint, "", true, keyValue(o.KeyTonic, o.KeyMode), nil, 0)
	}
	// SQL-NULL legacy energy columns are independently measured local evidence.
	// Explicit envelopes, including corrupt/empty ones, never reconstruct them.
	if !a.LocalScalarEnvelopePresent && a.SourceFingerprint == fingerprint &&
		(a.Status == TrackAnalysisComplete || a.Status == TrackAnalysisPartial || a.Status == TrackAnalysisFailed) &&
		a.EnergyLevel != nil && a.EnergyLevelConfidence != nil && a.EnergyAlgorithmVersion != nil {
		at := int64(0)
		if a.AnalyzedAt != nil {
			at = *a.AnalyzedAt
		}
		raw, err := json.Marshal(*a.EnergyLevel)
		field := SpotifyScalarField{Key: "local_energy_level", Metric: "local_energy_level", Units: "level_1_10", Value: raw,
			Confidence: a.EnergyLevelConfidence, AdapterRevision: *a.EnergyAlgorithmVersion, RetrievedAt: time.UnixMilli(at).UTC()}
		if err == nil && currentLocalScalarField(field) {
			result = append(result, ScalarCandidate{SpotifyScalarField: field, Source: "local", SourceFingerprint: a.SourceFingerprint})
		}
	}
	if a.SourceFingerprint == fingerprint {
		if l := CurrentLocalScalars(&a); l != nil {
			for _, field := range l.Fields {
				result = append(result, ScalarCandidate{SpotifyScalarField: field, Source: "local", SourceFingerprint: l.SourceFingerprint})
			}
			add("tempo_bpm", "local", l.SourceFingerprint, l.AlgorithmVersion, false, l.BPM, l.BPMConfidence, l.MeasuredAt)
			add("key_mode", "local", l.SourceFingerprint, l.AlgorithmVersion, false, keyValue(l.KeyTonic, l.KeyMode), l.KeyConfidence, l.MeasuredAt)
		}
	}
	return result
}

func ResolveAnalysisScalarFields(a TrackAnalysis, o TrackAnalysisOverride, fingerprint string, provider []SpotifyScalarField) []EffectiveScalar {
	candidates := AnalysisScalarCandidates(a, o, fingerprint)
	for _, field := range provider {
		source := "spotify_private"
		if field.DurableImport {
			source = "spotify_download_import"
		}
		candidates = append(candidates, ScalarCandidate{SpotifyScalarField: field, Source: source, SourceFingerprint: fingerprint})
	}
	keys := []string{"local_duration_seconds", "local_energy_level", "integrated_lufs_bs1770", "true_peak_dbtp", "tempo_bpm", "key_mode", "provider_loudness_db", "time_signature", "duration_seconds", "duration_milliseconds", "spotify_energy_score", "spotify_danceability_score", "spotify_acousticness_score", "spotify_instrumentalness_score", "spotify_liveness_score", "spotify_speechiness_score", "spotify_valence_score"}
	result := make([]EffectiveScalar, 0, len(keys))
	for _, key := range keys {
		result = append(result, ResolveEffectiveScalar(key, fingerprint, candidates))
	}
	return result
}

func validEffectiveScalarCandidate(c ScalarCandidate) bool {
	if c.Source == "manual" && c.Key == "local_energy_level" {
		return validLocalScalarField(c.SpotifyScalarField)
	}
	if c.Source == "local" {
		if currentLocalScalarField(c.SpotifyScalarField) {
			return true
		}
		if c.Key != "tempo_bpm" && c.Key != "key_mode" {
			return false
		}
	}
	return validProviderScalarCandidate(c.SpotifyScalarField)
}
