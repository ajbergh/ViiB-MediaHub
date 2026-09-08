package dj

import (
	"testing"

	"github.com/ajbergh/viib-mediahub/internal/db"
)

func TestEffectiveBPMOverridesLegacyMetadataForScoring(t *testing.T) {
	song := db.Song{ID: "song", BPM: 90, Tempo: TempoSlow}
	context := NewScoreContext()
	context.EffectiveBPM[song.ID] = 128
	if got := getSongBPM(song, context); got != 128 {
		t.Fatalf("getSongBPM() = %d, want measured 128", got)
	}
	context.EffectiveBPM = nil
	if got := getSongBPM(song, context); got != 90 {
		t.Fatalf("getSongBPM() without measurement = %d, want legacy 90", got)
	}
}
