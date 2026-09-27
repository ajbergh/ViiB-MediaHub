package api

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ajbergh/viib-mediahub/internal/db"
)

func TestV2LibrarySnapshotIncludesBatchedPathFreeStemStatus(t *testing.T) {
	database, err := db.New(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	for _, id := range []string{"with-stems", "without-stems"} {
		if err := database.SaveSong(&db.Song{ID: id, Title: id, FilePath: id + ".wav", AddedAt: 1}); err != nil {
			t.Fatal(err)
		}
	}
	if err := database.UpsertStemSet(db.StemSet{ID: "set", SongID: "with-stems", Status: "ready", PackagePath: "C:/private/package.viibstems"}); err != nil {
		t.Fatal(err)
	}
	router := (&API{db: database}).V2Routes()
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/library/snapshot?limit=10", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	body := recorder.Body.String()
	if !strings.Contains(body, `"stemStatus":"ready"`) || !strings.Contains(body, `"stemStatus":"none"`) {
		t.Fatalf("snapshot omitted path-free stem status: %s", body)
	}
	if strings.Contains(body, `"packagePath"`) || strings.Contains(body, `C:/private/package.viibstems`) {
		t.Fatalf("snapshot exposed registry paths: %s", body)
	}
}

func TestLegacySongListIncludesStemStatus(t *testing.T) {
	database, err := db.New(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := database.SaveSong(&db.Song{ID: "with-stems", Title: "t", FilePath: "t.wav", AddedAt: 1}); err != nil {
		t.Fatal(err)
	}
	if err := database.UpsertStemSet(db.StemSet{ID: "set", SongID: "with-stems", Status: "ready", PackagePath: "C:/private/package.viibstems"}); err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	(&API{db: database}).getSongs(recorder, httptest.NewRequest(http.MethodGet, "/api/songs", nil))
	if body := recorder.Body.String(); !strings.Contains(body, `"stemStatus":"ready"`) {
		t.Fatalf("legacy song list omitted stem status: %s", body)
	}
}

func TestStemStatusChangeAdvancesLibraryRevision(t *testing.T) {
	database, err := db.New(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := database.EnsureLibrarySyncSchema(); err != nil {
		t.Fatal(err)
	}
	if err := database.SaveSong(&db.Song{ID: "song", Title: "t", FilePath: "t.wav", AddedAt: 1}); err != nil {
		t.Fatal(err)
	}
	api := &API{db: database}
	before, err := database.LibraryRevision()
	if err != nil {
		t.Fatal(err)
	}

	api.trackStemStatusChange("song")() // unchanged: no change recorded
	if revision, _ := database.LibraryRevision(); revision != before {
		t.Fatalf("unchanged stem status advanced revision %d -> %d", before, revision)
	}

	done := api.trackStemStatusChange("song")
	if err := database.UpsertStemSet(db.StemSet{ID: "set", SongID: "song", Status: "ready", PackagePath: "C:/p.viibstems"}); err != nil {
		t.Fatal(err)
	}
	done()
	changes, err := database.GetLibraryChanges(before, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes.Changes) != 1 || changes.Changes[0].SongID != "song" || changes.Changes[0].Operation != "upsert" {
		t.Fatalf("expected one upsert for song, got %+v", changes.Changes)
	}
	if changes.ToRevision != before+1 {
		t.Fatalf("revision = %d, want %d", changes.ToRevision, before+1)
	}
}
