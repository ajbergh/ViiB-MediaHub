package db

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestPreparationPublicationRollsBackLateDatabaseFailure(t *testing.T) {
	d, err := New(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if err := d.SaveSong(&Song{ID: "song", Title: "Song", Artist: "Artist", Album: "Album", FilePath: "song.wav", AddedAt: 1}); err != nil {
		t.Fatal(err)
	}
	bpm, source := 120.0, "measured"
	old := TrackAnalysis{SongID: "song", Status: TrackAnalysisPartial, SourceFingerprint: "bytes", AnalysisVersion: 1, AlgorithmVersion: "v1", BPM: &bpm, BPMSource: &source}
	if err := d.UpsertTrackAnalysis(old); err != nil {
		t.Fatal(err)
	}
	old, _ = d.GetTrackAnalysis("song")
	artifact := TrackAnalysisArtifact{ID: "song:test", SongID: "song", Kind: "test", FormatVersion: 1, AlgorithmVersion: "v1", Encoding: "opaque", Provenance: "measured", SourceFingerprint: "bytes", Data: []byte("old")}
	if err := d.UpsertTrackAnalysisArtifact(artifact); err != nil {
		t.Fatal(err)
	}
	artifact, _ = d.GetTrackAnalysisArtifact("song", "test", 1, "v1")
	if err := d.SaveDJHotCues("song", []DJHotCue{{Slot: 1, Position: 3, Origin: "user", Label: "My cue", Locked: true}}); err != nil {
		t.Fatal(err)
	}
	cues, _ := d.GetDJHotCues("song")
	state := TrackCapabilityStatus{SongID: "song", SourceFingerprint: "bytes", Capability: "test", Version: "v1", State: "unavailable", Reason: "prior"}
	if err := d.PutTrackCapabilityStatuses([]TrackCapabilityStatus{state}); err != nil {
		t.Fatal(err)
	}
	states, _ := d.GetTrackCapabilityStatuses("song", "bytes")
	replacement := old
	newBPM := 130.0
	replacement.BPM = &newBPM
	newArtifact := artifact
	newArtifact.Data = []byte("new")
	confidence := .8
	state.State = "available"
	state.Reason = ""
	p := TrackPreparationPublication{Analysis: replacement, Artifacts: []TrackAnalysisArtifact{newArtifact}, ApplyCues: true, CueMode: GeneratedCueFillEmpty, Cues: []DJHotCue{{Slot: 2, Position: 5, Origin: "analysis", GeneratorVersion: "v1", Kind: "intro", Confidence: &confidence, SourceFingerprint: "bytes"}}, Capabilities: []TrackCapabilityStatus{state}}
	// Fail at the last publication stage, after scalar/artifact/cue SQL ran.
	if _, err := d.conn.Exec(`CREATE TRIGGER fail_completion BEFORE INSERT ON track_metadata_capability_status BEGIN SELECT RAISE(ABORT,'test completion failure'); END`); err != nil {
		t.Fatal(err)
	}
	if d.PublishTrackPreparation(p) == nil {
		t.Fatal("late failure ignored")
	}
	got, _ := d.GetTrackAnalysis("song")
	if !reflect.DeepEqual(old, got) {
		t.Fatal("scalar escaped rollback")
	}
	gotArtifact, _ := d.GetTrackAnalysisArtifact("song", "test", 1, "v1")
	if !reflect.DeepEqual(artifact, gotArtifact) {
		t.Fatal("artifact escaped rollback")
	}
	gotCues, _ := d.GetDJHotCues("song")
	if !reflect.DeepEqual(cues, gotCues) {
		t.Fatal("cue merge escaped rollback")
	}
	gotStates, _ := d.GetTrackCapabilityStatuses("song", "bytes")
	if !reflect.DeepEqual(states, gotStates) {
		t.Fatal("completion escaped rollback")
	}
	if _, err := d.conn.Exec(`DROP TRIGGER fail_completion`); err != nil {
		t.Fatal(err)
	}
	if err := d.PublishTrackPreparation(p); err != nil {
		t.Fatal(err)
	}
	got, _ = d.GetTrackAnalysis("song")
	gotArtifact, _ = d.GetTrackAnalysisArtifact("song", "test", 1, "v1")
	gotCues, _ = d.GetDJHotCues("song")
	gotStates, _ = d.GetTrackCapabilityStatuses("song", "bytes")
	if *got.BPM != newBPM || string(gotArtifact.Data) != "new" || len(gotCues) != 2 || gotCues[0].Origin != "user" || gotStates["test"].State != "available" {
		t.Fatal("successful retry did not publish all domains")
	}
	mismatched := p
	wrongBPM := 140.0
	mismatched.Analysis.BPM = &wrongBPM
	wrongArtifact := newArtifact
	wrongArtifact.SourceFingerprint = "other-bytes"
	mismatched.Artifacts = []TrackAnalysisArtifact{wrongArtifact}
	if d.PublishTrackPreparation(mismatched) == nil {
		t.Fatal("cross-source artifact accepted")
	}
	got, _ = d.GetTrackAnalysis("song")
	if *got.BPM != newBPM {
		t.Fatal("source rejection partially replaced scalars")
	}

}
