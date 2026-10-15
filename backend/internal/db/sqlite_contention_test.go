package db

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"
)

type contentionTestError int

func (e contentionTestError) Error() string { return "sqlite test error" }
func (e contentionTestError) Code() int     { return int(e) }

func TestSQLiteBusyRetryIsBoundedAndCancelable(t *testing.T) {
	for _, code := range []int{5, 517} {
		attempts := 0
		err := retrySQLiteBusy(context.Background(), func() error { attempts++; return fmt.Errorf("wrapped: %w", contentionTestError(code)) })
		if !isSQLiteBusy(err) || attempts != 3 {
			t.Fatalf("code=%d attempts=%d err=%v", code, attempts, err)
		}
	}
	attempts := 0
	failure := errors.New("constraint failed")
	if err := retrySQLiteBusy(context.Background(), func() error { attempts++; return failure }); err != failure || attempts != 1 {
		t.Fatal("retried a permanent error")
	}
	ctx, cancel := context.WithCancel(context.Background())
	attempts = 0
	err := retrySQLiteBusy(ctx, func() error { attempts++; cancel(); return contentionTestError(5) })
	if !errors.Is(err, context.Canceled) || attempts != 1 {
		t.Fatalf("cancellation: attempts=%d err=%v", attempts, err)
	}
}

func TestRuntimePoolIsBoundedBeforeFeatureInitialization(t *testing.T) {
	d, err := New(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if got := d.conn.Stats().MaxOpenConnections; got != 4 {
		t.Fatalf("startup pool cap=%d, want 4", got)
	}
}

func TestDownloadAndScannerRecoverAfterBusyTimeout(t *testing.T) {
	d, path := evidenceFixture(t)
	if err := d.EnsureExternalTrackAnalysisSchema(); err != nil {
		t.Fatal(err)
	}
	if err := d.RecordScannerFailure(path, "test", "test failure"); err != nil {
		t.Fatal(err)
	}
	if err := d.AddDownload(&SpotifyDownload{ID: "busy-download", SpotifyID: referenceID, Type: "track", Status: "queued"}); err != nil {
		t.Fatal(err)
	}
	if changed, err := d.MarkDownloadStarted("busy-download"); err != nil || !changed {
		t.Fatalf("start: %v %v", changed, err)
	}
	if err := d.EnsureSemanticSchema(); err != nil {
		t.Fatal(err)
	}
	writer, err := d.conn.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Rollback()
	if _, err := writer.Exec(`UPDATE settings SET value=value WHERE 0`); err != nil {
		t.Fatal(err)
	}
	released := make(chan error, 1)
	go func() { time.Sleep(6 * time.Second); released <- writer.Commit() }()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	results := make(chan error, 4)
	go func() {
		changed, err := d.MarkDownloadCompletedWithEvidence(ctx, "busy-download", path)
		if err == nil && !changed {
			err = errors.New("download did not complete")
		}
		results <- err
	}()
	go func() { results <- d.SaveScanState(ScanState{LastScanTime: 1234, FilesChanged: 3}) }()
	go func() { results <- d.ClearScannerFailure(path) }()
	go func() {
		results <- d.UpsertSemanticDocuments(ctx, []SemanticDocument{{EntityType: SemanticEntityArtist, EntityKey: "artist", Content: "Artist", ContentHash: "hash", DocumentVersion: 1}})
	}()
	for range 4 {
		select {
		case err := <-results:
			if err != nil {
				t.Error(err)
			}
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	if err := <-released; err != nil {
		t.Fatal(err)
	}
	download, err := d.GetDownload("busy-download")
	if err != nil || download.Status != "completed" || download.FilePath != path {
		t.Fatalf("completion=%+v err=%v", download, err)
	}
	var evidence int
	if err := d.conn.QueryRow(`SELECT count(*) FROM spotify_download_evidence WHERE file_path=?`, path).Scan(&evidence); err != nil || evidence != 1 {
		t.Fatalf("evidence=%d err=%v", evidence, err)
	}
	state, err := d.GetScanState()
	if err != nil || state.LastScanTime != 1234 {
		t.Fatalf("state=%+v err=%v", state, err)
	}
	if allowed, err := d.ScannerFailureRetryAllowed(path); err != nil || !allowed {
		t.Fatalf("failure not cleared: %v %v", allowed, err)
	}
}

func TestSemanticTransactionReservesWriterBeforeReading(t *testing.T) {
	d := newSemanticTestDB(t)
	if err := d.EnsureSemanticSchema(); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	tx, err := d.beginSemanticWriteTx(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	other, err := d.conn.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	if _, err := other.ExecContext(ctx, `PRAGMA busy_timeout=1`); err != nil {
		t.Fatal(err)
	}
	defer other.ExecContext(ctx, `PRAGMA busy_timeout=5000`)
	if _, err := other.ExecContext(ctx, `UPDATE settings SET value=value WHERE 0`); !isSQLiteBusy(err) {
		t.Fatalf("semantic transaction did not reserve writer: %v", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE semantic_index_state SET last_error=''`); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if _, err := other.ExecContext(ctx, `UPDATE settings SET value=value WHERE 0`); err != nil {
		t.Fatal(err)
	}
}
