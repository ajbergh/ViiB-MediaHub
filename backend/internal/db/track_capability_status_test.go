package db

import (
	"path/filepath"
	"testing"
)

func TestTrackCapabilityStatusPersistsCurrentVersionAndRejectsOlderAttempt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.db")
	d, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = d.Close() }()
	if err := d.SaveSong(&Song{ID: "song", Title: "Song", Artist: "Artist", Album: "Album", FilePath: "song.wav", AddedAt: 1}); err != nil {
		t.Fatal(err)
	}
	state := TrackCapabilityStatus{SongID: "song", SourceFingerprint: "bytes1", Capability: "local_tempo", Version: "v1", State: "unavailable", Reason: "insufficient_evidence", UpdatedAt: 100}
	if err := d.PutTrackCapabilityStatuses([]TrackCapabilityStatus{state}); err != nil {
		t.Fatal(err)
	}
	older := state
	older.SourceFingerprint = "oldbytes"
	older.UpdatedAt = 99
	older.State = "available"
	if err := d.PutTrackCapabilityStatuses([]TrackCapabilityStatus{older}); err != nil {
		t.Fatal(err)
	}
	got, err := d.GetTrackCapabilityStatuses("song", "bytes1")
	if err != nil || got["local_tempo"].State != "unavailable" {
		t.Fatal("older attempt replaced current abstention")
	}
	state.Version = "v2"
	state.State = "available"
	state.Reason = ""
	state.UpdatedAt = 101
	if err := d.PutTrackCapabilityStatuses([]TrackCapabilityStatus{state}); err != nil {
		t.Fatal(err)
	}
	if err := d.Close(); err != nil {
		t.Fatal(err)
	}
	d, err = New(path)
	if err != nil {
		t.Fatal(err)
	}
	got, err = d.GetTrackCapabilityStatuses("song", "bytes1")
	if err != nil || len(got) != 1 || got["local_tempo"].Version != "v2" {
		t.Fatal("restart/version replacement failed")
	}
	wrong, err := d.GetTrackCapabilityStatuses("song", "bytes2")
	if err != nil || len(wrong) != 0 {
		t.Fatal("state applied to different source")
	}
	invalid := state
	invalid.State = "invented"
	state.UpdatedAt = 102
	state.State = "failed"
	if d.PutTrackCapabilityStatuses([]TrackCapabilityStatus{state, invalid}) == nil {
		t.Fatal("invalid batch accepted")
	}
	got, err = d.GetTrackCapabilityStatuses("song", "bytes1")
	if err != nil || got["local_tempo"].State != "available" {
		t.Fatal("invalid batch partially changed state")
	}
}
