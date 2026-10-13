package analysis

import "testing"

func TestPreparationScalarsEligibilityAndPairs(t *testing.T) {
	bpm := 128.0
	key, otherKey, mode := 2, 8, 1
	confidence := .7
	detailed := Observation{TrackID: "recording", AccountContext: "account", SourceEndpoint: "audio_analysis", BPM: &bpm, BPMConfidence: &confidence, Key: &key, Mode: &mode}
	if got := PreparationScalars(nil, &detailed); got == nil || got.SourceEndpoint != "audio_analysis" || got.BPMConfidence != &confidence {
		t.Fatal("detailed-only fallback lost")
	}
	if PreparationScalars(nil, nil) != nil {
		t.Fatal("empty inputs fabricated result")
	}
	features := Observation{TrackID: "recording", AccountContext: "account", Key: &otherKey}
	got := PreparationScalars(&features, &detailed)
	if got.Key != &key || got.Mode != &mode || got.KeyOrigin != &detailed {
		t.Fatal("partial feature pair mixed with detailed mode")
	}
	for _, mismatch := range []Observation{{TrackID: "other", AccountContext: "account", BPM: &bpm, Key: &key, Mode: &mode}, {TrackID: "recording", AccountContext: "other", BPM: &bpm, Key: &key, Mode: &mode}} {
		got := PreparationScalars(&features, &mismatch)
		if got.BPM != nil || got.Mode != nil || got.Key != &otherKey {
			t.Fatal("mismatched identity merged")
		}
	}
	features.BPM, features.Mode = &bpm, &mode
	got = PreparationScalars(&features, &detailed)
	if got.BPMConfidence != nil || got.BPMOrigin != nil || got.KeyOrigin != nil || got.Key != &otherKey {
		t.Fatal("detailed overwrote feature precedence or supplied confidence")
	}
}
