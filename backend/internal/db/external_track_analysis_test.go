package db

import (
	spotifyanalysis "github.com/ajbergh/viib-mediahub/internal/spotify/analysis"
	"math"
	"path/filepath"
	"testing"
	"time"
)

const referenceID = "5r9W9MJLvHk83fcZSPQ8SE"

func referenceObservation() spotifyanalysis.Observation {
	bpm := 108.022
	zero := 0.0
	return spotifyanalysis.Observation{TrackID: referenceID, Source: "spotify_internal", SourceEndpoint: "audio_features",
		RetrievedAt: time.Unix(1700000000, 0).UTC(), BPM: &bpm, BPMConfidence: &zero}
}
func TestExternalAnalysisPersistenceAndIsolation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.db")
	d, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if d != nil {
			d.Close()
		}
	}()
	if err = d.EnsureExternalTrackAnalysisSchema(); err != nil {
		t.Fatal(err)
	}
	if err = d.EnsureExternalTrackAnalysisSchema(); err != nil {
		t.Fatal(err)
	}
	if err = d.SaveSong(&Song{ID: "song", Title: "S", Artist: "A", Album: "B", FilePath: "song.wav", AddedAt: 1, BPM: 128}); err != nil {
		t.Fatal(err)
	}
	if _, err = d.conn.Exec("UPDATE songs SET bpm=128 WHERE id='song'"); err != nil {
		t.Fatal(err)
	}
	if err = d.EnsureTrackAnalysisSchema(); err != nil {
		t.Fatal(err)
	}
	_, err = d.conn.Exec(`INSERT INTO track_analysis(song_id,status,analysis_version,algorithm_version,source_fingerprint,bpm,bpm_source)
 VALUES ('song','complete',1,'local-v1','source',127.5,'measured');
 INSERT INTO track_analysis_overrides(song_id,bpm,bpm_locked,updated_at) VALUES ('song',126.5,1,1)`)
	if err != nil {
		t.Fatal(err)
	}
	o := referenceObservation()
	if err = d.PutExternalAnalysis(o, "fixture-v1", o.RetrievedAt.Add(7*24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	failure := ExternalAnalysisStatus{Code: spotifyanalysis.NotFound, CheckedAt: o.RetrievedAt.Add(time.Hour), RetryAt: o.RetrievedAt.Add(2 * time.Hour)}
	if err = d.PutExternalAnalysisStatus(referenceID, "audio_features", failure); err != nil {
		t.Fatal(err)
	}
	older := o
	older.RetrievedAt = o.RetrievedAt.Add(-time.Hour)
	bpm := 90.0
	older.BPM = &bpm
	if err = d.PutExternalAnalysis(older, "older", o.RetrievedAt.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	d.Close()
	d = nil
	d, err = New(path)
	if err != nil {
		t.Fatal(err)
	}
	cache, err := d.GetExternalAnalysis(referenceID, "audio_features")
	if err != nil || cache == nil || *cache.Observation.BPM != 108.022 || cache.Observation.Key != nil ||
		cache.Observation.BPMConfidence == nil || *cache.Observation.BPMConfidence != 0 || cache.AdapterRevision != "fixture-v1" {
		t.Fatalf("restart cache: %+v %v", cache, err)
	}
	status, err := d.GetExternalAnalysisStatus(referenceID, "audio_features")
	if err != nil || status == nil || status.Code != spotifyanalysis.NotFound {
		t.Fatalf("failure status: %+v %v", status, err)
	}
	detailed, err := d.GetExternalAnalysis(referenceID, "audio_analysis")
	if err != nil || detailed != nil {
		t.Fatalf("endpoint isolation: %+v %v", detailed, err)
	}
	var legacy int
	var local, manual float64
	if err = d.conn.QueryRow(`SELECT s.bpm,a.bpm,o.bpm FROM songs s JOIN track_analysis a ON a.song_id=s.id JOIN track_analysis_overrides o ON o.song_id=s.id`).Scan(&legacy, &local, &manual); err != nil {
		t.Fatal(err)
	}
	if legacy != 128 || local != 127.5 || manual != 126.5 {
		t.Fatal("reference storage changed local values")
	}
	if err = d.PurgeExternalAnalysis(); err != nil {
		t.Fatal(err)
	}
	cache, err = d.GetExternalAnalysis(referenceID, "audio_features")
	if err != nil || cache != nil {
		t.Fatal("purge retained cache")
	}
	status, err = d.GetExternalAnalysisStatus(referenceID, "audio_features")
	if err != nil || status != nil {
		t.Fatal("purge retained failures")
	}
	var count int
	d.conn.QueryRow("SELECT count(*) FROM track_analysis_overrides").Scan(&count)
	if count != 1 {
		t.Fatal("purge changed manual values")
	}
}
func TestSpotifyRecordingConfirmationIsSourceBound(t *testing.T) {
	d, err := New(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	for _, id := range []string{"a", "b"} {
		if err = d.SaveSong(&Song{ID: id, Title: "S", Artist: "A", Album: "B", FilePath: id + ".wav", AddedAt: 1}); err != nil {
			t.Fatal(err)
		}
		if err = d.RefreshTrackAnalysisSourceRevision(id, "source-v1"); err != nil {
			t.Fatal(err)
		}
		if ok, err := d.ConfirmSpotifyRecording(id, referenceID, "source-v1", false); err == nil || ok {
			t.Fatal("unconfirmed link accepted")
		}
		if ok, err := d.ConfirmSpotifyRecording(id, referenceID, "source-old", true); err != nil || ok {
			t.Fatal("stale source accepted")
		}
		if ok, err := d.ConfirmSpotifyRecording(id, referenceID, "source-v1", true); err != nil || !ok {
			t.Fatalf("confirmation %v %v", ok, err)
		}
	}
	if link, err := d.GetSpotifyRecording("a", "source-v2"); err != nil || link != nil {
		t.Fatal("changed source retained active link")
	}
	if link, err := d.GetSpotifyRecording("a", ""); err != nil || link != nil {
		t.Fatal("unavailable source retained active link")
	}
	if _, err = d.conn.Exec("DELETE FROM songs WHERE id='a'"); err != nil {
		t.Fatal(err)
	}
	var count int
	if err = d.conn.QueryRow("SELECT count(*) FROM track_external_identity").Scan(&count); err != nil || count != 1 {
		t.Fatalf("cascade count=%d err=%v", count, err)
	}
	if link, err := d.GetSpotifyRecording("b", "source-v1"); err != nil || link == nil {
		t.Fatal("another copy's identity removed")
	}
}
func TestExternalAnalysisRejectsMalformedValuesAndOldFailure(t *testing.T) {
	d, err := New(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	o := referenceObservation()
	invalid := o
	nan := math.NaN()
	invalid.BPM = &nan
	if err = d.PutExternalAnalysis(invalid, "fixture", o.RetrievedAt.Add(time.Hour)); err == nil {
		t.Fatal("NaN accepted")
	}
	invalid = o
	negative := -1.0
	invalid.BPMConfidence = &negative
	if err = d.PutExternalAnalysis(invalid, "fixture", o.RetrievedAt.Add(time.Hour)); err == nil {
		t.Fatal("invalid confidence accepted")
	}
	invalid = o
	invalid.SourceEndpoint = "other"
	if err = d.PutExternalAnalysis(invalid, "fixture", o.RetrievedAt.Add(time.Hour)); err == nil {
		t.Fatal("unknown endpoint accepted")
	}
	newer := ExternalAnalysisStatus{Code: spotifyanalysis.RateLimited, CheckedAt: o.RetrievedAt.Add(time.Hour), RetryAt: o.RetrievedAt.Add(2 * time.Hour)}
	if err = d.PutExternalAnalysisStatus(referenceID, "audio_features", newer); err != nil {
		t.Fatal(err)
	}
	old := newer
	old.CheckedAt = o.RetrievedAt
	old.Code = spotifyanalysis.NotFound
	if err = d.PutExternalAnalysisStatus(referenceID, "audio_features", old); err != nil {
		t.Fatal(err)
	}
	status, err := d.GetExternalAnalysisStatus(referenceID, "audio_features")
	if err != nil || status.Code != newer.Code {
		t.Fatal("old failure replaced new cooldown")
	}
	old.Code = "raw secret error"
	if err = d.PutExternalAnalysisStatus(referenceID, "audio_features", old); err == nil {
		t.Fatal("arbitrary failure text accepted")
	}
}
