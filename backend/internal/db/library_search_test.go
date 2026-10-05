// Tests and fixtures for library search behavior.

package db

import (
	"fmt"
	"path/filepath"
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
