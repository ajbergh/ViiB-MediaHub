// Tests validation of Ogg page checksums and end-of-stream with bounded trailing padding.
package spotify

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/art-media-platform/librespot-go/librespot/asset"
)

func testOggPage(flags byte, body []byte) []byte {
	page := make([]byte, 28+len(body))
	copy(page, "OggS")
	page[5] = flags
	page[26] = 1
	page[27] = byte(len(body))
	copy(page[28:], body)
	var crc uint32
	for _, value := range page {
		crc = crc<<8 ^ oggCRCTable[byte(crc>>24)^value]
	}
	binary.LittleEndian.PutUint32(page[22:26], crc)
	return page
}

func TestValidateOggPagesRejectsMiddleCorruption(t *testing.T) {
	first := testOggPage(2, []byte("header"))
	middle := testOggPage(0, []byte("audio"))
	last := testOggPage(4, []byte("end"))
	valid := append(append(append([]byte{}, first...), middle...), last...)
	if err := validateOggReader(context.Background(), bytes.NewReader(valid)); err != nil {
		t.Fatal(err)
	}
	for _, offset := range []int{len(first) + 22, len(first) + 28} {
		damaged := append([]byte{}, valid...)
		damaged[offset] ^= 1
		if !errors.Is(validateOggReader(context.Background(), bytes.NewReader(damaged)), ErrOggIntegrity) {
			t.Fatal("middle page corruption accepted")
		}
	}
	for _, invalid := range [][]byte{nil, valid[:len(valid)-1], valid[:len(first)], append(append([]byte{}, valid...), 1)} {
		if validateOggReader(context.Background(), bytes.NewReader(invalid)) == nil {
			t.Fatal("incomplete container accepted")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if !errors.Is(validateOggReader(ctx, bytes.NewReader(valid)), context.Canceled) {
		t.Fatal("cancellation ignored")
	}
}

func TestValidateOggAllowsBoundedZeroTrailerAfterEOS(t *testing.T) {
	complete := append(testOggPage(2, []byte("header")), testOggPage(4, []byte("audio"))...)
	for _, count := range []int{1, 3, 15} {
		padded := append(append([]byte{}, complete...), make([]byte, count)...)
		if err := validateOggReader(context.Background(), bytes.NewReader(padded)); err != nil {
			t.Fatalf("zero trailer %d: %v", count, err)
		}
	}
	for _, invalid := range [][]byte{
		append(append([]byte{}, complete...), make([]byte, 16)...),
		append(testOggPage(2, []byte("header")), 0, 0, 0),
		append(append([]byte{}, complete...), []byte("Ogg")...),
	} {
		if validateOggReader(context.Background(), bytes.NewReader(invalid)) == nil {
			t.Fatal("unbounded, incomplete or nonzero trailer accepted")
		}
	}
}

func TestValidateOggIncompleteTrailerDiagnostics(t *testing.T) {
	for _, test := range []struct {
		flags byte
		tail  []byte
		want  string
	}{
		{0, []byte{0, 0, 0}, "preceding EOS=false; zero trailer=true"},
		{4, []byte{'O', 'g', 'g'}, "preceding EOS=true; zero trailer=false"},
	} {
		data := append(testOggPage(test.flags, []byte("audio")), test.tail...)
		err := validateOggReader(context.Background(), bytes.NewReader(data))
		if !errors.Is(err, ErrOggIntegrity) || !strings.Contains(err.Error(), test.want) {
			t.Fatalf("missing tail diagnosis: %v", err)
		}
	}
}

func TestNormalizeSpotifyOggWordPadding(t *testing.T) {
	for _, count := range []int{1, 2, 3} {
		t.Run(strconv.Itoa(count), func(t *testing.T) {
			// A CRC-valid BOS/EOS page with exactly count bytes of word padding.
			// 28-byte page + 167-byte prefix == 3 mod 4.
			bodySize := (5 - count) % 4
			complete := testOggPage(6, make([]byte, bodySize))
			padded := append(append([]byte{}, complete...), bytes.Repeat([]byte{0xa5}, count)...)
			if (len(padded)+asset.SPOTIFY_OGG_HEADER_SIZE)%4 != 0 {
				t.Fatal("fixture is not transport-word aligned")
			}
			if validateOggReader(context.Background(), bytes.NewReader(padded)) == nil {
				t.Fatal("generic validation accepted nonzero transport padding")
			}
			path := filepath.Join(t.TempDir(), "candidate.vctemp")
			if err := os.WriteFile(path, padded, 0600); err != nil {
				t.Fatal(err)
			}
			if err := normalizeSpotifyOggDownload(context.Background(), path, int64(len(padded))); err != nil {
				t.Fatal(err)
			}
			got, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(got, complete) {
				t.Fatalf("audio pages changed: %v", err)
			}
			if err := validateOggPages(context.Background(), path); err != nil {
				t.Fatalf("normalized candidate fails strict validation: %v", err)
			}
		})
	}
}

func TestNormalizeSpotifyOggRejectsDamageWithoutMutation(t *testing.T) {
	complete := testOggPage(6, []byte{1, 2}) // needs three transport bytes
	corrupt := append([]byte{}, complete...)
	corrupt[len(corrupt)-1] ^= 1
	for _, test := range []struct {
		name string
		data []byte
	}{
		{"checksum", append(corrupt, 1, 2, 3)},
		{"missing EOS", append(testOggPage(2, []byte{1, 2}), 1, 2, 3)},
		{"wrong alignment", append(append([]byte{}, complete...), 1, 2)},
		{"four nonzero bytes", append(append([]byte{}, complete...), 1, 2, 3, 4)},
		{"partial next page", append(append([]byte{}, complete...), []byte("OggS")...)},
		{"truncated payload", complete[:len(complete)-1]},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "candidate.vctemp")
			if err := os.WriteFile(path, test.data, 0600); err != nil {
				t.Fatal(err)
			}
			if err := normalizeSpotifyOggDownload(context.Background(), path, int64(len(test.data))); !errors.Is(err, ErrOggIntegrity) {
				t.Fatalf("damaged file accepted: %v", err)
			}
			got, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(got, test.data) {
				t.Fatalf("failed validation modified candidate: %v", err)
			}
		})
	}
}

func TestNormalizeSpotifyOggCancellationAndSizeMismatch(t *testing.T) {
	data := append(testOggPage(6, []byte{1, 2}), 1, 2, 3)
	path := filepath.Join(t.TempDir(), "candidate.vctemp")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := normalizeSpotifyOggDownload(ctx, path, int64(len(data))); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation ignored: %v", err)
	}
	if err := normalizeSpotifyOggDownload(context.Background(), path, int64(len(data)+1)); !errors.Is(err, ErrOggIntegrity) {
		t.Fatalf("size mismatch accepted: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(got, data) {
		t.Fatalf("candidate changed: %v", err)
	}
}

func TestNormalizeSpotifyOggPreservesPagesWithLegacyZeroPadding(t *testing.T) {
	complete := testOggPage(6, []byte("audio"))
	for _, count := range []int{0, 3, 15} {
		path := filepath.Join(t.TempDir(), "candidate.vctemp")
		data := append(append([]byte{}, complete...), make([]byte, count)...)
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
		if err := normalizeSpotifyOggDownload(context.Background(), path, int64(len(data))); err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(got, complete) {
			t.Fatalf("pages changed with %d zero bytes: %v", count, err)
		}
	}
}
