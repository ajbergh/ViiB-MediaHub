package db

import (
	"database/sql"
	"errors"
	"reflect"
	"testing"
	"time"
)

func TestWaveformLeaseContendsWithoutScalarRow(t *testing.T) {
	d := selectionDatabase(t)
	saveSelectionSong(t, d, "song", 1)
	token, ok, err := d.ClaimWaveformLease("song", "fp")
	if err != nil || !ok {
		t.Fatal(ok, err)
	}
	if _, err := d.GetTrackAnalysis("song"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal("fabricated scalar row", err)
	}
	if _, ok, err := d.ClaimWaveformLease("song", "fp"); err != nil || ok {
		t.Fatal("second waveform owner", ok, err)
	}
	if _, ok, err := d.ClaimTrackAnalysisLease("song", "fp", 1, "v1"); ok || !errors.Is(err, ErrWaveformLeaseBusy) {
		t.Fatal("preparation bypassed waveform", ok, err)
	}
	a := TrackAnalysisArtifact{ID: "wave", SongID: "song", Kind: "local_amplitude", FormatVersion: 1, AlgorithmVersion: "v1", Encoding: "opaque", Provenance: "measured", SourceFingerprint: "fp", Data: []byte("wave")}
	if err := d.PublishWaveformLease("stale", a); !errors.Is(err, ErrTrackAnalysisLeaseLost) {
		t.Fatal(err)
	}
	if err := d.PublishWaveformLease(token, a); err != nil {
		t.Fatal(err)
	}
	if _, err := d.GetTrackAnalysis("song"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal("waveform published scalar row", err)
	}
	prep, ok, err := d.ClaimTrackAnalysisLease("song", "fp", 1, "v1")
	if err != nil || !ok {
		t.Fatal(ok, err)
	}
	if _, ok, err := d.ClaimWaveformLease("song", "fp"); err != nil || ok {
		t.Fatal("waveform bypassed preparation", ok, err)
	}
	if err := d.ReleaseTrackAnalysisLease("song", prep); err != nil {
		t.Fatal(err)
	}
}
func TestWaveformLeaseExpiryAndSourceRevocation(t *testing.T) {
	d := selectionDatabase(t)
	saveSelectionSong(t, d, "song", 1)
	first, _, err := d.ClaimWaveformLease("song", "old")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.conn.Exec(`UPDATE track_waveform_leases SET renewed_at=?`, time.Now().UnixMilli()-TrackAnalysisLeaseMillis-1); err != nil {
		t.Fatal(err)
	}
	if err := d.RenewWaveformLease("song", "old", first); !errors.Is(err, ErrTrackAnalysisLeaseLost) {
		t.Fatal(err)
	}
	second, ok, err := d.ClaimWaveformLease("song", "old")
	if err != nil || !ok || first == second {
		t.Fatal(ok, err)
	}
	if err := d.ReleaseWaveformLease("song", first); !errors.Is(err, ErrTrackAnalysisLeaseLost) {
		t.Fatal(err)
	}
	if err := d.RefreshTrackAnalysisSourceRevision("song", "new"); err != nil {
		t.Fatal(err)
	}
	if err := d.RenewWaveformLease("song", "old", second); !errors.Is(err, ErrTrackAnalysisLeaseLost) {
		t.Fatal(err)
	}
	if _, ok, err := d.ClaimTrackAnalysisLease("song", "new", 1, "v1"); err != nil || !ok {
		t.Fatal(ok, err)
	}
}

func TestWaveformPublicationRetainsScalarAndRollsBackLease(t *testing.T) {
	d := selectionDatabase(t)
	saveSelectionSong(t, d, "song", 1)
	before := TrackAnalysis{SongID: "song", SourceFingerprint: "fp", Status: TrackAnalysisComplete, AnalysisVersion: 1, AlgorithmVersion: "v1"}
	if err := d.UpsertTrackAnalysis(before); err != nil {
		t.Fatal(err)
	}
	stored, err := d.GetTrackAnalysis("song")
	if err != nil {
		t.Fatal(err)
	}
	token, ok, err := d.ClaimWaveformLease("song", "fp")
	if err != nil || !ok {
		t.Fatal(ok, err)
	}
	a := TrackAnalysisArtifact{ID: "wave", SongID: "song", Kind: "local_amplitude", FormatVersion: 1, AlgorithmVersion: "v1", Encoding: "opaque", Provenance: "measured", SourceFingerprint: "fp", Data: []byte("wave")}
	if _, err := d.conn.Exec(`CREATE TRIGGER fail_wave BEFORE INSERT ON track_analysis_artifacts BEGIN SELECT RAISE(ABORT,'fail'); END`); err != nil {
		t.Fatal(err)
	}
	if err := d.PublishWaveformLease(token, a); err == nil {
		t.Fatal("artifact failure ignored")
	}
	if err := d.RenewWaveformLease("song", "fp", token); err != nil {
		t.Fatal("rollback consumed lease", err)
	}
	if _, err := d.conn.Exec(`DROP TRIGGER fail_wave`); err != nil {
		t.Fatal(err)
	}
	if err := d.PublishWaveformLease(token, a); err != nil {
		t.Fatal(err)
	}
	after, err := d.GetTrackAnalysis("song")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(stored, after) {
		t.Fatal("lazy publication changed scalar record")
	}
}
