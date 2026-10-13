package db

import (
	"errors"
	"testing"
	"time"
)

func TestTrackAnalysisLeaseFencesSupersededWorkerAndAtomicPublication(t *testing.T) {
	d := selectionDatabase(t)
	saveSelectionSong(t, d, "song", 1)
	first, ok, err := d.ClaimTrackAnalysisLease("song", "fp", 1, "test-v1")
	if err != nil || !ok || first == "" {
		t.Fatal(first, ok, err)
	}
	if _, ok, err := d.ClaimTrackAnalysisLease("song", "fp", 1, "test-v1"); err != nil || ok {
		t.Fatal("live claim stolen", ok, err)
	}
	if _, err := d.conn.Exec(`UPDATE track_analysis SET analyzed_at=? WHERE song_id='song'`, time.Now().UnixMilli()-TrackAnalysisLeaseMillis-1); err != nil {
		t.Fatal(err)
	}
	if err := d.RenewTrackAnalysisLease("song", "fp", first); !errors.Is(err, ErrTrackAnalysisLeaseLost) {
		t.Fatal("expired lease revived", err)
	}
	second, ok, err := d.ClaimTrackAnalysisLease("song", "fp", 1, "test-v1")
	if err != nil || !ok || second == first {
		t.Fatal(second, ok, err)
	}
	artifact := TrackAnalysisArtifact{ID: "song:lease", SongID: "song", Kind: "test", FormatVersion: 1, AlgorithmVersion: "test-v1", Encoding: "opaque", SourceFingerprint: "fp", Data: []byte("new")}
	state := TrackCapabilityStatus{SongID: "song", SourceFingerprint: "fp", Capability: "test", Version: "v1", State: "available"}
	confidence := .8
	p := TrackPreparationPublication{ClaimToken: first, Analysis: TrackAnalysis{SongID: "song", SourceFingerprint: "fp", Status: TrackAnalysisComplete, AnalysisVersion: 1, AlgorithmVersion: "test-v1"}, Artifacts: []TrackAnalysisArtifact{artifact}, ApplyCues: true, CueMode: GeneratedCueFillEmpty, Cues: []DJHotCue{{Slot: 1, Position: 3, Confidence: &confidence, Origin: "analysis", GeneratorVersion: "v1", Kind: "intro", SourceFingerprint: "fp"}}, Capabilities: []TrackCapabilityStatus{state}}
	if err := d.PublishTrackPreparation(p); !errors.Is(err, ErrTrackAnalysisLeaseLost) {
		t.Fatal("stale worker published", err)
	}
	if err := d.ReleaseTrackAnalysisLease("song", first); !errors.Is(err, ErrTrackAnalysisLeaseLost) {
		t.Fatal("stale worker released new claim", err)
	}
	if err := d.RenewTrackAnalysisLease("song", "fp", first); !errors.Is(err, ErrTrackAnalysisLeaseLost) {
		t.Fatal("stale worker renewed new claim", err)
	}
	if err := d.ReleaseTrackAnalysis("song"); err != nil {
		t.Fatal(err)
	}
	if err := d.UpsertTrackAnalysis(p.Analysis); !errors.Is(err, ErrTrackAnalysisLeaseLost) {
		t.Fatal("legacy upsert replaced managed claim", err)
	}
	p.ClaimToken = ""
	if err := d.PublishTrackPreparation(p); !errors.Is(err, ErrTrackAnalysisLeaseLost) {
		t.Fatal("unowned publisher replaced managed claim", err)
	}
	var token, status string
	if err := d.conn.QueryRow(`SELECT claim_token,status FROM track_analysis WHERE song_id='song'`).Scan(&token, &status); err != nil || token != second || status != TrackAnalysisRunning {
		t.Fatal(token, status, err)
	}
	for _, table := range []string{"track_analysis_artifacts", "dj_hot_cues", "track_metadata_capability_status"} {
		var count int
		if err := d.conn.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count); err != nil || count != 0 {
			t.Fatal("stale domain mutation", table, count, err)
		}
	}
	p.ClaimToken = second
	if _, err := d.conn.Exec(`CREATE TRIGGER fail_lease_completion BEFORE INSERT ON track_metadata_capability_status BEGIN SELECT RAISE(ABORT,'completion failure'); END`); err != nil {
		t.Fatal(err)
	}
	if err := d.PublishTrackPreparation(p); err == nil {
		t.Fatal("late failure ignored")
	}
	if err := d.RenewTrackAnalysisLease("song", "fp", second); err != nil {
		t.Fatal("rollback consumed live token", err)
	}
	if _, err := d.conn.Exec(`DROP TRIGGER fail_lease_completion`); err != nil {
		t.Fatal(err)
	}
	if err := d.PublishTrackPreparation(p); err != nil {
		t.Fatal(err)
	}
	var remaining int
	if err := d.conn.QueryRow(`SELECT COUNT(*) FROM track_analysis WHERE song_id='song' AND status='complete' AND claim_token IS NULL`).Scan(&remaining); err != nil || remaining != 1 {
		t.Fatal("publication did not settle lease", remaining, err)
	}
	if got, err := d.GetTrackAnalysisArtifact("song", "test", 1, "test-v1"); err != nil || string(got.Data) != "new" {
		t.Fatal(got, err)
	}
	if got, err := d.GetDJHotCues("song"); err != nil || len(got) != 1 {
		t.Fatal(got, err)
	}
	if got, err := d.GetTrackCapabilityStatuses("song", "fp"); err != nil || got["test"].State != "available" {
		t.Fatal(got, err)
	}
	if err := d.ReleaseTrackAnalysisLease("song", second); !errors.Is(err, ErrTrackAnalysisLeaseLost) {
		t.Fatal("completed claim released", err)
	}
}

