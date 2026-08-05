package db

import (
	"path/filepath"
	"testing"
)

func openLibrarySyncTestDB(t *testing.T) *DB {
	t.Helper()
	database, err := New(filepath.Join(t.TempDir(), "library.db"))
	if err != nil { t.Fatalf("open database: %v", err) }
	t.Cleanup(func() { _ = database.Close() })
	if err := database.EnsureLibrarySyncSchema(); err != nil {
		t.Fatalf("ensure library sync schema: %v", err)
	}
	return database
}

func testLibrarySong(id, title, path string) Song {
	return Song{
		ID: id, Title: title, Artist: "Example Artist", Album: "Example Album",
		Duration: 120, FilePath: path, AddedAt: 1, FileHash: "hash-" + id,
	}
}

func TestLibrarySnapshotAndChanges(t *testing.T) {
	database := openLibrarySyncTestDB(t)
	songs := []Song{
		testLibrarySong("a", "Alpha", filepath.Join(t.TempDir(), "a.mp3")),
		testLibrarySong("b", "Beta", filepath.Join(t.TempDir(), "b.mp3")),
	}
	if err := database.SaveSongs(songs); err != nil { t.Fatalf("save songs: %v", err) }

	revision, err := database.LibraryRevision()
	if err != nil { t.Fatalf("library revision: %v", err) }
	if revision != 2 { t.Fatalf("expected revision 2, got %d", revision) }

	first, err := database.ListSongsPage("", 1)
	if err != nil { t.Fatalf("first snapshot page: %v", err) }
	if len(first.Songs) != 1 || !first.HasMore || first.NextCursor != "a" {
		t.Fatalf("unexpected first page: %#v", first)
	}
	second, err := database.ListSongsPage(first.NextCursor, 1)
	if err != nil { t.Fatalf("second snapshot page: %v", err) }
	if len(second.Songs) != 1 || second.HasMore || second.Songs[0].ID != "b" {
		t.Fatalf("unexpected second page: %#v", second)
	}

	updated := songs[0]
	updated.Title = "Alpha Updated"
	if err := database.SaveSong(&updated); err != nil { t.Fatalf("update song: %v", err) }
	if _, err := database.DeleteSongsByFilePaths([]string{songs[1].FilePath}); err != nil {
		t.Fatalf("delete song: %v", err)
	}

	changes, err := database.GetLibraryChanges(2, 20)
	if err != nil { t.Fatalf("get changes: %v", err) }
	if len(changes.Changes) != 2 {
		t.Fatalf("expected update and delete changes, got %#v", changes.Changes)
	}
	if changes.Changes[0].Operation != "upsert" || changes.Changes[1].Operation != "delete" {
		t.Fatalf("unexpected operations: %#v", changes.Changes)
	}
	if len(changes.Songs) != 1 || changes.Songs[0].Title != "Alpha Updated" {
		t.Fatalf("expected current upsert payload, got %#v", changes.Songs)
	}
}

func TestSearchLibraryUsesBackfilledIndex(t *testing.T) {
	database := openLibrarySyncTestDB(t)
	song := testLibrarySong("search", "Northern Lights", filepath.Join(t.TempDir(), "northern.flac"))
	song.Genre = []string{"Ambient"}
	if err := database.SaveSong(&song); err != nil { t.Fatalf("save song: %v", err) }

	result, err := database.SearchLibrary("northern", 20)
	if err != nil { t.Fatalf("search library: %v", err) }
	if len(result.Tracks) != 1 || result.Tracks[0].ID != song.ID {
		t.Fatalf("unexpected track search result: %#v", result.Tracks)
	}

	genreResult, err := database.SearchLibrary("ambient", 20)
	if err != nil { t.Fatalf("search genre: %v", err) }
	if len(genreResult.Tracks) != 1 { t.Fatalf("expected genre match") }
}
