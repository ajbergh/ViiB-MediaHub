package api

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ajbergh/viib-mediahub/internal/db"
)

func TestV2StemStatusReturnsMetadataWithoutPackagePaths(t *testing.T) {
	database, err := db.New(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err = database.SaveSong(&db.Song{ID: "song", Title: "Song", FilePath: "source.wav", AddedAt: 1}); err != nil {
		t.Fatal(err)
	}
	if err = database.UpsertStemSet(db.StemSet{ID: "set", SongID: "song", SourceAudioHash: "hash", Status: "ready", PackagePath: "C:/private/song.viibstems", Stems: []db.StemArtifact{{Name: "vocals", RelativePath: "private/vocals.wav", SHA256: "hash", SizeBytes: 4}}}); err != nil {
		t.Fatal(err)
	}
	router := (&API{db: database}).V2StemRoutes()
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/song", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	body := recorder.Body.String()
	if strings.Contains(body, "C:/private") || strings.Contains(body, "private/vocals.wav") || !strings.Contains(body, "\"sourceAudioHash\":\"hash\"") {
		t.Fatalf("unexpected status response: %s", body)
	}
}

func TestV2StemStatusRejectsUnknownSong(t *testing.T) {
	database, err := db.New(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	router := (&API{db: database}).V2StemRoutes()
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/missing", nil))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}
