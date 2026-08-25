package db

import (
	"path/filepath"
	"testing"
)

func openPlexTestDB(t *testing.T) *DB {
	t.Helper()
	database, err := New(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	if err := database.EnsurePlexSchema(); err != nil {
		database.Close()
		t.Fatalf("ensure Plex schema: %v", err)
	}
	return database
}

func plexFixture(sourceID, libraryID, machineID, ratingKey, title string, updatedAt int64) PlexCatalogTrack {
	return PlexCatalogTrack{
		SongID: "plex_song_" + ratingKey, SourceID: sourceID, LibraryID: libraryID, MachineID: machineID,
		RatingKey: ratingKey, MetadataKey: "/library/metadata/" + ratingKey, MediaKey: "/library/parts/" + ratingKey + "/file.flac",
		ArtworkKey: "/library/metadata/" + ratingKey + "/thumb/1", Container: "flac", AudioCodec: "flac", UpdatedAt: updatedAt,
		Title: title, Artist: "Artist", Album: "Album", AlbumArtist: "Artist", TrackNumber: 1, DiscNumber: 1,
		Genres: []string{"rock"}, Year: 2025, Duration: 245, AddedAt: 1000,
	}
}

func TestPlexSyncAddUpdateRemoveAndOfflineRetention(t *testing.T) {
	database := openPlexTestDB(t)
	defer database.Close()
	const sourceID, libraryID, machineID = "plexsrc_test", "2", "machine-test"
	if err := database.SavePlexSource(PlexSource{ID: sourceID, MachineIdentifier: machineID, BaseURL: "http://127.0.0.1:32400", Name: "Plex", LibraryID: libraryID, LibraryTitle: "Music", Active: true, Available: true}); err != nil {
		t.Fatal(err)
	}

	added, updated, removed, err := database.SyncPlexLibrary(sourceID, libraryID, []PlexCatalogTrack{
		plexFixture(sourceID, libraryID, machineID, "1", "First", 100),
		plexFixture(sourceID, libraryID, machineID, "2", "Second", 100),
	})
	if err != nil || added != 2 || updated != 0 || removed != 0 {
		t.Fatalf("initial sync: added=%d updated=%d removed=%d err=%v", added, updated, removed, err)
	}
	first, err := database.GetSongByID("plex_song_1")
	if err != nil || first.Title != "First" || first.FilePath != "plex://machine-test/2/1" {
		t.Fatalf("unexpected first song: %#v err=%v", first, err)
	}

	updatedTrack := plexFixture(sourceID, libraryID, machineID, "1", "First (Remastered)", 200)
	added, updated, removed, err = database.SyncPlexLibrary(sourceID, libraryID, []PlexCatalogTrack{updatedTrack})
	if err != nil || added != 0 || updated != 1 || removed != 1 {
		t.Fatalf("second sync: added=%d updated=%d removed=%d err=%v", added, updated, removed, err)
	}
	first, _ = database.GetSongByID("plex_song_1")
	if first.Title != "First (Remastered)" {
		t.Fatalf("metadata was not updated: %#v", first)
	}
	if removedSong, err := database.GetSongByID("plex_song_2"); err == nil || removedSong != nil {
		t.Fatalf("expected removed song lookup to fail, song=%#v err=%v", removedSong, err)
	}
	added, updated, removed, err = database.SyncPlexLibrary(sourceID, libraryID, []PlexCatalogTrack{updatedTrack})
	if err != nil || added != 0 || updated != 0 || removed != 0 {
		t.Fatalf("unchanged resync should be a no-op: added=%d updated=%d removed=%d err=%v", added, updated, removed, err)
	}

	if err := database.SetPlexSyncState(sourceID, "error", "offline", false, 0); err != nil {
		t.Fatal(err)
	}
	if retained, err := database.GetSongByID("plex_song_1"); err != nil || retained.Title == "" {
		t.Fatalf("offline state deleted cached track: %#v err=%v", retained, err)
	}
}

func TestPlexSyncRetainsExistingTrackWhenSnapshotOmitsPlayablePart(t *testing.T) {
	database := openPlexTestDB(t)
	defer database.Close()
	const sourceID, libraryID, machineID = "plexsrc_test", "2", "machine-test"
	if err := database.SavePlexSource(PlexSource{ID: sourceID, MachineIdentifier: machineID, BaseURL: "http://127.0.0.1:32400", Name: "Plex", LibraryID: libraryID, LibraryTitle: "Music", Active: true, Available: true}); err != nil {
		t.Fatal(err)
	}
	original := plexFixture(sourceID, libraryID, machineID, "1", "Still Here", 100)
	if _, _, _, err := database.SyncPlexLibrary(sourceID, libraryID, []PlexCatalogTrack{original}); err != nil {
		t.Fatal(err)
	}

	presenceOnly := original
	presenceOnly.MediaKey = ""
	presenceOnly.Container = ""
	presenceOnly.AudioCodec = ""
	presenceOnly.UpdatedAt = 200
	added, updated, removed, err := database.SyncPlexLibrary(sourceID, libraryID, []PlexCatalogTrack{presenceOnly})
	if err != nil || added != 0 || updated != 0 || removed != 0 {
		t.Fatalf("presence-only sync: added=%d updated=%d removed=%d err=%v", added, updated, removed, err)
	}
	retained, err := database.GetSongByID("plex_song_1")
	if err != nil || retained == nil || retained.Title != "Still Here" {
		t.Fatalf("temporarily unplayable track was removed: song=%#v err=%v", retained, err)
	}
	trackSource, err := database.GetPlexTrackSource("plex_song_1")
	if err != nil || trackSource == nil || trackSource.MediaKey != original.MediaKey {
		t.Fatalf("cached playable source should remain intact: source=%#v err=%v", trackSource, err)
	}
}

func TestChangingPlexLibraryRetainsCacheUntilSuccessfulSync(t *testing.T) {
	database := openPlexTestDB(t)
	defer database.Close()
	const sourceID, machineID = "plexsrc_test", "machine-test"
	if err := database.SavePlexSource(PlexSource{ID: sourceID, MachineIdentifier: machineID, BaseURL: "http://127.0.0.1:32400", Name: "Plex", LibraryID: "2", LibraryTitle: "Music A", Active: true, Available: true}); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := database.SyncPlexLibrary(sourceID, "2", []PlexCatalogTrack{plexFixture(sourceID, "2", machineID, "1", "Old Library Track", 1)}); err != nil {
		t.Fatal(err)
	}

	if err := database.SetPlexLibrary(sourceID, "3", "Music B"); err != nil {
		t.Fatal(err)
	}
	// Selecting a different library is not an authoritative remote read. The old
	// cache must remain available if the server becomes unreachable before sync.
	if oldSong, err := database.GetSongByID("plex_song_1"); err != nil || oldSong == nil {
		t.Fatalf("old selected library cache was removed before successful sync: song=%#v err=%v", oldSong, err)
	}
	if err := database.SetPlexSyncState(sourceID, "error", "offline", false, 0); err != nil {
		t.Fatal(err)
	}
	if retained, err := database.GetSongByID("plex_song_1"); err != nil || retained == nil {
		t.Fatalf("offline library switch deleted cached track: song=%#v err=%v", retained, err)
	}

	source, err := database.GetActivePlexSource()
	if err != nil || source.LibraryID != "3" || source.LibraryTitle != "Music B" {
		t.Fatalf("unexpected source after library selection: %#v err=%v", source, err)
	}

	added, updated, removed, err := database.SyncPlexLibrary(sourceID, "3", []PlexCatalogTrack{
		plexFixture(sourceID, "3", machineID, "2", "New Library Track", 1),
	})
	if err != nil || added != 1 || updated != 0 || removed != 1 {
		t.Fatalf("new library authoritative sync: added=%d updated=%d removed=%d err=%v", added, updated, removed, err)
	}
	if oldSong, err := database.GetSongByID("plex_song_1"); err == nil || oldSong != nil {
		t.Fatalf("old library track remained after successful new-library sync: song=%#v err=%v", oldSong, err)
	}
	if newSong, err := database.GetSongByID("plex_song_2"); err != nil || newSong == nil || newSong.Title != "New Library Track" {
		t.Fatalf("new library track missing after successful sync: song=%#v err=%v", newSong, err)
	}
}

func TestPlexDiagnosticsIgnoreRemoteSyntheticPaths(t *testing.T) {
	database := openPlexTestDB(t)
	defer database.Close()
	const sourceID, libraryID, machineID = "plexsrc_test", "2", "machine-test"
	if err := database.SavePlexSource(PlexSource{ID: sourceID, MachineIdentifier: machineID, BaseURL: "http://127.0.0.1:32400", Name: "Plex", LibraryID: libraryID, Active: true, Available: false}); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := database.SyncPlexLibrary(sourceID, libraryID, []PlexCatalogTrack{plexFixture(sourceID, libraryID, machineID, "1", "Remote", 1)}); err != nil {
		t.Fatal(err)
	}
	diagnostics, err := database.RunLibraryDiagnostics()
	if err != nil {
		t.Fatal(err)
	}
	if len(diagnostics.MissingMedia) != 0 {
		t.Fatalf("Plex remote catalog incorrectly reported as missing local media: %#v", diagnostics.MissingMedia)
	}
}
