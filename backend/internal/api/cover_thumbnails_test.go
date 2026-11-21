// Tests thumbnail size, caching, and refresh when source artwork changes.
package api

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/ajbergh/viib-mediahub/internal/db"
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
	if cc := first.Header().Get("Cache-Control"); cc != "private, max-age=86400" {
		t.Fatalf("thumbnail Cache-Control: %q", cc)
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

func TestCoverThumbnailCacheLRUEviction(t *testing.T) {
	var cache coverThumbnailCache
	key := func(name string) coverThumbnailKey {
		return coverThumbnailKey{path: name, size: 96}
	}

	// Item-count bound: touch A so B becomes least recently used.
	cache.mu.Lock()
	cache.addWithBoundsLocked(key("a"), coverThumbnail{data: []byte{1, 2, 3}}, 3, 1024)
	cache.addWithBoundsLocked(key("b"), coverThumbnail{data: []byte{4, 5, 6}}, 3, 1024)
	cache.addWithBoundsLocked(key("c"), coverThumbnail{data: []byte{7, 8, 9}}, 3, 1024)
	cache.mu.Unlock()

	if _, ok := cache.get(key("a")); !ok {
		t.Fatal("expected key a in cache")
	}

	cache.mu.Lock()
	cache.addWithBoundsLocked(key("d"), coverThumbnail{data: []byte{10, 11, 12}}, 3, 1024)
	cache.mu.Unlock()

	if _, ok := cache.get(key("b")); ok {
		t.Fatal("expected least-recently-used key b to be evicted")
	}
	for _, kept := range []string{"a", "c", "d"} {
		if _, ok := cache.get(key(kept)); !ok {
			t.Fatalf("expected key %s to remain in cache", kept)
		}
	}

	// Byte-budget bound: order is now a, c, d (d most recent). Touch a so c becomes LRU.
	if _, ok := cache.get(key("a")); !ok {
		t.Fatal("expected key a in cache")
	}
	cache.mu.Lock()
	cache.addWithBoundsLocked(key("e"), coverThumbnail{data: make([]byte, 5)}, 10, 10)
	gotBytes := cache.bytes
	cache.mu.Unlock()

	if gotBytes > 10 {
		t.Fatalf("cache bytes exceeded budget: %d > 10", gotBytes)
	}
	if _, ok := cache.get(key("c")); ok {
		t.Fatal("expected key c to be evicted under byte budget")
	}
	if _, ok := cache.get(key("d")); ok {
		t.Fatal("expected key d to be evicted under byte budget")
	}
	if _, ok := cache.get(key("a")); !ok {
		t.Fatal("expected recently touched key a to survive byte eviction")
	}
	if _, ok := cache.get(key("e")); !ok {
		t.Fatal("expected newly added key e in cache")
	}
}

func TestCoverThumbnailFastPathMatchesGeneric(t *testing.T) {
	referenceResize := func(src image.Image, size int) *image.RGBA {
		bounds := src.Bounds()
		width, height := bounds.Dx(), bounds.Dy()
		longSide := max(width, height)
		scaledWidth, scaledHeight := max(1, width*size/longSide), max(1, height*size/longSide)
		dst := image.NewRGBA(image.Rect(0, 0, scaledWidth, scaledHeight))
		for y := 0; y < scaledHeight; y++ {
			for x := 0; x < scaledWidth; x++ {
				var red, green, blue, alpha uint32
				for dy := 0; dy < 2; dy++ {
					sy := bounds.Min.Y + (4*y+2*dy+1)*height/(4*scaledHeight)
					for dx := 0; dx < 2; dx++ {
						sx := bounds.Min.X + (4*x+2*dx+1)*width/(4*scaledWidth)
						r, g, b, a := src.At(sx, sy).RGBA()
						red, green, blue, alpha = red+r, green+g, blue+b, alpha+a
					}
				}
				dst.SetRGBA(x, y, color.RGBA{R: uint8(red >> 10), G: uint8(green >> 10), B: uint8(blue >> 10), A: uint8(alpha >> 10)})
			}
		}
		return dst
	}

	assertMatches := func(t *testing.T, name string, src image.Image, size int) {
		t.Helper()
		got := resizeCoverThumbnail(src, size)
		want := referenceResize(src, size)
		if !got.Bounds().Eq(want.Bounds()) {
			t.Fatalf("%s: bounds %v != %v", name, got.Bounds(), want.Bounds())
		}
		if !bytes.Equal(got.Pix, want.Pix) {
			for y := 0; y < want.Bounds().Dy(); y++ {
				for x := 0; x < want.Bounds().Dx(); x++ {
					g := got.RGBAAt(x, y)
					w := want.RGBAAt(x, y)
					if g != w {
						t.Fatalf("%s: pixel (%d,%d) got %+v, want %+v", name, x, y, g, w)
					}
				}
			}
		}
	}

	rect := image.Rect(5, 7, 305, 224)

	rgba := image.NewRGBA(rect)
	for y := rect.Min.Y; y < rect.Max.Y; y++ {
		for x := rect.Min.X; x < rect.Max.X; x++ {
			rgba.SetRGBA(x, y, color.RGBA{
				R: uint8((x*7 + y*3) & 0xff),
				G: uint8((x*11 + y*5) & 0xff),
				B: uint8((x*13 + y*17) & 0xff),
				A: uint8(60 + (x+y)%196),
			})
		}
	}
	assertMatches(t, "RGBA", rgba, 96)

	nrgba := image.NewNRGBA(rect)
	for y := rect.Min.Y; y < rect.Max.Y; y++ {
		for x := rect.Min.X; x < rect.Max.X; x++ {
			var a uint8
			switch (x + y) % 5 {
			case 0:
				a = 0
			case 1:
				a = 255
			default:
				a = uint8(1 + (x*19+y*23)%254)
			}
			nrgba.SetNRGBA(x, y, color.NRGBA{
				R: uint8((x*3 + y*9) & 0xff),
				G: uint8((x*5 + y*13) & 0xff),
				B: uint8((x*7 + y*11) & 0xff),
				A: a,
			})
		}
	}
	assertMatches(t, "NRGBA", nrgba, 96)

	ratios := []struct {
		name  string
		ratio image.YCbCrSubsampleRatio
	}{
		{"YCbCr444", image.YCbCrSubsampleRatio444},
		{"YCbCr422", image.YCbCrSubsampleRatio422},
		{"YCbCr420", image.YCbCrSubsampleRatio420},
		{"YCbCr440", image.YCbCrSubsampleRatio440},
		{"YCbCr411", image.YCbCrSubsampleRatio411},
		{"YCbCr410", image.YCbCrSubsampleRatio410},
	}
	for _, tc := range ratios {
		ycc := image.NewYCbCr(rect, tc.ratio)
		for i := range ycc.Y {
			ycc.Y[i] = uint8((i * 37) & 0xff)
		}
		for i := range ycc.Cb {
			ycc.Cb[i] = uint8((i*53 + 17) & 0xff)
			ycc.Cr[i] = uint8((i*71 + 89) & 0xff)
		}
		assertMatches(t, tc.name, ycc, 96)
	}
}

func TestCoverThumbnailConcurrentDecode(t *testing.T) {
	dir := t.TempDir()
	paths := make([]string, 6)
	for i := range paths {
		paths[i] = filepath.Join(dir, fmt.Sprintf("cover-%d.jpg", i))
		img := image.NewRGBA(image.Rect(0, 0, 320, 320))
		for y := 0; y < 320; y++ {
			for x := 0; x < 320; x++ {
				img.SetRGBA(x, y, color.RGBA{R: uint8((x + i*20) & 0xff), G: uint8(y & 0xff), B: 120, A: 255})
			}
		}
		var buf bytes.Buffer
		if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 85}); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(paths[i], buf.Bytes(), 0600); err != nil {
			t.Fatal(err)
		}
	}

	api := &API{}
	var wg sync.WaitGroup
	errs := make(chan error, len(paths)*2)
	for round := 0; round < 2; round++ {
		for _, p := range paths {
			wg.Add(1)
			go func(path string) {
				defer wg.Done()
				info, err := os.Stat(path)
				if err != nil {
					errs <- err
					return
				}
				w := httptest.NewRecorder()
				api.serveCoverFile(w, httptest.NewRequest(http.MethodGet, "/api/cover/x?size=96", nil), path, info)
				if w.Code != http.StatusOK {
					errs <- fmt.Errorf("status %d for %s", w.Code, path)
				}
			}(p)
		}
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	wantSlots := min(4, max(2, runtime.NumCPU()/2))
	if cap(api.coverThumbnails.decode) != wantSlots {
		t.Fatalf("decode semaphore capacity: got %d, want %d", cap(api.coverThumbnails.decode), wantSlots)
	}
}

