package db

import (
	"database/sql"
	"encoding/json"
	"time"
)

func (d *DB) CreateJob(job Job) error {
	if err := d.EnsureJobSchema(); err != nil { return err }
	now := time.Now().UnixMilli()
	if job.Status == "" { job.Status = JobStatusQueued }
	job.CreatedAt = now
	job.UpdatedAt = now
	_, err := d.conn.Exec(`
		INSERT INTO operation_jobs(
			id, type, status, progress_current, progress_total, message,
			parameters, result, error_code, error_message, attempts,
			created_at, started_at, completed_at, updated_at
		) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULL, NULL, ?)
	`, job.ID, job.Type, job.Status, job.ProgressCurrent, job.ProgressTotal,
		job.Message, nullableJSON(job.Parameters), nullableJSON(job.Result),
		job.ErrorCode, job.ErrorMessage, job.Attempts, job.CreatedAt, job.UpdatedAt)
	return err
}

func nullableJSON(value json.RawMessage) any {
	if len(value) == 0 { return nil }
	return string(value)
}

func (d *DB) StartJob(id, message string) error {
	now := time.Now().UnixMilli()
	result, err := d.conn.Exec(`
		UPDATE operation_jobs
		SET status = ?, message = ?, attempts = attempts + 1,
		    started_at = ?, completed_at = NULL, error_code = NULL,
		    error_message = NULL, updated_at = ?
		WHERE id = ? AND status IN (?, ?, ?)
	`, JobStatusRunning, message, now, now, id,
		JobStatusQueued, JobStatusFailed, JobStatusInterrupted)
	if err != nil { return err }
	rows, err := result.RowsAffected()
	if err != nil { return err }
	if rows == 0 { return sql.ErrNoRows }
	return nil
}

func (d *DB) UpdateJobProgress(id string, current, total int64, message string) error {
	_, err := d.conn.Exec(`
		UPDATE operation_jobs SET progress_current = ?, progress_total = ?,
		message = ?, updated_at = ? WHERE id = ? AND status = ?
	`, current, total, message, time.Now().UnixMilli(), id, JobStatusRunning)
	return err
}

func (d *DB) CompleteJob(id string, result any, message string) error {
	payload, err := json.Marshal(result)
	if err != nil { return err }
	now := time.Now().UnixMilli()
	_, err = d.conn.Exec(`
		UPDATE operation_jobs SET status = ?, result = ?, message = ?,
		completed_at = ?, updated_at = ? WHERE id = ?
	`, JobStatusSucceeded, string(payload), message, now, now, id)
	return err
}

func (d *DB) FailJob(id, code, message string) error {
	now := time.Now().UnixMilli()
	_, err := d.conn.Exec(`
		UPDATE operation_jobs SET status = ?, error_code = ?, error_message = ?,
		message = ?, completed_at = ?, updated_at = ? WHERE id = ?
	`, JobStatusFailed, code, message, message, now, now, id)
	return err
}

func (d *DB) RequestJobCancellation(id string) (bool, error) {
	result, err := d.conn.Exec(`
		UPDATE operation_jobs SET status = ?, message = 'Cancellation requested', updated_at = ?
		WHERE id = ? AND status IN (?, ?)
	`, JobStatusCanceling, time.Now().UnixMilli(), id, JobStatusQueued, JobStatusRunning)
	if err != nil { return false, err }
	rows, err := result.RowsAffected()
	return rows > 0, err
}

func (d *DB) CancelJob(id, message string) error {
	now := time.Now().UnixMilli()
	_, err := d.conn.Exec(`
		UPDATE operation_jobs SET status = ?, message = ?, completed_at = ?, updated_at = ? WHERE id = ?
	`, JobStatusCanceled, message, now, now, id)
	return err
}

func (d *DB) GetJob(id string) (Job, error) {
	if err := d.EnsureJobSchema(); err != nil { return Job{}, err }
	row := d.conn.QueryRow(`
		SELECT id, type, status, progress_current, progress_total, message,
		       parameters, result, error_code, error_message, attempts,
		       created_at, started_at, completed_at, updated_at
		FROM operation_jobs WHERE id = ?`, id)
	return scanJob(row)
}

type jobScanner interface { Scan(dest ...any) error }

func scanJob(row jobScanner) (Job, error) {
	var job Job
	var message, parameters, result, errorCode, errorMessage sql.NullString
	var startedAt, completedAt sql.NullInt64
	err := row.Scan(&job.ID, &job.Type, &job.Status, &job.ProgressCurrent,
		&job.ProgressTotal, &message, &parameters, &result, &errorCode,
		&errorMessage, &job.Attempts, &job.CreatedAt, &startedAt,
		&completedAt, &job.UpdatedAt)
	if err != nil { return Job{}, err }
	if message.Valid { job.Message = message.String }
	if parameters.Valid { job.Parameters = json.RawMessage(parameters.String) }
	if result.Valid { job.Result = json.RawMessage(result.String) }
	if errorCode.Valid { job.ErrorCode = errorCode.String }
	if errorMessage.Valid { job.ErrorMessage = errorMessage.String }
	if startedAt.Valid { job.StartedAt = startedAt.Int64 }
	if completedAt.Valid { job.CompletedAt = completedAt.Int64 }
	return job, nil
}

func (d *DB) ListJobs(limit int, status string) ([]Job, error) {
	if err := d.EnsureJobSchema(); err != nil { return nil, err }
	if limit <= 0 || limit > 500 { limit = 100 }
	query := `SELECT id, type, status, progress_current, progress_total, message,
		parameters, result, error_code, error_message, attempts,
		created_at, started_at, completed_at, updated_at FROM operation_jobs`
	args := []any{}
	if status != "" { query += ` WHERE status = ?`; args = append(args, status) }
	query += ` ORDER BY created_at DESC LIMIT ?`; args = append(args, limit)
	rows, err := d.conn.Query(query, args...)
	if err != nil { return nil, err }
	defer rows.Close()
	jobs := make([]Job, 0)
	for rows.Next() {
		job, err := scanJob(rows)
		if err != nil { return nil, err }
		jobs = append(jobs, job)
	}
	return jobs, rows.Err()
}
