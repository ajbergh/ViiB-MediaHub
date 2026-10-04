package spotify

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
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
	header := make([]byte, 27)
	segments := make([]byte, 255)
	payload := make([]byte, 255*255)
	pages := 0
	ended := false
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		read, err := io.ReadFull(source, header)
		// Observed Spotify assets can end with a few zero bytes after a complete EOS
		// page. Accept at most 15 bytes, never nonzero data or a partial Ogg page.
		if err == io.ErrUnexpectedEOF && pages > 0 && ended && read <= 15 {
			zero := true
			for _, value := range header[:read] {
				zero = zero && value == 0
			}
			if zero {
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
			return ErrOggIntegrity
		}
		if string(header[:4]) != "OggS" || header[4] != 0 {
			return ErrOggIntegrity
		}
		count := int(header[26])
		if _, err := io.ReadFull(source, segments[:count]); err != nil {
			return ErrOggIntegrity
		}
		size := 0
		for _, value := range segments[:count] {
			size += int(value)
		}
		if _, err := io.ReadFull(source, payload[:size]); err != nil {
			return ErrOggIntegrity
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
			return fmt.Errorf("%w: page checksum mismatch", ErrOggIntegrity)
		}
		ended = header[5]&4 != 0
		pages++
	}
}
