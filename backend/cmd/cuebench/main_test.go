package main

import (
	"github.com/ajbergh/viib-mediahub/internal/analysisbench"
	"testing"
)

func TestSelectionExcludesReservedAudioAndBalancesCollections(t *testing.T) {
	bpm := 120.0
	m := analysisbench.CorpusManifest{Version: "phase0-spotify-corpus-v1", EvidenceClass: "lawful-real-audio"}
	for _, tc := range []struct{ id, genre, split string }{{"reserved", "A", "held_out"}, {"a1", "A", "tuning"}, {"a2", "A", "tuning"}, {"b1", "B", "tuning"}} {
		m.Tracks = append(m.Tracks, analysisbench.CorpusTrack{ID: tc.id, Path: "does-not-exist.wav", Genre: tc.genre, Split: tc.split, License: "private-local", LabelSource: "test", ExpectedBPM: &bpm})
	}
	selected, err := selectTuning(m, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(selected) != 2 || selected[0].ID != "a1" || selected[1].ID != "b1" {
		t.Fatalf("unexpected selection: %#v", selected)
	}
}
