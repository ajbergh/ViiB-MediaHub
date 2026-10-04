package spotify

import (
	"go.senan.xyz/taglib"
	"os"
	"path/filepath"
	"testing"
)

func TestDownloadOggMetadataRoundTripsSpotifyScalars(t *testing.T) {
	// The small Ogg fixture comes from oggvorbis v1.0.5; see testdata/OGGVORBIS_LICENSE.
	data, err := os.ReadFile("testdata/metadata.ogg")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "track.ogg")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	bpm := 109.724
	d := &Downloader{}
	if err := d.writeOggMetadata(path, "Artist", "Title", "Album", &DownloadMetadata{BPM: &bpm, InitialKey: "Am"}); err != nil {
		t.Fatal(err)
	}
	tags, err := taglib.ReadTags(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(tags["BPM"]) != 1 || tags["BPM"][0] != "109.724" || len(tags["INITIALKEY"]) != 1 || tags["INITIALKEY"][0] != "Am" {
		t.Fatalf("tags: %+v", tags)
	}
}
