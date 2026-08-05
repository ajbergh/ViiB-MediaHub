package db

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

func TestConfigureRuntimeAndScannerFailures(t *testing.T) {
	database, err := New(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err := database.ConfigureRuntime(); err != nil {
		t.Fatalf("configure runtime: %v", err)
	}
	if err := database.ConfigureRuntime(); err != nil {
		t.Fatalf("configure runtime twice: %v", err)
	}

	stats := database.RuntimeStats()
	if stats.OpenConnections < 0 {
		t.Fatalf("invalid runtime stats: %#v", stats)
	}
	assertRuntimePolicyOnEveryConnection(t, database)

	path := filepath.Join(t.TempDir(), "bad.flac")
	allowed, err := database.ScannerFailureRetryAllowed(path)
	if err != nil || !allowed {
		t.Fatalf("new path should be retryable: allowed=%v err=%v", allowed, err)
	}
	if err := database.RecordScannerFailure(path, "metadata_timeout", "timeout"); err != nil {
		t.Fatalf("record scanner failure: %v", err)
	}
	allowed, err = database.ScannerFailureRetryAllowed(path)
	if err != nil {
		t.Fatalf("read scanner failure: %v", err)
	}
	if allowed {
		t.Fatalf("newly quarantined path should not be retryable immediately")
	}
	failures, err := database.ListScannerFailures(10)
	if err != nil || len(failures) != 1 {
		t.Fatalf("unexpected failures: %#v err=%v", failures, err)
	}
	if err := database.ClearScannerFailure(path); err != nil {
		t.Fatalf("clear failure: %v", err)
	}
	allowed, err = database.ScannerFailureRetryAllowed(path)
	if err != nil || !allowed {
		t.Fatalf("cleared path should be retryable")
	}
}

func assertRuntimePolicyOnEveryConnection(t *testing.T, database *DB) {
	t.Helper()
	connections := make([]*sql.Conn, 0, 4)
	for i := 0; i < 4; i++ {
		conn, err := database.conn.Conn(context.Background())
		if err != nil {
			t.Fatalf("reserve connection %d: %v", i, err)
		}
		connections = append(connections, conn)
		t.Cleanup(func() { _ = conn.Close() })

		var busyTimeout, synchronous, autoCheckpoint, tempStore, foreignKeys int
		var journalMode string
		checks := []struct {
			query string
			dest  any
		}{
			{`PRAGMA busy_timeout`, &busyTimeout},
			{`PRAGMA journal_mode`, &journalMode},
			{`PRAGMA synchronous`, &synchronous},
			{`PRAGMA wal_autocheckpoint`, &autoCheckpoint},
			{`PRAGMA temp_store`, &tempStore},
			{`PRAGMA foreign_keys`, &foreignKeys},
		}
		for _, check := range checks {
			if err := conn.QueryRowContext(context.Background(), check.query).Scan(check.dest); err != nil {
				t.Fatalf("connection %d %s: %v", i, check.query, err)
			}
		}
		if busyTimeout != 5000 || journalMode != "wal" || synchronous != 1 ||
			autoCheckpoint != 1000 || tempStore != 2 || foreignKeys != 1 {
			t.Fatalf("connection %d does not have the runtime policy: busy=%d journal=%s sync=%d checkpoint=%d temp=%d foreignKeys=%d",
				i, busyTimeout, journalMode, synchronous, autoCheckpoint, tempStore, foreignKeys)
		}
	}
	for _, conn := range connections {
		_ = conn.Close()
	}
}
