//go:build spotify_research

package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestPlaylistShapeCountsEveryRowWithoutPrivateValues(t *testing.T) {
	var playlist map[string]any
	body := `{"content":{"totalCount":4,"items":[{"itemV2":{"data":{"__typename":"Track","name":"private-title","artists":{"items":[{}]},"albumOfTrack":{"uri":"private-album","name":"private-name"},"trackDuration":{"totalMilliseconds":12345}}}},null,{"itemV2":{"data":{"__typename":"private-type"}}},{"itemV2":{"data":{"__typename":"Track","albumOfTrack":{}}}}]},"private-user":"private-value"}`
	if err := json.Unmarshal([]byte(body), &playlist); err != nil {
		t.Fatal(err)
	}
	counts := playlistShape(playlist)
	if counts["rows"] != 4 || counts["Track"] != 2 || counts["nullOrInvalidRows"] != 1 || counts["otherType"] != 1 ||
		counts["missingAlbumURI"] != 1 || counts["missingAlbumName"] != 1 || counts["missingArtists"] != 1 {
		t.Fatal("missed later row", counts)
	}
	raw, _ := json.Marshal(counts)
	if strings.Contains(string(raw), "private") || strings.Contains(string(raw), "12345") {
		t.Fatal("private values leaked")
	}
}
