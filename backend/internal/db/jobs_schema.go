package db

import (
	"encoding/json"
	"sync"
	"time"
)

const (
	JobStatusQueued      = "queued"
	JobStatusRunning     = "running"
	JobStatusSucceeded   = "succeeded"
	JobStatusFailed      = "failed"
	JobStatusCanceling   = "canceling"
	JobStatusCanceled    = "canceled"
	JobStatusInterrupted = "interrupted"
)

type Job struct {
	ID              string          `json:"id"`
	Type            string          `json:"type"`
	Status          string          `json:"status"`
	ProgressCurrent int64           `json:"progressCurrent"`
	ProgressTotal   int64           `json:"progressTotal"`
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

type jobSchemaResult struct { err error }
var jobSchemas sync.Map // map[*DB]jobSchemaResult

// EnsureJobSchema installs and performs restart recovery exactly once for each
// live DB handle. A successful initialization is stored as a non-nil result
// object so sync.Map never receives a nil value.
func (d *DB) EnsureJobSchema() error {
	if value, ok := jobSchemas.Load(d); ok { return value.(jobSchemaResult).err }

	_, err := d.conn.Exec(`
		CREATE TABLE IF NOT EXISTS operation_jobs (
			id TEXT PRIMARY KEY,
			type TEXT NOT NULL,
			status TEXT NOT NULL,
			progress_current INTEGER NOT NULL DEFAULT 0,
			progress_total INTEGER NOT NULL DEFAULT 0,
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
	if err == nil {
		now := time.Now().UnixMilli()
		_, err = d.conn.Exec(`
			UPDATE operation_jobs
			SET status = ?, error_code = 'process_restarted',
			    error_message = 'The application restarted while the job was active',
			    completed_at = ?, updated_at = ?
			WHERE status IN (?, ?, ?)
		`, JobStatusInterrupted, now, now, JobStatusQueued, JobStatusRunning, JobStatusCanceling)
	}
	result := jobSchemaResult{err: err}
	actual, loaded := jobSchemas.LoadOrStore(d, result)
	if loaded { return actual.(jobSchemaResult).err }
	return err
}
