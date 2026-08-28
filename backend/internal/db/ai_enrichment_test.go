package db

import (
	"path/filepath"
	"testing"
)

func TestAIEnrichmentSelectsAllNeededFieldsAndPreservesExistingMetadata(t *testing.T) {
	database, err := New(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer database.Close()
	if err := database.EnsureLibrarySyncSchema(); err != nil {
		t.Fatalf("initialize library synchronization: %v", err)
	}

	if err := database.SaveSongs([]Song{
		{ID: "remaster", Title: "Song", Artist: "Artist", Album: "Album", FilePath: "remaster.mp3", FileHash: "one", AddedAt: 1},
		{ID: "mood", Title: "Other", Artist: "Artist", Album: "Album", FilePath: "mood.mp3", FileHash: "two", AddedAt: 2},
	}); err != nil {
		t.Fatalf("save songs: %v", err)
	}
	if _, err := database.conn.Exec(`UPDATE songs SET genre = '["Rock"]', mood = 'happy', year_uncertain = 1 WHERE id = 'remaster'`); err != nil {
		t.Fatalf("seed remaster: %v", err)
	}
	if _, err := database.conn.Exec(`UPDATE songs SET genre = '["Jazz","Fusion"]' WHERE id = 'mood'`); err != nil {
		t.Fatalf("seed mood: %v", err)
	}

	songs, err := database.GetSongsForAIEnrichment(10, false, 0)
	if err != nil {
		t.Fatalf("GetSongsForAIEnrichment: %v", err)
	}
	if len(songs) != 2 {
		t.Fatalf("selected %d songs, want 2", len(songs))
	}
	revisionBefore, err := database.LibraryRevision()
	if err != nil {
		t.Fatalf("read library revision: %v", err)
	}

	result, err := database.ApplyAIEnrichmentBatch([]AIEnrichmentUpdate{{
		SongID: "remaster", Genres: []string{"alternative rock", "indie rock"},
		Mood: "peaceful", Energy: "low", Tempo: "slow", BPM: 70, OriginalYear: 1994,
	}}, false)
	if err != nil {
		t.Fatalf("ApplyAIEnrichmentBatch: %v", err)
	}
	if result.Songs != 1 || result.Genres != 1 || result.Mood != 0 || result.Years != 1 {
		t.Fatalf("apply result = %#v, want songs=1 genres=1 mood=0 years=1", result)
	}

	var genre, mood string
	var originalYear int
	var uncertain bool
	if err := database.conn.QueryRow(`SELECT genre, mood, original_year, year_uncertain FROM songs WHERE id = 'remaster'`).Scan(&genre, &mood, &originalYear, &uncertain); err != nil {
		t.Fatalf("read enriched song: %v", err)
	}
	if genre != `["Alternative Rock","Indie Rock"]` || mood != "happy" || originalYear != 1994 || uncertain {
		t.Fatalf("unexpected stored values: genre=%s mood=%s year=%d uncertain=%v", genre, mood, originalYear, uncertain)
	}

	changes, err := database.GetLibraryChanges(revisionBefore, 10)
	if err != nil {
		t.Fatalf("read enrichment delta: %v", err)
	}
	if len(changes.Changes) != 1 || len(changes.Songs) != 1 || changes.Songs[0].ID != "remaster" {
		t.Fatalf("enrichment delta = %#v", changes)
	}
	if got := changes.Songs[0]; len(got.Genre) != 2 || got.OriginalYear != 1994 || got.Mood != "happy" {
		t.Fatalf("enriched delta song = %#v", got)
	}

	if err := database.UpdateGenreStats(); err != nil {
		t.Fatalf("refresh genre stats: %v", err)
	}
	stats, err := database.GetAllGenreStats()
	if err != nil {
		t.Fatalf("read genre stats: %v", err)
	}
	counts := make(map[string]int, len(stats))
	for _, stat := range stats {
		counts[stat.Name] = stat.Count
	}
	if counts["Alternative Rock"] != 1 || counts["Indie Rock"] != 1 || counts["Fusion"] != 1 {
		t.Fatalf("genre counts = %#v", counts)
	}
}
