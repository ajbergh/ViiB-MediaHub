package api

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ajbergh/viib-mediahub/internal/db"
)

// Exercise the real backup/stage router and offline activation with synthetic
// relations. This is clean-shutdown evidence, not crash/installer qualification.
func TestBackupStageRestorePreservesRelationsAndRollback(t *testing.T) {
	dataDir := t.TempDir()
	databasePath := filepath.Join(dataDir, "library.db")
	database, err := db.New(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	mediaDir := t.TempDir()
	media := []byte("synthetic media bytes; never decoded")
	for _, id := range []string{"a", "b"} {
		path := filepath.Join(mediaDir, id+".mp3")
		must(os.WriteFile(path, media, 0600))
		must(database.SaveSong(&db.Song{ID: id, Title: "Original " + id, Artist: "Fixture artist", Album: "Fixture album", Genre: []string{"Fixture"}, FilePath: path, Duration: 60, AddedAt: 1, FileHash: "fixture-" + id}))
	}
	must(database.SavePlaylist(&db.Playlist{ID: "ordered", Name: "Original playlist", SongIDs: []string{"b", "a", "b"}, CreatedAt: 1}))
	_, err = database.SetLike("a", true)
	must(err)
	must(database.UpdatePlayCount("a"))
	must(database.RecordListeningEvent(db.ListeningEvent{SongID: "a", EventType: "play_complete", PlayDuration: 60, SongDuration: 60, Genre: "Fixture", Mood: "calm", Energy: "low", Context: "playlist"}))
	must(database.UpdateGenrePreferences("Fixture"))
	for key, value := range map[string]string{"theme": "dark", "fixture_volume": "0.4", "spotify_webplayer_session": "synthetic-cookie", "spotify_credentials": "synthetic-token"} {
		must(database.SetSetting(key, value))
	}
	before := readRestoreRelations(t, databasePath)
	if len(before["catalog"]) != 2 || len(before["playlists"]) != 1 || len(before["likes"]) != 1 || len(before["listening_events"]) != 1 || len(before["genre_preferences"]) != 1 || len(before["settings"]) != 2 {
		t.Fatal("incomplete pre-backup relation fixture")
	}
	a := &API{db: database, dataDir: dataDir}
	router := a.V2LibraryOperationRoutes()
	post := func(path, body string, status int) *httptest.ResponseRecorder {
		t.Helper()
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, path, strings.NewReader(body)))
		if response.Code != status {
			t.Fatalf("POST %s: %d %s", path, response.Code, response.Body.String())
		}
		return response
	}
	created := post("/backups", `{"name":"continuity-fixture.zip"}`, http.StatusCreated)
	var backup backupInfo
	must(json.Unmarshal(created.Body.Bytes(), &backup))
	if backup.Name != "continuity-fixture.zip" || backup.Size == 0 {
		t.Fatalf("unexpected backup: %+v", backup)
	}
	assertRestoreCredentialCount(t, databasePath, 2) // Export must not log out the live app.

	// Every checked relation diverges after the recovery point is created.
	song, err := database.GetSongByID("a")
	must(err)
	song.Title = "Edited after backup"
	must(database.SaveSong(song))
	must(database.SaveSong(&db.Song{ID: "c", Title: "Added after backup", Artist: "Fixture artist", Album: "Fixture album", FilePath: filepath.Join(mediaDir, "c.mp3"), Duration: 30, AddedAt: 2, FileHash: "fixture-c"}))
	must(database.SavePlaylist(&db.Playlist{ID: "ordered", Name: "Changed playlist", SongIDs: []string{"c", "a"}, CreatedAt: 1}))
	_, err = database.SetLike("a", false)
	must(err)
	_, err = database.SetLike("c", true)
	must(err)
	must(database.UpdatePlayCount("c"))
	must(database.RecordListeningEvent(db.ListeningEvent{SongID: "c", EventType: "skip_early", PlayDuration: 2, SongDuration: 30, Genre: "Fixture", Context: "queue"}))
	must(database.UpdateGenrePreferences("Fixture"))
	must(database.SetSetting("theme", "light"))
	must(database.SetSetting("fixture_volume", "0.9"))
	after := readRestoreRelations(t, databasePath)
	for relation, original := range before {
		if reflect.DeepEqual(original, after[relation]) {
			t.Fatalf("fixture did not change relation %s", relation)
		}
	}
	stageBody, err := json.Marshal(map[string]string{"name": backup.Name})
	must(err)
	post("/restore/preview", string(stageBody), http.StatusOK)
	staged := post("/restore/stage", string(stageBody), http.StatusAccepted)
	var result struct{ Staged, RestartRequired bool }
	must(json.Unmarshal(staged.Body.Bytes(), &result))
	if !result.Staged || !result.RestartRequired {
		t.Fatal("restore was not staged for offline activation")
	}
	assertRestoreRelations(t, "live database before activation", readRestoreRelations(t, databasePath), after)
	assertRestoreCredentialCount(t, filepath.Join(dataDir, "restore-pending", "library.db"), 0)
	must(database.Close()) // Release the application lock before offline activation.
	applied, rollback, err := db.ApplyPendingRestore(dataDir)
	must(err)
	if !applied || rollback == "" {
		t.Fatalf("restore: applied=%v rollback=%q", applied, rollback)
	}
	must(db.ValidateSQLiteCopy(databasePath))
	must(db.ValidateSQLiteCopy(rollback))
	reopened, err := db.New(databasePath)
	must(err)
	defer reopened.Close()
	assertRestoreRelations(t, "restored database", readRestoreRelations(t, databasePath), before)
	assertRestoreRelations(t, "rollback database", readRestoreRelations(t, rollback), after)
	assertRestoreCredentialCount(t, databasePath, 0)
	assertRestoreCredentialCount(t, rollback, 2)
	if _, err := os.Stat(filepath.Join(dataDir, "restore-pending")); !os.IsNotExist(err) {
		t.Fatalf("pending restore not cleaned: %v", err)
	}
	for _, id := range []string{"a", "b"} {
		retained, err := os.ReadFile(filepath.Join(mediaDir, id+".mp3"))
		must(err)
		if !bytes.Equal(retained, media) {
			t.Fatal("restore changed original media bytes")
		}
	}
}

