package catalog

import (
	"encoding/json"
	"net/url"
	"strings"
	"testing"
)

func TestSearchNormalizationPreservesActionFields(t *testing.T) {
	artistID := strings.Repeat("A", 22)
	albumID := strings.Repeat("B", 22)
	trackID := strings.Repeat("T", 22)
	playlistID := strings.Repeat("P", 22)
	artist := map[string]any{"uri": "spotify:artist:" + artistID, "profile": map[string]any{"name": "Artist"}}
	sources := []any{map[string]any{"url": "https://images.example/cover", "height": 640, "width": 640}}
	album := map[string]any{"__typename": "Album", "uri": "spotify:album:" + albumID, "name": "Album", "artists": map[string]any{"items": []any{artist}}, "coverArt": map[string]any{"sources": sources}, "date": map[string]any{"year": 2020}, "type": "ALBUM"}
	track := map[string]any{"__typename": "Track", "uri": "spotify:track:" + trackID, "name": "Track", "artists": map[string]any{"items": []any{artist}}, "albumOfTrack": album, "duration": map[string]any{"totalMilliseconds": 123456}, "contentRating": map[string]any{"label": "EXPLICIT"}}
	raw, _ := json.Marshal(map[string]any{"item": map[string]any{"data": track}})
	converted, err := normalizeTrack(raw)
	if err != nil || converted.ID != trackID || converted.DurationMS != 123456 || converted.Album.Name != "Album" || converted.Artists[0].Name != "Artist" || converted.Explicit == nil || !*converted.Explicit {
		t.Fatalf("track: %+v %v", converted, err)
	}
	if converted.Album.Images[0].URL != "https://images.example/cover" || converted.ExternalURLs["spotify"] != "https://open.spotify.com/track/"+trackID {
		t.Fatal("track action fields lost")
	}
	raw, _ = json.Marshal(map[string]any{"data": album})
	convertedAlbum, err := normalizeAlbum(raw)
	if err != nil || convertedAlbum.Artists[0].ID != artistID || convertedAlbum.ReleaseDate == nil || *convertedAlbum.ReleaseDate != "2020" {
		t.Fatalf("album: %v", err)
	}
	artist["__typename"] = "Artist"
	artist["visuals"] = map[string]any{"avatarImage": map[string]any{"sources": sources}}
	raw, _ = json.Marshal(map[string]any{"data": artist})
	convertedArtist, err := normalizeArtist(raw)
	if err != nil || convertedArtist.ID != artistID || convertedArtist.Name != "Artist" || convertedArtist.ExternalURLs["spotify"] == "" || convertedArtist.Followers != nil {
		t.Fatalf("artist: %+v %v", convertedArtist, err)
	}
	playlist := map[string]any{"__typename": "Playlist", "uri": "spotify:playlist:" + playlistID, "name": "Playlist", "images": map[string]any{"items": []any{map[string]any{"sources": sources}}}, "ownerV2": map[string]any{"data": map[string]any{"uri": "spotify:user:owner", "name": "Owner"}}}
	raw, _ = json.Marshal(map[string]any{"data": playlist})
	convertedPlaylist, err := normalizePlaylist(raw)
	if err != nil || convertedPlaylist.ID != playlistID || convertedPlaylist.Owner.DisplayName != "Owner" || len(convertedPlaylist.Images) != 1 {
		t.Fatalf("playlist: %+v %v", convertedPlaylist, err)
	}
}
func TestSearchPageBoundsAndUnavailableRows(t *testing.T) {
	total := 5
	query := SearchQuery{Term: "music & artist", Types: []string{"album"}, Limit: 2, Offset: 0}
	page, err := normalizePage(&wirePage{Total: &total, Items: []json.RawMessage{json.RawMessage("{\"data\":{\"__typename\":\"NotFound\"}}"), json.RawMessage("{\"data\":{\"__typename\":\"NotFound\"}}")}}, query, "album", normalizeAlbum)
	if err != nil || len(page.Items) != 2 || page.Items[0] != nil || page.Next == nil {
		t.Fatal("unavailable rows lost cursor position")
	}
	next, err := url.Parse(*page.Next)
	if err != nil || next.Query().Get("offset") != "2" || next.Query().Get("q") != query.Term {
		t.Fatal("bad pagination")
	}
	query.Offset = 4
	page, err = normalizePage(&wirePage{Total: &total, Items: []json.RawMessage{}}, query, "album", normalizeAlbum)
	if err != nil || page.Items == nil || page.Next != nil || page.Previous == nil {
		t.Fatal("terminal page lacks explicit null/empty fields")
	}
	raw, _ := json.Marshal(page)
	if !strings.Contains(string(raw), "\"next\":null") || !strings.Contains(string(raw), "\"items\":[]") {
		t.Fatal("terminal page is ambiguous")
	}
}
func TestSearchRejectsMissingDurationAndInvalidQuery(t *testing.T) {
	raw := json.RawMessage("{\"item\":{\"data\":{\"__typename\":\"Track\",\"uri\":\"spotify:track:TTTTTTTTTTTTTTTTTTTTTT\",\"name\":\"Track\"}}}")
	if _, err := normalizeTrack(raw); err != ErrSchema {
		t.Fatal("invalid track was made playable")
	}
	for _, values := range []url.Values{{"q": {""}}, {"q": {"music"}, "limit": {"0"}}, {"q": {"music"}, "offset": {"-1"}}, {"q": {"music"}, "type": {"user"}}, {"q": {"music"}, "type": {"track,track"}}} {
		if _, err := ParseSearchQuery(values); err != ErrInvalidQuery {
			t.Fatal("invalid query accepted")
		}
	}
}

// An intentionally unnamed playlist is valid Spotify data. A missing/null name
// is a schema error; preserve the empty value rather than inventing a title.
func TestPlaylistSummaryPreservesPresentEmptyName(t *testing.T) {
	playlist, err := normalizePlaylist(json.RawMessage(`{"data":{"__typename":"Playlist","uri":"spotify:playlist:1234567890123456789012","name":"","ownerV2":{"data":{"uri":"spotify:user:fixture","name":"Owner"}}}}`))
	if err != nil || playlist == nil || playlist.Name != "" {
		t.Fatalf("unnamed playlist rejected: %v", err)
	}
	for _, raw := range []string{
		`{"data":{"__typename":"Playlist","uri":"spotify:playlist:1234567890123456789012"}}`,
		`{"data":{"__typename":"Playlist","uri":"spotify:playlist:1234567890123456789012","name":null}}`,
	} {
		if _, err := normalizePlaylist(json.RawMessage(raw)); err == nil {
			t.Fatal("missing name accepted")
		}
	}
}
