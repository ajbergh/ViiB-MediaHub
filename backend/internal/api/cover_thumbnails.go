package api

import (
	"bytes"
	"container/list"
	"crypto/sha256"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"net/http"
	"os"
	"runtime"
	"sync"
	"time"
)

const (
	thumbnailCacheBytes = 32 << 20
	thumbnailCacheItems = 512
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

type coverThumbnailEntry struct {
	key  coverThumbnailKey
	item coverThumbnail
}

// Bounded concurrent decoders prevent large artwork grids from serializing
// behind a single decoder while capping transient decode memory. Cached reads
// do not wait for decoding, and cancelled requests leave the queue immediately.
type coverThumbnailCache struct {
	init   sync.Once
	decode chan struct{}
	mu     sync.Mutex
	items  map[coverThumbnailKey]*list.Element
	order  *list.List // front is most recently used, back is least recently used
	bytes  int
}

func (c *coverThumbnailCache) get(key coverThumbnailKey) (coverThumbnail, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.items == nil {
		return coverThumbnail{}, false
	}
	elem, ok := c.items[key]
	if !ok {
		return coverThumbnail{}, false
	}
	c.order.MoveToFront(elem)
	return elem.Value.(coverThumbnailEntry).item, true
}

func (c *coverThumbnailCache) add(key coverThumbnailKey, item coverThumbnail) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.addWithBoundsLocked(key, item, thumbnailCacheItems, thumbnailCacheBytes)
}

func (c *coverThumbnailCache) addWithBoundsLocked(key coverThumbnailKey, item coverThumbnail, maxItems, maxBytes int) {
	if c.items == nil {
		c.items = make(map[coverThumbnailKey]*list.Element)
		c.order = list.New()
	}
	itemBytes := len(item.data)
	if elem, ok := c.items[key]; ok {
		prev := elem.Value.(coverThumbnailEntry)
		c.bytes -= len(prev.item.data)
		c.bytes += itemBytes
		elem.Value = coverThumbnailEntry{key: key, item: item}
		c.order.MoveToFront(elem)
		for c.order.Len() > 1 && c.bytes > maxBytes {
			c.evictOldestLocked()
		}
		return
	}
	for c.order.Len() > 0 && (c.order.Len() >= maxItems || c.bytes+itemBytes > maxBytes) {
		c.evictOldestLocked()
	}
	elem := c.order.PushFront(coverThumbnailEntry{key: key, item: item})
	c.items[key] = elem
	c.bytes += itemBytes
}

