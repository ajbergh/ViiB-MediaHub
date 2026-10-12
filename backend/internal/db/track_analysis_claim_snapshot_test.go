package db

import (
	"errors"
	"path/filepath"
	"reflect"
	"testing"
)

func TestTrackAnalysisClaimSnapshotPreservesLatestTerminalProjection(t *testing.T) {
	d, err := New(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if err := d.SaveSong(&Song{ID: "song", Title: "Song", FilePath: "song.wav"}); err != nil {
		t.Fatal(err)
	}
	token, prior, claimed, err := d.ClaimTrackAnalysisLeaseWithSnapshot("song", "old-bytes", 1, "old-version")
	if err != nil || !claimed || prior != nil {
		t.Fatalf("new claim: %v %v %v", prior, claimed, err)
	}
	bpm := 123.0
	analyzedAt := int64(12345)
	record := TrackAnalysis{SongID: "song", Status: TrackAnalysisPartial, SourceFingerprint: "old-bytes", AnalysisVersion: 1, AlgorithmVersion: "old-version", BPM: &bpm, AnalyzedAt: &analyzedAt}
	if err := d.PublishTrackPreparation(TrackPreparationPublication{Analysis: record, ClaimToken: token}); err != nil {
		t.Fatal(err)
	}
	want, err := d.GetTrackAnalysis("song")
	if err != nil {
		t.Fatal(err)
	}
	token, prior, claimed, err = d.ClaimTrackAnalysisLeaseWithSnapshot("song", "new-bytes", 2, "new-version")
	if err != nil || !claimed || prior == nil || !reflect.DeepEqual(*prior, want) {
		t.Fatalf("snapshot changed prior facts: %+v want=%+v claimed=%v err=%v", prior, want, claimed, err)
	}
	if token == "" {
		t.Fatal("missing ownership generation")
	}
	if _, prior, claimed, err := d.ClaimTrackAnalysisLeaseWithSnapshot("song", "new-bytes", 2, "new-version"); err != nil || claimed || prior != nil {
		t.Fatalf("live claim leaked snapshot: %+v %v %v", prior, claimed, err)
	}
	if prior.ValidFor("new-bytes", 2, "new-version") {
		t.Fatal("claim rewrote snapshot validity")
	}
	if err := d.RenewTrackAnalysisLease("song", "new-bytes", token); err != nil {
		t.Fatal(err)
	}
	if err := d.ReleaseTrackAnalysisLease("song", token); err != nil {
		t.Fatal(err)
	}
	waveToken, claimed, err := d.ClaimWaveformLease("song", "new-bytes")
	if err != nil || !claimed {
		t.Fatal(claimed, err)
	}
	defer d.ReleaseWaveformLease("song", waveToken)
	if _, prior, claimed, err := d.ClaimTrackAnalysisLeaseWithSnapshot("song", "new-bytes", 2, "new-version"); !errors.Is(err, ErrWaveformLeaseBusy) || claimed || prior != nil {
		t.Fatalf("waveform exclusion: %+v %v %v", prior, claimed, err)
	}
}

func TestTrackAnalysisClaimComparesObservedRevisionBeforeMutation(t *testing.T) {
	d, err := New(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if err := d.SaveSong(&Song{ID: "song", Title: "Song", FilePath: "song.wav"}); err != nil {
		t.Fatal(err)
	}
	oldFP := "old-bytes"
	old := TrackAnalysis{SongID: "song", Status: TrackAnalysisPartial, SourceFingerprint: oldFP, AnalysisVersion: 1, AlgorithmVersion: "v1"}
	if err := d.UpsertTrackAnalysis(old); err != nil {
		t.Fatal(err)
	}
	// A real source replacement is allowed when the observed prior row matches.
	token, snapshot, claimed, err := d.ClaimTrackAnalysisLeaseIfUnchanged("song", "new-bytes", 2, "v2", &oldFP)
	if err != nil || !claimed || snapshot == nil || snapshot.SourceFingerprint != oldFP {
		t.Fatal(snapshot, claimed, err)
	}
	latest := old
	latest.SourceFingerprint, latest.AlgorithmVersion, latest.AnalysisVersion = "new-bytes", "v2", 2
	bpm := 111.0
	latest.BPM = &bpm
	if err := d.PublishTrackPreparation(TrackPreparationPublication{Analysis: latest, ClaimToken: token}); err != nil {
		t.Fatal(err)
	}
	want, err := d.GetTrackAnalysis("song")
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []*string{nil, &oldFP} {
		if token, prior, claimed, err := d.ClaimTrackAnalysisLeaseIfUnchanged("song", oldFP, 1, "v1", expected); err != nil || claimed || token != "" || prior != nil {
			t.Fatal("stale revision claimed", token, prior, claimed, err)
		}
		got, err := d.GetTrackAnalysis("song")
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatal("stale revision mutated projection", got, want, err)
		}
	}
}
