package db

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
)

// The schema fixture was exported using DB.New and EnsureTrackAnalysisSchema
// from v1.0.0-rc5 (f9f419ab795c918e71fa569957c5cb937ca3beff), not reconstructed
// from the current schema. All inserted rows below are synthetic.
func TestSpotifyMigrationFromRC5PreservesLibrary(t *testing.T) {
	schema, err := os.ReadFile("testdata/spotify_rc5_schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "library.db")
	old, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = old.Exec(string(schema)); err != nil {
		old.Close()
		t.Fatal(err)
	}
	_, err = old.Exec(`
 INSERT INTO songs(id,title,artist,album,file_path,added_at,bpm) VALUES('song','Released song','Artist','Album','song.mp3',1,128);
 INSERT INTO playlists(id,name,song_ids,created_at) VALUES('playlist','Released set','["song","song"]',1);
 INSERT INTO track_analysis(song_id,status,analysis_version,algorithm_version,source_fingerprint,bpm,bpm_source,energy_level) VALUES('song','complete',1,'released-v1','source-v1',128.375,'measured',7);
 INSERT INTO track_analysis_overrides(song_id,bpm,bpm_source_fingerprint,key_tonic,key_mode,bpm_locked,key_locked,updated_at) VALUES('song',129,'source-v1',0,'major',1,1,1);
 INSERT INTO track_analysis_source_revisions(song_id,source_fingerprint,updated_at) VALUES('song','source-v1',1);
 INSERT INTO dj_hot_cues(song_id,slot,position,label,origin,locked,source_fingerprint,created_at) VALUES('song',1,8.5,'Locked cue','user',1,'source-v1',1);
 INSERT INTO track_external_identity(song_id,provider,external_id,link_origin,source_fingerprint,confirmed_at) VALUES('song','spotify','TTTTTTTTTTTTTTTTTTTTTT','manual_confirmation','source-v1',1);
 INSERT INTO spotify_download_evidence(file_path,content_sha256,file_size,mtime_ns,spotify_id,completed_at,features_json) VALUES('song.mp3','digest',23,42,'TTTTTTTTTTTTTTTTTTTTTT',1,'{}');
 `)
	if err != nil {
		old.Close()
		t.Fatal(err)
	}
	if err = old.Close(); err != nil {
		t.Fatal(err)
	}
	for restart := 0; restart < 2; restart++ {
		d, err := New(path)
		if err != nil {
			t.Fatal(err)
		}
		if err = d.EnsureTrackAnalysisSchema(); err != nil {
			d.Close()
			t.Fatal(err)
		}
		checks := map[string]string{
			"legacy metadata":     `SELECT COUNT(*) FROM songs WHERE id='song' AND title='Released song' AND bpm=128`,
			"playlist duplicates": `SELECT COUNT(*) FROM playlists WHERE id='playlist' AND song_ids='["song","song"]'`,
			"local measurements":  `SELECT COUNT(*) FROM track_analysis WHERE song_id='song' AND bpm=128.375 AND bpm_source='measured' AND energy_level=7`,
			"manual locks":        `SELECT COUNT(*) FROM track_analysis_overrides WHERE song_id='song' AND bpm=129 AND bpm_locked=1 AND key_locked=1 AND key_tonic=0 AND key_mode='major' AND bpm_source_fingerprint='source-v1'`,
			"cue":                 `SELECT COUNT(*) FROM dj_hot_cues WHERE song_id='song' AND slot=1 AND position=8.5 AND label='Locked cue' AND locked=1`,
			"recording":           `SELECT COUNT(*) FROM track_external_identity WHERE song_id='song' AND link_origin='manual_confirmation' AND source_fingerprint='source-v1'`,
			"download evidence":   `SELECT COUNT(*) FROM spotify_download_evidence WHERE file_path='song.mp3' AND content_sha256='digest' AND file_size=23 AND mtime_ns=42`,
			"claim generation":    `SELECT COUNT(*) FROM pragma_table_info('track_analysis') WHERE name='claim_token'`,
			"score index":         `SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name='idx_spotify_audio_observations_score_context'`,
			"scalar imports":      `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='spotify_download_scalar_imports'`,
			"attempt imports":     `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='spotify_download_field_attempt_imports'`,
		}
		for name, query := range checks {
			var n int
			if err = d.conn.QueryRow(query).Scan(&n); err != nil || n != 1 {
				d.Close()
				t.Fatalf("restart %d %s: count=%d err=%v", restart, name, n, err)
			}
		}
		rows, err := d.conn.Query("PRAGMA foreign_key_check")
		if err != nil {
			d.Close()
			t.Fatal(err)
		}
		if rows.Next() {
			rows.Close()
			d.Close()
			t.Fatal("foreign key violation after migration")
		}
		if err = rows.Err(); err != nil {
			t.Fatal(err)
		}
		rows.Close()
		var enabled int
		if err = d.conn.QueryRow("PRAGMA foreign_keys").Scan(&enabled); err != nil || enabled != 1 {
			d.Close()
			t.Fatal("foreign keys disabled", err)
		}
		if err = d.Close(); err != nil {
			t.Fatal(err)
		}
	}
}
