package db

import (
	"database/sql"
	"net/url"
	"path/filepath"
	"strings"
)

// CreateSpotifyCredentialFreeCopy preserves library/settings data while omitting
// Spotify login material from an exported backup. Only the new copy is edited.
// VACUUM removes deleted credential bytes from free pages before hashing/export.
func (d *DB) CreateSpotifyCredentialFreeCopy(destination string) error {
	if err := d.CreateConsistentCopy(destination); err != nil {
		return err
	}
	absolute, err := filepath.Abs(destination)
	if err != nil {
		return err
	}
	path := filepath.ToSlash(absolute)
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	uri := &url.URL{Scheme: "file", Path: path, RawQuery: "mode=rw"}
	conn, err := sql.Open("sqlite", uri.String())
	if err != nil {
		return err
	}
	defer conn.Close()
	conn.SetMaxOpenConns(1)
	if _, err := conn.Exec(`PRAGMA secure_delete=ON`); err != nil {
		return err
	}
	if _, err := conn.Exec(`DELETE FROM settings WHERE key IN ('spotify_webplayer_session','spotify_credentials')`); err != nil {
		return err
	}
	_, err = conn.Exec(`VACUUM`)
	return err
}
