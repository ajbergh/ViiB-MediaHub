package db

import (
	"encoding/json"
	"github.com/ajbergh/viib-mediahub/internal/analysis/featurecontract"
	"math"
	"testing"
	"time"
)

func TestEffectiveScalarPrecedenceAndSourceEligibility(t *testing.T) {
	at := time.Now()
	makeCandidate := func(source string, value string) ScalarCandidate {
		return ScalarCandidate{SpotifyScalarField: SpotifyScalarField{Key: "tempo_bpm", Metric: "tempo", Units: "bpm", Value: json.RawMessage(value), RetrievedAt: at, AdapterRevision: "test-v1"}, Source: source, SourceFingerprint: "current"}
	}
	local := makeCandidate("local", "120")
	provider := makeCandidate("spotify_download_import", "121")
	manual := makeCandidate("manual", "122")
	manual.Locked = true
	for _, tc := range []struct {
		name       string
		candidates []ScalarCandidate
		want       string
		lastGood   bool
	}{
		{"manual", []ScalarCandidate{local, provider, manual}, "manual", false},
		{"provider", []ScalarCandidate{local, provider}, "spotify_download_import", false},
		{"local", []ScalarCandidate{local}, "local", false},
		{"unknown", nil, "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := ResolveEffectiveScalar("tempo_bpm", "current", tc.candidates)
			if tc.want == "" {
				if got.State != "unknown" || got.Selected != nil {
					t.Fatal(got)
				}
			} else if got.Selected == nil || got.Selected.Source != tc.want {
				t.Fatal(got)
			}
		})
	}
	provider.Stale = true
	got := ResolveEffectiveScalar("tempo_bpm", "current", []ScalarCandidate{provider, local})
	if got.Selected == nil || got.Selected.Source != "local" || got.LastGood == nil {
		t.Fatalf("stale fallback: %+v", got)
	}
	manual.SourceFingerprint = "old"
	local.SourceFingerprint = "old"
	provider.SourceFingerprint = "old"
	got = ResolveEffectiveScalar("tempo_bpm", "current", []ScalarCandidate{manual, local, provider})
	if got.Selected != nil || got.LastGood != nil {
		t.Fatalf("old source admitted: %+v", got)
	}
}

func TestEffectiveScalarSeparatesMetricsAndPreservesZero(t *testing.T) {
	field := SpotifyScalarField{Key: "spotify_energy_score", Metric: "spotify_energy", Units: "unit_interval", Value: json.RawMessage("0")}
	c := ScalarCandidate{SpotifyScalarField: field, Source: "spotify_private", SourceFingerprint: "fp"}
	got := ResolveEffectiveScalar(field.Key, "fp", []ScalarCandidate{c})
	if got.Selected == nil || string(got.Selected.Value) != "0" {
		t.Fatal(got)
	}
	local := c
	local.Source = "local"
	local.AdapterRevision = "fake-v1"
	if got := ResolveEffectiveScalar(field.Key, "fp", []ScalarCandidate{local}); got.Selected != nil {
		t.Fatal("local algorithm fabricated provider score")
	}
	c.Metric = "local_energy_level"
	c.Units = "level_1_10"
	if got := ResolveEffectiveScalar(field.Key, "fp", []ScalarCandidate{c}); got.Selected != nil {
		t.Fatal("local energy substituted for Spotify score")
	}
	c.Key = "provider_loudness_db"
	c.Metric = "bs1770_integrated_loudness"
	c.Units = "LUFS"
	if got := ResolveEffectiveScalar(c.Key, "fp", []ScalarCandidate{c}); got.Selected != nil {
		t.Fatal("LUFS substituted for Spotify loudness")
	}
}

func TestAnalysisScalarAdapterPreservesCoupledZeroKey(t *testing.T) {
	tonic := 0
	mode := "minor"
	o := TrackAnalysisOverride{KeyLocked: true, KeyTonic: &tonic, KeyMode: &mode, KeySourceFingerprint: "fp"}
	fields := ResolveAnalysisScalarFields(TrackAnalysis{}, o, "fp", nil)
	for _, field := range fields {
		if field.Key == "key_mode" {
			if field.Selected == nil || string(field.Selected.Value) != `{"tonic":0,"mode":0}` {
				t.Fatalf("zero key lost: %+v", field)
			}
			return
		}
	}
	t.Fatal("missing key result")
}

func TestLocalMetricRegistryRejectsNullAndWrongDefinitions(t *testing.T) {
	base := SpotifyScalarField{Key: "integrated_lufs_bs1770", Metric: "bs1770_integrated_loudness", Units: "LUFS", Value: json.RawMessage("0"), AdapterRevision: "bs1770-v1", RetrievedAt: time.Now()}
	if !validLocalScalarField(base) {
		t.Fatal("finite zero loudness rejected")
	}
	for _, raw := range []string{"null", " null ", "1e999", "{}"} {
		f := base
		f.Value = json.RawMessage(raw)
		if validLocalScalarField(f) {
			t.Fatalf("accepted %q", raw)
		}
	}
	f := base
	f.Metric = "spotify_track_loudness"
	f.Units = "dB"
	if validLocalScalarField(f) {
		t.Fatal("provider definition admitted as local LUFS")
	}
}

