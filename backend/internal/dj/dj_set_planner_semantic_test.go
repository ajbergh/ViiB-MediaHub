package dj

import "testing"

func TestParseAndValidatePlanPreservesAndFallsBackPhaseSemanticQueries(t *testing.T) {
	planner := &LLMPlanner{}
	response := `{"intentSummary":"late-night atmospheric alternative","phases":[
		{"name":"Warm-up","targetEnergy":"low","targetTempo":"slow","targetMoods":["moody"],"targetCount":1,"minBPM":80,"maxBPM":100,"notes":"slow start","semanticQuery":"spacious nocturnal alternative","negativeSemanticQuery":"bright pop","styleHints":["atmospheric","Atmospheric"]},
		{"name":"Build","targetEnergy":"medium","targetTempo":"medium","targetMoods":["intense"],"targetCount":1,"minBPM":100,"maxBPM":120,"notes":"rising tension"},
		{"name":"Peak","targetEnergy":"high","targetTempo":"fast","targetMoods":["driving"],"targetCount":1,"minBPM":120,"maxBPM":140,"notes":"maximum drive"}
	]}`
	plan, err := planner.parseAndValidatePlan(response, DefaultPlanOptions(), LibraryContext{}, 3)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Phases[0].SemanticQuery != "spacious nocturnal alternative" || plan.Phases[0].NegativeSemanticQuery != "bright pop" || len(plan.Phases[0].StyleHints) != 1 {
		t.Fatalf("first phase=%#v", plan.Phases[0])
	}
	if plan.Phases[1].SemanticQuery == "" || plan.Phases[2].SemanticQuery == "" {
		t.Fatalf("fallback queries missing: %#v", plan.Phases)
	}
}
