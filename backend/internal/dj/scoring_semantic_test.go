package dj

import (
	"testing"

	"github.com/ajbergh/viib-mediahub/internal/db"
)

func TestScoreSongForPhasePrioritizesSemanticRelevanceWhenAvailable(t *testing.T) {
	phase := DJPhase{TargetEnergy: EnergyMedium, TargetTempo: TempoMedium, TargetMoods: []string{"calm"}, MinBPM: 100, MaxBPM: 120}
	first := db.Song{ID: "first", Artist: "A", Energy: EnergyMedium, Tempo: TempoMedium, Mood: "calm", BPM: 110}
	second := first
	second.ID, second.Artist = "second", "B"
	context := NewScoreContext()
	context.SemanticScores[first.ID] = 0.1
	context.SemanticScores[second.ID] = 0.9
	firstScore, firstBreakdown := ScoreSongForPhase(first, phase, context, GetPersona(PersonaFlowMaster))
	secondScore, secondBreakdown := ScoreSongForPhase(second, phase, context, GetPersona(PersonaFlowMaster))
	if secondScore <= firstScore || firstBreakdown.SemanticScore != 0.1 || secondBreakdown.SemanticScore != 0.9 {
		t.Fatalf("scores first=%f second=%f breakdowns=%#v %#v", firstScore, secondScore, firstBreakdown, secondBreakdown)
	}
}
