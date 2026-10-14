// Tests and fixtures for sequencer phase pool behavior.

package dj

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/ajbergh/viib-mediahub/internal/db"
	"github.com/ajbergh/viib-mediahub/internal/playlist"
)

func TestAudioOrderingSurvivesLowStrictnessAndUsesActualDuration(t *testing.T) {
	for iteration := 0; iteration < 12; iteration++ {
		ctx := NewScoreContext()
		ctx.FlowStrictness = 0
		ctx.AudioIntent = playlist.AudioIntent{EnergyDirection: "build"}
		ctx.AudioFeatures = map[string]playlist.AudioFeatures{}
		var songs []db.Song
		for i := 0; i < 10; i++ {
			id := fmt.Sprint(i)
			songs = append(songs, db.Song{ID: id, Artist: id, Duration: 60})
			ctx.AudioFeatures[id] = playlist.AudioFeatures{DurationSeconds: 60, EnergyLevel: &playlist.Observation{Value: float64(i + 1), Confidence: 1, Source: "manual"}}
		}
		plan := &DJSetPlan{TargetDurationMin: 6, Phases: []DJPhase{{Name: "Build", TargetCount: 2, TargetEnergy: EnergyHigh}}}
		queue, phases, err := NewSequencer().BuildQueue(songs, plan, GetPersona(PersonaFlowMaster), ctx)
		if err != nil || len(queue) != 6 || phases[0].SongCount != 6 {
			t.Fatalf("queue=%v phases=%v err=%v", queue, phases, err)
		}
		for i := 1; i < len(queue); i++ {
			if ctx.AudioFeatures[queue[i].ID].EnergyLevel.Value < ctx.AudioFeatures[queue[i-1].ID].EnergyLevel.Value {
				t.Fatal("post-selection shuffle broke energy build")
			}
		}
		for i, id := range phases[0].SongIDs {
			if id != queue[i].ID {
				t.Fatal("phase order differs from queue")
			}
		}
	}
}

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

func TestPhaseExpandsWhenPreferredBucketContainsOnlyPreviouslyUsedTracks(t *testing.T) {
	used := db.Song{ID: "used", Artist: "First", Energy: EnergyLow, Tempo: TempoSlow}
	fresh := db.Song{ID: "fresh", Artist: "Second", Energy: EnergyHigh, Tempo: TempoFast}
	plan := &DJSetPlan{Phases: []DJPhase{{Name: "First", TargetEnergy: EnergyLow, TargetTempo: TempoSlow, TargetCount: 1}, {Name: "Second", TargetEnergy: EnergyLow, TargetTempo: TempoSlow, TargetCount: 1}}}
	queue, phases, err := NewSequencer().BuildQueueFromPhasePools([]PhaseCandidatePool{{Songs: []db.Song{used}}, {Songs: []db.Song{used, fresh}}}, plan, GetPersona(PersonaFlowMaster), NewScoreContext())
	if err != nil || len(queue) != 2 || queue[0].ID != "used" || queue[1].ID != "fresh" || phases[1].SongCount != 1 {
		t.Fatalf("queue=%v phases=%v err=%v", queue, phases, err)
	}
}
