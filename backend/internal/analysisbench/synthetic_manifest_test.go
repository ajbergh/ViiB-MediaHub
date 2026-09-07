package analysisbench

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestSyntheticCorpusManifestIsStableAndOnlyContainsComparableFixtures(t *testing.T) {
	fixtures, err := Phase0SyntheticFixtures()
	if err != nil {
		t.Fatalf("Phase0SyntheticFixtures() error = %v", err)
	}
	first, err := SyntheticCorpusManifest(fixtures, "generated-audio")
	if err != nil {
		t.Fatalf("SyntheticCorpusManifest() error = %v", err)
	}
	second, err := SyntheticCorpusManifest(fixtures, "generated-audio")
	if err != nil {
		t.Fatalf("SyntheticCorpusManifest() second call error = %v", err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatal("synthetic manifest is not deterministic")
	}
	if len(first.Tracks) != 33 {
		t.Fatalf("manifest tracks = %d, want 33 comparable fixtures", len(first.Tracks))
	}
	heldOut := 0
	for _, track := range first.Tracks {
		if track.License != "generated" || track.Genre != "synthetic" {
			t.Fatalf("synthetic manifest track = %#v, want generated synthetic labels", track)
		}
		if track.Split == SplitHeldOut {
			heldOut++
		}
		if track.ID == "tempo-ramp-110-130" || track.ID == "sine-a4" || track.ID == "detuned-a4-438" || track.ID == "detuned-a4-442" {
			t.Fatalf("non-comparable fixture %q was included", track.ID)
		}
	}
	if heldOut != 11 {
		t.Fatalf("held-out synthetic tracks = %d, want 11", heldOut)
	}
	var metered *CorpusTrack
	for index := range first.Tracks {
		if first.Tracks[index].ID == "metered-6-8-120-eighth" {
			metered = &first.Tracks[index]
			break
		}
	}
	if metered == nil || metered.ExpectedBPM == nil || *metered.ExpectedBPM != 120 || metered.Path != filepath.Join("generated-audio", "metered-6-8-120-eighth.wav") {
		t.Fatalf("metered fixture manifest row = %#v", metered)
	}
}

func TestWriteSyntheticCorpusManifestIsNonOverwritingAndLoadable(t *testing.T) {
	fixtures, err := DefaultFixtures()
	if err != nil {
		t.Fatalf("DefaultFixtures() error = %v", err)
	}
	path := filepath.Join(t.TempDir(), "manifest.json")
	written, err := WriteSyntheticCorpusManifest(path, "audio", fixtures)
	if err != nil {
		t.Fatalf("WriteSyntheticCorpusManifest() error = %v", err)
	}
	loaded, err := LoadManifest(path)
	if err != nil {
		t.Fatalf("LoadManifest() error = %v", err)
	}
	if !reflect.DeepEqual(written, loaded) {
		t.Fatalf("loaded manifest = %#v, want %#v", loaded, written)
	}
	if _, err := WriteSyntheticCorpusManifest(path, "audio", fixtures); err == nil {
		t.Fatal("WriteSyntheticCorpusManifest overwrote an existing artifact")
	}
}
