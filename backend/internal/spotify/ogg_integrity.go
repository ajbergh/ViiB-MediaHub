// Validates Ogg page checksums and end-of-stream with bounded trailing padding.
package spotify

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/art-media-platform/librespot-go/librespot/asset"
)

// ErrOggIntegrity identifies malformed, incomplete or checksum-invalid Ogg pages.
var ErrOggIntegrity = errors.New("invalid Ogg page integrity")

// IsOggIntegrityError identifies a damaged candidate for bounded download retry.
func IsOggIntegrityError(err error) bool { return errors.Is(err, ErrOggIntegrity) }

var oggCRCTable = func() [256]uint32 {
	var table [256]uint32
	for i := range table {
		value := uint32(i) << 24
		for j := 0; j < 8; j++ {
			if value&0x80000000 != 0 {
				value = value<<1 ^ 0x04c11db7
			} else {
				value <<= 1
			}
		}
		table[i] = value
	}
	return table
}()

// Check every page without loading the recording into memory. A checksum mismatch
// is rejected rather than repaired, since it may reflect damaged audio payload.
func validateOggPages(ctx context.Context, path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	return validateOggReader(ctx, file)
}

func validateOggReader(ctx context.Context, source io.Reader) error {
	return validateOggReaderWithPadding(ctx, source, nil)
}

// Only raw Spotify candidates may contain arbitrary-valued word padding. The
// AP advertises a length in four-byte words, including the proprietary prefix;
// decrypted padding bytes need not be zero. Strip them only after every page
// passed CRC and the final page declared EOS. Generic/local validation stays
// strict, and never interprets arbitrary nonzero trailing data as padding.
func normalizeSpotifyOggDownload(ctx context.Context, path string, transferred int64) error {
	file, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return err
	}
	if info.Size() != transferred {
		return fmt.Errorf("%w: downloaded size differs from transfer length", ErrOggIntegrity)
	}
	return validateOggReaderWithPadding(ctx, file, func(end int64, count int) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := file.Truncate(end); err != nil {
			return fmt.Errorf("remove Spotify transport padding: %w", err)
		}
		dLog("Removed %d trailing transport padding bytes after validated Ogg EOS", count)
		return nil
	})
}

func validateOggReaderWithPadding(ctx context.Context, source io.Reader, trimPadding func(int64, int) error) error {
	header := make([]byte, 27)
	segments := make([]byte, 255)
	payload := make([]byte, 255*255)
	pages := 0
	var offset int64
	ended := false
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		read, err := io.ReadFull(source, header)
		// A raw Spotify asset is rounded up to a four-byte word. The rounding
		// includes the 167-byte prefix that librespot removes from reader output.
		// Accept only the exact 1-3 bytes needed to complete that word, at EOF
		// after CRC-valid EOS; never skip a page or ignore a checksum failure.
		if err == io.ErrUnexpectedEOF && pages > 0 && ended && trimPadding != nil {
			expected := int((4 - (offset+int64(asset.SPOTIFY_OGG_HEADER_SIZE))%4) % 4)
			if read > 0 && read == expected {
				return trimPadding(offset, read)
			}
		}
		// Observed Spotify assets can end with a few zero bytes after a complete EOS
		// page. Accept at most 15 bytes, never nonzero data or a partial Ogg page.
		if err == io.ErrUnexpectedEOF && pages > 0 && ended && read <= 15 {
			zero := true
			for _, value := range header[:read] {
				zero = zero && value == 0
			}
			if zero {
				if trimPadding != nil {
					return trimPadding(offset, read)
				}
				return nil
			}
		}
		if err == io.EOF && pages > 0 && ended {
			return nil
		}
		if err == io.EOF && pages > 0 {
			return fmt.Errorf("%w: missing end-of-stream page", ErrOggIntegrity)
		}
		if err != nil {
			zeroTrailer := read > 0
			for _, value := range header[:read] {
				zeroTrailer = zeroTrailer && value == 0
			}
			return fmt.Errorf("%w: incomplete page header at page %d byte %d (%d of 27 bytes; preceding EOS=%t; zero trailer=%t)", ErrOggIntegrity, pages, offset, read, ended, zeroTrailer)
		}
		if string(header[:4]) != "OggS" || header[4] != 0 {
			return fmt.Errorf("%w: invalid capture or version at page %d byte %d", ErrOggIntegrity, pages, offset)
		}
		count := int(header[26])
		if _, err := io.ReadFull(source, segments[:count]); err != nil {
			return fmt.Errorf("%w: incomplete segment table at page %d byte %d", ErrOggIntegrity, pages, offset)
		}
		size := 0
		for _, value := range segments[:count] {
			size += int(value)
		}
		if _, err := io.ReadFull(source, payload[:size]); err != nil {
			return fmt.Errorf("%w: incomplete page payload at page %d byte %d", ErrOggIntegrity, pages, offset)
		}
		expected := binary.LittleEndian.Uint32(header[22:26])
		clear(header[22:26])
		crc := uint32(0)
		for _, part := range [][]byte{header, segments[:count], payload[:size]} {
			for _, value := range part {
				crc = crc<<8 ^ oggCRCTable[byte(crc>>24)^value]
			}
		}
		if crc != expected {
			return fmt.Errorf("%w: page checksum mismatch at page %d byte %d", ErrOggIntegrity, pages, offset)
		}
		ended = header[5]&4 != 0
		pages++
		offset += int64(len(header) + count + size)
	}
}
