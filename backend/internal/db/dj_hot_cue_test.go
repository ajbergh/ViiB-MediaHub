package db

import (
	"path/filepath"
	"testing"
)

func TestDJHotCueProvenanceRoundTripsAndLegacyPayloadDefaultsToUser(t *testing.T) {
	database, err := New(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if _, err := database.conn.Exec(`INSERT INTO songs(id, title, artist, album, file_path, added_at) VALUES ('song', 'Song', 'Artist', 'Album', 'song.mp3', 1)`); err != nil {
		t.Fatal(err)
	}

	confidence := 0.875
	const updatedAt = int64(1725000000123)
	if err := database.SaveDJHotCues("song", []DJHotCue{{
		Slot: 2, Position: 12.5, Label: "Verse", Color: "#123456", Origin: "analysis",
		GeneratorVersion: "cue-v2", Confidence: &confidence, Kind: "verse", Locked: true,
		Rationale: "qualified-measured-downbeat", SourceFingerprint: "source-hash", DownbeatAligned: true, UpdatedAt: updatedAt,
	}}); err != nil {
		t.Fatal(err)
	}
	loaded, err := database.GetDJHotCues("song")
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 1 {
		t.Fatalf("loaded cue count = %d, want 1", len(loaded))
	}
	cue := loaded[0]
	if cue.Slot != 2 || cue.Position != 12.5 || cue.Label != "Verse" || cue.Color != "#123456" || cue.Origin != "analysis" || cue.GeneratorVersion != "cue-v2" || cue.Confidence == nil || *cue.Confidence != confidence || cue.Kind != "verse" || !cue.Locked || cue.Rationale != "qualified-measured-downbeat" || cue.SourceFingerprint != "source-hash" || !cue.DownbeatAligned || cue.UpdatedAt != updatedAt {
		t.Fatalf("loaded cue = %#v", cue)
	}
	if err := database.SaveDJHotCues("song", []DJHotCue{{
		Slot: 2, Position: 12.5, Origin: "analysis", GeneratorVersion: "cue-v2", Confidence: &confidence, Kind: "verse",
	}}); err != nil {
		t.Fatal(err)
	}
	loaded, err = database.GetDJHotCues("song")
	if err != nil || len(loaded) != 1 || loaded[0].Rationale != "qualified-measured-downbeat" || loaded[0].SourceFingerprint != "source-hash" || !loaded[0].DownbeatAligned {
		t.Fatalf("sparse analysis round-trip erased metadata: %#v, %v", loaded, err)
	}

	// Older full-set clients omit provenance; the saved row stays a valid user cue.
	if err := database.SaveDJHotCues("song", []DJHotCue{{Slot: 1, Position: 4, Color: "#abcdef"}}); err != nil {
		t.Fatal(err)
	}
	loaded, err = database.GetDJHotCues("song")
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 1 || loaded[0].Origin != "user" || loaded[0].UpdatedAt <= 0 || loaded[0].Locked {
		t.Fatalf("legacy payload cue = %#v, want user origin, write time, and unlocked", loaded)
	}
}

func TestDJHotCueMigrationAddsProvenanceToExistingRows(t *testing.T) {
	database, err := New(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if _, err := database.conn.Exec(`DROP TABLE dj_hot_cues`); err != nil {
		t.Fatal(err)
	}
	if _, err := database.conn.Exec(`CREATE TABLE dj_hot_cues (
		id INTEGER PRIMARY KEY AUTOINCREMENT, song_id TEXT NOT NULL, slot INTEGER NOT NULL,
		position REAL NOT NULL, label TEXT, color TEXT DEFAULT '#FF5500', created_at INTEGER NOT NULL,
		UNIQUE(song_id, slot)
	)`); err != nil {
		t.Fatal(err)
	}
	if _, err := database.conn.Exec(`INSERT INTO dj_hot_cues(song_id, slot, position, label, color, created_at) VALUES ('song', 3, 8, 'Old cue', '#ff0000', 1234)`); err != nil {
		t.Fatal(err)
	}
	if err := database.migrateColumns(); err != nil {
		t.Fatal(err)
	}
	if _, err := database.conn.Exec(`DROP TABLE dj_hot_cue_suppressions`); err != nil {
		t.Fatal(err)
	}
	if err := database.migrateColumns(); err != nil {
		t.Fatal(err)
	}
	var suppressionTableCount int
	if err := database.conn.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='dj_hot_cue_suppressions'`).Scan(&suppressionTableCount); err != nil || suppressionTableCount != 1 {
		t.Fatalf("suppression migration table count = %d, %v", suppressionTableCount, err)
	}
	var origin string
	var locked int
	var updatedAt int64
	if err := database.conn.QueryRow(`SELECT origin, locked, updated_at FROM dj_hot_cues WHERE song_id='song'`).Scan(&origin, &locked, &updatedAt); err != nil {
		t.Fatal(err)
	}
	if origin != "user" || locked != 0 || updatedAt != 1234000 {
		t.Fatalf("migrated provenance = origin %q locked %d updatedAt %d", origin, locked, updatedAt)
	}
	migrated, err := database.GetDJHotCues("song")
	if err != nil || len(migrated) != 1 || migrated[0].Rationale != "" || migrated[0].SourceFingerprint != "" || migrated[0].DownbeatAligned {
		t.Fatalf("migrated optional cue metadata = %#v, %v; want empty/unaligned defaults", migrated, err)
	}
}

func TestSaveDJHotCuesRejectsInvalidSlotsBeforeReplacingRows(t *testing.T) {
	database, err := New(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if _, err := database.conn.Exec(`INSERT INTO songs(id, title, artist, album, file_path, added_at) VALUES ('song', 'Song', 'Artist', 'Album', 'song.mp3', 1)`); err != nil {
		t.Fatal(err)
	}
	if err := database.SaveDJHotCues("song", []DJHotCue{{Slot: 1, Position: 3, Color: "#fff"}}); err != nil {
		t.Fatal(err)
	}
	if err := database.SaveDJHotCues("song", []DJHotCue{{Slot: 9, Position: 4, Color: "#000"}}); err == nil {
		t.Fatal("SaveDJHotCues accepted slot 9")
	}
	loaded, err := database.GetDJHotCues("song")
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 1 || loaded[0].Slot != 1 {
		t.Fatalf("existing cue set after invalid save = %#v", loaded)
	}
}

func TestGeneratedCueDeletionTombstoneLifecycleAndReplacement(t *testing.T) {
	database, err := New(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if _, err := database.conn.Exec(`INSERT INTO songs(id, title, artist, album, file_path, added_at) VALUES ('song', 'Song', 'Artist', 'Album', 'song.mp3', 1)`); err != nil {
		t.Fatal(err)
	}
	confidence := .75
	if err := database.SaveDJHotCues("song", []DJHotCue{
		{Slot: 1, Position: 4, Label: "Manual", Origin: "user"},
		{Slot: 2, Position: 8, Label: "Generated mix-in", Origin: "analysis", GeneratorVersion: "auto-cues-v1", Confidence: &confidence, Kind: "mix-in"},
		{Slot: 4, Position: 24, Label: "Stale", Origin: "analysis", GeneratorVersion: "auto-cues-v1", Confidence: &confidence, Kind: "section"},
		{Slot: 5, Position: 32, Label: "Locked", Origin: "analysis", GeneratorVersion: "auto-cues-v1", Confidence: &confidence, Kind: "section", Locked: true},
	}); err != nil {
		t.Fatal(err)
	}
	// Full-set omission represents user deletion of slot 2's generated cue.
	if err := database.SaveDJHotCues("song", []DJHotCue{
		{Slot: 1, Position: 4, Label: "Manual", Origin: "user"},
		{Slot: 5, Position: 32, Label: "Locked", Origin: "analysis", GeneratorVersion: "auto-cues-v1", Confidence: &confidence, Kind: "section", Locked: true},
	}); err != nil {
		t.Fatal(err)
	}
	suppressions, err := database.GetDJHotCueSuppressions("song")
	if err != nil || len(suppressions) != 2 {
		t.Fatalf("deletion suppressions = %#v, %v; want deleted generated slots 2 and 4", suppressions, err)
	}
	if suppressions[0] != (DJHotCueSuppression{Slot: 2, Kind: "mix-in"}) {
		t.Fatalf("suppression is not scoped by slot/kind: %#v", suppressions)
	}

	newConfidence := .9
	generated := []DJHotCue{
		{Slot: 1, Position: 5, Label: "Should not replace manual", Origin: "analysis", GeneratorVersion: "auto-cues-v1", Confidence: &newConfidence, Kind: "intro"},
		{Slot: 2, Position: 10, Label: "Different role", Origin: "analysis", GeneratorVersion: "auto-cues-v1", Confidence: &newConfidence, Kind: "section"},
		{Slot: 3, Position: 16, Label: "New section", Origin: "analysis", GeneratorVersion: "auto-cues-v1", Confidence: &newConfidence, Kind: "section"},
	}
	if err := database.ApplyGeneratedDJHotCues("song", generated, GeneratedCueReplaceGenerated); err != nil {
		t.Fatal(err)
	}
	loaded, err := database.GetDJHotCues("song")
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 4 || loaded[0].Origin != "user" || loaded[1].Slot != 2 || loaded[1].Kind != "section" || loaded[2].Slot != 3 || loaded[3].Slot != 5 || !loaded[3].Locked {
		t.Fatalf("generated apply did not preserve user/tombstone policy: %#v", loaded)
	}
	// A newly generated semantic kind at a deleted slot may be used, but the
	// original mix-in deletion stays suppressed until an explicit user action.
	if len(suppressions) == 0 {
		t.Fatal("generator unexpectedly cleared deletion tombstones")
	}
	if err := database.SaveDJHotCues("song", []DJHotCue{
		{Slot: 1, Position: 4, Label: "Manual", Origin: "user"},
		{Slot: 2, Position: 11, Label: "User replacement", Origin: "user"},
		{Slot: 3, Position: 16, Label: "New section", Origin: "analysis", GeneratorVersion: "auto-cues-v1", Confidence: &newConfidence, Kind: "section"},
		{Slot: 5, Position: 32, Label: "Locked", Origin: "analysis", GeneratorVersion: "auto-cues-v1", Confidence: &confidence, Kind: "section", Locked: true},
	}); err != nil {
		t.Fatal(err)
	}
	suppressions, err = database.GetDJHotCueSuppressions("song")
	if err != nil {
		t.Fatal(err)
	}
	for _, suppression := range suppressions {
		if suppression.Slot == 2 {
			t.Fatalf("explicit user re-add did not clear slot 2 tombstone: %#v", suppressions)
		}
	}
	if err := database.ClearDJHotCueSuppression("song", 4); err != nil {
		t.Fatal(err)
	}

	// Refresh removes unlocked analysis cues that are no longer candidates,
	// including all such cues for an empty result, while retaining locked rows.
	if err := database.ApplyGeneratedDJHotCues("song", []DJHotCue{{Slot: 6, Position: 40, Label: "Fresh", Origin: "analysis", GeneratorVersion: "auto-cues-v1", Confidence: &newConfidence, Kind: "section"}}, GeneratedCueReplaceGenerated); err != nil {
		t.Fatal(err)
	}
	if err := database.ApplyGeneratedDJHotCues("song", nil, GeneratedCueReplaceGenerated); err != nil {
		t.Fatal(err)
	}
	loaded, err = database.GetDJHotCues("song")
	if err != nil {
		t.Fatal(err)
	}
	for _, cue := range loaded {
		if cue.Origin == "analysis" && !cue.Locked {
			t.Fatalf("empty refresh retained stale generated cue: %#v", loaded)
		}
	}
}
