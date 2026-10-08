package api

import (
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/ajbergh/viib-mediahub/internal/db"
)

// Opt-in: requires a built frontend served at VIIB_BROWSER_PREVIEW_URL and
// installed Playwright Chromium. Library/audio/analysis remain deterministic
// fixtures; playlist POST and GET use the real API and temporary SQLite DB.
func TestPlaylistBrowserPersistentReadback(t *testing.T) {
	preview := os.Getenv("VIIB_BROWSER_PREVIEW_URL")
	if preview == "" {
		t.Skip("set VIIB_BROWSER_PREVIEW_URL to run browser persistence audit")
	}
	a, _, _ := fixtureCookieRuntime(t)
	for i := 0; i < 4; i++ {
		id := []string{"score-0", "score-1", "score-2", "score-3"}[i]
		if err := a.db.SaveSong(&db.Song{ID: id, FilePath: filepath.Join(t.TempDir(), id+".wav"), Title: id, Artist: "Fixture", Album: "Score audit", AddedAt: 1}); err != nil {
			t.Fatal(err)
		}
	}
	server := httptest.NewServer(a.Routes())
	defer server.Close()
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(t.Context(), "node", filepath.Join(root, "scripts", "dj-metadata-playlist-audit.mjs"))
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "DJ_AUDIT_MIX_PLAYLIST=1", "DJ_AUDIT_WIDTH=1470", "DJ_AUDIT_HEIGHT=825", "DJ_AUDIT_URL="+preview+"/dj", "DJ_AUDIT_PLAYLIST_BACKEND="+server.URL+"/playlists")
	output, err := cmd.CombinedOutput()
	t.Log(string(output))
	if err != nil {
		t.Fatal(err)
	}
	playlists, err := a.db.GetAllPlaylists()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, p := range playlists {
		if p.Name == "Verified retry mix" {
			found = true
			if len(p.SongIDs) != 2 || p.SongIDs[0] != "score-0" || p.SongIDs[1] != "score-3" {
				t.Fatalf("persisted order: %+v", p)
			}
		}
	}
	if !found {
		t.Fatal("browser-created playlist missing from SQLite")
	}
}