func TestSaveSongsWithResultBatchReconcilesOnlyMatchingDownloads(t *testing.T) {
	dir := t.TempDir()
	database, err := db.New(filepath.Join(dir, "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	matchedPath := filepath.Join(dir, "matched.mp3")
	plainPath := filepath.Join(dir, "plain.mp3")
	if err := os.WriteFile(matchedPath, []byte("downloaded audio bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(plainPath, []byte("local audio bytes"), 0600); err != nil {
		t.Fatal(err)
	}

	const recordingID = "4uLU6hMCjMI75M1A2tKUQC"
	if err := database.AddDownload(&db.SpotifyDownload{
		ID: "dl-1", SpotifyID: recordingID, Type: "track", Title: "Matched", Status: "queued", AddedAt: 1,
	}); err != nil {
		t.Fatal(err)
	}
	if changed, err := database.MarkDownloadStarted("dl-1"); err != nil || !changed {
		t.Fatalf("mark started: %v %v", changed, err)
	}
	if changed, err := database.MarkDownloadCompletedWithEvidence(context.Background(), "dl-1", matchedPath); err != nil || !changed {
		t.Fatalf("complete with evidence: %v %v", changed, err)
	}

	res, err := database.SaveSongsWithResult([]db.Song{
		{ID: "s1", FilePath: matchedPath, Title: "Matched", Artist: "Artist", Album: "Album", FileHash: "h1", AddedAt: 1},
		{ID: "s2", FilePath: plainPath, Title: "Plain", Artist: "Artist", Album: "Album", FileHash: "h2", AddedAt: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Inserted != 2 || res.Updated != 0 {
		t.Fatalf("unexpected upsert result: %+v", res)
	}

	info, err := os.Stat(matchedPath)
	if err != nil {
		t.Fatal(err)
	}
	fp := db.LocalSourceFingerprint(db.Song{FilePath: matchedPath, FileHash: "h1"}, info)
	link, err := database.GetSpotifyRecording("s1", fp)
	if err != nil || link == nil || link.ExternalID != recordingID {
		t.Fatalf("expected reconciled spotify recording for matched song: %#v %v", link, err)
	}

	infoPlain, err := os.Stat(plainPath)
	if err != nil {
		t.Fatal(err)
	}
	fpPlain := db.LocalSourceFingerprint(db.Song{FilePath: plainPath, FileHash: "h2"}, infoPlain)
	plainLink, err := database.GetSpotifyRecording("s2", fpPlain)
	if err != nil || plainLink != nil {
		t.Fatalf("expected no spotify link for plain song: %#v %v", plainLink, err)
	}
}

func TestGenreStatsUpdatePreservesTopArtistsAndHandlesEscapes(t *testing.T) {
	dir := t.TempDir()
	database, err := db.New(filepath.Join(dir, "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	// Empty library should succeed without error and return no genre stats.
	if err := database.UpdateGenreStats(); err != nil {
		t.Fatalf("empty UpdateGenreStats: %v", err)
	}
	stats, err := database.GetAllGenreStats()
	if err != nil || len(stats) != 0 {
		t.Fatalf("expected empty genre stats, got %+v (%v)", stats, err)
	}

	songs := []db.Song{
		{ID: "1", FilePath: filepath.Join(dir, "1.mp3"), Title: "T1", Artist: "Artist A", Album: "Al", Genre: []string{"Rock", "Indie"}, CoverPath: "c1.jpg", AddedAt: 1},
		{ID: "2", FilePath: filepath.Join(dir, "2.mp3"), Title: "T2", Artist: "Artist A", Album: "Al", Genre: []string{"Rock"}, CoverPath: "c2.jpg", AddedAt: 1},
		{ID: "3", FilePath: filepath.Join(dir, "3.mp3"), Title: "T3", Artist: "Artist B", Album: "Al", Genre: []string{"Rock", `Rock"n"Roll`}, CoverPath: "c3.jpg", AddedAt: 1},
	}
	if err := database.SaveSongs(songs); err != nil {
		t.Fatal(err)
	}
	if err := database.UpdateGenreStats(); err != nil {
		t.Fatalf("UpdateGenreStats: %v", err)
	}
	stats, err = database.GetAllGenreStats()
	if err != nil {
		t.Fatal(err)
	}
	if len(stats) != 3 || stats[0].Name != "Rock" || stats[0].Count != 3 || stats[0].TopArtists[0] != "Artist A" || stats[0].CoverUrl != "/api/cover/1" {
		t.Fatalf("unexpected genre stats: %+v", stats)
	}
}

func BenchmarkMakeCoverThumbnail(b *testing.B) {
	path := filepath.Join(b.TempDir(), "large.jpg")
	img := image.NewRGBA(image.Rect(0, 0, 1200, 1200))
	for y := 0; y < 1200; y++ {
		for x := 0; x < 1200; x++ {
			img.SetRGBA(x, y, color.RGBA{
				R: uint8((x * 3) & 0xff),
				G: uint8((y * 5) & 0xff),
				B: uint8(((x + y) * 7) & 0xff),
				A: 255,
			})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}); err != nil {
		b.Fatal(err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0600); err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		thumb := makeCoverThumbnail(path, 256)
		if len(thumb.data) == 0 {
			b.Fatal("expected non-empty thumbnail")
		}
	}
}

func BenchmarkSaveSongsWithResultNoDownloads(b *testing.B) {
	dir := b.TempDir()
	database, err := db.New(filepath.Join(dir, "bench.db"))
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { database.Close() })

	songs := make([]db.Song, 100)
	for i := range songs {
		songs[i] = db.Song{
			ID:       fmt.Sprintf("song-%03d", i),
			FilePath: filepath.Join(dir, fmt.Sprintf("track-%03d.mp3", i)),
			Title:    fmt.Sprintf("Track %d", i),
			Artist:   "Artist",
			Album:    "Album",
			Genre:    []string{"Electronic", "Ambient"},
			FileHash: fmt.Sprintf("hash-%03d", i),
			AddedAt:  1,
		}
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := database.SaveSongsWithResult(songs); err != nil {
			b.Fatal(err)
		}
	}
}
