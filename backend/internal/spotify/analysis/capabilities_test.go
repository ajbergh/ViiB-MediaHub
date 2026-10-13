package analysis

import "testing"

func TestCapabilitySemanticsAndCoverage(t *testing.T) {
	seen := map[string]CapabilityDefinition{}
	for _, d := range Capabilities() {
		if d.Key == "" || d.SemanticMetric == "" || d.Units == "" || d.SchemaVersion <= 0 || d.ValidationPolicy == "" || d.FallbackPolicy == "" {
			t.Fatalf("incomplete definition: %+v", d)
		}
		if _, ok := seen[d.Key]; ok {
			t.Fatalf("duplicate %s", d.Key)
		}
		seen[d.Key] = d
	}
	if seen["provider_loudness_db"].Units == seen["integrated_lufs_bs1770"].Units {
		t.Fatal("provider dB conflated with LUFS")
	}
	for key, want := range map[string]struct{ metric, units string }{
		"local_duration_seconds": {"decoded_file_duration", "seconds"},
		"integrated_lufs_bs1770": {"bs1770_integrated_loudness", "LUFS"},
		"true_peak_dbtp":         {"bs1770_true_peak", "dBTP"},
	} {
		definition, ok := seen[key]
		if !ok || definition.SemanticMetric != want.metric || definition.Units != want.units || len(definition.SpotifyResources) != 0 {
			t.Errorf("local scalar capability %s = %+v, want metric=%q units=%q and no Spotify resource", key, definition, want.metric, want.units)
		}
	}
	if seen["spotify_energy_score"].LocalEstimator != "" || seen["spotify_energy_score"].RequiredForPreparation {
		t.Fatal("unqualified energy estimator claimed")
	}
	durationMS, ok := seen["duration_milliseconds"]
	if !ok || durationMS.SemanticMetric != "recording_duration" || durationMS.Units != "milliseconds" || durationMS.LocalEstimator != "" {
		t.Fatalf("catalog duration units or semantic distinction omitted: %+v", durationMS)
	}
	if !seen["local_energy_level"].RequiredForPreparation || !seen["dj_cue_candidates"].RequiredForPreparation {
		t.Fatal("local preparation omitted")
	}
	first := Capabilities()
	first[0].SpotifyResources[0] = "changed"
	if Capabilities()[0].SpotifyResources[0] == "changed" {
		t.Fatal("mutable shared registry")
	}
}

func TestSpotifyTimingCapabilitiesDeclareAudioAnalysisResources(t *testing.T) {
	byKey := map[string]CapabilityDefinition{}
	for _, capability := range Capabilities() {
		byKey[capability.Key] = capability
	}
	for _, key := range []string{"beat_intervals", "bar_intervals", "tatum_intervals"} {
		capability := byKey[key]
		if capability.FieldGroup != "provider_artifact" || len(capability.SpotifyResources) != 1 || capability.SpotifyResources[0] != "audio_analysis" {
			t.Fatalf("%s capability omits its Spotify source: %+v", key, capability)
		}
	}
}
