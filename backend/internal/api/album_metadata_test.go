package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ajbergh/viib-mediahub/internal/db"
	"github.com/ajbergh/viib-mediahub/internal/scanner"
)

func TestAlbumMetadataUpdatesSelectedTracksAtomically(t *testing.T) {
	dir := t.TempDir()
	database, err := db.New(filepath.Join(dir, "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := database.SaveSongs([]db.Song{
		{ID: "a", Title: "One", Artist: "Singer A", Album: "Shared title", AlbumArtist: "Compilation", TrackNumber: 1, FilePath: filepath.Join(dir, "one.ogg")},
		{ID: "b", Title: "Two", Artist: "Singer B", Album: "Shared title", AlbumArtist: "Compilation", TrackNumber: 2, DiscNumber: 2, FilePath: filepath.Join(dir, "two.ogg")},
		{ID: "c", Title: "Other", Artist: "Unrelated", Album: "Shared title", FilePath: filepath.Join(dir, "other.ogg")},
	}); err != nil {
		t.Fatal(err)
	}
	router := (&API{db: database, scanner: &scanner.Scanner{}}).V2LibraryOperationRoutes()
	patch := func(body string, status int) *httptest.ResponseRecorder {
		t.Helper()
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodPatch, "/albums/metadata", strings.NewReader(body)))
		if response.Code != status {
			t.Fatalf("PATCH album metadata: %d %s", response.Code, response.Body.String())
		}
		return response
	}

	patch(`{"songIds":["a","missing"],"album":"Should roll back","year":2000}`, http.StatusNotFound)
	patch(`{"songIds":["a","b"],"title":"Unintended title"}`, http.StatusBadRequest)
	response := patch(`{"songIds":["a","b"],"album":" New Album ","albumArtist":" New Artist ","genre":["soul","SOUL"]}`, http.StatusOK)
	var updated []db.Song
	if err := json.Unmarshal(response.Body.Bytes(), &updated); err != nil {
		t.Fatal(err)
	}
	if len(updated) != 2 || updated[0].ID != "a" || updated[1].ID != "b" {
		t.Fatalf("Unexpected updated tracks: %+v", updated)
	}
	for _, track := range updated {
		if track.Album != "New Album" || track.AlbumArtist != "New Artist" || track.Year != 0 || track.OriginalYear != 0 || len(track.Genre) != 1 || track.Genre[0] != "Soul" {
			t.Fatalf("Unexpected album metadata: %+v", track)
		}
	}
	if updated[0].Title != "One" || updated[0].Artist != "Singer A" || updated[1].TrackNumber != 2 || updated[1].DiscNumber != 2 {
		t.Fatalf("Track-specific metadata changed: %+v", updated)
	}
	other, err := database.GetSongByID("c")
	if err != nil || other.Album != "Shared title" {
		t.Fatalf("Unrelated album changed: %+v %v", other, err)
	}
}
