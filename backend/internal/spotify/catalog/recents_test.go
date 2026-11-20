package catalog

import (
	"encoding/json"
	"net/url"
	"testing"
)

func TestRecentsQueryBounds(t *testing.T) {
	for _, query := range []string{"limit=0", "limit=101", "offset=-1", "offset=100001", "before=123", "limit=50&limit=100", "limit=NaN"} {
		v, _ := url.ParseQuery(query)
		if _, err := ParseRecentsQuery(v); err != ErrInvalidQuery {
			t.Fatalf("accepted %s", query)
		}
	}
	q, err := ParseRecentsQuery(url.Values{"limit": {"100"}, "offset": {"1200"}})
	if err != nil || q.Limit != 100 || q.Offset != 1200 {
		t.Fatal("valid recents page rejected")
	}
}

func TestRecentDatesGroupsAndUnavailableRows(t *testing.T) {
	for _, body := range []string{
		`{"addedAt":{"year":2026,"month":2,"day":30},"entity":null,"formatListAttributes":[{"key":"children_group_id","value":"5"}]}`,
		`{"addedAt":{"year":2026,"month":11,"day":15},"entity":null}`,
		`{"addedAt":{"year":2026,"month":11,"day":15},"entity":{"_uri":"spotify:track:AAAAAAAAAAAAAAAAAAAAAA","data":{"__typename":"Entity","uri":"spotify:track:BBBBBBBBBBBBBBBBBBBBBB"}}}`,
	} {
		var wire recentWireItem
		if json.Unmarshal([]byte(body), &wire) != nil {
			t.Fatal("bad fixture")
		}
		if _, err := normalizeRecent(wire); err == nil {
			t.Fatal("malformed history accepted")
		}
	}
	var wire recentWireItem
	json.Unmarshal([]byte(`{"addedAt":{"year":2026,"month":11,"day":15},"entity":{"data":{"__typename":"NotFound"}},"formatListAttributes":[{"key":"group_id_5","value":""}]}`), &wire)
	entry, err := normalizeRecent(wire)
	if err != nil || entry.Type != "unavailable" || entry.Date != "2026-11-15" || entry.Track != nil || entry.GroupID != "5" {
		t.Fatal("unavailable entry lost its position")
	}
}
