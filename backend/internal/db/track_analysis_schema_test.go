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

func TestTrackAnalysisRepositoryPersistsAndDetectsSourceChanges(t *testing.T) {
	database, err := New(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if _, err := database.conn.Exec(`INSERT INTO songs(id, title, artist, album, file_path, added_at) VALUES ('song', 'Song', 'Artist', 'Album', 'song.mp3', 1)`); err != nil {
		t.Fatal(err)
	}
	bpm, confidence, tonic := 128.375, 0.82, 9
	if err := database.UpsertTrackAnalysis(TrackAnalysis{
		SongID: "song", Status: TrackAnalysisComplete, AnalysisVersion: 1, AlgorithmVersion: "tempo-v1",
		SourceFingerprint: "source-v1", BPM: &bpm, BPMConfidence: &confidence, KeyTonic: &tonic,
	}); err != nil {
		t.Fatal(err)
	}
	loaded, err := database.GetTrackAnalysis("song")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.BPM == nil || *loaded.BPM != bpm || loaded.KeyTonic == nil || *loaded.KeyTonic != tonic || loaded.BPMConfidence == nil || *loaded.BPMConfidence != confidence {
		t.Fatalf("loaded analysis = %#v", loaded)
	}
	for fingerprint, want := range map[string]bool{"source-v1": false, "source-v2": true} {
		stale, err := database.TrackAnalysisStale("song", fingerprint)
		if err != nil || stale != want {
			t.Fatalf("stale(%q) = %t, %v; want %t", fingerprint, stale, err, want)
		}
	}
	missing, err := database.TrackAnalysisStale("missing", "source-v1")
	if err != nil || !missing {
		t.Fatalf("missing analysis stale = %t, %v; want true, nil", missing, err)
	}
}

func TestTrackAnalysisArtifactAndOverrideRepositories(t *testing.T) {
	database, err := New(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if _, err := database.conn.Exec(`INSERT INTO songs(id, title, artist, album, file_path, added_at) VALUES ('song', 'Song', 'Artist', 'Album', 'song.mp3', 1)`); err != nil {
		t.Fatal(err)
	}
	if err := database.UpsertTrackAnalysisArtifact(TrackAnalysisArtifact{ID: "grid-v1", SongID: "song", Kind: "beatgrid", FormatVersion: 1, AlgorithmVersion: "grid-v1", Encoding: "binary-v1", Data: []byte{1, 2, 3}}); err != nil {
		t.Fatal(err)
	}
	artifact, err := database.GetTrackAnalysisArtifact("song", "beatgrid", 1, "grid-v1")
	if err != nil || artifact.ID != "grid-v1" || string(artifact.Data) != string([]byte{1, 2, 3}) {
		t.Fatalf("artifact = %#v, %v", artifact, err)
	}
	bpm, tonic := 127.5, 2
	mode := "minor"
	if err := database.UpsertTrackAnalysisOverride(TrackAnalysisOverride{SongID: "song", BPM: &bpm, KeyTonic: &tonic, KeyMode: &mode, BeatgridArtifactID: &artifact.ID, BPMLocked: true, KeyLocked: true, BeatgridLocked: true}); err != nil {
		t.Fatal(err)
	}
	override, err := database.GetTrackAnalysisOverride("song")
	if err != nil || override.BPM == nil || *override.BPM != bpm || override.KeyTonic == nil || *override.KeyTonic != tonic || !override.BPMLocked || !override.KeyLocked || !override.BeatgridLocked {
		t.Fatalf("override = %#v, %v", override, err)
	}
}

func TestResolveEffectiveBPMUsesOnlyManualOrLocalAnalysis(t *testing.T) {
	measured := 128.25
	measuredSource := "measured"
	importedSource := "imported"
	manual := 127.5

	complete := &TrackAnalysis{Status: TrackAnalysisComplete, BPM: &measured, BPMSource: &measuredSource}
	partial := &TrackAnalysis{Status: TrackAnalysisPartial, BPM: &measured, BPMSource: &measuredSource}
	imported := &TrackAnalysis{Status: TrackAnalysisComplete, BPM: &measured, BPMSource: &importedSource}
	running := &TrackAnalysis{Status: TrackAnalysisRunning, BPM: &measured, BPMSource: &measuredSource}
	failed := &TrackAnalysis{Status: TrackAnalysisFailed}

	cases := []struct {
		name   string
		inputs EffectiveBPMInputs
		want   string
		sync   bool
		value  *float64
	}{
		{"unknown", EffectiveBPMInputs{}, EffectiveBPMUnknown, false, nil},
		{"measured analysis", EffectiveBPMInputs{Analysis: complete}, EffectiveBPMMeasured, true, &measured},
		{"partial tempo is still measured", EffectiveBPMInputs{Analysis: partial}, EffectiveBPMMeasured, true, &measured},
		{"imported tag is rejected", EffectiveBPMInputs{Analysis: imported}, EffectiveBPMUnknown, false, nil},
		{"running row is not trusted", EffectiveBPMInputs{Analysis: running}, EffectiveBPMUnknown, false, nil},
		{"failed row is unknown", EffectiveBPMInputs{Analysis: failed}, EffectiveBPMUnknown, false, nil},
		{"manual lock wins", EffectiveBPMInputs{Override: &TrackAnalysisOverride{BPM: &manual, BPMLocked: true}, Analysis: complete}, EffectiveBPMManual, true, &manual},
		{"unlocked override does not win", EffectiveBPMInputs{Override: &TrackAnalysisOverride{BPM: &manual}, Analysis: complete}, EffectiveBPMMeasured, true, &measured},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			result := ResolveEffectiveBPM(test.inputs)
			if result.Source != test.want || result.SyncAllowed != test.sync {
				t.Fatalf("result = %#v, want source %q sync %v", result, test.want, test.sync)
			}
			if test.value != nil && (result.Value == nil || *result.Value != *test.value) {
				t.Fatalf("result.Value = %v, want %v", result.Value, *test.value)
			}
			if test.value == nil && result.Value != nil {
				t.Fatalf("result.Value = %v, want nil", *result.Value)
			}
		})
	}
}

