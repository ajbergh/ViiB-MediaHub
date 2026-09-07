package analysis

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ajbergh/viib-mediahub/internal/db"
)

func TestResolveLocalSourceUsesSongIdentityAndCurrentFileRevision(t *testing.T) {
	database, err := db.New(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	path := filepath.Join(t.TempDir(), "track.wav")
	if err := os.WriteFile(path, []byte("pcm"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := database.SaveSong(&db.Song{ID: "song", Title: "Song", Artist: "Artist", Album: "Album", FilePath: path, FileHash: "catalog-hash", AddedAt: 1}); err != nil {
		t.Fatal(err)
	}
	source, err := ResolveLocalSource(database, "song")
	if err != nil || source.Size != 3 || !strings.HasPrefix(source.Fingerprint, "catalog-hash:3:") {
		t.Fatalf("source = %#v, %v", source, err)
	}
	stream, err := source.Open()
	if err != nil {
		t.Fatal(err)
	}
	stream.Close()
}
