package analysis

// CapabilityDefinition keeps semantic alternatives separate and makes gaps explicit.
// Unqualified estimators must never satisfy preparation by fabricating a value.
type CapabilityDefinition struct {
	Key                    string   `json:"key"`
	FieldGroup             string   `json:"fieldGroup"`
	SemanticMetric         string   `json:"semanticMetric"`
	Units                  string   `json:"units"`
	ValueType              string   `json:"valueType"`
	SpotifyResources       []string `json:"spotifyResources"`
	LocalEstimator         string   `json:"localEstimator,omitempty"`
	Dependencies           []string `json:"dependencies,omitempty"`
	SchemaVersion          int      `json:"schemaVersion"`
	EstimatorVersion       string   `json:"estimatorVersion,omitempty"`
	ValidationPolicy       string   `json:"validationPolicy"`
	RequiredForPreparation bool     `json:"requiredForPreparation"`
	FallbackPolicy         string   `json:"fallbackPolicy"`
}

// Capabilities returns a fresh inventory; callers cannot mutate shared policy.
func Capabilities() []CapabilityDefinition {
	scalar := func(key, metric, units, local string, required bool) CapabilityDefinition {
		fallback := "local_estimator_unavailable"
		if local != "" {
			fallback = "qualified_current_source"
		}
		return CapabilityDefinition{Key: key, FieldGroup: "audio_scalar", SemanticMetric: metric, Units: units, ValueType: "number", SpotifyResources: []string{"audio_features", "audio_analysis"}, LocalEstimator: local, SchemaVersion: 1, ValidationPolicy: "finite_field_range", RequiredForPreparation: required, FallbackPolicy: fallback}
	}
	values := []CapabilityDefinition{
		scalar("tempo_bpm", "tempo", "bpm", "tempo", true),
		scalar("key_mode", "tonic_and_mode", "pitch_class_and_mode", "key", true),
		scalar("time_signature", "measured_meter", "beats_per_bar", "", false),
		scalar("provider_loudness_db", "spotify_track_loudness", "dB", "", false),
		scalar("duration_seconds", "recording_duration", "seconds", "decoded_frames", false),
	}
	values[1].ValueType = "key_mode_pair"
	for _, name := range []string{"energy", "danceability", "acousticness", "instrumentalness", "liveness", "speechiness", "valence"} {
		definition := scalar("spotify_"+name+"_score", "spotify_"+name, "unit_interval", "", false)
		definition.SpotifyResources = []string{"audio_features"}
		definition.ValidationPolicy = "finite_0_1"
		values = append(values, definition)
	}
	for _, item := range []struct {
		key, metric, units, estimator string
		required                      bool
	}{
		{"integrated_lufs", "bs1770_integrated_loudness", "LUFS", "bs1770", true},
		{"local_energy_level", "dj_energy_level", "level_1_10", "features", true},
		{"local_structure", "energy_structure", "seconds", "features", true},
		{"local_amplitude", "amplitude_envelope", "normalized_peak", "waveform", true},
		{"local_three_band_estimate", "local_band_envelopes", "normalized_band_peak", "", false},
		{"beat_intervals", "measured_beat_intervals", "seconds", "", false},
		{"bar_intervals", "measured_downbeats", "seconds", "", false},
		{"tatum_intervals", "measured_subdivisions", "seconds", "", false},
		{"local_segments", "local_spectral_segments", "seconds", "", false},
		{"dj_cue_candidates", "dj_cue_candidates", "seconds", "features", true},
	} {
		d := scalar(item.key, item.metric, item.units, item.estimator, item.required)
		d.SpotifyResources = nil
		d.FieldGroup = "local_artifact"
		d.ValueType = "artifact"
		d.ValidationPolicy = "versioned_source_fingerprint"
		values = append(values, d)
	}
	for _, item := range []struct{ key, resource string }{{"spotify_three_band", "three_band_waveform"}, {"spotify_detailed_analysis", "audio_analysis"}} {
		values = append(values, CapabilityDefinition{Key: item.key, FieldGroup: "provider_artifact", SemanticMetric: item.key, Units: "provider_native", ValueType: "artifact", SpotifyResources: []string{item.resource}, SchemaVersion: 1, ValidationPolicy: "bounded_validated_artifact", FallbackPolicy: "independent_local_alternative"})
	}
	return values
}
