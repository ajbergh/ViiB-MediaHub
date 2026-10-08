package db

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func retentionRun(t *testing.T, d *DB, path string, now time.Time) DownloadRetentionResult {
	t.Helper()
	r, err := d.MaintainSpotifyDownloadRetention(t.Context(), []string{filepath.Dir(path)}, nil, now, 128)
	if err != nil {
		t.Fatal(err)
	}
	return r
}
func retentionCount(t *testing.T, d *DB, table string) int {
	t.Helper()
	var n int
	if err := d.conn.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}
func TestDownloadRetentionPendingGraceAndWholeGroup(t *testing.T) {
	d, path := evidenceFixture(t)
	if err := d.ActivateSpotifyMetadataContext("owner"); err != nil {
		t.Fatal(err)
	}
	collectionPages(t, d)
	id := queueLineage(t, d, "collections", collectionOrigins())
	completeLineage(t, d, path, id)
	finishEvidence(t, d, path, "conflict", "11dFghVXANMlKmJXsNCbNl")
	if _, err := d.DeleteCompletedDownloads(); err != nil {
		t.Fatal(err)
	}
	if err := d.PurgeSpotifyMetadata(); err != nil {
		t.Fatal(err)
	}
	now := time.Now().Add(90 * 24 * time.Hour)
	if r, err := d.MaintainSpotifyDownloadRetention(t.Context(), nil, nil, now, 128); err != nil || r.Checked != 0 {
		t.Fatal(r, err)
	}
	if retentionCount(t, d, "spotify_download_evidence") != 2 {
		t.Fatal("pending bundles lost")
	}
	first := retentionRun(t, d, path, now)
	if first.Orphaned != 1 || first.Collected != 0 {
		t.Fatal(first)
	}
	if r := retentionRun(t, d, path, now.Add(SpotifyDownloadOrphanGrace-time.Millisecond)); r.Collected != 0 {
		t.Fatal(r)
	}
	if r := retentionRun(t, d, path, now.Add(SpotifyDownloadOrphanGrace)); r.Collected != 1 {
		t.Fatal(r)
	}
	for _, table := range downloadRevisionTables {
		if n := retentionCount(t, d, table); n != 0 {
			t.Fatal(table, n)
		}
	}
	if n := retentionCount(t, d, "spotify_download_revision_retention"); n != 0 {
		t.Fatal(n)
	}
}
func TestDownloadRetentionOwnedAndUnlinkChoice(t *testing.T) {
	d, path := evidenceFixture(t)
	finishEvidence(t, d, path, "job", referenceID)
	fp := scanEvidence(t, d, path, "song")
	if err := d.DeleteSpotifyRecording("song"); err != nil {
		t.Fatal(err)
	}
	now := time.Now().Add(90 * 24 * time.Hour)
	if r := retentionRun(t, d, path, now); r.Owned != 1 || r.Collected != 0 {
		t.Fatal(r)
	}
	if err := d.DeleteSong("song"); err != nil {
		t.Fatal(err)
	}
	if r := retentionRun(t, d, path, now); r.Collected != 1 {
		t.Fatal(r)
	}
	if n := retentionCount(t, d, "spotify_download_revision_suppression"); n != 1 {
		t.Fatal("choice erased", n)
	}
	fp = scanEvidence(t, d, path, "new")
	if allowed, err := d.SpotifySearchAllowed("new", fp); err != nil || allowed {
		t.Fatal("choice bypassed after payload collection", allowed, err)
	}
	if err := d.RefreshTrackAnalysisSourceRevision("new", fp); err != nil {
		t.Fatal(err)
	}
	if changed, err := d.ConfirmSpotifyRecording("new", referenceID, fp, true); err != nil || !changed {
		t.Fatal(changed, err)
	}
	if n := retentionCount(t, d, "spotify_download_revision_suppression"); n != 0 {
		t.Fatal(n)
	}
}
func TestDownloadRetentionRescanAndSourceReplacement(t *testing.T) {
	d, path := evidenceFixture(t)
	finishEvidence(t, d, path, "old", referenceID)
	scanEvidence(t, d, path, "song")
	if err := d.DeleteSong("song"); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	if r := retentionRun(t, d, path, now); r.Collected != 0 {
		t.Fatal(r)
	}
	scanEvidence(t, d, path, "canonical")
	if r := retentionRun(t, d, path, now.Add(90*24*time.Hour)); r.Owned != 1 || r.Collected != 0 {
		t.Fatal(r)
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
	finishEvidence(t, d, path, "new", referenceID)
	scanEvidence(t, d, path, "canonical")
	r := retentionRun(t, d, path, now.Add(91*24*time.Hour))
	if r.Owned != 1 || r.Collected != 1 {
		t.Fatal("same-fingerprint overwrite not retired", r)
	}
	if n := retentionCount(t, d, "spotify_download_evidence"); n != 1 {
		t.Fatal(n)
	}
}
func TestDownloadRetentionRollbackAndBounds(t *testing.T) {
	d, path := evidenceFixture(t)
	finishEvidence(t, d, path, "job", referenceID)
	now := time.Now()
	retentionRun(t, d, path, now)
	if _, err := d.conn.Exec(`CREATE TRIGGER fail_retention BEFORE DELETE ON spotify_download_evidence BEGIN SELECT RAISE(ABORT,'fixture'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := d.MaintainSpotifyDownloadRetention(t.Context(), []string{filepath.Dir(path)}, nil, now.Add(SpotifyDownloadOrphanGrace), 1); err == nil {
		t.Fatal("expected rollback")
	}
	if n := retentionCount(t, d, "spotify_download_revision_retention"); n != 1 {
		t.Fatal(n)
	}
	if n := retentionCount(t, d, "spotify_download_evidence"); n != 1 {
		t.Fatal(n)
	}
	if _, err := d.conn.Exec("DROP TRIGGER fail_retention"); err != nil {
		t.Fatal(err)
	}
	second := filepath.Join(filepath.Dir(path), "second.mp3")
	if err := os.WriteFile(second, []byte("second"), 0600); err != nil {
		t.Fatal(err)
	}
	finishEvidence(t, d, second, "second", referenceID)
	if r, err := d.MaintainSpotifyDownloadRetention(t.Context(), []string{filepath.Dir(path)}, nil, now.Add(SpotifyDownloadOrphanGrace), 1); err != nil || r.Checked != 1 {
		t.Fatal(r, err)
	}
	if _, err := d.MaintainSpotifyDownloadRetention(t.Context(), nil, nil, now, 129); err == nil {
		t.Fatal("invalid bound accepted")
	}
}
func TestDownloadRetentionUnavailableRootPreservesPending(t *testing.T) {
	d, path := evidenceFixture(t)
	finishEvidence(t, d, path, "job", referenceID)
	root := filepath.Join(filepath.Dir(path), "missing-root")
	if r, err := d.MaintainSpotifyDownloadRetention(t.Context(), []string{root}, nil, time.Now().Add(90*24*time.Hour), 128); err != nil || r.Checked != 0 {
		t.Fatal(r, err)
	}
	var pending bool
	if err := d.conn.QueryRow("SELECT pending_scan FROM spotify_download_revision_retention").Scan(&pending); err != nil || !pending {
		t.Fatal(pending, err)
	}
}

func TestDownloadRetentionRetiresOnlyCollectedScalarProjection(t *testing.T) {
	d, path := evidenceFixture(t)
	finishEvidence(t, d, path, "job", referenceID)
	fp := scanEvidence(t, d, path, "song")
	o := referenceObservation()
	raw, err := json.Marshal(o)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.conn.Exec("UPDATE spotify_download_evidence SET features_json=?", string(raw)); err != nil {
		t.Fatal(err)
	}
	if err := d.ReconcileSpotifyDownload(t.Context(), path); err != nil {
		t.Fatal(err)
	}
	record, err := d.GetTrackAnalysis("song")
	if err != nil || record.SpotifyBindings == nil || record.SpotifyBindings.BPM.DownloadRevision == "" {
		t.Fatal(record, err)
	}
	oldIdentity := record.SpotifyBindings.BPM.DownloadRevision
	// Preserve measured fields and manual locks while removing only old provider facts.
	measured := 123.0
	energy := 7
	record.Local = &LocalScalarObservation{SourceFingerprint: fp, AlgorithmVersion: "fixture", BPM: &measured}
	record.EnergyLevel = &energy
	record.EnergyLevelConfidence = scalarPtr(0.8)
	record.EnergyAlgorithmVersion = scalarPtr("fixture")
	if err := d.UpsertTrackAnalysis(record); err != nil {
		t.Fatal(err)
	}
	if err := d.UpsertTrackAnalysisOverride(TrackAnalysisOverride{SongID: "song", BPM: scalarPtr(140.0), BPMLocked: true}); err != nil {
		t.Fatal(err)
	}
	next := filepath.Join(filepath.Dir(path), "moved.mp3")
	if err := os.Rename(path, next); err != nil {
		t.Fatal(err)
	}
	if _, err := d.conn.Exec("UPDATE songs SET file_path=? WHERE id='song'", next); err != nil {
		t.Fatal(err)
	}
	if r := retentionRun(t, d, path, time.Now().Add(90*24*time.Hour)); r.Collected != 1 {
		t.Fatal(r)
	}
	record, err = d.GetTrackAnalysis("song")
	if err != nil || record.SpotifyBindings != nil || record.BPM == nil || *record.BPM != measured || record.EnergyLevel == nil || *record.EnergyLevel != energy {
		t.Fatal(record, oldIdentity, err)
	}
	override, err := d.GetTrackAnalysisOverride("song")
	if err != nil || override.BPM == nil || *override.BPM != 140 || !override.BPMLocked {
		t.Fatal(override, err)
	}
}

func TestDownloadRetentionKeepsNewScalarAtCollidingSource(t *testing.T) {
	d, path := evidenceFixture(t)
	finishEvidence(t, d, path, "old", referenceID)
	scanEvidence(t, d, path, "song")
	o := referenceObservation()
	raw, err := json.Marshal(o)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.conn.Exec("UPDATE spotify_download_evidence SET features_json=?", string(raw)); err != nil {
		t.Fatal(err)
	}
	if err := d.ReconcileSpotifyDownload(t.Context(), path); err != nil {
		t.Fatal(err)
	}
	old, err := d.GetTrackAnalysis("song")
	if err != nil {
		t.Fatal(err)
	}
	oldKey := old.SpotifyBindings.BPM.DownloadRevision
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	audio, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	audio[0] ^= 1
	if err := os.WriteFile(path, audio, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
	finishEvidence(t, d, path, "new", referenceID)
	// Both revisions have the same source token and recording but new scalars.
	o.BPM = scalarPtr(149.0)
	raw, err = json.Marshal(o)
	if err != nil {
		t.Fatal(err)
	}
	revision, err := readDownloadRevision(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.conn.Exec("UPDATE spotify_download_evidence SET features_json=? WHERE content_sha256=?", string(raw), revision.digest); err != nil {
		t.Fatal(err)
	}
	if err := d.ReconcileSpotifyDownload(t.Context(), path); err != nil {
		t.Fatal(err)
	}
	current, err := d.GetTrackAnalysis("song")
	if err != nil || *current.BPM != 149 || current.SpotifyBindings.BPM.DownloadRevision == oldKey {
		t.Fatal(current, err)
	}
	now := time.Now().Add(90 * 24 * time.Hour)
	if r := retentionRun(t, d, path, now); r.Collected != 1 || r.Owned != 1 {
		t.Fatal(r)
	}
	current, err = d.GetTrackAnalysis("song")
	if err != nil || current.SpotifyBindings == nil || *current.BPM != 149 {
		t.Fatal("new projection removed", current, err)
	}
}

func TestDownloadRetentionWindowsCaseVariantOwnership(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows path identity")
	}
	d, path := evidenceFixture(t)
	finishEvidence(t, d, path, "job", referenceID)
	scanEvidence(t, d, path, "song")
	if _, err := d.conn.Exec("UPDATE songs SET file_path=? WHERE id='song'", strings.ToUpper(path)); err != nil {
		t.Fatal(err)
	}
	if r := retentionRun(t, d, path, time.Now().Add(90*24*time.Hour)); r.Owned != 1 || r.Collected != 0 {
		t.Fatal("case variant owner lost", r)
	}
}

func TestDownloadRetentionAllRemovalPathsStartGrace(t *testing.T) {
	for _, mode := range []string{"delete", "clear", "paths", "plex_snapshot", "plex_remove"} {
		t.Run(mode, func(t *testing.T) {
			d, path := evidenceFixture(t)
			finishEvidence(t, d, path, "job", referenceID)
			scanEvidence(t, d, path, "song")
			if strings.HasPrefix(mode, "plex_") {
				if err := d.SavePlexSource(PlexSource{ID: "source", MachineIdentifier: "machine", BaseURL: "http://127.0.0.1:32400", Name: "Fixture", LibraryID: "2", Active: true, Available: true}); err != nil {
					t.Fatal(err)
				}
				track := plexFixture("source", "2", "machine", "1", "Fixture", 1)
				if _, _, _, err := d.SyncPlexLibrary("source", "2", []PlexCatalogTrack{track}); err != nil {
					t.Fatal(err)
				}
				// A historical binding can survive a source-mode change; direct Plex
				// removal must mark it before the FK cascade, like local deletion does.
				if _, err := d.conn.Exec("UPDATE spotify_download_import_bindings SET song_id=? WHERE song_id='song'", track.SongID); err != nil {
					t.Fatal(err)
				}
			}
			switch mode {
			case "delete":
				if err := d.DeleteSong("song"); err != nil {
					t.Fatal(err)
				}
			case "clear":
				if err := d.ClearSongs(); err != nil {
					t.Fatal(err)
				}
			case "paths":
				if _, err := d.DeleteSongsByFilePaths([]string{path}); err != nil {
					t.Fatal(err)
				}
			case "plex_snapshot":
				if _, _, removed, err := d.SyncPlexLibrary("source", "2", nil); err != nil || removed != 1 {
					t.Fatal(removed, err)
				}
			case "plex_remove":
				if err := d.RemovePlexSource("source"); err != nil {
					t.Fatal(err)
				}
			}
			var pending bool
			var orphan sql.NullInt64
			if err := d.conn.QueryRow("SELECT pending_scan,orphaned_at FROM spotify_download_revision_retention").Scan(&pending, &orphan); err != nil || pending || !orphan.Valid {
				t.Fatal("removal did not start grace", pending, orphan, err)
			}
			if retentionCount(t, d, "spotify_download_evidence") != 1 {
				t.Fatal("immediate removal discarded evidence")
			}
		})
	}
}

func TestDownloadRetentionCompletionAfterCoverageStaysPending(t *testing.T) {
	d, path := evidenceFixture(t)
	coverage := time.Now()
	finishEvidence(t, d, path, "job", referenceID)
	if r, err := d.MaintainSpotifyDownloadRetention(t.Context(), []string{filepath.Dir(path)}, nil, coverage, 128); err != nil || r.Orphaned != 0 || r.Collected != 0 {
		t.Fatal(r, err)
	}
	var pending bool
	if err := d.conn.QueryRow("SELECT pending_scan FROM spotify_download_revision_retention").Scan(&pending); err != nil || !pending {
		t.Fatal("stale traversal retired new completion", pending, err)
	}
}

func TestDownloadRetentionDeferredGroupDoesNotStarveNext(t *testing.T) {
	d, path := evidenceFixture(t)
	finishEvidence(t, d, path, "a", referenceID)
	second := filepath.Join(filepath.Dir(path), "z.mp3")
	if err := os.WriteFile(second, []byte("second"), 0600); err != nil {
		t.Fatal(err)
	}
	finishEvidence(t, d, second, "z", referenceID)
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := d.conn.Exec("UPDATE spotify_download_revision_retention SET pending_scan=0,orphaned_at=?", time.Now().Add(-31*24*time.Hour).UnixMilli()); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	if r, err := d.MaintainSpotifyDownloadRetention(t.Context(), []string{filepath.Dir(path)}, nil, now, 1); err != nil || r.Checked != 1 || r.Collected != 0 {
		t.Fatal(r, err)
	}
	if r, err := d.MaintainSpotifyDownloadRetention(t.Context(), []string{filepath.Dir(path)}, nil, now.Add(time.Millisecond), 1); err != nil || r.Collected != 1 {
		t.Fatal("deferred group starved unrelated collection", r, err)
	}
	if n := retentionCount(t, d, "spotify_download_evidence"); n != 1 {
		t.Fatal(n)
	}
}

func TestDownloadRetentionSymlinkDescendantIsNotFullCoverage(t *testing.T) {
	d, path := evidenceFixture(t)
	finishEvidence(t, d, path, "job", referenceID)
	covered := t.TempDir()
	link := filepath.Join(covered, "outside")
	if err := os.Symlink(filepath.Dir(path), link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	alias := filepath.Join(link, filepath.Base(path))
	if _, err := d.conn.Exec("UPDATE spotify_download_revision_retention SET file_path=?,pending_scan=0,orphaned_at=?", alias, time.Now().Add(-31*24*time.Hour).UnixMilli()); err != nil {
		t.Fatal(err)
	}
	if r, err := d.MaintainSpotifyDownloadRetention(t.Context(), []string{covered}, nil, time.Now(), 128); err != nil || r.Collected != 0 {
		t.Fatal(r, err)
	}
	if retentionCount(t, d, "spotify_download_revision_retention") != 1 {
		t.Fatal("symlink outside coverage collected")
	}
}

func TestDownloadRetentionDefersOwnedPreparationProjection(t *testing.T) {
	d, path := evidenceFixture(t)
	finishEvidence(t, d, path, "job", referenceID)
	scanEvidence(t, d, path, "song")
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
	if _, err := d.conn.Exec("UPDATE track_analysis SET status='running' WHERE song_id='song'"); err != nil {
		t.Fatal(err)
	}
	next := filepath.Join(filepath.Dir(path), "moved.mp3")
	if err := os.Rename(path, next); err != nil {
		t.Fatal(err)
	}
	if _, err := d.conn.Exec("UPDATE songs SET file_path=? WHERE id='song'", next); err != nil {
		t.Fatal(err)
	}
	now := time.Now().Add(90 * 24 * time.Hour)
	if r := retentionRun(t, d, path, now); r.Collected != 0 {
		t.Fatal("owned projection retired", r)
	}
	if retentionCount(t, d, "spotify_download_evidence") != 1 {
		t.Fatal("busy preparation evidence removed")
	}
	if _, err := d.conn.Exec("UPDATE track_analysis SET status='partial' WHERE song_id='song'"); err != nil {
		t.Fatal(err)
	}
	if r := retentionRun(t, d, path, now.Add(time.Second)); r.Collected != 1 {
		t.Fatal("released projection never collected", r)
	}
}
