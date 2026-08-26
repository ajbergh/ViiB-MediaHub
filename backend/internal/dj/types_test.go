package dj

import "testing"

func TestPlanCacheKeyUsesDecimalValuesAndTimeContext(t *testing.T) {
	base := PlanCacheKey{Provider: "ollama", Model: "model", NormalizedPrompt: "late night", Persona: PersonaFlowMaster, TargetDurationMin: 45, FlowStrictness: 60}
	if got := base.String(); got != "ollama|model|late night|FlowMaster|45|60|false" {
		t.Fatalf("cache key=%q", got)
	}
	duration := base
	duration.TargetDurationMin = 46
	strictness := base
	strictness.FlowStrictness = 61
	withTime := base
	withTime.UseTimeContext = true
	withTime.TimeBucket = "night"
	if base.String() == duration.String() || base.String() == strictness.String() || base.String() == withTime.String() {
		t.Fatal("distinct plan options produced the same cache key")
	}
}
