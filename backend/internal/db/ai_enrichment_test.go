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

	result, err := database.ApplyAIEnrichmentBatch([]AIEnrichmentUpdate{{
		SongID: "remaster", Genres: []string{"alternative rock", "indie rock"},
		Mood: "peaceful", Energy: "low", Tempo: "slow", BPM: 70, OriginalYear: 1994,
	}}, false)
	if err != nil {
		t.Fatalf("ApplyAIEnrichmentBatch: %v", err)
	}
	if result.Genres != 1 || result.Mood != 0 || result.Years != 1 {
		t.Fatalf("apply result = %#v, want genres=1 mood=0 years=1", result)
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
}
