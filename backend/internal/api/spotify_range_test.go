// Tests parsing single HTTP byte ranges, including suffix, open-ended, and zero-byte boundaries.
package api

import "testing"

func TestSpotifyMediaByteRanges(t *testing.T) {
	for _, test := range []struct {
		header     string
		start, end int64
	}{
		{"bytes=0-0", 0, 0}, {"bytes=1-3", 1, 3}, {"bytes=4-", 4, 9}, {"bytes=0-999", 0, 9}, {"bytes=-3", 7, 9}, {"bytes=-999", 0, 9},
	} {
		start, end, err := parseSpotifyByteRange(test.header, 10)
		if err != nil || start != test.start || end != test.end {
			t.Errorf("%q got %d-%d %v", test.header, start, end, err)
		}
	}
	for _, header := range []string{"bytes=10-", "bytes=4-2", "bytes=-0", "bytes=", "bytes=-", "bytes=0-1,3-4", "bytes=+1-2", "bytes=1-2garbage", "bytes=999999999999999999999-", "bytes=-1-2", "items=0-2", "bytes= 1-2"} {
		if _, _, err := parseSpotifyByteRange(header, 10); err == nil {
			t.Errorf("invalid %q accepted", header)
		}
	}
	if _, _, err := parseSpotifyByteRange("bytes=0-0", 0); err == nil {
		t.Fatal("zero length resource accepted")
	}
}