func TestTrackAnalysisLeaseRenewalAndSourceFence(t *testing.T) {
	d := selectionDatabase(t)
	saveSelectionSong(t, d, "song", 1)
	token, ok, err := d.ClaimTrackAnalysisLease("song", "fp", 1, "v1")
	if err != nil || !ok {
		t.Fatal(ok, err)
	}
	old := time.Now().UnixMilli() - TrackAnalysisLeaseMillis/2
	if _, err := d.conn.Exec(`UPDATE track_analysis SET analyzed_at=? WHERE song_id='song'`, old); err != nil {
		t.Fatal(err)
	}
	if err := d.RenewTrackAnalysisLease("song", "other-source", token); !errors.Is(err, ErrTrackAnalysisLeaseLost) {
		t.Fatal("wrong source renewed", err)
	}
	if err := d.RenewTrackAnalysisLease("song", "fp", token); err != nil {
		t.Fatal(err)
	}
	got, err := d.GetTrackAnalysis("song")
	if err != nil || got.AnalyzedAt == nil || *got.AnalyzedAt <= old {
		t.Fatal(got, err)
	}
	if err := d.ReleaseTrackAnalysisLease("song", token); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := d.ClaimTrackAnalysisLease("song", "new", 1, "v2"); err != nil || !ok {
		t.Fatal("release blocked immediate reclaim", ok, err)
	}
}

func TestSourceRevisionRevokesManagedPreparationLease(t *testing.T) {
	d := selectionDatabase(t)
	saveSelectionSong(t, d, "song", 1)
	token, claimed, err := d.ClaimTrackAnalysisLease("song", "old", 1, "test-v1")
	if err != nil || !claimed {
		t.Fatal(claimed, err)
	}
	if err := d.RefreshTrackAnalysisSourceRevision("song", "new"); err != nil {
		t.Fatal(err)
	}
	if err := d.RenewTrackAnalysisLease("song", "old", token); !errors.Is(err, ErrTrackAnalysisLeaseLost) {
		t.Fatal("old generation renewed", err)
	}
	if err := d.PublishTrackPreparation(TrackPreparationPublication{ClaimToken: token, Analysis: TrackAnalysis{SongID: "song", SourceFingerprint: "old", Status: TrackAnalysisComplete}}); !errors.Is(err, ErrTrackAnalysisLeaseLost) {
		t.Fatal("old generation published", err)
	}
	next, claimed, err := d.ClaimTrackAnalysisLease("song", "new", 1, "test-v1")
	if err != nil || !claimed || next == token {
		t.Fatal(next, claimed, err)
	}
	if err := d.RefreshTrackAnalysisSourceRevision("song", "new"); err != nil {
		t.Fatal(err)
	}
	if err := d.RenewTrackAnalysisLease("song", "new", next); err != nil {
		t.Fatal("same revision revoked claim", err)
	}
}
