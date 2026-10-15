package db

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/logger"
)

func isSQLiteBusy(err error) bool {
	var sqliteError interface{ Code() int }
	return errors.As(err, &sqliteError) && sqliteError.Code()&0xff == 5
}

// Retry only atomic statements or transactions that have rolled back. Never
// replay filesystem/provider work or a transaction whose commit succeeded.
// SQLite already waits five seconds per attempt; keep the extra waits bounded.
func retrySQLiteBusy(ctx context.Context, operation func() error) error {
	for attempt := 0; ; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		err := operation()
		if err == nil || !isSQLiteBusy(err) || attempt == 2 {
			return err
		}
		logger.Log("Database", "SQLite writer busy; retrying database operation (%d/3)", attempt+2)
		timer := time.NewTimer(time.Duration(attempt+1) * 100 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func (d *DB) execSQLiteWrite(ctx context.Context, query string, args ...any) (sql.Result, error) {
	var result sql.Result
	err := retrySQLiteBusy(ctx, func() error {
		var err error
		result, err = d.conn.ExecContext(ctx, query, args...)
		return err
	})
	return result, err
}

// Reserve the writer before any read snapshot is created, preventing deferred
// WAL transactions from failing when another writer commits between read/write.
func (d *DB) beginSemanticWriteTx(ctx context.Context) (*sql.Tx, error) {
	var tx *sql.Tx
	err := retrySQLiteBusy(ctx, func() error {
		var err error
		tx, err = d.conn.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `UPDATE semantic_index_state SET document_revision=document_revision WHERE 0`)
		if err != nil {
			_ = tx.Rollback()
		}
		return err
	})
	if err != nil {
		return nil, err
	}
	return tx, nil
}
