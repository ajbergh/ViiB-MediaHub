package analysis

import (
	"context"
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

func TestStreamLocalMonoDecodesAndDownmixesCanonicalSong(t *testing.T) {
	database, err := db.New(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	path := filepath.Join(t.TempDir(), "track.wav")
	if err := os.WriteFile(path, makePCM16WAV(t, 2, 22050, []int16{32767, -32768, 16384, 16384}), 0600); err != nil {
		t.Fatal(err)
	}
	if err := database.SaveSong(&db.Song{ID: "song", Title: "Song", Artist: "Artist", Album: "Album", FilePath: path, AddedAt: 1}); err != nil {
		t.Fatal(err)
	}
	var received []float32
	source, err := StreamLocalMono(context.Background(), database, NewDefaultDecoderRegistry(), "song", func(chunk MonoChunk) error {
		received = append(received, chunk.Samples...)
		if chunk.SampleRate != 22050 {
			t.Fatalf("sample rate = %d", chunk.SampleRate)
		}
		return nil
	})
	if err != nil || source.SongID != "song" || len(received) != 2 || received[0] > .001 || received[0] < -.001 || received[1] != .5 {
		t.Fatalf("stream = %#v, samples = %#v, err = %v", source, received, err)
	}
}
