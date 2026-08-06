// backup_primitives.go contains the SQLite-copy and integrity primitives used
// by local backup creation and offline restore validation.
package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// DatabasePath returns SQLite's configured path for the main database.
func (d *DB) DatabasePath() (string, error) {
	rows, err := d.conn.Query(`PRAGMA database_list`)
	if err != nil { return "", err }
	defer rows.Close()
	for rows.Next() {
		var sequence int
		var name, path string
		if err := rows.Scan(&sequence, &name, &path); err != nil { return "", err }
		if name == "main" { return path, nil }
	}
	return "", fmt.Errorf("main SQLite database path was not found")
}

// CreateConsistentCopy uses SQLite VACUUM INTO so the archive never copies a
// database file independently from outstanding WAL frames.
func (d *DB) CreateConsistentCopy(destination string) error {
	if err := d.CheckpointWAL(); err != nil { return err }
	if err := os.MkdirAll(filepath.Dir(destination), 0700); err != nil { return err }
	_ = os.Remove(destination)
	if _, err := d.conn.Exec(`VACUUM INTO ?`, destination); err == nil { return nil }
	escaped := strings.ReplaceAll(destination, "'", "''")
	_, err := d.conn.Exec(`VACUUM INTO '` + escaped + `'`)
	return err
}

// ValidateSQLiteCopy opens a database copy read-only and requires integrity_check to return ok.
func ValidateSQLiteCopy(path string) error {
	conn, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path)+"?mode=ro&_pragma=foreign_keys(1)")
	if err != nil { return err }
	defer conn.Close()
	var integrity string
	if err := conn.QueryRow(`PRAGMA integrity_check`).Scan(&integrity); err != nil { return err }
	if integrity != "ok" { return fmt.Errorf("SQLite integrity check failed: %s", integrity) }
	return nil
}
