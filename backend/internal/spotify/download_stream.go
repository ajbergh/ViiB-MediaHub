package spotify

import (
	"context"
	"fmt"
	"io"

	"github.com/art-media-platform/librespot-go/librespot/asset"
)

// copySpotifyDownload avoids two hazards in the pinned librespot asset reader:
// Read loses its already-copied bytes if acquiring the next chunk returns an
// error, and a chunk-aligned EOF can cause it to request a nonexistent chunk.
// SeekEnd reports the logical size (the 167-byte Spotify prefix is excluded).
// Bound each Read by both that size and the encrypted transport chunk boundary.
// Do not use io.Copy/ReadFull here: their reads can cross those boundaries.
func copySpotifyDownload(ctx context.Context, dst io.Writer, src io.ReadSeeker, progress func(int64, int64)) (int64, error) {
	size, err := src.Seek(0, io.SeekEnd)
	if err != nil {
		return 0, fmt.Errorf("get Spotify asset length: %w", err)
	}
	if size <= 0 {
		return 0, fmt.Errorf("%w: invalid Spotify asset length", ErrOggIntegrity)
	}
	if _, err := src.Seek(0, io.SeekStart); err != nil {
		return 0, fmt.Errorf("rewind Spotify asset: %w", err)
	}
	const chunkSize int64 = 128 * 1024 // librespot's 32768 four-byte words
	buf := make([]byte, 64*1024)
	var copied int64
	for copied < size {
		if err := ctx.Err(); err != nil {
			return copied, err
		}
		rawOffset := copied + int64(asset.SPOTIFY_OGG_HEADER_SIZE)
		limit := min(int64(len(buf)), size-copied, chunkSize-rawOffset%chunkSize)
		n, readErr := src.Read(buf[:int(limit)])
		if n < 0 || int64(n) > limit {
			return copied, fmt.Errorf("%w: invalid Spotify read count", ErrOggIntegrity)
		}
		if n > 0 {
			written, writeErr := dst.Write(buf[:n])
			copied += int64(written)
			if writeErr != nil {
				return copied, writeErr
			}
			if written != n {
				return copied, io.ErrShortWrite
			}
			if progress != nil {
				progress(copied, size)
			}
		}
		if readErr != nil {
			if err := ctx.Err(); err != nil {
				return copied, err
			}
			if readErr == io.EOF {
				if copied == size {
					return copied, nil
				}
				return copied, fmt.Errorf("%w: truncated Spotify asset (%d of %d bytes): %w", ErrOggIntegrity, copied, size, io.ErrUnexpectedEOF)
			}
			return copied, readErr
		}
		if n == 0 {
			return copied, io.ErrNoProgress
		}
	}
	return copied, nil
}
