package dj

import (
	"testing"

	"github.com/ajbergh/viib-mediahub/internal/db"
)

func TestScoringUsesOnlyEffectiveLocalBPM(t *testing.T) {
	song := db.Song{ID: "song", BPM: 90, Tempo: TempoSlow}
	context := NewScoreContext()
	context.EffectiveBPM[song.ID] = 128
	if got := getSongBPM(song, context); got != 128 {
		t.Fatalf("getSongBPM() = %d, want measured 128", got)
	}
	context.EffectiveBPM = nil
	if got := getSongBPM(song, context); got != 0 {
		t.Fatalf("getSongBPM() without local measurement = %d, want unknown", got)
	}
}

func TestMeasuredEnergyOverridesTagForPhaseScoring(t *testing.T) {
	song := db.Song{ID: "song", Energy: "low"}
	context := NewScoreContext()
	context.EffectiveEnergy[song.ID] = .82
	measured := scoreSongEnergy(song, EnergyHigh, context)
	fallback := scoreEnergyMatch(song.Energy, EnergyHigh)
	if measured <= fallback {
		t.Fatalf("measured energy score = %f, tag fallback = %f; measured curve should win", measured, fallback)
	}
}
