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
	if seen["provider_loudness_db"].Units == seen["integrated_lufs"].Units {
		t.Fatal("provider dB conflated with LUFS")
	}
	if seen["spotify_energy_score"].LocalEstimator != "" || seen["spotify_energy_score"].RequiredForPreparation {
		t.Fatal("unqualified energy estimator claimed")
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
