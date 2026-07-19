package api

import (
	"reflect"
	"testing"
)

func TestParseM3UIgnoresMetadataAndBlankLines(t *testing.T) {
	content := "\ufeff#EXTM3U\n#EXTINF:180,Artist - Song\nC:\\Music\\song.flac\n\n# comment\n/home/user/music/other.ogg\n"
	got := parseM3U(content)
	want := []string{"C:\\Music\\song.flac", "/home/user/music/other.ogg"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseM3U() = %#v, want %#v", got, want)
	}
}

func TestSafePlaylistFilenameRemovesReservedCharacters(t *testing.T) {
	got := safePlaylistFilename(`Road/Trip: 2026?`)
	if got != "Road-Trip- 2026" {
		t.Fatalf("safePlaylistFilename() = %q", got)
	}
}
