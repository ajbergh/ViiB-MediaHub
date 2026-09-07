// track_analysis_schema.go installs durable, song-keyed analysis storage.
package db

import "sync"

type trackAnalysisSchemaResult struct{ err error }

var trackAnalysisSchemas sync.Map // map[*DB]trackAnalysisSchemaResult

// EnsureTrackAnalysisSchema installs the additive analysis result, artifact,
// and manual-override tables. It intentionally does not alter songs.bpm: that
// integer column remains legacy inferred metadata, never measured tempo.
func (d *DB) EnsureTrackAnalysisSchema() error {
	if value, ok := trackAnalysisSchemas.Load(d); ok {
		return value.(trackAnalysisSchemaResult).err
	}
	_, err := d.conn.Exec(`
		CREATE TABLE IF NOT EXISTS track_analysis (
			song_id TEXT PRIMARY KEY REFERENCES songs(id) ON DELETE CASCADE,
			status TEXT NOT NULL CHECK(status IN ('pending', 'running', 'complete', 'partial', 'failed', 'unsupported')),
			analysis_version INTEGER NOT NULL,
			algorithm_version TEXT NOT NULL,
			decoder_id TEXT,
			source_fingerprint TEXT NOT NULL,
			source_size INTEGER,
			source_mtime INTEGER,
			source_revision TEXT,
			bpm REAL,
			bpm_confidence REAL,
			bpm_alt_candidate REAL,
			tempo_stability REAL,
			tempo_kind TEXT CHECK(tempo_kind IS NULL OR tempo_kind IN ('unknown', 'static', 'dynamic-candidate', 'dynamic')),
			bpm_source TEXT CHECK(bpm_source IS NULL OR bpm_source IN ('measured', 'imported', 'manual', 'legacy-ai')),
			key_tonic INTEGER CHECK(key_tonic IS NULL OR key_tonic BETWEEN 0 AND 11),
			key_mode TEXT CHECK(key_mode IS NULL OR key_mode IN ('major', 'minor')),
			key_confidence REAL,
			key_source TEXT CHECK(key_source IS NULL OR key_source IN ('measured', 'imported', 'manual')),
			camelot_key TEXT,
			open_key TEXT,
			analyzed_at INTEGER,
			error_code TEXT,
			error_message TEXT
		);
		CREATE INDEX IF NOT EXISTS idx_track_analysis_status ON track_analysis(status);
		CREATE INDEX IF NOT EXISTS idx_track_analysis_fingerprint ON track_analysis(source_fingerprint);

		CREATE TABLE IF NOT EXISTS track_analysis_artifacts (
			id TEXT PRIMARY KEY,
			song_id TEXT NOT NULL REFERENCES songs(id) ON DELETE CASCADE,
			kind TEXT NOT NULL,
			format_version INTEGER NOT NULL,
			algorithm_version TEXT NOT NULL,
			encoding TEXT NOT NULL,
			data BLOB NOT NULL,
			created_at INTEGER NOT NULL,
			UNIQUE(song_id, kind, format_version, algorithm_version)
		);
		CREATE INDEX IF NOT EXISTS idx_track_analysis_artifacts_song ON track_analysis_artifacts(song_id);

		CREATE TABLE IF NOT EXISTS track_analysis_overrides (
			song_id TEXT PRIMARY KEY REFERENCES songs(id) ON DELETE CASCADE,
			bpm REAL,
			key_tonic INTEGER CHECK(key_tonic IS NULL OR key_tonic BETWEEN 0 AND 11),
			key_mode TEXT CHECK(key_mode IS NULL OR key_mode IN ('major', 'minor')),
			beatgrid_artifact_id TEXT,
			bpm_locked INTEGER NOT NULL DEFAULT 0 CHECK(bpm_locked IN (0, 1)),
			key_locked INTEGER NOT NULL DEFAULT 0 CHECK(key_locked IN (0, 1)),
			beatgrid_locked INTEGER NOT NULL DEFAULT 0 CHECK(beatgrid_locked IN (0, 1)),
			updated_at INTEGER NOT NULL
		);
	`)
	result := trackAnalysisSchemaResult{err: err}
	actual, loaded := trackAnalysisSchemas.LoadOrStore(d, result)
	if loaded {
		return actual.(trackAnalysisSchemaResult).err
	}
	return err
}
