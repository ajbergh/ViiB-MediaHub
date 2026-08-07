package api

import (
	"testing"

	"github.com/ajbergh/viib-mediahub/internal/db"
)

func TestExtractDecadeFromPrompt(t *testing.T) {
	minYear, maxYear := extractDecadeFromPrompt("90s west coast hip-hop")
	if minYear != 1990 || maxYear != 1999 {
		t.Fatalf("extractDecadeFromPrompt() = %d-%d, want 1990-1999", minYear, maxYear)
	}
}

func TestSongMatchesYearPrefersOriginalReleaseYear(t *testing.T) {
	tests := []struct {
		name string
		song db.Song
		want bool
	}{
		{"90s original despite 2004 remaster", db.Song{Year: 2004, OriginalYear: 1994}, true},
		{"2004 release without original year", db.Song{Year: 2004}, false},
		{"unknown year cannot satisfy explicit era", db.Song{}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := songMatchesYear(tt.song, 1990, 1999); got != tt.want {
				t.Fatalf("songMatchesYear() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestApplyPlayHistoryFiltersSupportsDatabaseSongs(t *testing.T) {
	api := &API{}
	songs := []any{
		db.Song{ID: "recent", Artist: "Artist A", PlayCount: 9},
		db.Song{ID: "first", Artist: "Artist A", PlayCount: 1},
		db.Song{ID: "duplicate", Artist: "artist a", PlayCount: 2},
		db.Song{ID: "other", Artist: "Artist B", PlayCount: 3},
	}

	got := api.applyPlayHistoryFilters(songs, map[string]bool{"recent": true}, "favorites", true, 10)
	if len(got) != 2 {
		t.Fatalf("filtered song count = %d, want 2", len(got))
	}
	if got[0].(db.Song).ID != "other" || got[1].(db.Song).ID != "first" {
		t.Fatalf("unexpected filtered songs: %#v", got)
	}
}
