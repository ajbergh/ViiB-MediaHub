package db

import (
	"testing"
	"time"
)

func TestNextAlbumMetadataAcceptsLegacyAndRendererTimestamps(t *testing.T) {
	database := openLibrarySyncTestDB(t)
	now := time.Now()
	old := now.Add(-31 * 24 * time.Hour)
	album := AlbumMetadata{AlbumKey: "Album::Artist", AlbumName: "Album", ArtistName: "Artist", SpotifyChecked: true}
	for _, timestamp := range []int64{now.Unix(), now.UnixMilli(), old.Unix(), old.UnixMilli()} {
		album.FetchedAt = timestamp
		if err := database.SaveAlbumMetadata(&album); err != nil {
			t.Fatal(err)
		}
		next, err := database.NextAlbumNeedingMetadata(30)
		if err != nil {
			t.Fatal(err)
		}
		expired := timestamp == old.Unix() || timestamp == old.UnixMilli()
		if (next != nil) != expired {
			t.Fatalf("timestamp %d: candidate %+v, expected expired=%v", timestamp, next, expired)
		}
	}
}
