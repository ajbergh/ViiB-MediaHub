package spotify

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"testing"
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
