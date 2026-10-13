package scanner

import (
	"bytes"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/analysisbench"
	"github.com/ajbergh/viib-mediahub/internal/db"
)

func retentionScannerFixture(t *testing.T) (*db.DB, *sql.DB, *Scanner, string) {
	t.Helper()
	root := t.TempDir()
	databasePath := filepath.Join(t.TempDir(), "library.db")
	database, err := db.New(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	inspect, err := sql.Open("sqlite", databasePath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { inspect.Close() })
	if err := database.AddScanFolder(&db.ScanFolder{ID: "root", Path: root}); err != nil {
		t.Fatal(err)
	}
	scanner := New(database, t.TempDir())
	t.Cleanup(scanner.Close)
	return database, inspect, scanner, root
}
func completeScannerEvidence(t *testing.T, database *db.DB, path string) {
	t.Helper()
	if err := database.AddDownload(&db.SpotifyDownload{ID: "job", SpotifyID: "TTTTTTTTTTTTTTTTTTTTTT", Type: "track", Title: "Fixture", Status: "queued", AddedAt: 1}); err != nil {
		t.Fatal(err)
	}
	if changed, err := database.MarkDownloadStarted("job"); err != nil || !changed {
		t.Fatal(changed, err)
	}
	if changed, err := database.MarkDownloadCompletedWithEvidence(t.Context(), "job", path); err != nil || !changed {
		t.Fatal(changed, err)
	}
}
func TestFullScanCollectsExpiredDownloadedRevision(t *testing.T) {
	database, inspect, scanner, root := retentionScannerFixture(t)
	path := filepath.Join(root, "missing.mp3")
	if err := os.WriteFile(path, []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	completeScannerEvidence(t, database, path)
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if _, err := inspect.Exec("UPDATE spotify_download_revision_retention SET pending_scan=0,orphaned_at=?", time.Now().Add(-31*24*time.Hour).UnixMilli()); err != nil {
		t.Fatal(err)
	}
	if result, err := scanner.ScanAll(); err != nil || result.Errors != 0 {
		t.Fatal(result, err)
	}
	var n int
	if err := inspect.QueryRow("SELECT COUNT(*) FROM spotify_download_evidence").Scan(&n); err != nil || n != 0 {
		t.Fatal("full hook did not collect", n, err)
	}
}
func TestFailedFullScanPreservesPendingDownloadedRevision(t *testing.T) {
	database, inspect, scanner, root := retentionScannerFixture(t)
	path := filepath.Join(root, "pending.mp3")
	if err := os.WriteFile(path, []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	completeScannerEvidence(t, database, path)
	moved := root + "-temporarily-away"
	if err := os.Rename(root, moved); err != nil {
		t.Fatal(err)
	}
	defer os.Rename(moved, root)
	if result, err := scanner.ScanAll(); err != nil || result.Errors == 0 {
		t.Fatal(result, err)
	}
	var pending bool
	var checked int64
	if err := inspect.QueryRow("SELECT pending_scan,last_checked_at FROM spotify_download_revision_retention").Scan(&pending, &checked); err != nil || !pending || checked != 0 {
		t.Fatal("failed root advanced retention", pending, checked, err)
	}
}
func TestIncrementalMultipleBatchesRetainFirstBatchEvidence(t *testing.T) {
	database, inspect, scanner, root := retentionScannerFixture(t)
	fixture, err := analysisbench.NewClickTrack("fixture", 128, 1, 8000, 1)
	if err != nil {
		t.Fatal(err)
	}
	var wav bytes.Buffer
	if err := analysisbench.WriteWAVPCM16(&wav, fixture); err != nil {
		t.Fatal(err)
	}
	changes := []FileChange{}
	first := ""
	for i := 0; i < incrementalBatchSize+1; i++ {
		path := filepath.Join(root, fmt.Sprintf("%03d.wav", i))
		if err := os.WriteFile(path, wav.Bytes(), 0600); err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			first = path
		}
		changes = append(changes, FileChange{Path: path, ChangeType: ChangeTypeCreated})
	}
	completeScannerEvidence(t, database, first)
	if result, err := scanner.ProcessChanges(changes); err != nil || result.Errors != 0 || result.TotalFiles != incrementalBatchSize+1 {
		t.Fatal(result, err)
	}
	var pending bool
	var checked int64
	if err := inspect.QueryRow("SELECT pending_scan,last_checked_at FROM spotify_download_revision_retention").Scan(&pending, &checked); err != nil || pending || checked == 0 {
		t.Fatal("first batch not examined", pending, checked, err)
	}
}
func TestIncrementalDeletionRunsRetention(t *testing.T) {
	database, inspect, scanner, root := retentionScannerFixture(t)
	path := filepath.Join(root, "missing.mp3")
	if err := os.WriteFile(path, []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	completeScannerEvidence(t, database, path)
	// No canonical song is needed: old pending/absent completions are still
	// eligible after verified path coverage and a previously established grace.
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if _, err := inspect.Exec("UPDATE spotify_download_revision_retention SET pending_scan=0,orphaned_at=?", time.Now().Add(-31*24*time.Hour).UnixMilli()); err != nil {
		t.Fatal(err)
	}
	if _, err := scanner.ProcessChanges([]FileChange{{Path: path, ChangeType: ChangeTypeDeleted}}); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := inspect.QueryRow("SELECT COUNT(*) FROM spotify_download_evidence").Scan(&n); err != nil || n != 0 {
		t.Fatal("incremental hook did not collect", n, err)
	}
}

func TestSuccessfulScansCollectPrivateCachePayloads(t *testing.T) {
	for _, incremental := range []bool{false, true} {
		t.Run(fmt.Sprint(incremental), func(t *testing.T) {
			database, inspect, scanner, root := retentionScannerFixture(t)
			if err := database.ActivateSpotifyMetadataContext("owner"); err != nil {
				t.Fatal(err)
			}
			at := time.Now().Add(-24 * time.Hour)
			if err := database.PutSpotifyEntitySnapshot(db.SpotifyEntitySnapshot{SpotifySnapshotKey: db.SpotifySnapshotKey{EntityType: "track", SpotifyID: "TTTTTTTTTTTTTTTTTTTTTT", Resource: "track", ContextKey: "owner"}, SchemaVersion: 1, AdapterRevision: "fixture", Payload: []byte(`{}`), RetrievedAt: at, ExpiresAt: at.Add(time.Hour)}); err != nil {
				t.Fatal(err)
			}
			if incremental {
				if result, err := scanner.ProcessChanges([]FileChange{{Path: filepath.Join(root, "missing.mp3"), ChangeType: ChangeTypeDeleted}}); err != nil || result.Errors != 0 {
					t.Fatal(result, err)
				}
			} else if result, err := scanner.ScanAll(); err != nil || result.Errors != 0 {
				t.Fatal(result, err)
			}
			var count int
			if err := inspect.QueryRow("SELECT COUNT(*) FROM spotify_entity_snapshots").Scan(&count); err != nil || count != 0 {
				t.Fatal("actual scanner hook did not collect private payload", count, err)
			}
		})
	}
}

func TestFailedScanDefersPrivateCacheCleanup(t *testing.T) {
	database, inspect, scanner, root := retentionScannerFixture(t)
	if err := database.ActivateSpotifyMetadataContext("owner"); err != nil {
		t.Fatal(err)
	}
	at := time.Now().Add(-24 * time.Hour)
	if err := database.PutSpotifyEntitySnapshot(db.SpotifyEntitySnapshot{SpotifySnapshotKey: db.SpotifySnapshotKey{EntityType: "track", SpotifyID: "TTTTTTTTTTTTTTTTTTTTTT", Resource: "track", ContextKey: "owner"}, SchemaVersion: 1, AdapterRevision: "fixture", Payload: []byte(`{}`), RetrievedAt: at, ExpiresAt: at.Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(root); err != nil {
		t.Fatal(err)
	}
	if result, err := scanner.ScanAll(); err == nil && result.Errors == 0 {
		t.Fatal("missing root unexpectedly succeeded")
	}
	var count int
	if err := inspect.QueryRow("SELECT COUNT(*) FROM spotify_entity_snapshots").Scan(&count); err != nil || count != 1 {
		t.Fatal("failed scan collected private payload", count, err)
	}
}
