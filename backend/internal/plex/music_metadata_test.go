package plex

import "testing"

func TestMapPlexTrackDoesNotTreatOriginalTitleAsArtist(t *testing.T) {
	track, ok := mapPlexTrack(plexTrack{
		RatingKey:     "localized-1",
		Title:         "Localized title",
		OriginalTitle: "Original-language title",
		ParentTitle:   "Album",
		Media: []plexMedia{{
			Container:  "flac",
			AudioCodec: "flac",
			Part:       []plexPart{{Key: "/library/parts/localized-1/file.flac"}},
		}},
	})
	if !ok {
		t.Fatal("expected track to map")
	}
	if track.Artist != "" || track.AlbumArtist != "" {
		t.Fatalf("originalTitle leaked into artist fields: artist=%q albumArtist=%q", track.Artist, track.AlbumArtist)
	}
	if track.Title != "Localized title" {
		t.Fatalf("title=%q", track.Title)
	}
}
