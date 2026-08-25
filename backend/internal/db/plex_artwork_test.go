package db

import (
	"strings"
	"testing"
)

func TestPlexSyncRefreshesArtworkWhenKeyChangesWithoutUpdatedAtChange(t *testing.T) {
	database := openPlexTestDB(t)
	defer database.Close()
	const sourceID, libraryID, machineID = "plexsrc_art", "2", "machine-art"
	if err := database.SavePlexSource(PlexSource{
		ID: sourceID, MachineIdentifier: machineID, BaseURL: "http://127.0.0.1:32400",
		Name: "Plex", LibraryID: libraryID, LibraryTitle: "Music", Active: true, Available: true,
	}); err != nil {
		t.Fatal(err)
	}

	track := plexFixture(sourceID, libraryID, machineID, "1", "Track", 100)
	track.ArtworkKey = "/library/metadata/album/thumb/100"
	added, updated, removed, err := database.SyncPlexLibrary(sourceID, libraryID, []PlexCatalogTrack{track})
	if err != nil || added != 1 || updated != 0 || removed != 0 {
		t.Fatalf("initial sync: added=%d updated=%d removed=%d err=%v", added, updated, removed, err)
	}
	first, err := database.GetSongByID(track.SongID)
	if err != nil {
		t.Fatal(err)
	}
	firstCover := first.CoverPath
	if !strings.HasPrefix(firstCover, "plex://art/"+track.SongID+"?v=") {
		t.Fatalf("unexpected first cover path: %q", firstCover)
	}

	track.ArtworkKey = "/library/metadata/album/thumb/101"
	added, updated, removed, err = database.SyncPlexLibrary(sourceID, libraryID, []PlexCatalogTrack{track})
	if err != nil || added != 0 || updated != 1 || removed != 0 {
		t.Fatalf("artwork-only sync: added=%d updated=%d removed=%d err=%v", added, updated, removed, err)
	}

	source, err := database.GetPlexTrackSource(track.SongID)
	if err != nil || source == nil {
		t.Fatalf("source lookup: %#v err=%v", source, err)
	}
	if source.ArtworkKey != "/library/metadata/album/thumb/101" {
		t.Fatalf("artwork key=%q", source.ArtworkKey)
	}
	second, err := database.GetSongByID(track.SongID)
	if err != nil {
		t.Fatal(err)
	}
	if second.CoverPath == firstCover {
		t.Fatalf("browser cache key did not change with Plex artwork: %q", second.CoverPath)
	}
}

func TestPlexSyncRefreshesMediaKeyWhenUpdatedAtIsUnchanged(t *testing.T) {
	database := openPlexTestDB(t)
	defer database.Close()
	const sourceID, libraryID, machineID = "plexsrc_media", "2", "machine-media"
	if err := database.SavePlexSource(PlexSource{
		ID: sourceID, MachineIdentifier: machineID, BaseURL: "http://127.0.0.1:32400",
		Name: "Plex", LibraryID: libraryID, Active: true, Available: true,
	}); err != nil {
		t.Fatal(err)
	}

	track := plexFixture(sourceID, libraryID, machineID, "1", "Track", 100)
	if _, _, _, err := database.SyncPlexLibrary(sourceID, libraryID, []PlexCatalogTrack{track}); err != nil {
		t.Fatal(err)
	}
	track.MediaKey = "/library/parts/1/replaced.flac"
	_, updated, _, err := database.SyncPlexLibrary(sourceID, libraryID, []PlexCatalogTrack{track})
	if err != nil || updated != 1 {
		t.Fatalf("media-key-only sync updated=%d err=%v", updated, err)
	}
	source, err := database.GetPlexTrackSource(track.SongID)
	if err != nil || source.MediaKey != track.MediaKey {
		t.Fatalf("media key=%q err=%v", source.MediaKey, err)
	}
}

func TestPlexSyncPublishesAndReconcilesArtistArtwork(t *testing.T) {
	database := openPlexTestDB(t)
	defer database.Close()
	const sourceID, libraryID, machineID = "plexsrc_artist_art", "2", "machine-artist-art"
	if err := database.SavePlexSource(PlexSource{
		ID: sourceID, MachineIdentifier: machineID, BaseURL: "http://127.0.0.1:32400",
		Name: "Plex", LibraryID: libraryID, Active: true, Available: true,
	}); err != nil {
		t.Fatal(err)
	}

	track := plexFixture(sourceID, libraryID, machineID, "1", "Track", 100)
	track.Artist = "The Lumineers"
	track.AlbumArtist = track.Artist
	track.ArtistArtworkKey = "/library/metadata/artist-8/thumb/42"
	if _, _, _, err := database.SyncPlexLibrary(sourceID, libraryID, []PlexCatalogTrack{track}); err != nil {
		t.Fatal(err)
	}

	artwork, err := database.GetActivePlexArtistArtwork(track.Artist)
	if err != nil || artwork == nil || artwork.SourceID != sourceID || artwork.ArtworkKey != track.ArtistArtworkKey {
		t.Fatalf("artist artwork=%#v err=%v", artwork, err)
	}
	metadata, err := database.GetArtistMetadata(track.Artist)
	if err != nil || metadata == nil || !strings.HasPrefix(metadata.PlexImageURL, "/api/v2/plex/artist-artwork/The%20Lumineers?v=") {
		t.Fatalf("artist metadata=%#v err=%v", metadata, err)
	}

	track.ArtistArtworkKey = ""
	if _, _, _, err := database.SyncPlexLibrary(sourceID, libraryID, []PlexCatalogTrack{track}); err != nil {
		t.Fatal(err)
	}
	artwork, err = database.GetActivePlexArtistArtwork(track.Artist)
	if err != nil || artwork != nil {
		t.Fatalf("stale artwork=%#v err=%v", artwork, err)
	}
	metadata, err = database.GetArtistMetadata(track.Artist)
	if err != nil || metadata == nil || metadata.PlexImageURL != "" {
		t.Fatalf("stale Plex image URL=%#v err=%v", metadata, err)
	}
}
