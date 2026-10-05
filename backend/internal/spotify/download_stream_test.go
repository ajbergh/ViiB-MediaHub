// Tests and fixtures for download stream behavior.

package spotify

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strconv"
	"testing"

	"github.com/art-media-platform/librespot-go/librespot/asset"
)

// Model the pinned reader's return-0-on-next-chunk-error behavior. Bytes are
// copied and the cursor advanced before an error, but the returned count is 0.
// Crossing the logical EOF models EOF from lockChunkAtOfs; crossing a transport
// boundary models a failed next-chunk acquisition. No credentials/network needed.
type boundaryAssetReader struct {
	data    []byte
	pos     int64
	reads   int
	crossed bool
}

func (r *boundaryAssetReader) Seek(offset int64, whence int) (int64, error) {
	switch whence {
	case io.SeekStart:
		r.pos = offset
	case io.SeekCurrent:
		r.pos += offset
	case io.SeekEnd:
		r.pos = int64(len(r.data)) + offset
	}
	return r.pos, nil
}

func (r *boundaryAssetReader) Read(p []byte) (int, error) {
	r.reads++
	if r.pos >= int64(len(r.data)) {
		return 0, io.EOF
	}
	remaining := int64(len(r.data)) - r.pos
	chunkRemaining := int64(128*1024) - (r.pos+int64(asset.SPOTIFY_OGG_HEADER_SIZE))%(128*1024)
	available := min(remaining, chunkRemaining)
	n := copy(p, r.data[r.pos:r.pos+min(available, int64(len(p)))])
	r.pos += int64(n)
	if int64(len(p)) > available {
		r.crossed = true
		if available == remaining {
			return 0, io.EOF
		}
		return 0, errors.New("next chunk acquisition failed")
	}
	return n, nil
}

func TestCopySpotifyDownloadPreservesBoundaryBytes(t *testing.T) {
	for _, size := range []int{1, 65535, 65536, 65537, 128*1024 - asset.SPOTIFY_OGG_HEADER_SIZE, 128*1024 + 17, 256*1024 - asset.SPOTIFY_OGG_HEADER_SIZE, 3*128*1024 + 731} {
		t.Run(strconv.Itoa(size), func(t *testing.T) {
			data := make([]byte, size)
			for i := range data {
				data[i] = byte(i*31 + i/257)
			}
			src := &boundaryAssetReader{data: data}
			var dst bytes.Buffer
			var lastProgress int64
			copied, err := copySpotifyDownload(context.Background(), &dst, src, func(done, total int64) {
				if done <= lastProgress || total != int64(size) {
					t.Fatalf("invalid progress: %d/%d after %d", done, total, lastProgress)
				}
				lastProgress = done
			})
			if err != nil || copied != int64(size) || lastProgress != copied || !bytes.Equal(dst.Bytes(), data) || src.crossed {
				t.Fatalf("copy lost bytes: copied=%d size=%d crossed=%v err=%v", copied, size, src.crossed, err)
			}
		})
	}
}

func TestUnboundedSpotifyReadLosesCopiedTail(t *testing.T) {
	data := bytes.Repeat([]byte{1}, 65537)
	src := &boundaryAssetReader{data: data}
	var dst bytes.Buffer
	buf := make([]byte, 65536)
	for {
		n, err := src.Read(buf)
		dst.Write(buf[:n])
		if err != nil {
			break
		}
	}
	if dst.Len() != len(data)-1 || !src.crossed {
		t.Fatal("fixture did not reproduce the pinned reader's byte loss")
	}
}

func TestCopySpotifyDownloadPreservesOggEOSAtChunkBoundary(t *testing.T) {
	// The raw asset ends exactly on a transport boundary. The pinned reader
	// can attempt the following chunk and return 0, EOF, dropping the EOS page
	// already copied into its caller's final buffer.
	size := 128*1024 - asset.SPOTIFY_OGG_HEADER_SIZE
	var data []byte
	for len(data)+283 < size {
		data = append(data, testOggPage(0, make([]byte, 255))...)
	}
	// Leave enough room for the final page's header and a bounded body.
	remaining := size - len(data)
	if remaining < 28 {
		data = data[:len(data)-283]
		remaining += 283
	}
	if remaining > 283 {
		data = append(data, testOggPage(0, make([]byte, remaining-28-28))...)
		remaining = 28
	}
	data = append(data, testOggPage(4, make([]byte, remaining-28))...)
	if len(data) != size {
		t.Fatalf("fixture size: %d, want %d", len(data), size)
	}
	var dst bytes.Buffer
	_, err := copySpotifyDownload(context.Background(), &dst, &boundaryAssetReader{data: data}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateOggReader(context.Background(), bytes.NewReader(dst.Bytes())); err != nil {
		t.Fatalf("EOS lost: %v", err)
	}
}

type advertisedAssetReader struct {
	*bytes.Reader
	size int64
}

func (r advertisedAssetReader) Seek(offset int64, whence int) (int64, error) {
	if whence == io.SeekEnd {
		return r.size + offset, nil
	}
	return r.Reader.Seek(offset, whence)
}

func TestCopySpotifyDownloadRejectsPrematureEOF(t *testing.T) {
	src := advertisedAssetReader{bytes.NewReader([]byte("short")), 100}
	var dst bytes.Buffer
	n, err := copySpotifyDownload(context.Background(), &dst, src, nil)
	if n != 5 || !errors.Is(err, ErrOggIntegrity) || !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("premature EOF accepted: %d, %v", n, err)
	}
}

func TestCopySpotifyDownloadCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	src := &boundaryAssetReader{data: make([]byte, 256*1024)}
	var dst bytes.Buffer
	n, err := copySpotifyDownload(ctx, &dst, src, func(int64, int64) { cancel() })
	if !errors.Is(err, context.Canceled) || n != 65536 || src.reads != 1 {
		t.Fatalf("copy continued after cancellation: %d, %d reads, %v", n, src.reads, err)
	}
}

type stalledAssetReader struct{ *bytes.Reader }

func (stalledAssetReader) Read([]byte) (int, error) { return 0, nil }

func TestCopySpotifyDownloadRejectsNoProgress(t *testing.T) {
	var dst bytes.Buffer
	_, err := copySpotifyDownload(context.Background(), &dst, stalledAssetReader{bytes.NewReader([]byte("data"))}, nil)
	if !errors.Is(err, io.ErrNoProgress) {
		t.Fatalf("stalled reader accepted: %v", err)
	}
}