func TestEffectiveLocalFieldRequiresCurrentRevision(t *testing.T) {
	field := SpotifyScalarField{Key: "local_duration_seconds", Metric: "decoded_file_duration", Units: "seconds", Value: json.RawMessage("12"), RetrievedAt: time.Now(), AdapterRevision: LocalDurationAlgorithmVersion}
	candidate := ScalarCandidate{SpotifyScalarField: field, Source: "local", SourceFingerprint: "fp"}
	if got := ResolveEffectiveScalar(field.Key, "fp", []ScalarCandidate{candidate}); got.Selected == nil {
		t.Fatal("current revision rejected")
	}
	candidate.AdapterRevision = "obsolete-decoder-v0"
	if !validLocalScalarField(candidate.SpotifyScalarField) {
		t.Fatal("historical observation cannot be retained")
	}
	if got := ResolveEffectiveScalar(field.Key, "fp", []ScalarCandidate{candidate}); got.Selected != nil {
		t.Fatal("obsolete revision applied")
	}
}

func TestGenericLocalMetricRevisionQualification(t *testing.T) {
	for _, tc := range []struct{ key, metric, units, revision, value string }{
		{"local_energy_level", "local_energy_level", "level_1_10", featurecontract.EnergyLevelAlgorithmVersion, "4"},
		{"integrated_lufs_bs1770", "bs1770_integrated_loudness", "LUFS", featurecontract.BS1770AlgorithmVersion, "-14"},
		{"true_peak_dbtp", "bs1770_true_peak", "dBTP", featurecontract.BS1770AlgorithmVersion, "-1"},
	} {
		t.Run(tc.key, func(t *testing.T) {
			c := ScalarCandidate{SpotifyScalarField: SpotifyScalarField{Key: tc.key, Metric: tc.metric, Units: tc.units, AdapterRevision: tc.revision, Value: json.RawMessage(tc.value), RetrievedAt: time.Now()}, Source: "local", SourceFingerprint: "fp"}
			if got := ResolveEffectiveScalar(tc.key, "fp", []ScalarCandidate{c}); got.Selected == nil {
				t.Fatal("current metric rejected")
			}
			c.AdapterRevision = "future-or-unknown"
			if got := ResolveEffectiveScalar(tc.key, "fp", []ScalarCandidate{c}); got.Selected != nil {
				t.Fatal("unqualified metric applied")
			}
		})
	}
}

func TestLegacyEnergyCanonicalQualification(t *testing.T) {
	level, confidence, version := 6, .8, featurecontract.EnergyLevelAlgorithmVersion
	base := TrackAnalysis{Status: TrackAnalysisComplete, SourceFingerprint: "fp", EnergyLevel: &level, EnergyLevelConfidence: &confidence, EnergyAlgorithmVersion: &version}
	for _, tc := range []struct {
		name   string
		change func(*TrackAnalysis)
		fp     string
		want   bool
	}{
		{"complete", func(*TrackAnalysis) {}, "fp", true},
		{"partial", func(a *TrackAnalysis) { a.Status = TrackAnalysisPartial }, "fp", true},
		{"failed", func(a *TrackAnalysis) { a.Status = TrackAnalysisFailed }, "fp", true},
		{"running", func(a *TrackAnalysis) { a.Status = TrackAnalysisRunning }, "fp", false},
		{"pending", func(a *TrackAnalysis) { a.Status = TrackAnalysisPending }, "fp", false},
		{"unsupported", func(a *TrackAnalysis) { a.Status = TrackAnalysisUnsupported }, "fp", false},
		{"replacement", func(*TrackAnalysis) {}, "next", false},
		{"no fingerprint", func(*TrackAnalysis) {}, "", false},
		{"explicit empty", func(a *TrackAnalysis) { a.LocalScalarEnvelopePresent = true; a.Local = &LocalScalarObservation{} }, "fp", false},
		{"reconstructed bpm", func(a *TrackAnalysis) {
			bpm := 120.0
			a.Local = &LocalScalarObservation{SourceFingerprint: "fp", AlgorithmVersion: "v1", BPM: &bpm}
		}, "fp", true},
		{"missing confidence", func(a *TrackAnalysis) { a.EnergyLevelConfidence = nil }, "fp", false},
		{"invalid confidence", func(a *TrackAnalysis) { v := math.NaN(); a.EnergyLevelConfidence = &v }, "fp", false},
		{"out of range confidence", func(a *TrackAnalysis) { v := 1.1; a.EnergyLevelConfidence = &v }, "fp", false},
		{"invalid level", func(a *TrackAnalysis) { v := 11; a.EnergyLevel = &v }, "fp", false},
		{"obsolete", func(a *TrackAnalysis) { v := "old"; a.EnergyAlgorithmVersion = &v }, "fp", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := base
			tc.change(&a)
			got := ResolveEffectiveScalar("local_energy_level", tc.fp, AnalysisScalarCandidates(a, TrackAnalysisOverride{}, tc.fp))
			if (got.Selected != nil) != tc.want {
				t.Fatalf("selection=%+v want=%v", got, tc.want)
			}
			if got.Selected != nil && (got.Selected.Source != "local" || string(got.Selected.Value) != "6" || got.Selected.RetrievedAt.IsZero()) {
				t.Fatalf("legacy provenance lost: %+v", got.Selected)
			}
		})
	}
}
