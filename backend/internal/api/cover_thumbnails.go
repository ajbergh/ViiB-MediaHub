package api

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"net/http"
	"os"
	"sync"
	"time"
)

const (
	thumbnailCacheBytes = 24 << 20
	thumbnailCacheItems = 256
)

type coverThumbnailKey struct {
	path     string
	modified time.Time
	bytes    int64
	size     int
}

type coverThumbnail struct {
	data     []byte
	mimeType string
	etag     string
}

// One decoder limits transient memory on busy artwork grids. Cached reads do
// not wait for decoding, and cancelled requests leave the queue immediately.
type coverThumbnailCache struct {
	init   sync.Once
	decode chan struct{}
	mu     sync.Mutex
	items  map[coverThumbnailKey]coverThumbnail
	order  []coverThumbnailKey
	bytes  int
}

func (c *coverThumbnailCache) get(key coverThumbnailKey) (coverThumbnail, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	item, ok := c.items[key]
	return item, ok
}

func (c *coverThumbnailCache) add(key coverThumbnailKey, item coverThumbnail) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.items == nil {
		c.items = make(map[coverThumbnailKey]coverThumbnail)
	}
	for len(c.order) > 0 && (len(c.order) >= thumbnailCacheItems || c.bytes+len(item.data) > thumbnailCacheBytes) {
		oldest := c.order[0]
		c.order[0] = coverThumbnailKey{}
		c.order = c.order[1:]
		c.bytes -= len(c.items[oldest].data)
		delete(c.items, oldest)
	}
	c.order = append(c.order, key)
	c.items[key] = item
	c.bytes += len(item.data)
}

func (a *API) serveCoverFile(w http.ResponseWriter, r *http.Request, path string, info os.FileInfo) {
	var size int
	switch r.URL.Query().Get("size") {
	case "96":
		size = 96
	case "256":
		size = 256
	case "512":
		size = 512
	}
	if size == 0 || !info.Mode().IsRegular() || info.Size() > 16<<20 {
		http.ServeFile(w, r, path)
		return
	}

	key := coverThumbnailKey{path: path, modified: info.ModTime(), bytes: info.Size(), size: size}
	cache := &a.coverThumbnails
	item, ok := cache.get(key)
	if !ok {
		cache.init.Do(func() { cache.decode = make(chan struct{}, 1) })
		select {
		case cache.decode <- struct{}{}:
		case <-r.Context().Done():
			return
		}
		item, ok = cache.get(key)
		if !ok && r.Context().Err() == nil {
			item = makeCoverThumbnail(path, size)
			cache.add(key, item)
		}
		<-cache.decode
	}
	if r.Context().Err() != nil {
		return
	}
	if len(item.data) == 0 {
		http.ServeFile(w, r, path)
		return
	}
	w.Header().Set("Content-Type", item.mimeType)
	w.Header().Set("ETag", item.etag)
	http.ServeContent(w, r, "", info.ModTime(), bytes.NewReader(item.data))
}

func makeCoverThumbnail(path string, size int) coverThumbnail {
	file, err := os.Open(path)
	if err != nil {
		return coverThumbnail{}
	}
	defer file.Close()
	config, _, err := image.DecodeConfig(file)
	if err != nil || config.Width <= 0 || config.Height <= 0 || config.Width > 16_000_000/config.Height {
		return coverThumbnail{}
	}
	if max(config.Width, config.Height) <= size {
		return coverThumbnail{}
	}
	if _, err := file.Seek(0, 0); err != nil {
		return coverThumbnail{}
	}
	src, _, err := image.Decode(file)
	if err != nil {
		return coverThumbnail{}
	}

	bounds := src.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	longSide := max(width, height)
	scaledWidth, scaledHeight := max(1, width*size/longSide), max(1, height*size/longSide)
	dst := image.NewRGBA(image.Rect(0, 0, scaledWidth, scaledHeight))
	// Sample across each output pixel to reduce aliasing on large covers.
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

	var encoded bytes.Buffer
	mimeType := "image/jpeg"
	if dst.Opaque() {
		err = jpeg.Encode(&encoded, dst, &jpeg.Options{Quality: 85})
	} else {
		mimeType = "image/png"
		err = png.Encode(&encoded, dst)
	}
	if err != nil {
		return coverThumbnail{}
	}
	data := encoded.Bytes()
	digest := sha256.Sum256(data)
	return coverThumbnail{data: data, mimeType: mimeType, etag: fmt.Sprintf(`"%x"`, digest[:16])}
}
