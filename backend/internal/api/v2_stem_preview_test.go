package api

import (
	"encoding/binary"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ajbergh/viib-mediahub/internal/stems"
)

func TestV2StemPreviewStreamsAllowlistedWAVWithByteRanges(t *testing.T) {
	f := newStemFrameFixture(t, stems.LayoutFour)
	req := httptest.NewRequest(http.MethodGet, "/song/set/vocals", nil)
	req.Header.Set("Range", "bytes=44-47")
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)
	if rec.Code != http.StatusPartialContent {
		t.Fatalf("range status=%d body=%s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Content-Type") != "audio/wav" || rec.Header().Get("Content-Range") != "bytes 44-47/56" {
		t.Fatalf("unexpected range headers %v", rec.Header())
	}
	if rec.Body.Len() != 4 {
		t.Fatalf("range bytes=%d, want 4", rec.Body.Len())
	}
	full := f.request("/song/set/frames?startFrame=0&frameCount=1&layout=dj4&format=f32le") // verifies the same package remains frame-readable after preview.
	if full.Code != http.StatusOK {
		t.Fatalf("frame API status after preview=%d", full.Code)
	}
	for _, name := range []string{"drums", "bass", "other"} {
		r := f.request("/song/set/" + name)
		if r.Code != http.StatusOK {
			t.Errorf("allowlisted %s status=%d", name, r.Code)
		}
	}
	for _, name := range []string{"guitar", "piano", "not-a-stem", ".."} {
		r := f.request("/song/set/" + name)
		if r.Code == http.StatusOK {
			t.Errorf("unexpected artifact %q was served", name)
		}
	}
}

func TestV2StemPreviewEnforcesSongOwnershipAndReadyState(t *testing.T) {
	f := newStemFrameFixture(t, stems.LayoutFour)
	for _, tc := range []struct {
		url    string
		status int
	}{{"/other/set/vocals", http.StatusNotFound}, {"/song/missing/vocals", http.StatusNotFound}} {
		r := f.request(tc.url)
		if r.Code != tc.status {
			t.Errorf("GET %s=%d, want %d", tc.url, r.Code, tc.status)
		}
	}
	sets, err := f.database.ListStemSets("song")
	if err != nil {
		t.Fatal(err)
	}
	sets[0].Status = "stale"
	if err = f.database.UpsertStemSet(sets[0]); err != nil {
		t.Fatal(err)
	}
	r := f.request("/song/set/vocals")
	if r.Code != http.StatusConflict {
		t.Fatalf("non-ready package status=%d, want 409", r.Code)
	}
}

func TestV2StemPreviewRejectsCurrentPackageChecksumAndTraversalFailures(t *testing.T) {
	t.Run("changed artifact", func(t *testing.T) {
		f := newStemFrameFixture(t, stems.LayoutFour)
		warm := f.request("/song/set/vocals")
		if warm.Code != http.StatusOK {
			t.Fatalf("warm request status=%d", warm.Code)
		}
		path := filepath.Join(f.packagePath, "vocals.wav")
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		data[len(data)-1] ^= 1
		if err = os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
		r := f.request("/song/set/vocals")
		if r.Code != http.StatusConflict {
			t.Fatalf("changed artifact status=%d, want 409", r.Code)
		}
	})
	t.Run("manifest traversal", func(t *testing.T) {
		f := newStemFrameFixture(t, stems.LayoutFour)
		path := filepath.Join(f.packagePath, stems.ManifestFilename)
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		changed := strings.Replace(string(data), `"path":"vocals.wav"`, `"path":"../outside.wav"`, 1)
		if changed == string(data) {
			t.Fatal("fixture artifact path was not replaced")
		}
		if err = os.WriteFile(path, []byte(changed), 0600); err != nil {
			t.Fatal(err)
		}
		r := f.request("/song/set/vocals")
		if r.Code != http.StatusConflict {
			t.Fatalf("traversal manifest status=%d, want 409", r.Code)
		}
	})
}

func TestV2StemPreviewAcceptsRetaggedSourceWithMatchingPCM32Identity(t *testing.T) {
	f := newStemFrameFixture(t, stems.LayoutFour)
	song, err := f.database.GetSongByID("song")
	if err != nil {
		t.Fatal(err)
	}
	source, err := os.ReadFile(song.FilePath)
	if err != nil {
		t.Fatal(err)
	}
	retagged := append([]byte(nil), source...)
	retagged = append(retagged, []byte("JUNK")...)
	retagged = append(retagged, 0, 0, 0, 0)
	binary.LittleEndian.PutUint32(retagged[4:8], uint32(len(retagged)-8))
	if err = os.WriteFile(song.FilePath, retagged, 0600); err != nil {
		t.Fatal(err)
	}
	r := f.request("/song/set/vocals")
	if r.Code != http.StatusOK {
		t.Fatalf("retagged source status=%d body=%s", r.Code, r.Body.String())
	}
}
