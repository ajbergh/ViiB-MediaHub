// jobs_schema.go defines durable operation-job state and restart recovery.
package db

import (
	"encoding/json"
	"strings"
	"sync"
	"time"
)

const (
	JobStatusQueued      = "queued"
	JobStatusRunning     = "running"
	JobStatusPaused      = "paused"
	JobStatusSucceeded   = "succeeded"
	JobStatusFailed      = "failed"
	JobStatusCanceling   = "canceling"
	JobStatusCanceled    = "canceled"
	JobStatusInterrupted = "interrupted"
)

// Job is the persisted state of a scan or aggregate-refresh operation.
type Job struct {
	ID              string          `json:"id"`
	Type            string          `json:"type"`
	Status          string          `json:"status"`
	ProgressCurrent int64           `json:"progressCurrent"`
	ProgressTotal   int64           `json:"progressTotal"`
	Priority        int             `json:"priority"`
	Message         string          `json:"message,omitempty"`
	Parameters      json.RawMessage `json:"parameters,omitempty"`
	Result          json.RawMessage `json:"result,omitempty"`
	ErrorCode       string          `json:"errorCode,omitempty"`
	ErrorMessage    string          `json:"errorMessage,omitempty"`
	Attempts        int             `json:"attempts"`
	CreatedAt       int64           `json:"createdAt"`
	StartedAt       int64           `json:"startedAt,omitempty"`
	CompletedAt     int64           `json:"completedAt,omitempty"`
	UpdatedAt       int64           `json:"updatedAt"`
}

type jobSchemaResult struct{ err error }

var jobSchemas sync.Map // map[*DB]jobSchemaResult

// EnsureJobSchema installs and performs restart recovery exactly once for each
// live DB handle. A successful initialization is stored as a non-nil result
// object so sync.Map never receives a nil value.
func (d *DB) EnsureJobSchema() error {
	if value, ok := jobSchemas.Load(d); ok {
		return value.(jobSchemaResult).err
	}

	_, err := d.conn.Exec(`
		CREATE TABLE IF NOT EXISTS operation_jobs (
			id TEXT PRIMARY KEY,
			type TEXT NOT NULL,
			status TEXT NOT NULL,
			progress_current INTEGER NOT NULL DEFAULT 0,
			progress_total INTEGER NOT NULL DEFAULT 0,
			priority INTEGER NOT NULL DEFAULT 0,
			available_at INTEGER NOT NULL DEFAULT 0,
			message TEXT,
			parameters TEXT,
			result TEXT,
			error_code TEXT,
			error_message TEXT,
			attempts INTEGER NOT NULL DEFAULT 0,
			created_at INTEGER NOT NULL,
			started_at INTEGER,
			completed_at INTEGER,
			updated_at INTEGER NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_operation_jobs_status ON operation_jobs(status, updated_at);
		CREATE INDEX IF NOT EXISTS idx_operation_jobs_type ON operation_jobs(type, created_at);
	`)
	// A database created before the scheduler existed has neither the priority
	// nor the available_at column. Both must be added before any index or query
	// references them, so the queue index is created only after the migrations
	// succeed.
	if err == nil {
		err = addJobColumns(d,
			`ALTER TABLE operation_jobs ADD COLUMN priority INTEGER NOT NULL DEFAULT 0`,
			`ALTER TABLE operation_jobs ADD COLUMN available_at INTEGER NOT NULL DEFAULT 0`,
		)
	}
	if err == nil {
		_, err = d.conn.Exec(`CREATE INDEX IF NOT EXISTS idx_operation_jobs_queue ON operation_jobs(status, priority DESC, created_at)`)
	}
	if err == nil {
		now := time.Now().UnixMilli()
		_, err = d.conn.Exec(`
			UPDATE operation_jobs
			SET status = ?, error_code = 'process_restarted',
			    error_message = 'The application restarted while the job was active',
			    completed_at = ?, updated_at = ?
			WHERE status IN (?, ?)
		`, JobStatusInterrupted, now, now, JobStatusRunning, JobStatusCanceling)
	}
	result := jobSchemaResult{err: err}
	actual, loaded := jobSchemas.LoadOrStore(d, result)
	if loaded {
		return actual.(jobSchemaResult).err
	}
	return err
}

// addJobColumns applies additive column migrations, treating an existing column
// as success so the migration is idempotent on a current database.
func addJobColumns(d *DB, statements ...string) error {
	for _, statement := range statements {
		if _, err := d.conn.Exec(statement); err != nil {
			if !strings.Contains(strings.ToLower(err.Error()), "duplicate column") {
				return err
			}
		}
	}
	return nil
}
