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