// Direct SQL independently checks exact persisted rows, including detailed
// history (which has no exported read API), timestamps and playlist JSON order.
func readRestoreRelations(t *testing.T, path string) map[string][][]any {
	t.Helper()
	connection, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	result := map[string][][]any{}
	for name, query := range map[string]string{
		"catalog":           "SELECT * FROM songs ORDER BY id",
		"playlists":         "SELECT * FROM playlists ORDER BY id",
		"likes":             "SELECT id, liked_at FROM songs WHERE liked=1 ORDER BY id",
		"listening_events":  "SELECT * FROM listening_events ORDER BY id",
		"genre_preferences": "SELECT * FROM genre_preferences ORDER BY genre",
		"settings":          "SELECT key, value FROM settings WHERE key IN ('theme','fixture_volume') ORDER BY key",
	} {
		rows, err := connection.Query(query)
		if err != nil {
			t.Fatal(err)
		}
		columns, err := rows.Columns()
		if err != nil {
			rows.Close()
			t.Fatal(err)
		}
		for rows.Next() {
			values := make([]any, len(columns))
			pointers := make([]any, len(columns))
			for i := range values {
				pointers[i] = &values[i]
			}
			if err := rows.Scan(pointers...); err != nil {
				rows.Close()
				t.Fatal(err)
			}
			result[name] = append(result[name], values)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			t.Fatal(err)
		}
	}
	return result
}

func assertRestoreRelations(t *testing.T, scope string, got, want map[string][][]any) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		for relation := range want {
			if !reflect.DeepEqual(got[relation], want[relation]) {
				t.Errorf("%s lost or changed %s", scope, relation)
			}
		}
	}
}

func assertRestoreCredentialCount(t *testing.T, path string, expected int) {
	t.Helper()
	connection, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	var count int
	if err := connection.QueryRow("SELECT count(*) FROM settings WHERE key IN ('spotify_webplayer_session','spotify_credentials')").Scan(&count); err != nil || count != expected {
		t.Fatalf("credential row count: got=%d want=%d err=%v", count, expected, err)
	}
}
