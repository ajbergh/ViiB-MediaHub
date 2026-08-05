package db

import (
	"path/filepath"
	"testing"
)

func TestConfigureRuntimeAndScannerFailures(t *testing.T) {
	database, err := New(filepath.Join(t.TempDir(), "library.db"))
	if err != nil { t.Fatalf("open database: %v", err) }
	defer database.Close()
	if err := database.ConfigureRuntime(); err != nil { t.Fatalf("configure runtime: %v", err) }
	if err := database.ConfigureRuntime(); err != nil { t.Fatalf("configure runtime twice: %v", err) }

	stats := database.RuntimeStats()
	if stats.OpenConnections < 0 { t.Fatalf("invalid runtime stats: %#v", stats) }

	path := filepath.Join(t.TempDir(), "bad.flac")
	allowed, err := database.ScannerFailureRetryAllowed(path)
	if err != nil || !allowed { t.Fatalf("new path should be retryable: allowed=%v err=%v", allowed, err) }
	if err := database.RecordScannerFailure(path, "metadata_timeout", "timeout"); err != nil {
		t.Fatalf("record scanner failure: %v", err)
	}
	allowed, err = database.ScannerFailureRetryAllowed(path)
	if err != nil { t.Fatalf("read scanner failure: %v", err) }
	if allowed { t.Fatalf("newly quarantined path should not be retryable immediately") }
	failures, err := database.ListScannerFailures(10)
	if err != nil || len(failures) != 1 { t.Fatalf("unexpected failures: %#v err=%v", failures, err) }
	if err := database.ClearScannerFailure(path); err != nil { t.Fatalf("clear failure: %v", err) }
	allowed, err = database.ScannerFailureRetryAllowed(path)
	if err != nil || !allowed { t.Fatalf("cleared path should be retryable") }
}
