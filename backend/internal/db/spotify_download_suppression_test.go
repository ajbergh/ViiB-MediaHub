package db

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDownloadSuppressionSurvivesCanonicalRescan(t *testing.T) {
	d, path := evidenceFixture(t)
	finishEvidence(t, d, path, "job", referenceID)
	fp := scanEvidence(t, d, path, "old")
	if err := d.DeleteSpotifyRecording("old"); err != nil {
		t.Fatal(err)
	}
	if _, err := d.DeleteCompletedDownloads(); err != nil {
		t.Fatal(err)
	}
	if err := d.PurgeSpotifyMetadata(); err != nil {
		t.Fatal(err)
	}
	if err := d.DeleteSong("old"); err != nil {
		t.Fatal(err)
	}
	d.Close()
	reopened, err := New(filepath.Join(filepath.Dir(path), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	fp = scanEvidence(t, reopened, path, "new")
	if link, err := reopened.GetSpotifyRecording("new", fp); err != nil || link != nil {
		t.Fatal("removal reversed", link, err)
	}
	if allowed, err := reopened.SpotifySearchAllowed("new", fp); err != nil || allowed {
		t.Fatal("search bypassed removal", allowed, err)
	}
	if err := reopened.RefreshTrackAnalysisSourceRevision("new", fp); err != nil {
		t.Fatal(err)
	}
	if changed, err := reopened.SaveSpotifySearchMatch("new", referenceID, fp); err != nil || changed {
		t.Fatal("automatic search bypassed removal", changed, err)
	}
	if changed, err := reopened.ConfirmSpotifyRecording("new", referenceID, "stale", true); err != nil || changed {
		t.Fatal(changed, err)
	}
	var count int
	if err := reopened.conn.QueryRow("SELECT COUNT(*) FROM spotify_download_revision_suppression").Scan(&count); err != nil || count != 1 {
		t.Fatal("stale confirmation cleared tombstone", count, err)
	}
	if changed, err := reopened.ConfirmSpotifyRecording("new", referenceID, fp, true); err != nil || !changed {
		t.Fatal(changed, err)
	}
	if err := reopened.conn.QueryRow("SELECT COUNT(*) FROM spotify_download_revision_suppression").Scan(&count); err != nil || count != 0 {
		t.Fatal(count, err)
	}
	if err := reopened.DeleteSong("new"); err != nil {
		t.Fatal(err)
	}
	fp = scanEvidence(t, reopened, path, "third")
	if link, err := reopened.GetSpotifyRecording("third", fp); err != nil || link == nil {
		t.Fatal("deliberate relink not retained", link, err)
	}
}

func TestDownloadSuppressionExactRevisionAndMigration(t *testing.T) {
	d, path := evidenceFixture(t)
	finishEvidence(t, d, path, "job", referenceID)
	fp := scanEvidence(t, d, path, "old")
	// Simulate an existing release's song-scoped removal before migration.
	if _, err := d.conn.Exec(`INSERT INTO track_external_identity_suppression VALUES('old',?); DELETE FROM track_external_identity WHERE song_id='old'`, fp); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := d.EnsureSpotifyMetadataSchema(); err != nil {
			t.Fatal(err)
		}
	}
	if err := d.DeleteSong("old"); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	changed := append([]byte{}, original...)
	changed[0] ^= 1
	if err := os.WriteFile(path, changed, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
	finishEvidence(t, d, path, "changed", referenceID)
	fp = scanEvidence(t, d, path, "changed-song")
	if link, err := d.GetSpotifyRecording("changed-song", fp); err != nil || link == nil {
		t.Fatal("new bytes incorrectly suppressed", link, err)
	}
	if err := d.DeleteSong("changed-song"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
	fp = scanEvidence(t, d, path, "restored")
	if link, err := d.GetSpotifyRecording("restored", fp); err != nil || link != nil {
		t.Fatal("old revision removal lost", link, err)
	}
}

func TestDownloadSuppressionRollback(t *testing.T) {
	d, path := evidenceFixture(t)
	finishEvidence(t, d, path, "job", referenceID)
	fp := scanEvidence(t, d, path, "song")
	if _, err := d.conn.Exec(`CREATE TRIGGER fail_suppression BEFORE INSERT ON spotify_download_revision_suppression BEGIN SELECT RAISE(ABORT,'fixture'); END`); err != nil {
		t.Fatal(err)
	}
	if err := d.DeleteSpotifyRecording("song"); err == nil {
		t.Fatal("expected failure")
	}
	if link, err := d.GetSpotifyRecording("song", fp); err != nil || link == nil {
		t.Fatal("partial removal", link, err)
	}
	var count int
	if err := d.conn.QueryRow("SELECT COUNT(*) FROM track_external_identity_suppression").Scan(&count); err != nil || count != 0 {
		t.Fatal(count, err)
	}
}

func TestDownloadSuppressionAmbiguousRescan(t *testing.T) {
	d, path := evidenceFixture(t)
	finishEvidence(t, d, path, "one", referenceID)
	scanEvidence(t, d, path, "old")
	if err := d.DeleteSpotifyRecording("old"); err != nil {
		t.Fatal(err)
	}
	finishEvidence(t, d, path, "two", "11dFghVXANMlKmJXsNCbNl")
	if err := d.DeleteSong("old"); err != nil {
		t.Fatal(err)
	}
	fp := scanEvidence(t, d, path, "new")
	if allowed, err := d.SpotifySearchAllowed("new", fp); err != nil || allowed {
		t.Fatal("ambiguous evidence lost removal", allowed, err)
	}
	if err := d.RefreshTrackAnalysisSourceRevision("new", fp); err != nil {
		t.Fatal(err)
	}
	if changed, err := d.SaveSpotifySearchMatch("new", referenceID, fp); err != nil || changed {
		t.Fatal(changed, err)
	}
	if changed, err := d.ConfirmSpotifyRecording("new", "11dFghVXANMlKmJXsNCbNl", fp, true); err != nil || !changed {
		t.Fatal(changed, err)
	}
	var recording string
	if err := d.conn.QueryRow("SELECT spotify_id FROM spotify_download_import_bindings WHERE song_id='new'").Scan(&recording); err != nil || recording != "11dFghVXANMlKmJXsNCbNl" {
		t.Fatal(recording, err)
	}
}

func TestDownloadIdentityBindingPublicationRollback(t *testing.T) {
	d, path := evidenceFixture(t)
	finishEvidence(t, d, path, "job", referenceID)
	if _, err := d.conn.Exec(`CREATE TRIGGER fail_binding BEFORE INSERT ON spotify_download_import_bindings BEGIN SELECT RAISE(ABORT,'fixture'); END`); err != nil {
		t.Fatal(err)
	}
	fp := scanEvidence(t, d, path, "song")
	if link, err := d.GetSpotifyRecording("song", fp); err != nil || link != nil {
		t.Fatal("partial identity published", link, err)
	}
	if _, err := d.conn.Exec("DROP TRIGGER fail_binding"); err != nil {
		t.Fatal(err)
	}
	if err := d.ReconcileSpotifyDownload(t.Context(), path); err != nil {
		t.Fatal(err)
	}
	if link, err := d.GetSpotifyRecording("song", fp); err != nil || link == nil {
		t.Fatal("retry failed", link, err)
	}
}

func TestDownloadSuppressionCoarseFingerprintCollision(t *testing.T) {
	for _, mode := range []string{"mtime", "path"} {
		t.Run(mode, func(t *testing.T) {
			d, path := evidenceFixture(t)
			finishEvidence(t, d, path, "job", referenceID)
			fp := scanEvidence(t, d, path, "song")
			if err := d.DeleteSpotifyRecording("song"); err != nil {
				t.Fatal(err)
			}
			info, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			if mode == "mtime" {
				stamp := info.ModTime().Add(100 * time.Nanosecond)
				if stamp.UnixMilli() != info.ModTime().UnixMilli() {
					stamp = info.ModTime().Add(-100 * time.Nanosecond)
				}
				if err := os.Chtimes(path, stamp, stamp); err != nil {
					t.Fatal(err)
				}
			} else {
				next := filepath.Join(filepath.Dir(path), "moved.mp3")
				if err := os.Rename(path, next); err != nil {
					t.Fatal(err)
				}
				path = next
				if _, err := d.conn.Exec("UPDATE songs SET file_path=? WHERE id='song'", path); err != nil {
					t.Fatal(err)
				}
			}
			finishEvidence(t, d, path, "new", referenceID)
			next := scanEvidence(t, d, path, "song")
			if next != fp {
				t.Fatal("fixture must collide", next, fp)
			}
			if link, err := d.GetSpotifyRecording("song", fp); err != nil || link == nil {
				t.Fatal("distinct exact revision suppressed", link, err)
			}
		})
	}
}

func TestDownloadSuppressionConfirmationRejectsChangedBytes(t *testing.T) {
	d, path := evidenceFixture(t)
	finishEvidence(t, d, path, "job", referenceID)
	fp := scanEvidence(t, d, path, "song")
	if err := d.DeleteSpotifyRecording("song"); err != nil {
		t.Fatal(err)
	}
	if err := d.RefreshTrackAnalysisSourceRevision("song", fp); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	raw[0] ^= 1
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
	if changed, err := d.ConfirmSpotifyRecording("song", referenceID, fp, true); err != nil || changed {
		t.Fatal("changed bytes admitted", changed, err)
	}
	var n int
	if err := d.conn.QueryRow("SELECT COUNT(*) FROM spotify_download_revision_suppression").Scan(&n); err != nil || n != 1 {
		t.Fatal(n, err)
	}
}

func TestDownloadSuppressionConcurrentReconcileUnlink(t *testing.T) {
	d, path := evidenceFixture(t)
	finishEvidence(t, d, path, "job", referenceID)
	fp := scanEvidence(t, d, path, "song")
	if err := d.RefreshTrackAnalysisSourceRevision("song", fp); err != nil {
		t.Fatal(err)
	}
	for range 20 {
		if changed, err := d.ConfirmSpotifyRecording("song", referenceID, fp, true); err != nil || !changed {
			t.Fatal(changed, err)
		}
		results := make(chan error, 2)
		go func() { results <- d.ReconcileSpotifyDownload(t.Context(), path) }()
		go func() { results <- d.DeleteSpotifyRecording("song") }()
		for range 2 {
			if err := <-results; err != nil {
				t.Fatal(err)
			}
		}
		if err := d.ReconcileSpotifyDownload(t.Context(), path); err != nil {
			t.Fatal(err)
		}
		if link, err := d.GetSpotifyRecording("song", fp); err != nil || link != nil {
			t.Fatal("unlink reversed", link, err)
		}
		var count int
		if err := d.conn.QueryRow("SELECT COUNT(*) FROM spotify_download_revision_suppression").Scan(&count); err != nil || count != 1 {
			t.Fatal(count, err)
		}
	}
}

func TestDownloadSuppressionUpgradeFromAbsentTables(t *testing.T) {
	d, path := evidenceFixture(t)
	finishEvidence(t, d, path, "job", referenceID)
	fp := scanEvidence(t, d, path, "song")
	if _, err := d.conn.Exec(`DROP TABLE spotify_download_suppression_bindings; DROP TABLE spotify_download_revision_suppression; INSERT INTO track_external_identity_suppression VALUES('song',?); DELETE FROM track_external_identity WHERE song_id='song'`, fp); err != nil {
		t.Fatal(err)
	}
	d.Close()
	reopened, err := New(filepath.Join(filepath.Dir(path), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if err := reopened.DeleteSong("song"); err != nil {
		t.Fatal(err)
	}
	fp = scanEvidence(t, reopened, path, "new")
	if allowed, err := reopened.SpotifySearchAllowed("new", fp); err != nil || allowed {
		t.Fatal(allowed, err)
	}
}

func TestDownloadPublicationPreservesDifferentManualRecording(t *testing.T) {
	d, path := evidenceFixture(t)
	finishEvidence(t, d, path, "job", referenceID)
	fp := scanEvidence(t, d, path, "song")
	if err := d.RefreshTrackAnalysisSourceRevision("song", fp); err != nil {
		t.Fatal(err)
	}
	if changed, err := d.ConfirmSpotifyRecording("song", "11dFghVXANMlKmJXsNCbNl", fp, true); err != nil || !changed {
		t.Fatal(changed, err)
	}
	raw, err := json.Marshal(referenceObservation())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.conn.Exec("UPDATE spotify_download_evidence SET features_json=?", string(raw)); err != nil {
		t.Fatal(err)
	}
	if err := d.ReconcileSpotifyDownload(t.Context(), path); err != nil {
		t.Fatal(err)
	}
	if link, err := d.GetSpotifyRecording("song", fp); err != nil || link == nil || link.ExternalID != "11dFghVXANMlKmJXsNCbNl" {
		t.Fatal(link, err)
	}
	var count int
	if err := d.conn.QueryRow("SELECT COUNT(*) FROM track_analysis WHERE song_id='song'").Scan(&count); err != nil || count != 0 {
		t.Fatal("wrong recording scalars published", count, err)
	}
}
