package api

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCoverThumbnailPreservesAspectRatioAndRefreshesOnFileChange(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cover.png")
	write := func(value color.RGBA, modified time.Time) {
		t.Helper()
		pixels := image.NewRGBA(image.Rect(0, 0, 240, 120))
		for y := 0; y < 120; y++ {
			for x := 0; x < 240; x++ {
				pixels.SetRGBA(x, y, value)
			}
		}
		var data bytes.Buffer
		if err := png.Encode(&data, pixels); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data.Bytes(), 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(path, modified, modified); err != nil {
			t.Fatal(err)
		}
	}

	api := &API{}
	read := func(query string) *httptest.ResponseRecorder {
		t.Helper()
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		w := httptest.NewRecorder()
		api.serveCoverFile(w, httptest.NewRequest(http.MethodGet, "/api/cover/example"+query, nil), path, info)
		if w.Code != http.StatusOK {
			t.Fatalf("got HTTP %d", w.Code)
		}
		return w
	}
	start := time.Now().Add(-time.Minute)
	write(color.RGBA{R: 220, A: 255}, start)
	first := read("?size=96")
	if first.Header().Get("Content-Type") != "image/jpeg" {
		t.Fatalf("thumbnail content type: %q", first.Header().Get("Content-Type"))
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(first.Body.Bytes()))
	if err != nil || cfg.Width != 96 || cfg.Height != 48 {
		t.Fatalf("thumbnail size: %dx%d (%v)", cfg.Width, cfg.Height, err)
	}
	if original := read(""); original.Header().Get("Content-Type") != "image/png" {
		t.Fatalf("original content type: %q", original.Header().Get("Content-Type"))
	}
	if cached := read("?size=96"); !bytes.Equal(first.Body.Bytes(), cached.Body.Bytes()) {
		t.Fatal("expected stable thumbnail bytes")
	}
	write(color.RGBA{B: 220, A: 255}, start.Add(time.Second))
	if changed := read("?size=96"); bytes.Equal(first.Body.Bytes(), changed.Body.Bytes()) {
		t.Fatal("thumbnail did not reflect the updated artwork")
	}
}
