// Verifies the seekable playback fallback with a local Vorbis fixture.
package spotify

import (
	"context"
	"io"
	"os"
	"testing"

	"github.com/hajimehoshi/go-mp3"
)

func TestPrepareMP3StreamAsset(t *testing.T) {
	source, err := os.Open("testdata/metadata.ogg")
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	asset, err := prepareMP3StreamAsset(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}
	defer asset.close()
	reader, err := asset.newReader()
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	if asset.contentType != "audio/mpeg" || asset.size <= 0 {
		t.Fatalf("unexpected media asset: %s, %d bytes", asset.contentType, asset.size)
	}
	end, err := reader.Seek(0, io.SeekEnd)
	if err != nil || end != asset.size {
		t.Fatalf("seek end = %d, %v, want %d", end, err, asset.size)
	}
	if _, err := reader.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	decoder, err := mp3.NewDecoder(reader)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.ReadFull(decoder, make([]byte, 4096)); err != nil {
		t.Fatalf("cannot decode compatibility stream: %v", err)
	}
}
