package api

import (
	"github.com/ajbergh/viib-mediahub/internal/db"
	"github.com/ajbergh/viib-mediahub/internal/scanner"
	"github.com/go-chi/chi/v5"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestExpiredDeltaCursorReturnsResnapshotRequired(t *testing.T) {
	database, err := db.New(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := database.EnsureLibrarySyncSchema(); err != nil {
		t.Fatal(err)
	}
	song := db.Song{ID: "song", Title: "Song", FilePath: filepath.Join(t.TempDir(), "song.mp3"), AddedAt: 1}
	for i := 0; i < 8; i++ {
		song.PlayCount = i
		if err := database.SaveSong(&song); err != nil {
			t.Fatal(err)
		}
	}
	if err := database.PruneLibraryChanges(2); err != nil {
		t.Fatal(err)
	}
	api := &API{db: database}
	response := httptest.NewRecorder()
	api.getLibraryChangesV2(response, httptest.NewRequest(http.MethodGet, "/changes?since=0", nil))
	if response.Code != http.StatusGone || !strings.Contains(response.Body.String(), "resnapshot_required") {
		t.Fatalf("expired cursor response: %d %s", response.Code, response.Body.String())
	}
}
func TestRepairAPIRequiresPreviewedIDs(t *testing.T) {
	root := t.TempDir()
	database, err := db.New(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := database.AddScanFolder(&db.ScanFolder{ID: "root", Path: root}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"previewed", "later"} {
		if err := database.SaveSong(&db.Song{ID: id, FilePath: filepath.Join(root, id+".mp3"), AddedAt: 1}); err != nil {
			t.Fatal(err)
		}
	}
	sc := scanner.New(database, t.TempDir())
	defer sc.Close()
	api := &API{db: database, scanner: sc}
	router := chi.NewRouter()
	router.Post("/repair", api.repairLibraryV2)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/repair", strings.NewReader(`{"removeMissing":true,"confirmedSongIds":["previewed"]}`)))
	if response.Code != http.StatusOK {
		t.Fatalf("repair failed: %s", response.Body.String())
	}
	if _, err := database.GetSongByID("later"); err != nil {
		t.Fatal("deleted unpreviewed song", err)
	}
	if _, err := database.GetSongByID("previewed"); err == nil {
		t.Fatal("confirmed missing song retained")
	}
}
