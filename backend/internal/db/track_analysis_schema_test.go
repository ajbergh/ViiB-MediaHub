package db

import (
	"database/sql"
	"path/filepath"
	"testing"
)

func TestTrackAnalysisSchemaPreservesLegacySongBPM(t *testing.T) {
	database, err := New(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if _, err := database.conn.Exec(`INSERT INTO songs(id, title, artist, album, file_path, added_at, bpm) VALUES ('song', 'Song', 'Artist', 'Album', 'song.mp3', 1, 128)`); err != nil {
		t.Fatal(err)
	}
	if err := database.EnsureTrackAnalysisSchema(); err != nil {
		t.Fatal(err)
	}
	if _, err := database.conn.Exec(`INSERT INTO track_analysis(song_id, status, analysis_version, algorithm_version, source_fingerprint, bpm, bpm_source) VALUES ('song', 'complete', 1, 'tempo-v1', 'source-v1', 128.375, 'measured')`); err != nil {
		t.Fatal(err)
	}
	var legacyBPM int
	if err := database.conn.QueryRow(`SELECT bpm FROM songs WHERE id = 'song'`).Scan(&legacyBPM); err != nil {
		t.Fatal(err)
	}
	if legacyBPM != 128 {
		t.Fatalf("legacy songs.bpm = %d, want 128", legacyBPM)
	}
	var measured float64
	if err := database.conn.QueryRow(`SELECT bpm FROM track_analysis WHERE song_id = 'song'`).Scan(&measured); err != nil {
		t.Fatal(err)
	}
	if measured != 128.375 {
		t.Fatalf("measured BPM = %v, want 128.375", measured)
	}
}

func TestTrackAnalysisSchemaCascadesArtifactsAndOverrides(t *testing.T) {
	database, err := New(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := database.EnsureTrackAnalysisSchema(); err != nil {
		t.Fatal(err)
	}
	if _, err := database.conn.Exec(`INSERT INTO songs(id, title, artist, album, file_path, added_at) VALUES ('song', 'Song', 'Artist', 'Album', 'song.mp3', 1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := database.conn.Exec(`INSERT INTO track_analysis_artifacts(id, song_id, kind, format_version, algorithm_version, encoding, data, created_at) VALUES ('grid', 'song', 'beatgrid', 1, 'grid-v1', 'binary-v1', X'00', 1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := database.conn.Exec(`INSERT INTO track_analysis_overrides(song_id, bpm_locked, updated_at) VALUES ('song', 1, 1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := database.conn.Exec(`DELETE FROM songs WHERE id = 'song'`); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"track_analysis_artifacts", "track_analysis_overrides"} {
		var count int
		if err := database.conn.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&count); err != nil && err != sql.ErrNoRows {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("%s rows after song deletion = %d, want 0", table, count)
		}
	}
}
