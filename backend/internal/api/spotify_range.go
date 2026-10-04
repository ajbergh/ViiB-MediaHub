package api

import (
	"fmt"
	"strconv"
	"strings"
)

// Single byte ranges used by media elements, including suffix/tail probes.
// Multiple or invalid ranges are rejected rather than expanded to a full file.
func parseSpotifyByteRange(header string, size int64) (int64, int64, error) {
	invalid := fmt.Errorf("invalid Spotify byte range")
	if size <= 0 || len(header) > 128 || !strings.HasPrefix(header, "bytes=") || strings.Contains(header, ",") {
		return 0, 0, invalid
	}
	parts := strings.Split(strings.TrimPrefix(header, "bytes="), "-")
	if len(parts) != 2 || (parts[0] == "" && parts[1] == "") {
		return 0, 0, invalid
	}
	parse := func(value string) (int64, error) {
		if value == "" {
			return 0, invalid
		}
		for _, digit := range value {
			if digit < '0' || digit > '9' {
				return 0, invalid
			}
		}
		return strconv.ParseInt(value, 10, 64)
	}
	if parts[0] == "" {
		count, err := parse(parts[1])
		if err != nil || count <= 0 {
			return 0, 0, invalid
		}
		if count > size {
			count = size
		}
		return size - count, size - 1, nil
	}
	start, err := parse(parts[0])
	if err != nil || start >= size {
		return 0, 0, invalid
	}
	end := size - 1
	if parts[1] != "" {
		end, err = parse(parts[1])
		if err != nil || end < start {
			return 0, 0, invalid
		}
		if end >= size {
			end = size - 1
		}
	}
	return start, end, nil
}
