package catalog

import (
	"encoding/json"
	"net/url"
	"strings"
	"testing"
)

func TestCaptureRESTOriginalObjectsAndRelations(t *testing.T) {
	id := strings.Repeat("A", 22)
	track := strings.Repeat("T", 22)
	raw := []byte(`{"type":"playlist","id":"` + id + `","uri":"spotify:playlist:` + id + `","snapshot_id":"revision","future_field":{"value":0},"tracks":{"offset":7,"items":[{"track":{"type":"track","id":"` + track + `","name":"Song"}},{"track":null},{"track":{"type":"track","id":"` + track + `","name":"Song"}}]}}`)
	target, _ := url.Parse("https://api.spotify.com/v1/playlists/" + id)
	entities, err := CaptureREST(target, raw)
	if err != nil || len(entities) < 2 {
		t.Fatalf("capture: %+v %v", entities, err)
	}
	root := entities[0]
	if root.EntityType != "playlist" || !strings.Contains(string(root.Payload), "future_field") || !strings.Contains(string(root.Payload), "snapshot_id") {
		t.Fatal("complete original object lost")
	}
	if len(root.Relations) != 3 || root.Relations[0].Position != 7 || root.Relations[2].Position != 9 || root.Relations[0].ChildID != track || !root.Relations[1].Unavailable || root.Relations[2].ChildID != track {
		t.Fatalf("ordered duplicate/null relations: %+v", root.Relations)
	}
	for _, path := range []string{"/v1/me", "/v1/audio-features/" + track, "/api/token"} {
		target, _ = url.Parse("https://api.spotify.com" + path)
		entities, err = CaptureREST(target, raw)
		if err != nil || len(entities) != 0 {
			t.Fatal("non-catalog capture")
		}
	}
	target, _ = url.Parse("https://api.spotify.com/v1/tracks/" + track)
	_, err = CaptureREST(target, []byte(`{"type":"track","id":"`+track+`","uri":"spotify:track:`+id+`"}`))
	if err == nil {
		t.Fatal("mismatched recording accepted")
	}
}

func TestCaptureRESTSavedLibraryPages(t *testing.T) {
	id := strings.Repeat("T", 22)
	target, _ := url.Parse("https://api.spotify.com/v1/me/tracks?offset=10&limit=3")
	raw := []byte(`{"offset":10,"limit":3,"total":13,"next":null,"items":[{"added_at":"2026-01-01","track":{"type":"track","id":"` + id + `","future":false}},{"track":null},{"track":{"type":"track","id":"` + id + `"}}]}`)
	entities, err := CaptureREST(target, raw)
	if err != nil || len(entities) != 2 {
		t.Fatalf("library capture: %+v %v", entities, err)
	}
	page := entities[0]
	if page.EntityType != "library" || page.ID != "rest_saved_tracks" || len(page.Relations) != 3 || page.Relations[0].Position != 10 || !page.Relations[1].Unavailable || page.Relations[2].Position != 12 || !strings.Contains(string(page.Payload), "added_at") {
		t.Fatalf("page relations: %+v", page)
	}
}

func TestCaptureRESTStandaloneTrackPage(t *testing.T) {
	id, track := strings.Repeat("A", 22), strings.Repeat("T", 22)
	for _, kind := range []string{"albums", "playlists"} {
		target, _ := url.Parse("https://api.spotify.com/v1/" + kind + "/" + id + "/tracks?offset=20&limit=2")
		rows := `[{"type":"track","id":"` + track + `"},null]`
		if kind == "playlists" {
			rows = `[{"track":{"type":"track","id":"` + track + `"}},{"track":null}]`
		}
		raw := []byte(`{"offset":20,"limit":2,"total":22,"next":null,"items":` + rows + `}`)
		entities, err := CaptureREST(target, raw)
		if err != nil || len(entities) != 2 {
			t.Fatalf("standalone page: %+v %v", entities, err)
		}
		page := entities[0]
		if page.ID != id || page.EntityType != strings.TrimSuffix(kind, "s") || len(page.Relations) != 2 || page.Relations[0].Position != 20 || page.Relations[0].ChildID != track || !page.Relations[1].Unavailable || !strings.Contains(string(page.Payload), "total") {
			t.Fatalf("parent page lost: %+v", page)
		}
	}
	target, _ := url.Parse("https://api.spotify.com/v1/albums/" + id + "/tracks?offset=invalid")
	if _, err := CaptureREST(target, []byte(`{"items":[],"offset":0}`)); err == nil {
		t.Fatal("invalid checkpoint offset accepted")
	}
}

func TestCaptureRESTTrackAlbumGraph(t *testing.T) {
	trackID := strings.Repeat("T", 22)
	albumID := strings.Repeat("A", 22)
	artistID := strings.Repeat("B", 22)
	artist := map[string]any{"id": artistID, "type": "artist", "uri": "spotify:artist:" + artistID, "name": "Artist"}
	album := map[string]any{"id": albumID, "type": "album", "uri": "spotify:album:" + albumID, "name": "Album", "artists": []any{artist}}
	track := map[string]any{"id": trackID, "type": "track", "uri": "spotify:track:" + trackID, "name": "Track", "duration_ms": 180000, "artists": []any{artist}, "album": album}
	for _, fixture := range []struct {
		name   string
		target string
		body   any
	}{
		{name: "individual", target: "https://api.spotify.com/v1/tracks/" + trackID, body: track},
		{name: "batch", target: "https://api.spotify.com/v1/tracks?ids=" + trackID, body: map[string]any{"tracks": []any{track}}},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			target, err := url.Parse(fixture.target)
			if err != nil {
				t.Fatal(err)
			}
			raw, err := json.Marshal(fixture.body)
			if err != nil {
				t.Fatal(err)
			}
			entities, err := CaptureREST(target, raw)
			if err != nil {
				t.Fatal(err)
			}
			var root, capturedAlbum bool
			for _, entity := range entities {
				if entity.EntityType == "track" && entity.ID == trackID {
					root = true
					foundAlbum := false
					for _, relation := range entity.Relations {
						if relation.Kind == "album" && relation.Position == 0 && relation.ChildType == "album" && relation.ChildID == albumID && !relation.Unavailable {
							foundAlbum = true
						}
					}
					if !foundAlbum {
						t.Fatalf("track-to-album edge missing: %+v", entity.Relations)
					}
				}
				if entity.EntityType == "album" && entity.ID == albumID {
					capturedAlbum = true
				}
			}
			if !root || !capturedAlbum {
				t.Fatalf("REST graph did not capture track and album entities: %+v", entities)
			}
		})
	}
}

func TestCaptureRESTTrackMalformedAlbumIsUnavailable(t *testing.T) {
	trackID := strings.Repeat("T", 22)
	raw, err := json.Marshal(map[string]any{"id": trackID, "type": "track", "uri": "spotify:track:" + trackID, "name": "Track", "duration_ms": 180000, "album": map[string]any{"id": "bad", "type": "album"}})
	if err != nil {
		t.Fatal(err)
	}
	target, _ := url.Parse("https://api.spotify.com/v1/tracks/" + trackID)
	entities, err := CaptureREST(target, raw)
	if err != nil {
		t.Fatal(err)
	}
	for _, entity := range entities {
		if entity.EntityType != "track" || entity.ID != trackID {
			continue
		}
		for _, relation := range entity.Relations {
			if relation.Kind == "album" && relation.Unavailable && relation.ChildID == "" && relation.ChildType == "" {
				return
			}
		}
		t.Fatalf("malformed album identity was admitted: %+v", entity.Relations)
	}
	t.Fatal("track entity not captured")
}
