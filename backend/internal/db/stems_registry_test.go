package db

import (
	"path/filepath"
	"testing"
)

func TestStemRegistryPersistsMetadataAndUnlinksWithoutTouchingPackage(t *testing.T) {
	database, err := New(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err = database.SaveSong(&Song{ID: "song", Title: "Song", FilePath: "source.wav", AddedAt: 1}); err != nil {
		t.Fatal(err)
	}
	set := StemSet{ID: "set", SongID: "song", SourceAudioHash: "sha256", AudioSHA256: "audio-sha", ModelName: "model", ModelVersion: "1", GeneratorName: "generator", GeneratorVersion: "2", Layout: "four", Status: "ready", SampleRate: 48000, Channels: 2, Frames: 100, DurationSeconds: 100.0 / 48000, PackagePath: "/packages/song.viibstems", DiscoverySource: "library", ManifestSchemaVersion: 1, ExplicitlyLinked: true, Stems: []StemArtifact{{Name: "vocals", RelativePath: "vocals.wav", SHA256: "stem-hash", SizeBytes: 100, SampleRate: 48000, Channels: 2, Frames: 100, Encoding: "pcm_s16le"}}}
	if err = database.UpsertStemSet(set); err != nil {
		t.Fatal(err)
	}
	sets, err := database.ListStemSets("song")
	if err != nil {
		t.Fatal(err)
	}
	if len(sets) != 1 || sets[0].Status != "ready" || sets[0].SourceAudioHash != "sha256" || !sets[0].ExplicitlyLinked || len(sets[0].Stems) != 1 || sets[0].Stems[0].RelativePath != "vocals.wav" {
		t.Fatalf("unexpected persisted stem set: %+v", sets)
	}
	if err = database.DeleteStemSet("song", "set"); err != nil {
		t.Fatal(err)
	}
	sets, err = database.ListStemSets("song")
	if err != nil || len(sets) != 0 {
		t.Fatalf("unlink did not clear only registry state: sets=%+v err=%v", sets, err)
	}
}

func TestStemLocationsReplaceAndSort(t *testing.T) {
	database, err := New(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err = database.SetStemLocations([]StemLocation{{ID: "b", Path: "/z/library", Enabled: true}, {ID: "a", Path: "/a/library", Enabled: false}}); err != nil {
		t.Fatal(err)
	}
	locations, err := database.ListStemLocations()
	if err != nil {
		t.Fatal(err)
	}
	if len(locations) != 2 || locations[0].Path != "/a/library" || locations[0].Enabled || locations[1].Path != "/z/library" {
		t.Fatalf("unexpected locations: %+v", locations)
	}
}
