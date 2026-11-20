// Tests and fixtures for library search behavior.

package db

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

func TestSearchLibraryArtistSubstring(t *testing.T) {
	database := openLibrarySyncTestDB(t)
	songs := []Song{}
	for i := 0; i < 3; i++ {
		song := testLibrarySong(fmt.Sprint(i), "Tonight", filepath.Join(t.TempDir(), fmt.Sprintf("%d.flac", i)))
		song.Artist = "The Smashing Pumpkins"
		song.AlbumArtist = song.Artist
		song.Album = "Mellon Collie"
		if i == 1 {
			song.Artist = "Guest Singer"
			song.Album = "Siamese Dream"
		}
		if i == 2 {
			song.Artist = "Pumpkins Tribute"
			song.AlbumArtist = ""
		}
		songs = append(songs, song)
	}
	if err := database.SaveSongs(songs); err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{"Pumpkins", "  PUMPKINS  ", "smashing pump"} {
		result, err := database.SearchLibrary(query, 20)
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, artist := range result.Artists {
			if artist.Name == "The Smashing Pumpkins" {
				found = true
				if artist.SongCount != 2 || artist.AlbumCount != 2 {
					t.Fatalf("double-counted or missing songs: %#v", artist)
				}
			}
		}
		if !found {
			t.Fatalf("%q did not find band: %#v", query, result.Artists)
		}
		if len(result.Tracks) < 2 || len(result.Albums) < 2 {
			t.Fatalf("missing artist-associated results: %#v", result)
		}
		if query == "Pumpkins" && result.Artists[0].Name != "Pumpkins Tribute" {
			t.Fatalf("prefix match should rank first: %#v", result.Artists)
		}
	}
	for _, query := range []string{"%", "_", "Pupkins"} {
		result, err := database.SearchLibrary(query, 20)
		if err != nil {
			t.Fatal(err)
		}
		if len(result.Artists) != 0 {
			t.Fatalf("%q unexpectedly matched: %#v", query, result.Artists)
		}
	}
	if _, err := database.conn.Exec("UPDATE songs SET ignored = 1 WHERE id IN ('0', '1')"); err != nil {
		t.Fatal(err)
	}
	result, err := database.SearchLibrary("smashing", 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Artists)+len(result.Tracks)+len(result.Albums) != 0 {
		t.Fatalf("ignored songs leaked into search: %#v", result)
	}
}

