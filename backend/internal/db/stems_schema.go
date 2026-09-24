package db

import "sync"

type stemSchemaResult struct{ err error }

var stemSchemas sync.Map // map[*DB]stemSchemaResult

// EnsureStemSchema installs the durable package registry without changing the
// legacy songs or analysis schemas.
func (d *DB) EnsureStemSchema() error {
	if v, ok := stemSchemas.Load(d); ok {
		return v.(stemSchemaResult).err
	}
	_, err := d.conn.Exec(`
		CREATE TABLE IF NOT EXISTS track_stem_sets (
			id TEXT PRIMARY KEY,
			song_id TEXT NOT NULL REFERENCES songs(id) ON DELETE CASCADE,
			source_audio_hash TEXT NOT NULL DEFAULT '',
			audio_sha256 TEXT NOT NULL DEFAULT '',
			model_name TEXT NOT NULL DEFAULT '', model_version TEXT NOT NULL DEFAULT '',
			generator_name TEXT NOT NULL DEFAULT '', generator_version TEXT NOT NULL DEFAULT '',
			stem_layout TEXT NOT NULL DEFAULT '', status TEXT NOT NULL CHECK(status IN ('discovered','validating','ready','stale','invalid','unavailable')),
			sample_rate INTEGER NOT NULL DEFAULT 0, channels INTEGER NOT NULL DEFAULT 0,
			frames INTEGER NOT NULL DEFAULT 0, duration_seconds REAL NOT NULL DEFAULT 0,
			decoder_delay_frames INTEGER NOT NULL DEFAULT 0, start_trim_frames INTEGER NOT NULL DEFAULT 0,
			package_path TEXT NOT NULL, discovery_source TEXT NOT NULL DEFAULT '',
			manifest_schema_version INTEGER NOT NULL DEFAULT 0, generated_at TEXT NOT NULL DEFAULT '',
			validation_error_code TEXT NOT NULL DEFAULT '', validation_error_message TEXT NOT NULL DEFAULT '',
			manually_invalidated INTEGER NOT NULL DEFAULT 0 CHECK(manually_invalidated IN (0,1)),
			explicitly_linked INTEGER NOT NULL DEFAULT 0 CHECK(explicitly_linked IN (0,1)),
			updated_at INTEGER NOT NULL,
			UNIQUE(song_id, package_path)
		);
		CREATE INDEX IF NOT EXISTS idx_track_stem_sets_song_status ON track_stem_sets(song_id,status);
		CREATE TABLE IF NOT EXISTS track_stems (
			stem_set_id TEXT NOT NULL REFERENCES track_stem_sets(id) ON DELETE CASCADE,
			name TEXT NOT NULL, relative_path TEXT NOT NULL, sha256 TEXT NOT NULL, size_bytes INTEGER NOT NULL,
			sample_rate INTEGER NOT NULL, channels INTEGER NOT NULL, frames INTEGER NOT NULL, encoding TEXT NOT NULL,
			PRIMARY KEY(stem_set_id,name)
		);
		CREATE TABLE IF NOT EXISTS stem_locations (
			id TEXT PRIMARY KEY, path TEXT NOT NULL UNIQUE, enabled INTEGER NOT NULL DEFAULT 1 CHECK(enabled IN (0,1)), created_at INTEGER NOT NULL
		);
	`)
	actual, loaded := stemSchemas.LoadOrStore(d, stemSchemaResult{err})
	if loaded {
		return actual.(stemSchemaResult).err
	}
	return err
}
