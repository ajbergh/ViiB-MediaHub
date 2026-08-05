package db

import (
	"database/sql"
	"database/sql/driver"
	"fmt"
	"sync"
	"time"

	"github.com/mattn/go-sqlite3"
)

const sqliteRuntimeDriverName = "viib_sqlite3"

var (
	runtimePolicy    sync.Map
	sqliteDriverOnce sync.Once
)

var connectionPragmas = []string{
	`PRAGMA busy_timeout = 5000`,
	`PRAGMA journal_mode = WAL`,
	`PRAGMA synchronous = NORMAL`,
	`PRAGMA wal_autocheckpoint = 1000`,
	`PRAGMA temp_store = MEMORY`,
	`PRAGMA foreign_keys = ON`,
}

func registerSQLiteRuntimeDriver() {
	sqliteDriverOnce.Do(func() {
		sql.Register(sqliteRuntimeDriverName, &sqlite3.SQLiteDriver{
			ConnectHook: func(conn *sqlite3.SQLiteConn) error {
				executor, ok := any(conn).(interface {
					Exec(string, []driver.Value) (driver.Result, error)
				})
				if !ok {
					return fmt.Errorf("SQLite connection does not support runtime PRAGMAs")
				}
				for _, pragma := range connectionPragmas {
					if _, err := executor.Exec(pragma, nil); err != nil {
						return fmt.Errorf("apply SQLite connection policy %q: %w", pragma, err)
					}
				}
				return nil
			},
		})
	})
}

func sqliteRuntimeDSN(dbPath string) string {
	return dbPath + "?_foreign_keys=on&_journal_mode=WAL&_busy_timeout=5000&_synchronous=NORMAL"
}

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
	OpenConnections int   `json:"openConnections"`
	InUse           int   `json:"inUse"`
	Idle            int   `json:"idle"`
	WaitCount       int64 `json:"waitCount"`
	WaitDurationMS  int64 `json:"waitDurationMs"`
}

func (d *DB) RuntimeStats() SQLiteRuntimeStats {
	stats := d.conn.Stats()
	return SQLiteRuntimeStats{
		OpenConnections: stats.OpenConnections,
		InUse:           stats.InUse,
		Idle:            stats.Idle,
		WaitCount:       stats.WaitCount,
		WaitDurationMS:  stats.WaitDuration.Milliseconds(),
	}
}
