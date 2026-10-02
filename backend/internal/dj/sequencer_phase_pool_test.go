package dj

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/ajbergh/viib-mediahub/internal/db"
)

func TestBuildQueueFromPhasePoolsUsesEachSemanticPoolAndPreventsReuse(t *testing.T) {
	plan := &DJSetPlan{Phases: []DJPhase{
		{Name: "Warm-up", TargetEnergy: EnergyLow, TargetTempo: TempoSlow, TargetCount: 1},
		{Name: "Peak", TargetEnergy: EnergyHigh, TargetTempo: TempoFast, TargetCount: 1},
	}}
	warmup := db.Song{ID: "warmup", Artist: "Warm Artist", Energy: EnergyLow, Tempo: TempoSlow, BPM: 90}
	peak := db.Song{ID: "peak", Artist: "Peak Artist", Energy: EnergyHigh, Tempo: TempoFast, BPM: 130}
	queue, phases, err := NewSequencer().BuildQueueFromPhasePools([]PhaseCandidatePool{
		{Songs: []db.Song{warmup}, SemanticScores: map[string]float64{warmup.ID: 0.9}},
		{Songs: []db.Song{peak}, SemanticScores: map[string]float64{peak.ID: 0.9}},
	}, plan, GetPersona(PersonaFlowMaster), NewScoreContext())
	if err != nil {
		t.Fatal(err)
	}
	if len(queue) != 2 || queue[0].ID != warmup.ID || queue[1].ID != peak.ID || len(phases) != 2 {
		t.Fatalf("queue=%#v phases=%#v", queue, phases)
	}
}

func TestBuildQueueFromPhasePoolsRejectsUncoveredOrEmptyPools(t *testing.T) {
	plan := &DJSetPlan{Phases: []DJPhase{{Name: "Warm-up", TargetCount: 1}}}
	if _, _, err := NewSequencer().BuildQueueFromPhasePools(nil, plan, GetPersona(PersonaFlowMaster), NewScoreContext()); err == nil {
		t.Fatal("missing pools were accepted")
	}
	if _, _, err := NewSequencer().BuildQueueFromPhasePools([]PhaseCandidatePool{{}}, plan, GetPersona(PersonaFlowMaster), NewScoreContext()); err == nil {
		t.Fatal("empty pool was accepted")
	}
}

func TestStochasticSelectionFiltersUsedBeforeWindowAndPreservesInput(t *testing.T) {
	ctx := NewScoreContext()
	scored := make([]ScoredSong, 120)
	for i := range scored {
		scored[i] = ScoredSong{Song: db.Song{ID: fmt.Sprint(i), Artist: fmt.Sprint(i)}, Score: 1}
		if i < 55 {
			ctx.UsedSongIDs[fmt.Sprint(i)] = true
		}
	}
	before := append([]ScoredSong(nil), scored...)
	selected := NewSequencer().selectWithStochasticity(scored, 60, 0, ctx)
	if len(selected) != 60 {
		t.Fatalf("phase truncated despite eligible candidates: %d", len(selected))
	}
	seen := map[string]bool{}
	for _, song := range selected {
		if ctx.UsedSongIDs[song.ID] || seen[song.ID] {
			t.Fatalf("reused %s", song.ID)
		}
		seen[song.ID] = true
	}
	if !reflect.DeepEqual(scored, before) {
		t.Fatal("caller pool was mutated")
	}
}
