// scanner_failures.go persists bounded retry state for troublesome media files.
package db

import (
	"database/sql"
	"time"
)

// ScannerFailure records repeated media parsing failures without blocking the
// rest of a library scan.
type ScannerFailure struct {
	FilePath    string `json:"filePath"`
	FailureKind string `json:"failureKind"`
	Message     string `json:"message"`
	Attempts    int    `json:"attempts"`
	FirstSeenAt int64  `json:"firstSeenAt"`
	LastSeenAt  int64  `json:"lastSeenAt"`
	RetryAfter  int64  `json:"retryAfter"`
}

func (d *DB) EnsureScannerFailureSchema() error {
	_, err := d.conn.Exec(`
		CREATE TABLE IF NOT EXISTS scanner_failures (
			file_path TEXT PRIMARY KEY,
			failure_kind TEXT NOT NULL,
			message TEXT NOT NULL,
			attempts INTEGER NOT NULL DEFAULT 1,
			first_seen_at INTEGER NOT NULL,
			last_seen_at INTEGER NOT NULL,
			retry_after INTEGER NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_scanner_failures_retry ON scanner_failures(retry_after);
	`)
	return err
}

// RecordScannerFailure increments the failure count and delays the next retry
// with a capped quadratic backoff.
func (d *DB) RecordScannerFailure(filePath, kind, message string) error {
	if err := d.EnsureScannerFailureSchema(); err != nil { return err }
	now := time.Now().UnixMilli()
	_, err := d.conn.Exec(`
		INSERT INTO scanner_failures(file_path, failure_kind, message, attempts, first_seen_at, last_seen_at, retry_after)
		VALUES(?, ?, ?, 1, ?, ?, ?)
		ON CONFLICT(file_path) DO UPDATE SET
			failure_kind = excluded.failure_kind,
			message = excluded.message,
			attempts = scanner_failures.attempts + 1,
			last_seen_at = excluded.last_seen_at,
			retry_after = excluded.last_seen_at + MIN(86400000, 60000 * (scanner_failures.attempts + 1) * (scanner_failures.attempts + 1))
	`, filePath, kind, message, now, now, now+60000)
	return err
}

func (d *DB) ClearScannerFailure(filePath string) error {
	if err := d.EnsureScannerFailureSchema(); err != nil { return err }
	_, err := d.conn.Exec(`DELETE FROM scanner_failures WHERE file_path = ?`, filePath)
	return err
}

// ScannerFailureRetryAllowed reports whether a quarantined path may be parsed again.
func (d *DB) ScannerFailureRetryAllowed(filePath string) (bool, error) {
	if err := d.EnsureScannerFailureSchema(); err != nil { return false, err }
	var retryAfter int64
	err := d.conn.QueryRow(`SELECT retry_after FROM scanner_failures WHERE file_path = ?`, filePath).Scan(&retryAfter)
	if err == sql.ErrNoRows { return true, nil }
	if err != nil { return false, err }
	return time.Now().UnixMilli() >= retryAfter, nil
}

func (d *DB) ListScannerFailures(limit int) ([]ScannerFailure, error) {
	if err := d.EnsureScannerFailureSchema(); err != nil { return nil, err }
	if limit <= 0 || limit > 1000 { limit = 250 }
	rows, err := d.conn.Query(`
		SELECT file_path, failure_kind, message, attempts, first_seen_at, last_seen_at, retry_after
		FROM scanner_failures ORDER BY last_seen_at DESC LIMIT ?`, limit)
	if err != nil { return nil, err }
	defer rows.Close()
	failures := make([]ScannerFailure, 0)
	for rows.Next() {
		var failure ScannerFailure
		if err := rows.Scan(&failure.FilePath, &failure.FailureKind, &failure.Message, &failure.Attempts, &failure.FirstSeenAt, &failure.LastSeenAt, &failure.RetryAfter); err != nil {
			return nil, err
		}
		failures = append(failures, failure)
	}
	return failures, rows.Err()
}
