package db

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"
)

const testAlgorithmVersion = "track-v1;tempo-test;key-test"

func selectionDatabase(t *testing.T) *DB {
	t.Helper()
	database, err := New(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	return database
}

func saveSelectionSong(t *testing.T, database *DB, id string, addedAt int64) {
	t.Helper()
	if err := database.SaveSong(&Song{ID: id, Title: id, Artist: "Artist", Album: "Album", FilePath: filepath.Join(t.TempDir(), id+".wav"), AddedAt: addedAt}); err != nil {
		t.Fatal(err)
	}
}

func TestParseAnalysisSelectionDefaultsToMissing(t *testing.T) {
	selection, err := ParseAnalysisSelection(nil)
	if err != nil {
		t.Fatal(err)
	}
	if selection.Mode != AnalysisSelectionMissing {
		t.Fatalf("mode = %q, want %q", selection.Mode, AnalysisSelectionMissing)
	}
}

func TestParseAnalysisSelectionRejectsIncompleteSelections(t *testing.T) {
	for _, payload := range []string{
		`{"mode":"ids"}`,
		`{"mode":"playlist"}`,
		`{"mode":"everything"}`,
		`{"mode":"ids","songIds":[]}`,
	} {
		if _, err := ParseAnalysisSelection(json.RawMessage(payload)); err == nil {
			t.Errorf("ParseAnalysisSelection(%s) = nil error, want rejection", payload)
		}
	}
}

func TestParseAnalysisSelectionRoundTripsThroughJobParameters(t *testing.T) {
	original := AnalysisSelection{Mode: AnalysisSelectionIDs, SongIDs: []string{"a", "b"}}
	encoded, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	// A resumed job must re-expand the identical selection from the persisted
	// parameters, so the encoding has to survive a round trip verbatim.
	restored, err := ParseAnalysisSelection(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if restored.Mode != original.Mode || len(restored.SongIDs) != 2 || restored.SongIDs[0] != "a" || restored.SongIDs[1] != "b" {
		t.Fatalf("restored = %#v, want %#v", restored, original)
	}
}

func TestExpandAnalysisSelectionModes(t *testing.T) {
	database := selectionDatabase(t)
	saveSelectionSong(t, database, "first", 1)
	saveSelectionSong(t, database, "second", 2)
	saveSelectionSong(t, database, "third", 3)

	// "second" already has a current-version result; "third" has an outdated one.
	if err := database.UpsertTrackAnalysis(TrackAnalysis{
		SongID: "second", Status: TrackAnalysisComplete, AnalysisVersion: 1,
		AlgorithmVersion: testAlgorithmVersion, SourceFingerprint: "fp-second",
	}); err != nil {
		t.Fatal(err)
	}
	if err := database.UpsertTrackAnalysis(TrackAnalysis{
		SongID: "third", Status: TrackAnalysisComplete, AnalysisVersion: 1,
		AlgorithmVersion: "track-v0;old", SourceFingerprint: "fp-third",
	}); err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct {
		name      string
		selection AnalysisSelection
		want      []string
	}{
		{name: "all", selection: AnalysisSelection{Mode: AnalysisSelectionAll}, want: []string{"first", "second", "third"}},
		{name: "missing", selection: AnalysisSelection{Mode: AnalysisSelectionMissing}, want: []string{"first"}},
		{name: "stale", selection: AnalysisSelection{Mode: AnalysisSelectionStale}, want: []string{"first", "third"}},
		{name: "ids", selection: AnalysisSelection{Mode: AnalysisSelectionIDs, SongIDs: []string{"third", "first"}}, want: []string{"first", "third"}},
	} {
		got, err := database.ExpandAnalysisSelection(test.selection, 1, testAlgorithmVersion)
		if err != nil {
			t.Fatalf("%s: %v", test.name, err)
		}
		if !equalStrings(got, test.want) {
			t.Errorf("%s: got %v, want %v", test.name, got, test.want)
		}
	}
}

func TestExpandAnalysisSelectionIDsDeduplicatesAndIgnoresUnknown(t *testing.T) {
	database := selectionDatabase(t)
	saveSelectionSong(t, database, "known", 1)
	got, err := database.ExpandAnalysisSelection(AnalysisSelection{
		Mode:    AnalysisSelectionIDs,
		SongIDs: []string{"known", "known", "  known  ", "", "absent"},
	}, 1, testAlgorithmVersion)
	if err != nil {
		t.Fatal(err)
	}
	if !equalStrings(got, []string{"known"}) {
		t.Fatalf("got %v, want [known]", got)
	}
}

func TestExpandAnalysisSelectionPlaylist(t *testing.T) {
	database := selectionDatabase(t)
	saveSelectionSong(t, database, "in", 1)
	saveSelectionSong(t, database, "out", 2)
	if err := database.SavePlaylist(&Playlist{ID: "list", Name: "List", SongIDs: []string{"in"}, CreatedAt: 1}); err != nil {
		t.Fatal(err)
	}
	got, err := database.ExpandAnalysisSelection(AnalysisSelection{Mode: AnalysisSelectionPlaylist, PlaylistID: "list"}, 1, testAlgorithmVersion)
	if err != nil {
		t.Fatal(err)
	}
	if !equalStrings(got, []string{"in"}) {
		t.Fatalf("got %v, want [in]", got)
	}
}

// Plex-backed songs need an authenticated source adapter that does not exist
// yet. Expanding them would only queue guaranteed failures.
func TestExpandAnalysisSelectionExcludesPlexTracks(t *testing.T) {
	database := selectionDatabase(t)
	saveSelectionSong(t, database, "local", 1)
	saveSelectionSong(t, database, "remote", 2)
	if err := database.EnsurePlexSchema(); err != nil {
		t.Fatal(err)
	}
	if _, err := database.conn.Exec(`INSERT INTO plex_sources(id, machine_identifier, base_url, connected_at)
		VALUES('src', 'machine', 'http://plex.invalid', 1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := database.conn.Exec(`INSERT INTO plex_tracks(song_id, source_id, library_id, machine_identifier, rating_key, media_key, updated_at)
		VALUES('remote', 'src', 'lib', 'machine', 'rating', 'media', 1)`); err != nil {
		t.Fatal(err)
	}
	got, err := database.ExpandAnalysisSelection(AnalysisSelection{Mode: AnalysisSelectionAll}, 1, testAlgorithmVersion)
	if err != nil {
		t.Fatal(err)
	}
	if !equalStrings(got, []string{"local"}) {
		t.Fatalf("got %v, want [local]", got)
	}
}

func TestTrackAnalysisValidRequiresMatchingSourceAndVersions(t *testing.T) {
	database := selectionDatabase(t)
	saveSelectionSong(t, database, "song", 1)
	if err := database.UpsertTrackAnalysis(TrackAnalysis{
		SongID: "song", Status: TrackAnalysisComplete, AnalysisVersion: 1,
		AlgorithmVersion: testAlgorithmVersion, SourceFingerprint: "fp",
	}); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name             string
		fingerprint      string
		analysisVersion  int
		algorithmVersion string
		want             bool
	}{
		{name: "current", fingerprint: "fp", analysisVersion: 1, algorithmVersion: testAlgorithmVersion, want: true},
		{name: "source changed", fingerprint: "other", analysisVersion: 1, algorithmVersion: testAlgorithmVersion},
		{name: "analysis version bumped", fingerprint: "fp", analysisVersion: 2, algorithmVersion: testAlgorithmVersion},
		{name: "algorithm changed", fingerprint: "fp", analysisVersion: 1, algorithmVersion: "track-v2"},
	} {
		got, err := database.TrackAnalysisValid("song", test.fingerprint, test.analysisVersion, test.algorithmVersion)
		if err != nil {
			t.Fatalf("%s: %v", test.name, err)
		}
		if got != test.want {
			t.Errorf("%s: valid = %v, want %v", test.name, got, test.want)
		}
	}
	// A song with no record at all is never valid.
	got, err := database.TrackAnalysisValid("absent", "fp", 1, testAlgorithmVersion)
	if err != nil {
		t.Fatal(err)
	}
	if got {
		t.Fatal("a song without a record must not be reported valid")
	}
}

// A poison source that failed against the current analyzer and the current
// bytes is settled. Treating it as invalid would re-decode it on every run.
func TestTrackAnalysisValidTreatsSettledFailuresAsDone(t *testing.T) {
	database := selectionDatabase(t)
	saveSelectionSong(t, database, "poison", 1)
	for _, status := range []string{TrackAnalysisFailed, TrackAnalysisUnsupported, TrackAnalysisPartial} {
		if err := database.UpsertTrackAnalysis(TrackAnalysis{
			SongID: "poison", Status: status, AnalysisVersion: 1,
			AlgorithmVersion: testAlgorithmVersion, SourceFingerprint: "fp",
		}); err != nil {
			t.Fatal(err)
		}
		got, err := database.TrackAnalysisValid("poison", "fp", 1, testAlgorithmVersion)
		if err != nil {
			t.Fatal(err)
		}
		if !got {
			t.Errorf("status %q must be treated as settled", status)
		}
	}
	// An in-flight record is not settled and must be re-dispatched.
	if err := database.UpsertTrackAnalysis(TrackAnalysis{
		SongID: "poison", Status: TrackAnalysisRunning, AnalysisVersion: 1,
		AlgorithmVersion: testAlgorithmVersion, SourceFingerprint: "fp",
	}); err != nil {
		t.Fatal(err)
	}
	got, err := database.TrackAnalysisValid("poison", "fp", 1, testAlgorithmVersion)
	if err != nil {
		t.Fatal(err)
	}
	if got {
		t.Fatal("a running record must not be treated as settled")
	}
}

func equalStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

// Two overlapping jobs must not both analyze the same track. The claim is the
// single-flight primitive that prevents duplicate decoding.
func TestClaimTrackAnalysisIsSingleFlight(t *testing.T) {
	database := selectionDatabase(t)
	saveSelectionSong(t, database, "song", 1)

	first, err := database.ClaimTrackAnalysis("song", "fp", 1, testAlgorithmVersion)
	if err != nil {
		t.Fatal(err)
	}
	if !first {
		t.Fatal("the first claim must be granted")
	}
	second, err := database.ClaimTrackAnalysis("song", "fp", 1, testAlgorithmVersion)
	if err != nil {
		t.Fatal(err)
	}
	if second {
		t.Fatal("a second concurrent claim must be refused")
	}

	record, err := database.GetTrackAnalysis("song")
	if err != nil {
		t.Fatal(err)
	}
	if record.Status != TrackAnalysisRunning {
		t.Fatalf("status = %q, want %q", record.Status, TrackAnalysisRunning)
	}
}

// A claim must never destroy an existing measurement, because a re-analysis
// that is later canceled would otherwise lose the previous result.
func TestClaimTrackAnalysisPreservesMeasuredValues(t *testing.T) {
	database := selectionDatabase(t)
	saveSelectionSong(t, database, "song", 1)
	bpm := 128.5
	if err := database.UpsertTrackAnalysis(TrackAnalysis{
		SongID: "song", Status: TrackAnalysisComplete, AnalysisVersion: 1,
		AlgorithmVersion: testAlgorithmVersion, SourceFingerprint: "old", BPM: &bpm,
	}); err != nil {
		t.Fatal(err)
	}
	claimed, err := database.ClaimTrackAnalysis("song", "new", 1, testAlgorithmVersion)
	if err != nil {
		t.Fatal(err)
	}
	if !claimed {
		t.Fatal("a settled record must be claimable for re-analysis")
	}
	record, err := database.GetTrackAnalysis("song")
	if err != nil {
		t.Fatal(err)
	}
	if record.BPM == nil || *record.BPM != bpm {
		t.Fatalf("claim destroyed the measured BPM: %#v", record)
	}
}

// Releasing a claim returns the track to the work list immediately, so a
// canceled run followed by a resume does not skip the interrupted track.
func TestReleaseTrackAnalysisAllowsImmediateReclaim(t *testing.T) {
	database := selectionDatabase(t)
	saveSelectionSong(t, database, "song", 1)
	if claimed, err := database.ClaimTrackAnalysis("song", "fp", 1, testAlgorithmVersion); err != nil || !claimed {
		t.Fatalf("first claim = %v, %v", claimed, err)
	}
	if err := database.ReleaseTrackAnalysis("song"); err != nil {
		t.Fatal(err)
	}
	reclaimed, err := database.ClaimTrackAnalysis("song", "fp", 1, testAlgorithmVersion)
	if err != nil {
		t.Fatal(err)
	}
	if !reclaimed {
		t.Fatal("a released track must be immediately reclaimable")
	}
	// A released track is not a settled result.
	valid, err := database.TrackAnalysisValid("song", "fp", 1, testAlgorithmVersion)
	if err != nil {
		t.Fatal(err)
	}
	if valid {
		t.Fatal("a released track must not be reported valid")
	}
}

// Work orphaned by a crash must not be leased forever.
func TestClaimTrackAnalysisRecoversExpiredLease(t *testing.T) {
	database := selectionDatabase(t)
	saveSelectionSong(t, database, "song", 1)
	if claimed, err := database.ClaimTrackAnalysis("song", "fp", 1, testAlgorithmVersion); err != nil || !claimed {
		t.Fatalf("first claim = %v, %v", claimed, err)
	}
	// Simulate a holder that died long enough ago for its lease to expire.
	expired := time.Now().UnixMilli() - TrackAnalysisLeaseMillis - 1000
	if _, err := database.conn.Exec(`UPDATE track_analysis SET analyzed_at = ? WHERE song_id = 'song'`, expired); err != nil {
		t.Fatal(err)
	}
	reclaimed, err := database.ClaimTrackAnalysis("song", "fp", 1, testAlgorithmVersion)
	if err != nil {
		t.Fatal(err)
	}
	if !reclaimed {
		t.Fatal("an expired lease must be reclaimable")
	}
}
