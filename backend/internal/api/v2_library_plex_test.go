package api

import (
	"testing"

	"github.com/ajbergh/viib-mediahub/internal/db"
)

func TestTransformLibrarySongForAPIPreservesPlexArtworkVersion(t *testing.T) {
	song := db.Song{
		ID:        "plex_abc123",
		FilePath:  "plex://machine/library/42",
		CoverPath: "plex://art/plex_abc123?v=deadbeef",
	}

	transformLibrarySongForAPI(&song)

	if song.Path != "plex://machine/library/42" {
		t.Fatalf("path=%q want original Plex source path", song.Path)
	}
	if song.FilePath != "/api/audio/plex_abc123" {
		t.Fatalf("file path=%q", song.FilePath)
	}
	if song.CoverPath != "/api/cover/plex_abc123?v=deadbeef" {
		t.Fatalf("cover path=%q", song.CoverPath)
	}
}

func TestTransformLibrarySongForAPIDoesNotInventLocalCoverVersion(t *testing.T) {
	song := db.Song{
		ID:        "local123",
		FilePath:  "/music/album/track.flac",
		CoverPath: "/cache/covers/album.jpg",
	}

	transformLibrarySongForAPI(&song)

	if song.CoverPath != "/api/cover/local123" {
		t.Fatalf("cover path=%q", song.CoverPath)
	}
}
