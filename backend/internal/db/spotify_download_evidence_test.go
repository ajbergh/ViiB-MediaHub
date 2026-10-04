// Tests recording and reconciliation of Spotify download evidence against local file identity.
package db

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func evidenceFixture(t *testing.T) (*DB, string) {
	t.Helper()
	dir := t.TempDir()
	d, err := New(filepath.Join(dir, "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	path := filepath.Join(dir, "final.mp3")
	if err = os.WriteFile(path, []byte("final tagged converted bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	return d, path
}
func finishEvidence(t *testing.T, d *DB, path, id, recording string) {
	t.Helper()
	if err := d.AddDownload(&SpotifyDownload{ID: id, SpotifyID: recording, Type: "track", Title: "Fixture", Status: "queued", AddedAt: 1}); err != nil {
		t.Fatal(err)
	}
	if changed, err := d.MarkDownloadStarted(id); err != nil || !changed {
		t.Fatalf("start: %v %v", changed, err)
	}
	if changed, err := d.MarkDownloadCompletedWithEvidence(context.Background(), id, path); err != nil || !changed {
		t.Fatalf("complete: %v %v", changed, err)
	}
}
func scanEvidence(t *testing.T, d *DB, path, id string) string {
	t.Helper()
	_, err := d.SaveSongsWithResult([]Song{{ID: id, FilePath: path, Title: "Fixture", Artist: "Artist", Album: "Album", FileHash: "stable-hash", AddedAt: 1}})
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return LocalSourceFingerprint(Song{FilePath: path, FileHash: "stable-hash"}, info)
}
func TestDownloadEvidenceSurvivesQueueClearRestartAndCanonicalUpsert(t *testing.T) {
	d, path := evidenceFixture(t)
	if err := d.SaveSong(&Song{ID: "canonical", FilePath: path, Title: "Fixture", Artist: "Artist", Album: "Album", AddedAt: 1}); err != nil {
		t.Fatal(err)
	}
	finishEvidence(t, d, path, "job", referenceID)
	if _, err := d.DeleteCompletedDownloads(); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(filepath.Dir(path), "library.db")
	d.Close()
	reopened, err := New(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	fingerprint := scanEvidence(t, reopened, path, "proposed")
	link, err := reopened.GetSpotifyRecording("canonical", fingerprint)
	if err != nil || link == nil || link.ExternalID != referenceID {
		t.Fatalf("source revision reconciliation: %#v %v", link, err)
	}
	var origin, id string
	if err = reopened.conn.QueryRow("SELECT link_origin,external_id FROM track_external_identity WHERE song_id='canonical'").Scan(&origin, &id); err != nil || origin != "download_completion" || id != referenceID {
		t.Fatalf("durable evidence/link: %s %s %v", origin, id, err)
	}
	// A song absent at completion is linked after queue clearing, using its canonical ID.
	_, err = reopened.conn.Exec("DELETE FROM track_external_identity; DELETE FROM songs")
	if err != nil {
		t.Fatal(err)
	}
	fingerprint = scanEvidence(t, reopened, path, "canonical")
	link, err = reopened.GetSpotifyRecording("canonical", fingerprint)
	if err != nil || link == nil || link.ExternalID != referenceID {
		t.Fatalf("link after restart/queue clear: %#v %v", link, err)
	}
	scanEvidence(t, reopened, path, "different-proposed-id")
	link, err = reopened.GetSpotifyRecording("canonical", fingerprint)
	if err != nil || link == nil {
		t.Fatalf("canonical upsert link lost: %#v %v", link, err)
	}
}
func TestDownloadEvidenceRejectsSamePathReplacementAndAmbiguity(t *testing.T) {
	for _, mode := range []string{"replacement", "ambiguity", "historical"} {
		t.Run(mode, func(t *testing.T) {
			d, path := evidenceFixture(t)
			if mode == "historical" {
				if err := d.AddDownload(&SpotifyDownload{ID: "old", SpotifyID: referenceID, Type: "track", Title: "Fixture", Status: "queued", AddedAt: 1}); err != nil {
					t.Fatal(err)
				}
				d.MarkDownloadStarted("old")
				if changed, err := d.MarkDownloadCompleted("old", path); err != nil || !changed {
					t.Fatal(err)
				}
			} else {
				finishEvidence(t, d, path, "one", referenceID)
			}
			if mode == "replacement" {
				info, _ := os.Stat(path)
				bytes, _ := os.ReadFile(path)
				bytes[0] ^= 1
				if err := os.WriteFile(path, bytes, 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.Chtimes(path, time.Now(), info.ModTime()); err != nil {
					t.Fatal(err)
				}
			}
			if mode == "ambiguity" {
				finishEvidence(t, d, path, "two", "11dFghVXANMlKmJXsNCbNl")
			}
			fingerprint := scanEvidence(t, d, path, "song")
			link, err := d.GetSpotifyRecording("song", fingerprint)
			if err != nil || link != nil {
				t.Fatalf("unsafe evidence linked: %#v %v", link, err)
			}
		})
	}
}
func TestDownloadEvidencePreservesManualLinksAndRemoval(t *testing.T) {
	d, path := evidenceFixture(t)
	fingerprint := scanEvidence(t, d, path, "song")
	if err := d.RefreshTrackAnalysisSourceRevision("song", fingerprint); err != nil {
		t.Fatal(err)
	}
	if changed, err := d.ConfirmSpotifyRecording("song", "11dFghVXANMlKmJXsNCbNl", fingerprint, true); err != nil || !changed {
		t.Fatalf("manual: %v %v", changed, err)
	}
	finishEvidence(t, d, path, "job", referenceID)
	scanEvidence(t, d, path, "song")
	link, err := d.GetSpotifyRecording("song", fingerprint)
	if err != nil || link == nil || link.ExternalID != "11dFghVXANMlKmJXsNCbNl" || link.LinkOrigin != "manual_confirmation" {
		t.Fatalf("manual overwritten: %#v %v", link, err)
	}
	if err = d.DeleteSpotifyRecording("song"); err != nil {
		t.Fatal(err)
	}
	scanEvidence(t, d, path, "song")
	link, err = d.GetSpotifyRecording("song", fingerprint)
	if err != nil || link != nil {
		t.Fatalf("explicit removal reversed: %#v %v", link, err)
	}
	if changed, err := d.ConfirmSpotifyRecording("song", referenceID, fingerprint, true); err != nil || !changed {
		t.Fatalf("reconfirm: %v %v", changed, err)
	}
}
func TestDownloadEvidenceCancelledCompletionHasNoSideEffects(t *testing.T) {
	d, path := evidenceFixture(t)
	if err := d.AddDownload(&SpotifyDownload{ID: "job", SpotifyID: referenceID, Type: "track", Title: "Fixture", Status: "queued", AddedAt: 1}); err != nil {
		t.Fatal(err)
	}
	d.MarkDownloadStarted("job")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if changed, err := d.MarkDownloadCompletedWithEvidence(ctx, "job", path); changed || err == nil {
		t.Fatalf("cancelled completion: %v %v", changed, err)
	}
	row, err := d.GetDownload("job")
	if err != nil || row.Status != "downloading" {
		t.Fatalf("state changed: %#v %v", row, err)
	}
	var count int
	if err = d.conn.QueryRow("SELECT COUNT(*) FROM spotify_download_evidence").Scan(&count); err != nil || count != 0 {
		t.Fatalf("evidence leaked: %d %v", count, err)
	}
}
func TestIdentitySchemaMigrationPreservesManualAndForeignKey(t *testing.T) {
	d, path := evidenceFixture(t)
	fingerprint := scanEvidence(t, d, path, "song")
	_, err := d.conn.Exec(`DROP TABLE track_external_identity; CREATE TABLE track_external_identity (
 song_id TEXT NOT NULL REFERENCES songs(id) ON DELETE CASCADE,
 provider TEXT NOT NULL CHECK(provider='spotify'), external_id TEXT NOT NULL,
 link_origin TEXT NOT NULL CHECK(link_origin='manual_confirmation'),
 source_fingerprint TEXT NOT NULL, confirmed_at INTEGER NOT NULL, PRIMARY KEY(song_id,provider));`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = d.conn.Exec("INSERT INTO track_external_identity VALUES (?,'spotify',?,'manual_confirmation',?,1)", "song", referenceID, fingerprint)
	if err != nil {
		t.Fatal(err)
	}
	d.externalSchemaReady = false
	if err = d.EnsureExternalTrackAnalysisSchema(); err != nil {
		t.Fatal(err)
	}
	link, err := d.GetSpotifyRecording("song", fingerprint)
	if err != nil || link == nil || link.LinkOrigin != "manual_confirmation" {
		t.Fatalf("migration lost manual link: %#v %v", link, err)
	}
	if _, err = d.conn.Exec("DELETE FROM songs WHERE id='song'"); err != nil {
		t.Fatal(err)
	}
	var count int
	if err = d.conn.QueryRow("SELECT COUNT(*) FROM track_external_identity").Scan(&count); err != nil || count != 0 {
		t.Fatalf("foreign key lost: %d %v", count, err)
	}
}

func TestConflictingCompletionRetiresEarlierAutomaticIdentity(t *testing.T) {
	d, path := evidenceFixture(t)
	fingerprint := scanEvidence(t, d, path, "song")
	finishEvidence(t, d, path, "one", referenceID)
	link, err := d.GetSpotifyRecording("song", fingerprint)
	if err != nil || link == nil {
		t.Fatalf("first link: %#v %v", link, err)
	}
	finishEvidence(t, d, path, "two", "11dFghVXANMlKmJXsNCbNl")
	link, err = d.GetSpotifyRecording("song", fingerprint)
	if err != nil || link != nil {
		t.Fatalf("conflicting evidence retained identity: %#v %v", link, err)
	}
}
