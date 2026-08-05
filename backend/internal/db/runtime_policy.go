package db

import (
	"fmt"
	"sync"
	"time"
)

var runtimePolicy sync.Map

// ConfigureRuntime applies an explicit SQLite policy instead of relying on
// database/sql defaults. WAL still permits readers during writes, while a small
// pool and busy timeout prevent scanner bursts from creating unbounded
// connection and lock contention.
func (d *DB) ConfigureRuntime() error {
	if _, loaded := runtimePolicy.LoadOrStore(d, struct{}{}); loaded {
		return nil
	}

	d.conn.SetMaxOpenConns(4)
	d.conn.SetMaxIdleConns(2)
	d.conn.SetConnMaxIdleTime(5 * time.Minute)
	d.conn.SetConnMaxLifetime(0)

	pragmas := []string{
		`PRAGMA busy_timeout = 5000`,
		`PRAGMA journal_mode = WAL`,
		`PRAGMA synchronous = NORMAL`,
		`PRAGMA wal_autocheckpoint = 1000`,
		`PRAGMA temp_store = MEMORY`,
		`PRAGMA foreign_keys = ON`,
	}
	for _, pragma := range pragmas {
		if _, err := d.conn.Exec(pragma); err != nil {
			runtimePolicy.Delete(d)
			return fmt.Errorf("apply SQLite runtime policy %q: %w", pragma, err)
		}
	}
	return nil
}

// CheckpointWAL bounds WAL growth after bulk work. PASSIVE avoids blocking
// active readers; a later checkpoint can complete remaining frames.
func (d *DB) CheckpointWAL() error {
	_, err := d.conn.Exec(`PRAGMA wal_checkpoint(PASSIVE)`)
	return err
}

// SQLiteRuntimeStats is intentionally small and safe to surface in local-only
// diagnostics.
type SQLiteRuntimeStats struct {
	OpenConnections int `json:"openConnections"`
	InUse           int `json:"inUse"`
	Idle            int `json:"idle"`
	WaitCount       int64 `json:"waitCount"`
	WaitDurationMS  int64 `json:"waitDurationMs"`
}

func (d *DB) RuntimeStats() SQLiteRuntimeStats {
	stats := d.conn.Stats()
	return SQLiteRuntimeStats{
		OpenConnections: stats.OpenConnections,
		InUse: stats.InUse,
		Idle: stats.Idle,
		WaitCount: stats.WaitCount,
		WaitDurationMS: stats.WaitDuration.Milliseconds(),
	}
}
