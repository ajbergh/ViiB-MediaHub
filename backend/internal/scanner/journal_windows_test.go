//go:build windows

package scanner

import (
	"encoding/binary"
	"testing"
	"unicode/utf16"
)

func TestDecodeUSNRecordFileName(t *testing.T) {
	const name = "set-\U0001f3b5.wav"
	encoded := utf16.Encode([]rune(name))
	record := make([]byte, int(usnRecordV2HeaderSize)+len(encoded)*2)
	for i, codeUnit := range encoded {
		binary.LittleEndian.PutUint16(record[int(usnRecordV2HeaderSize)+i*2:], codeUnit)
	}

	decoded, err := decodeUSNRecordFileName(record, uint16(usnRecordV2HeaderSize), uint16(len(encoded)*2))
	if err != nil {
		t.Fatalf("decodeUSNRecordFileName() error = %v", err)
	}
	if decoded != name {
		t.Fatalf("decodeUSNRecordFileName() = %q, want %q", decoded, name)
	}
}

func TestDecodeUSNRecordFileNameRejectsInvalidRanges(t *testing.T) {
	record := make([]byte, usnRecordV2HeaderSize+4)
	for _, tc := range []struct {
		name   string
		offset uint16
		length uint16
	}{
		{name: "offset before header", offset: 0, length: 2},
		{name: "odd length", offset: uint16(usnRecordV2HeaderSize), length: 3},
		{name: "past record", offset: uint16(usnRecordV2HeaderSize) + 2, length: 4},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := decodeUSNRecordFileName(record, tc.offset, tc.length); err == nil {
				t.Fatal("decodeUSNRecordFileName() error = nil, want malformed record error")
			}
		})
	}
}
