package db

import "testing"

func TestPlexSyncDoesNotAssumeZeroUpdatedAtIsStable(t *testing.T) {
	database := openPlexTestDB(t)
	defer database.Close()
	const sourceID, libraryID, machineID = "plexsrc_zero_updated", "2", "machine-zero-updated"
	if err := database.SavePlexSource(PlexSource{ID: sourceID, MachineIdentifier: machineID, BaseURL: "http://127.0.0.1:32400", Name: "Plex", LibraryID: libraryID, Active: true, Available: true}); err != nil {
		t.Fatal(err)
	}

	first := plexFixture(sourceID, libraryID, machineID, "1", "Original title", 0)
	if added, updated, removed, err := database.SyncPlexLibrary(sourceID, libraryID, []PlexCatalogTrack{first}); err != nil || added != 1 || updated != 0 || removed != 0 {
		t.Fatalf("initial sync: added=%d updated=%d removed=%d err=%v", added, updated, removed, err)
	}

	changed := first
	changed.Title = "Changed without timestamp"
	added, updated, removed, err := database.SyncPlexLibrary(sourceID, libraryID, []PlexCatalogTrack{changed})
	if err != nil || added != 0 || updated != 1 || removed != 0 {
		t.Fatalf("zero-updatedAt refresh: added=%d updated=%d removed=%d err=%v", added, updated, removed, err)
	}
	song, err := database.GetSongByID(first.SongID)
	if err != nil || song == nil || song.Title != changed.Title {
		t.Fatalf("metadata did not refresh without updatedAt: song=%#v err=%v", song, err)
	}
}
