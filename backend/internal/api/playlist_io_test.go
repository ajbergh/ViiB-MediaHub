// Tests and fixtures for playlist io behavior.

package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/ajbergh/viib-mediahub/internal/db"
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

func TestCreatePlaylistPersistsExactReferenceFirstCandidateSelection(t *testing.T) {
	api, _ := newBPMRouteTestAPI(t, false)
	payload := []byte(`{"name":"Reference mix","songIds":["reference","candidate-b","candidate-a"]}`)
	recorder := httptest.NewRecorder()
	api.Routes().ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/playlists", bytes.NewReader(payload)))
	if recorder.Code != http.StatusOK {
		t.Fatalf("POST playlist = %d: %s", recorder.Code, recorder.Body.String())
	}
	var created db.Playlist
	if err := json.NewDecoder(recorder.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	if created.ID == "" || created.CreatedAt == 0 || created.Name != "Reference mix" || !reflect.DeepEqual(created.SongIDs, []string{"reference", "candidate-b", "candidate-a"}) {
		t.Fatalf("created playlist = %#v", created)
	}
	listed := httptest.NewRecorder()
	api.Routes().ServeHTTP(listed, httptest.NewRequest(http.MethodGet, "/playlists", nil))
	if listed.Code != http.StatusOK {
		t.Fatalf("GET playlists = %d: %s", listed.Code, listed.Body.String())
	}
	var playlists []db.Playlist
	if err := json.NewDecoder(listed.Body).Decode(&playlists); err != nil || len(playlists) != 1 || playlists[0].ID != created.ID || !reflect.DeepEqual(playlists[0].SongIDs, created.SongIDs) {
		t.Fatalf("GET playlists = %#v, err=%v", playlists, err)
	}
}

func TestCreatePlaylistRejectsInvalidBoundariesAndPreservesRepeatedTracks(t *testing.T) {
	longID := strings.Repeat("x", 257)
	for _, test := range []struct {
		name string
		p    db.Playlist
	}{
		{name: "empty name", p: db.Playlist{Name: "", SongIDs: []string{"a"}}},
		{name: "whitespace name", p: db.Playlist{Name: "  \t", SongIDs: []string{"a"}}},
		{name: "name too long", p: db.Playlist{Name: string(make([]byte, 201)), SongIDs: []string{"a"}}},
		{name: "cover path too long", p: db.Playlist{Name: "Mix", CoverPath: strings.Repeat("c", 2049), SongIDs: []string{"a"}}},
		{name: "empty track id", p: db.Playlist{Name: "Mix", SongIDs: []string{""}}},
		{name: "track id too long", p: db.Playlist{Name: "Mix", SongIDs: []string{longID}}},
		{name: "too many tracks", p: db.Playlist{Name: "Mix", SongIDs: make([]string, 10001)}},
	} {
		t.Run(test.name, func(t *testing.T) {
			api, _ := newBPMRouteTestAPI(t, false)
			body, err := json.Marshal(test.p)
			if err != nil {
				t.Fatal(err)
			}
			recorder := httptest.NewRecorder()
			api.Routes().ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/playlists", bytes.NewReader(body)))
			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("POST invalid playlist = %d: %s", recorder.Code, recorder.Body.String())
			}
			playlists, err := api.db.GetAllPlaylists()
			if err != nil || len(playlists) != 0 {
				t.Fatalf("invalid playlist persisted: %#v, err=%v", playlists, err)
			}
		})
	}

	api, _ := newBPMRouteTestAPI(t, false)
	payload, err := json.Marshal(db.Playlist{Name: "Repeat", SongIDs: []string{"a", "b", "a"}})
	if err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	api.Routes().ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/playlists", bytes.NewReader(payload)))
	if recorder.Code != http.StatusOK {
		t.Fatalf("POST repeated-track playlist = %d: %s", recorder.Code, recorder.Body.String())
	}
	var created db.Playlist
	if err := json.NewDecoder(recorder.Body).Decode(&created); err != nil || created.Name != "Repeat" || !reflect.DeepEqual(created.SongIDs, []string{"a", "b", "a"}) {
		t.Fatalf("repeated track order = %#v, err=%v", created.SongIDs, err)
	}

	trimmedPayload := []byte(`{"name":"  Clean name  ","songIds":["a"]}`)
	trimmed := httptest.NewRecorder()
	api.Routes().ServeHTTP(trimmed, httptest.NewRequest(http.MethodPost, "/playlists", bytes.NewReader(trimmedPayload)))
	var normalized db.Playlist
	if trimmed.Code != http.StatusOK || json.NewDecoder(trimmed.Body).Decode(&normalized) != nil || normalized.Name != "Clean name" {
		t.Fatalf("trimmed playlist = %d, %+v: %s", trimmed.Code, normalized, trimmed.Body.String())
	}
}

func TestUpdatePlaylistRejectsInvalidBoundariesWithoutChangingStoredPlaylist(t *testing.T) {
	api, _ := newBPMRouteTestAPI(t, false)
	if err := api.db.SavePlaylist(&db.Playlist{ID: "mix", Name: "Original", SongIDs: []string{"a", "b"}, CreatedAt: 1}); err != nil {
		t.Fatal(err)
	}
	payload := []byte(`{"name":"  ","songIds":["changed"]}`)
	recorder := httptest.NewRecorder()
	api.Routes().ServeHTTP(recorder, httptest.NewRequest(http.MethodPut, "/playlists/mix", bytes.NewReader(payload)))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("PUT invalid playlist = %d: %s", recorder.Code, recorder.Body.String())
	}
	playlists, err := api.db.GetAllPlaylists()
	if err != nil || len(playlists) != 1 || playlists[0].Name != "Original" || !reflect.DeepEqual(playlists[0].SongIDs, []string{"a", "b"}) {
		t.Fatalf("invalid update modified playlist: %#v, err=%v", playlists, err)
	}
}

func TestUpdatePlaylistNotFoundDoesNotCreateAndPreservesCreationTime(t *testing.T) {
	api, _ := newBPMRouteTestAPI(t, false)
	if err := api.db.SavePlaylist(&db.Playlist{ID: "existing", Name: "Original", SongIDs: []string{"a"}, CreatedAt: 123}); err != nil {
		t.Fatal(err)
	}
	body := []byte(`{"name":"Changed","songIds":["b"],"createdAt":999}`)
	missing := httptest.NewRecorder()
	api.Routes().ServeHTTP(missing, httptest.NewRequest(http.MethodPut, "/playlists/missing", bytes.NewReader(body)))
	if missing.Code != http.StatusNotFound {
		t.Fatalf("PUT missing playlist = %d: %s", missing.Code, missing.Body.String())
	}
	updated := httptest.NewRecorder()
	api.Routes().ServeHTTP(updated, httptest.NewRequest(http.MethodPut, "/playlists/existing", bytes.NewReader(body)))
	if updated.Code != http.StatusOK {
		t.Fatalf("PUT existing playlist = %d: %s", updated.Code, updated.Body.String())
	}
	var result db.Playlist
	if err := json.NewDecoder(updated.Body).Decode(&result); err != nil || result.CreatedAt != 123 || result.Name != "Changed" || !reflect.DeepEqual(result.SongIDs, []string{"b"}) {
		t.Fatalf("updated playlist = %+v, err=%v", result, err)
	}
	all, err := api.db.GetAllPlaylists()
	if err != nil || len(all) != 1 || all[0].ID != "existing" {
		t.Fatalf("PUT created or lost playlist: %+v, err=%v", all, err)
	}
}
