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
			energy_level INTEGER CHECK(energy_level IS NULL OR energy_level BETWEEN 1 AND 10),
			energy_level_confidence REAL CHECK(energy_level_confidence IS NULL OR energy_level_confidence BETWEEN 0 AND 1),
			energy_algorithm_version TEXT,
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
			provenance TEXT NOT NULL DEFAULT 'unknown' CHECK(provenance IN ('measured', 'inferred-from-meter', 'manual', 'unknown')),
			source_fingerprint TEXT NOT NULL DEFAULT '',
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
	if err == nil {
		err = ensureTrackAnalysisEnergyColumns(d)
	}
	if err == nil {
		err = ensureTrackAnalysisArtifactProvenanceColumn(d)
	}
	if err == nil {
		err = ensureTrackAnalysisArtifactSourceFingerprintColumn(d)
	}
	result := trackAnalysisSchemaResult{err: err}
	actual, loaded := trackAnalysisSchemas.LoadOrStore(d, result)
	if loaded {
		return actual.(trackAnalysisSchemaResult).err
	}
	return err
}

// ensureTrackAnalysisArtifactSourceFingerprintColumn upgrades legacy opaque
// artifacts conservatively: old rows remain unbound to a source and therefore
// cannot be presented as current evidence.
func ensureTrackAnalysisArtifactSourceFingerprintColumn(d *DB) error {
	rows, err := d.conn.Query(`PRAGMA table_info(track_analysis_artifacts)`)
	if err != nil {
		return err
	}
	hasColumn := false
	for rows.Next() {
		var cid, notnull, pk int
		var name, typ string
		var defaultValue any
		if err := rows.Scan(&cid, &name, &typ, &notnull, &defaultValue, &pk); err != nil {
			rows.Close()
			return err
		}
		if name == "source_fingerprint" {
			hasColumn = true
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if hasColumn {
		return nil
	}
	_, err = d.conn.Exec(`ALTER TABLE track_analysis_artifacts ADD COLUMN source_fingerprint TEXT NOT NULL DEFAULT ''`)
	return err
}

// ensureTrackAnalysisArtifactProvenanceColumn upgrades pre-provenance installs.
// Existing generated beat grids used phase-derived beats with assumed 4/4 bar
// starts, so they are inferred-from-meter. Locked editor artifacts are manual;
// unlocked overrides remain unknown because their ownership was ambiguous.
func ensureTrackAnalysisArtifactProvenanceColumn(d *DB) error {
	rows, err := d.conn.Query(`PRAGMA table_info(track_analysis_artifacts)`)
	if err != nil {
		return err
	}
	hasColumn := false
	for rows.Next() {
		var cid, notnull, pk int
		var name, typ string
		var defaultValue any
		if err := rows.Scan(&cid, &name, &typ, &notnull, &defaultValue, &pk); err != nil {
			rows.Close()
			return err
		}
		if name == "provenance" {
			hasColumn = true
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if hasColumn {
		return nil
	}
	if _, err := d.conn.Exec(`ALTER TABLE track_analysis_artifacts ADD COLUMN provenance TEXT NOT NULL DEFAULT 'unknown' CHECK(provenance IN ('measured', 'inferred-from-meter', 'manual', 'unknown'))`); err != nil {
		return err
	}
	_, err = d.conn.Exec(`UPDATE track_analysis_artifacts
		SET provenance = CASE
			WHEN EXISTS (SELECT 1 FROM track_analysis_overrides o WHERE o.song_id = track_analysis_artifacts.song_id AND o.beatgrid_artifact_id = track_analysis_artifacts.id AND o.beatgrid_locked = 1) THEN 'manual'
			WHEN EXISTS (SELECT 1 FROM track_analysis_overrides o WHERE o.song_id = track_analysis_artifacts.song_id AND o.beatgrid_artifact_id = track_analysis_artifacts.id) THEN 'unknown'
			ELSE 'inferred-from-meter'
		END
		WHERE kind = 'beatgrid'`)
	return err
}

// ensureTrackAnalysisEnergyColumns upgrades installations created before the
// Energy Level fields existed. SQLite ALTER TABLE ADD COLUMN is additive and
// safe to rerun when guarded by PRAGMA table_info.
func ensureTrackAnalysisEnergyColumns(d *DB) error {
	rows, err := d.conn.Query(`PRAGMA table_info(track_analysis)`)
	if err != nil {
		return err
	}
	columns := map[string]bool{}
	for rows.Next() {
		var cid, notnull, pk int
		var name, typ string
		var defaultValue any
		if err := rows.Scan(&cid, &name, &typ, &notnull, &defaultValue, &pk); err != nil {
			rows.Close()
			return err
		}
		columns[name] = true
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, column := range []struct{ name, definition string }{
		{"energy_level", "INTEGER CHECK(energy_level IS NULL OR energy_level BETWEEN 1 AND 10)"},
		{"energy_level_confidence", "REAL CHECK(energy_level_confidence IS NULL OR energy_level_confidence BETWEEN 0 AND 1)"},
		{"energy_algorithm_version", "TEXT"},
	} {
		if columns[column.name] {
			continue
		}
		if _, err := d.conn.Exec(`ALTER TABLE track_analysis ADD COLUMN ` + column.name + ` ` + column.definition); err != nil {
			return err
		}
	}
	return nil
}