func TestSearchLibraryContextCancellationAndEdgeCases(t *testing.T) {
	database := openLibrarySyncTestDB(t)
	dir := t.TempDir()
	songs := []Song{
		func() Song {
			s := testLibrarySong("u2", "With or Without You", filepath.Join(dir, "u2.flac"))
			s.Artist = "U2"
			s.Album = "The Joshua Tree"
			return s
		}(),
		func() Song {
			s := testLibrarySong("acdc", "Back in Black", filepath.Join(dir, "acdc.flac"))
			s.Artist = "AC/DC"
			s.AlbumArtist = "AC/DC"
			s.Album = "Back in Black"
			return s
		}(),
		func() Song {
			s := testLibrarySong("quote", "Track", filepath.Join(dir, "quote.flac"))
			s.Artist = `Bob "The Boss" Smith`
			s.Album = "Live"
			return s
		}(),
	}
	if err := database.SaveSongs(songs); err != nil {
		t.Fatal(err)
	}

	// Short 1-2 char artist query fallback.
	shortRes, err := database.SearchLibraryContext(context.Background(), "u2", 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(shortRes.Artists) != 1 || shortRes.Artists[0].Name != "U2" {
		t.Fatalf("short query failed: %#v", shortRes.Artists)
	}

	// Punctuation and quote escaping in >= 3 char FTS queries.
	for _, q := range []string{"ac/dc", `"the boss"`} {
		res, err := database.SearchLibraryContext(context.Background(), q, 20)
		if err != nil {
			t.Fatalf("query %q failed: %v", q, err)
		}
		if len(res.Tracks) != 1 || len(res.Artists) != 1 {
			t.Fatalf("query %q unexpected result: %#v", q, res)
		}
	}

	// 3-char wildcard/punctuation queries must not match unrelated tracks or error.
	for _, q := range []string{"%%%", "___", "..."} {
		res, err := database.SearchLibraryContext(context.Background(), q, 20)
		if err != nil {
			t.Fatalf("query %q failed: %v", q, err)
		}
		if len(res.Tracks)+len(res.Albums)+len(res.Artists) != 0 {
			t.Fatalf("query %q unexpectedly matched: %#v", q, res)
		}
	}

	// Triggers keep FTS in sync on update and delete.
	updated := songs[1]
	updated.Artist = "Updated Ensemble"
	updated.AlbumArtist = "Updated Ensemble"
	if err := database.SaveSong(&updated); err != nil {
		t.Fatal(err)
	}
	oldRes, err := database.SearchLibraryContext(context.Background(), "ac/dc", 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(oldRes.Tracks) != 0 || len(oldRes.Artists) != 0 {
		t.Fatalf("old artist remained in FTS after update: %#v", oldRes)
	}
	newRes, err := database.SearchLibraryContext(context.Background(), "ensemble", 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(newRes.Tracks) != 1 || len(newRes.Artists) != 1 {
		t.Fatalf("updated artist missing from FTS: %#v", newRes)
	}
	if err := database.DeleteSong(updated.ID); err != nil {
		t.Fatal(err)
	}
	delRes, err := database.SearchLibraryContext(context.Background(), "ensemble", 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(delRes.Tracks) != 0 || len(delRes.Artists) != 0 {
		t.Fatalf("deleted song remained in FTS: %#v", delRes)
	}

	// Context cancellation propagates cleanly.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := database.SearchLibraryContext(ctx, "live", 20); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestSearchLibraryBackfillsFTSOnceOnExistingDatabase(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "library.db")
	database, err := New(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.EnsureLibrarySyncSchema(); err != nil {
		t.Fatal(err)
	}
	song := testLibrarySong("pre-fts", "Horizon", filepath.Join(t.TempDir(), "horizon.flac"))
	song.Artist = "The Midnight Runners"
	if err := database.SaveSong(&song); err != nil {
		t.Fatal(err)
	}
	// Simulate an upgraded database where song_search exists but FTS has not been backfilled yet.
	if _, err := database.conn.Exec(`DELETE FROM song_artist_fts; UPDATE song_search_meta SET artist_fts_ready = 0 WHERE id = 1;`); err != nil {
		t.Fatal(err)
	}
	_ = database.Close()

	reopened, err := New(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if err := reopened.EnsureLibrarySyncSchema(); err != nil {
		t.Fatal(err)
	}
	res, err := reopened.SearchLibrary("midnight", 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Tracks) != 1 || len(res.Artists) != 1 {
		t.Fatalf("backfilled FTS missed existing row: %#v", res)
	}
}

func TestSearchLibraryQueryPlanAvoidsFullSongSearchScan(t *testing.T) {
	database := openLibrarySyncTestDB(t)
	lower, upper := searchPrefixBounds("pumpkins")
	genreLower, genreUpper := searchPrefixBounds(`["pumpkins`)
	phrase := fts5TrigramPhrase("pumpkins")

	rows, err := database.conn.Query(`EXPLAIN QUERY PLAN
		WITH matched_songs AS (
			SELECT song_id FROM song_search WHERE title >= ? AND title < ?
			UNION
			SELECT song_id FROM song_search WHERE album >= ? AND album < ?
			UNION
			SELECT song_id FROM song_search WHERE artist >= ? AND artist < ?
			UNION
			SELECT song_id FROM song_search WHERE album_artist >= ? AND album_artist < ?
			UNION
			SELECT song_id FROM song_search WHERE genre >= ? AND genre < ?
			UNION
			SELECT song_id FROM song_search WHERE file_path >= ? AND file_path < ?
			UNION
			SELECT song_id FROM song_artist_fts
			WHERE song_artist_fts MATCH ?
			  AND (instr(artist, ?) > 0 OR instr(album_artist, ?) > 0)
		)
		SELECT songs.id FROM songs
		JOIN matched_songs m ON m.song_id = songs.id
		JOIN song_search ss ON ss.song_id = songs.id
		WHERE COALESCE(songs.ignored, 0) = 0
		LIMIT 50`,
		lower, upper, lower, upper, lower, upper, lower, upper, genreLower, genreUpper, lower, upper,
		phrase, "pumpkins", "pumpkins")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var plan strings.Builder
	for rows.Next() {
		var id, parent, unused int
		var detail string
		if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
			t.Fatal(err)
		}
		plan.WriteString(detail)
		plan.WriteByte('\n')
	}
	planText := plan.String()
	if strings.Contains(planText, "SCAN song_search\n") || strings.Contains(planText, "SCAN songs\n") {
		t.Fatalf("unexpected full table scan in query plan:\n%s", planText)
	}
	if !strings.Contains(planText, "VIRTUAL TABLE INDEX") {
		t.Fatalf("expected FTS5 virtual table index usage in query plan:\n%s", planText)
	}
}

func BenchmarkSearchLibrary50k(b *testing.B) {
	database, err := New(filepath.Join(b.TempDir(), "bench.db"))
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = database.Close() })
	if err := database.EnsureLibrarySyncSchema(); err != nil {
		b.Fatal(err)
	}

	const total = 50000
	batch := make([]Song, 0, 1000)
	for i := 0; i < total; i++ {
		artist := fmt.Sprintf("Artist %05d", i)
		album := fmt.Sprintf("Album %05d", i)
		title := fmt.Sprintf("Track %05d", i)
		if i == 25000 {
			artist = "The Smashing Pumpkins"
			album = "Mellon Collie"
			title = "Tonight Tonight"
		} else if i == 25001 {
			artist = "Pumpkins Tribute"
			album = "Covers"
			title = "Zero"
		}
		batch = append(batch, Song{
			ID:          fmt.Sprintf("song-%05d", i),
			Title:       title,
			Artist:      artist,
			AlbumArtist: artist,
			Album:       album,
			Genre:       []string{"Rock"},
			Duration:    180,
			FilePath:    fmt.Sprintf("/music/%05d.flac", i),
			AddedAt:     1,
			FileHash:    fmt.Sprintf("hash-%05d", i),
		})
		if len(batch) == cap(batch) {
			if err := database.SaveSongs(batch); err != nil {
				b.Fatal(err)
			}
			batch = batch[:0]
		}
	}

	b.Run("TitlePrefix", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			res, err := database.SearchLibrary("tonight", 50)
			if err != nil || len(res.Tracks) == 0 {
				b.Fatalf("unexpected search result: len=%d err=%v", len(res.Tracks), err)
			}
		}
	})

	b.Run("ArtistSubstring", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			res, err := database.SearchLibrary("pumpkins", 50)
			if err != nil || len(res.Artists) < 2 || res.Artists[0].Name != "Pumpkins Tribute" {
				b.Fatalf("unexpected artist search result: %#v err=%v", res.Artists, err)
			}
		}
	})
}
