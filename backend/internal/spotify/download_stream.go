// Defines download stream functionality for package spotify.

package spotify

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/art-media-platform/librespot-go/librespot/asset"
)

// ErrDownloadAudioTimeout means that an open Spotify asset stopped yielding bytes.
var ErrDownloadAudioTimeout = errors.New("timed out waiting for Spotify audio transfer data")

// Closing the asset task wakes a librespot reader waiting for a missing chunk.
// Use a transfer-specific cause so an idle reader can be retried without
// treating a user cancellation or account change as a transport failure.
func copySpotifyDownloadWithTimeout(ctx context.Context, dst io.Writer, src io.ReadSeeker, interrupt func(), idleTimeout time.Duration, progress ProgressCallback) (int64, error) {
	transfer, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	interrupted := make(chan struct{})
	stopInterrupt := context.AfterFunc(transfer, func() {
		defer close(interrupted)
		interrupt()
	})
	defer func() {
		if !stopInterrupt() {
			<-interrupted
		}
	}()

	idle := time.AfterFunc(idleTimeout, func() { cancel(ErrDownloadAudioTimeout) })
	defer idle.Stop()
	n, err := copySpotifyDownload(transfer, dst, src, func(done, total int64) {
		idle.Reset(idleTimeout)
		if progress != nil {
			progress(done, total)
		}
	})
	if parentErr := ctx.Err(); parentErr != nil {
		return n, parentErr
	}
	if cause := context.Cause(transfer); cause != nil {
		return n, cause
	}
	return n, err
}

// copySpotifyDownload avoids two hazards in the pinned librespot asset reader:
// Read loses its already-copied bytes if acquiring the next chunk returns an
// error, and a chunk-aligned EOF can cause it to request a nonexistent chunk.
// SeekEnd reports the logical size (the 167-byte Spotify prefix is excluded).
// Bound each Read by both that size and the encrypted transport chunk boundary.
// Do not use io.Copy/ReadFull here: their reads can cross those boundaries.
func copySpotifyDownload(ctx context.Context, dst io.Writer, src io.ReadSeeker, progress func(int64, int64)) (int64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
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
