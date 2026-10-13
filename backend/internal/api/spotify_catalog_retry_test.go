package api

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"github.com/ajbergh/viib-mediahub/internal/db"
)

func catalogBusyFixture(t *testing.T) error {
	t.Helper()
	conn, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "busy.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err := conn.Exec(`CREATE TABLE writes (value INTEGER)`); err != nil {
		t.Fatal(err)
	}
	tx, err := conn.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`INSERT INTO writes VALUES (1)`); err != nil {
		t.Fatal(err)
	}
	_, busy := conn.Exec(`INSERT INTO writes VALUES (2)`)
	if !sqliteWriteBusy(busy) {
		t.Fatalf("expected actual SQLite write contention, got %v", busy)
	}
	return busy
}

func TestCatalogWriteRetriesBusyButNotRetirement(t *testing.T) {
	busy := catalogBusyFixture(t)
	attempts := 0
	err := retrySpotifyCatalogWrite(t.Context(), func() error {
		attempts++
		if attempts < 3 {
			return busy
		}
		return nil
	})
	if err != nil || attempts != 3 {
		t.Fatalf("retry did not recover: attempts=%d err=%v", attempts, err)
	}
	attempts = 0
	err = retrySpotifyCatalogWrite(t.Context(), func() error {
		attempts++
		return db.ErrSpotifyMetadataRuntimeSuperseded
	})
	if !errors.Is(err, db.ErrSpotifyMetadataRuntimeSuperseded) || attempts != 1 {
		t.Fatalf("retirement must not retry: attempts=%d err=%v", attempts, err)
	}
	attempts = 0
	err = retrySpotifyCatalogWrite(t.Context(), func() error { attempts++; return busy })
	if !errors.Is(err, busy) || attempts != 4 {
		t.Fatalf("retry must remain bounded: attempts=%d err=%v", attempts, err)
	}
}

func TestCatalogWriteRetryStopsOnCancellation(t *testing.T) {
	busy := catalogBusyFixture(t)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	attempts := 0
	err := retrySpotifyCatalogWrite(ctx, func() error {
		attempts++
		cancel()
		return busy
	})
	if !errors.Is(err, context.Canceled) || attempts != 1 {
		t.Fatalf("cancellation must stop retry: attempts=%d err=%v", attempts, err)
	}
}
