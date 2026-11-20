// Prepares a reusable, seekable Spotify audio asset for players without Vorbis.
package spotify

import (
	"context"
	"io"
	"os"

	"github.com/ajbergh/viib-mediahub/internal/audio"
)

func prepareMP3StreamAsset(ctx context.Context, source io.Reader) (*streamAsset, error) {
	temp, err := os.CreateTemp("", "viib-spotify-mp3-*")
	if err != nil {
		return nil, err
	}
	path := temp.Name()
	ready := false
	defer func() {
		_ = temp.Close()
		if !ready {
			_ = os.Remove(path)
		}
	}()
	if err := audio.EncodeOggVorbisMP3(ctx, source, temp); err != nil {
		return nil, err
	}
	size, err := temp.Seek(0, io.SeekCurrent)
	if err != nil {
		return nil, err
	}
	if err := temp.Close(); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	ready = true
	return &streamAsset{
		size:        size,
		contentType: "audio/mpeg",
		newReader:   func() (io.ReadSeekCloser, error) { return os.Open(path) },
		close:       func() { _ = os.Remove(path) },
	}, nil
}
