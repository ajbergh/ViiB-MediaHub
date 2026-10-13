package db

import (
	"database/sql"
	"path/filepath"
	"testing"
)

func TestAnalysisSchemaRetriesAfterInitializationFailure(t *testing.T) {
	d, err := New(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	trackAnalysisSchemas.Delete(d)
	original := d.conn
	closed, err := sql.Open(sqliteRuntimeDriverName, sqliteRuntimeDSN(":memory:"))
	if err != nil {
		t.Fatal(err)
	}
	if err := closed.Close(); err != nil {
		t.Fatal(err)
	}
	d.conn = closed
	first := d.EnsureTrackAnalysisSchema()
	d.conn = original
	if first == nil {
		t.Fatal("fixture did not fail initialization")
	}
	if err := d.EnsureTrackAnalysisSchema(); err != nil {
		t.Fatalf("initial failure was cached permanently: %v", err)
	}
	if _, ok := trackAnalysisSchemas.Load(d); !ok {
		t.Fatal("successful initialization was not cached")
	}
}
