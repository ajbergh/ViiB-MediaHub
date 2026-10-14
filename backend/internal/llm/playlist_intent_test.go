// Tests and fixtures for playlist intent behavior.

package llm

import (
	"strings"
	"testing"
)

func TestParsePlaylistIntentResponseNormalizesAndBoundsValues(t *testing.T) {
	response := `{"intentSummary":"  darker  alternative  ","semanticQuery":"atmospheric alternative rock","negativeSemanticQuery":"bright pop","includeArtists":["Radiohead","radiohead"],"excludeArtists":["Nirvana"],"preferredGenres":["Alternative Rock"],"minYear":1999,"maxYear":1990,"yearConstraintHard":true,"discoveryBias":1.2,"familiarityBias":-0.4}`
	intent, err := ParsePlaylistIntentResponse(response, "90s alternative")
	if err != nil {
		t.Fatal(err)
	}
	if intent.IntentSummary != "darker alternative" || intent.MinYear != 1990 || intent.MaxYear != 1999 || len(intent.IncludeArtists) != 1 || intent.DiscoveryBias != 1 || intent.FamiliarityBias != 0 {
		t.Fatalf("intent=%#v", intent)
	}
}

func TestPlaylistAudioIntentSurvivesFallbackAndModelCorrections(t *testing.T) {
	intent, err := ParsePlaylistIntentResponse(`{"semanticQuery":"dance","minBPM":90,"maxBPM":100,"bpmConstraintHard":false,"minEnergyLevel":7,"maxEnergyLevel":9,"energyConstraintHard":true,"harmonicMix":true,"energyDirection":"build","targetDurationMinutes":45}`, "between 120 and 128 BPM")
	if err != nil || intent.MinBPM != 120 || intent.MaxBPM != 128 || !intent.BPMConstraintHard || !intent.EnergyConstraintHard || !intent.HarmonicMix || intent.TargetDurationMinutes != 45 {
		t.Fatalf("intent=%+v err=%v", intent, err)
	}
	fallback := FallbackPlaylistIntent("45 minutes of calm music around 100 BPM, energy level 2–4")
	if fallback.MinBPM != 100 || fallback.BPMConstraintHard || !fallback.EnergyConstraintHard || fallback.TargetDurationMinutes != 45 || len(fallback.PreferredMoods) == 0 {
		t.Fatalf("fallback=%+v", fallback)
	}
}

func TestParsePlaylistIntentResponseFallsBackForEmptyRequiredText(t *testing.T) {
	intent, err := ParsePlaylistIntentResponse(`{"semanticQuery":""}`, "  Quiet  Sunday  music ")
	if err != nil {
		t.Fatal(err)
	}
	if intent.SemanticQuery != "Quiet Sunday music" || intent.IntentSummary != "Quiet Sunday music" {
		t.Fatalf("intent=%#v", intent)
	}
}

func TestParsePlaylistIntentResponseRejectsUnknownFieldsAndCapsText(t *testing.T) {
	if _, err := ParsePlaylistIntentResponse(`{"semanticQuery":"x","unknown":true}`, "prompt"); err == nil {
		t.Fatal("unknown field accepted")
	}
	long := strings.Repeat("a", maxSemanticIntentTextLength+20)
	intent, err := ParsePlaylistIntentResponse(`{"semanticQuery":"`+long+`"}`, "prompt")
	if err != nil || len([]rune(intent.SemanticQuery)) != maxSemanticIntentTextLength {
		t.Fatalf("intent=%#v err=%v", intent, err)
	}
}

func TestFallbackPlaylistIntentSeparatesHardExclusionFromPositiveJazzQuery(t *testing.T) {
	intent := FallbackPlaylistIntent("new-school upbeat jazz - no christmas music")
	if intent.SemanticQuery != "new-school upbeat jazz" {
		t.Fatalf("positive query=%q", intent.SemanticQuery)
	}
	if len(intent.ExcludedTerms) != 1 || intent.ExcludedTerms[0] != "christmas" || intent.NegativeSemanticQuery != "christmas" {
		t.Fatalf("exclusions=%#v negative=%q", intent.ExcludedTerms, intent.NegativeSemanticQuery)
	}
	if len(intent.RequiredStyles) != 1 || intent.RequiredStyles[0] != "jazz" {
		t.Fatalf("required styles=%#v", intent.RequiredStyles)
	}
}

func TestNormalizePlaylistIntentCannotDropExplicitPromptConstraints(t *testing.T) {
	intent, err := ParsePlaylistIntentResponse(`{"intentSummary":"modern jazz","semanticQuery":"modern jazz christmas","negativeSemanticQuery":"","requiredStyles":[],"excludedTerms":[]}`, "modern jazz without christmas music")
	if err != nil {
		t.Fatal(err)
	}
	if intent.SemanticQuery != "modern jazz" || len(intent.ExcludedTerms) != 1 || intent.ExcludedTerms[0] != "christmas" || len(intent.RequiredStyles) != 1 || intent.RequiredStyles[0] != "jazz" {
		t.Fatalf("intent=%#v", intent)
	}
}

func TestFallbackPlaylistIntentExtractsMultipleExclusionClauses(t *testing.T) {
	intent := FallbackPlaylistIntent("upbeat jazz without christmas and no pop music")
	if intent.SemanticQuery != "upbeat jazz" {
		t.Fatalf("positive query=%q", intent.SemanticQuery)
	}
	if len(intent.ExcludedTerms) != 2 || intent.ExcludedTerms[0] != "christmas" || intent.ExcludedTerms[1] != "pop" {
		t.Fatalf("exclusions=%#v", intent.ExcludedTerms)
	}
	if len(intent.RequiredStyles) != 1 || intent.RequiredStyles[0] != "jazz" {
		t.Fatalf("required styles=%#v", intent.RequiredStyles)
	}
}
