// Tests artist discography identity, release normalization, bounds, and unavailable responses.
package catalog

import (
	"encoding/json"
	"errors"
	"net/url"
	"strings"
	"testing"
)

func discographyFixture() map[string]any {
	return map[string]any{"__typename": "Artist", "id": strings.Repeat("A", 22), "discography": map[string]any{"all": map[string]any{"items": []any{map[string]any{"releases": map[string]any{"items": []any{map[string]any{"uri": "spotify:album:" + strings.Repeat("B", 22), "name": "Release", "type": "ALBUM", "date": map[string]any{"isoString": "2024-05-12T00:00:00Z"}, "coverArt": map[string]any{"sources": []any{map[string]any{"url": "https://images.example/release"}}}, "tracks": map[string]any{"totalCount": 12}}}}}}}}}
}
func TestArtistAlbumsNormalization(t *testing.T) {
	fixture := discographyFixture()
	group := fixture["discography"].(map[string]any)["all"].(map[string]any)["items"].([]any)[0].(map[string]any)
	list := group["releases"].(map[string]any)
	first := list["items"].([]any)[0]
	list["items"] = []any{first, first, map[string]any{"uri": "spotify:album:" + strings.Repeat("S", 22), "name": "Single", "type": "SINGLE", "tracks": map[string]any{"totalCount": 1}}}
	raw, _ := json.Marshal(fixture)
	result, err := normalizeArtistAlbums(raw, strings.Repeat("A", 22))
	if err != nil || len(result.Items) != 2 || result.Truncated || result.Limit != 100 {
		t.Fatalf("discography: %+v %v", result, err)
	}
	album := result.Items[0]
	if album.Name != "Release" || album.AlbumType != "album" || album.ReleaseDate == nil || *album.ReleaseDate != "2024-05-12" || len(album.Images) != 1 || album.ExternalURLs["spotify"] == "" {
		t.Fatalf("action fields missing: %+v", album)
	}
	if result.Items[1].AlbumType != "single" || result.Items[1].ReleaseDate != nil {
		t.Fatal("single or nullable date lost")
	}
}
func TestArtistAlbumsEmptyBoundAndSchema(t *testing.T) {
	id := strings.Repeat("A", 22)
	for _, raw := range []string{`{}`, `{"discography":{"all":{"items":null}}}`, `{"id":"wrong","discography":{"all":{"items":[]}}}`, `{"discography":{"all":{"items":[{"releases":{"items":[{"uri":"bad","name":"Bad","type":"ALBUM","tracks":{"totalCount":1}}]}}]}}}`} {
		if _, err := normalizeArtistAlbums(json.RawMessage(raw), id); err != ErrSchema {
			t.Fatalf("schema accepted: %s %v", raw, err)
		}
	}
	result, err := normalizeArtistAlbums(json.RawMessage(`{"discography":{"all":{"items":[]}}}`), id)
	if err != nil || result.Items == nil || len(result.Items) != 0 {
		t.Fatal("empty artist discography failed", err)
	}
	groups := make([]any, 100)
	for i := range groups {
		groups[i] = map[string]any{"releases": map[string]any{"items": []any{}}}
	}
	raw, _ := json.Marshal(map[string]any{"discography": map[string]any{"all": map[string]any{"items": groups}}})
	result, err = normalizeArtistAlbums(raw, id)
	if err != nil || !result.Truncated {
		t.Fatal("bounded response presented as complete", err)
	}
	raw, _ = json.Marshal(map[string]any{"discography": map[string]any{"all": map[string]any{"items": append(groups, groups[0])}}})
	if _, err := normalizeArtistAlbums(raw, id); err != ErrSchema {
		t.Fatal("oversized response accepted")
	}
	_, err = normalizeArtistAlbums(json.RawMessage(`{"__typename":"NotFound"}`), id)
	var failure *HTTPError
	if !errors.As(err, &failure) || failure.Status != 404 {
		t.Fatal("unavailable artist did not return 404", err)
	}
}
func TestArtistAlbumsQueryBounds(t *testing.T) {
	id := strings.Repeat("A", 22)
	for _, values := range []url.Values{{"offset": {"1"}}, {"limit": {"101"}}, {"limit": {"bad"}}, {"include_groups": {"album"}}, {"market": {"invalid"}}} {
		if ValidateArtistAlbumsQuery(id, values) != ErrInvalidQuery {
			t.Fatal("invalid query accepted", values)
		}
	}
	if ValidateArtistAlbumsQuery(id, url.Values{"limit": {"100"}, "offset": {"0"}}) != nil {
		t.Fatal("valid bounded query rejected")
	}
}