func TestResolveEffectiveKeyPrefersLockedManualThenMeasured(t *testing.T) {
	manualTonic, measuredTonic := 9, 0
	manualMode, measuredMode := "minor", "major"
	measuredSource, importedSource, inferredSource := "measured", "imported", "inferred"
	complete := &TrackAnalysis{Status: TrackAnalysisComplete, KeyTonic: &measuredTonic, KeyMode: &measuredMode, KeySource: &measuredSource}
	partial := &TrackAnalysis{Status: TrackAnalysisPartial, KeyTonic: &measuredTonic, KeyMode: &measuredMode, KeySource: &measuredSource}

	tests := []struct {
		name   string
		inputs EffectiveKeyInputs
		source string
		tonic  *int
		mode   *string
	}{
		{"locked manual wins", EffectiveKeyInputs{Override: &TrackAnalysisOverride{KeyTonic: &manualTonic, KeyMode: &manualMode, KeyLocked: true}, Analysis: complete}, EffectiveKeyManual, &manualTonic, &manualMode},
		{"unlocked override does not win", EffectiveKeyInputs{Override: &TrackAnalysisOverride{KeyTonic: &manualTonic, KeyMode: &manualMode}, Analysis: complete}, EffectiveKeyMeasured, &measuredTonic, &measuredMode},
		{"partial measured key is usable", EffectiveKeyInputs{Analysis: partial}, EffectiveKeyMeasured, &measuredTonic, &measuredMode},
		{"imported key is rejected", EffectiveKeyInputs{Analysis: &TrackAnalysis{Status: TrackAnalysisComplete, KeyTonic: &measuredTonic, KeyMode: &measuredMode, KeySource: &importedSource}}, EffectiveKeyUnknown, nil, nil},
		{"untrusted source is unknown", EffectiveKeyInputs{Analysis: &TrackAnalysis{Status: TrackAnalysisComplete, KeyTonic: &measuredTonic, KeyMode: &measuredMode, KeySource: &inferredSource}}, EffectiveKeyUnknown, nil, nil},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := ResolveEffectiveKey(test.inputs)
			if got.Source != test.source || got.Tonic != test.tonic || got.Mode != test.mode {
				t.Fatalf("ResolveEffectiveKey() = %#v, want source=%q tonic=%#v mode=%#v", got, test.source, test.tonic, test.mode)
			}
		})
	}
}
