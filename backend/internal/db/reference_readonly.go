package db

import (
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// OpenReferenceReadOnly opens an existing library in a consistent read-only
// transaction. It deliberately skips migrations, encryption and WAL policy.
// Only already-installed reference schemas are accepted. Close releases it.
func OpenReferenceReadOnly(path string) (*DB, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("reference database must be an existing regular file")
	}
	// Use the plain driver: the runtime driver applies write-oriented pragmas.
	uriPath := filepath.ToSlash(absolute)
	if !strings.HasPrefix(uriPath, "/") {
		uriPath = "/" + uriPath
	}
	uri := &url.URL{Scheme: "file", Path: uriPath}
	query := url.Values{"mode": {"ro"}, "_pragma": {"query_only(1)", "busy_timeout(5000)"}}
	uri.RawQuery = query.Encode()
	conn, err := sql.Open("sqlite", uri.String())
	if err != nil {
		return nil, err
	}
	conn.SetMaxOpenConns(1)
	conn.SetMaxIdleConns(1)
	if _, err = conn.Exec("BEGIN"); err != nil {
		conn.Close()
		return nil, err
	}
	var count int
	err = conn.QueryRow(`SELECT count(*) FROM sqlite_master WHERE type='table' AND name IN ('songs','track_external_identity','external_track_analysis','external_track_analysis_status')`).Scan(&count)
	if err != nil || count != 4 {
		conn.Close()
		return nil, fmt.Errorf("reference database schemas are unavailable")
	}
	return &DB{conn: conn, externalSchemaReady: true}, nil
}
