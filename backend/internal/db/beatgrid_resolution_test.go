package db

import (
	"github.com/ajbergh/viib-mediahub/internal/analysis/beatgrid"
	"reflect"
	"testing"
)

func TestManualBeatGridAtomicSourceFencePreservesOtherOverrides(t *testing.T) {
	d := selectionDatabase(t)
	saveSelectionSong(t, d, "song", 1)
	bpm := 123.
	tonic := 2
	mode := "minor"
	if err := d.UpsertTrackAnalysisOverride(TrackAnalysisOverride{SongID: "song", BPM: &bpm, BPMLocked: true, KeyTonic: &tonic, KeyMode: &mode, KeyLocked: true}); err != nil {
		t.Fatal(err)
	}
	if err := d.RefreshTrackAnalysisSourceRevision("song", "fp"); err != nil {
		t.Fatal(err)
	}
	data, err := (beatgrid.Grid{Beats: []float64{0, .5, 1}, Provenance: beatgrid.ProvenanceManual}).Encode()
	if err != nil {
		t.Fatal(err)
	}
	a := TrackAnalysisArtifact{ID: "grid", SongID: "song", Kind: beatgrid.ArtifactKind, FormatVersion: beatgrid.FormatVersion, AlgorithmVersion: beatgrid.AlgorithmVersion, Encoding: beatgrid.Encoding, Provenance: "manual", SourceFingerprint: "fp", Data: data}
	if ok, err := d.SaveManualBeatGridIfSourceCurrent(a, true, nil); err != nil || !ok {
		t.Fatal(ok, err)
	}
	before, err := d.GetTrackAnalysisOverride("song")
	if err != nil || before.BPM == nil || *before.BPM != bpm || !before.KeyLocked || *before.KeyTonic != tonic {
		t.Fatal(before, err)
	}
	if _, err := d.conn.Exec(`CREATE TRIGGER fail_grid_state BEFORE INSERT ON track_metadata_capability_status BEGIN SELECT RAISE(ABORT,'state failure'); END`); err != nil {
		t.Fatal(err)
	}
	beforeArtifact, err := d.GetTrackAnalysisArtifact("song", beatgrid.ArtifactKind, beatgrid.FormatVersion, beatgrid.AlgorithmVersion)
	if err != nil {
		t.Fatal(err)
	}
	a.Data, err = (beatgrid.Grid{Beats: []float64{.1, .6, 1.1}, Provenance: beatgrid.ProvenanceManual}).Encode()
	if err != nil {
		t.Fatal(err)
	}
	changed := 130.
	if _, err := d.SaveManualBeatGridIfSourceCurrent(a, false, &changed); err == nil {
		t.Fatal("late transaction failure ignored")
	}
	afterArtifact, err := d.GetTrackAnalysisArtifact("song", beatgrid.ArtifactKind, beatgrid.FormatVersion, beatgrid.AlgorithmVersion)
	if err != nil || !reflect.DeepEqual(beforeArtifact, afterArtifact) {
		t.Fatal("failed transaction changed artifact", err)
	}
	after, err := d.GetTrackAnalysisOverride("song")
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("failed grid write mutated overrides", after, err)
	}
	if _, err := d.conn.Exec(`DROP TRIGGER fail_grid_state`); err != nil {
		t.Fatal(err)
	}
	if err := d.RefreshTrackAnalysisSourceRevision("song", "replacement"); err != nil {
		t.Fatal(err)
	}
	if ok, err := d.SaveManualBeatGridIfSourceCurrent(a, false, &changed); err != nil || ok {
		t.Fatal("stale grid saved", ok, err)
	}
	if ok, err := d.ResetBeatGridIfSourceCurrent("song", "fp"); err != nil || ok {
		t.Fatal("stale reset accepted", ok, err)
	}
	if ok, err := d.ResetBeatGridIfSourceCurrent("song", "replacement"); err != nil || !ok {
		t.Fatal(ok, err)
	}
	after, err = d.GetTrackAnalysisOverride("song")
	if err != nil || after.BeatgridLocked || after.BeatgridArtifactID != nil || *after.BPM != bpm || !after.KeyLocked {
		t.Fatal(after, err)
	}
}