func (c *coverThumbnailCache) evictOldestLocked() {
	oldest := c.order.Back()
	if oldest == nil {
		return
	}
	entry := oldest.Value.(coverThumbnailEntry)
	c.order.Remove(oldest)
	delete(c.items, entry.key)
	c.bytes -= len(entry.item.data)
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
		cache.init.Do(func() {
			slots := min(4, max(2, runtime.NumCPU()/2))
			cache.decode = make(chan struct{}, slots)
		})
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
	w.Header().Set("Cache-Control", "private, max-age=86400")
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

	dst := resizeCoverThumbnail(src, size)
	if dst == nil {
		return coverThumbnail{}
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

// resizeCoverThumbnail downscales src so its longest edge is size pixels using
// a 2x2 sample pattern per output pixel to reduce aliasing.
func resizeCoverThumbnail(src image.Image, size int) *image.RGBA {
	bounds := src.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	if width <= 0 || height <= 0 || size <= 0 {
		return nil
	}
	longSide := max(width, height)
	scaledWidth, scaledHeight := max(1, width*size/longSide), max(1, height*size/longSide)
	dst := image.NewRGBA(image.Rect(0, 0, scaledWidth, scaledHeight))

	var sxStack [1024]int
	var sxCoords []int
	if scaledWidth*2 <= len(sxStack) {
		sxCoords = sxStack[:scaledWidth*2]
	} else {
		sxCoords = make([]int, scaledWidth*2)
	}
	for x := 0; x < scaledWidth; x++ {
		sxCoords[2*x] = bounds.Min.X + (4*x+1)*width/(4*scaledWidth)
		sxCoords[2*x+1] = bounds.Min.X + (4*x+3)*width/(4*scaledWidth)
	}

	switch img := src.(type) {
	case *image.RGBA:
		minX, minY := img.Rect.Min.X, img.Rect.Min.Y
		pix := img.Pix
		stride := img.Stride
		for i := range sxCoords {
			sxCoords[i] = (sxCoords[i] - minX) * 4
		}
		for y := 0; y < scaledHeight; y++ {
			yOff0 := (bounds.Min.Y + (4*y+1)*height/(4*scaledHeight) - minY) * stride
			yOff1 := (bounds.Min.Y + (4*y+3)*height/(4*scaledHeight) - minY) * stride
			dstOff := y * dst.Stride
			for x := 0; x < scaledWidth; x++ {
				xOff0 := sxCoords[2*x]
				xOff1 := sxCoords[2*x+1]
				p00 := pix[yOff0+xOff0 : yOff0+xOff0+4 : yOff0+xOff0+4]
				p10 := pix[yOff0+xOff1 : yOff0+xOff1+4 : yOff0+xOff1+4]
				p01 := pix[yOff1+xOff0 : yOff1+xOff0+4 : yOff1+xOff0+4]
				p11 := pix[yOff1+xOff1 : yOff1+xOff1+4 : yOff1+xOff1+4]
				red := (uint32(p00[0]) + uint32(p10[0]) + uint32(p01[0]) + uint32(p11[0])) * 0x101
				green := (uint32(p00[1]) + uint32(p10[1]) + uint32(p01[1]) + uint32(p11[1])) * 0x101
				blue := (uint32(p00[2]) + uint32(p10[2]) + uint32(p01[2]) + uint32(p11[2])) * 0x101
				alpha := (uint32(p00[3]) + uint32(p10[3]) + uint32(p01[3]) + uint32(p11[3])) * 0x101
				d := dst.Pix[dstOff : dstOff+4 : dstOff+4]
				d[0] = uint8(red >> 10)
				d[1] = uint8(green >> 10)
				d[2] = uint8(blue >> 10)
				d[3] = uint8(alpha >> 10)
				dstOff += 4
			}
		}
	case *image.NRGBA:
		minX, minY := img.Rect.Min.X, img.Rect.Min.Y
		pix := img.Pix
		stride := img.Stride
		for i := range sxCoords {
			sxCoords[i] = (sxCoords[i] - minX) * 4
		}
		for y := 0; y < scaledHeight; y++ {
			yOff0 := (bounds.Min.Y + (4*y+1)*height/(4*scaledHeight) - minY) * stride
			yOff1 := (bounds.Min.Y + (4*y+3)*height/(4*scaledHeight) - minY) * stride
			dstOff := y * dst.Stride
			for x := 0; x < scaledWidth; x++ {
				xOff0 := sxCoords[2*x]
				xOff1 := sxCoords[2*x+1]
				r0, g0, b0, a0 := nrgbaSample(pix, yOff0+xOff0)
				r1, g1, b1, a1 := nrgbaSample(pix, yOff0+xOff1)
				r2, g2, b2, a2 := nrgbaSample(pix, yOff1+xOff0)
				r3, g3, b3, a3 := nrgbaSample(pix, yOff1+xOff1)
				d := dst.Pix[dstOff : dstOff+4 : dstOff+4]
				d[0] = uint8((r0 + r1 + r2 + r3) >> 10)
				d[1] = uint8((g0 + g1 + g2 + g3) >> 10)
				d[2] = uint8((b0 + b1 + b2 + b3) >> 10)
				d[3] = uint8((a0 + a1 + a2 + a3) >> 10)
				dstOff += 4
			}
		}
	case *image.YCbCr:
		var divX, divY int
		switch img.SubsampleRatio {
		case image.YCbCrSubsampleRatio422:
			divX, divY = 2, 1
		case image.YCbCrSubsampleRatio420:
			divX, divY = 2, 2
		case image.YCbCrSubsampleRatio440:
			divX, divY = 1, 2
		case image.YCbCrSubsampleRatio411:
			divX, divY = 4, 1
		case image.YCbCrSubsampleRatio410:
			divX, divY = 4, 2
		default:
			divX, divY = 1, 1
		}
		var cxStack [1024]int
		var cxCoords []int
		if len(sxCoords) <= len(cxStack) {
			cxCoords = cxStack[:len(sxCoords)]
		} else {
			cxCoords = make([]int, len(sxCoords))
		}
		minX, minY := img.Rect.Min.X, img.Rect.Min.Y
		minCX, minCY := minX/divX, minY/divY
		for i, sx := range sxCoords {
			cxCoords[i] = sx/divX - minCX
			sxCoords[i] = sx - minX
		}
		yPix, cbPix, crPix := img.Y, img.Cb, img.Cr
		yStride, cStride := img.YStride, img.CStride
		for y := 0; y < scaledHeight; y++ {
			sy0 := bounds.Min.Y + (4*y+1)*height/(4*scaledHeight)
			sy1 := bounds.Min.Y + (4*y+3)*height/(4*scaledHeight)
			yyOff0 := (sy0 - minY) * yStride
			yyOff1 := (sy1 - minY) * yStride
			cyOff0 := (sy0/divY - minCY) * cStride
			cyOff1 := (sy1/divY - minCY) * cStride
			dstOff := y * dst.Stride
			for x := 0; x < scaledWidth; x++ {
				xy0, xy1 := sxCoords[2*x], sxCoords[2*x+1]
				xc0, xc1 := cxCoords[2*x], cxCoords[2*x+1]
				ci00 := cyOff0 + xc0
				ci10 := cyOff0 + xc1
				ci01 := cyOff1 + xc0
				ci11 := cyOff1 + xc1
				r0, g0, b0 := ycbcrToRGBA(yPix[yyOff0+xy0], cbPix[ci00], crPix[ci00])
				r1, g1, b1 := ycbcrToRGBA(yPix[yyOff0+xy1], cbPix[ci10], crPix[ci10])
				r2, g2, b2 := ycbcrToRGBA(yPix[yyOff1+xy0], cbPix[ci01], crPix[ci01])
				r3, g3, b3 := ycbcrToRGBA(yPix[yyOff1+xy1], cbPix[ci11], crPix[ci11])
				d := dst.Pix[dstOff : dstOff+4 : dstOff+4]
				d[0] = uint8((r0 + r1 + r2 + r3) >> 10)
				d[1] = uint8((g0 + g1 + g2 + g3) >> 10)
				d[2] = uint8((b0 + b1 + b2 + b3) >> 10)
				d[3] = 0xff
				dstOff += 4
			}
		}
	default:
		for y := 0; y < scaledHeight; y++ {
			sy0 := bounds.Min.Y + (4*y+1)*height/(4*scaledHeight)
			sy1 := bounds.Min.Y + (4*y+3)*height/(4*scaledHeight)
			for x := 0; x < scaledWidth; x++ {
				sx0, sx1 := sxCoords[2*x], sxCoords[2*x+1]
				r0, g0, b0, a0 := src.At(sx0, sy0).RGBA()
				r1, g1, b1, a1 := src.At(sx1, sy0).RGBA()
				r2, g2, b2, a2 := src.At(sx0, sy1).RGBA()
				r3, g3, b3, a3 := src.At(sx1, sy1).RGBA()
				dst.SetRGBA(x, y, color.RGBA{
					R: uint8((r0 + r1 + r2 + r3) >> 10),
					G: uint8((g0 + g1 + g2 + g3) >> 10),
					B: uint8((b0 + b1 + b2 + b3) >> 10),
					A: uint8((a0 + a1 + a2 + a3) >> 10),
				})
			}
		}
	}
	return dst
}

func nrgbaSample(pix []byte, off int) (uint32, uint32, uint32, uint32) {
	p := pix[off : off+4 : off+4]
	a := uint32(p[3])
	switch a {
	case 0:
		return 0, 0, 0, 0
	case 0xff:
		return uint32(p[0]) * 0x101, uint32(p[1]) * 0x101, uint32(p[2]) * 0x101, 0xffff
	default:
		return (uint32(p[0]) * 0x101 * a) / 0xff,
			(uint32(p[1]) * 0x101 * a) / 0xff,
			(uint32(p[2]) * 0x101 * a) / 0xff,
			a * 0x101
	}
}

func ycbcrToRGBA(yVal, cbVal, crVal byte) (uint32, uint32, uint32) {
	yy1 := int32(yVal) * 0x10101
	cb1 := int32(cbVal) - 128
	cr1 := int32(crVal) - 128

	r := yy1 + 91881*cr1
	if uint32(r)&0xff000000 == 0 {
		r >>= 8
	} else {
		r = ^(r >> 31) & 0xffff
	}

	g := yy1 - 22554*cb1 - 46802*cr1
	if uint32(g)&0xff000000 == 0 {
		g >>= 8
	} else {
		g = ^(g >> 31) & 0xffff
	}

	b := yy1 + 116130*cb1
	if uint32(b)&0xff000000 == 0 {
		b >>= 8
	} else {
		b = ^(b >> 31) & 0xffff
	}

	return uint32(r), uint32(g), uint32(b)
}
